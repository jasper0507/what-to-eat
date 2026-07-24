package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type candidateDishResponse struct {
	catalogDishResponse
	PreferenceWeight float64 `json:"preference_weight"`
}

type candidateDishInput struct {
	DishID           string  `json:"dish_id"`
	PreferenceWeight float64 `json:"preference_weight"`
}

func (a *App) listCandidatePool(context *gin.Context) {
	account, ok := a.currentAccount(context)
	if !ok {
		return
	}

	rows, err := a.db.QueryContext(
		context,
		`SELECT catalog_dishes.source_path, catalog_dishes.name, candidate_pool.preference_weight
		 FROM candidate_pool
		 JOIN catalog_dishes ON catalog_dishes.source_path = candidate_pool.dish_id
		 WHERE candidate_pool.account_id = ?
		 ORDER BY catalog_dishes.name`,
		account.ID,
	)
	if err != nil {
		writeInternalError(context, "list Candidate pool", err)
		return
	}
	defer rows.Close()

	dishes := make([]candidateDishResponse, 0)
	for rows.Next() {
		var sourcePath, name string
		var weight float64
		if err := rows.Scan(&sourcePath, &name, &weight); err != nil {
			writeInternalError(context, "read Candidate pool member", err)
			return
		}
		dishes = append(dishes, candidateDishResponse{
			catalogDishResponse: catalogDish(sourcePath, name),
			PreferenceWeight:    weight,
		})
	}
	if err := rows.Err(); err != nil {
		writeInternalError(context, "finish Candidate pool list", err)
		return
	}

	context.JSON(http.StatusOK, gin.H{"dishes": dishes})
}

func (a *App) addCandidatePoolDish(context *gin.Context) {
	account, ok := a.currentAccount(context)
	if !ok {
		return
	}

	var input candidateDishInput
	if err := context.ShouldBindJSON(&input); err != nil ||
		!validDishID(input.DishID) ||
		!validPreferenceWeight(input.PreferenceWeight) {
		writeError(context, http.StatusBadRequest, "invalid_request", "Dish 或 Preference weight 无效")
		return
	}

	result, err := a.db.ExecContext(
		context,
		`INSERT INTO candidate_pool (account_id, dish_id, preference_weight)
		 SELECT ?, source_path, ?
		 FROM catalog_dishes
		 WHERE source_path = ?
		 ON CONFLICT(account_id, dish_id) DO NOTHING`,
		account.ID,
		input.PreferenceWeight,
		input.DishID,
	)
	if err != nil {
		writeInternalError(context, "add Candidate pool member", err)
		return
	}
	affected, err := result.RowsAffected()
	if err != nil {
		writeInternalError(context, "read added Candidate pool member", err)
		return
	}
	if affected == 0 {
		writeError(context, http.StatusNotFound, "dish_unavailable", "无法加入 Candidate pool")
		return
	}
	context.Status(http.StatusCreated)
}

func (a *App) updateCandidatePoolDish(context *gin.Context) {
	account, ok := a.currentAccount(context)
	if !ok {
		return
	}

	var input candidateDishInput
	if err := context.ShouldBindJSON(&input); err != nil ||
		!validDishID(input.DishID) ||
		!validPreferenceWeight(input.PreferenceWeight) {
		writeError(context, http.StatusBadRequest, "invalid_request", "Dish 或 Preference weight 无效")
		return
	}

	result, err := a.db.ExecContext(
		context,
		`UPDATE candidate_pool
		 SET preference_weight = ?
		 WHERE account_id = ? AND dish_id = ?`,
		input.PreferenceWeight,
		account.ID,
		input.DishID,
	)
	if err != nil {
		writeInternalError(context, "update Candidate pool member", err)
		return
	}
	affected, err := result.RowsAffected()
	if err != nil {
		writeInternalError(context, "read updated Candidate pool member", err)
		return
	}
	if affected == 0 {
		writeError(context, http.StatusNotFound, "candidate_pool_member_not_found", "Candidate pool 中没有这个 Dish")
		return
	}

	context.Status(http.StatusNoContent)
}

func (a *App) removeCandidatePoolDish(context *gin.Context) {
	account, ok := a.currentAccount(context)
	if !ok {
		return
	}
	dishID := context.Query("dish_id")
	if !validDishID(dishID) {
		writeError(context, http.StatusBadRequest, "invalid_request", "Dish 无效")
		return
	}

	result, err := a.db.ExecContext(
		context,
		"DELETE FROM candidate_pool WHERE account_id = ? AND dish_id = ?",
		account.ID,
		dishID,
	)
	if err != nil {
		writeInternalError(context, "remove Candidate pool member", err)
		return
	}
	affected, err := result.RowsAffected()
	if err != nil {
		writeInternalError(context, "read removed Candidate pool member", err)
		return
	}
	if affected == 0 {
		writeError(context, http.StatusNotFound, "candidate_pool_member_not_found", "Candidate pool 中没有这个 Dish")
		return
	}
	context.Status(http.StatusNoContent)
}

func validPreferenceWeight(weight float64) bool {
	return weight >= 0.1 && weight <= 5
}

func validDishID(dishID string) bool {
	return dishID != "" && dishID == strings.TrimSpace(dishID) && len(dishID) <= 500
}
