//go:build testserver

package main

import (
	"log"
	"net/http"
	"os"

	"github.com/jasper0507/what-to-eat/internal/server"
)

func main() {
	app, err := server.NewWithScriptedNIMRulesForTest(server.Config{
		DatabasePath:  ":memory:",
		SessionSecret: []byte("browser-test-session-secret-at-least-32-bytes"),
		SecureCookies: true,
		CatalogDir:    "../internal/server/testdata/catalog",
		WebDir:        "dist",
	}, []server.ScriptedNIMRuleForTest{
		{
			Contains: "运行完整旅程",
			Reply:    "已经按你的偏好建立 Candidate pool。",
			Complete: true,
			Preferences: map[string]float64{
				"番茄炒蛋": 5,
				"番茄牛腩": 4,
			},
		},
		{
			Contains:    "继续完成恢复测试",
			Reply:       "进度恢复成功，已经建立 Candidate pool。",
			Complete:    true,
			Preferences: map[string]float64{"番茄牛腩": 4.5},
		},
		{
			Contains: "浏览器恢复",
			Reply:    "番茄牛腩记下了，再确认一下它有多喜欢？",
		},
		{
			Contains: "浏览器失败",
			Error:    "scripted outage",
		},
		{
			Contains:    "浏览器成功",
			Reply:       "已经按你的偏好建立 Candidate pool。",
			Complete:    true,
			Preferences: map[string]float64{"番茄炒蛋": 5},
		},
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
