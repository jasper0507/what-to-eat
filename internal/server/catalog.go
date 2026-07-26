package server

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/jasper0507/what-to-eat/internal/catalog"
)

func (a *App) searchCatalog(context *gin.Context) {
	query := strings.TrimSpace(context.Query("q"))
	if query == "" || utf8.RuneCountInString(query) > 100 {
		writeError(context, http.StatusBadRequest, codeInvalidQuery, "请输入有效的 Dish 名称")
		return
	}

	dishes, err := a.catalog.Search(context, query)
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

	recipe, err := a.catalog.Recipe(context, dishID)
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
