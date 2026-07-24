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
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Category         string   `json:"category"`
	RecipePath       string   `json:"recipe_path"`
	Tags             []string `json:"tags"`
	PreferenceWeight float64  `json:"preference_weight"`
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
		`{"dish_id":"vegetable_dish/番茄炒蛋.md","preference_weight":1.4}`,
		sessionCookie,
	)
	if addResponse.Code != http.StatusCreated {
		t.Fatalf("add status = %d, want %d; body = %s", addResponse.Code, http.StatusCreated, addResponse.Body)
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
		dish.PreferenceWeight != 1.4 {
		t.Errorf("dish = %#v, want 番茄炒蛋 with weight 1.4", dish)
	}
}

func TestPreferenceWeightPersistsAfterRelogin(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "what-to-eat.db")
	app := openCatalogApp(t, databasePath)
	sessionCookie := registerCatalogEater(t, app)

	addResponse := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/candidate-pool/dishes",
		`{"dish_id":"meat_dish/番茄牛腩.md","preference_weight":1}`,
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
		`{"dish_id":"meat_dish/番茄牛腩.md","preference_weight":2.2}`,
		sessionCookie,
	)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d; body = %s", updateResponse.Code, http.StatusOK, updateResponse.Body)
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
	if len(result.Dishes) != 1 || result.Dishes[0].PreferenceWeight != 2.2 {
		t.Errorf("dishes after relogin = %#v, want weight 2.2", result.Dishes)
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
		`{"dish_id":"drink/柠檬水.md","preference_weight":1}`,
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
		`{"dish_id":"vegetable_dish/番茄炒蛋.md","preference_weight":1.4}`,
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
		`{"dish_id":"vegetable_dish/番茄炒蛋.md","preference_weight":5}`,
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
	if len(result.Dishes) != 1 || result.Dishes[0].PreferenceWeight != 1.4 {
		t.Errorf("first Account dishes = %#v, want unchanged weight 1.4", result.Dishes)
	}
}

func TestCandidatePoolRejectsInvalidOrUnknownDish(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCatalogEater(t, app)

	invalid := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/candidate-pool/dishes",
		`{"dish_id":"","preference_weight":1}`,
		sessionCookie,
	)
	if invalid.Code != http.StatusBadRequest {
		t.Errorf("invalid Dish status = %d, want %d", invalid.Code, http.StatusBadRequest)
	}

	unknown := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/candidate-pool/dishes",
		`{"dish_id":"vegetable_dish/不存在.md","preference_weight":1}`,
		sessionCookie,
	)
	if unknown.Code != http.StatusNotFound {
		t.Errorf("unknown Dish status = %d, want %d", unknown.Code, http.StatusNotFound)
	}
}
