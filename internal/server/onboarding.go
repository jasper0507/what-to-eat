package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const (
	onboardingNotStarted onboardingStatus = "not_started"
	onboardingInProgress onboardingStatus = "in_progress"
	onboardingFailed     onboardingStatus = "failed"
	onboardingCompleted  onboardingStatus = "completed"
	onboardingManual     onboardingStatus = "manual"

	onboardingRateLimit    = 10
	onboardingRatePeriod   = time.Minute
	onboardingAttemptLimit = 30
	maxOnboardingMessage   = 600
)

var (
	errNIMUnavailable         = errors.New("NVIDIA NIM is unavailable")
	errOnboardingFinished     = errors.New("Onboarding interview is already finished")
	errOnboardingRetryNeeded  = errors.New("Onboarding interview must be retried")
	errOnboardingRetryInvalid = errors.New("Onboarding interview cannot be retried")
	errOnboardingRateLimited  = errors.New("Onboarding interview is rate limited")
	errOnboardingLimitReached = errors.New("Onboarding interview attempt limit reached")
)

type onboardingStatus string

type onboardingMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type onboardingState struct {
	Status   onboardingStatus    `json:"status"`
	Messages []onboardingMessage `json:"messages"`
	CanRetry bool                `json:"can_retry"`
}

type onboardingMessageInput struct {
	Message string `json:"message"`
}

type onboardingRateWindow struct {
	count     int
	expiresAt time.Time
}

type onboardingInterview struct {
	db      *sql.DB
	pool    *candidatePool
	nim     onboardingNIM
	locksMu sync.Mutex
	locks   map[int64]*sync.Mutex
	rateMu  sync.Mutex
	rate    map[int64]onboardingRateWindow
}

func newOnboardingInterview(db *sql.DB, pool *candidatePool, nim onboardingNIM) *onboardingInterview {
	return &onboardingInterview{
		db:    db,
		pool:  pool,
		nim:   nim,
		locks: make(map[int64]*sync.Mutex),
		rate:  make(map[int64]onboardingRateWindow),
	}
}

func (o *onboardingInterview) State(
	context context.Context,
	accountID int64,
) (onboardingState, error) {
	lock := o.accountLock(accountID)
	lock.Lock()
	defer lock.Unlock()
	return o.state(context, accountID)
}

func (o *onboardingInterview) Send(
	context context.Context,
	accountID int64,
	message string,
) (onboardingState, error) {
	return o.exchange(context, accountID, message, false)
}

func (o *onboardingInterview) Retry(
	context context.Context,
	accountID int64,
) (onboardingState, error) {
	return o.exchange(context, accountID, "", true)
}

func (o *onboardingInterview) Manual(
	context context.Context,
	accountID int64,
) (onboardingState, error) {
	lock := o.accountLock(accountID)
	lock.Lock()
	defer lock.Unlock()

	state, err := o.state(context, accountID)
	if err != nil {
		return onboardingState{}, err
	}
	if state.Status == onboardingCompleted {
		return onboardingState{}, errOnboardingFinished
	}
	now := time.Now().Unix()
	if _, err := o.db.ExecContext(
		context,
		`INSERT INTO onboarding_interviews (account_id, status, updated_at)
		 VALUES (?, 'manual', ?)
		 ON CONFLICT(account_id) DO UPDATE SET
			status = 'manual',
			updated_at = excluded.updated_at`,
		accountID,
		now,
	); err != nil {
		return onboardingState{}, err
	}
	return o.state(context, accountID)
}

func (o *onboardingInterview) exchange(
	context context.Context,
	accountID int64,
	message string,
	retry bool,
) (onboardingState, error) {
	lock := o.accountLock(accountID)
	lock.Lock()
	defer lock.Unlock()

	state, err := o.state(context, accountID)
	if err != nil {
		return onboardingState{}, err
	}
	if state.Status == onboardingCompleted || state.Status == onboardingManual {
		return onboardingState{}, errOnboardingFinished
	}
	if retry && !state.CanRetry {
		return onboardingState{}, errOnboardingRetryInvalid
	}
	if !retry && state.CanRetry {
		return onboardingState{}, errOnboardingRetryNeeded
	}

	var attempts int
	err = o.db.QueryRowContext(
		context,
		"SELECT attempt_count FROM onboarding_interviews WHERE account_id = ?",
		accountID,
	).Scan(&attempts)
	if !errors.Is(err, sql.ErrNoRows) && err != nil {
		return onboardingState{}, err
	}
	if attempts >= onboardingAttemptLimit {
		return onboardingState{}, errOnboardingLimitReached
	}
	if !o.allowNIMCall(accountID, time.Now()) {
		return onboardingState{}, errOnboardingRateLimited
	}

	transaction, err := o.db.BeginTx(context, nil)
	if err != nil {
		return onboardingState{}, err
	}
	defer transaction.Rollback()
	now := time.Now().Unix()
	if _, err := transaction.ExecContext(
		context,
		`INSERT INTO onboarding_interviews (
			account_id, status, attempt_count, updated_at
		 )
		 VALUES (?, 'in_progress', 1, ?)
		 ON CONFLICT(account_id) DO UPDATE SET
			status = 'in_progress',
			attempt_count = onboarding_interviews.attempt_count + 1,
			updated_at = excluded.updated_at`,
		accountID,
		now,
	); err != nil {
		return onboardingState{}, err
	}
	if !retry {
		if _, err := transaction.ExecContext(
			context,
			`INSERT INTO onboarding_messages (account_id, role, content, created_at)
			 VALUES (?, 'user', ?, ?)`,
			accountID,
			message,
			now,
		); err != nil {
			return onboardingState{}, err
		}
	}
	if err := transaction.Commit(); err != nil {
		return onboardingState{}, err
	}

	messages, err := o.messages(context, accountID)
	if err != nil {
		return onboardingState{}, err
	}
	result, err := o.nim.Respond(context, messages)
	if err != nil {
		if _, updateErr := o.db.ExecContext(
			context,
			`UPDATE onboarding_interviews
			 SET status = 'failed', updated_at = ?
			 WHERE account_id = ?`,
			time.Now().Unix(),
			accountID,
		); updateErr != nil {
			return onboardingState{}, updateErr
		}
		return onboardingState{}, fmt.Errorf("%w: %v", errNIMUnavailable, err)
	}
	if err := o.saveResult(context, accountID, result); err != nil {
		return onboardingState{}, err
	}
	return o.state(context, accountID)
}

func (o *onboardingInterview) saveResult(
	context context.Context,
	accountID int64,
	result nimInterviewResult,
) error {
	transaction, err := o.db.BeginTx(context, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()

	status := onboardingInProgress
	mapped := 0
	if result.Complete {
		for _, preference := range result.Preferences {
			var dishID string
			err := transaction.QueryRowContext(
				context,
				`SELECT source_path
				 FROM catalog_dishes
				 WHERE name = ?
				 ORDER BY source_path
				 LIMIT 1`,
				preference.DishName,
			).Scan(&dishID)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			weight := preference.Weight
			if math.IsNaN(weight) || math.IsInf(weight, 0) {
				continue
			}
			weight = min(maxPreferenceWeight, max(1, weight))
			err = o.pool.Admit(context, transaction, accountID, dishID, weight)
			if errors.Is(err, errDishRejected) {
				continue
			}
			if err != nil {
				return err
			}
			mapped++
		}
		if mapped > 0 {
			status = onboardingCompleted
		} else {
			result.Reply += " 我还没能把这些菜对应到 Catalog，请再告诉我具体菜名。"
		}
	}

	now := time.Now().Unix()
	if _, err := transaction.ExecContext(
		context,
		`INSERT INTO onboarding_messages (account_id, role, content, created_at)
		 VALUES (?, 'assistant', ?, ?)`,
		accountID,
		result.Reply,
		now,
	); err != nil {
		return err
	}
	if _, err := transaction.ExecContext(
		context,
		`UPDATE onboarding_interviews
		 SET status = ?, updated_at = ?
		 WHERE account_id = ?`,
		status,
		now,
		accountID,
	); err != nil {
		return err
	}
	return transaction.Commit()
}

func (o *onboardingInterview) state(
	context context.Context,
	accountID int64,
) (onboardingState, error) {
	state := onboardingState{
		Status:   onboardingNotStarted,
		Messages: []onboardingMessage{},
	}
	var status string
	err := o.db.QueryRowContext(
		context,
		"SELECT status FROM onboarding_interviews WHERE account_id = ?",
		accountID,
	).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		var hasCandidates bool
		if err := o.db.QueryRowContext(
			context,
			"SELECT EXISTS(SELECT 1 FROM candidate_pool WHERE account_id = ?)",
			accountID,
		).Scan(&hasCandidates); err != nil {
			return onboardingState{}, err
		}
		if hasCandidates {
			state.Status = onboardingManual
		}
		return state, nil
	}
	if err != nil {
		return onboardingState{}, err
	}
	state.Status = onboardingStatus(status)
	state.Messages, err = o.messages(context, accountID)
	if err != nil {
		return onboardingState{}, err
	}
	state.CanRetry = state.Status == onboardingFailed ||
		(state.Status == onboardingInProgress &&
			len(state.Messages) > 0 &&
			state.Messages[len(state.Messages)-1].Role == "user")
	return state, nil
}

func (o *onboardingInterview) messages(
	context context.Context,
	accountID int64,
) ([]onboardingMessage, error) {
	rows, err := o.db.QueryContext(
		context,
		`SELECT role, content
		 FROM onboarding_messages
		 WHERE account_id = ?
		 ORDER BY id`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]onboardingMessage, 0)
	for rows.Next() {
		var message onboardingMessage
		if err := rows.Scan(&message.Role, &message.Content); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (o *onboardingInterview) allowNIMCall(accountID int64, now time.Time) bool {
	o.rateMu.Lock()
	defer o.rateMu.Unlock()

	window, exists := o.rate[accountID]
	if !exists || !now.Before(window.expiresAt) {
		o.rate[accountID] = onboardingRateWindow{
			count:     1,
			expiresAt: now.Add(onboardingRatePeriod),
		}
		return true
	}
	if window.count >= onboardingRateLimit {
		return false
	}
	window.count++
	o.rate[accountID] = window
	return true
}

func (o *onboardingInterview) accountLock(accountID int64) *sync.Mutex {
	o.locksMu.Lock()
	defer o.locksMu.Unlock()
	lock := o.locks[accountID]
	if lock == nil {
		lock = &sync.Mutex{}
		o.locks[accountID] = lock
	}
	return lock
}

func (a *App) getOnboardingInterview(context *gin.Context) {
	account := sessionAccount(context)
	state, err := a.onboarding.State(context, account.ID)
	if err != nil {
		writeInternalError(context, "read Onboarding interview", err)
		return
	}
	context.JSON(http.StatusOK, state)
}

func (a *App) sendOnboardingMessage(context *gin.Context) {
	account := sessionAccount(context)
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 4<<10)
	var input onboardingMessageInput
	if err := context.ShouldBindJSON(&input); err != nil {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "消息格式无效")
		return
	}
	input.Message = strings.TrimSpace(input.Message)
	if input.Message == "" || utf8.RuneCountInString(input.Message) > maxOnboardingMessage {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "消息须为 1–600 个字符")
		return
	}
	state, err := a.onboarding.Send(context, account.ID, input.Message)
	if err != nil {
		a.writeOnboardingError(context, account.ID, err)
		return
	}
	context.JSON(http.StatusOK, state)
}

func (a *App) retryOnboardingInterview(context *gin.Context) {
	account := sessionAccount(context)
	state, err := a.onboarding.Retry(context, account.ID)
	if err != nil {
		a.writeOnboardingError(context, account.ID, err)
		return
	}
	context.JSON(http.StatusOK, state)
}

func (a *App) useManualOnboarding(context *gin.Context) {
	account := sessionAccount(context)
	state, err := a.onboarding.Manual(context, account.ID)
	if err != nil {
		a.writeOnboardingError(context, account.ID, err)
		return
	}
	context.JSON(http.StatusOK, state)
}

func (a *App) writeOnboardingError(
	context *gin.Context,
	accountID int64,
	err error,
) {
	switch {
	case errors.Is(err, errNIMUnavailable):
		log.Printf("NIM Onboarding call failed for Account %d: %v", accountID, err)
		writeError(
			context,
			http.StatusServiceUnavailable,
			codeNIMUnavailable,
			"NIM 暂时不可用，请重试或改用手工 Catalog 编辑",
		)
	case errors.Is(err, errOnboardingRateLimited):
		context.Header("Retry-After", strconv.Itoa(int(onboardingRatePeriod.Seconds())))
		writeError(
			context,
			http.StatusTooManyRequests,
			codeRateLimited,
			"访谈消息过于频繁，请稍后再试",
		)
	case errors.Is(err, errOnboardingLimitReached):
		writeError(
			context,
			http.StatusTooManyRequests,
			codeInterviewLimitReached,
			"访谈次数已达上限，请改用手工 Catalog 编辑",
		)
	case errors.Is(err, errOnboardingRetryNeeded):
		writeError(
			context,
			http.StatusConflict,
			codeRetryRequired,
			"上一条消息尚未完成，请先重试",
		)
	case errors.Is(err, errOnboardingRetryInvalid):
		writeError(
			context,
			http.StatusConflict,
			codeRetryUnavailable,
			"当前没有可重试的访谈消息",
		)
	case errors.Is(err, errOnboardingFinished):
		writeError(
			context,
			http.StatusConflict,
			codeInterviewFinished,
			"Onboarding interview 已结束",
		)
	default:
		writeInternalError(context, "update Onboarding interview", err)
	}
}
