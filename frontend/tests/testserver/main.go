//go:build testserver

package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jasper0507/what-to-eat/internal/server"
)

func main() {
	// :memory: 库随连接生灭，查询被取消触发连接重建即丢库——用一次性临时文件
	databaseDir, err := os.MkdirTemp("", "what2eat-testserver")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(databaseDir)
	app, err := server.New(server.Config{
		DatabasePath:  filepath.Join(databaseDir, "what-to-eat.db"),
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
