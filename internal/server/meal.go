package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/jasper0507/what-to-eat/internal/meal"
)

type tasteRatingInput struct {
	Rating int `json:"rating"`
}

func (a *App) resumeMeal(context *gin.Context) {
	owner := sessionAccount(context)
	state, err := a.meals.Resume(context, owner.ID)
	if err != nil {
		writeInternalError(context, "resume Meal lifecycle", err)
		return
	}
	context.JSON(http.StatusOK, state)
}

func (a *App) beginMeal(context *gin.Context) {
	owner := sessionAccount(context)
	state, created, err := a.meals.Begin(context, owner.ID)
	switch {
	case errors.Is(err, meal.ErrPendingRatings):
		writeError(
			context,
			http.StatusConflict,
			codePendingRatings,
			"请先解决所有 Pending rating，再开始新的 Decision",
		)
	case errors.Is(err, meal.ErrCandidatePoolEmpty):
		writeError(
			context,
			http.StatusConflict,
			codeCandidatePoolEmpty,
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
	owner := sessionAccount(context)
	decisionID, err := strconv.ParseInt(context.Param("decisionID"), 10, 64)
	if err != nil || decisionID <= 0 {
		writeError(context, http.StatusNotFound, codeDecisionNotFound, "Decision 不存在")
		return
	}
	state, err := a.meals.Reroll(context, owner.ID, decisionID)
	switch {
	case errors.Is(err, meal.ErrCandidatePoolEmpty):
		writeError(
			context,
			http.StatusConflict,
			codeCandidatePoolEmpty,
			"Candidate pool 为空，无法 Reroll Decision",
		)
	case errors.Is(err, meal.ErrDecisionNotFound):
		writeError(context, http.StatusNotFound, codeDecisionNotFound, "Decision 不存在")
	case err != nil:
		writeInternalError(context, "reroll Decision", err)
	default:
		context.JSON(http.StatusOK, state)
	}
}

func (a *App) acceptDecision(context *gin.Context) {
	owner := sessionAccount(context)
	decisionID, err := strconv.ParseInt(context.Param("decisionID"), 10, 64)
	if err != nil || decisionID <= 0 {
		writeError(context, http.StatusNotFound, codeDecisionNotFound, "Decision 不存在")
		return
	}
	result, err := a.meals.Accept(context, owner.ID, decisionID)
	switch {
	case errors.Is(err, meal.ErrDecisionNotFound):
		writeError(context, http.StatusNotFound, codeDecisionNotFound, "Decision 不存在")
	case err != nil:
		writeInternalError(context, "accept Decision", err)
	default:
		context.JSON(http.StatusOK, result)
	}
}

func (a *App) ratePendingRating(context *gin.Context) {
	owner := sessionAccount(context)
	pendingRatingID, err := strconv.ParseInt(context.Param("pendingRatingID"), 10, 64)
	if err != nil || pendingRatingID <= 0 {
		writeError(context, http.StatusNotFound, codePendingRatingNotFound, "Pending rating 不存在")
		return
	}
	var input tasteRatingInput
	if err := context.ShouldBindJSON(&input); err != nil || input.Rating < 1 || input.Rating > 5 {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "Taste rating 必须为 1–5")
		return
	}
	result, err := a.meals.Rate(context, owner.ID, pendingRatingID, input.Rating)
	switch {
	case errors.Is(err, meal.ErrPendingRatingNotFound):
		writeError(context, http.StatusNotFound, codePendingRatingNotFound, "Pending rating 不存在")
	case errors.Is(err, meal.ErrTasteRatingConflict):
		writeError(context, http.StatusConflict, codeRatingConflict, "Taste rating 与已有结果冲突")
	case err != nil:
		writeInternalError(context, "resolve Pending rating", err)
	default:
		context.JSON(http.StatusOK, result)
	}
}
