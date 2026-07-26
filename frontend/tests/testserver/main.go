//go:build testserver

package main

import (
	"log"
	"net/http"
	"os"

	"github.com/jasper0507/what-to-eat/internal/server"
)

func main() {
	app, err := server.New(server.Config{
		DatabasePath:  ":memory:",
		SessionSecret: []byte("browser-test-session-secret-at-least-32-bytes"),
		SecureCookies: true,
		CatalogDir:    "../internal/server/testdata/catalog",
		WebDir:        "dist",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()

	port := os.Getenv("PORT")
	if port == "" {
		port = "4174"
	}
	log.Fatal(http.ListenAndServe(":"+port, app))
}
