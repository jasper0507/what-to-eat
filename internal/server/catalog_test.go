package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jasper0507/what-to-eat/internal/server"
)

// testConfig 是测试共用的 Config 基座；databasePath 为空时使用临时目录，
// discovery 为 nil 时使用生产默认值。
func testConfig(
	t *testing.T,
	databasePath string,
	discovery *server.DiscoveryConfig,
) server.Config {
	t.Helper()
	if databasePath == "" {
		databasePath = filepath.Join(t.TempDir(), "what-to-eat.db")
	}
	return server.Config{
		DatabasePath:  databasePath,
		SessionSecret: testSessionSecret,
		CatalogDir:    filepath.Join("testdata", "catalog"),
		Discovery:     discovery,
	}
}

func openCatalogApp(t *testing.T, databasePath string) *server.App {
	return openCatalogAppWithDecisionSeed(t, databasePath, nil)
}

func openCatalogAppWithDecisionSeed(
	t *testing.T,
	databasePath string,
	seed *int64,
) *server.App {
	t.Helper()
	config := testConfig(t, databasePath, &server.DiscoveryConfig{Enabled: false})
	var app *server.App
	var err error
	if seed == nil {
		app, err = server.New(config)
	} else {
		app, err = server.NewWithDecisionRandomSeedForTest(config, *seed)
	}
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func registerCatalogEater(t *testing.T, app *server.App) *http.Cookie {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/auth/register",
		bytes.NewBufferString(`{"username":"catalog_eater","password":"correct horse battery staple"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d", response.Code, http.StatusCreated)
	}
	return response.Result().Cookies()[0]
}

func TestCatalogSearchRequiresAuthenticatedAccount(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	request := httptest.NewRequest(http.MethodGet, "/api/catalog/dishes?q=番茄", nil)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestAuthenticatedEaterCanSearchCatalogByDishName(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCatalogEater(t, app)

	request := httptest.NewRequest(http.MethodGet, "/api/catalog/dishes?q=炒蛋", nil)
	request.AddCookie(sessionCookie)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body)
	}
	var result struct {
		Dishes []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Category string `json:"category"`
		} `json:"dishes"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Dishes) != 1 {
		t.Fatalf("dishes = %#v, want one match", result.Dishes)
	}
	dish := result.Dishes[0]
	if dish.ID != "vegetable_dish/番茄炒蛋.md" ||
		dish.Name != "番茄炒蛋" ||
		dish.Category != "素菜" {
		t.Errorf("dish = %#v, want imported 番茄炒蛋 with stable metadata", dish)
	}
}

func TestRepeatedCatalogImportKeepsDishIdentity(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "what-to-eat.db")
	app := openCatalogApp(t, databasePath)
	sessionCookie := registerCatalogEater(t, app)

	search := func(app *server.App) []struct {
		ID string `json:"id"`
	} {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/api/catalog/dishes?q=柠檬水", nil)
		request.AddCookie(sessionCookie)
		response := httptest.NewRecorder()
		app.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("search status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body)
		}
		var result struct {
			Dishes []struct {
				ID string `json:"id"`
			} `json:"dishes"`
		}
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result.Dishes
	}

	firstImport := search(app)
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
	app = openCatalogApp(t, databasePath)
	t.Cleanup(func() { app.Close() })
	secondImport := search(app)

	if len(firstImport) != 1 || len(secondImport) != 1 {
		t.Fatalf("imported dishes = (%#v, %#v), want one Dish after each import", firstImport, secondImport)
	}
	if firstImport[0].ID == "" || firstImport[0].ID != secondImport[0].ID {
		t.Errorf("Dish IDs = (%q, %q), want the same non-empty identity", firstImport[0].ID, secondImport[0].ID)
	}
}

func TestCatalogSearchRejectsInvalidQuery(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCatalogEater(t, app)

	request := httptest.NewRequest(http.MethodGet, "/api/catalog/dishes?q=%20%20", nil)
	request.AddCookie(sessionCookie)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestCatalogSearchReturnsEmptyResultsWithoutCreatingDish(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCatalogEater(t, app)

	for range 2 {
		request := httptest.NewRequest(http.MethodGet, "/api/catalog/dishes?q=不存在的自由文本", nil)
		request.AddCookie(sessionCookie)
		response := httptest.NewRecorder()
		app.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Body.String() != `{"dishes":[]}` {
			t.Fatalf("search response = (%d, %q), want (200, %q)", response.Code, response.Body, `{"dishes":[]}`)
		}
	}
}
