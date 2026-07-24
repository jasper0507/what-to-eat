package server

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	mathrand "math/rand"
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
	sqliteDriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type Config struct {
	DatabasePath  string
	SecureCookies bool
	WebDir        string
	CatalogDir    string
	Discovery     *DiscoveryConfig
}

type App struct {
	db                *sql.DB
	router            *gin.Engine
	secureCookies     bool
	dummyPasswordHash []byte
	loginFailures     map[string]loginFailureWindow
	loginFailuresMu   sync.Mutex
	mealLifecycle     *mealLifecycle
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

type catalogDishResponse struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Category   string   `json:"category"`
	RecipePath string   `json:"recipe_path"`
	Tags       []string `json:"tags"`
}

func New(config Config) (*App, error) {
	return newApp(config, newDecisionRandom())
}

func newApp(config Config, decisionRandom *mathrand.Rand) (*App, error) {
	discovery, err := normalizeDiscoveryConfig(config.Discovery)
	if err != nil {
		return nil, fmt.Errorf("configure Discovery: %w", err)
	}
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
		CREATE TABLE IF NOT EXISTS catalog_dishes (
			source_path TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			recipe TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS candidate_pool (
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			dish_id TEXT NOT NULL REFERENCES catalog_dishes(source_path) ON DELETE CASCADE,
			preference_weight REAL NOT NULL CHECK (
				preference_weight >= 0.1 AND preference_weight <= 5
			),
			PRIMARY KEY (account_id, dish_id)
		);
		CREATE TABLE IF NOT EXISTS meals (
			id INTEGER PRIMARY KEY,
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			status TEXT NOT NULL CHECK (status IN ('active', 'accepted')),
			created_at INTEGER NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS one_active_meal_per_account
			ON meals(account_id) WHERE status = 'active';
		CREATE TABLE IF NOT EXISTS decisions (
			id INTEGER PRIMARY KEY,
			meal_id INTEGER NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
			dish_id TEXT NOT NULL REFERENCES catalog_dishes(source_path),
			mode TEXT NOT NULL CHECK (mode IN ('pool', 'discovery')),
			reason TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL CHECK (status IN ('active', 'accepted')),
			rerolled_to_id INTEGER REFERENCES decisions(id),
			created_at INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS eating_records (
			id INTEGER PRIMARY KEY,
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			sequence INTEGER NOT NULL CHECK (sequence > 0),
			meal_id INTEGER NOT NULL UNIQUE REFERENCES meals(id) ON DELETE CASCADE,
			decision_id INTEGER NOT NULL UNIQUE REFERENCES decisions(id),
			dish_id TEXT NOT NULL REFERENCES catalog_dishes(source_path),
			accepted_at INTEGER NOT NULL,
			UNIQUE (account_id, sequence)
		);
	`); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrateLegacyCatalogSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate Catalog: %w", err)
	}
	if err := migrateRerollSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate Reroll: %w", err)
	}
	if err := migrateDiscoverySchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate Discovery: %w", err)
	}
	if config.CatalogDir != "" {
		if err := importCatalog(db, config.CatalogDir); err != nil {
			db.Close()
			return nil, fmt.Errorf("import Catalog: %w", err)
		}
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
		mealLifecycle:     newMealLifecycle(db, decisionRandom, discovery),
	}
	app.routes(config.WebDir)
	return app, nil
}

func (a *App) routes(webDir string) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/api/healthz", func(context *gin.Context) {
		context.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.POST("/api/auth/register", a.register)
	router.POST("/api/auth/login", a.login)
	router.GET("/api/auth/session", a.session)
	router.GET("/api/catalog/dishes", a.searchCatalog)
	router.GET("/api/catalog/recipes", a.getRecipe)
	router.GET("/api/candidate-pool/dishes", a.listCandidatePool)
	router.POST("/api/candidate-pool/dishes", a.addCandidatePoolDish)
	router.PATCH("/api/candidate-pool/dishes", a.updateCandidatePoolDish)
	router.DELETE("/api/candidate-pool/dishes", a.removeCandidatePoolDish)
	router.GET("/api/meals/resume", a.resumeMeal)
	router.POST("/api/meals", a.beginMeal)
	router.POST("/api/decisions/:decisionID/reroll", a.rerollDecision)
	router.POST("/api/decisions/:decisionID/accept", a.acceptDecision)
	if webDir != "" {
		indexPath := filepath.Join(webDir, "index.html")
		router.Static("/assets", filepath.Join(webDir, "assets"))
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
		writeInternalError(context, "begin registration transaction", err)
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
		if isUniqueConstraint(err) {
			writeError(context, http.StatusConflict, "account_unavailable", "无法创建 Account")
		} else {
			writeInternalError(context, "create Account", err)
		}
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		writeInternalError(context, "read created Account ID", err)
		return
	}

	token, tokenHash, expiresAt, err := newSession()
	if err != nil {
		writeInternalError(context, "generate registration session", err)
		return
	}
	if _, err := transaction.ExecContext(
		context,
		"INSERT INTO sessions (token_hash, account_id, expires_at) VALUES (?, ?, ?)",
		tokenHash,
		id,
		expiresAt.Unix(),
	); err != nil {
		writeInternalError(context, "store registration session", err)
		return
	}
	if err := transaction.Commit(); err != nil {
		writeInternalError(context, "commit registration", err)
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
		writeInternalError(context, "find Account for login", err)
		return
	}

	if err := bcrypt.CompareHashAndPassword(passwordHash, []byte(input.Password)); err != nil || account.ID == 0 {
		a.recordLoginFailure(clientIP, time.Now())
		writeError(context, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误")
		return
	}

	token, tokenHash, expiresAt, err := newSession()
	if err != nil {
		writeInternalError(context, "generate login session", err)
		return
	}
	if _, err := a.db.ExecContext(
		context,
		"INSERT INTO sessions (token_hash, account_id, expires_at) VALUES (?, ?, ?)",
		tokenHash,
		account.ID,
		expiresAt.Unix(),
	); err != nil {
		writeInternalError(context, "store login session", err)
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

func (a *App) session(context *gin.Context) {
	account, ok := a.currentAccount(context)
	if !ok {
		return
	}
	context.JSON(http.StatusOK, gin.H{"account": account})
}

func (a *App) currentAccount(context *gin.Context) (accountResponse, bool) {
	cookie, err := context.Cookie("what2eat_session")
	if err != nil {
		writeError(context, http.StatusUnauthorized, "unauthorized", "需要登录")
		return accountResponse{}, false
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
		return accountResponse{}, false
	}
	if err != nil {
		writeInternalError(context, "restore Account session", err)
		return accountResponse{}, false
	}

	return account, true
}

func (a *App) searchCatalog(context *gin.Context) {
	if _, ok := a.currentAccount(context); !ok {
		return
	}
	query := strings.TrimSpace(context.Query("q"))
	if query == "" || utf8.RuneCountInString(query) > 100 {
		writeError(context, http.StatusBadRequest, "invalid_query", "请输入有效的 Dish 名称")
		return
	}

	rows, err := a.db.QueryContext(
		context,
		`SELECT source_path, name
		 FROM catalog_dishes
		 WHERE instr(name, ?) > 0
		 ORDER BY name
		 LIMIT 50`,
		query,
	)
	if err != nil {
		writeInternalError(context, "search Catalog", err)
		return
	}
	defer rows.Close()

	dishes := make([]catalogDishResponse, 0)
	for rows.Next() {
		var sourcePath, name string
		if err := rows.Scan(&sourcePath, &name); err != nil {
			writeInternalError(context, "read Catalog search result", err)
			return
		}
		dishes = append(dishes, catalogDish(sourcePath, name))
	}
	if err := rows.Err(); err != nil {
		writeInternalError(context, "finish Catalog search", err)
		return
	}

	context.JSON(http.StatusOK, gin.H{"dishes": dishes})
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

func writeInternalError(context *gin.Context, operation string, err error) {
	log.Printf("%s: %v", operation, err)
	writeError(context, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
}

func isUniqueConstraint(err error) bool {
	var sqliteError *sqliteDriver.Error
	return errors.As(err, &sqliteError) && sqliteError.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}
