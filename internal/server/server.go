// Package server 是 HTTP adapter 与 composition root：装配各模块、注册
// 路由、把模块结果与错误翻译成 wire 契约。领域规则住在
// internal/{account,pool,meal,catalog} 各模块包里。
package server

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	mathrand "math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jasper0507/what-to-eat/internal/account"
	"github.com/jasper0507/what-to-eat/internal/catalog"
	"github.com/jasper0507/what-to-eat/internal/meal"
	"github.com/jasper0507/what-to-eat/internal/pool"
	"github.com/jasper0507/what-to-eat/internal/schema"
)

// 模块配置以别名形式出现在公共 Config 上，调用方无需 import 模块包。
type DiscoveryConfig = meal.DiscoveryConfig

func DefaultDiscoveryConfig() DiscoveryConfig {
	return meal.DefaultDiscoveryConfig()
}

type Config struct {
	DatabasePath  string
	SessionSecret []byte
	SecureCookies bool
	WebDir        string
	CatalogDir    string
	Discovery     *DiscoveryConfig
}

// ConfigFromEnv 从环境变量组装 Config，未设置的项使用默认值。
// APP_ENV=production 同时启用 SecureCookies。
func ConfigFromEnv() Config {
	return Config{
		DatabasePath:  envOrDefault("DATABASE_PATH", "data/what-to-eat.db"),
		SessionSecret: []byte(os.Getenv("SESSION_SECRET")),
		SecureCookies: os.Getenv("APP_ENV") == "production",
		WebDir:        envOrDefault("WEB_DIR", "frontend/dist"),
		CatalogDir:    os.Getenv("CATALOG_DIR"),
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// databaseDSN 给库路径带上连接级 PRAGMA：foreign_keys 是每条连接各自的
// 开关，而连接池随时可能重建连接（如查询被取消后驱动丢弃被 interrupt 的
// 连接）——必须让每条新连接自带约束，绝不能依赖启动时执行一次。
func databaseDSN(path string) string {
	return "file:" + path + "?_pragma=foreign_keys(1)"
}

type App struct {
	db            *sql.DB
	router        *gin.Engine
	secureCookies bool
	sessions      *account.Sessions
	pool          *pool.Pool
	catalog       *catalog.Catalog
	meals         *meal.Lifecycle
}

func New(config Config) (*App, error) {
	return newApp(config, meal.NewDecisionRandom())
}

func newApp(config Config, decisionRandom *mathrand.Rand) (*App, error) {
	if config.DatabasePath == "" {
		return nil, errors.New("DatabasePath is required")
	}
	if len(config.SessionSecret) < 32 {
		return nil, errors.New("SessionSecret must contain at least 32 bytes")
	}
	discovery := DefaultDiscoveryConfig()
	if config.Discovery != nil {
		discovery = *config.Discovery
	}
	db, err := sql.Open("sqlite", databaseDSN(config.DatabasePath))
	if err != nil {
		return nil, fmt.Errorf("open SQLite database %q: %w", config.DatabasePath, err)
	}
	db.SetMaxOpenConns(1)

	if err := schema.Migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize SQLite database %q: %w", config.DatabasePath, err)
	}
	if config.CatalogDir != "" {
		if err := catalog.Import(db, config.CatalogDir); err != nil {
			db.Close()
			return nil, fmt.Errorf("import Catalog: %w", err)
		}
	}

	sessions, err := account.NewSessions(db, config.SessionSecret, time.Now)
	if err != nil {
		db.Close()
		return nil, err
	}

	candidates := pool.New(db)
	app := &App{
		db:            db,
		secureCookies: config.SecureCookies,
		sessions:      sessions,
		pool:          candidates,
		catalog:       catalog.New(db),
		meals:         meal.New(db, candidates, decisionRandom, discovery),
	}
	app.routes(config.WebDir, config.CatalogDir)
	return app, nil
}

func (a *App) routes(webDir, catalogDir string) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/api/healthz", func(context *gin.Context) {
		context.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.POST("/api/auth/register", a.register)
	router.POST("/api/auth/login", a.login)
	router.POST("/api/auth/logout", a.logout)
	authorized := router.Group("/api", a.requireAccount)
	authorized.GET("/auth/session", a.session)
	authorized.GET("/catalog/dishes", a.searchCatalog)
	authorized.GET("/catalog/recipes", a.getRecipe)
	authorized.GET("/candidate-pool/dishes", a.listCandidatePool)
	authorized.POST("/candidate-pool/dishes", a.addCandidatePoolDish)
	authorized.PATCH("/candidate-pool/dishes", a.updateCandidatePoolDish)
	authorized.DELETE("/candidate-pool/dishes", a.removeCandidatePoolDish)
	authorized.GET("/meals/resume", a.resumeMeal)
	authorized.POST("/meals", a.beginMeal)
	authorized.POST("/meals/abandon", a.abandonMeal)
	authorized.POST("/meals/hand-pick", a.handPickDish)
	authorized.GET("/eating-records", a.listEatingRecords)
	authorized.POST("/eating-records/:recordID/rate", a.rateEatingRecord)
	authorized.POST("/decisions/:decisionID/reroll", a.rerollDecision)
	authorized.POST("/decisions/:decisionID/accept", a.acceptDecision)
	authorized.POST("/pending-ratings/:pendingRatingID/rate", a.ratePendingRating)
	if catalogDir != "" {
		// 菜谱页图片：CATALOG_DIR 静态挂载，同源、走会话鉴权
		authorized.Static("/catalog/assets", catalogDir)
	}
	if webDir != "" {
		indexPath := filepath.Join(webDir, "index.html")
		router.Static("/assets", filepath.Join(webDir, "assets"))
		router.GET("/", func(context *gin.Context) {
			context.File(indexPath)
		})
		router.NoRoute(func(context *gin.Context) {
			if strings.HasPrefix(context.Request.URL.Path, "/api/") {
				writeError(context, http.StatusNotFound, codeNotFound, "资源不存在")
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

func writeError(context *gin.Context, status int, code, message string) {
	context.JSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

// statusClientClosedRequest 是 nginx 惯例的 499：客户端已离场。
const statusClientClosedRequest = 499

func writeInternalError(context *gin.Context, operation string, err error) {
	// 请求 context 已取消说明客户端离场（前端换页/换 key 的日常 abort），
	// 不是服务端故障：不落错误日志，也不往断开的连接上编 500。
	if context.Request.Context().Err() != nil {
		context.Status(statusClientClosedRequest)
		return
	}
	log.Printf("%s: %v", operation, err)
	writeError(context, http.StatusInternalServerError, codeInternalError, "服务暂时不可用")
}
