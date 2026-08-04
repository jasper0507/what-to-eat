package pool_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jasper0507/what-to-eat/internal/engine"
	"github.com/jasper0507/what-to-eat/internal/pool"
	"github.com/jasper0507/what-to-eat/internal/schema"

	_ "modernc.org/sqlite"
)

func openPoolDB(t *testing.T) (*sql.DB, *pool.Pool, int64, string) {
	t.Helper()
	db, err := sql.Open(
		"sqlite",
		"file:"+filepath.Join(t.TempDir(), "pool.db")+"?_pragma=foreign_keys(1)",
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := schema.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO accounts (username, username_key, password_hash) VALUES ('a', 'a', 'x')`,
	); err != nil {
		t.Fatal(err)
	}
	var accountID int64
	if err := db.QueryRow(`SELECT id FROM accounts`).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	dishID := "meat_dish/宫保鸡丁.md"
	if _, err := db.Exec(
		`INSERT INTO catalog_dishes (source_path, name, recipe) VALUES (?, '宫保鸡丁', '#')`,
		dishID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO candidate_pool (account_id, dish_id, tier) VALUES (?, ?, ?)`,
		accountID,
		dishID,
		engine.TierHang,
	); err != nil {
		t.Fatal(err)
	}
	return db, pool.New(db), accountID, dishID
}

func tierOf(t *testing.T, db *sql.DB, accountID int64, dishID string) int {
	t.Helper()
	var tier int
	err := db.QueryRow(
		`SELECT tier FROM candidate_pool WHERE account_id = ? AND dish_id = ?`,
		accountID,
		dishID,
	).Scan(&tier)
	if err != nil {
		t.Fatal(err)
	}
	return tier
}

func swapsOf(t *testing.T, db *sql.DB, accountID int64, dishID string) int {
	t.Helper()
	var swaps int
	err := db.QueryRow(
		`SELECT swaps FROM pool_demotions WHERE account_id = ? AND dish_id = ?`,
		accountID,
		dishID,
	).Scan(&swaps)
	if err == sql.ErrNoRows {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return swaps
}

func TestRecordSwapDemotesAtThresholdAndFloorsAtRenShangRen(t *testing.T) {
	db, candidates, accountID, dishID := openPoolDB(t)
	ctx := context.Background()

	for i := 0; i < engine.DemotionSwapThreshold-1; i++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := candidates.RecordSwap(ctx, tx, accountID, dishID); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	if got := tierOf(t, db, accountID, dishID); got != engine.TierHang {
		t.Fatalf("tier after %d swaps = %d, want still 夯", engine.DemotionSwapThreshold-1, got)
	}
	if got := swapsOf(t, db, accountID, dishID); got != engine.DemotionSwapThreshold-1 {
		t.Fatalf("swaps = %d, want %d", got, engine.DemotionSwapThreshold-1)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := candidates.RecordSwap(ctx, tx, accountID, dishID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := tierOf(t, db, accountID, dishID); got != engine.TierDingJian {
		t.Fatalf("tier after demotion = %d, want 顶尖", got)
	}
	if got := swapsOf(t, db, accountID, dishID); got != 0 {
		t.Fatalf("swaps after demotion = %d, want 0", got)
	}

	// 连降到人上人地板：从顶尖再降两次 → 人上人，再降仍停在人上人。
	for range 6 {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := candidates.RecordSwap(ctx, tx, accountID, dishID); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	if got := tierOf(t, db, accountID, dishID); got != engine.TierRenShangRen {
		t.Fatalf("tier floor = %d, want 人上人", got)
	}
}

func TestRecordSwapWhenDishLeftPoolStillCounts(t *testing.T) {
	db, candidates, accountID, dishID := openPoolDB(t)
	ctx := context.Background()
	if _, err := db.Exec(
		`DELETE FROM candidate_pool WHERE account_id = ? AND dish_id = ?`,
		accountID,
		dishID,
	); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := candidates.RecordSwap(ctx, tx, accountID, dishID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := swapsOf(t, db, accountID, dishID); got != 1 {
		t.Fatalf("swaps when not in pool = %d, want 1", got)
	}
}

func TestResetSwapsClearsDemotionLedger(t *testing.T) {
	db, candidates, accountID, dishID := openPoolDB(t)
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := candidates.RecordSwap(ctx, tx, accountID, dishID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := candidates.ResetSwaps(ctx, tx, accountID, dishID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := swapsOf(t, db, accountID, dishID); got != 0 {
		t.Fatalf("swaps after ResetSwaps = %d, want 0", got)
	}
}
