package server

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	mathrand "math/rand"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

type Config struct {
	DatabasePath  string
	SessionSecret []byte
	SecureCookies bool
	WebDir        string
	CatalogDir    string
	Discovery     *DiscoveryConfig
	NIM           *NIMConfig
}

type App struct {
	db            *sql.DB
	router        *gin.Engine
	secureCookies bool
	sessions      *accountSessions
	candidatePool *candidatePool
	mealLifecycle *mealLifecycle
	onboarding    *onboardingInterview
}

type catalogDishResponse struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Category   string   `json:"category"`
	RecipePath string   `json:"recipe_path"`
	Tags       []string `json:"tags"`
}

func New(config Config) (*App, error) {
	nim, err := newNIMAdapter(config.NIM)
	if err != nil {
		return nil, fmt.Errorf("configure NVIDIA NIM: %w", err)
	}
	return newApp(config, newDecisionRandom(), nim)
}

func newApp(
	config Config,
	decisionRandom *mathrand.Rand,
	nim onboardingNIM,
) (*App, error) {
	if config.DatabasePath == "" {
		return nil, errors.New("DatabasePath is required")
	}
	if len(config.SessionSecret) < 32 {
		return nil, errors.New("SessionSecret must contain at least 32 bytes")
	}
	discovery, err := normalizeDiscoveryConfig(config.Discovery)
	if err != nil {
		return nil, fmt.Errorf("configure Discovery: %w", err)
	}
	db, err := sql.Open("sqlite", config.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database %q: %w", config.DatabasePath, err)
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
		CREATE TABLE IF NOT EXISTS onboarding_interviews (
			account_id INTEGER PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
			status TEXT NOT NULL CHECK (
				status IN ('in_progress', 'failed', 'completed', 'manual')
			),
			attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
			updated_at INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS onboarding_messages (
			id INTEGER PRIMARY KEY,
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
			content TEXT NOT NULL,
			created_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS onboarding_messages_by_account
			ON onboarding_messages(account_id, id);
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
		return nil, fmt.Errorf("initialize SQLite database %q: %w", config.DatabasePath, err)
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
	if err := migratePendingRatingSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate Pending rating: %w", err)
	}
	if config.CatalogDir != "" {
		if err := importCatalog(db, config.CatalogDir); err != nil {
			db.Close()
			return nil, fmt.Errorf("import Catalog: %w", err)
		}
	}

	sessions, err := newAccountSessions(db, config.SessionSecret, time.Now)
	if err != nil {
		db.Close()
		return nil, err
	}

	pool := newCandidatePool(db)
	app := &App{
		db:            db,
		secureCookies: config.SecureCookies,
		sessions:      sessions,
		candidatePool: pool,
		mealLifecycle: newMealLifecycle(db, pool, decisionRandom, discovery),
		onboarding:    newOnboardingInterview(db, pool, nim),
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
	authorized := router.Group("/api", a.requireAccount)
	authorized.GET("/auth/session", a.session)
	authorized.GET("/catalog/dishes", a.searchCatalog)
	authorized.GET("/catalog/recipes", a.getRecipe)
	authorized.GET("/candidate-pool/dishes", a.listCandidatePool)
	authorized.POST("/candidate-pool/dishes", a.addCandidatePoolDish)
	authorized.PATCH("/candidate-pool/dishes", a.updateCandidatePoolDish)
	authorized.DELETE("/candidate-pool/dishes", a.removeCandidatePoolDish)
	authorized.GET("/onboarding/interview", a.getOnboardingInterview)
	authorized.POST("/onboarding/interview/messages", a.sendOnboardingMessage)
	authorized.POST("/onboarding/interview/retry", a.retryOnboardingInterview)
	authorized.POST("/onboarding/interview/manual", a.useManualOnboarding)
	authorized.GET("/meals/resume", a.resumeMeal)
	authorized.POST("/meals", a.beginMeal)
	authorized.POST("/decisions/:decisionID/reroll", a.rerollDecision)
	authorized.POST("/decisions/:decisionID/accept", a.acceptDecision)
	authorized.POST("/pending-ratings/:pendingRatingID/rate", a.ratePendingRating)
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

func (a *App) searchCatalog(context *gin.Context) {
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
