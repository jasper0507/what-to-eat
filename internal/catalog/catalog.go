// Package catalog 拥有 HowToCook Catalog：source_path 分类学的唯一解析、
// Dish 视图、Recipe 读取、全文导入与搜索。
package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/jasper0507/what-to-eat/internal/engine"
)

// nullableArg 把可空整数翻译为 SQL 参数。
func nullableArg(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

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
// Difficulty/CookMinutes/Calories 是导入富化的信息小字素材，只有走
// 元数据查询的路径（揭示、菜谱页）才会填充。
type Dish struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Difficulty  *int   `json:"difficulty,omitempty"`
	CookMinutes *int   `json:"cook_minutes,omitempty"`
	Calories    *int   `json:"calories,omitempty"`
}

// Recipe 是 Dish 的烹饪说明视图。Images 是导入期收集的图片引用：
// Catalog 相对路径（经静态挂载访问）或外链 URL。
type Recipe struct {
	Dish    Dish     `json:"dish"`
	Content string   `json:"content"`
	Images  []string `json:"images"`
}

// PathCategory 是 source_path 编码规则的唯一出处：首段是类别目录，末段是
// Recipe 文件名。单段路径没有类别。
func PathCategory(sourcePath string) string {
	if category, _, ok := strings.Cut(sourcePath, "/"); ok {
		return category
	}
	return ""
}

func NewDish(sourcePath, name string) Dish {
	dish := Dish{ID: sourcePath, Name: name}
	category := PathCategory(sourcePath)
	if category == "" {
		dish.Category = "其他"
		return dish
	}
	dish.Category = howToCookCategories[category]
	if dish.Category == "" {
		dish.Category = category
	}
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
	var name, content, imagesJSON string
	var difficulty, calories, cookMinutes sql.NullInt64
	err := c.db.QueryRowContext(
		context,
		`SELECT name, recipe, images, difficulty, calories, cook_minutes
		 FROM catalog_dishes
		 WHERE source_path = ?`,
		dishID,
	).Scan(&name, &content, &imagesJSON, &difficulty, &calories, &cookMinutes)
	if errors.Is(err, sql.ErrNoRows) {
		return Recipe{}, ErrRecipeNotFound
	}
	if err != nil {
		return Recipe{}, err
	}
	dish := NewDish(dishID, name)
	dish.Difficulty = nullableInt(difficulty)
	dish.Calories = nullableInt(calories)
	dish.CookMinutes = nullableInt(cookMinutes)
	recipe := Recipe{Dish: dish, Content: content, Images: []string{}}
	if err := json.Unmarshal([]byte(imagesJSON), &recipe.Images); err != nil {
		return Recipe{}, fmt.Errorf("decode images for %s: %w", dishID, err)
	}
	return recipe, nil
}

func nullableInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	number := int(value.Int64)
	return &number
}

// Profiles 返回全部菜的 Taste profile 与展示名，供 Discovery 相似度使用。
func Profiles(
	context context.Context,
	queryer interface {
		QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	},
) (map[string]engine.Profile, map[string]string, error) {
	rows, err := queryer.QueryContext(
		context,
		"SELECT source_path, name, ingredients, flavors, techniques FROM catalog_dishes",
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	profiles := make(map[string]engine.Profile)
	names := make(map[string]string)
	for rows.Next() {
		var sourcePath, name, ingredients, flavors, techniques string
		if err := rows.Scan(&sourcePath, &name, &ingredients, &flavors, &techniques); err != nil {
			return nil, nil, err
		}
		profile := engine.Profile{Category: PathCategory(sourcePath)}
		for _, pair := range []struct {
			raw    string
			target *[]string
		}{
			{ingredients, &profile.Ingredients},
			{flavors, &profile.Flavors},
			{techniques, &profile.Techniques},
		} {
			if err := json.Unmarshal([]byte(pair.raw), pair.target); err != nil {
				return nil, nil, fmt.Errorf("decode profile for %s: %w", sourcePath, err)
			}
		}
		profiles[sourcePath] = profile
		names[sourcePath] = name
	}
	return profiles, names, rows.Err()
}

// Import 把 HowToCook 语料目录导入 catalog_dishes，重复导入按 source_path 覆盖。
func Import(db *sql.DB, root string) error {
	transaction, err := db.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()

	statement, err := transaction.Prepare(`
		INSERT INTO catalog_dishes (
			source_path, name, recipe,
			ingredients, flavors, techniques, images,
			difficulty, calories, cook_minutes
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_path) DO UPDATE SET
			name = excluded.name,
			recipe = excluded.recipe,
			ingredients = excluded.ingredients,
			flavors = excluded.flavors,
			techniques = excluded.techniques,
			images = excluded.images,
			difficulty = excluded.difficulty,
			calories = excluded.calories,
			cook_minutes = excluded.cook_minutes
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
		enrichment := Enrich(relativePath, name, string(recipe))
		encoded := make([]string, 0, 4)
		for _, values := range [][]string{
			enrichment.Profile.Ingredients,
			enrichment.Profile.Flavors,
			enrichment.Profile.Techniques,
			enrichment.Images,
		} {
			buffer, err := json.Marshal(values)
			if err != nil {
				return err
			}
			encoded = append(encoded, string(buffer))
		}
		if _, err := statement.Exec(
			relativePath,
			name,
			string(recipe),
			encoded[0],
			encoded[1],
			encoded[2],
			encoded[3],
			nullableArg(enrichment.Difficulty),
			nullableArg(enrichment.Calories),
			nullableArg(enrichment.CookMinutes),
		); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return transaction.Commit()
}
