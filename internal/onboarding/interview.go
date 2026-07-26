// Package onboarding 拥有 Onboarding interview 模块与其 NVIDIA NIM 私有
// port（ADR-0010/0019）：访谈状态机、按账号限流与串行化、结果落库与
// 初始 Candidate pool 播种。
package onboarding

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/jasper0507/what-to-eat/internal/engine"
	"github.com/jasper0507/what-to-eat/internal/pool"
)

const (
	StatusNotStarted Status = "not_started"
	StatusInProgress Status = "in_progress"
	StatusFailed     Status = "failed"
	StatusCompleted  Status = "completed"
	StatusManual     Status = "manual"

	rateLimit    = 10
	RatePeriod   = time.Minute
	attemptLimit = 30
	// MaxMessageRunes 是单条访谈消息的长度上限（HTTP adapter 用它做输入校验）。
	MaxMessageRunes = 600
)

var (
	ErrNIMUnavailable      = errors.New("NVIDIA NIM is unavailable")
	ErrFinished            = errors.New("Onboarding interview is already finished")
	ErrRetryRequired       = errors.New("Onboarding interview must be retried")
	ErrRetryUnavailable    = errors.New("Onboarding interview cannot be retried")
	ErrRateLimited         = errors.New("Onboarding interview is rate limited")
	ErrAttemptLimitReached = errors.New("Onboarding interview attempt limit reached")
)

type Status string

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type State struct {
	Status   Status    `json:"status"`
	Messages []Message `json:"messages"`
	CanRetry bool      `json:"can_retry"`
}

type rateWindow struct {
	count     int
	expiresAt time.Time
}

type Interview struct {
	db      *sql.DB
	pool    *pool.Pool
	nim     NIM
	locksMu sync.Mutex
	locks   map[int64]*sync.Mutex
	rateMu  sync.Mutex
	rate    map[int64]rateWindow
}

func NewInterview(db *sql.DB, candidates *pool.Pool, nim NIM) *Interview {
	return &Interview{
		db:    db,
		pool:  candidates,
		nim:   nim,
		locks: make(map[int64]*sync.Mutex),
		rate:  make(map[int64]rateWindow),
	}
}

func (o *Interview) State(
	context context.Context,
	accountID int64,
) (State, error) {
	lock := o.accountLock(accountID)
	lock.Lock()
	defer lock.Unlock()
	return o.state(context, accountID)
}

func (o *Interview) Send(
	context context.Context,
	accountID int64,
	message string,
) (State, error) {
	return o.exchange(context, accountID, message, false)
}

func (o *Interview) Retry(
	context context.Context,
	accountID int64,
) (State, error) {
	return o.exchange(context, accountID, "", true)
}

func (o *Interview) Manual(
	context context.Context,
	accountID int64,
) (State, error) {
	lock := o.accountLock(accountID)
	lock.Lock()
	defer lock.Unlock()

	state, err := o.state(context, accountID)
	if err != nil {
		return State{}, err
	}
	if state.Status == StatusCompleted {
		return State{}, ErrFinished
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
		return State{}, err
	}
	return o.state(context, accountID)
}

func (o *Interview) exchange(
	context context.Context,
	accountID int64,
	message string,
	retry bool,
) (State, error) {
	lock := o.accountLock(accountID)
	lock.Lock()
	defer lock.Unlock()

	state, err := o.state(context, accountID)
	if err != nil {
		return State{}, err
	}
	if state.Status == StatusCompleted || state.Status == StatusManual {
		return State{}, ErrFinished
	}
	if retry && !state.CanRetry {
		return State{}, ErrRetryUnavailable
	}
	if !retry && state.CanRetry {
		return State{}, ErrRetryRequired
	}

	var attempts int
	err = o.db.QueryRowContext(
		context,
		"SELECT attempt_count FROM onboarding_interviews WHERE account_id = ?",
		accountID,
	).Scan(&attempts)
	if !errors.Is(err, sql.ErrNoRows) && err != nil {
		return State{}, err
	}
	if attempts >= attemptLimit {
		return State{}, ErrAttemptLimitReached
	}
	if !o.allowNIMCall(accountID, time.Now()) {
		return State{}, ErrRateLimited
	}

	transaction, err := o.db.BeginTx(context, nil)
	if err != nil {
		return State{}, err
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
		return State{}, err
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
			return State{}, err
		}
	}
	if err := transaction.Commit(); err != nil {
		return State{}, err
	}

	messages, err := o.messages(context, accountID)
	if err != nil {
		return State{}, err
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
			return State{}, updateErr
		}
		return State{}, fmt.Errorf("%w: %v", ErrNIMUnavailable, err)
	}
	if err := o.saveResult(context, accountID, result); err != nil {
		return State{}, err
	}
	return o.state(context, accountID)
}

func (o *Interview) saveResult(
	context context.Context,
	accountID int64,
	result NIMResult,
) error {
	transaction, err := o.db.BeginTx(context, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()

	status := StatusInProgress
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
			// 过渡垫片：NIM 仍报连续权重（1–5），折算入池上三档；
			// 第 3 段访谈换脑后由提示词直接产出档位语言
			tier := engine.TierRenShangRen
			switch {
			case weight >= 4.5:
				tier = engine.TierHang
			case weight >= 3.5:
				tier = engine.TierDingJian
			}
			err = o.pool.Admit(context, transaction, accountID, dishID, tier)
			if errors.Is(err, pool.ErrDishRejected) {
				continue
			}
			if err != nil {
				return err
			}
			mapped++
		}
		if mapped > 0 {
			status = StatusCompleted
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

func (o *Interview) state(
	context context.Context,
	accountID int64,
) (State, error) {
	state := State{
		Status:   StatusNotStarted,
		Messages: []Message{},
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
			return State{}, err
		}
		if hasCandidates {
			state.Status = StatusManual
		}
		return state, nil
	}
	if err != nil {
		return State{}, err
	}
	state.Status = Status(status)
	state.Messages, err = o.messages(context, accountID)
	if err != nil {
		return State{}, err
	}
	state.CanRetry = state.Status == StatusFailed ||
		(state.Status == StatusInProgress &&
			len(state.Messages) > 0 &&
			state.Messages[len(state.Messages)-1].Role == "user")
	return state, nil
}

func (o *Interview) messages(
	context context.Context,
	accountID int64,
) ([]Message, error) {
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

	messages := make([]Message, 0)
	for rows.Next() {
		var message Message
		if err := rows.Scan(&message.Role, &message.Content); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (o *Interview) allowNIMCall(accountID int64, now time.Time) bool {
	o.rateMu.Lock()
	defer o.rateMu.Unlock()

	window, exists := o.rate[accountID]
	if !exists || !now.Before(window.expiresAt) {
		o.rate[accountID] = rateWindow{
			count:     1,
			expiresAt: now.Add(RatePeriod),
		}
		return true
	}
	if window.count >= rateLimit {
		return false
	}
	window.count++
	o.rate[accountID] = window
	return true
}

func (o *Interview) accountLock(accountID int64) *sync.Mutex {
	o.locksMu.Lock()
	defer o.locksMu.Unlock()
	lock := o.locks[accountID]
	if lock == nil {
		lock = &sync.Mutex{}
		o.locks[accountID] = lock
	}
	return lock
}
