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
	config := server.ConfigFromEnv()
	if err := os.MkdirAll(filepath.Dir(config.DatabasePath), 0o750); err != nil {
		log.Fatal(err)
	}

	app, err := server.New(config)
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	httpServer := &http.Server{
		Addr:              ":" + port,
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
