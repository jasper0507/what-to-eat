package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Preference weight 的合法域，唯一来源；schema CHECK 仅作存储层兜底。
const (
	minPreferenceWeight = 0.1
	maxPreferenceWeight = 5
)

var errDishRejected = errors.New("Dish carries a rejection mark")

// candidatePool 拥有 Candidate pool 的 membership 与 Preference weight 不变量，
// 以及 rejection mark 的全部写侧语义。候选选择（cooldown/recency/加权抽样）
// 依 ADR-0019 留在 mealLifecycle。
type candidatePool struct {
	db *sql.DB
}

func newCandidatePool(db *sql.DB) *candidatePool {
	return &candidatePool{db: db}
}

type candidateDishResponse struct {
	catalogDishResponse
	PreferenceWeight float64 `json:"preference_weight"`
}

type candidateDishInput struct {
	DishID           string  `json:"dish_id"`
	PreferenceWeight float64 `json:"preference_weight"`
}

func (p *candidatePool) List(
	context context.Context,
	accountID int64,
) ([]candidateDishResponse, error) {
	rows, err := p.db.QueryContext(
		context,
		`SELECT catalog_dishes.source_path, catalog_dishes.name, candidate_pool.preference_weight
		 FROM candidate_pool
		 JOIN catalog_dishes ON catalog_dishes.source_path = candidate_pool.dish_id
		 WHERE candidate_pool.account_id = ?
		 ORDER BY catalog_dishes.name`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dishes := make([]candidateDishResponse, 0)
	for rows.Next() {
		var sourcePath, name string
		var weight float64
		if err := rows.Scan(&sourcePath, &name, &weight); err != nil {
			return nil, err
		}
		dishes = append(dishes, candidateDishResponse{
			catalogDishResponse: catalogDish(sourcePath, name),
			PreferenceWeight:    weight,
		})
	}
	return dishes, rows.Err()
}

// Add 把 Catalog 中的 Dish 加入 Candidate pool。显式添加同时清除该 Dish 的
// rejection mark：Eater 主动加回被拒的菜即视为撤销拒绝。只有当既没有插入
// 池成员也没有清除标记（Dish 已在池中且未被拒，或不在 Catalog 中）时才返回
// added=false。
func (p *candidatePool) Add(
	context context.Context,
	accountID int64,
	dishID string,
	weight float64,
) (added bool, err error) {
	transaction, err := p.db.BeginTx(context, nil)
	if err != nil {
		return false, err
	}
	defer transaction.Rollback()

	clearedResult, err := transaction.ExecContext(
		context,
		"DELETE FROM rejection_marks WHERE account_id = ? AND dish_id = ?",
		accountID,
		dishID,
	)
	if err != nil {
		return false, err
	}
	cleared, err := clearedResult.RowsAffected()
	if err != nil {
		return false, err
	}

	insertResult, err := transaction.ExecContext(
		context,
		`INSERT INTO candidate_pool (account_id, dish_id, preference_weight)
		 SELECT ?, source_path, ?
		 FROM catalog_dishes
		 WHERE source_path = ?
		 ON CONFLICT(account_id, dish_id) DO NOTHING`,
		accountID,
		weight,
		dishID,
	)
	if err != nil {
		return false, err
	}
	inserted, err := insertResult.RowsAffected()
	if err != nil {
		return false, err
	}
	if inserted == 0 && cleared == 0 {
		return false, nil
	}
	return true, transaction.Commit()
}

func (p *candidatePool) UpdateWeight(
	context context.Context,
	accountID int64,
	dishID string,
	weight float64,
) (found bool, err error) {
	result, err := p.db.ExecContext(
		context,
		`UPDATE candidate_pool
		 SET preference_weight = ?
		 WHERE account_id = ? AND dish_id = ?`,
		weight,
		accountID,
		dishID,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func (p *candidatePool) Remove(
	context context.Context,
	accountID int64,
	dishID string,
) (found bool, err error) {
	result, err := p.db.ExecContext(
		context,
		"DELETE FROM candidate_pool WHERE account_id = ? AND dish_id = ?",
		accountID,
		dishID,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

// Admit 在调用方事务内执行 pool admission。评分派生的 weight 覆盖已有值：
// Taste rating 是最新的真实进食信号。带 rejection mark 的 Dish 不可接纳。
func (p *candidatePool) Admit(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
	dishID string,
	weight float64,
) error {
	var rejected bool
	if err := transaction.QueryRowContext(
		context,
		`SELECT EXISTS(
			SELECT 1
			FROM rejection_marks
			WHERE account_id = ? AND dish_id = ?
		 )`,
		accountID,
		dishID,
	).Scan(&rejected); err != nil {
		return err
	}
	if rejected {
		return errDishRejected
	}
	_, err := transaction.ExecContext(
		context,
		`INSERT INTO candidate_pool (account_id, dish_id, preference_weight)
		 VALUES (?, ?, ?)
		 ON CONFLICT(account_id, dish_id)
		 DO UPDATE SET preference_weight = excluded.preference_weight`,
		accountID,
		dishID,
		weight,
	)
	return err
}

// Reject 在调用方事务内执行拒绝：移出 Candidate pool 并落 rejection mark。
func (p *candidatePool) Reject(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
	dishID string,
	rating int,
) error {
	if _, err := transaction.ExecContext(
		context,
		"DELETE FROM candidate_pool WHERE account_id = ? AND dish_id = ?",
		accountID,
		dishID,
	); err != nil {
		return err
	}
	_, err := transaction.ExecContext(
		context,
		`INSERT INTO rejection_marks (account_id, dish_id, rating, created_at)
		 VALUES (?, ?, ?, unixepoch())
		 ON CONFLICT(account_id, dish_id)
		 DO UPDATE SET rating = excluded.rating, created_at = excluded.created_at`,
		accountID,
		dishID,
		rating,
	)
	return err
}

func (a *App) listCandidatePool(context *gin.Context) {
	account := sessionAccount(context)
	dishes, err := a.candidatePool.List(context, account.ID)
	if err != nil {
		writeInternalError(context, "list Candidate pool", err)
		return
	}
	context.JSON(http.StatusOK, gin.H{"dishes": dishes})
}

func (a *App) addCandidatePoolDish(context *gin.Context) {
	account := sessionAccount(context)

	var input candidateDishInput
	if err := context.ShouldBindJSON(&input); err != nil ||
		!validDishID(input.DishID) ||
		!validPreferenceWeight(input.PreferenceWeight) {
		writeError(context, http.StatusBadRequest, "invalid_request", "Dish 或 Preference weight 无效")
		return
	}

	added, err := a.candidatePool.Add(context, account.ID, input.DishID, input.PreferenceWeight)
	if err != nil {
		writeInternalError(context, "add Candidate pool member", err)
		return
	}
	if !added {
		writeError(context, http.StatusNotFound, "dish_unavailable", "无法加入 Candidate pool")
		return
	}
	context.Status(http.StatusCreated)
}

func (a *App) updateCandidatePoolDish(context *gin.Context) {
	account := sessionAccount(context)

	var input candidateDishInput
	if err := context.ShouldBindJSON(&input); err != nil ||
		!validDishID(input.DishID) ||
		!validPreferenceWeight(input.PreferenceWeight) {
		writeError(context, http.StatusBadRequest, "invalid_request", "Dish 或 Preference weight 无效")
		return
	}

	found, err := a.candidatePool.UpdateWeight(context, account.ID, input.DishID, input.PreferenceWeight)
	if err != nil {
		writeInternalError(context, "update Candidate pool member", err)
		return
	}
	if !found {
		writeError(context, http.StatusNotFound, "candidate_pool_member_not_found", "Candidate pool 中没有这个 Dish")
		return
	}
	context.Status(http.StatusNoContent)
}

func (a *App) removeCandidatePoolDish(context *gin.Context) {
	account := sessionAccount(context)
	dishID := context.Query("dish_id")
	if !validDishID(dishID) {
		writeError(context, http.StatusBadRequest, "invalid_request", "Dish 无效")
		return
	}

	found, err := a.candidatePool.Remove(context, account.ID, dishID)
	if err != nil {
		writeInternalError(context, "remove Candidate pool member", err)
		return
	}
	if !found {
		writeError(context, http.StatusNotFound, "candidate_pool_member_not_found", "Candidate pool 中没有这个 Dish")
		return
	}
	context.Status(http.StatusNoContent)
}

func validPreferenceWeight(weight float64) bool {
	return weight >= minPreferenceWeight && weight <= maxPreferenceWeight
}

func validDishID(dishID string) bool {
	return dishID != "" && dishID == strings.TrimSpace(dishID) && len(dishID) <= 500
}
