// Package meal 是 ADR-0019 的 Meal lifecycle 深模块：Decision、Reroll、
// Acceptance、anti-repeat、Discovery、Pending rating、Taste rating 与
// 评分驱动的 pool admission/rejection 全部藏在 Resume/Begin/Reroll/
// Accept/Rate 五个行为之后。
package meal

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/jasper0507/what-to-eat/internal/catalog"
	"github.com/jasper0507/what-to-eat/internal/pool"
)

const (
	StatusCandidatePoolEmpty Status = "candidate_pool_empty"
	StatusReady              Status = "ready"
	StatusActiveDecision     Status = "active_decision"
	StatusPendingRatings     Status = "pending_ratings"
	ModePool                 Mode   = "pool"
	ModeDiscovery            Mode   = "discovery"
	defaultCooldown                 = 2
	defaultRecencyWindow            = 7
)

const activeDecisionQuery = `
	SELECT decisions.id, meals.id, decisions.mode, decisions.reason,
	       catalog_dishes.source_path, catalog_dishes.name
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
	RequiredSignals       int
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
		RequiredSignals:       2,
		RecentMealWindow:      3,
		MaxDiscoveriesPerMeal: 2,
	}
}

func NormalizeDiscoveryConfig(config *DiscoveryConfig) (DiscoveryConfig, error) {
	if config == nil {
		return DefaultDiscoveryConfig(), nil
	}
	if !config.Enabled {
		return *config, nil
	}
	if config.MaxPoolSize < 0 ||
		config.MaxEligibleDishes < 0 ||
		config.MinRerolls < 1 ||
		config.RequiredSignals < 1 ||
		config.RequiredSignals > 3 ||
		config.RecentMealWindow < 0 ||
		config.MaxDiscoveriesPerMeal < 1 {
		return DiscoveryConfig{}, errors.New("invalid Discovery config")
	}
	return *config, nil
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
	Status         Status          `json:"status"`
	Decision       *Decision       `json:"decision,omitempty"`
	PendingRatings []PendingRating `json:"pending_ratings,omitempty"`
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

// weightedDish 的 weight 是抽样权重：pool 路径为经 Recency window 调整的
// Preference weight，discovery 路径为相似度得分。两种量只各自参与
// chooseWeightedDish 的加权抽样，永不混在同一候选列表里比较。
type weightedDish struct {
	id, name string
	weight   float64
}

type poolSnapshot struct {
	members          []weightedDish
	selectable       []weightedDish
	cooldownEligible int
	history          []string
}

type decisionChoice struct {
	dish   weightedDish
	mode   Mode
	reason string
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
	PendingRatingID  int64        `json:"pending_rating_id"`
	Rating           int          `json:"rating"`
	Outcome          string       `json:"outcome"`
	PreferenceWeight *float64     `json:"preference_weight,omitempty"`
	Dish             catalog.Dish `json:"dish"`
}

func scanActiveDecision(row *sql.Row) (State, error) {
	var decision Decision
	var mode, dishID, dishName string
	err := row.Scan(
		&decision.ID,
		&decision.MealID,
		&mode,
		&decision.Reason,
		&dishID,
		&dishName,
	)
	if err == nil {
		decision.Mode = Mode(mode)
		decision.Dish = catalog.NewDish(dishID, dishName)
		return State{Status: StatusActiveDecision, Decision: &decision}, nil
	}
	return State{}, err
}

func (m *Lifecycle) Resume(context context.Context, accountID int64) (State, error) {
	pendingRatings, err := m.unresolvedPendingRatings(context, m.db, accountID)
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
		m.db.QueryRowContext(context, activeDecisionQuery, accountID),
	)
	if err == nil {
		return activeDecision, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return State{}, err
	}

	var hasCandidates bool
	if err := m.db.QueryRowContext(
		context,
		`SELECT EXISTS(
			SELECT 1
			FROM candidate_pool
			WHERE candidate_pool.account_id = ?
			  AND NOT EXISTS (
				SELECT 1
				FROM rejection_marks
				WHERE rejection_marks.account_id = candidate_pool.account_id
				  AND rejection_marks.dish_id = candidate_pool.dish_id
			  )
		 )`,
		accountID,
	).Scan(&hasCandidates); err != nil {
		return State{}, err
	}
	if !hasCandidates {
		return State{Status: StatusCandidatePoolEmpty}, nil
	}
	return State{Status: StatusReady}, nil
}

// sqlQueryer 让同一读取逻辑既可跑在 *sql.DB 上也可跑在事务内。
type sqlQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
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

func (m *Lifecycle) poolCandidates(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
) (poolSnapshot, error) {
	rows, err := transaction.QueryContext(
		context,
		`SELECT catalog_dishes.source_path, catalog_dishes.name, candidate_pool.preference_weight
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
	allCandidates := make([]weightedDish, 0)
	for rows.Next() {
		var candidate weightedDish
		if err := rows.Scan(&candidate.id, &candidate.name, &candidate.weight); err != nil {
			rows.Close()
			return poolSnapshot{}, err
		}
		allCandidates = append(allCandidates, candidate)
	}
	if err := rows.Close(); err != nil {
		return poolSnapshot{}, err
	}
	if err := rows.Err(); err != nil {
		return poolSnapshot{}, err
	}

	historyRows, err := transaction.QueryContext(
		context,
		`SELECT dish_id
		 FROM eating_records
		 WHERE account_id = ?
		 ORDER BY sequence DESC
		 LIMIT ?`,
		accountID,
		defaultRecencyWindow,
	)
	if err != nil {
		return poolSnapshot{}, err
	}
	history := make([]string, 0, defaultRecencyWindow)
	for historyRows.Next() {
		var dishID string
		if err := historyRows.Scan(&dishID); err != nil {
			historyRows.Close()
			return poolSnapshot{}, err
		}
		history = append(history, dishID)
	}
	if err := historyRows.Close(); err != nil {
		return poolSnapshot{}, err
	}
	if err := historyRows.Err(); err != nil {
		return poolSnapshot{}, err
	}

	fullCooldown := cooldownSize(history)
	snapshot := poolSnapshot{members: allCandidates, history: history}
	for cooldown := fullCooldown; cooldown >= 0; cooldown-- {
		excluded := cooldownSet(history, cooldown)
		candidates := make([]weightedDish, 0, len(allCandidates))
		for _, candidate := range allCandidates {
			if !excluded[candidate.id] {
				candidates = append(candidates, candidate)
			}
		}
		if cooldown == fullCooldown {
			snapshot.cooldownEligible = len(candidates)
		}
		if len(candidates) > 0 {
			if cooldown == fullCooldown {
				halveRecentWeights(candidates, history)
			}
			snapshot.selectable = candidates
			return snapshot, nil
		}
	}
	snapshot.selectable = allCandidates
	return snapshot, nil
}

// 选择内核：Cooldown、Recency window 与 Session penalty 的唯一实现，
// pool 与 discovery 两条路径共用。

// cooldownSize 把 ADR-0002 的 Cooldown 长度折算到既有 Eating record 数量上。
func cooldownSize(history []string) int {
	return min(len(history), defaultCooldown)
}

// cooldownSet 返回处于 Cooldown 的 Dish 集合（history 最近 size 条）。
func cooldownSet(history []string, size int) map[string]bool {
	excluded := make(map[string]bool, size)
	for _, dishID := range history[:size] {
		excluded[dishID] = true
	}
	return excluded
}

// halveRecentWeights 对 Recency window 内（Cooldown 之外）的每次既往
// Acceptance 把对应 Dish 的抽样权重减半。
func halveRecentWeights(candidates []weightedDish, history []string) {
	for _, dishID := range history[cooldownSize(history):] {
		for index := range candidates {
			if candidates[index].id == dishID {
				candidates[index].weight /= 2
			}
		}
	}
}

// leastShownDishes 应用 Session penalty：只保留本 Meal 内展示次数最少的 Dish。
func leastShownDishes(candidates []weightedDish, shown map[string]int) []weightedDish {
	least := make([]weightedDish, 0, len(candidates))
	minShown := -1
	for _, candidate := range candidates {
		count := shown[candidate.id]
		switch {
		case minShown == -1 || count < minShown:
			minShown = count
			least = append(least[:0], candidate)
		case count == minShown:
			least = append(least, candidate)
		}
	}
	return least
}

// topWeightDishes 保留抽样权重最高的 Dish，并列全部保留。
func topWeightDishes(candidates []weightedDish) []weightedDish {
	top := make([]weightedDish, 0, len(candidates))
	best := 0.0
	for _, candidate := range candidates {
		switch {
		case len(top) == 0 || candidate.weight > best:
			best = candidate.weight
			top = append(top[:0], candidate)
		case candidate.weight == best:
			top = append(top, candidate)
		}
	}
	return top
}

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
			WHERE account_id = ? AND status = 'accepted'
			ORDER BY id DESC
			LIMIT ?
		 )`,
		accountID,
		m.discovery.RecentMealWindow,
	).Scan(&rerolls)
	return rerolls, err
}

// mealShownCounts 统计本 Meal 内已展示的 Dish（Session penalty 的输入）
// 以及 Decision 与 Discovery 的数量。
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

func (m *Lifecycle) discoveryPressure(
	pool poolSnapshot,
	rerolls int,
) (bool, string) {
	if !m.discovery.Enabled {
		return false, ""
	}
	reasons := make([]string, 0, 3)
	if len(pool.members) <= m.discovery.MaxPoolSize {
		reasons = append(reasons, "Candidate pool 较小")
	}
	if pool.cooldownEligible <= m.discovery.MaxEligibleDishes {
		reasons = append(reasons, "Cooldown 后可选 Dish 较少")
	}
	if rerolls >= m.discovery.MinRerolls {
		reasons = append(reasons, "最近 Reroll 较多")
	}
	if len(reasons) < m.discovery.RequiredSignals {
		return false, ""
	}
	return true, strings.Join(reasons, "，") + "，试试相似的新菜。"
}

func (m *Lifecycle) discoveryDish(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
	pool poolSnapshot,
	shown map[string]int,
) (weightedDish, bool, error) {
	rows, err := transaction.QueryContext(
		context,
		`SELECT catalog_dishes.source_path, catalog_dishes.name
		 FROM catalog_dishes
		 WHERE NOT EXISTS (
			SELECT 1
			FROM candidate_pool
			WHERE candidate_pool.account_id = ?
			  AND candidate_pool.dish_id = catalog_dishes.source_path
		 )
		   AND NOT EXISTS (
			SELECT 1
			FROM rejection_marks
			WHERE rejection_marks.account_id = ?
			  AND rejection_marks.dish_id = catalog_dishes.source_path
		   )
		 ORDER BY catalog_dishes.source_path`,
		accountID,
		accountID,
	)
	if err != nil {
		return weightedDish{}, false, err
	}
	defer rows.Close()

	excluded := cooldownSet(pool.history, cooldownSize(pool.history))
	candidates := make([]weightedDish, 0)
	for rows.Next() {
		var candidate weightedDish
		if err := rows.Scan(&candidate.id, &candidate.name); err != nil {
			return weightedDish{}, false, err
		}
		if excluded[candidate.id] {
			continue
		}
		for _, reference := range pool.members {
			candidate.weight = max(candidate.weight, dishSimilarity(candidate, reference)*reference.weight)
		}
		if candidate.weight > 0 {
			candidates = append(candidates, candidate)
		}
	}
	if err := rows.Err(); err != nil {
		return weightedDish{}, false, err
	}
	halveRecentWeights(candidates, pool.history)
	candidates = topWeightDishes(leastShownDishes(candidates, shown))
	if len(candidates) == 0 {
		return weightedDish{}, false, nil
	}
	return m.chooseWeightedDish(candidates), true, nil
}

type selectionInput struct {
	pool           poolSnapshot
	poolChoices    []weightedDish
	rerolls        int
	discoveryCount int
	shown          map[string]int
}

// chooseDecision 决定本次 Decision 走 pool 还是 Discovery，并只为选中的
// 路径消耗随机抽样。
func (m *Lifecycle) chooseDecision(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
	input selectionInput,
) (decisionChoice, error) {
	triggered, reason := m.discoveryPressure(input.pool, input.rerolls)
	if triggered && input.discoveryCount < m.discovery.MaxDiscoveriesPerMeal {
		discovery, found, err := m.discoveryDish(
			context,
			transaction,
			accountID,
			input.pool,
			input.shown,
		)
		if err != nil {
			return decisionChoice{}, err
		}
		if found {
			return decisionChoice{
				dish:   discovery,
				mode:   ModeDiscovery,
				reason: reason,
			}, nil
		}
	}
	return decisionChoice{
		dish: m.chooseWeightedDish(input.poolChoices),
		mode: ModePool,
	}, nil
}

func dishSimilarity(candidate, reference weightedDish) float64 {
	candidateTaxonomy := catalog.PathTaxonomy(candidate.id)
	referenceTaxonomy := catalog.PathTaxonomy(reference.id)
	score := 0.0
	if candidateTaxonomy.Category != "" &&
		candidateTaxonomy.Category == referenceTaxonomy.Category {
		score += 4
	}
	referenceTags := make(map[string]bool)
	for _, tag := range referenceTaxonomy.Tags {
		referenceTags[tag] = true
	}
	for _, tag := range candidateTaxonomy.Tags {
		if referenceTags[tag] {
			score += 2
		}
	}
	referenceKeywords := nameBigrams(reference.name)
	for keyword := range nameBigrams(candidate.name) {
		if referenceKeywords[keyword] {
			score += 3
		}
	}
	return score
}

func nameBigrams(name string) map[string]bool {
	runes := []rune(name)
	keywords := make(map[string]bool, max(0, len(runes)-1))
	for index := 0; index+1 < len(runes); index++ {
		keywords[string(runes[index:index+2])] = true
	}
	return keywords
}

func (m *Lifecycle) chooseWeightedDish(candidates []weightedDish) weightedDish {
	totalWeight := 0.0
	for _, candidate := range candidates {
		totalWeight += candidate.weight
	}
	m.randomMu.Lock()
	target := m.random.Float64() * totalWeight
	m.randomMu.Unlock()
	selected := candidates[len(candidates)-1]
	for _, candidate := range candidates {
		target -= candidate.weight
		if target < 0 {
			return candidate
		}
	}
	return selected
}

func (m *Lifecycle) Begin(context context.Context, accountID int64) (state State, created bool, err error) {
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
		return activeDecision, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return State{}, false, err
	}

	pool, err := m.poolCandidates(context, transaction, accountID)
	if err != nil {
		return State{}, false, err
	}
	if len(pool.selectable) == 0 {
		return State{Status: StatusCandidatePoolEmpty}, false, ErrCandidatePoolEmpty
	}
	recentRerolls, err := m.recentRerolls(context, transaction, accountID)
	if err != nil {
		return State{}, false, err
	}
	choice, err := m.chooseDecision(context, transaction, accountID, selectionInput{
		pool:        pool,
		poolChoices: pool.selectable,
		rerolls:     recentRerolls,
	})
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
	var decision Decision
	decision.MealID, err = mealResult.LastInsertId()
	if err != nil {
		return State{}, false, err
	}
	decisionResult, err := transaction.ExecContext(
		context,
		`INSERT INTO decisions (meal_id, dish_id, mode, reason, status, created_at)
		 VALUES (?, ?, ?, ?, 'active', unixepoch())`,
		decision.MealID,
		choice.dish.id,
		string(choice.mode),
		choice.reason,
	)
	if err != nil {
		return State{}, false, err
	}
	decision.ID, err = decisionResult.LastInsertId()
	if err != nil {
		return State{}, false, err
	}
	decision.Mode = choice.mode
	decision.Reason = choice.reason
	decision.Dish = catalog.NewDish(choice.dish.id, choice.dish.name)
	if err := transaction.Commit(); err != nil {
		return State{}, false, err
	}
	return State{Status: StatusActiveDecision, Decision: &decision}, true, nil
}

func (m *Lifecycle) Reroll(
	context context.Context,
	accountID, decisionID int64,
) (State, error) {
	transaction, err := m.db.BeginTx(context, nil)
	if err != nil {
		return State{}, err
	}
	defer transaction.Rollback()

	var mealID int64
	var mealStatus, decisionStatus, currentDishID string
	var replacementID sql.NullInt64
	err = transaction.QueryRowContext(
		context,
		`SELECT meals.id, meals.status, decisions.status, decisions.dish_id,
		        decisions.rerolled_to_id
		 FROM decisions
		 JOIN meals ON meals.id = decisions.meal_id
		 WHERE decisions.id = ? AND meals.account_id = ?`,
		decisionID,
		accountID,
	).Scan(&mealID, &mealStatus, &decisionStatus, &currentDishID, &replacementID)
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
		return replacement, err
	}
	if mealStatus != "active" || decisionStatus != "active" {
		return State{}, ErrDecisionNotFound
	}

	pool, err := m.poolCandidates(context, transaction, accountID)
	if err != nil {
		return State{}, err
	}
	if len(pool.selectable) == 0 {
		return State{Status: StatusCandidatePoolEmpty}, ErrCandidatePoolEmpty
	}

	shown, decisionCount, discoveryCount, err := mealShownCounts(context, transaction, mealID)
	if err != nil {
		return State{}, err
	}

	poolChoices := pool.selectable
	if len(poolChoices) > 1 {
		remaining := make([]weightedDish, 0, len(poolChoices))
		for _, candidate := range poolChoices {
			if candidate.id != currentDishID {
				remaining = append(remaining, candidate)
			}
		}
		poolChoices = remaining
	}
	recentRerolls, err := m.recentRerolls(context, transaction, accountID)
	if err != nil {
		return State{}, err
	}
	completedRerolls := max(0, decisionCount-1)
	rerollsIncludingCurrentRequest := completedRerolls + 1
	choice, err := m.chooseDecision(context, transaction, accountID, selectionInput{
		pool:           pool,
		poolChoices:    leastShownDishes(poolChoices, shown),
		rerolls:        rerollsIncludingCurrentRequest + recentRerolls,
		discoveryCount: discoveryCount,
		shown:          shown,
	})
	if err != nil {
		return State{}, err
	}

	if _, err := transaction.ExecContext(
		context,
		"UPDATE decisions SET rerolled_to_id = id WHERE id = ? AND rerolled_to_id IS NULL",
		decisionID,
	); err != nil {
		return State{}, err
	}
	result, err := transaction.ExecContext(
		context,
		`INSERT INTO decisions (meal_id, dish_id, mode, reason, status, created_at)
		 VALUES (?, ?, ?, ?, 'active', unixepoch())`,
		mealID,
		choice.dish.id,
		string(choice.mode),
		choice.reason,
	)
	if err != nil {
		return State{}, err
	}
	newDecisionID, err := result.LastInsertId()
	if err != nil {
		return State{}, err
	}
	if _, err := transaction.ExecContext(
		context,
		"UPDATE decisions SET rerolled_to_id = ? WHERE id = ? AND rerolled_to_id = id",
		newDecisionID,
		decisionID,
	); err != nil {
		return State{}, err
	}
	if err := transaction.Commit(); err != nil {
		return State{}, err
	}

	replacement := Decision{
		ID:     newDecisionID,
		MealID: mealID,
		Mode:   choice.mode,
		Reason: choice.reason,
		Dish:   catalog.NewDish(choice.dish.id, choice.dish.name),
	}
	return State{Status: StatusActiveDecision, Decision: &replacement}, nil
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
	var status, mode, dishID, dishName string
	var existingSequence, rerolledToID sql.NullInt64
	err = transaction.QueryRowContext(
		context,
		`SELECT meals.id, decisions.status, decisions.mode, catalog_dishes.source_path,
		        catalog_dishes.name, eating_records.sequence, decisions.rerolled_to_id
		 FROM decisions
		 JOIN meals ON meals.id = decisions.meal_id
		 JOIN catalog_dishes ON catalog_dishes.source_path = decisions.dish_id
		 LEFT JOIN eating_records ON eating_records.decision_id = decisions.id
		 WHERE decisions.id = ? AND meals.account_id = ?`,
		decisionID,
		accountID,
	).Scan(&mealID, &status, &mode, &dishID, &dishName, &existingSequence, &rerolledToID)
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
	if status != "active" {
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

	if weight, admitted := preferenceWeightForTasteRating(rating); admitted {
		err := m.pool.Admit(context, transaction, accountID, dishID, weight)
		if errors.Is(err, pool.ErrDishRejected) {
			return TasteRating{}, ErrTasteRatingConflict
		}
		if err != nil {
			return TasteRating{}, err
		}
	} else {
		if err := m.pool.Reject(context, transaction, accountID, dishID, rating); err != nil {
			return TasteRating{}, err
		}
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
	if weight, admitted := preferenceWeightForTasteRating(rating); admitted {
		result.Outcome = "pool_admission"
		result.PreferenceWeight = &weight
	}
	return result
}

func preferenceWeightForTasteRating(rating int) (float64, bool) {
	switch rating {
	case 3:
		return 0.7, true
	case 4:
		return 1.0, true
	case 5:
		return 1.3, true
	default:
		return 0, false
	}
}
