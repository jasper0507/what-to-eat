package server

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/jasper0507/what-to-eat/internal/catalog"
)

// git LFS pointer 文件头（HowToCook 未 smudge 时 *.jpg 等实际是这段 ASCII）。
const gitLFSPointerPrefix = "version https://git-lfs.github.com/spec/v1"

// serveCatalogAsset 从 CATALOG_DIR 提供菜谱图。路径与旧 /catalog/assets 分离，
// 避免边缘缓存住历史 LFS pointer 响应；private 缓存，避免 CDN 缓存鉴权资源。
func serveCatalogAsset(catalogDir string) gin.HandlerFunc {
	root := filepath.Clean(catalogDir)
	return func(context *gin.Context) {
		relative := strings.TrimPrefix(context.Param("filepath"), "/")
		if relative == "" || strings.Contains(relative, "..") {
			writeError(context, http.StatusNotFound, codeNotFound, "资源不存在")
			return
		}
		fullPath := filepath.Clean(filepath.Join(root, filepath.FromSlash(relative)))
		if fullPath != root && !strings.HasPrefix(fullPath, root+string(os.PathSeparator)) {
			writeError(context, http.StatusNotFound, codeNotFound, "资源不存在")
			return
		}
		file, err := os.Open(fullPath)
		if err != nil {
			writeError(context, http.StatusNotFound, codeNotFound, "资源不存在")
			return
		}
		defer file.Close()

		info, err := file.Stat()
		if err != nil || info.IsDir() {
			writeError(context, http.StatusNotFound, codeNotFound, "资源不存在")
			return
		}
		// 拒绝把 LFS pointer 当图片吐出（浏览器解不出，前端 onError 后表现为「无图」）
		head := make([]byte, len(gitLFSPointerPrefix))
		n, _ := io.ReadFull(file, head)
		if n == len(gitLFSPointerPrefix) && string(head) == gitLFSPointerPrefix {
			writeError(context, http.StatusNotFound, codeNotFound, "资源不存在")
			return
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			writeInternalError(context, "seek catalog asset", err)
			return
		}

		// private：同源鉴权资源不进共享 CDN，避免错误 body 被边缘长期 HIT。
		context.Header("Cache-Control", "private, max-age=86400")
		http.ServeContent(context.Writer, context.Request, info.Name(), info.ModTime(), file)
	}
}

func (a *App) searchCatalog(context *gin.Context) {
	query := strings.TrimSpace(context.Query("q"))
	if query == "" || utf8.RuneCountInString(query) > 100 {
		writeError(context, http.StatusBadRequest, codeInvalidQuery, "请输入有效的 Dish 名称")
		return
	}

	dishes, err := a.catalog.Search(context.Request.Context(), query)
	if err != nil {
		writeInternalError(context, "search Catalog", err)
		return
	}
	context.JSON(http.StatusOK, gin.H{"dishes": dishes})
}

func (a *App) getRecipe(context *gin.Context) {
	dishID := context.Query("dish_id")
	if !catalog.ValidDishID(dishID) {
		writeError(context, http.StatusBadRequest, codeInvalidRequest, "Dish 无效")
		return
	}

	recipe, err := a.catalog.Recipe(context.Request.Context(), dishID)
	if errors.Is(err, catalog.ErrRecipeNotFound) {
		writeError(context, http.StatusNotFound, codeRecipeNotFound, "Recipe 不存在")
		return
	}
	if err != nil {
		writeInternalError(context, "read Recipe", err)
		return
	}
	context.JSON(http.StatusOK, recipe)
}
