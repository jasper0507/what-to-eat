// Package meal 是 Meal lifecycle 深模块（ADR-0019/0022）：事务壳、状态机与
// 记账。评分智能（四因子、放宽、相似度、理由）全部住在 internal/engine 纯函数
// 包里；本包只负责拼装快照、抽样落库与 Reroll budget / 降档 / 三出口的账本。
package meal

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"sync"
	"time"

	"github.com/jasper0507/what-to-eat/internal/catalog"
	"github.com/jasper0507/what-to-eat/internal/engine"
	"github.com/jasper0507/what-to-eat/internal/pool"
)

const (
	StatusCandidatePoolEmpty Status = "candidate_pool_empty"
	StatusReady              Status = "ready"
	StatusActiveDecision     Status = "active_decision"
	StatusPendingRatings     Status = "pending_ratings"
	ModePool                 Mode   = "pool"
	ModeDiscovery            Mode   = "discovery"
	ModeHandPick             Mode   = "hand_pick"

	// RerollBudget 是每 Meal 的 Reroll 上限，服务端结算（ADR-0022）。
	RerollBudget = 3
)

const activeDecisionQuery = `
	SELECT decisions.id, meals.id, decisions.mode, decisions.reason,
	       catalog_dishes.source_path, catalog_dishes.name,
	       catalog_dishes.difficulty, catalog_dishes.cook_minutes
	FROM meals
	JOIN decisions ON decisions.meal_id = meals.id
	JOIN catalog_dishes ON catalog_dishes.source_path = decisions.dish_id
	WHERE meals.account_id = ? AND meals.status = 'active' AND decisions.status = 'active'
	      AND decisions.rerolled_to_id IS NULL
`

var (
	ErrCandidatePoolEmpty    = errors.New("Candidate pool is empty")
	ErrDecisionNotFound      = errors.New("Decision not found")
	ErrPendingRatingNotFound = errors.New("Pending rating not found")
	ErrPendingRatings        = errors.New("Pending ratings must be resolved")
	ErrTasteRatingConflict   = errors.New("Taste rating conflicts with the resolved rating")
	ErrRerollBudgetExhausted = errors.New("Reroll budget is exhausted")
	ErrNoActiveMeal          = errors.New("no active Meal")
	ErrHandPickLocked        = errors.New("hand pick unlocks only at budget exhaustion")
	ErrDishNotInPool         = errors.New("Dish is not in the Candidate pool")
	ErrEatingRecordNotFound  = errors.New("Eating record not found")
)

type Lifecycle struct {
	db        *sql.DB
	pool      *pool.Pool
	random    *rand.Rand
	randomMu  sync.Mutex
	discovery DiscoveryConfig
}

type Status string
type Mode string

type DiscoveryConfig struct {
	Enabled               bool
	MaxPoolSize           int
	MaxEligibleDishes     int
	MinRerolls            int
	RecentMealWindow      int
	MaxDiscoveriesPerMeal int
}

func NewDecisionRandom() *rand.Rand {
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

func DefaultDiscoveryConfig() DiscoveryConfig {
	return DiscoveryConfig{
		Enabled:               true,
		MaxPoolSize:           3,
		MaxEligibleDishes:     1,
		MinRerolls:            2,
		RecentMealWindow:      3,
		MaxDiscoveriesPerMeal: 2,
	}
}

func New(
	db *sql.DB,
	candidates *pool.Pool,
	random *rand.Rand,
	discovery DiscoveryConfig,
) *Lifecycle {
	return &Lifecycle{db: db, pool: candidates, random: random, discovery: discovery}
}

type State struct {
	Status           Status          `json:"status"`
	Decision         *Decision       `json:"decision,omitempty"`
	RerollsRemaining *int            `json:"rerolls_remaining,omitempty"`
	PendingRatings   []PendingRating `json:"pending_ratings,omitempty"`
}

type Decision struct {
	ID     int64        `json:"id"`
	MealID int64        `json:"meal_id"`
	Mode   Mode         `json:"mode"`
	Reason string       `json:"reason,omitempty"`
	Dish   catalog.Dish `json:"dish"`
}

type EatingRecord struct {
	Sequence int64 `json:"sequence"`
}

type RecipeRef struct {
	Dish catalog.Dish `json:"dish"`
}

type Acceptance struct {
	EatingRecord  EatingRecord   `json:"eating_record"`
	Recipe        RecipeRef      `json:"recipe"`
	PendingRating *PendingRating `json:"pending_rating,omitempty"`
}

type PendingRating struct {
	ID     int64        `json:"id"`
	MealID int64        `json:"meal_id"`
	MealAt int64        `json:"meal_at"`
	Dish   catalog.Dish `json:"dish"`
}

type TasteRating struct {
	PendingRatingID int64        `json:"pending_rating_id"`
	Rating          int          `json:"rating"`
	Outcome         string       `json:"outcome"`
	Tier            *int         `json:"tier,omitempty"`
	Dish            catalog.Dish `json:"dish"`
}

// HistoryEntry 是轻历史条目：吃过什么、什么模式、评过几档、现在几档。
type HistoryEntry struct {
	ID         int64        `json:"id"`
	Sequence   int64        `json:"sequence"`
	Dish       catalog.Dish `json:"dish"`
	Mode       Mode         `json:"mode"`
	AcceptedAt int64        `json:"accepted_at"`
	Rating     *int         `json:"rating,omitempty"`
	PoolTier   *int         `json:"pool_tier,omitempty"`
}

func intPointer(value int) *int {
	return &value
}

func (m *Lifecycle) randomFloat() float64 {
	m.randomMu.Lock()
	defer m.randomMu.Unlock()
	return m.random.Float64()
}

// weightedPick 按权重轮盘赌抽样：random ∈ [0,1) 定格在总权重轴上，
// 逐项递减落在谁身上就选谁。
func weightedPick[T any](random float64, items []T, weightOf func(T) float64) T {
	total := 0.0
	for _, item := range items {
		total += weightOf(item)
	}
	target := random * total
	selected := items[len(items)-1]
	for _, item := range items {
		target -= weightOf(item)
		if target < 0 {
			selected = item
			break
		}
	}
	return selected
}

func scanActiveDecision(row *sql.Row) (State, error) {
	var decision Decision
	var mode, dishID, dishName string
	var difficulty, cookMinutes sql.NullInt64
	err := row.Scan(
		&decision.ID,
		&decision.MealID,
		&mode,
		&decision.Reason,
		&dishID,
		&dishName,
		&difficulty,
		&cookMinutes,
	)
	if err == nil {
		decision.Mode = Mode(mode)
		decision.Dish = dishWithMeta(dishID, dishName, difficulty, cookMinutes)
		return State{Status: StatusActiveDecision, Decision: &decision}, nil
	}
	return State{}, err
}

func dishWithMeta(
	dishID, dishName string,
	difficulty, cookMinutes sql.NullInt64,
) catalog.Dish {
	dish := catalog.NewDish(dishID, dishName)
	if difficulty.Valid {
		dish.Difficulty = intPointer(int(difficulty.Int64))
	}
	if cookMinutes.Valid {
		dish.CookMinutes = intPointer(int(cookMinutes.Int64))
	}
	return dish
}

// sqlQueryer 让同一读取逻辑既可跑在 *sql.DB 上也可跑在事务内。
type sqlQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (m *Lifecycle) Resume(context context.Context, accountID int64) (State, error) {
	return m.resumeOn(context, m.db, accountID)
}

func (m *Lifecycle) resumeOn(
	context context.Context,
	queryer sqlQueryer,
	accountID int64,
) (State, error) {
	pendingRatings, err := m.unresolvedPendingRatings(context, queryer, accountID)
	if err != nil {
		return State{}, err
	}
	if len(pendingRatings) > 0 {
		return State{
			Status:         StatusPendingRatings,
			PendingRatings: pendingRatings,
		}, nil
	}

	activeDecision, err := scanActiveDecision(
		queryer.QueryRowContext(context, activeDecisionQuery, accountID),
	)
	if err == nil {
		remaining, err := m.rerollsRemaining(context, queryer, activeDecision.Decision.MealID)
		if err != nil {
			return State{}, err
		}
		activeDecision.RerollsRemaining = intPointer(remaining)
		return activeDecision, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return State{}, err
	}

	hasCandidates, err := hasDecidablePool(context, queryer, accountID)
	if err != nil {
		return State{}, err
	}
	if !hasCandidates {
		return State{Status: StatusCandidatePoolEmpty}, nil
	}
	return State{Status: StatusReady}, nil
}

// hasDecidablePool 判定池是否可开饭：与 Begin 的口径一致——只剩永零类
// （酱料/饮品/甜品/半成品）的池视同空池（ADR-0022），Resume 不许报 ready
// 而 Begin 又拒绝，否则开始按钮成死键。
func hasDecidablePool(
	context context.Context,
	queryer sqlQueryer,
	accountID int64,
) (bool, error) {
	rows, err := queryer.QueryContext(
		context,
		`SELECT candidate_pool.dish_id
		 FROM candidate_pool
		 WHERE candidate_pool.account_id = ?
		   AND NOT EXISTS (
			SELECT 1
			FROM rejection_marks
			WHERE rejection_marks.account_id = candidate_pool.account_id
			  AND rejection_marks.dish_id = candidate_pool.dish_id
		   )`,
		accountID,
	)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var dishID string
		if err := rows.Scan(&dishID); err != nil {
			return false, err
		}
		if engine.ClassifyOccasion(catalog.PathCategory(dishID)) != engine.OccasionNever {
			return true, nil
		}
	}
	return false, rows.Err()
}

// rerollsRemaining 从 decisions 账本读出该 Meal 的剩余 Reroll 额度。
func (m *Lifecycle) rerollsRemaining(
	context context.Context,
	queryer sqlQueryer,
	mealID int64,
) (int, error) {
	var used int
	err := queryer.QueryRowContext(
		context,
		"SELECT COUNT(*) FROM decisions WHERE meal_id = ? AND rerolled_to_id IS NOT NULL",
		mealID,
	).Scan(&used)
	return max(0, RerollBudget-used), err
}

func (m *Lifecycle) unresolvedPendingRatings(
	context context.Context,
	queryer sqlQueryer,
	accountID int64,
) ([]PendingRating, error) {
	rows, err := queryer.QueryContext(
		context,
		`SELECT pending_ratings.id, pending_ratings.meal_id, pending_ratings.meal_at,
		        catalog_dishes.source_path, catalog_dishes.name
		 FROM pending_ratings
		 JOIN catalog_dishes ON catalog_dishes.source_path = pending_ratings.dish_id
		 WHERE pending_ratings.account_id = ? AND pending_ratings.rating IS NULL
		 ORDER BY pending_ratings.meal_at, pending_ratings.id`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pendingRatings := make([]PendingRating, 0)
	for rows.Next() {
		var pending PendingRating
		var dishID, dishName string
		if err := rows.Scan(
			&pending.ID,
			&pending.MealID,
			&pending.MealAt,
			&dishID,
			&dishName,
		); err != nil {
			return nil, err
		}
		pending.Dish = catalog.NewDish(dishID, dishName)
		pendingRatings = append(pendingRatings, pending)
	}
	return pendingRatings, rows.Err()
}

func pendingRatingForDecision(
	context context.Context,
	transaction *sql.Tx,
	accountID, decisionID int64,
) (*PendingRating, error) {
	var pending PendingRating
	var dishID, dishName string
	err := transaction.QueryRowContext(
		context,
		`SELECT pending_ratings.id, pending_ratings.meal_id, pending_ratings.meal_at,
		        catalog_dishes.source_path, catalog_dishes.name
		 FROM pending_ratings
		 JOIN catalog_dishes ON catalog_dishes.source_path = pending_ratings.dish_id
		 WHERE pending_ratings.account_id = ? AND pending_ratings.decision_id = ?`,
		accountID,
		decisionID,
	).Scan(
		&pending.ID,
		&pending.MealID,
		&pending.MealAt,
		&dishID,
		&dishName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pending.Dish = catalog.NewDish(dishID, dishName)
	return &pending, nil
}

// poolSnapshot 是一次揭示的评分输入：池成员的引擎候选快照与新鲜感账本。
type poolSnapshot struct {
	candidates []engine.Candidate
	tiers      map[string]int
	// freshnessEligible 是新鲜感 > 0 的池菜数（探索压力信号之一）。
	freshnessEligible int
	// lastAccepted / totalRecords 供 Discovery 复算任意菜的新鲜感。
	lastAccepted map[string]int64
	totalRecords int64
}

func (s poolSnapshot) freshnessOf(dishID string) float64 {
	lastSequence, everEaten := s.lastAccepted[dishID]
	return engine.FreshnessFactor(int(s.totalRecords-lastSequence), everEaten)
}

func (m *Lifecycle) poolCandidates(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
	shown map[string]int,
) (poolSnapshot, error) {
	snapshot := poolSnapshot{
		tiers:        make(map[string]int),
		lastAccepted: make(map[string]int64),
	}

	if err := transaction.QueryRowContext(
		context,
		"SELECT COALESCE(MAX(sequence), 0) FROM eating_records WHERE account_id = ?",
		accountID,
	).Scan(&snapshot.totalRecords); err != nil {
		return poolSnapshot{}, err
	}
	historyRows, err := transaction.QueryContext(
		context,
		`SELECT dish_id, MAX(sequence)
		 FROM eating_records
		 WHERE account_id = ?
		 GROUP BY dish_id`,
		accountID,
	)
	if err != nil {
		return poolSnapshot{}, err
	}
	for historyRows.Next() {
		var dishID string
		var lastSequence int64
		if err := historyRows.Scan(&dishID, &lastSequence); err != nil {
			historyRows.Close()
			return poolSnapshot{}, err
		}
		snapshot.lastAccepted[dishID] = lastSequence
	}
	if err := errors.Join(historyRows.Close(), historyRows.Err()); err != nil {
		return poolSnapshot{}, err
	}

	rows, err := transaction.QueryContext(
		context,
		`SELECT catalog_dishes.source_path, catalog_dishes.name, candidate_pool.tier
		 FROM candidate_pool
		 JOIN catalog_dishes ON catalog_dishes.source_path = candidate_pool.dish_id
		 WHERE candidate_pool.account_id = ?
		   AND NOT EXISTS (
			SELECT 1
			FROM rejection_marks
			WHERE rejection_marks.account_id = candidate_pool.account_id
			  AND rejection_marks.dish_id = candidate_pool.dish_id
		   )
		 ORDER BY catalog_dishes.source_path`,
		accountID,
	)
	if err != nil {
		return poolSnapshot{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var candidate engine.Candidate
		if err := rows.Scan(&candidate.ID, &candidate.Name, &candidate.Tier); err != nil {
			return poolSnapshot{}, err
		}
		lastSequence, everEaten := snapshot.lastAccepted[candidate.ID]
		candidate.EverEaten = everEaten
		candidate.Distance = int(snapshot.totalRecords - lastSequence)
		candidate.Occasion = engine.ClassifyOccasion(catalog.PathCategory(candidate.ID))
		candidate.ShownThisMeal = shown[candidate.ID] > 0
		snapshot.tiers[candidate.ID] = candidate.Tier
		snapshot.candidates = append(snapshot.candidates, candidate)
		if engine.FreshnessFactor(candidate.Distance, candidate.EverEaten) > 0 {
			snapshot.freshnessEligible++
		}
	}
	return snapshot, rows.Err()
}

// recentRerolls 统计探索压力的 Reroll 信号：窗口为最近 N 个已了结
// （接受或放弃）的 Meal——放弃也是响亮的「没懂我」（ADR-0022 修正案）。
func (m *Lifecycle) recentRerolls(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
) (int, error) {
	if !m.discovery.Enabled || m.discovery.RecentMealWindow <= 0 {
		return 0, nil
	}
	var rerolls int
	err := transaction.QueryRowContext(
		context,
		`SELECT COUNT(*)
		 FROM decisions
		 WHERE rerolled_to_id IS NOT NULL AND meal_id IN (
			SELECT id
			FROM meals
			WHERE account_id = ? AND status IN ('accepted', 'abandoned')
			ORDER BY id DESC
			LIMIT ?
		 )`,
		accountID,
		m.discovery.RecentMealWindow,
	).Scan(&rerolls)
	return rerolls, err
}

// mealShownCounts 统计本 Meal 内已展示的 Dish（本顿否决的输入）与 Discovery 数。
func mealShownCounts(
	context context.Context,
	transaction *sql.Tx,
	mealID int64,
) (shown map[string]int, decisionCount, discoveryCount int, err error) {
	rows, err := transaction.QueryContext(
		context,
		"SELECT dish_id, mode FROM decisions WHERE meal_id = ?",
		mealID,
	)
	if err != nil {
		return nil, 0, 0, err
	}
	shown = make(map[string]int)
	for rows.Next() {
		var dishID, mode string
		if err := rows.Scan(&dishID, &mode); err != nil {
			rows.Close()
			return nil, 0, 0, err
		}
		shown[dishID]++
		decisionCount++
		if mode == string(ModeDiscovery) {
			discoveryCount++
		}
	}
	if err := rows.Close(); err != nil {
		return nil, 0, 0, err
	}
	return shown, decisionCount, discoveryCount, rows.Err()
}

// discoverySignals 数探索压力信号：池小 / 新鲜感可选少 / 近窗 Reroll 多。
func (m *Lifecycle) discoverySignals(snapshot poolSnapshot, rerolls int) int {
	if !m.discovery.Enabled {
		return 0
	}
	signals := 0
	if len(snapshot.candidates) <= m.discovery.MaxPoolSize {
		signals++
	}
	if snapshot.freshnessEligible <= m.discovery.MaxEligibleDishes {
		signals++
	}
	if rerolls >= m.discovery.MinRerolls {
		signals++
	}
	return signals
}

type decisionChoice struct {
	dishID string
	name   string
	mode   Mode
	reason string
}

// discoveryDish 从池外候选按 相似度×参照喜爱×场合×新鲜感 加权抽样。
func (m *Lifecycle) discoveryDish(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
	snapshot poolSnapshot,
	shown map[string]int,
	hour int,
) (decisionChoice, bool, error) {
	profiles, names, err := catalog.Profiles(context, transaction)
	if err != nil {
		return decisionChoice{}, false, err
	}

	rejectedRows, err := transaction.QueryContext(
		context,
		"SELECT dish_id FROM rejection_marks WHERE account_id = ?",
		accountID,
	)
	if err != nil {
		return decisionChoice{}, false, err
	}
	rejected := make(map[string]bool)
	for rejectedRows.Next() {
		var dishID string
		if err := rejectedRows.Scan(&dishID); err != nil {
			rejectedRows.Close()
			return decisionChoice{}, false, err
		}
		rejected[dishID] = true
	}
	if err := errors.Join(rejectedRows.Close(), rejectedRows.Err()); err != nil {
		return decisionChoice{}, false, err
	}

	type discoveryCandidate struct {
		id, name  string
		weight    float64
		reference string
		hits      engine.SimilarityHits
	}
	candidates := make([]discoveryCandidate, 0)
	for dishID, profile := range profiles {
		if _, inPool := snapshot.tiers[dishID]; inPool || rejected[dishID] || shown[dishID] > 0 {
			continue
		}
		occasion := engine.OccasionFactor(
			engine.ClassifyOccasion(catalog.PathCategory(dishID)),
			hour,
		)
		if occasion == 0 {
			continue
		}
		freshness := snapshot.freshnessOf(dishID)
		if freshness == 0 {
			continue
		}
		best := discoveryCandidate{id: dishID, name: names[dishID]}
		for _, reference := range snapshot.candidates {
			similarity, hits := engine.Similarity(profile, profiles[reference.ID])
			weighted := similarity * engine.TierMultiplier(reference.Tier)
			if weighted > best.weight {
				best.weight = weighted
				best.reference = reference.Name
				best.hits = hits
			}
		}
		if best.weight <= 0 {
			continue
		}
		best.weight *= occasion * freshness
		candidates = append(candidates, best)
	}
	if len(candidates) == 0 {
		return decisionChoice{}, false, nil
	}

	selected := weightedPick(
		m.randomFloat(),
		candidates,
		func(candidate discoveryCandidate) float64 { return candidate.weight },
	)
	reason := engine.ComposeReason(engine.ReasonInput{
		Discovery: &engine.DiscoveryReason{
			ReferenceName: selected.reference,
			Hits:          selected.hits,
		},
	})
	return decisionChoice{
		dishID: selected.id,
		name:   selected.name,
		mode:   ModeDiscovery,
		reason: reason,
	}, true, nil
}

type selectionInput struct {
	snapshot       poolSnapshot
	shown          map[string]int
	excludeDishID  string
	rerolls        int
	discoveryCount int
	hour           int
}

// chooseDecision 先掷探索概率，未中或无候选则回落池内四因子抽样。
func (m *Lifecycle) chooseDecision(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
	input selectionInput,
) (decisionChoice, error) {
	signals := m.discoverySignals(input.snapshot, input.rerolls)
	if signals > 0 &&
		input.discoveryCount < m.discovery.MaxDiscoveriesPerMeal &&
		m.randomFloat() < engine.DiscoveryProbability(signals) {
		discovery, found, err := m.discoveryDish(
			context,
			transaction,
			accountID,
			input.snapshot,
			input.shown,
			input.hour,
		)
		if err != nil {
			return decisionChoice{}, err
		}
		if found {
			return discovery, nil
		}
	}

	weighted, relaxation := engine.ScorePool(input.snapshot.candidates, input.hour)
	// 放宽到本顿否决时才可能重现已展示的菜；只要还有别的选择，就不把
	// 正被换掉的那道原样端回来。
	if input.excludeDishID != "" && len(weighted) > 1 {
		remaining := make([]engine.Weighted, 0, len(weighted))
		for _, entry := range weighted {
			if entry.Candidate.ID != input.excludeDishID {
				remaining = append(remaining, entry)
			}
		}
		if len(remaining) > 0 {
			weighted = remaining
		}
	}
	if len(weighted) == 0 {
		return decisionChoice{}, ErrCandidatePoolEmpty
	}

	selected := weightedPick(
		m.randomFloat(),
		weighted,
		func(entry engine.Weighted) float64 { return entry.Weight },
	)
	reason := engine.ComposeReason(engine.ReasonInput{
		Relaxation: relaxation,
		Tier:       selected.Candidate.Tier,
		Distance:   selected.Candidate.Distance,
		EverEaten:  selected.Candidate.EverEaten,
		Occasion:   selected.Candidate.Occasion,
		Hour:       input.hour,
	})
	return decisionChoice{
		dishID: selected.Candidate.ID,
		name:   selected.Candidate.Name,
		mode:   ModePool,
		reason: reason,
	}, nil
}

func (m *Lifecycle) insertDecision(
	context context.Context,
	transaction *sql.Tx,
	mealID int64,
	choice decisionChoice,
) (Decision, error) {
	result, err := transaction.ExecContext(
		context,
		`INSERT INTO decisions (meal_id, dish_id, mode, reason, status, created_at)
		 VALUES (?, ?, ?, ?, 'active', unixepoch())`,
		mealID,
		choice.dishID,
		string(choice.mode),
		choice.reason,
	)
	if err != nil {
		return Decision{}, err
	}
	decision := Decision{
		MealID: mealID,
		Mode:   choice.mode,
		Reason: choice.reason,
	}
	decision.ID, err = result.LastInsertId()
	if err != nil {
		return Decision{}, err
	}
	var difficulty, cookMinutes sql.NullInt64
	if err := transaction.QueryRowContext(
		context,
		"SELECT difficulty, cook_minutes FROM catalog_dishes WHERE source_path = ?",
		choice.dishID,
	).Scan(&difficulty, &cookMinutes); err != nil {
		return Decision{}, err
	}
	decision.Dish = dishWithMeta(choice.dishID, choice.name, difficulty, cookMinutes)
	return decision, nil
}

func (m *Lifecycle) Begin(
	context context.Context,
	accountID int64,
	localHour int,
) (state State, created bool, err error) {
	transaction, err := m.db.BeginTx(context, nil)
	if err != nil {
		return State{}, false, err
	}
	defer transaction.Rollback()

	pendingRatings, err := m.unresolvedPendingRatings(context, transaction, accountID)
	if err != nil {
		return State{}, false, err
	}
	if len(pendingRatings) > 0 {
		return State{
			Status:         StatusPendingRatings,
			PendingRatings: pendingRatings,
		}, false, ErrPendingRatings
	}

	activeDecision, err := scanActiveDecision(
		transaction.QueryRowContext(context, activeDecisionQuery, accountID),
	)
	if err == nil {
		remaining, err := m.rerollsRemaining(context, transaction, activeDecision.Decision.MealID)
		if err != nil {
			return State{}, false, err
		}
		activeDecision.RerollsRemaining = intPointer(remaining)
		return activeDecision, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return State{}, false, err
	}

	snapshot, err := m.poolCandidates(context, transaction, accountID, nil)
	if err != nil {
		return State{}, false, err
	}
	if len(snapshot.candidates) == 0 {
		return State{Status: StatusCandidatePoolEmpty}, false, ErrCandidatePoolEmpty
	}
	recentRerolls, err := m.recentRerolls(context, transaction, accountID)
	if err != nil {
		return State{}, false, err
	}
	choice, err := m.chooseDecision(context, transaction, accountID, selectionInput{
		snapshot: snapshot,
		rerolls:  recentRerolls,
		hour:     localHour,
	})
	if errors.Is(err, ErrCandidatePoolEmpty) {
		// 池里只剩永零类（酱料/饮品/甜品/半成品）：视同空池
		return State{Status: StatusCandidatePoolEmpty}, false, ErrCandidatePoolEmpty
	}
	if err != nil {
		return State{}, false, err
	}

	mealResult, err := transaction.ExecContext(
		context,
		"INSERT INTO meals (account_id, status, created_at) VALUES (?, 'active', unixepoch())",
		accountID,
	)
	if err != nil {
		return State{}, false, err
	}
	mealID, err := mealResult.LastInsertId()
	if err != nil {
		return State{}, false, err
	}
	decision, err := m.insertDecision(context, transaction, mealID, choice)
	if err != nil {
		return State{}, false, err
	}
	if err := transaction.Commit(); err != nil {
		return State{}, false, err
	}
	return State{
		Status:           StatusActiveDecision,
		Decision:         &decision,
		RerollsRemaining: intPointer(RerollBudget),
	}, true, nil
}

func (m *Lifecycle) Reroll(
	context context.Context,
	accountID, decisionID int64,
	localHour int,
) (State, error) {
	transaction, err := m.db.BeginTx(context, nil)
	if err != nil {
		return State{}, err
	}
	defer transaction.Rollback()

	var mealID int64
	var mealStatus, decisionStatus, currentDishID, currentMode string
	var replacementID sql.NullInt64
	err = transaction.QueryRowContext(
		context,
		`SELECT meals.id, meals.status, decisions.status, decisions.dish_id,
		        decisions.mode, decisions.rerolled_to_id
		 FROM decisions
		 JOIN meals ON meals.id = decisions.meal_id
		 WHERE decisions.id = ? AND meals.account_id = ?`,
		decisionID,
		accountID,
	).Scan(&mealID, &mealStatus, &decisionStatus, &currentDishID, &currentMode, &replacementID)
	if errors.Is(err, sql.ErrNoRows) {
		return State{}, ErrDecisionNotFound
	}
	if err != nil {
		return State{}, err
	}
	if replacementID.Valid {
		replacement, err := scanActiveDecision(transaction.QueryRowContext(
			context,
			activeDecisionQuery+" AND decisions.id = ?",
			accountID,
			replacementID.Int64,
		))
		if errors.Is(err, sql.ErrNoRows) {
			return State{}, ErrDecisionNotFound
		}
		if err != nil {
			return State{}, err
		}
		remaining, err := m.rerollsRemaining(context, transaction, mealID)
		if err != nil {
			return State{}, err
		}
		replacement.RerollsRemaining = intPointer(remaining)
		return replacement, nil
	}
	if mealStatus != "active" || decisionStatus != "active" {
		return State{}, ErrDecisionNotFound
	}

	shown, decisionCount, discoveryCount, err := mealShownCounts(context, transaction, mealID)
	if err != nil {
		return State{}, err
	}
	completedRerolls := max(0, decisionCount-1)
	if completedRerolls >= RerollBudget {
		return State{}, ErrRerollBudgetExhausted
	}

	snapshot, err := m.poolCandidates(context, transaction, accountID, shown)
	if err != nil {
		return State{}, err
	}
	if len(snapshot.candidates) == 0 {
		return State{Status: StatusCandidatePoolEmpty}, ErrCandidatePoolEmpty
	}

	recentRerolls, err := m.recentRerolls(context, transaction, accountID)
	if err != nil {
		return State{}, err
	}
	choice, err := m.chooseDecision(context, transaction, accountID, selectionInput{
		snapshot:       snapshot,
		shown:          shown,
		excludeDishID:  currentDishID,
		rerolls:        completedRerolls + 1 + recentRerolls,
		discoveryCount: discoveryCount,
		hour:           localHour,
	})
	if err != nil {
		if errors.Is(err, ErrCandidatePoolEmpty) {
			return State{Status: StatusCandidatePoolEmpty}, ErrCandidatePoolEmpty
		}
		return State{}, err
	}

	if _, err := transaction.ExecContext(
		context,
		"UPDATE decisions SET rerolled_to_id = id WHERE id = ? AND rerolled_to_id IS NULL",
		decisionID,
	); err != nil {
		return State{}, err
	}
	decision, err := m.insertDecision(context, transaction, mealID, choice)
	if err != nil {
		return State{}, err
	}
	if _, err := transaction.ExecContext(
		context,
		"UPDATE decisions SET rerolled_to_id = ? WHERE id = ? AND rerolled_to_id = id",
		decision.ID,
		decisionID,
	); err != nil {
		return State{}, err
	}
	// 自动降档记账：pool 模式的展示被换 +1；连换达阈值降一档并清零
	if currentMode == string(ModePool) {
		if err := m.recordSwap(context, transaction, accountID, currentDishID); err != nil {
			return State{}, err
		}
	}
	if err := transaction.Commit(); err != nil {
		return State{}, err
	}

	return State{
		Status:           StatusActiveDecision,
		Decision:         &decision,
		RerollsRemaining: intPointer(RerollBudget - completedRerolls - 1),
	}, nil
}

// recordSwap 落一次「被换」，达阈值时执行降档（地板人上人）并清零计数。
// 菜已不在池中（被移除/被拒）时计数照落、降档自然无事发生。
func (m *Lifecycle) recordSwap(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
	dishID string,
) error {
	if _, err := transaction.ExecContext(
		context,
		`INSERT INTO pool_demotions (account_id, dish_id, swaps)
		 VALUES (?, ?, 1)
		 ON CONFLICT(account_id, dish_id) DO UPDATE SET swaps = swaps + 1`,
		accountID,
		dishID,
	); err != nil {
		return err
	}
	var swaps int
	if err := transaction.QueryRowContext(
		context,
		"SELECT swaps FROM pool_demotions WHERE account_id = ? AND dish_id = ?",
		accountID,
		dishID,
	).Scan(&swaps); err != nil {
		return err
	}
	if swaps < engine.DemotionSwapThreshold {
		return nil
	}
	if _, err := transaction.ExecContext(
		context,
		`UPDATE candidate_pool
		 SET tier = MAX(?, tier - 1)
		 WHERE account_id = ? AND dish_id = ?`,
		engine.TierRenShangRen,
		accountID,
		dishID,
	); err != nil {
		return err
	}
	_, err := transaction.ExecContext(
		context,
		"UPDATE pool_demotions SET swaps = 0 WHERE account_id = ? AND dish_id = ?",
		accountID,
		dishID,
	)
	return err
}

// resetSwaps 任何接受即清零该菜的连换计数。
func resetSwaps(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
	dishID string,
) error {
	_, err := transaction.ExecContext(
		context,
		"DELETE FROM pool_demotions WHERE account_id = ? AND dish_id = ?",
		accountID,
		dishID,
	)
	return err
}

func (m *Lifecycle) Accept(
	context context.Context,
	accountID, decisionID int64,
) (Acceptance, error) {
	transaction, err := m.db.BeginTx(context, nil)
	if err != nil {
		return Acceptance{}, err
	}
	defer transaction.Rollback()

	var mealID int64
	var mealStatus, status, mode, dishID, dishName string
	var existingSequence, rerolledToID sql.NullInt64
	err = transaction.QueryRowContext(
		context,
		`SELECT meals.id, meals.status, decisions.status, decisions.mode,
		        catalog_dishes.source_path, catalog_dishes.name,
		        eating_records.sequence, decisions.rerolled_to_id
		 FROM decisions
		 JOIN meals ON meals.id = decisions.meal_id
		 JOIN catalog_dishes ON catalog_dishes.source_path = decisions.dish_id
		 LEFT JOIN eating_records ON eating_records.decision_id = decisions.id
		 WHERE decisions.id = ? AND meals.account_id = ?`,
		decisionID,
		accountID,
	).Scan(&mealID, &mealStatus, &status, &mode, &dishID, &dishName, &existingSequence, &rerolledToID)
	if errors.Is(err, sql.ErrNoRows) {
		return Acceptance{}, ErrDecisionNotFound
	}
	if err != nil {
		return Acceptance{}, err
	}

	result := Acceptance{
		Recipe: RecipeRef{
			Dish: catalog.NewDish(dishID, dishName),
		},
	}
	if rerolledToID.Valid {
		return Acceptance{}, ErrDecisionNotFound
	}
	if status == "accepted" && existingSequence.Valid {
		result.EatingRecord.Sequence = existingSequence.Int64
		result.PendingRating, err = pendingRatingForDecision(
			context,
			transaction,
			accountID,
			decisionID,
		)
		if err != nil {
			return Acceptance{}, err
		}
		return result, nil
	}
	// Meal 状态守卫必须在幂等重放分支之后：合法接受后 Meal 已是 accepted。
	// 已放弃/已了结的 Meal 上站着的旧 Decision 一律 404——否则放弃的这顿会
	// 落下幽灵吃饭记录，违反「放弃：无吃饭记录、不进冷却」（ADR-0022）。
	if mealStatus != "active" || status != "active" {
		return Acceptance{}, ErrDecisionNotFound
	}

	if err := transaction.QueryRowContext(
		context,
		"SELECT COALESCE(MAX(sequence), 0) + 1 FROM eating_records WHERE account_id = ?",
		accountID,
	).Scan(&result.EatingRecord.Sequence); err != nil {
		return Acceptance{}, err
	}
	if _, err := transaction.ExecContext(
		context,
		`INSERT INTO eating_records (
			account_id, sequence, meal_id, decision_id, dish_id, accepted_at
		 ) VALUES (?, ?, ?, ?, ?, unixepoch())`,
		accountID,
		result.EatingRecord.Sequence,
		mealID,
		decisionID,
		dishID,
	); err != nil {
		return Acceptance{}, err
	}
	if _, err := transaction.ExecContext(
		context,
		"UPDATE decisions SET status = 'accepted' WHERE id = ? AND status = 'active'",
		decisionID,
	); err != nil {
		return Acceptance{}, err
	}
	if _, err := transaction.ExecContext(
		context,
		"UPDATE meals SET status = 'accepted' WHERE id = ? AND status = 'active'",
		mealID,
	); err != nil {
		return Acceptance{}, err
	}
	if err := resetSwaps(context, transaction, accountID, dishID); err != nil {
		return Acceptance{}, err
	}
	if mode == string(ModeDiscovery) {
		if _, err := transaction.ExecContext(
			context,
			`INSERT INTO pending_ratings (
				account_id, meal_id, decision_id, dish_id, meal_at
			 )
			 SELECT account_id, meal_id, decision_id, dish_id, accepted_at
			 FROM eating_records
			 WHERE decision_id = ?`,
			decisionID,
		); err != nil {
			return Acceptance{}, err
		}
		result.PendingRating, err = pendingRatingForDecision(
			context,
			transaction,
			accountID,
			decisionID,
		)
		if err != nil {
			return Acceptance{}, err
		}
	}
	if err := transaction.Commit(); err != nil {
		return Acceptance{}, err
	}
	return result, nil
}

// Abandon 放弃本顿：Meal → abandoned，无吃饭记录，不进冷却；站着的那道菜
// 不计入降档。返回放弃后的最新状态。
func (m *Lifecycle) Abandon(context context.Context, accountID int64) (State, error) {
	result, err := m.db.ExecContext(
		context,
		"UPDATE meals SET status = 'abandoned' WHERE account_id = ? AND status = 'active'",
		accountID,
	)
	if err != nil {
		return State{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return State{}, err
	}
	if affected == 0 {
		return State{}, ErrNoActiveMeal
	}
	return m.Resume(context, accountID)
}

// HandPick 亲自点一道：仅 Reroll budget 耗尽时解锁，从池中手选一道成为
// 本顿结局，落 hand_pick 模式的已接受 Decision 与正常吃饭记录。
func (m *Lifecycle) HandPick(
	context context.Context,
	accountID int64,
	dishID string,
) (Acceptance, error) {
	transaction, err := m.db.BeginTx(context, nil)
	if err != nil {
		return Acceptance{}, err
	}
	defer transaction.Rollback()

	var mealID int64
	err = transaction.QueryRowContext(
		context,
		"SELECT id FROM meals WHERE account_id = ? AND status = 'active'",
		accountID,
	).Scan(&mealID)
	if errors.Is(err, sql.ErrNoRows) {
		return Acceptance{}, ErrNoActiveMeal
	}
	if err != nil {
		return Acceptance{}, err
	}

	remaining, err := m.rerollsRemaining(context, transaction, mealID)
	if err != nil {
		return Acceptance{}, err
	}
	if remaining > 0 {
		return Acceptance{}, ErrHandPickLocked
	}

	var dishName string
	err = transaction.QueryRowContext(
		context,
		`SELECT catalog_dishes.name
		 FROM candidate_pool
		 JOIN catalog_dishes ON catalog_dishes.source_path = candidate_pool.dish_id
		 WHERE candidate_pool.account_id = ? AND candidate_pool.dish_id = ?
		   AND NOT EXISTS (
			SELECT 1 FROM rejection_marks
			WHERE rejection_marks.account_id = ? AND rejection_marks.dish_id = ?
		   )`,
		accountID,
		dishID,
		accountID,
		dishID,
	).Scan(&dishName)
	if errors.Is(err, sql.ErrNoRows) {
		return Acceptance{}, ErrDishNotInPool
	}
	if err != nil {
		return Acceptance{}, err
	}

	decisionResult, err := transaction.ExecContext(
		context,
		`INSERT INTO decisions (meal_id, dish_id, mode, reason, status, created_at)
		 VALUES (?, ?, ?, ?, 'accepted', unixepoch())`,
		mealID,
		dishID,
		string(ModeHandPick),
		"你自己点的。",
	)
	if err != nil {
		return Acceptance{}, err
	}
	handPickDecisionID, err := decisionResult.LastInsertId()
	if err != nil {
		return Acceptance{}, err
	}
	// 站着的最后一次揭示被手选取代：按 Reroll 的既有建模标记 rerolled_to_id，
	// 维持「active Decision 只存在于 active Meal」的不变量，陈旧 accept 得到
	// 干净的 404 而不是撞 eating_records.meal_id 唯一约束的 500。
	if _, err := transaction.ExecContext(
		context,
		`UPDATE decisions SET rerolled_to_id = ?
		 WHERE meal_id = ? AND status = 'active' AND rerolled_to_id IS NULL`,
		handPickDecisionID,
		mealID,
	); err != nil {
		return Acceptance{}, err
	}

	result := Acceptance{Recipe: RecipeRef{Dish: catalog.NewDish(dishID, dishName)}}
	if err := transaction.QueryRowContext(
		context,
		"SELECT COALESCE(MAX(sequence), 0) + 1 FROM eating_records WHERE account_id = ?",
		accountID,
	).Scan(&result.EatingRecord.Sequence); err != nil {
		return Acceptance{}, err
	}
	if _, err := transaction.ExecContext(
		context,
		`INSERT INTO eating_records (
			account_id, sequence, meal_id, decision_id, dish_id, accepted_at
		 ) VALUES (?, ?, ?, ?, ?, unixepoch())`,
		accountID,
		result.EatingRecord.Sequence,
		mealID,
		handPickDecisionID,
		dishID,
	); err != nil {
		return Acceptance{}, err
	}
	if _, err := transaction.ExecContext(
		context,
		"UPDATE meals SET status = 'accepted' WHERE id = ? AND status = 'active'",
		mealID,
	); err != nil {
		return Acceptance{}, err
	}
	if err := resetSwaps(context, transaction, accountID, dishID); err != nil {
		return Acceptance{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Acceptance{}, err
	}
	return result, nil
}

// History 返回轻历史：最近的吃饭记录、评分与当前池档。
func (m *Lifecycle) History(
	context context.Context,
	accountID int64,
	limit int,
) ([]HistoryEntry, error) {
	rows, err := m.db.QueryContext(
		context,
		`SELECT eating_records.id, eating_records.sequence, eating_records.dish_id,
		        catalog_dishes.name, decisions.mode, eating_records.accepted_at,
		        pending_ratings.rating, candidate_pool.tier
		 FROM eating_records
		 JOIN catalog_dishes ON catalog_dishes.source_path = eating_records.dish_id
		 JOIN decisions ON decisions.id = eating_records.decision_id
		 LEFT JOIN pending_ratings ON pending_ratings.decision_id = eating_records.decision_id
		 LEFT JOIN candidate_pool ON candidate_pool.account_id = eating_records.account_id
		        AND candidate_pool.dish_id = eating_records.dish_id
		 WHERE eating_records.account_id = ?
		 ORDER BY eating_records.sequence DESC
		 LIMIT ?`,
		accountID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]HistoryEntry, 0)
	for rows.Next() {
		var entry HistoryEntry
		var dishID, dishName, mode string
		var rating, tier sql.NullInt64
		if err := rows.Scan(
			&entry.ID,
			&entry.Sequence,
			&dishID,
			&dishName,
			&mode,
			&entry.AcceptedAt,
			&rating,
			&tier,
		); err != nil {
			return nil, err
		}
		entry.Dish = catalog.NewDish(dishID, dishName)
		entry.Mode = Mode(mode)
		if rating.Valid {
			entry.Rating = intPointer(int(rating.Int64))
		}
		if tier.Valid {
			entry.PoolTier = intPointer(int(tier.Int64))
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// applyTasteRating 是评分落库的唯一实现：上三档 admit 定档，下两档拒绝。
func (m *Lifecycle) applyTasteRating(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
	dishID string,
	rating int,
) error {
	if engine.ValidTier(rating) {
		err := m.pool.Admit(context, transaction, accountID, dishID, rating)
		if errors.Is(err, pool.ErrDishRejected) {
			return ErrTasteRatingConflict
		}
		return err
	}
	return m.pool.Reject(context, transaction, accountID, dishID, rating)
}

func (m *Lifecycle) Rate(
	context context.Context,
	accountID, pendingRatingID int64,
	rating int,
) (TasteRating, error) {
	transaction, err := m.db.BeginTx(context, nil)
	if err != nil {
		return TasteRating{}, err
	}
	defer transaction.Rollback()

	var existingRating sql.NullInt64
	var dishID, dishName string
	err = transaction.QueryRowContext(
		context,
		`SELECT pending_ratings.rating, catalog_dishes.source_path, catalog_dishes.name
		 FROM pending_ratings
		 JOIN catalog_dishes ON catalog_dishes.source_path = pending_ratings.dish_id
		 WHERE pending_ratings.id = ? AND pending_ratings.account_id = ?`,
		pendingRatingID,
		accountID,
	).Scan(&existingRating, &dishID, &dishName)
	if errors.Is(err, sql.ErrNoRows) {
		return TasteRating{}, ErrPendingRatingNotFound
	}
	if err != nil {
		return TasteRating{}, err
	}
	if existingRating.Valid {
		if int(existingRating.Int64) != rating {
			return TasteRating{}, ErrTasteRatingConflict
		}
		return newTasteRating(pendingRatingID, rating, dishID, dishName), nil
	}

	if err := m.applyTasteRating(context, transaction, accountID, dishID, rating); err != nil {
		return TasteRating{}, err
	}
	if _, err := transaction.ExecContext(
		context,
		`UPDATE pending_ratings
		 SET rating = ?, resolved_at = unixepoch()
		 WHERE id = ? AND account_id = ? AND rating IS NULL`,
		rating,
		pendingRatingID,
		accountID,
	); err != nil {
		return TasteRating{}, err
	}
	if err := transaction.Commit(); err != nil {
		return TasteRating{}, err
	}
	return newTasteRating(pendingRatingID, rating, dishID, dishName), nil
}

// RateRecord 是轻历史的补评分：可选、绝不拦路。已有未解决的 Pending rating
// 时等价于解决它；从未有评分行时补一条已解决的评分（不产生拦截）。
func (m *Lifecycle) RateRecord(
	context context.Context,
	accountID, eatingRecordID int64,
	rating int,
) (TasteRating, error) {
	transaction, err := m.db.BeginTx(context, nil)
	if err != nil {
		return TasteRating{}, err
	}
	defer transaction.Rollback()

	var decisionID, mealID, acceptedAt int64
	var dishID, dishName string
	err = transaction.QueryRowContext(
		context,
		`SELECT eating_records.decision_id, eating_records.meal_id,
		        eating_records.accepted_at, eating_records.dish_id, catalog_dishes.name
		 FROM eating_records
		 JOIN catalog_dishes ON catalog_dishes.source_path = eating_records.dish_id
		 WHERE eating_records.id = ? AND eating_records.account_id = ?`,
		eatingRecordID,
		accountID,
	).Scan(&decisionID, &mealID, &acceptedAt, &dishID, &dishName)
	if errors.Is(err, sql.ErrNoRows) {
		return TasteRating{}, ErrEatingRecordNotFound
	}
	if err != nil {
		return TasteRating{}, err
	}

	var pendingID int64
	var existingRating sql.NullInt64
	err = transaction.QueryRowContext(
		context,
		"SELECT id, rating FROM pending_ratings WHERE decision_id = ?",
		decisionID,
	).Scan(&pendingID, &existingRating)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if err := m.applyTasteRating(context, transaction, accountID, dishID, rating); err != nil {
			return TasteRating{}, err
		}
		result, err := transaction.ExecContext(
			context,
			`INSERT INTO pending_ratings (
				account_id, meal_id, decision_id, dish_id, meal_at, rating, resolved_at
			 ) VALUES (?, ?, ?, ?, ?, ?, unixepoch())`,
			accountID,
			mealID,
			decisionID,
			dishID,
			acceptedAt,
			rating,
		)
		if err != nil {
			return TasteRating{}, err
		}
		pendingID, err = result.LastInsertId()
		if err != nil {
			return TasteRating{}, err
		}
	case err != nil:
		return TasteRating{}, err
	case existingRating.Valid:
		if int(existingRating.Int64) != rating {
			return TasteRating{}, ErrTasteRatingConflict
		}
		return newTasteRating(pendingID, rating, dishID, dishName), nil
	default:
		if err := m.applyTasteRating(context, transaction, accountID, dishID, rating); err != nil {
			return TasteRating{}, err
		}
		if _, err := transaction.ExecContext(
			context,
			`UPDATE pending_ratings
			 SET rating = ?, resolved_at = unixepoch()
			 WHERE id = ? AND rating IS NULL`,
			rating,
			pendingID,
		); err != nil {
			return TasteRating{}, err
		}
	}
	if err := transaction.Commit(); err != nil {
		return TasteRating{}, err
	}
	return newTasteRating(pendingID, rating, dishID, dishName), nil
}

func newTasteRating(
	pendingRatingID int64,
	rating int,
	dishID, dishName string,
) TasteRating {
	result := TasteRating{
		PendingRatingID: pendingRatingID,
		Rating:          rating,
		Outcome:         "rejection_mark",
		Dish:            catalog.NewDish(dishID, dishName),
	}
	if engine.ValidTier(rating) {
		result.Outcome = "pool_admission"
		result.Tier = intPointer(rating)
	}
	return result
}
