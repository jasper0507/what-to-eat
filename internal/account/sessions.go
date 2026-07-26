// Package account 拥有 Account 身份与会话的全部规则：注册、登录（含按 IP
// 的失败限流）、会话签发与校验。cookie 属于 HTTP adapter，留在 server 包；
// clock 只经构造器注入，不进入公共 interface。
package account

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	sqliteDriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	sessionLifetime    = 30 * 24 * time.Hour
	loginFailureLimit  = 5
	loginFailureWindow = time.Minute
)

var (
	ErrUnauthenticated    = errors.New("session is missing or expired")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrLoginRateLimited   = errors.New("login attempts are rate limited")
	ErrUnavailable        = errors.New("Account username is unavailable")
	ErrPasswordUnusable   = errors.New("password cannot be hashed")
)

type Account struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Grant 是一次成功注册或登录的结果：Account 身份加上待写入 cookie 的会话。
type Grant struct {
	Account   Account
	Token     string
	ExpiresAt time.Time
}

type loginFailures struct {
	count     int
	expiresAt time.Time
}

type Sessions struct {
	db                *sql.DB
	secret            []byte
	dummyPasswordHash []byte
	clock             func() time.Time
	failuresMu        sync.Mutex
	failures          map[string]loginFailures
}

func NewSessions(
	db *sql.DB,
	secret []byte,
	clock func() time.Time,
) (*Sessions, error) {
	dummyPasswordHash, err := bcrypt.GenerateFromPassword(
		[]byte("dummy-password"),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return nil, err
	}
	return &Sessions{
		db:                db,
		secret:            append([]byte(nil), secret...),
		dummyPasswordHash: dummyPasswordHash,
		clock:             clock,
		failures:          make(map[string]loginFailures),
	}, nil
}

// Register 创建 Account 并在同一事务内签发首个会话。
func (s *Sessions) Register(
	context context.Context,
	input Credentials,
) (Grant, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return Grant{}, fmt.Errorf("%w: %v", ErrPasswordUnusable, err)
	}

	transaction, err := s.db.BeginTx(context, nil)
	if err != nil {
		return Grant{}, err
	}
	defer transaction.Rollback()

	result, err := transaction.ExecContext(
		context,
		"INSERT INTO accounts (username, username_key, password_hash) VALUES (?, ?, ?)",
		input.Username,
		strings.ToLower(input.Username),
		string(hash),
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return Grant{}, ErrUnavailable
		}
		return Grant{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Grant{}, err
	}

	grant, tokenHash, err := s.newGrant(Account{ID: id, Username: input.Username})
	if err != nil {
		return Grant{}, err
	}
	if _, err := transaction.ExecContext(
		context,
		"INSERT INTO sessions (token_hash, account_id, expires_at) VALUES (?, ?, ?)",
		tokenHash,
		id,
		grant.ExpiresAt.Unix(),
	); err != nil {
		return Grant{}, err
	}
	return grant, transaction.Commit()
}

// Login 验证凭据并签发会话。同一 IP 一分钟内失败五次后被限流。
func (s *Sessions) Login(
	context context.Context,
	input Credentials,
	clientIP string,
) (Grant, error) {
	if s.loginIsBlocked(clientIP) {
		return Grant{}, ErrLoginRateLimited
	}

	var found Account
	passwordHash := s.dummyPasswordHash
	var storedHash string
	err := s.db.QueryRowContext(
		context,
		"SELECT id, username, password_hash FROM accounts WHERE username_key = ?",
		strings.ToLower(input.Username),
	).Scan(&found.ID, &found.Username, &storedHash)
	switch {
	case err == nil:
		passwordHash = []byte(storedHash)
	case errors.Is(err, sql.ErrNoRows):
	default:
		return Grant{}, err
	}

	if err := bcrypt.CompareHashAndPassword(passwordHash, []byte(input.Password)); err != nil || found.ID == 0 {
		s.recordLoginFailure(clientIP)
		return Grant{}, ErrInvalidCredentials
	}

	grant, tokenHash, err := s.newGrant(found)
	if err != nil {
		return Grant{}, err
	}
	if _, err := s.db.ExecContext(
		context,
		"INSERT INTO sessions (token_hash, account_id, expires_at) VALUES (?, ?, ?)",
		tokenHash,
		found.ID,
		grant.ExpiresAt.Unix(),
	); err != nil {
		return Grant{}, err
	}
	return grant, nil
}

// Verify 校验会话 token；未登录或过期返回 ErrUnauthenticated。
func (s *Sessions) Verify(
	context context.Context,
	token string,
) (Account, error) {
	tokenHash := s.hashToken(token)
	var found Account
	err := s.db.QueryRowContext(
		context,
		`SELECT accounts.id, accounts.username
		 FROM sessions
		 JOIN accounts ON accounts.id = sessions.account_id
		 WHERE sessions.token_hash = ? AND sessions.expires_at > ?`,
		tokenHash,
		s.clock().Unix(),
	).Scan(&found.ID, &found.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrUnauthenticated
	}
	if err != nil {
		return Account{}, err
	}
	return found, nil
}

func (s *Sessions) newGrant(owner Account) (Grant, []byte, error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return Grant{}, nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(randomBytes)
	grant := Grant{
		Account:   owner,
		Token:     token,
		ExpiresAt: s.clock().Add(sessionLifetime),
	}
	return grant, s.hashToken(token), nil
}

func (s *Sessions) hashToken(token string) []byte {
	hash := hmac.New(sha256.New, s.secret)
	_, _ = hash.Write([]byte(token))
	return hash.Sum(nil)
}

func (s *Sessions) loginIsBlocked(clientIP string) bool {
	s.failuresMu.Lock()
	defer s.failuresMu.Unlock()

	window, exists := s.failures[clientIP]
	if !exists || !s.clock().Before(window.expiresAt) {
		delete(s.failures, clientIP)
		return false
	}
	return window.count >= loginFailureLimit
}

func (s *Sessions) recordLoginFailure(clientIP string) {
	s.failuresMu.Lock()
	defer s.failuresMu.Unlock()

	window, exists := s.failures[clientIP]
	if !exists || !s.clock().Before(window.expiresAt) {
		window = loginFailures{expiresAt: s.clock().Add(loginFailureWindow)}
	}
	window.count++
	s.failures[clientIP] = window
}

// ValidCredentials 是注册输入的合法性规则：3–32 位字母数字下划线连字符
// 用户名，8 字符以上且不超过 72 字节（bcrypt 上限）的密码。
func ValidCredentials(input Credentials) bool {
	if input.Username != strings.TrimSpace(input.Username) {
		return false
	}
	usernameLength := utf8.RuneCountInString(input.Username)
	if usernameLength < 3 || usernameLength > 32 {
		return false
	}
	for _, character := range input.Username {
		if !unicode.IsLetter(character) && !unicode.IsNumber(character) && character != '_' && character != '-' {
			return false
		}
	}
	passwordBytes := len([]byte(input.Password))
	passwordCharacters := utf8.RuneCountInString(input.Password)
	return passwordCharacters >= 8 && passwordBytes <= 72
}

func isUniqueConstraint(err error) bool {
	var sqliteError *sqliteDriver.Error
	return errors.As(err, &sqliteError) && sqliteError.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}
