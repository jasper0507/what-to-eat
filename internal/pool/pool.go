// Package pool 拥有 Candidate pool 的 membership 与 Preference weight
// 不变量，以及 rejection mark 的全部写侧语义。候选选择
// （cooldown/recency/加权抽样）依 ADR-0019 留在 meal.Lifecycle。
package pool

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jasper0507/what-to-eat/internal/catalog"
)

// Preference weight 的合法域，唯一来源；schema CHECK 仅作存储层兜底。
const (
	MinPreferenceWeight = 0.1
	MaxPreferenceWeight = 5
)

var ErrDishRejected = errors.New("Dish carries a rejection mark")

func ValidWeight(weight float64) bool {
	return weight >= MinPreferenceWeight && weight <= MaxPreferenceWeight
}

type Pool struct {
	db *sql.DB
}

func New(db *sql.DB) *Pool {
	return &Pool{db: db}
}

// Member 是池成员视图：Catalog Dish 加上 Eater 的 Preference weight。
type Member struct {
	catalog.Dish
	PreferenceWeight float64 `json:"preference_weight"`
}

func (p *Pool) List(
	context context.Context,
	accountID int64,
) ([]Member, error) {
	rows, err := p.db.QueryContext(
		context,
		`SELECT catalog_dishes.source_path, catalog_dishes.name, candidate_pool.preference_weight
		 FROM candidate_pool
		 JOIN catalog_dishes ON catalog_dishes.source_path = candidate_pool.dish_id
		 WHERE candidate_pool.account_id = ?
		 ORDER BY catalog_dishes.name`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dishes := make([]Member, 0)
	for rows.Next() {
		var sourcePath, name string
		var weight float64
		if err := rows.Scan(&sourcePath, &name, &weight); err != nil {
			return nil, err
		}
		dishes = append(dishes, Member{
			Dish:             catalog.NewDish(sourcePath, name),
			PreferenceWeight: weight,
		})
	}
	return dishes, rows.Err()
}

// Add 把 Catalog 中的 Dish 加入 Candidate pool。显式添加同时清除该 Dish 的
// rejection mark：Eater 主动加回被拒的菜即视为撤销拒绝。只有当既没有插入
// 池成员也没有清除标记（Dish 已在池中且未被拒，或不在 Catalog 中）时才返回
// added=false。
func (p *Pool) Add(
	context context.Context,
	accountID int64,
	dishID string,
	weight float64,
) (added bool, err error) {
	transaction, err := p.db.BeginTx(context, nil)
	if err != nil {
		return false, err
	}
	defer transaction.Rollback()

	clearedResult, err := transaction.ExecContext(
		context,
		"DELETE FROM rejection_marks WHERE account_id = ? AND dish_id = ?",
		accountID,
		dishID,
	)
	if err != nil {
		return false, err
	}
	cleared, err := clearedResult.RowsAffected()
	if err != nil {
		return false, err
	}

	insertResult, err := transaction.ExecContext(
		context,
		`INSERT INTO candidate_pool (account_id, dish_id, preference_weight)
		 SELECT ?, source_path, ?
		 FROM catalog_dishes
		 WHERE source_path = ?
		 ON CONFLICT(account_id, dish_id) DO NOTHING`,
		accountID,
		weight,
		dishID,
	)
	if err != nil {
		return false, err
	}
	inserted, err := insertResult.RowsAffected()
	if err != nil {
		return false, err
	}
	if inserted == 0 && cleared == 0 {
		return false, nil
	}
	return true, transaction.Commit()
}

func (p *Pool) UpdateWeight(
	context context.Context,
	accountID int64,
	dishID string,
	weight float64,
) (found bool, err error) {
	result, err := p.db.ExecContext(
		context,
		`UPDATE candidate_pool
		 SET preference_weight = ?
		 WHERE account_id = ? AND dish_id = ?`,
		weight,
		accountID,
		dishID,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func (p *Pool) Remove(
	context context.Context,
	accountID int64,
	dishID string,
) (found bool, err error) {
	result, err := p.db.ExecContext(
		context,
		"DELETE FROM candidate_pool WHERE account_id = ? AND dish_id = ?",
		accountID,
		dishID,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

// Admit 在调用方事务内执行 pool admission。评分派生的 weight 覆盖已有值：
// Taste rating 是最新的真实进食信号。带 rejection mark 的 Dish 不可接纳。
func (p *Pool) Admit(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
	dishID string,
	weight float64,
) error {
	var rejected bool
	if err := transaction.QueryRowContext(
		context,
		`SELECT EXISTS(
			SELECT 1
			FROM rejection_marks
			WHERE account_id = ? AND dish_id = ?
		 )`,
		accountID,
		dishID,
	).Scan(&rejected); err != nil {
		return err
	}
	if rejected {
		return ErrDishRejected
	}
	_, err := transaction.ExecContext(
		context,
		`INSERT INTO candidate_pool (account_id, dish_id, preference_weight)
		 VALUES (?, ?, ?)
		 ON CONFLICT(account_id, dish_id)
		 DO UPDATE SET preference_weight = excluded.preference_weight`,
		accountID,
		dishID,
		weight,
	)
	return err
}

// Reject 在调用方事务内执行拒绝：移出 Candidate pool 并落 rejection mark。
func (p *Pool) Reject(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
	dishID string,
	rating int,
) error {
	if _, err := transaction.ExecContext(
		context,
		"DELETE FROM candidate_pool WHERE account_id = ? AND dish_id = ?",
		accountID,
		dishID,
	); err != nil {
		return err
	}
	_, err := transaction.ExecContext(
		context,
		`INSERT INTO rejection_marks (account_id, dish_id, rating, created_at)
		 VALUES (?, ?, ?, unixepoch())
		 ON CONFLICT(account_id, dish_id)
		 DO UPDATE SET rating = excluded.rating, created_at = excluded.created_at`,
		accountID,
		dishID,
		rating,
	)
	return err
}
