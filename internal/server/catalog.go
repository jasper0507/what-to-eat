package server

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const maxRecipeBytes = 2 << 20

type recipeResponse struct {
	Dish    catalogDishResponse `json:"dish"`
	Content string              `json:"content"`
}

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

func catalogDish(sourcePath, name string) catalogDishResponse {
	dish := catalogDishResponse{
		ID:         sourcePath,
		Name:       name,
		RecipePath: sourcePath,
		Tags:       []string{},
	}
	pathParts := strings.Split(sourcePath, "/")
	if len(pathParts) == 1 {
		dish.Category = "其他"
		return dish
	}
	dish.Category = howToCookCategories[pathParts[0]]
	if dish.Category == "" {
		dish.Category = pathParts[0]
	}
	dish.Tags = pathParts[1 : len(pathParts)-1]
	return dish
}

func (a *App) getRecipe(context *gin.Context) {
	dishID := context.Query("dish_id")
	if !validDishID(dishID) {
		writeError(context, http.StatusBadRequest, "invalid_request", "Dish 无效")
		return
	}

	var name, content string
	err := a.db.QueryRowContext(
		context,
		"SELECT name, recipe FROM catalog_dishes WHERE source_path = ?",
		dishID,
	).Scan(&name, &content)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(context, http.StatusNotFound, "recipe_not_found", "Recipe 不存在")
		return
	}
	if err != nil {
		writeInternalError(context, "read Recipe", err)
		return
	}
	context.JSON(http.StatusOK, recipeResponse{
		Dish:    catalogDish(dishID, name),
		Content: content,
	})
}


func importCatalog(db *sql.DB, root string) error {
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
