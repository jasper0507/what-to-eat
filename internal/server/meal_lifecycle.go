package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	mealStatusCandidatePoolEmpty = "candidate_pool_empty"
	mealStatusReady              = "ready"
)

var (
	errCandidatePoolEmpty     = errors.New("Candidate pool is empty")
	errDecisionNotImplemented = errors.New("Decision is not implemented")
)

type mealLifecycle struct {
	db *sql.DB
}

type mealResume struct {
	Status  string       `json:"status"`
	Actions []mealAction `json:"actions"`
}

type mealAction struct {
	Kind string `json:"kind"`
	Href string `json:"href"`
}

func (m *mealLifecycle) Resume(context context.Context, accountID int64) (mealResume, error) {
	var hasCandidates bool
	if err := m.db.QueryRowContext(
		context,
		"SELECT EXISTS(SELECT 1 FROM candidate_pool WHERE account_id = ?)",
		accountID,
	).Scan(&hasCandidates); err != nil {
		return mealResume{}, err
	}
	if !hasCandidates {
		return mealResume{
			Status: mealStatusCandidatePoolEmpty,
			Actions: []mealAction{{
				Kind: "catalog_search",
				Href: "/candidate-pool",
			}},
		}, nil
	}
	return mealResume{Status: mealStatusReady, Actions: []mealAction{}}, nil
}

func (m *mealLifecycle) Begin(context context.Context, accountID int64) (mealResume, error) {
	resume, err := m.Resume(context, accountID)
	if err != nil {
		return mealResume{}, err
	}
	if resume.Status == mealStatusCandidatePoolEmpty {
		return resume, errCandidatePoolEmpty
	}
	return resume, errDecisionNotImplemented
}

func (a *App) resumeMeal(context *gin.Context) {
	account, ok := a.currentAccount(context)
	if !ok {
		return
	}
	resume, err := a.mealLifecycle.Resume(context, account.ID)
	if err != nil {
		writeInternalError(context, "resume Meal lifecycle", err)
		return
	}
	context.JSON(http.StatusOK, resume)
}

func (a *App) beginMeal(context *gin.Context) {
	account, ok := a.currentAccount(context)
	if !ok {
		return
	}
	resume, err := a.mealLifecycle.Begin(context, account.ID)
	switch {
	case errors.Is(err, errCandidatePoolEmpty):
		context.JSON(http.StatusConflict, gin.H{
			"error": gin.H{
				"code":    mealStatusCandidatePoolEmpty,
				"message": "Candidate pool 为空，无法创建 Decision",
			},
			"actions": resume.Actions,
		})
	case errors.Is(err, errDecisionNotImplemented):
		writeError(context, http.StatusNotImplemented, "decision_not_implemented", "Decision 功能尚未开放")
	case err != nil:
		writeInternalError(context, "begin Meal lifecycle", err)
	default:
		writeInternalError(context, "begin Meal lifecycle", errors.New("missing Decision result"))
	}
}
