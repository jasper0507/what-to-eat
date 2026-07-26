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

var ErrDishRejected = errors.New("Dish carries a rejection mark")

type Pool struct {
	db *sql.DB
}

func New(db *sql.DB) *Pool {
	return &Pool{db: db}
}

// Member 是池成员视图：Catalog Dish 加上 Eater 的档位（3 人上人 / 4 顶尖 /
// 5 夯）。数字权重是引擎内部实现，wire 只说档位语言（ADR-0022）。
type Member struct {
	catalog.Dish
	Tier int `json:"tier"`
}

func (p *Pool) List(
	context context.Context,
	accountID int64,
) ([]Member, error) {
	rows, err := p.db.QueryContext(
		context,
		`SELECT catalog_dishes.source_path, catalog_dishes.name, candidate_pool.tier
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
		var tier int
		if err := rows.Scan(&sourcePath, &name, &tier); err != nil {
			return nil, err
		}
		dishes = append(dishes, Member{
			Dish: catalog.NewDish(sourcePath, name),
			Tier: tier,
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
	tier int,
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
		`INSERT INTO candidate_pool (account_id, dish_id, tier)
		 SELECT ?, source_path, ?
		 FROM catalog_dishes
		 WHERE source_path = ?
		 ON CONFLICT(account_id, dish_id) DO NOTHING`,
		accountID,
		tier,
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

func (p *Pool) UpdateTier(
	context context.Context,
	accountID int64,
	dishID string,
	tier int,
) (found bool, err error) {
	result, err := p.db.ExecContext(
		context,
		`UPDATE candidate_pool
		 SET tier = ?
		 WHERE account_id = ? AND dish_id = ?`,
		tier,
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

// Admit 在调用方事务内执行 pool admission。评分派生的档位覆盖已有值：
// Taste rating 是最新的真实进食信号。带 rejection mark 的 Dish 不可接纳。
func (p *Pool) Admit(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
	dishID string,
	tier int,
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
		`INSERT INTO candidate_pool (account_id, dish_id, tier)
		 VALUES (?, ?, ?)
		 ON CONFLICT(account_id, dish_id)
		 DO UPDATE SET tier = excluded.tier`,
		accountID,
		dishID,
		tier,
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
