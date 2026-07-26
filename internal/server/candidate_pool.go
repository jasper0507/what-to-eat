package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jasper0507/what-to-eat/internal/catalog"
	"github.com/jasper0507/what-to-eat/internal/pool"
)

type candidateDishInput struct {
	DishID           string  `json:"dish_id"`
	PreferenceWeight float64 `json:"preference_weight"`
}

func (a *App) listCandidatePool(context *gin.Context) {
	owner := sessionAccount(context)
	dishes, err := a.pool.List(context, owner.ID)
	if err != nil {
		writeInternalError(context, "list Candidate pool", err)
		return
	}
	context.JSON(http.StatusOK, gin.H{"dishes": dishes})
}

func (a *App) addCandidatePoolDish(context *gin.Context) {
	owner := sessionAccount(context)

	var input candidateDishInput
	if err := context.ShouldBindJSON(&input); err != nil ||
		!catalog.ValidDishID(input.DishID) ||
		!pool.ValidWeight(input.PreferenceWeight) {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "Dish 或 Preference weight 无效")
		return
	}

	added, err := a.pool.Add(context, owner.ID, input.DishID, input.PreferenceWeight)
	if err != nil {
		writeInternalError(context, "add Candidate pool member", err)
		return
	}
	if !added {
		writeError(context, http.StatusNotFound, codeDishUnavailable, "无法加入 Candidate pool")
		return
	}
	context.Status(http.StatusCreated)
}

func (a *App) updateCandidatePoolDish(context *gin.Context) {
	owner := sessionAccount(context)

	var input candidateDishInput
	if err := context.ShouldBindJSON(&input); err != nil ||
		!catalog.ValidDishID(input.DishID) ||
		!pool.ValidWeight(input.PreferenceWeight) {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "Dish 或 Preference weight 无效")
		return
	}

	found, err := a.pool.UpdateWeight(context, owner.ID, input.DishID, input.PreferenceWeight)
	if err != nil {
		writeInternalError(context, "update Candidate pool member", err)
		return
	}
	if !found {
		writeError(context, http.StatusNotFound, codePoolMemberNotFound, "Candidate pool 中没有这个 Dish")
		return
	}
	context.Status(http.StatusNoContent)
}

func (a *App) removeCandidatePoolDish(context *gin.Context) {
	owner := sessionAccount(context)
	dishID := context.Query("dish_id")
	if !catalog.ValidDishID(dishID) {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "Dish 无效")
		return
	}

	found, err := a.pool.Remove(context, owner.ID, dishID)
	if err != nil {
		writeInternalError(context, "remove Candidate pool member", err)
		return
	}
	if !found {
		writeError(context, http.StatusNotFound, codePoolMemberNotFound, "Candidate pool 中没有这个 Dish")
		return
	}
	context.Status(http.StatusNoContent)
}
