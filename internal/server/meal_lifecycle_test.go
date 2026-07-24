package server_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type mealDecision struct {
	ID     int64  `json:"id"`
	MealID int64  `json:"meal_id"`
	Mode   string `json:"mode"`
	Dish   struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"dish"`
}

type mealState struct {
	Status   string       `json:"status"`
	Decision mealDecision `json:"decision"`
}

type acceptanceResult struct {
	EatingRecord struct {
		Sequence int64 `json:"sequence"`
	} `json:"eating_record"`
	Recipe struct {
		Dish candidateDish `json:"dish"`
	} `json:"recipe"`
}

func TestResumeReflectsCandidatePoolReadiness(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCatalogEater(t, app)

	resume := func() string {
		t.Helper()
		response := candidatePoolRequest(
			t,
			app,
			http.MethodGet,
			"/api/meals/resume",
			"",
			sessionCookie,
		)
		if response.Code != http.StatusOK {
			t.Fatalf("resume status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body)
		}
		if strings.Contains(response.Body.String(), `"actions"`) {
			t.Errorf("Resume body = %q, want readiness only", response.Body)
		}
		var result struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result.Status
	}

	empty := resume()
	if empty != "candidate_pool_empty" {
		t.Errorf("empty Resume = %q, want candidate_pool_empty", empty)
	}

	addResponse := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/candidate-pool/dishes",
		`{"dish_id":"vegetable_dish/番茄炒蛋.md","preference_weight":1}`,
		sessionCookie,
	)
	if addResponse.Code != http.StatusCreated {
		t.Fatalf("add status = %d, want %d; body = %s", addResponse.Code, http.StatusCreated, addResponse.Body)
	}

	ready := resume()
	if ready != "ready" {
		t.Errorf("ready Resume = %q, want ready", ready)
	}
}

func TestBeginCreatesOnePoolDecisionAndResumeReturnsIt(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCatalogEater(t, app)

	addResponse := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/candidate-pool/dishes",
		`{"dish_id":"vegetable_dish/番茄炒蛋.md","preference_weight":1}`,
		sessionCookie,
	)
	if addResponse.Code != http.StatusCreated {
		t.Fatalf("add status = %d, want %d; body = %s", addResponse.Code, http.StatusCreated, addResponse.Body)
	}

	beginResponse := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/meals",
		"",
		sessionCookie,
	)
	if beginResponse.Code != http.StatusCreated {
		t.Fatalf("Begin status = %d, want %d; body = %s", beginResponse.Code, http.StatusCreated, beginResponse.Body)
	}
	var begun mealState
	if err := json.NewDecoder(beginResponse.Body).Decode(&begun); err != nil {
		t.Fatal(err)
	}
	if begun.Status != "active_decision" ||
		begun.Decision.ID == 0 ||
		begun.Decision.MealID == 0 ||
		begun.Decision.Mode != "pool" ||
		begun.Decision.Dish.ID != "vegetable_dish/番茄炒蛋.md" ||
		begun.Decision.Dish.Name != "番茄炒蛋" {
		t.Fatalf("Begin = %#v, want one active Pool Decision for 番茄炒蛋", begun)
	}
	if strings.Contains(beginResponse.Body.String(), `"dishes"`) ||
		strings.Contains(beginResponse.Body.String(), `"candidates"`) {
		t.Errorf("Begin body = %q, want exactly one Dish rather than a shortlist", beginResponse.Body)
	}

	repeatedBegin := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/meals",
		"",
		sessionCookie,
	)
	if repeatedBegin.Code != http.StatusOK {
		t.Fatalf("repeated Begin status = %d, want %d; body = %s", repeatedBegin.Code, http.StatusOK, repeatedBegin.Body)
	}
	var repeated mealState
	if err := json.NewDecoder(repeatedBegin.Body).Decode(&repeated); err != nil {
		t.Fatal(err)
	}
	if repeated.Decision.ID != begun.Decision.ID || repeated.Decision.MealID != begun.Decision.MealID {
		t.Errorf("repeated Begin = %#v, want existing Decision %#v", repeated, begun.Decision)
	}

	resumeResponse := candidatePoolRequest(
		t,
		app,
		http.MethodGet,
		"/api/meals/resume",
		"",
		sessionCookie,
	)
	if resumeResponse.Code != http.StatusOK {
		t.Fatalf("Resume status = %d, want %d; body = %s", resumeResponse.Code, http.StatusOK, resumeResponse.Body)
	}
	var resumed mealState
	if err := json.NewDecoder(resumeResponse.Body).Decode(&resumed); err != nil {
		t.Fatal(err)
	}
	if resumed.Status != "active_decision" || resumed.Decision.ID != begun.Decision.ID {
		t.Errorf("Resume = %#v, want active Decision %#v", resumed, begun.Decision)
	}
}

func TestAcceptanceIsAccountScopedIdempotentAndOpensRecipe(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	ownerCookie := registerCandidateEater(t, app, "meal_owner")
	otherCookie := registerCandidateEater(t, app, "other_eater")

	addResponse := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/candidate-pool/dishes",
		`{"dish_id":"meat_dish/番茄牛腩.md","preference_weight":1}`,
		ownerCookie,
	)
	if addResponse.Code != http.StatusCreated {
		t.Fatalf("add status = %d, want %d; body = %s", addResponse.Code, http.StatusCreated, addResponse.Body)
	}

	beginResponse := candidatePoolRequest(t, app, http.MethodPost, "/api/meals", "", ownerCookie)
	if beginResponse.Code != http.StatusCreated {
		t.Fatalf("Begin status = %d, want %d; body = %s", beginResponse.Code, http.StatusCreated, beginResponse.Body)
	}
	var begun mealState
	if err := json.NewDecoder(beginResponse.Body).Decode(&begun); err != nil {
		t.Fatal(err)
	}
	acceptPath := "/api/decisions/" + strconv.FormatInt(begun.Decision.ID, 10) + "/accept"
	otherAcceptance := candidatePoolRequest(t, app, http.MethodPost, acceptPath, "", otherCookie)
	if otherAcceptance.Code != http.StatusNotFound {
		t.Errorf(
			"other Account Acceptance status = %d, want %d",
			otherAcceptance.Code,
			http.StatusNotFound,
		)
	}

	var firstAcceptance acceptanceResult
	for attempt := range 2 {
		response := candidatePoolRequest(t, app, http.MethodPost, acceptPath, "", ownerCookie)
		if response.Code != http.StatusOK {
			t.Fatalf("Acceptance attempt %d status = %d, want %d; body = %s", attempt+1, response.Code, http.StatusOK, response.Body)
		}
		var accepted acceptanceResult
		if err := json.NewDecoder(response.Body).Decode(&accepted); err != nil {
			t.Fatal(err)
		}
		if attempt == 0 {
			firstAcceptance = accepted
		} else if !reflect.DeepEqual(accepted, firstAcceptance) {
			t.Errorf("repeated Acceptance = %#v, want original result %#v", accepted, firstAcceptance)
		}
	}
	if firstAcceptance.EatingRecord.Sequence != 1 ||
		firstAcceptance.Recipe.Dish.ID != "meat_dish/番茄牛腩.md" ||
		firstAcceptance.Recipe.Dish.Name != "番茄牛腩" {
		t.Errorf("Acceptance = %#v, want first Eating record and 番茄牛腩 Recipe reference", firstAcceptance)
	}

	recipePath := "/api/catalog/recipes?dish_id=" +
		url.QueryEscape(firstAcceptance.Recipe.Dish.ID)
	recipeResponse := candidatePoolRequest(t, app, http.MethodGet, recipePath, "", ownerCookie)
	if recipeResponse.Code != http.StatusOK {
		t.Fatalf("Recipe status = %d, want %d; body = %s", recipeResponse.Code, http.StatusOK, recipeResponse.Body)
	}
	var recipe struct {
		Dish    candidateDish `json:"dish"`
		Content string        `json:"content"`
	}
	if err := json.NewDecoder(recipeResponse.Body).Decode(&recipe); err != nil {
		t.Fatal(err)
	}
	if recipe.Dish.ID != firstAcceptance.Recipe.Dish.ID ||
		!strings.Contains(recipe.Content, "炖煮牛腩和番茄") {
		t.Errorf("Recipe = %#v, want accepted 番茄牛腩 HowToCook content", recipe)
	}
}

func TestPreferenceWeightAffectsRepeatedOnDemandDecisions(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCatalogEater(t, app)

	for _, body := range []string{
		`{"dish_id":"vegetable_dish/番茄炒蛋.md","preference_weight":0.1}`,
		`{"dish_id":"meat_dish/番茄牛腩.md","preference_weight":5}`,
	} {
		response := candidatePoolRequest(
			t,
			app,
			http.MethodPost,
			"/api/candidate-pool/dishes",
			body,
			sessionCookie,
		)
		if response.Code != http.StatusCreated {
			t.Fatalf("add status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body)
		}
	}

	counts := map[string]int{}
	for mealNumber := int64(1); mealNumber <= 80; mealNumber++ {
		beginResponse := candidatePoolRequest(t, app, http.MethodPost, "/api/meals", "", sessionCookie)
		if beginResponse.Code != http.StatusCreated {
			t.Fatalf("Meal %d Begin status = %d, want %d; body = %s", mealNumber, beginResponse.Code, http.StatusCreated, beginResponse.Body)
		}
		var begun mealState
		if err := json.NewDecoder(beginResponse.Body).Decode(&begun); err != nil {
			t.Fatal(err)
		}
		counts[begun.Decision.Dish.ID]++

		acceptResponse := candidatePoolRequest(
			t,
			app,
			http.MethodPost,
			"/api/decisions/"+strconv.FormatInt(begun.Decision.ID, 10)+"/accept",
			"",
			sessionCookie,
		)
		if acceptResponse.Code != http.StatusOK {
			t.Fatalf("Meal %d Acceptance status = %d, want %d; body = %s", mealNumber, acceptResponse.Code, http.StatusOK, acceptResponse.Body)
		}
		var accepted acceptanceResult
		if err := json.NewDecoder(acceptResponse.Body).Decode(&accepted); err != nil {
			t.Fatal(err)
		}
		if accepted.EatingRecord.Sequence != mealNumber {
			t.Fatalf(
				"Meal %d Eating record sequence = %d, want %d",
				mealNumber,
				accepted.EatingRecord.Sequence,
				mealNumber,
			)
		}
	}

	highWeight := counts["meat_dish/番茄牛腩.md"]
	lowWeight := counts["vegetable_dish/番茄炒蛋.md"]
	if highWeight <= lowWeight {
		t.Errorf("Decision counts = %#v, want weight 5 Dish more often than weight 0.1 Dish", counts)
	}
}

func TestPoolDecisionSelectionIsIsolatedByAccount(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	firstCookie := registerCandidateEater(t, app, "first_meal_eater")
	secondCookie := registerCandidateEater(t, app, "second_meal_eater")

	for _, fixture := range []struct {
		cookie *http.Cookie
		body   string
	}{
		{firstCookie, `{"dish_id":"drink/柠檬水.md","preference_weight":1}`},
		{secondCookie, `{"dish_id":"vegetable_dish/番茄炒蛋.md","preference_weight":1}`},
	} {
		response := candidatePoolRequest(
			t,
			app,
			http.MethodPost,
			"/api/candidate-pool/dishes",
			fixture.body,
			fixture.cookie,
		)
		if response.Code != http.StatusCreated {
			t.Fatalf("add status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body)
		}
	}

	for _, fixture := range []struct {
		cookie *http.Cookie
		dishID string
	}{
		{firstCookie, "drink/柠檬水.md"},
		{secondCookie, "vegetable_dish/番茄炒蛋.md"},
	} {
		response := candidatePoolRequest(t, app, http.MethodPost, "/api/meals", "", fixture.cookie)
		if response.Code != http.StatusCreated {
			t.Fatalf("Begin status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body)
		}
		var begun mealState
		if err := json.NewDecoder(response.Body).Decode(&begun); err != nil {
			t.Fatal(err)
		}
		if begun.Decision.Dish.ID != fixture.dishID {
			t.Errorf("Decision Dish = %q, want own Account Dish %q", begun.Decision.Dish.ID, fixture.dishID)
		}
	}
}

func TestCandidatePoolEmptyBlocksDecisionWithoutCreatingMeal(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCatalogEater(t, app)

	for range 2 {
		beginResponse := candidatePoolRequest(
			t,
			app,
			http.MethodPost,
			"/api/meals",
			"",
			sessionCookie,
		)
		if beginResponse.Code != http.StatusConflict {
			t.Fatalf(
				"begin status = %d, want %d; body = %s",
				beginResponse.Code,
				http.StatusConflict,
				beginResponse.Body,
			)
		}
		var result struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if strings.Contains(beginResponse.Body.String(), `"actions"`) {
			t.Errorf("blocked Decision body = %q, want error only", beginResponse.Body)
		}
		if err := json.NewDecoder(beginResponse.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if result.Error.Code != "candidate_pool_empty" {
			t.Errorf("blocked Decision = %#v, want candidate_pool_empty", result)
		}
	}

	resumeResponse := candidatePoolRequest(
		t,
		app,
		http.MethodGet,
		"/api/meals/resume",
		"",
		sessionCookie,
	)
	if resumeResponse.Code != http.StatusOK ||
		!strings.Contains(resumeResponse.Body.String(), `"status":"candidate_pool_empty"`) {
		t.Errorf("Resume after blocked attempts = (%d, %q), want candidate_pool_empty", resumeResponse.Code, resumeResponse.Body)
	}
}
