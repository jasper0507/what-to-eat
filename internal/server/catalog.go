package server

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxRecipeBytes = 2 << 20

var howToCookCategories = map[string]string{
	"aquatic":        "水产",
	"breakfast":      "早餐",
	"condiment":      "调味料",
	"dessert":        "甜品",
	"drink":          "饮料",
	"meat_dish":      "荤菜",
	"other":          "其他",
	"semi-finished":  "半成品加工",
	"soup":           "汤与粥",
	"staple":         "主食",
	"vegetable_dish": "素菜",
}

func importCatalog(db *sql.DB, root string) error {
	transaction, err := db.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()

	statement, err := transaction.Prepare(`
		INSERT INTO catalog_dishes (id, source_path, name, category, recipe, tags)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_path) DO UPDATE SET
			name = excluded.name,
			category = excluded.category,
			recipe = excluded.recipe,
			tags = excluded.tags
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
		pathParts := strings.Split(relativePath, "/")
		category := "其他"
		tags := make([]string, 0)
		if len(pathParts) > 1 {
			category = howToCookCategories[pathParts[0]]
			if category == "" {
				category = pathParts[0]
			}
			tags = pathParts[1 : len(pathParts)-1]
		}
		tagsJSON, err := json.Marshal(tags)
		if err != nil {
			return err
		}
		hash := sha256.Sum256([]byte(relativePath))
		id := "howtocook:" + hex.EncodeToString(hash[:])
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if _, err := statement.Exec(id, relativePath, name, category, string(recipe), tagsJSON); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return transaction.Commit()
}
