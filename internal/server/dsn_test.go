package server

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDatabaseDSNKeepsForeignKeysAfterConnectionRebuild 复现连接重建链路：
// 查询执行中被取消 → 驱动 sqlite3_interrupt → 连接归还时被判废丢弃 →
// 池子懒开新连接。foreign_keys 是连接级开关，只有 DSN 才能保证每条新
// 连接自带；退化成启动时执行一次的话，这条链路会让全进程约束静默失效。
func TestDatabaseDSNKeepsForeignKeysAfterConnectionRebuild(t *testing.T) {
	db, err := sql.Open("sqlite", databaseDSN(filepath.Join(t.TempDir(), "fk.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`
		CREATE TABLE parents (id INTEGER PRIMARY KEY);
		CREATE TABLE children (
			id INTEGER PRIMARY KEY,
			parent_id INTEGER NOT NULL REFERENCES parents(id)
		);
	`); err != nil {
		t.Fatal(err)
	}
	// 临时表随连接生灭：它消失即证明唯一连接真的被换掉了
	if _, err := db.Exec("CREATE TEMP TABLE probe (x INTEGER)"); err != nil {
		t.Fatal(err)
	}

	rebuilt := false
	for attempt := 0; attempt < 5 && !rebuilt; attempt++ {
		cancelContext, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()
		var count int64
		// 长查询保证取消命中语句执行中；报错是预期，忽略
		_ = db.QueryRowContext(
			cancelContext,
			`WITH RECURSIVE counter(x) AS (
				SELECT 1 UNION ALL SELECT x + 1 FROM counter WHERE x < 100000000
			 )
			 SELECT count(*) FROM counter`,
		).Scan(&count)
		cancel()
		var probe int
		if err := db.QueryRow("SELECT count(*) FROM probe").Scan(&probe); err != nil {
			rebuilt = true
		}
	}
	if !rebuilt {
		t.Fatal("interrupt 未触发连接重建，测试前提未成立")
	}

	var foreignKeys int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("PRAGMA foreign_keys = %d after rebuild, want 1", foreignKeys)
	}
	_, err = db.Exec("INSERT INTO children (parent_id) VALUES (999)")
	if err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("orphan insert error = %v, want FOREIGN KEY constraint failure", err)
	}
}
