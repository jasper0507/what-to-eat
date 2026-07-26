package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	sqliteDriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	sessionCookieName  = "what2eat_session"
	sessionLifetime    = 30 * 24 * time.Hour
	loginFailureLimit  = 5
	loginFailureWindow = time.Minute
)

var (
	errUnauthenticated    = errors.New("session is missing or expired")
	errInvalidCredentials = errors.New("invalid credentials")
	errLoginRateLimited   = errors.New("login attempts are rate limited")
	errAccountUnavailable = errors.New("Account username is unavailable")
	errPasswordUnusable   = errors.New("password cannot be hashed")
)

type accountResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type sessionGrant struct {
	account   accountResponse
	token     string
	expiresAt time.Time
}

type loginFailures struct {
	count     int
	expiresAt time.Time
}

// accountSessions 拥有 Account 身份与会话的全部规则：注册、登录（含按 IP
// 的失败限流）、会话签发与校验。cookie 属于 HTTP adapter，留在 handler 层；
// clock 只经构造器注入，不进入公共 interface。
type accountSessions struct {
	db                *sql.DB
	secret            []byte
	dummyPasswordHash []byte
	clock             func() time.Time
	failuresMu        sync.Mutex
	failures          map[string]loginFailures
}

func newAccountSessions(
	db *sql.DB,
	secret []byte,
	clock func() time.Time,
) (*accountSessions, error) {
	dummyPasswordHash, err := bcrypt.GenerateFromPassword(
		[]byte("dummy-password"),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return nil, err
	}
	return &accountSessions{
		db:                db,
		secret:            append([]byte(nil), secret...),
		dummyPasswordHash: dummyPasswordHash,
		clock:             clock,
		failures:          make(map[string]loginFailures),
	}, nil
}

// Register 创建 Account 并在同一事务内签发首个会话。
func (s *accountSessions) Register(
	context context.Context,
	input credentials,
) (sessionGrant, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return sessionGrant{}, fmt.Errorf("%w: %v", errPasswordUnusable, err)
	}

	transaction, err := s.db.BeginTx(context, nil)
	if err != nil {
		return sessionGrant{}, err
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
			return sessionGrant{}, errAccountUnavailable
		}
		return sessionGrant{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return sessionGrant{}, err
	}

	grant, tokenHash, err := s.newSessionGrant(accountResponse{ID: id, Username: input.Username})
	if err != nil {
		return sessionGrant{}, err
	}
	if _, err := transaction.ExecContext(
		context,
		"INSERT INTO sessions (token_hash, account_id, expires_at) VALUES (?, ?, ?)",
		tokenHash,
		id,
		grant.expiresAt.Unix(),
	); err != nil {
		return sessionGrant{}, err
	}
	return grant, transaction.Commit()
}

// Login 验证凭据并签发会话。同一 IP 一分钟内失败五次后被限流。
func (s *accountSessions) Login(
	context context.Context,
	input credentials,
	clientIP string,
) (sessionGrant, error) {
	if s.loginIsBlocked(clientIP) {
		return sessionGrant{}, errLoginRateLimited
	}

	var account accountResponse
	passwordHash := s.dummyPasswordHash
	var storedHash string
	err := s.db.QueryRowContext(
		context,
		"SELECT id, username, password_hash FROM accounts WHERE username_key = ?",
		strings.ToLower(input.Username),
	).Scan(&account.ID, &account.Username, &storedHash)
	switch {
	case err == nil:
		passwordHash = []byte(storedHash)
	case errors.Is(err, sql.ErrNoRows):
	default:
		return sessionGrant{}, err
	}

	if err := bcrypt.CompareHashAndPassword(passwordHash, []byte(input.Password)); err != nil || account.ID == 0 {
		s.recordLoginFailure(clientIP)
		return sessionGrant{}, errInvalidCredentials
	}

	grant, tokenHash, err := s.newSessionGrant(account)
	if err != nil {
		return sessionGrant{}, err
	}
	if _, err := s.db.ExecContext(
		context,
		"INSERT INTO sessions (token_hash, account_id, expires_at) VALUES (?, ?, ?)",
		tokenHash,
		account.ID,
		grant.expiresAt.Unix(),
	); err != nil {
		return sessionGrant{}, err
	}
	return grant, nil
}

// Verify 校验会话 token；未登录或过期返回 errUnauthenticated。
func (s *accountSessions) Verify(
	context context.Context,
	token string,
) (accountResponse, error) {
	tokenHash := s.hashToken(token)
	var account accountResponse
	err := s.db.QueryRowContext(
		context,
		`SELECT accounts.id, accounts.username
		 FROM sessions
		 JOIN accounts ON accounts.id = sessions.account_id
		 WHERE sessions.token_hash = ? AND sessions.expires_at > ?`,
		tokenHash,
		s.clock().Unix(),
	).Scan(&account.ID, &account.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return accountResponse{}, errUnauthenticated
	}
	if err != nil {
		return accountResponse{}, err
	}
	return account, nil
}

func (s *accountSessions) newSessionGrant(
	account accountResponse,
) (sessionGrant, []byte, error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return sessionGrant{}, nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(randomBytes)
	grant := sessionGrant{
		account:   account,
		token:     token,
		expiresAt: s.clock().Add(sessionLifetime),
	}
	return grant, s.hashToken(token), nil
}

func (s *accountSessions) hashToken(token string) []byte {
	hash := hmac.New(sha256.New, s.secret)
	_, _ = hash.Write([]byte(token))
	return hash.Sum(nil)
}

func (s *accountSessions) loginIsBlocked(clientIP string) bool {
	s.failuresMu.Lock()
	defer s.failuresMu.Unlock()

	window, exists := s.failures[clientIP]
	if !exists || !s.clock().Before(window.expiresAt) {
		delete(s.failures, clientIP)
		return false
	}
	return window.count >= loginFailureLimit
}

func (s *accountSessions) recordLoginFailure(clientIP string) {
	s.failuresMu.Lock()
	defer s.failuresMu.Unlock()

	window, exists := s.failures[clientIP]
	if !exists || !s.clock().Before(window.expiresAt) {
		window = loginFailures{expiresAt: s.clock().Add(loginFailureWindow)}
	}
	window.count++
	s.failures[clientIP] = window
}

const sessionAccountKey = "what2eat_account"

// requireAccount 是 accountSessions 在 HTTP seam 上的 adapter：校验会话并把
// Account 放进请求上下文，之后的 handler 用 sessionAccount 读取。
func (a *App) requireAccount(context *gin.Context) {
	cookie, err := context.Cookie(sessionCookieName)
	if err != nil {
		writeError(context, http.StatusUnauthorized, codeUnauthorized, "需要登录")
		context.Abort()
		return
	}
	account, err := a.sessions.Verify(context, cookie)
	if errors.Is(err, errUnauthenticated) {
		writeError(context, http.StatusUnauthorized, codeUnauthorized, "需要登录")
		context.Abort()
		return
	}
	if err != nil {
		writeInternalError(context, "restore Account session", err)
		context.Abort()
		return
	}
	context.Set(sessionAccountKey, account)
}

func sessionAccount(context *gin.Context) accountResponse {
	account, _ := context.MustGet(sessionAccountKey).(accountResponse)
	return account
}

func (a *App) register(context *gin.Context) {
	var input credentials
	if err := context.ShouldBindJSON(&input); err != nil || !validCredentials(input) {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "用户名或密码不符合要求")
		return
	}

	grant, err := a.sessions.Register(context, input)
	if errors.Is(err, errPasswordUnusable) {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "用户名或密码不符合要求")
		return
	}
	if errors.Is(err, errAccountUnavailable) {
		writeError(context, http.StatusConflict, codeAccountUnavailable, "无法创建 Account")
		return
	}
	if err != nil {
		writeInternalError(context, "register Account", err)
		return
	}

	a.setSessionCookie(context, grant.token, grant.expiresAt)
	context.JSON(http.StatusCreated, gin.H{"account": grant.account})
}

func (a *App) login(context *gin.Context) {
	var input credentials
	if err := context.ShouldBindJSON(&input); err != nil {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "请求格式无效")
		return
	}

	grant, err := a.sessions.Login(context, input, requestIP(context.Request))
	switch {
	case errors.Is(err, errLoginRateLimited):
		writeError(context, http.StatusTooManyRequests, codeRateLimited, "登录尝试过多，请稍后再试")
	case errors.Is(err, errInvalidCredentials):
		writeError(context, http.StatusUnauthorized, codeInvalidCredentials, "用户名或密码错误")
	case err != nil:
		writeInternalError(context, "login Account", err)
	default:
		a.setSessionCookie(context, grant.token, grant.expiresAt)
		context.JSON(http.StatusOK, gin.H{"account": grant.account})
	}
}

func (a *App) session(context *gin.Context) {
	context.JSON(http.StatusOK, gin.H{"account": sessionAccount(context)})
}

func (a *App) setSessionCookie(context *gin.Context, token string, expiresAt time.Time) {
	http.SetCookie(context.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		Secure:   a.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func requestIP(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return request.RemoteAddr
	}
	return host
}

func validCredentials(input credentials) bool {
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
