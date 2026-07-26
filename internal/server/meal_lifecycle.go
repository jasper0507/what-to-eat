package server

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	mealStatusCandidatePoolEmpty mealStatus   = "candidate_pool_empty"
	mealStatusReady              mealStatus   = "ready"
	mealStatusActiveDecision     mealStatus   = "active_decision"
	mealStatusPendingRatings     mealStatus   = "pending_ratings"
	decisionModePool             decisionMode = "pool"
	decisionModeDiscovery        decisionMode = "discovery"
	defaultCooldown                           = 2
	defaultRecencyWindow                      = 7
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
	errCandidatePoolEmpty    = errors.New("Candidate pool is empty")
	errDecisionNotFound      = errors.New("Decision not found")
	errPendingRatingNotFound = errors.New("Pending rating not found")
	errPendingRatings        = errors.New("Pending ratings must be resolved")
	errTasteRatingConflict   = errors.New("Taste rating conflicts with the resolved rating")
)

type mealLifecycle struct {
	db        *sql.DB
	pool      *candidatePool
	random    *rand.Rand
	randomMu  sync.Mutex
	discovery DiscoveryConfig
}

type mealStatus string
type decisionMode string

type DiscoveryConfig struct {
	Enabled               bool
	MaxPoolSize           int
	MaxEligibleDishes     int
	MinRerolls            int
	RequiredSignals       int
	RecentMealWindow      int
	MaxDiscoveriesPerMeal int
}

func newDecisionRandom() *rand.Rand {
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

func normalizeDiscoveryConfig(config *DiscoveryConfig) (DiscoveryConfig, error) {
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

func newMealLifecycle(
	db *sql.DB,
	pool *candidatePool,
	random *rand.Rand,
	discovery DiscoveryConfig,
) *mealLifecycle {
	return &mealLifecycle{db: db, pool: pool, random: random, discovery: discovery}
}

type mealState struct {
	Status         mealStatus              `json:"status"`
	Decision       *mealDecisionResponse   `json:"decision,omitempty"`
	PendingRatings []pendingRatingResponse `json:"pending_ratings,omitempty"`
}

type mealDecisionResponse struct {
	ID     int64               `json:"id"`
	MealID int64               `json:"meal_id"`
	Mode   decisionMode        `json:"mode"`
	Reason string              `json:"reason,omitempty"`
	Dish   catalogDishResponse `json:"dish"`
}

type eatingRecordResponse struct {
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
	mode   decisionMode
	reason string
}

type recipeReferenceResponse struct {
	Dish catalogDishResponse `json:"dish"`
}

type acceptanceResponse struct {
	EatingRecord  eatingRecordResponse    `json:"eating_record"`
	Recipe        recipeReferenceResponse `json:"recipe"`
	PendingRating *pendingRatingResponse  `json:"pending_rating,omitempty"`
}

type pendingRatingResponse struct {
	ID     int64               `json:"id"`
	MealID int64               `json:"meal_id"`
	MealAt int64               `json:"meal_at"`
	Dish   catalogDishResponse `json:"dish"`
}

type tasteRatingResponse struct {
	PendingRatingID  int64               `json:"pending_rating_id"`
	Rating           int                 `json:"rating"`
	Outcome          string              `json:"outcome"`
	PreferenceWeight *float64            `json:"preference_weight,omitempty"`
	Dish             catalogDishResponse `json:"dish"`
}

type tasteRatingInput struct {
	Rating int `json:"rating"`
}

func scanActiveDecision(row *sql.Row) (mealState, error) {
	var decision mealDecisionResponse
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
		decision.Mode = decisionMode(mode)
		decision.Dish = catalogDish(dishID, dishName)
		return mealState{Status: mealStatusActiveDecision, Decision: &decision}, nil
	}
	return mealState{}, err
}

func (m *mealLifecycle) Resume(context context.Context, accountID int64) (mealState, error) {
	pendingRatings, err := m.unresolvedPendingRatings(context, m.db, accountID)
	if err != nil {
		return mealState{}, err
	}
	if len(pendingRatings) > 0 {
		return mealState{
			Status:         mealStatusPendingRatings,
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
		return mealState{}, err
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
		return mealState{}, err
	}
	if !hasCandidates {
		return mealState{Status: mealStatusCandidatePoolEmpty}, nil
	}
	return mealState{Status: mealStatusReady}, nil
}

// sqlQueryer 让同一读取逻辑既可跑在 *sql.DB 上也可跑在事务内。
type sqlQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (m *mealLifecycle) unresolvedPendingRatings(
	context context.Context,
	queryer sqlQueryer,
	accountID int64,
) ([]pendingRatingResponse, error) {
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

	pendingRatings := make([]pendingRatingResponse, 0)
	for rows.Next() {
		var pending pendingRatingResponse
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
		pending.Dish = catalogDish(dishID, dishName)
		pendingRatings = append(pendingRatings, pending)
	}
	return pendingRatings, rows.Err()
}

func pendingRatingForDecision(
	context context.Context,
	transaction *sql.Tx,
	accountID, decisionID int64,
) (*pendingRatingResponse, error) {
	var pending pendingRatingResponse
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
	pending.Dish = catalogDish(dishID, dishName)
	return &pending, nil
}

func (m *mealLifecycle) poolCandidates(
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

func (m *mealLifecycle) recentRerolls(
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
		if mode == string(decisionModeDiscovery) {
			discoveryCount++
		}
	}
	if err := rows.Close(); err != nil {
		return nil, 0, 0, err
	}
	return shown, decisionCount, discoveryCount, rows.Err()
}

func (m *mealLifecycle) discoveryPressure(
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

func (m *mealLifecycle) discoveryDish(
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
func (m *mealLifecycle) chooseDecision(
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
				mode:   decisionModeDiscovery,
				reason: reason,
			}, nil
		}
	}
	return decisionChoice{
		dish: m.chooseWeightedDish(input.poolChoices),
		mode: decisionModePool,
	}, nil
}

func dishSimilarity(candidate, reference weightedDish) float64 {
	candidatePath := strings.Split(candidate.id, "/")
	referencePath := strings.Split(reference.id, "/")
	score := 0.0
	if len(candidatePath) > 1 &&
		len(referencePath) > 1 &&
		candidatePath[0] == referencePath[0] {
		score += 4
	}
	referenceTags := make(map[string]bool)
	for _, tag := range referencePath[1:max(1, len(referencePath)-1)] {
		referenceTags[tag] = true
	}
	for _, tag := range candidatePath[1:max(1, len(candidatePath)-1)] {
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

func (m *mealLifecycle) chooseWeightedDish(candidates []weightedDish) weightedDish {
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

func (m *mealLifecycle) Begin(context context.Context, accountID int64) (state mealState, created bool, err error) {
	transaction, err := m.db.BeginTx(context, nil)
	if err != nil {
		return mealState{}, false, err
	}
	defer transaction.Rollback()

	pendingRatings, err := m.unresolvedPendingRatings(context, transaction, accountID)
	if err != nil {
		return mealState{}, false, err
	}
	if len(pendingRatings) > 0 {
		return mealState{
			Status:         mealStatusPendingRatings,
			PendingRatings: pendingRatings,
		}, false, errPendingRatings
	}

	activeDecision, err := scanActiveDecision(
		transaction.QueryRowContext(context, activeDecisionQuery, accountID),
	)
	if err == nil {
		return activeDecision, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return mealState{}, false, err
	}

	pool, err := m.poolCandidates(context, transaction, accountID)
	if err != nil {
		return mealState{}, false, err
	}
	if len(pool.selectable) == 0 {
		return mealState{Status: mealStatusCandidatePoolEmpty}, false, errCandidatePoolEmpty
	}
	recentRerolls, err := m.recentRerolls(context, transaction, accountID)
	if err != nil {
		return mealState{}, false, err
	}
	choice, err := m.chooseDecision(context, transaction, accountID, selectionInput{
		pool:        pool,
		poolChoices: pool.selectable,
		rerolls:     recentRerolls,
	})
	if err != nil {
		return mealState{}, false, err
	}

	mealResult, err := transaction.ExecContext(
		context,
		"INSERT INTO meals (account_id, status, created_at) VALUES (?, 'active', unixepoch())",
		accountID,
	)
	if err != nil {
		return mealState{}, false, err
	}
	var decision mealDecisionResponse
	decision.MealID, err = mealResult.LastInsertId()
	if err != nil {
		return mealState{}, false, err
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
		return mealState{}, false, err
	}
	decision.ID, err = decisionResult.LastInsertId()
	if err != nil {
		return mealState{}, false, err
	}
	decision.Mode = choice.mode
	decision.Reason = choice.reason
	decision.Dish = catalogDish(choice.dish.id, choice.dish.name)
	if err := transaction.Commit(); err != nil {
		return mealState{}, false, err
	}
	return mealState{Status: mealStatusActiveDecision, Decision: &decision}, true, nil
}

func (m *mealLifecycle) Reroll(
	context context.Context,
	accountID, decisionID int64,
) (mealState, error) {
	transaction, err := m.db.BeginTx(context, nil)
	if err != nil {
		return mealState{}, err
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
		return mealState{}, errDecisionNotFound
	}
	if err != nil {
		return mealState{}, err
	}
	if replacementID.Valid {
		replacement, err := scanActiveDecision(transaction.QueryRowContext(
			context,
			activeDecisionQuery+" AND decisions.id = ?",
			accountID,
			replacementID.Int64,
		))
		if errors.Is(err, sql.ErrNoRows) {
			return mealState{}, errDecisionNotFound
		}
		return replacement, err
	}
	if mealStatus != "active" || decisionStatus != "active" {
		return mealState{}, errDecisionNotFound
	}

	pool, err := m.poolCandidates(context, transaction, accountID)
	if err != nil {
		return mealState{}, err
	}
	if len(pool.selectable) == 0 {
		return mealState{Status: mealStatusCandidatePoolEmpty}, errCandidatePoolEmpty
	}

	shown, decisionCount, discoveryCount, err := mealShownCounts(context, transaction, mealID)
	if err != nil {
		return mealState{}, err
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
		return mealState{}, err
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
		return mealState{}, err
	}

	if _, err := transaction.ExecContext(
		context,
		"UPDATE decisions SET rerolled_to_id = id WHERE id = ? AND rerolled_to_id IS NULL",
		decisionID,
	); err != nil {
		return mealState{}, err
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
		return mealState{}, err
	}
	newDecisionID, err := result.LastInsertId()
	if err != nil {
		return mealState{}, err
	}
	if _, err := transaction.ExecContext(
		context,
		"UPDATE decisions SET rerolled_to_id = ? WHERE id = ? AND rerolled_to_id = id",
		newDecisionID,
		decisionID,
	); err != nil {
		return mealState{}, err
	}
	if err := transaction.Commit(); err != nil {
		return mealState{}, err
	}

	replacement := mealDecisionResponse{
		ID:     newDecisionID,
		MealID: mealID,
		Mode:   choice.mode,
		Reason: choice.reason,
		Dish:   catalogDish(choice.dish.id, choice.dish.name),
	}
	return mealState{Status: mealStatusActiveDecision, Decision: &replacement}, nil
}

func (m *mealLifecycle) Accept(
	context context.Context,
	accountID, decisionID int64,
) (acceptanceResponse, error) {
	transaction, err := m.db.BeginTx(context, nil)
	if err != nil {
		return acceptanceResponse{}, err
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
		return acceptanceResponse{}, errDecisionNotFound
	}
	if err != nil {
		return acceptanceResponse{}, err
	}

	result := acceptanceResponse{
		Recipe: recipeReferenceResponse{
			Dish: catalogDish(dishID, dishName),
		},
	}
	if rerolledToID.Valid {
		return acceptanceResponse{}, errDecisionNotFound
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
			return acceptanceResponse{}, err
		}
		return result, nil
	}
	if status != "active" {
		return acceptanceResponse{}, errDecisionNotFound
	}

	if err := transaction.QueryRowContext(
		context,
		"SELECT COALESCE(MAX(sequence), 0) + 1 FROM eating_records WHERE account_id = ?",
		accountID,
	).Scan(&result.EatingRecord.Sequence); err != nil {
		return acceptanceResponse{}, err
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
		return acceptanceResponse{}, err
	}
	if _, err := transaction.ExecContext(
		context,
		"UPDATE decisions SET status = 'accepted' WHERE id = ? AND status = 'active'",
		decisionID,
	); err != nil {
		return acceptanceResponse{}, err
	}
	if _, err := transaction.ExecContext(
		context,
		"UPDATE meals SET status = 'accepted' WHERE id = ? AND status = 'active'",
		mealID,
	); err != nil {
		return acceptanceResponse{}, err
	}
	if mode == string(decisionModeDiscovery) {
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
			return acceptanceResponse{}, err
		}
		result.PendingRating, err = pendingRatingForDecision(
			context,
			transaction,
			accountID,
			decisionID,
		)
		if err != nil {
			return acceptanceResponse{}, err
		}
	}
	if err := transaction.Commit(); err != nil {
		return acceptanceResponse{}, err
	}
	return result, nil
}

func (m *mealLifecycle) Rate(
	context context.Context,
	accountID, pendingRatingID int64,
	rating int,
) (tasteRatingResponse, error) {
	transaction, err := m.db.BeginTx(context, nil)
	if err != nil {
		return tasteRatingResponse{}, err
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
		return tasteRatingResponse{}, errPendingRatingNotFound
	}
	if err != nil {
		return tasteRatingResponse{}, err
	}
	if existingRating.Valid {
		if int(existingRating.Int64) != rating {
			return tasteRatingResponse{}, errTasteRatingConflict
		}
		return newTasteRatingResponse(pendingRatingID, rating, dishID, dishName), nil
	}

	if weight, admitted := preferenceWeightForTasteRating(rating); admitted {
		err := m.pool.Admit(context, transaction, accountID, dishID, weight)
		if errors.Is(err, errDishRejected) {
			return tasteRatingResponse{}, errTasteRatingConflict
		}
		if err != nil {
			return tasteRatingResponse{}, err
		}
	} else {
		if err := m.pool.Reject(context, transaction, accountID, dishID, rating); err != nil {
			return tasteRatingResponse{}, err
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
		return tasteRatingResponse{}, err
	}
	if err := transaction.Commit(); err != nil {
		return tasteRatingResponse{}, err
	}
	return newTasteRatingResponse(pendingRatingID, rating, dishID, dishName), nil
}

func newTasteRatingResponse(
	pendingRatingID int64,
	rating int,
	dishID, dishName string,
) tasteRatingResponse {
	result := tasteRatingResponse{
		PendingRatingID: pendingRatingID,
		Rating:          rating,
		Outcome:         "rejection_mark",
		Dish:            catalogDish(dishID, dishName),
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

func (a *App) resumeMeal(context *gin.Context) {
	account := sessionAccount(context)
	state, err := a.mealLifecycle.Resume(context, account.ID)
	if err != nil {
		writeInternalError(context, "resume Meal lifecycle", err)
		return
	}
	context.JSON(http.StatusOK, state)
}

func (a *App) beginMeal(context *gin.Context) {
	account := sessionAccount(context)
	state, created, err := a.mealLifecycle.Begin(context, account.ID)
	switch {
	case errors.Is(err, errPendingRatings):
		writeError(
			context,
			http.StatusConflict,
			string(state.Status),
			"请先解决所有 Pending rating，再开始新的 Decision",
		)
	case errors.Is(err, errCandidatePoolEmpty):
		writeError(
			context,
			http.StatusConflict,
			string(state.Status),
			"Candidate pool 为空，无法创建 Decision",
		)
	case err != nil:
		writeInternalError(context, "begin Meal lifecycle", err)
	default:
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		context.JSON(status, state)
	}
}

func (a *App) rerollDecision(context *gin.Context) {
	account := sessionAccount(context)
	decisionID, err := strconv.ParseInt(context.Param("decisionID"), 10, 64)
	if err != nil || decisionID <= 0 {
		writeError(context, http.StatusNotFound, "decision_not_found", "Decision 不存在")
		return
	}
	state, err := a.mealLifecycle.Reroll(context, account.ID, decisionID)
	switch {
	case errors.Is(err, errCandidatePoolEmpty):
		writeError(
			context,
			http.StatusConflict,
			string(state.Status),
			"Candidate pool 为空，无法 Reroll Decision",
		)
	case errors.Is(err, errDecisionNotFound):
		writeError(context, http.StatusNotFound, "decision_not_found", "Decision 不存在")
	case err != nil:
		writeInternalError(context, "reroll Decision", err)
	default:
		context.JSON(http.StatusOK, state)
	}
}

func (a *App) acceptDecision(context *gin.Context) {
	account := sessionAccount(context)
	decisionID, err := strconv.ParseInt(context.Param("decisionID"), 10, 64)
	if err != nil || decisionID <= 0 {
		writeError(context, http.StatusNotFound, "decision_not_found", "Decision 不存在")
		return
	}
	result, err := a.mealLifecycle.Accept(context, account.ID, decisionID)
	switch {
	case errors.Is(err, errDecisionNotFound):
		writeError(context, http.StatusNotFound, "decision_not_found", "Decision 不存在")
	case err != nil:
		writeInternalError(context, "accept Decision", err)
	default:
		context.JSON(http.StatusOK, result)
	}
}

func (a *App) ratePendingRating(context *gin.Context) {
	account := sessionAccount(context)
	pendingRatingID, err := strconv.ParseInt(context.Param("pendingRatingID"), 10, 64)
	if err != nil || pendingRatingID <= 0 {
		writeError(context, http.StatusNotFound, "pending_rating_not_found", "Pending rating 不存在")
		return
	}
	var input tasteRatingInput
	if err := context.ShouldBindJSON(&input); err != nil || input.Rating < 1 || input.Rating > 5 {
		writeError(context, http.StatusBadRequest, "invalid_request", "Taste rating 必须为 1–5")
		return
	}
	result, err := a.mealLifecycle.Rate(context, account.ID, pendingRatingID, input.Rating)
	switch {
	case errors.Is(err, errPendingRatingNotFound):
		writeError(context, http.StatusNotFound, "pending_rating_not_found", "Pending rating 不存在")
	case errors.Is(err, errTasteRatingConflict):
		writeError(context, http.StatusConflict, "rating_conflict", "Taste rating 与已有结果冲突")
	case err != nil:
		writeInternalError(context, "resolve Pending rating", err)
	default:
		context.JSON(http.StatusOK, result)
	}
}

func migrateRerollSchema(db *sql.DB) error {
	var hasRerolledToID bool
	if err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM pragma_table_info('decisions') WHERE name = 'rerolled_to_id'
		)
	`).Scan(&hasRerolledToID); err != nil {
		return err
	}
	if !hasRerolledToID {
		if _, err := db.Exec(
			"ALTER TABLE decisions ADD COLUMN rerolled_to_id INTEGER REFERENCES decisions(id)",
		); err != nil {
			return err
		}
	}
	_, err := db.Exec(`
		DROP INDEX IF EXISTS one_active_decision_per_meal;
		CREATE UNIQUE INDEX one_active_decision_per_meal
			ON decisions(meal_id)
			WHERE status = 'active' AND rerolled_to_id IS NULL;
	`)
	return err
}

func migrateDiscoverySchema(db *sql.DB) (err error) {
	var createSQL string
	if err := db.QueryRow(
		"SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = 'decisions'",
	).Scan(&createSQL); err != nil {
		return err
	}
	var hasReason bool
	if err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM pragma_table_info('decisions') WHERE name = 'reason'
		)
	`).Scan(&hasReason); err != nil {
		return err
	}
	if hasReason && strings.Contains(createSQL, "'discovery'") {
		return nil
	}

	if _, err := db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	foreignKeysDisabled := true
	defer func() {
		if foreignKeysDisabled {
			_, restoreErr := db.Exec("PRAGMA foreign_keys = ON")
			err = errors.Join(err, restoreErr)
		}
	}()

	transaction, err := db.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`
		CREATE TABLE decisions_next (
			id INTEGER PRIMARY KEY,
			meal_id INTEGER NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
			dish_id TEXT NOT NULL REFERENCES catalog_dishes(source_path),
			mode TEXT NOT NULL CHECK (mode IN ('pool', 'discovery')),
			reason TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL CHECK (status IN ('active', 'accepted')),
			rerolled_to_id INTEGER REFERENCES decisions_next(id),
			created_at INTEGER NOT NULL
		);
	`); err != nil {
		return err
	}
	if hasReason {
		_, err = transaction.Exec(`
			INSERT INTO decisions_next (
				id, meal_id, dish_id, mode, reason, status, rerolled_to_id, created_at
			)
			SELECT id, meal_id, dish_id, mode, reason, status, rerolled_to_id, created_at
			FROM decisions;
		`)
	} else {
		_, err = transaction.Exec(`
			INSERT INTO decisions_next (
				id, meal_id, dish_id, mode, reason, status, rerolled_to_id, created_at
			)
			SELECT id, meal_id, dish_id, mode, '', status, rerolled_to_id, created_at
			FROM decisions;
		`)
	}
	if err != nil {
		return err
	}
	if _, err := transaction.Exec(`
		DROP TABLE decisions;
		ALTER TABLE decisions_next RENAME TO decisions;
		CREATE UNIQUE INDEX one_active_decision_per_meal
			ON decisions(meal_id)
			WHERE status = 'active' AND rerolled_to_id IS NULL;
	`); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return err
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return err
	}
	foreignKeysDisabled = false
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("Discovery migration left invalid foreign keys")
	}
	return rows.Err()
}

func migratePendingRatingSchema(db *sql.DB) error {
	transaction, err := db.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`
		CREATE TABLE IF NOT EXISTS pending_ratings (
			id INTEGER PRIMARY KEY,
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			meal_id INTEGER NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
			decision_id INTEGER NOT NULL UNIQUE REFERENCES decisions(id) ON DELETE CASCADE,
			dish_id TEXT NOT NULL REFERENCES catalog_dishes(source_path),
			meal_at INTEGER NOT NULL,
			rating INTEGER CHECK (rating BETWEEN 1 AND 5),
			resolved_at INTEGER,
			CHECK (
				(rating IS NULL AND resolved_at IS NULL) OR
				(rating IS NOT NULL AND resolved_at IS NOT NULL)
			)
		);
		CREATE INDEX IF NOT EXISTS pending_ratings_account_unresolved
			ON pending_ratings(account_id, meal_at, id)
			WHERE rating IS NULL;
		CREATE UNIQUE INDEX IF NOT EXISTS pending_ratings_account_dish_unresolved
			ON pending_ratings(account_id, dish_id)
			WHERE rating IS NULL;
		CREATE TABLE IF NOT EXISTS rejection_marks (
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			dish_id TEXT NOT NULL REFERENCES catalog_dishes(source_path) ON DELETE CASCADE,
			rating INTEGER NOT NULL CHECK (rating IN (1, 2)),
			created_at INTEGER NOT NULL,
			PRIMARY KEY (account_id, dish_id)
		);
	`); err != nil {
		return err
	}
	return transaction.Commit()
}
