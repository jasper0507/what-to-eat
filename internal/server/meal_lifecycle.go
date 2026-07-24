package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	mealStatusCandidatePoolEmpty mealReadiness = "candidate_pool_empty"
	mealStatusReady              mealReadiness = "ready"
)

var (
	errCandidatePoolEmpty     = errors.New("Candidate pool is empty")
	errDecisionNotImplemented = errors.New("Decision is not implemented")
)

type mealLifecycle struct {
	db *sql.DB
}

type mealReadiness string

func (m *mealLifecycle) Resume(context context.Context, accountID int64) (mealReadiness, error) {
	var hasCandidates bool
	if err := m.db.QueryRowContext(
		context,
		"SELECT EXISTS(SELECT 1 FROM candidate_pool WHERE account_id = ?)",
		accountID,
	).Scan(&hasCandidates); err != nil {
		return "", err
	}
	if !hasCandidates {
		return mealStatusCandidatePoolEmpty, nil
	}
	return mealStatusReady, nil
}

func (m *mealLifecycle) Begin(context context.Context, accountID int64) (mealReadiness, error) {
	readiness, err := m.Resume(context, accountID)
	if err != nil {
		return "", err
	}
	if readiness == mealStatusCandidatePoolEmpty {
		return readiness, errCandidatePoolEmpty
	}
	return readiness, errDecisionNotImplemented
}

func (a *App) resumeMeal(context *gin.Context) {
	account, ok := a.currentAccount(context)
	if !ok {
		return
	}
	readiness, err := a.mealLifecycle.Resume(context, account.ID)
	if err != nil {
		writeInternalError(context, "resume Meal lifecycle", err)
		return
	}
	context.JSON(http.StatusOK, gin.H{"status": readiness})
}

func (a *App) beginMeal(context *gin.Context) {
	account, ok := a.currentAccount(context)
	if !ok {
		return
	}
	readiness, err := a.mealLifecycle.Begin(context, account.ID)
	switch {
	case errors.Is(err, errCandidatePoolEmpty):
		writeError(
			context,
			http.StatusConflict,
			string(readiness),
			"Candidate pool 为空，无法创建 Decision",
		)
	case errors.Is(err, errDecisionNotImplemented):
		writeError(context, http.StatusNotImplemented, "decision_not_implemented", "Decision 功能尚未开放")
	case err != nil:
		writeInternalError(context, "begin Meal lifecycle", err)
	default:
		writeInternalError(context, "begin Meal lifecycle", errors.New("missing Decision result"))
	}
}
