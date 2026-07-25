package server_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jasper0507/what-to-eat/internal/server"
)

func openCatalogApp(t *testing.T, databasePath string) *server.App {
	return openCatalogAppWithDecisionSeed(t, databasePath, nil)
}

func openCatalogAppWithDecisionSeed(
	t *testing.T,
	databasePath string,
	seed *int64,
) *server.App {
	t.Helper()
	if databasePath == "" {
		databasePath = filepath.Join(t.TempDir(), "what-to-eat.db")
	}
	config := server.Config{
		DatabasePath:  databasePath,
		SessionSecret: testSessionSecret,
		CatalogDir:    filepath.Join("testdata", "catalog"),
		Discovery:     &server.DiscoveryConfig{Enabled: false},
	}
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
			ID         string   `json:"id"`
			Name       string   `json:"name"`
			Category   string   `json:"category"`
			RecipePath string   `json:"recipe_path"`
			Tags       []string `json:"tags"`
		} `json:"dishes"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Dishes) != 1 {
		t.Fatalf("dishes = %#v, want one match", result.Dishes)
	}
	dish := result.Dishes[0]
	if dish.ID == "" ||
		dish.Name != "番茄炒蛋" ||
		dish.Category != "素菜" ||
		dish.RecipePath != "vegetable_dish/番茄炒蛋.md" ||
		len(dish.Tags) != 0 {
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

func TestCatalogImportUpgradesLegacyCatalogSchema(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "what-to-eat.db")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE catalog_dishes (
			id TEXT PRIMARY KEY,
			source_path TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			category TEXT NOT NULL,
			recipe TEXT NOT NULL,
			tags TEXT NOT NULL
		);
		INSERT INTO catalog_dishes (id, source_path, name, category, recipe, tags)
		VALUES ('legacy-id', 'drink/柠檬水.md', '旧柠檬水', '饮料', '旧 Recipe', '[]');
	`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	app := openCatalogApp(t, databasePath)
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCatalogEater(t, app)

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
	if len(result.Dishes) != 1 || result.Dishes[0].ID != "drink/柠檬水.md" {
		t.Errorf("dishes = %#v, want migrated source path identity", result.Dishes)
	}
}

func TestAppUpgradesPoolOnlyDecisionSchemaForDiscovery(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "what-to-eat.db")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE decisions (
			id INTEGER PRIMARY KEY,
			meal_id INTEGER NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
			dish_id TEXT NOT NULL REFERENCES catalog_dishes(source_path),
			mode TEXT NOT NULL CHECK (mode = 'pool'),
			status TEXT NOT NULL CHECK (status IN ('active', 'accepted')),
			rerolled_to_id INTEGER REFERENCES decisions(id),
			created_at INTEGER NOT NULL
		);
	`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	discovery := server.DefaultDiscoveryConfig()
	discovery.MaxPoolSize = 1
	config := server.Config{
		DatabasePath:  databasePath,
		SessionSecret: testSessionSecret,
		CatalogDir:    filepath.Join("testdata", "catalog"),
		Discovery:     &discovery,
	}
	app, err := server.NewWithDecisionRandomSeedForTest(config, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	cookie := registerCandidateEater(t, app, "discovery_schema_eater")
	addCandidatePoolDish(t, app, cookie, "vegetable_dish/番茄炒蛋.md", 5)

	decision := beginMealDecision(t, app, cookie)
	if decision.Mode != "discovery" {
		t.Errorf("Decision after schema upgrade = %#v, want Discovery mode to be accepted", decision)
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
