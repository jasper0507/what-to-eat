package meal

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jasper0507/what-to-eat/internal/catalog"
)

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
	if err := m.pool.ResetSwaps(context, transaction, accountID, dishID); err != nil {
		return Acceptance{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Acceptance{}, err
	}
	return result, nil
}
