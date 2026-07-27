package schema

import "database/sql"

// Migrate 建立终态 schema：全部 IF NOT EXISTS，重复启动幂等；按旧迁移链
// 走到终态的既有库原样兼容。任何一张表长什么样，读本文件即可回答。
func Migrate(db *sql.DB) error {
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
			recipe TEXT NOT NULL,
			ingredients TEXT NOT NULL DEFAULT '[]',
			flavors TEXT NOT NULL DEFAULT '[]',
			techniques TEXT NOT NULL DEFAULT '[]',
			images TEXT NOT NULL DEFAULT '[]',
			difficulty INTEGER,
			calories INTEGER,
			cook_minutes INTEGER
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
		CREATE TABLE IF NOT EXISTS decisions (
			id INTEGER PRIMARY KEY,
			meal_id INTEGER NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
			dish_id TEXT NOT NULL REFERENCES catalog_dishes(source_path),
			mode TEXT NOT NULL CHECK (mode IN ('pool', 'discovery', 'hand_pick')),
			reason TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL CHECK (status IN ('active', 'accepted')),
			rerolled_to_id INTEGER REFERENCES decisions(id),
			created_at INTEGER NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS one_active_decision_per_meal
			ON decisions(meal_id)
			WHERE status = 'active' AND rerolled_to_id IS NULL;
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
		CREATE TABLE IF NOT EXISTS pool_demotions (
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			dish_id TEXT NOT NULL REFERENCES catalog_dishes(source_path) ON DELETE CASCADE,
			swaps INTEGER NOT NULL DEFAULT 0 CHECK (swaps >= 0),
			PRIMARY KEY (account_id, dish_id)
		);
	`)
	return err
}
