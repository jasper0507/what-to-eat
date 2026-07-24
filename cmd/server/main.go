package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jasper0507/what-to-eat/internal/server"
)

func main() {
	databasePath := envOrDefault("DATABASE_PATH", "data/what-to-eat.db")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o750); err != nil {
		log.Fatal(err)
	}

	app, err := server.New(server.Config{
		DatabasePath:  databasePath,
		SecureCookies: os.Getenv("APP_ENV") == "production",
		WebDir:        envOrDefault("WEB_DIR", "frontend/dist"),
		CatalogDir:    os.Getenv("CATALOG_DIR"),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()

	httpServer := &http.Server{
		Addr:              ":" + envOrDefault("PORT", "8080"),
		Handler:           app,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       time.Minute,
	}

	log.Printf("What to Eat listening on %s", httpServer.Addr)
	if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
