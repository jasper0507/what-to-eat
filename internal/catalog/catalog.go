// Package catalog 拥有 HowToCook Catalog：source_path 分类学的唯一解析、
// Dish 视图、Recipe 读取、全文导入与搜索。
package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxRecipeBytes = 2 << 20

var ErrRecipeNotFound = errors.New("Recipe not found")

var howToCookCategories = map[string]string{
	"aquatic":        "水产",
	"breakfast":      "早餐",
	"condiment":      "调味料",
	"dessert":        "甜品",
	"drink":          "饮料",
	"meat_dish":      "荤菜",
	"semi-finished":  "半成品加工",
	"soup":           "汤与粥",
	"staple":         "主食",
	"vegetable_dish": "素菜",
}

// Dish 是 Catalog 对外的 Dish 视图，也是各模块结果里复用的领域标识。
type Dish struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Category   string   `json:"category"`
	RecipePath string   `json:"recipe_path"`
	Tags       []string `json:"tags"`
}

// Recipe 是 Dish 的烹饪说明视图。
type Recipe struct {
	Dish    Dish   `json:"dish"`
	Content string `json:"content"`
}

// Taxonomy 是 source_path 编码规则的唯一出处：首段是类别目录，末段是
// Recipe 文件名，中间各段是标签。单段路径没有类别与标签。
type Taxonomy struct {
	Category string
	Tags     []string
}

func PathTaxonomy(sourcePath string) Taxonomy {
	parts := strings.Split(sourcePath, "/")
	if len(parts) < 2 {
		return Taxonomy{}
	}
	return Taxonomy{Category: parts[0], Tags: parts[1 : len(parts)-1]}
}

func NewDish(sourcePath, name string) Dish {
	dish := Dish{
		ID:         sourcePath,
		Name:       name,
		RecipePath: sourcePath,
		Tags:       []string{},
	}
	taxonomy := PathTaxonomy(sourcePath)
	if taxonomy.Category == "" {
		dish.Category = "其他"
		return dish
	}
	dish.Category = howToCookCategories[taxonomy.Category]
	if dish.Category == "" {
		dish.Category = taxonomy.Category
	}
	dish.Tags = taxonomy.Tags
	return dish
}

func ValidDishID(dishID string) bool {
	return dishID != "" && dishID == strings.TrimSpace(dishID) && len(dishID) <= 500
}

// Catalog 提供已导入语料上的搜索与 Recipe 读取。
type Catalog struct {
	db *sql.DB
}

func New(db *sql.DB) *Catalog {
	return &Catalog{db: db}
}

func (c *Catalog) Search(context context.Context, query string) ([]Dish, error) {
	rows, err := c.db.QueryContext(
		context,
		`SELECT source_path, name
		 FROM catalog_dishes
		 WHERE instr(name, ?) > 0
		 ORDER BY name
		 LIMIT 50`,
		query,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dishes := make([]Dish, 0)
	for rows.Next() {
		var sourcePath, name string
		if err := rows.Scan(&sourcePath, &name); err != nil {
			return nil, err
		}
		dishes = append(dishes, NewDish(sourcePath, name))
	}
	return dishes, rows.Err()
}

func (c *Catalog) Recipe(context context.Context, dishID string) (Recipe, error) {
	var name, content string
	err := c.db.QueryRowContext(
		context,
		"SELECT name, recipe FROM catalog_dishes WHERE source_path = ?",
		dishID,
	).Scan(&name, &content)
	if errors.Is(err, sql.ErrNoRows) {
		return Recipe{}, ErrRecipeNotFound
	}
	if err != nil {
		return Recipe{}, err
	}
	return Recipe{Dish: NewDish(dishID, name), Content: content}, nil
}

// Import 把 HowToCook 语料目录导入 catalog_dishes，重复导入按 source_path 覆盖。
func Import(db *sql.DB, root string) error {
	transaction, err := db.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()

	statement, err := transaction.Prepare(`
		INSERT INTO catalog_dishes (source_path, name, recipe)
		VALUES (?, ?, ?)
		ON CONFLICT(source_path) DO UPDATE SET
			name = excluded.name,
			recipe = excluded.recipe
	`)
	if err != nil {
		return err
	}
	defer statement.Close()

	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && entry.Name() == "template" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 ||
			!strings.EqualFold(filepath.Ext(entry.Name()), ".md") ||
			strings.EqualFold(entry.Name(), "README.md") {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > maxRecipeBytes {
			return fmt.Errorf("%s is not a valid Recipe file", path)
		}
		recipe, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if len(recipe) == 0 || !utf8.Valid(recipe) {
			return fmt.Errorf("%s is not valid UTF-8 Recipe content", path)
		}

		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relativePath = filepath.ToSlash(relativePath)
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if _, err := statement.Exec(relativePath, name, string(recipe)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return transaction.Commit()
}
