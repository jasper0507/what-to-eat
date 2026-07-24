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

func TestRerollReplacesCurrentDecisionWithoutCreatingEatingRecord(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCatalogEater(t, app)

	for _, body := range []string{
		`{"dish_id":"vegetable_dish/番茄炒蛋.md","preference_weight":1}`,
		`{"dish_id":"meat_dish/番茄牛腩.md","preference_weight":1}`,
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

	beginResponse := candidatePoolRequest(t, app, http.MethodPost, "/api/meals", "", sessionCookie)
	if beginResponse.Code != http.StatusCreated {
		t.Fatalf("Begin status = %d, want %d; body = %s", beginResponse.Code, http.StatusCreated, beginResponse.Body)
	}
	var begun mealState
	if err := json.NewDecoder(beginResponse.Body).Decode(&begun); err != nil {
		t.Fatal(err)
	}

	rerollPath := "/api/decisions/" + strconv.FormatInt(begun.Decision.ID, 10) + "/reroll"
	var replacement mealState
	for attempt := range 2 {
		response := candidatePoolRequest(t, app, http.MethodPost, rerollPath, "", sessionCookie)
		if response.Code != http.StatusOK {
			t.Fatalf("Reroll attempt %d status = %d, want %d; body = %s", attempt+1, response.Code, http.StatusOK, response.Body)
		}
		var rerolled mealState
		if err := json.NewDecoder(response.Body).Decode(&rerolled); err != nil {
			t.Fatal(err)
		}
		if attempt == 0 {
			replacement = rerolled
		} else if !reflect.DeepEqual(rerolled, replacement) {
			t.Errorf("repeated Reroll = %#v, want replacement %#v", rerolled, replacement)
		}
	}
	if replacement.Status != "active_decision" ||
		replacement.Decision.ID == begun.Decision.ID ||
		replacement.Decision.MealID != begun.Decision.MealID ||
		replacement.Decision.Dish.ID == begun.Decision.Dish.ID {
		t.Fatalf("Reroll = %#v, want a different Decision for Meal %d", replacement, begun.Decision.MealID)
	}

	resumeResponse := candidatePoolRequest(t, app, http.MethodGet, "/api/meals/resume", "", sessionCookie)
	if resumeResponse.Code != http.StatusOK {
		t.Fatalf("Resume status = %d, want %d; body = %s", resumeResponse.Code, http.StatusOK, resumeResponse.Body)
	}
	var resumed mealState
	if err := json.NewDecoder(resumeResponse.Body).Decode(&resumed); err != nil {
		t.Fatal(err)
	}
	if resumed.Decision.ID != replacement.Decision.ID {
		t.Errorf("Resume Decision = %d, want replacement %d", resumed.Decision.ID, replacement.Decision.ID)
	}

	rejectedAcceptance := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/decisions/"+strconv.FormatInt(begun.Decision.ID, 10)+"/accept",
		"",
		sessionCookie,
	)
	if rejectedAcceptance.Code != http.StatusNotFound {
		t.Errorf("rejected Decision Acceptance status = %d, want %d", rejectedAcceptance.Code, http.StatusNotFound)
	}

	acceptedResponse := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/decisions/"+strconv.FormatInt(replacement.Decision.ID, 10)+"/accept",
		"",
		sessionCookie,
	)
	if acceptedResponse.Code != http.StatusOK {
		t.Fatalf("replacement Acceptance status = %d, want %d; body = %s", acceptedResponse.Code, http.StatusOK, acceptedResponse.Body)
	}
	var accepted acceptanceResult
	if err := json.NewDecoder(acceptedResponse.Body).Decode(&accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.EatingRecord.Sequence != 1 {
		t.Errorf("Eating record sequence after Reroll = %d, want 1", accepted.EatingRecord.Sequence)
	}
}

func TestConsecutiveRerollsExplorePoolAndRejectNonCurrentDecisions(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	ownerCookie := registerCandidateEater(t, app, "reroll_owner")
	otherCookie := registerCandidateEater(t, app, "reroll_other")

	for _, body := range []string{
		`{"dish_id":"vegetable_dish/番茄炒蛋.md","preference_weight":1}`,
		`{"dish_id":"meat_dish/番茄牛腩.md","preference_weight":1}`,
		`{"dish_id":"drink/柠檬水.md","preference_weight":1}`,
	} {
		response := candidatePoolRequest(
			t,
			app,
			http.MethodPost,
			"/api/candidate-pool/dishes",
			body,
			ownerCookie,
		)
		if response.Code != http.StatusCreated {
			t.Fatalf("add status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body)
		}
	}

	beginResponse := candidatePoolRequest(t, app, http.MethodPost, "/api/meals", "", ownerCookie)
	if beginResponse.Code != http.StatusCreated {
		t.Fatalf("Begin status = %d, want %d; body = %s", beginResponse.Code, http.StatusCreated, beginResponse.Body)
	}
	var current mealState
	if err := json.NewDecoder(beginResponse.Body).Decode(&current); err != nil {
		t.Fatal(err)
	}
	firstDecision := current.Decision
	firstRerollPath := "/api/decisions/" + strconv.FormatInt(firstDecision.ID, 10) + "/reroll"
	otherReroll := candidatePoolRequest(t, app, http.MethodPost, firstRerollPath, "", otherCookie)
	if otherReroll.Code != http.StatusNotFound {
		t.Errorf("other Account Reroll status = %d, want %d", otherReroll.Code, http.StatusNotFound)
	}

	shownDishIDs := map[string]bool{firstDecision.Dish.ID: true}
	for range 2 {
		response := candidatePoolRequest(
			t,
			app,
			http.MethodPost,
			"/api/decisions/"+strconv.FormatInt(current.Decision.ID, 10)+"/reroll",
			"",
			ownerCookie,
		)
		if response.Code != http.StatusOK {
			t.Fatalf("Reroll status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body)
		}
		if err := json.NewDecoder(response.Body).Decode(&current); err != nil {
			t.Fatal(err)
		}
		if shownDishIDs[current.Decision.Dish.ID] {
			t.Fatalf("consecutive Reroll returned shown Dish %q before exploring pool", current.Decision.Dish.ID)
		}
		shownDishIDs[current.Decision.Dish.ID] = true
	}
	if len(shownDishIDs) != 3 {
		t.Errorf("shown Dishes = %#v, want all three Candidate pool Dishes", shownDishIDs)
	}

	staleReroll := candidatePoolRequest(t, app, http.MethodPost, firstRerollPath, "", ownerCookie)
	if staleReroll.Code != http.StatusNotFound {
		t.Errorf("non-current Reroll status = %d, want %d", staleReroll.Code, http.StatusNotFound)
	}

	acceptResponse := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/decisions/"+strconv.FormatInt(current.Decision.ID, 10)+"/accept",
		"",
		ownerCookie,
	)
	if acceptResponse.Code != http.StatusOK {
		t.Fatalf("Acceptance status = %d, want %d; body = %s", acceptResponse.Code, http.StatusOK, acceptResponse.Body)
	}
	acceptedReroll := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/decisions/"+strconv.FormatInt(current.Decision.ID, 10)+"/reroll",
		"",
		ownerCookie,
	)
	if acceptedReroll.Code != http.StatusNotFound {
		t.Errorf("accepted Decision Reroll status = %d, want %d", acceptedReroll.Code, http.StatusNotFound)
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
	seed := int64(2)
	app := openCatalogAppWithDecisionSeed(t, "", &seed)
	t.Cleanup(func() { app.Close() })

	counts := map[string]int{}
	for sample := range 30 {
		cookie := registerCandidateEater(t, app, "preference_eater_"+strconv.Itoa(sample))
		addCandidatePoolDish(t, app, cookie, "vegetable_dish/番茄炒蛋.md", 0.1)
		addCandidatePoolDish(t, app, cookie, "meat_dish/番茄牛腩.md", 5)
		counts[beginMealDecision(t, app, cookie).Dish.ID]++
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

func TestCooldownUsesAcceptanceSequenceWithinTheSameDay(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCandidateEater(t, app, "cooldown_eater")

	tomatoEgg := "vegetable_dish/番茄炒蛋.md"
	tomatoBeef := "meat_dish/番茄牛腩.md"
	lemonWater := "drink/柠檬水.md"

	addCandidatePoolDish(t, app, sessionCookie, tomatoEgg, 1)
	acceptMealDecision(t, app, sessionCookie, beginMealDecision(t, app, sessionCookie), 1)
	removeCandidatePoolDish(t, app, sessionCookie, tomatoEgg)

	addCandidatePoolDish(t, app, sessionCookie, tomatoBeef, 1)
	acceptMealDecision(t, app, sessionCookie, beginMealDecision(t, app, sessionCookie), 2)
	removeCandidatePoolDish(t, app, sessionCookie, tomatoBeef)

	addCandidatePoolDish(t, app, sessionCookie, tomatoEgg, 5)
	addCandidatePoolDish(t, app, sessionCookie, tomatoBeef, 5)
	addCandidatePoolDish(t, app, sessionCookie, lemonWater, 0.1)

	third := beginMealDecision(t, app, sessionCookie)
	if third.Dish.ID != lemonWater {
		t.Fatalf("third Decision Dish = %q, want only non-Cooldown Dish %q", third.Dish.ID, lemonWater)
	}
	acceptMealDecision(t, app, sessionCookie, third, 3)

	updateCandidatePoolDish(t, app, sessionCookie, tomatoEgg, 0.1)
	updateCandidatePoolDish(t, app, sessionCookie, tomatoBeef, 5)
	updateCandidatePoolDish(t, app, sessionCookie, lemonWater, 5)
	fourth := beginMealDecision(t, app, sessionCookie)
	if fourth.Dish.ID != tomatoEgg {
		t.Errorf("fourth Decision Dish = %q, want Dish %q after two further Acceptances", fourth.Dish.ID, tomatoEgg)
	}
}

func TestCooldownRelaxesOnlyAsFarAsNeededWithinCandidatePool(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCandidateEater(t, app, "cooldown_relaxation_eater")

	tomatoEgg := "vegetable_dish/番茄炒蛋.md"
	tomatoBeef := "meat_dish/番茄牛腩.md"

	addCandidatePoolDish(t, app, sessionCookie, tomatoEgg, 1)
	acceptMealDecision(t, app, sessionCookie, beginMealDecision(t, app, sessionCookie), 1)
	removeCandidatePoolDish(t, app, sessionCookie, tomatoEgg)
	addCandidatePoolDish(t, app, sessionCookie, tomatoBeef, 1)
	acceptMealDecision(t, app, sessionCookie, beginMealDecision(t, app, sessionCookie), 2)
	addCandidatePoolDish(t, app, sessionCookie, tomatoEgg, 0.1)
	updateCandidatePoolDish(t, app, sessionCookie, tomatoBeef, 5)

	decision := beginMealDecision(t, app, sessionCookie)
	if decision.Dish.ID != tomatoEgg {
		t.Errorf(
			"relaxed Decision Dish = %q, want %q after shortening Cooldown from two records to one",
			decision.Dish.ID,
			tomatoEgg,
		)
	}
}

func TestRecencyWindowDownweightsRepeatedDishOutsideCooldown(t *testing.T) {
	seed := int64(1)
	app := openCatalogAppWithDecisionSeed(t, "", &seed)
	t.Cleanup(func() { app.Close() })

	tomatoEgg := "vegetable_dish/番茄炒蛋.md"
	tomatoBeef := "meat_dish/番茄牛腩.md"
	lemonWater := "drink/柠檬水.md"
	counts := map[string]int{}

	for sample := range 30 {
		cookie := registerCandidateEater(t, app, "recency_eater_"+strconv.Itoa(sample))
		addCandidatePoolDish(t, app, cookie, tomatoEgg, 1)
		for sequence := int64(1); sequence <= 5; sequence++ {
			acceptMealDecision(t, app, cookie, beginMealDecision(t, app, cookie), sequence)
		}
		removeCandidatePoolDish(t, app, cookie, tomatoEgg)
		addCandidatePoolDish(t, app, cookie, tomatoBeef, 1)
		for sequence := int64(6); sequence <= 7; sequence++ {
			acceptMealDecision(t, app, cookie, beginMealDecision(t, app, cookie), sequence)
		}
		removeCandidatePoolDish(t, app, cookie, tomatoBeef)
		addCandidatePoolDish(t, app, cookie, tomatoEgg, 1)
		addCandidatePoolDish(t, app, cookie, lemonWater, 1)

		counts[beginMealDecision(t, app, cookie).Dish.ID]++
	}

	if counts[lemonWater] < 25 {
		t.Errorf(
			"Decision counts = %#v, want fresh Dish selected in at least 25/30 deterministic samples",
			counts,
		)
	}
}

func TestEatingHistoryIsAccountScoped(t *testing.T) {
	seed := int64(1)
	app := openCatalogAppWithDecisionSeed(t, "", &seed)
	t.Cleanup(func() { app.Close() })
	historyOwner := registerCandidateEater(t, app, "history_owner")
	otherCookie := registerCandidateEater(t, app, "history_other")

	tomatoEgg := "vegetable_dish/番茄炒蛋.md"
	lemonWater := "drink/柠檬水.md"
	addCandidatePoolDish(t, app, historyOwner, tomatoEgg, 1)
	acceptMealDecision(t, app, historyOwner, beginMealDecision(t, app, historyOwner), 1)

	addCandidatePoolDish(t, app, otherCookie, tomatoEgg, 5)
	addCandidatePoolDish(t, app, otherCookie, lemonWater, 0.1)
	decision := beginMealDecision(t, app, otherCookie)
	if decision.Dish.ID != tomatoEgg {
		t.Errorf(
			"other Account Decision Dish = %q, want high-weight Dish %q despite owner's Cooldown",
			decision.Dish.ID,
			tomatoEgg,
		)
	}
}

func TestRerollPreservesEatingHistoryCooldown(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	cookie := registerCandidateEater(t, app, "reroll_cooldown_eater")

	tomatoEgg := "vegetable_dish/番茄炒蛋.md"
	addCandidatePoolDish(t, app, cookie, tomatoEgg, 1)
	acceptMealDecision(t, app, cookie, beginMealDecision(t, app, cookie), 1)
	addCandidatePoolDish(t, app, cookie, "meat_dish/番茄牛腩.md", 1)
	addCandidatePoolDish(t, app, cookie, "drink/柠檬水.md", 1)

	current := beginMealDecision(t, app, cookie)
	response := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/decisions/"+strconv.FormatInt(current.ID, 10)+"/reroll",
		"",
		cookie,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("Reroll status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body)
	}
	var replacement mealState
	if err := json.NewDecoder(response.Body).Decode(&replacement); err != nil {
		t.Fatal(err)
	}
	if replacement.Decision.Dish.ID == tomatoEgg {
		t.Errorf("Reroll Dish = %q, want accepted Dish to remain in Cooldown", tomatoEgg)
	}
	if replacement.Decision.Dish.ID == current.Dish.ID {
		t.Errorf("Reroll Dish = %q, want the other eligible Candidate pool Dish", current.Dish.ID)
	}
}

func addCandidatePoolDish(t *testing.T, app http.Handler, cookie *http.Cookie, dishID string, weight float64) {
	t.Helper()
	response := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/candidate-pool/dishes",
		`{"dish_id":`+strconv.Quote(dishID)+`,"preference_weight":`+strconv.FormatFloat(weight, 'f', -1, 64)+`}`,
		cookie,
	)
	if response.Code != http.StatusCreated {
		t.Fatalf("add Dish %q status = %d, want %d; body = %s", dishID, response.Code, http.StatusCreated, response.Body)
	}
}

func updateCandidatePoolDish(t *testing.T, app http.Handler, cookie *http.Cookie, dishID string, weight float64) {
	t.Helper()
	response := candidatePoolRequest(
		t,
		app,
		http.MethodPatch,
		"/api/candidate-pool/dishes",
		`{"dish_id":`+strconv.Quote(dishID)+`,"preference_weight":`+strconv.FormatFloat(weight, 'f', -1, 64)+`}`,
		cookie,
	)
	if response.Code != http.StatusNoContent {
		t.Fatalf("update Dish %q status = %d, want %d; body = %s", dishID, response.Code, http.StatusNoContent, response.Body)
	}
}

func removeCandidatePoolDish(t *testing.T, app http.Handler, cookie *http.Cookie, dishID string) {
	t.Helper()
	response := candidatePoolRequest(
		t,
		app,
		http.MethodDelete,
		"/api/candidate-pool/dishes?dish_id="+url.QueryEscape(dishID),
		"",
		cookie,
	)
	if response.Code != http.StatusNoContent {
		t.Fatalf("remove Dish %q status = %d, want %d; body = %s", dishID, response.Code, http.StatusNoContent, response.Body)
	}
}

func beginMealDecision(t *testing.T, app http.Handler, cookie *http.Cookie) mealDecision {
	t.Helper()
	response := candidatePoolRequest(t, app, http.MethodPost, "/api/meals", "", cookie)
	if response.Code != http.StatusCreated {
		t.Fatalf("Begin status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body)
	}
	var result mealState
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result.Decision
}

func acceptMealDecision(
	t *testing.T,
	app http.Handler,
	cookie *http.Cookie,
	decision mealDecision,
	wantSequence int64,
) {
	t.Helper()
	response := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/decisions/"+strconv.FormatInt(decision.ID, 10)+"/accept",
		"",
		cookie,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("Acceptance status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body)
	}
	var result acceptanceResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.EatingRecord.Sequence != wantSequence {
		t.Fatalf("Eating record sequence = %d, want %d", result.EatingRecord.Sequence, wantSequence)
	}
}
