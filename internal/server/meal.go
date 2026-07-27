package server

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jasper0507/what-to-eat/internal/catalog"
	"github.com/jasper0507/what-to-eat/internal/meal"
)

type tasteRatingInput struct {
	Rating int `json:"rating"`
}

// localHourInput 是场合因子的客户端上报（ADR-0022）。缺省回落服务器本地
// 小时——揭示永远有一个时段语境，宁可粗也不空。
type localHourInput struct {
	LocalHour *int `json:"local_hour"`
}

type handPickInput struct {
	DishID string `json:"dish_id"`
}

func resolveLocalHour(reported *int) (int, bool) {
	if reported == nil {
		return time.Now().Hour(), true
	}
	if *reported < 0 || *reported > 23 {
		return 0, false
	}
	return *reported, true
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
	var input localHourInput
	// 空体合法（老客户端不带 local_hour）
	if context.Request.ContentLength > 0 {
		if err := context.ShouldBindJSON(&input); err != nil {
			writeError(context, http.StatusBadRequest, codeInvalidRequest, "请求无效")
			return
		}
	}
	hour, ok := resolveLocalHour(input.LocalHour)
	if !ok {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "local_hour 必须为 0–23")
		return
	}
	state, created, err := a.meals.Begin(context, owner.ID, hour)
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
	var input localHourInput
	if context.Request.ContentLength > 0 {
		if err := context.ShouldBindJSON(&input); err != nil {
			writeError(context, http.StatusBadRequest, codeInvalidRequest, "请求无效")
			return
		}
	}
	hour, ok := resolveLocalHour(input.LocalHour)
	if !ok {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "local_hour 必须为 0–23")
		return
	}
	state, err := a.meals.Reroll(context, owner.ID, decisionID, hour)
	switch {
	case errors.Is(err, meal.ErrRerollBudgetExhausted):
		writeError(
			context,
			http.StatusConflict,
			codeRerollBudgetExhausted,
			"这顿的换菜次数用完了",
		)
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

// abandonMeal 放弃本顿：三出口之一，无吃饭记录、不进冷却。
func (a *App) abandonMeal(context *gin.Context) {
	owner := sessionAccount(context)
	state, err := a.meals.Abandon(context, owner.ID)
	switch {
	case errors.Is(err, meal.ErrNoActiveMeal):
		writeError(context, http.StatusConflict, codeMealNotFound, "没有进行中的这一顿")
	case err != nil:
		writeInternalError(context, "abandon Meal", err)
	default:
		context.JSON(http.StatusOK, state)
	}
}

// handPickDish 亲自点一道：仅 Reroll budget 耗尽时解锁。
func (a *App) handPickDish(context *gin.Context) {
	owner := sessionAccount(context)
	var input handPickInput
	if err := context.ShouldBindJSON(&input); err != nil ||
		!catalog.ValidDishID(input.DishID) {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "Dish 无效")
		return
	}
	result, err := a.meals.HandPick(context, owner.ID, input.DishID)
	switch {
	case errors.Is(err, meal.ErrNoActiveMeal):
		writeError(context, http.StatusConflict, codeMealNotFound, "没有进行中的这一顿")
	case errors.Is(err, meal.ErrHandPickLocked):
		writeError(
			context,
			http.StatusConflict,
			codeHandPickLocked,
			"换菜次数还没用完，先看看它给的",
		)
	case errors.Is(err, meal.ErrDishNotInPool):
		writeError(context, http.StatusNotFound, codeDishUnavailable, "这道菜不在 Candidate pool 里")
	case err != nil:
		writeInternalError(context, "hand-pick Dish", err)
	default:
		context.JSON(http.StatusOK, result)
	}
}

// listEatingRecords 轻历史：最近吃过、评过几档、现在几档。
func (a *App) listEatingRecords(context *gin.Context) {
	owner := sessionAccount(context)
	limit := 20
	if raw := context.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(context, http.StatusBadRequest, codeInvalidQuery, "limit 必须为 1–100")
			return
		}
		limit = parsed
	}
	entries, err := a.meals.History(context, owner.ID, limit)
	if err != nil {
		writeInternalError(context, "list Eating records", err)
		return
	}
	context.JSON(http.StatusOK, gin.H{"records": entries})
}

// rateEatingRecord 轻历史的补评分：可选、绝不拦路。
func (a *App) rateEatingRecord(context *gin.Context) {
	owner := sessionAccount(context)
	recordID, err := strconv.ParseInt(context.Param("recordID"), 10, 64)
	if err != nil || recordID <= 0 {
		writeError(context, http.StatusNotFound, codeEatingRecordNotFound, "吃饭记录不存在")
		return
	}
	var input tasteRatingInput
	if err := context.ShouldBindJSON(&input); err != nil || input.Rating < 1 || input.Rating > 5 {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "Taste rating 必须为 1–5")
		return
	}
	result, err := a.meals.RateRecord(context, owner.ID, recordID, input.Rating)
	switch {
	case errors.Is(err, meal.ErrEatingRecordNotFound):
		writeError(context, http.StatusNotFound, codeEatingRecordNotFound, "吃饭记录不存在")
	case errors.Is(err, meal.ErrTasteRatingConflict):
		writeError(context, http.StatusConflict, codeRatingConflict, "Taste rating 与已有结果冲突")
	case err != nil:
		writeInternalError(context, "rate Eating record", err)
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
