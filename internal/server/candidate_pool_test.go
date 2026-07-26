package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

type candidateDish struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Category   string   `json:"category"`
	RecipePath string   `json:"recipe_path"`
	Tags       []string `json:"tags"`
	Tier       int      `json:"tier"`
}

func candidatePoolRequest(
	t *testing.T,
	app http.Handler,
	method string,
	path string,
	body string,
	sessionCookie *http.Cookie,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if sessionCookie != nil {
		request.AddCookie(sessionCookie)
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	return response
}

func registerCandidateEater(t *testing.T, app http.Handler, username string) *http.Cookie {
	t.Helper()
	response := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/auth/register",
		fmt.Sprintf(
			`{"username":%q,"password":"correct horse battery staple"}`,
			username,
		),
		nil,
	)
	if response.Code != http.StatusCreated {
		t.Fatalf("register %q status = %d, want %d; body = %s", username, response.Code, http.StatusCreated, response.Body)
	}
	return response.Result().Cookies()[0]
}

func TestEaterCanAddCatalogDishToCandidatePool(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCatalogEater(t, app)

	addResponse := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/candidate-pool/dishes",
		`{"dish_id":"vegetable_dish/番茄炒蛋.md","tier":4}`,
		sessionCookie,
	)
	if addResponse.Code != http.StatusCreated {
		t.Fatalf("add status = %d, want %d; body = %s", addResponse.Code, http.StatusCreated, addResponse.Body)
	}
	if addResponse.Body.Len() != 0 {
		t.Errorf("add body = %q, want empty", addResponse.Body)
	}

	listResponse := candidatePoolRequest(
		t,
		app,
		http.MethodGet,
		"/api/candidate-pool/dishes",
		"",
		sessionCookie,
	)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body = %s", listResponse.Code, http.StatusOK, listResponse.Body)
	}
	var result struct {
		Dishes []candidateDish `json:"dishes"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Dishes) != 1 {
		t.Fatalf("dishes = %#v, want one Candidate pool member", result.Dishes)
	}
	dish := result.Dishes[0]
	if dish.ID != "vegetable_dish/番茄炒蛋.md" ||
		dish.Name != "番茄炒蛋" ||
		dish.Category != "素菜" ||
		dish.RecipePath != dish.ID ||
		dish.Tier != 4 {
		t.Errorf("dish = %#v, want 番茄炒蛋 at 顶尖", dish)
	}
}

func TestTierPersistsAfterRelogin(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "what-to-eat.db")
	app := openCatalogApp(t, databasePath)
	sessionCookie := registerCatalogEater(t, app)

	addResponse := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/candidate-pool/dishes",
		`{"dish_id":"meat_dish/番茄牛腩.md","tier":4}`,
		sessionCookie,
	)
	if addResponse.Code != http.StatusCreated {
		t.Fatalf("add status = %d, want %d; body = %s", addResponse.Code, http.StatusCreated, addResponse.Body)
	}
	updateResponse := candidatePoolRequest(
		t,
		app,
		http.MethodPatch,
		"/api/candidate-pool/dishes",
		`{"dish_id":"meat_dish/番茄牛腩.md","tier":5}`,
		sessionCookie,
	)
	if updateResponse.Code != http.StatusNoContent {
		t.Fatalf(
			"update status = %d, want %d; body = %s",
			updateResponse.Code,
			http.StatusNoContent,
			updateResponse.Body,
		)
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	app = openCatalogApp(t, databasePath)
	t.Cleanup(func() { app.Close() })
	loginResponse := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/auth/login",
		`{"username":"catalog_eater","password":"correct horse battery staple"}`,
		nil,
	)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d; body = %s", loginResponse.Code, http.StatusOK, loginResponse.Body)
	}
	sessionCookie = loginResponse.Result().Cookies()[0]

	listResponse := candidatePoolRequest(
		t,
		app,
		http.MethodGet,
		"/api/candidate-pool/dishes",
		"",
		sessionCookie,
	)
	var result struct {
		Dishes []candidateDish `json:"dishes"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Dishes) != 1 || result.Dishes[0].Tier != 5 {
		t.Errorf("dishes after relogin = %#v, want 夯", result.Dishes)
	}
}

func TestEaterCanRemoveDishFromCandidatePool(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCatalogEater(t, app)
	dishID := "drink/柠檬水.md"

	addResponse := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/candidate-pool/dishes",
		`{"dish_id":"drink/柠檬水.md","tier":4}`,
		sessionCookie,
	)
	if addResponse.Code != http.StatusCreated {
		t.Fatalf("add status = %d, want %d; body = %s", addResponse.Code, http.StatusCreated, addResponse.Body)
	}

	removeResponse := candidatePoolRequest(
		t,
		app,
		http.MethodDelete,
		"/api/candidate-pool/dishes?dish_id="+url.QueryEscape(dishID),
		"",
		sessionCookie,
	)
	if removeResponse.Code != http.StatusNoContent {
		t.Fatalf(
			"remove status = %d, want %d; body = %s",
			removeResponse.Code,
			http.StatusNoContent,
			removeResponse.Body,
		)
	}

	listResponse := candidatePoolRequest(
		t,
		app,
		http.MethodGet,
		"/api/candidate-pool/dishes",
		"",
		sessionCookie,
	)
	if listResponse.Code != http.StatusOK || listResponse.Body.String() != `{"dishes":[]}` {
		t.Errorf("list response = (%d, %q), want an empty Candidate pool", listResponse.Code, listResponse.Body)
	}
}

func TestCandidatePoolIsIsolatedByAccount(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	firstCookie := registerCandidateEater(t, app, "first_pool_eater")
	secondCookie := registerCandidateEater(t, app, "second_pool_eater")

	addResponse := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/candidate-pool/dishes",
		`{"dish_id":"vegetable_dish/番茄炒蛋.md","tier":4}`,
		firstCookie,
	)
	if addResponse.Code != http.StatusCreated {
		t.Fatalf("add status = %d, want %d; body = %s", addResponse.Code, http.StatusCreated, addResponse.Body)
	}

	secondList := candidatePoolRequest(
		t,
		app,
		http.MethodGet,
		"/api/candidate-pool/dishes?account_id=1",
		"",
		secondCookie,
	)
	if secondList.Code != http.StatusOK || secondList.Body.String() != `{"dishes":[]}` {
		t.Errorf("second Account list = (%d, %q), want an empty Candidate pool", secondList.Code, secondList.Body)
	}

	secondUpdate := candidatePoolRequest(
		t,
		app,
		http.MethodPatch,
		"/api/candidate-pool/dishes?account_id=1",
		`{"dish_id":"vegetable_dish/番茄炒蛋.md","tier":5}`,
		secondCookie,
	)
	if secondUpdate.Code != http.StatusNotFound {
		t.Errorf("cross-Account update status = %d, want %d", secondUpdate.Code, http.StatusNotFound)
	}

	secondRemove := candidatePoolRequest(
		t,
		app,
		http.MethodDelete,
		"/api/candidate-pool/dishes?dish_id="+url.QueryEscape("vegetable_dish/番茄炒蛋.md")+"&account_id=1",
		"",
		secondCookie,
	)
	if secondRemove.Code != http.StatusNotFound {
		t.Errorf("cross-Account remove status = %d, want %d", secondRemove.Code, http.StatusNotFound)
	}

	firstList := candidatePoolRequest(
		t,
		app,
		http.MethodGet,
		"/api/candidate-pool/dishes",
		"",
		firstCookie,
	)
	var result struct {
		Dishes []candidateDish `json:"dishes"`
	}
	if err := json.NewDecoder(firstList.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Dishes) != 1 || result.Dishes[0].Tier != 4 {
		t.Errorf("first Account dishes = %#v, want unchanged 顶尖", result.Dishes)
	}
}

func TestCandidatePoolRejectsInvalidOrUnavailableDish(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCatalogEater(t, app)

	invalid := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/candidate-pool/dishes",
		`{"dish_id":"","tier":4}`,
		sessionCookie,
	)
	if invalid.Code != http.StatusBadRequest {
		t.Errorf("invalid Dish status = %d, want %d", invalid.Code, http.StatusBadRequest)
	}

	assertUnavailable := func(body string) {
		t.Helper()
		response := candidatePoolRequest(
			t,
			app,
			http.MethodPost,
			"/api/candidate-pool/dishes",
			body,
			sessionCookie,
		)
		if response.Code != http.StatusNotFound {
			t.Errorf("unavailable Dish status = %d, want %d", response.Code, http.StatusNotFound)
		}
		var result struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if result.Error.Code != "dish_unavailable" {
			t.Errorf("unavailable Dish error = %q, want dish_unavailable", result.Error.Code)
		}
	}

	// 入池只开上三档：下两档只存在于评分侧
	lowTier := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/candidate-pool/dishes",
		`{"dish_id":"vegetable_dish/番茄炒蛋.md","tier":2}`,
		sessionCookie,
	)
	if lowTier.Code != http.StatusBadRequest {
		t.Errorf("low tier status = %d, want %d", lowTier.Code, http.StatusBadRequest)
	}

	assertUnavailable(`{"dish_id":"vegetable_dish/不存在.md","tier":4}`)

	addResponse := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/candidate-pool/dishes",
		`{"dish_id":"vegetable_dish/番茄炒蛋.md","tier":4}`,
		sessionCookie,
	)
	if addResponse.Code != http.StatusCreated {
		t.Fatalf("add status = %d, want %d; body = %s", addResponse.Code, http.StatusCreated, addResponse.Body)
	}
	assertUnavailable(`{"dish_id":"vegetable_dish/番茄炒蛋.md","tier":4}`)

	// preference_weight 垫片已随段 2 前端铺开退役：旧字段不再被折算
	legacy := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/candidate-pool/dishes",
		`{"dish_id":"meat_dish/番茄牛腩.md","preference_weight":1.4}`,
		sessionCookie,
	)
	if legacy.Code != http.StatusBadRequest {
		t.Fatalf("legacy add status = %d, want %d; body = %s", legacy.Code, http.StatusBadRequest, legacy.Body)
	}
}
