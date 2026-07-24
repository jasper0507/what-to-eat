package server

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	mealStatusCandidatePoolEmpty mealStatus = "candidate_pool_empty"
	mealStatusReady              mealStatus = "ready"
	mealStatusActiveDecision     mealStatus = "active_decision"
)

const activeDecisionQuery = `
	SELECT decisions.id, meals.id, decisions.mode,
	       catalog_dishes.source_path, catalog_dishes.name
	FROM meals
	JOIN decisions ON decisions.meal_id = meals.id
	JOIN catalog_dishes ON catalog_dishes.source_path = decisions.dish_id
	WHERE meals.account_id = ? AND meals.status = 'active' AND decisions.status = 'active'
	      AND decisions.rerolled_to_id IS NULL
`

var (
	errCandidatePoolEmpty = errors.New("Candidate pool is empty")
	errDecisionNotFound   = errors.New("Decision not found")
)

type mealLifecycle struct {
	db       *sql.DB
	random   *rand.Rand
	randomMu sync.Mutex
}

type mealStatus string

func newDecisionRandom() *rand.Rand {
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

func newMealLifecycle(db *sql.DB, random *rand.Rand) *mealLifecycle {
	return &mealLifecycle{db: db, random: random}
}

type mealState struct {
	Status   mealStatus            `json:"status"`
	Decision *mealDecisionResponse `json:"decision,omitempty"`
}

type mealDecisionResponse struct {
	ID     int64               `json:"id"`
	MealID int64               `json:"meal_id"`
	Mode   string              `json:"mode"`
	Dish   catalogDishResponse `json:"dish"`
}

type eatingRecordResponse struct {
	Sequence int64 `json:"sequence"`
}

type weightedDish struct {
	id, name string
	weight   float64
}

type recipeReferenceResponse struct {
	Dish catalogDishResponse `json:"dish"`
}

type acceptanceResponse struct {
	EatingRecord eatingRecordResponse    `json:"eating_record"`
	Recipe       recipeReferenceResponse `json:"recipe"`
}

func scanActiveDecision(row *sql.Row) (mealState, error) {
	var decision mealDecisionResponse
	var dishID, dishName string
	err := row.Scan(&decision.ID, &decision.MealID, &decision.Mode, &dishID, &dishName)
	if err == nil {
		decision.Dish = catalogDish(dishID, dishName)
		return mealState{Status: mealStatusActiveDecision, Decision: &decision}, nil
	}
	return mealState{}, err
}

func (m *mealLifecycle) Resume(context context.Context, accountID int64) (mealState, error) {
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
		"SELECT EXISTS(SELECT 1 FROM candidate_pool WHERE account_id = ?)",
		accountID,
	).Scan(&hasCandidates); err != nil {
		return mealState{}, err
	}
	if !hasCandidates {
		return mealState{Status: mealStatusCandidatePoolEmpty}, nil
	}
	return mealState{Status: mealStatusReady}, nil
}

func (m *mealLifecycle) poolCandidates(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
) ([]weightedDish, error) {
	rows, err := transaction.QueryContext(
		context,
		`SELECT catalog_dishes.source_path, catalog_dishes.name, candidate_pool.preference_weight
		 FROM candidate_pool
		 JOIN catalog_dishes ON catalog_dishes.source_path = candidate_pool.dish_id
		 WHERE candidate_pool.account_id = ?
		 ORDER BY catalog_dishes.source_path`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	candidates := make([]weightedDish, 0)
	for rows.Next() {
		var candidate weightedDish
		if err := rows.Scan(&candidate.id, &candidate.name, &candidate.weight); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return candidates, rows.Err()
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

	activeDecision, err := scanActiveDecision(
		transaction.QueryRowContext(context, activeDecisionQuery, accountID),
	)
	if err == nil {
		return activeDecision, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return mealState{}, false, err
	}

	candidates, err := m.poolCandidates(context, transaction, accountID)
	if err != nil {
		return mealState{}, false, err
	}
	if len(candidates) == 0 {
		return mealState{Status: mealStatusCandidatePoolEmpty}, false, errCandidatePoolEmpty
	}
	selected := m.chooseWeightedDish(candidates)

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
		`INSERT INTO decisions (meal_id, dish_id, mode, status, created_at)
		 VALUES (?, ?, 'pool', 'active', unixepoch())`,
		decision.MealID,
		selected.id,
	)
	if err != nil {
		return mealState{}, false, err
	}
	decision.ID, err = decisionResult.LastInsertId()
	if err != nil {
		return mealState{}, false, err
	}
	decision.Mode = "pool"
	decision.Dish = catalogDish(selected.id, selected.name)
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
			`SELECT decisions.id, meals.id, decisions.mode,
			        catalog_dishes.source_path, catalog_dishes.name
			 FROM decisions
			 JOIN meals ON meals.id = decisions.meal_id
			 JOIN catalog_dishes ON catalog_dishes.source_path = decisions.dish_id
			 WHERE decisions.id = ? AND meals.account_id = ? AND meals.status = 'active'
			       AND decisions.status = 'active' AND decisions.rerolled_to_id IS NULL`,
			replacementID.Int64,
			accountID,
		))
		if errors.Is(err, sql.ErrNoRows) {
			return mealState{}, errDecisionNotFound
		}
		return replacement, err
	}
	if mealStatus != "active" || decisionStatus != "active" {
		return mealState{}, errDecisionNotFound
	}

	candidates, err := m.poolCandidates(context, transaction, accountID)
	if err != nil {
		return mealState{}, err
	}
	if len(candidates) == 0 {
		return mealState{Status: mealStatusCandidatePoolEmpty}, errCandidatePoolEmpty
	}

	shown := make(map[string]int)
	rows, err := transaction.QueryContext(
		context,
		"SELECT dish_id, COUNT(*) FROM decisions WHERE meal_id = ? GROUP BY dish_id",
		mealID,
	)
	if err != nil {
		return mealState{}, err
	}
	for rows.Next() {
		var dishID string
		var count int
		if err := rows.Scan(&dishID, &count); err != nil {
			rows.Close()
			return mealState{}, err
		}
		shown[dishID] = count
	}
	if err := rows.Close(); err != nil {
		return mealState{}, err
	}
	if err := rows.Err(); err != nil {
		return mealState{}, err
	}

	leastShown := make([]weightedDish, 0, len(candidates))
	minShown := -1
	for _, candidate := range candidates {
		if len(candidates) > 1 && candidate.id == currentDishID {
			continue
		}
		count := shown[candidate.id]
		switch {
		case minShown == -1 || count < minShown:
			minShown = count
			leastShown = append(leastShown[:0], candidate)
		case count == minShown:
			leastShown = append(leastShown, candidate)
		}
	}
	selected := m.chooseWeightedDish(leastShown)

	if _, err := transaction.ExecContext(
		context,
		"UPDATE decisions SET rerolled_to_id = id WHERE id = ? AND rerolled_to_id IS NULL",
		decisionID,
	); err != nil {
		return mealState{}, err
	}
	result, err := transaction.ExecContext(
		context,
		`INSERT INTO decisions (meal_id, dish_id, mode, status, created_at)
		 VALUES (?, ?, 'pool', 'active', unixepoch())`,
		mealID,
		selected.id,
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
		Mode:   "pool",
		Dish:   catalogDish(selected.id, selected.name),
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
	var status, dishID, dishName string
	var existingSequence, rerolledToID sql.NullInt64
	err = transaction.QueryRowContext(
		context,
		`SELECT meals.id, decisions.status, catalog_dishes.source_path,
		        catalog_dishes.name, eating_records.sequence, decisions.rerolled_to_id
		 FROM decisions
		 JOIN meals ON meals.id = decisions.meal_id
		 JOIN catalog_dishes ON catalog_dishes.source_path = decisions.dish_id
		 LEFT JOIN eating_records ON eating_records.decision_id = decisions.id
		 WHERE decisions.id = ? AND meals.account_id = ?`,
		decisionID,
		accountID,
	).Scan(&mealID, &status, &dishID, &dishName, &existingSequence, &rerolledToID)
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
	if err := transaction.Commit(); err != nil {
		return acceptanceResponse{}, err
	}
	return result, nil
}

func (a *App) resumeMeal(context *gin.Context) {
	account, ok := a.currentAccount(context)
	if !ok {
		return
	}
	state, err := a.mealLifecycle.Resume(context, account.ID)
	if err != nil {
		writeInternalError(context, "resume Meal lifecycle", err)
		return
	}
	context.JSON(http.StatusOK, state)
}

func (a *App) beginMeal(context *gin.Context) {
	account, ok := a.currentAccount(context)
	if !ok {
		return
	}
	state, created, err := a.mealLifecycle.Begin(context, account.ID)
	switch {
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
	account, ok := a.currentAccount(context)
	if !ok {
		return
	}
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
	account, ok := a.currentAccount(context)
	if !ok {
		return
	}
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
