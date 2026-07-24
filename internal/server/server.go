package server

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

type Config struct {
	DatabasePath  string
	SecureCookies bool
	WebDir        string
}

type App struct {
	db                *sql.DB
	router            *gin.Engine
	secureCookies     bool
	dummyPasswordHash []byte
	loginFailures     map[string]loginFailureWindow
	loginFailuresMu   sync.Mutex
	webDir            string
}

type loginFailureWindow struct {
	count     int
	expiresAt time.Time
}

type accountResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func New(config Config) (*App, error) {
	db, err := sql.Open("sqlite", config.DatabasePath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`
		PRAGMA foreign_keys = ON;
		CREATE TABLE IF NOT EXISTS accounts (
			id INTEGER PRIMARY KEY,
			username TEXT NOT NULL,
			username_key TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS sessions (
			token_hash BLOB PRIMARY KEY,
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			expires_at INTEGER NOT NULL
		);
	`); err != nil {
		db.Close()
		return nil, err
	}

	dummyPasswordHash, err := bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)
	if err != nil {
		db.Close()
		return nil, err
	}

	app := &App{
		db:                db,
		secureCookies:     config.SecureCookies,
		dummyPasswordHash: dummyPasswordHash,
		loginFailures:     make(map[string]loginFailureWindow),
		webDir:            config.WebDir,
	}
	app.routes()
	return app, nil
}

func (a *App) routes() {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/api/healthz", func(context *gin.Context) {
		context.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.POST("/api/auth/register", a.register)
	router.POST("/api/auth/login", a.login)
	router.GET("/api/auth/session", a.session)
	if configWebDir := a.webDir; configWebDir != "" {
		indexPath := filepath.Join(configWebDir, "index.html")
		router.Static("/assets", filepath.Join(configWebDir, "assets"))
		router.GET("/", func(context *gin.Context) {
			context.File(indexPath)
		})
		router.NoRoute(func(context *gin.Context) {
			if strings.HasPrefix(context.Request.URL.Path, "/api/") {
				writeError(context, http.StatusNotFound, "not_found", "资源不存在")
				return
			}
			context.File(indexPath)
		})
	}
	a.router = router
}

func (a *App) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	a.router.ServeHTTP(writer, request)
}

func (a *App) Close() error {
	return a.db.Close()
}

func (a *App) register(context *gin.Context) {
	var input credentials
	if err := context.ShouldBindJSON(&input); err != nil || !validCredentials(input) {
		writeError(context, http.StatusBadRequest, "invalid_request", "用户名或密码不符合要求")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(context, http.StatusBadRequest, "invalid_request", "用户名或密码不符合要求")
		return
	}

	transaction, err := a.db.BeginTx(context, nil)
	if err != nil {
		writeError(context, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
		return
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
		writeError(context, http.StatusConflict, "account_unavailable", "无法创建 Account")
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		writeError(context, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
		return
	}

	token, tokenHash, expiresAt, err := newSession()
	if err != nil {
		writeError(context, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
		return
	}
	if _, err := transaction.ExecContext(
		context,
		"INSERT INTO sessions (token_hash, account_id, expires_at) VALUES (?, ?, ?)",
		tokenHash,
		id,
		expiresAt.Unix(),
	); err != nil {
		writeError(context, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
		return
	}
	if err := transaction.Commit(); err != nil {
		writeError(context, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
		return
	}

	a.setSessionCookie(context, token, expiresAt)
	context.JSON(http.StatusCreated, gin.H{
		"account": accountResponse{ID: id, Username: input.Username},
	})
}

func (a *App) login(context *gin.Context) {
	var input credentials
	if err := context.ShouldBindJSON(&input); err != nil {
		writeError(context, http.StatusBadRequest, "invalid_request", "请求格式无效")
		return
	}
	clientIP := requestIP(context.Request)
	if a.loginIsBlocked(clientIP, time.Now()) {
		writeError(context, http.StatusTooManyRequests, "rate_limited", "登录尝试过多，请稍后再试")
		return
	}

	var account accountResponse
	passwordHash := a.dummyPasswordHash
	var storedHash string
	err := a.db.QueryRowContext(
		context,
		"SELECT id, username, password_hash FROM accounts WHERE username_key = ?",
		strings.ToLower(input.Username),
	).Scan(&account.ID, &account.Username, &storedHash)
	switch {
	case err == nil:
		passwordHash = []byte(storedHash)
	case errors.Is(err, sql.ErrNoRows):
	default:
		writeError(context, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
		return
	}

	if err := bcrypt.CompareHashAndPassword(passwordHash, []byte(input.Password)); err != nil || account.ID == 0 {
		a.recordLoginFailure(clientIP, time.Now())
		writeError(context, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误")
		return
	}
	a.clearLoginFailures(clientIP)

	token, tokenHash, expiresAt, err := newSession()
	if err != nil {
		writeError(context, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
		return
	}
	if _, err := a.db.ExecContext(
		context,
		"INSERT INTO sessions (token_hash, account_id, expires_at) VALUES (?, ?, ?)",
		tokenHash,
		account.ID,
		expiresAt.Unix(),
	); err != nil {
		writeError(context, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
		return
	}

	a.setSessionCookie(context, token, expiresAt)
	context.JSON(http.StatusOK, gin.H{"account": account})
}

func requestIP(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return request.RemoteAddr
	}
	return host
}

func (a *App) loginIsBlocked(clientIP string, now time.Time) bool {
	a.loginFailuresMu.Lock()
	defer a.loginFailuresMu.Unlock()

	window, exists := a.loginFailures[clientIP]
	if !exists || !now.Before(window.expiresAt) {
		delete(a.loginFailures, clientIP)
		return false
	}
	return window.count >= 5
}

func (a *App) recordLoginFailure(clientIP string, now time.Time) {
	a.loginFailuresMu.Lock()
	defer a.loginFailuresMu.Unlock()

	window, exists := a.loginFailures[clientIP]
	if !exists || !now.Before(window.expiresAt) {
		window = loginFailureWindow{expiresAt: now.Add(time.Minute)}
	}
	window.count++
	a.loginFailures[clientIP] = window
}

func (a *App) clearLoginFailures(clientIP string) {
	a.loginFailuresMu.Lock()
	defer a.loginFailuresMu.Unlock()
	delete(a.loginFailures, clientIP)
}

func (a *App) session(context *gin.Context) {
	cookie, err := context.Cookie("what2eat_session")
	if err != nil {
		writeError(context, http.StatusUnauthorized, "unauthorized", "需要登录")
		return
	}

	tokenHash := sha256.Sum256([]byte(cookie))
	var account accountResponse
	err = a.db.QueryRowContext(
		context,
		`SELECT accounts.id, accounts.username
		 FROM sessions
		 JOIN accounts ON accounts.id = sessions.account_id
		 WHERE sessions.token_hash = ? AND sessions.expires_at > ?`,
		tokenHash[:],
		time.Now().Unix(),
	).Scan(&account.ID, &account.Username)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(context, http.StatusUnauthorized, "unauthorized", "需要登录")
		return
	}
	if err != nil {
		writeError(context, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
		return
	}

	context.JSON(http.StatusOK, gin.H{"account": account})
}

func newSession() (token string, tokenHash []byte, expiresAt time.Time, err error) {
	randomBytes := make([]byte, 32)
	if _, err = rand.Read(randomBytes); err != nil {
		return "", nil, time.Time{}, err
	}
	token = base64.RawURLEncoding.EncodeToString(randomBytes)
	hash := sha256.Sum256([]byte(token))
	return token, hash[:], time.Now().Add(30 * 24 * time.Hour), nil
}

func (a *App) setSessionCookie(context *gin.Context, token string, expiresAt time.Time) {
	http.SetCookie(context.Writer, &http.Cookie{
		Name:     "what2eat_session",
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		Secure:   a.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
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

func writeError(context *gin.Context, status int, code, message string) {
	context.JSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}
