package server

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// schemaMigrations 是唯一的、按序执行的 schema 台账：任何一张表长什么样，
// 读本文件即可回答。新迁移追加到列表末尾。
var schemaMigrations = []struct {
	name  string
	apply func(*sql.DB) error
}{
	{"base schema", applyBaseSchema},
	{"legacy Catalog", migrateLegacyCatalogSchema},
	{"Reroll", migrateRerollSchema},
	{"Discovery", migrateDiscoverySchema},
	{"Pending rating", migratePendingRatingSchema},
}

func migrateSchema(db *sql.DB) error {
	for _, migration := range schemaMigrations {
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
		mode TEXT NOT NULL CHECK (mode IN ('pool', 'discovery')),
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
