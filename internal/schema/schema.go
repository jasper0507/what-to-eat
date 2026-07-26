package schema

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// migrations 是唯一的、按序执行的 schema 台账。
// 新迁移追加到列表末尾。
var migrations = []struct {
	name  string
	apply func(*sql.DB) error
}{
	{"base schema", applyBaseSchema},
	{"legacy Catalog", migrateLegacyCatalogSchema},
	{"Reroll", migrateRerollSchema},
	{"Discovery", migrateDiscoverySchema},
	{"Pending rating", migratePendingRatingSchema},
	{"Taste tier", migrateTasteTierSchema},
	{"Meal outcomes", migrateMealOutcomesSchema},
	{"Catalog enrichment", migrateCatalogEnrichmentSchema},
	{"Pool demotion", migratePoolDemotionSchema},
	{"Interview retirement", migrateInterviewRetirementSchema},
}

// Migrate 按序执行 schema 台账；任何一张表长什么样，读本文件即可回答。
func Migrate(db *sql.DB) error {
	for _, migration := range migrations {
		if err := migration.apply(db); err != nil {
			return fmt.Errorf("migrate %s: %w", migration.name, err)
		}
	}
	return nil
}

// decisionsTableDDL 是 decisions 表结构的唯一定义。Discovery 迁移重建表时
// 以 decisions_next 为名复用同一份定义。
func decisionsTableDDL(create, table string) string {
	return `CREATE TABLE ` + create + ` (
		id INTEGER PRIMARY KEY,
		meal_id INTEGER NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
		dish_id TEXT NOT NULL REFERENCES catalog_dishes(source_path),
		mode TEXT NOT NULL CHECK (mode IN ('pool', 'discovery', 'hand_pick')),
		reason TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL CHECK (status IN ('active', 'accepted')),
		rerolled_to_id INTEGER REFERENCES ` + table + `(id),
		created_at INTEGER NOT NULL
	);`
}

func applyBaseSchema(db *sql.DB) error {
	_, err := db.Exec(`
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
			tier INTEGER NOT NULL CHECK (tier IN (3, 4, 5)),
			PRIMARY KEY (account_id, dish_id)
		);
		CREATE TABLE IF NOT EXISTS meals (
			id INTEGER PRIMARY KEY,
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			status TEXT NOT NULL CHECK (status IN ('active', 'accepted', 'abandoned')),
			created_at INTEGER NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS one_active_meal_per_account
			ON meals(account_id) WHERE status = 'active';
	` + decisionsTableDDL("IF NOT EXISTS decisions", "decisions") + `
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
	`)
	return err
}

func migrateLegacyCatalogSchema(db *sql.DB) error {
	var legacyColumns int
	if err := db.QueryRow(`
		SELECT count(*)
		FROM pragma_table_info('catalog_dishes')
		WHERE name IN ('id', 'category', 'tags')
	`).Scan(&legacyColumns); err != nil {
		return err
	}
	if legacyColumns == 0 {
		return nil
	}

	transaction, err := db.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`
		CREATE TABLE catalog_dishes_next (
			source_path TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			recipe TEXT NOT NULL
		);
		INSERT INTO catalog_dishes_next (source_path, name, recipe)
			SELECT source_path, name, recipe FROM catalog_dishes;
		DROP TABLE catalog_dishes;
		ALTER TABLE catalog_dishes_next RENAME TO catalog_dishes;
	`); err != nil {
		return err
	}
	return transaction.Commit()
}

func migrateRerollSchema(db *sql.DB) error {
	var hasRerolledToID bool
	if err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM pragma_table_info('decisions') WHERE name = 'rerolled_to_id'
		)
	`).Scan(&hasRerolledToID); err != nil {
		return err
	}
	if !hasRerolledToID {
		if _, err := db.Exec(
			"ALTER TABLE decisions ADD COLUMN rerolled_to_id INTEGER REFERENCES decisions(id)",
		); err != nil {
			return err
		}
	}
	_, err := db.Exec(`
		DROP INDEX IF EXISTS one_active_decision_per_meal;
		CREATE UNIQUE INDEX one_active_decision_per_meal
			ON decisions(meal_id)
			WHERE status = 'active' AND rerolled_to_id IS NULL;
	`)
	return err
}

func migrateDiscoverySchema(db *sql.DB) (err error) {
	var createSQL string
	if err := db.QueryRow(
		"SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = 'decisions'",
	).Scan(&createSQL); err != nil {
		return err
	}
	var hasReason bool
	if err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM pragma_table_info('decisions') WHERE name = 'reason'
		)
	`).Scan(&hasReason); err != nil {
		return err
	}
	if hasReason && strings.Contains(createSQL, "'discovery'") {
		return nil
	}

	if _, err := db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	foreignKeysDisabled := true
	defer func() {
		if foreignKeysDisabled {
			_, restoreErr := db.Exec("PRAGMA foreign_keys = ON")
			err = errors.Join(err, restoreErr)
		}
	}()

	transaction, err := db.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(
		decisionsTableDDL("decisions_next", "decisions_next"),
	); err != nil {
		return err
	}
	if hasReason {
		_, err = transaction.Exec(`
			INSERT INTO decisions_next (
				id, meal_id, dish_id, mode, reason, status, rerolled_to_id, created_at
			)
			SELECT id, meal_id, dish_id, mode, reason, status, rerolled_to_id, created_at
			FROM decisions;
		`)
	} else {
		_, err = transaction.Exec(`
			INSERT INTO decisions_next (
				id, meal_id, dish_id, mode, reason, status, rerolled_to_id, created_at
			)
			SELECT id, meal_id, dish_id, mode, '', status, rerolled_to_id, created_at
			FROM decisions;
		`)
	}
	if err != nil {
		return err
	}
	if _, err := transaction.Exec(`
		DROP TABLE decisions;
		ALTER TABLE decisions_next RENAME TO decisions;
		CREATE UNIQUE INDEX one_active_decision_per_meal
			ON decisions(meal_id)
			WHERE status = 'active' AND rerolled_to_id IS NULL;
	`); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return err
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return err
	}
	foreignKeysDisabled = false
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("Discovery migration left invalid foreign keys")
	}
	return rows.Err()
}

// migrateTasteTierSchema 把 candidate_pool 的连续 preference_weight 重建为
// Taste rating 上三档（3 人上人 / 4 顶尖 / 5 夯）。回填映射覆盖旧评分派生值
// （0.7/1.0/1.3）与手工权重：<0.85→3、<1.15→4、其余→5（ADR-0022）。
func migrateTasteTierSchema(db *sql.DB) error {
	var hasWeight bool
	if err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM pragma_table_info('candidate_pool')
			WHERE name = 'preference_weight'
		)
	`).Scan(&hasWeight); err != nil {
		return err
	}
	if !hasWeight {
		return nil
	}

	transaction, err := db.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`
		CREATE TABLE candidate_pool_next (
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			dish_id TEXT NOT NULL REFERENCES catalog_dishes(source_path) ON DELETE CASCADE,
			tier INTEGER NOT NULL CHECK (tier IN (3, 4, 5)),
			PRIMARY KEY (account_id, dish_id)
		);
		INSERT INTO candidate_pool_next (account_id, dish_id, tier)
			SELECT account_id, dish_id,
			       CASE
				WHEN preference_weight < 0.85 THEN 3
				WHEN preference_weight < 1.15 THEN 4
				ELSE 5
			       END
			FROM candidate_pool;
		DROP TABLE candidate_pool;
		ALTER TABLE candidate_pool_next RENAME TO candidate_pool;
	`); err != nil {
		return err
	}
	return transaction.Commit()
}

// migrateMealOutcomesSchema 扩状态机：meals 增加 abandoned 结局、decisions
// 增加 hand_pick 模式（ADR-0022 的三出口）。两张表需按 SQLite 十二步法重建。
func migrateMealOutcomesSchema(db *sql.DB) (err error) {
	var mealsSQL, decisionsSQL string
	if err := db.QueryRow(
		"SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = 'meals'",
	).Scan(&mealsSQL); err != nil {
		return err
	}
	if err := db.QueryRow(
		"SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = 'decisions'",
	).Scan(&decisionsSQL); err != nil {
		return err
	}
	needMeals := !strings.Contains(mealsSQL, "'abandoned'")
	needDecisions := !strings.Contains(decisionsSQL, "'hand_pick'")
	if !needMeals && !needDecisions {
		return nil
	}

	if _, err := db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	foreignKeysDisabled := true
	defer func() {
		if foreignKeysDisabled {
			_, restoreErr := db.Exec("PRAGMA foreign_keys = ON")
			err = errors.Join(err, restoreErr)
		}
	}()

	transaction, err := db.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if needMeals {
		if _, err := transaction.Exec(`
			CREATE TABLE meals_next (
				id INTEGER PRIMARY KEY,
				account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
				status TEXT NOT NULL CHECK (status IN ('active', 'accepted', 'abandoned')),
				created_at INTEGER NOT NULL
			);
			INSERT INTO meals_next (id, account_id, status, created_at)
				SELECT id, account_id, status, created_at FROM meals;
			DROP TABLE meals;
			ALTER TABLE meals_next RENAME TO meals;
			CREATE UNIQUE INDEX one_active_meal_per_account
				ON meals(account_id) WHERE status = 'active';
		`); err != nil {
			return err
		}
	}
	if needDecisions {
		if _, err := transaction.Exec(
			decisionsTableDDL("decisions_next", "decisions_next"),
		); err != nil {
			return err
		}
		if _, err := transaction.Exec(`
			INSERT INTO decisions_next (
				id, meal_id, dish_id, mode, reason, status, rerolled_to_id, created_at
			)
			SELECT id, meal_id, dish_id, mode, reason, status, rerolled_to_id, created_at
			FROM decisions;
			DROP TABLE decisions;
			ALTER TABLE decisions_next RENAME TO decisions;
			CREATE UNIQUE INDEX one_active_decision_per_meal
				ON decisions(meal_id)
				WHERE status = 'active' AND rerolled_to_id IS NULL;
		`); err != nil {
			return err
		}
	}
	if err := transaction.Commit(); err != nil {
		return err
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return err
	}
	foreignKeysDisabled = false
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("Meal outcomes migration left invalid foreign keys")
	}
	return rows.Err()
}

// migrateCatalogEnrichmentSchema 给 catalog_dishes 追加导入富化列：Taste
// profile 三维（JSON 数组）与揭示/菜谱页要用的元数据（ADR-0022）。
func migrateCatalogEnrichmentSchema(db *sql.DB) error {
	columns := []struct{ name, definition string }{
		{"ingredients", "TEXT NOT NULL DEFAULT '[]'"},
		{"flavors", "TEXT NOT NULL DEFAULT '[]'"},
		{"techniques", "TEXT NOT NULL DEFAULT '[]'"},
		{"images", "TEXT NOT NULL DEFAULT '[]'"},
		{"difficulty", "INTEGER"},
		{"calories", "INTEGER"},
		{"cook_minutes", "INTEGER"},
	}
	for _, column := range columns {
		var exists bool
		if err := db.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM pragma_table_info('catalog_dishes') WHERE name = ?
			)
		`, column.name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.Exec(
			"ALTER TABLE catalog_dishes ADD COLUMN " + column.name + " " + column.definition,
		); err != nil {
			return err
		}
	}
	return nil
}

// migratePoolDemotionSchema 建自动降档的连换计数表（ADR-0022：被换 +1、
// 被接受清零、达 4 降一档并清零）。
// AI 口味访谈已废止（2026-07-27 修正案）：老库里的访谈表整块清退。
func migrateInterviewRetirementSchema(db *sql.DB) error {
	_, err := db.Exec(`
		DROP TABLE IF EXISTS onboarding_messages;
		DROP TABLE IF EXISTS onboarding_interviews;
	`)
	return err
}

func migratePoolDemotionSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS pool_demotions (
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			dish_id TEXT NOT NULL REFERENCES catalog_dishes(source_path) ON DELETE CASCADE,
			swaps INTEGER NOT NULL DEFAULT 0 CHECK (swaps >= 0),
			PRIMARY KEY (account_id, dish_id)
		);
	`)
	return err
}

func migratePendingRatingSchema(db *sql.DB) error {
	transaction, err := db.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`
		CREATE TABLE IF NOT EXISTS pending_ratings (
			id INTEGER PRIMARY KEY,
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			meal_id INTEGER NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
			decision_id INTEGER NOT NULL UNIQUE REFERENCES decisions(id) ON DELETE CASCADE,
			dish_id TEXT NOT NULL REFERENCES catalog_dishes(source_path),
			meal_at INTEGER NOT NULL,
			rating INTEGER CHECK (rating BETWEEN 1 AND 5),
			resolved_at INTEGER,
			CHECK (
				(rating IS NULL AND resolved_at IS NULL) OR
				(rating IS NOT NULL AND resolved_at IS NOT NULL)
			)
		);
		CREATE INDEX IF NOT EXISTS pending_ratings_account_unresolved
			ON pending_ratings(account_id, meal_at, id)
			WHERE rating IS NULL;
		CREATE UNIQUE INDEX IF NOT EXISTS pending_ratings_account_dish_unresolved
			ON pending_ratings(account_id, dish_id)
			WHERE rating IS NULL;
		CREATE TABLE IF NOT EXISTS rejection_marks (
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			dish_id TEXT NOT NULL REFERENCES catalog_dishes(source_path) ON DELETE CASCADE,
			rating INTEGER NOT NULL CHECK (rating IN (1, 2)),
			created_at INTEGER NOT NULL,
			PRIMARY KEY (account_id, dish_id)
		);
	`); err != nil {
		return err
	}
	return transaction.Commit()
}
