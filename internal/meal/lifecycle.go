// Package meal 是 Meal lifecycle 深模块（ADR-0019/0022）：事务壳、状态机与
// 记账。评分智能（四因子、放宽、相似度、理由）全部住在 internal/engine 纯函数
// 包里；本包只负责拼装快照、抽样落库与 Reroll budget / 三出口的账本（档位降档写侧在 pool）。
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
		if err := m.pool.RecordSwap(context, transaction, accountID, currentDishID); err != nil {
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
	if err := m.pool.ResetSwaps(context, transaction, accountID, dishID); err != nil {
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
