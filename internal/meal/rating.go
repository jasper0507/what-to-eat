package meal

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jasper0507/what-to-eat/internal/catalog"
	"github.com/jasper0507/what-to-eat/internal/engine"
	"github.com/jasper0507/what-to-eat/internal/pool"
)

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
