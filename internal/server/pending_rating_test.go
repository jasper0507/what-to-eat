package server_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/jasper0507/what-to-eat/internal/server"
)

type pendingRating struct {
	ID     int64         `json:"id"`
	MealID int64         `json:"meal_id"`
	MealAt int64         `json:"meal_at"`
	Dish   candidateDish `json:"dish"`
}

type tasteRatingResult struct {
	PendingRatingID  int64         `json:"pending_rating_id"`
	Rating           int           `json:"rating"`
	Outcome          string        `json:"outcome"`
	PreferenceWeight *float64      `json:"preference_weight"`
	Dish             candidateDish `json:"dish"`
}

func pendingRatingDiscoveryConfig(maxDiscoveriesPerMeal int) server.DiscoveryConfig {
	return server.DiscoveryConfig{
		Enabled:               true,
		MaxPoolSize:           1,
		MaxEligibleDishes:     1,
		MinRerolls:            2,
		RequiredSignals:       2,
		RecentMealWindow:      3,
		MaxDiscoveriesPerMeal: maxDiscoveriesPerMeal,
	}
}

func TestDiscoveryAcceptanceReturnsRecipeAndBlocksBeginWithPendingRating(t *testing.T) {
	app := openCatalogAppWithDiscovery(t, pendingRatingDiscoveryConfig(2), 1)
	t.Cleanup(func() { app.Close() })
	cookie := registerCandidateEater(t, app, "pending_acceptance_eater")
	addCandidatePoolDish(t, app, cookie, "vegetable_dish/番茄炒蛋.md", 5)
	decision := beginMealDecision(t, app, cookie)
	if decision.Mode != "discovery" {
		t.Fatalf("Decision mode = %q, want discovery", decision.Mode)
	}

	acceptPath := "/api/decisions/" + strconv.FormatInt(decision.ID, 10) + "/accept"
	var first acceptanceResult
	for attempt := range 2 {
		response := candidatePoolRequest(t, app, http.MethodPost, acceptPath, "", cookie)
		if response.Code != http.StatusOK {
			t.Fatalf("Acceptance attempt %d status = %d, want %d; body = %s", attempt+1, response.Code, http.StatusOK, response.Body)
		}
		var accepted acceptanceResult
		if err := json.NewDecoder(response.Body).Decode(&accepted); err != nil {
			t.Fatal(err)
		}
		if attempt == 0 {
			first = accepted
		} else if !reflect.DeepEqual(accepted, first) {
			t.Errorf("repeated Acceptance = %#v, want original result %#v", accepted, first)
		}
	}

	if first.PendingRating == nil ||
		first.PendingRating.ID <= 0 ||
		first.PendingRating.MealID != decision.MealID ||
		first.PendingRating.MealAt <= 0 ||
		first.PendingRating.Dish.ID != decision.Dish.ID {
		t.Fatalf("Pending rating = %#v, want accepted Discovery with Meal recall cues", first.PendingRating)
	}
	if first.Recipe.Dish.ID != decision.Dish.ID {
		t.Errorf("Recipe Dish = %q, want accepted Discovery %q", first.Recipe.Dish.ID, decision.Dish.ID)
	}
	recipe := candidatePoolRequest(
		t,
		app,
		http.MethodGet,
		"/api/catalog/recipes?dish_id="+url.QueryEscape(first.Recipe.Dish.ID),
		"",
		cookie,
	)
	if recipe.Code != http.StatusOK {
		t.Errorf("Recipe status = %d, want %d; body = %s", recipe.Code, http.StatusOK, recipe.Body)
	}

	resumeResponse := candidatePoolRequest(t, app, http.MethodGet, "/api/meals/resume", "", cookie)
	if resumeResponse.Code != http.StatusOK {
		t.Fatalf("Resume status = %d, want %d; body = %s", resumeResponse.Code, http.StatusOK, resumeResponse.Body)
	}
	var resumed mealState
	if err := json.NewDecoder(resumeResponse.Body).Decode(&resumed); err != nil {
		t.Fatal(err)
	}
	if resumed.Status != "pending_ratings" ||
		len(resumed.PendingRatings) != 1 ||
		!reflect.DeepEqual(resumed.PendingRatings[0], *first.PendingRating) {
		t.Errorf("Resume = %#v, want accepted Discovery Pending rating first", resumed)
	}

	begin := candidatePoolRequest(t, app, http.MethodPost, "/api/meals", "", cookie)
	if begin.Code != http.StatusConflict {
		t.Fatalf("Begin status = %d, want %d; body = %s", begin.Code, http.StatusConflict, begin.Body)
	}
	var blocked struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(begin.Body).Decode(&blocked); err != nil {
		t.Fatal(err)
	}
	if blocked.Error.Code != "pending_ratings" {
		t.Errorf("Begin error code = %q, want pending_ratings", blocked.Error.Code)
	}
}

func TestTasteRatingsThreeToFiveAdmitDishWithOrderedPreferenceWeights(t *testing.T) {
	app := openCatalogAppWithDiscovery(t, pendingRatingDiscoveryConfig(2), 2)
	t.Cleanup(func() { app.Close() })

	for _, fixture := range []struct {
		rating int
		weight float64
	}{
		{rating: 3, weight: 0.7},
		{rating: 4, weight: 1.0},
		{rating: 5, weight: 1.3},
	} {
		t.Run(strconv.Itoa(fixture.rating), func(t *testing.T) {
			cookie := registerCandidateEater(
				t,
				app,
				"rating_admission_"+strconv.Itoa(fixture.rating),
			)
			addCandidatePoolDish(t, app, cookie, "vegetable_dish/番茄炒蛋.md", 5)
			decision := beginMealDecision(t, app, cookie)
			accepted := acceptDecisionResult(t, app, cookie, decision)
			if accepted.PendingRating == nil {
				t.Fatal("Discovery Acceptance did not return a Pending rating")
			}

			rated := ratePending(t, app, cookie, accepted.PendingRating.ID, fixture.rating)
			if rated.PendingRatingID != accepted.PendingRating.ID ||
				rated.Rating != fixture.rating ||
				rated.Outcome != "pool_admission" ||
				rated.PreferenceWeight == nil ||
				*rated.PreferenceWeight != fixture.weight ||
				rated.Dish.ID != decision.Dish.ID {
				t.Errorf("Taste rating result = %#v, want Pool admission at %.1f", rated, fixture.weight)
			}

			list := candidatePoolRequest(
				t,
				app,
				http.MethodGet,
				"/api/candidate-pool/dishes",
				"",
				cookie,
			)
			if list.Code != http.StatusOK {
				t.Fatalf("Candidate pool status = %d, want %d; body = %s", list.Code, http.StatusOK, list.Body)
			}
			var pool struct {
				Dishes []candidateDish `json:"dishes"`
			}
			if err := json.NewDecoder(list.Body).Decode(&pool); err != nil {
				t.Fatal(err)
			}
			var admitted *candidateDish
			for index := range pool.Dishes {
				if pool.Dishes[index].ID == decision.Dish.ID {
					admitted = &pool.Dishes[index]
				}
			}
			if admitted == nil || admitted.PreferenceWeight != fixture.weight {
				t.Errorf("Candidate pool = %#v, want admitted Discovery at %.1f", pool.Dishes, fixture.weight)
			}

			updateCandidatePoolDish(t, app, cookie, decision.Dish.ID, 1.1)
			resume := candidatePoolRequest(t, app, http.MethodGet, "/api/meals/resume", "", cookie)
			var state mealState
			if resume.Code != http.StatusOK {
				t.Fatalf("Resume status = %d, want %d; body = %s", resume.Code, http.StatusOK, resume.Body)
			}
			if err := json.NewDecoder(resume.Body).Decode(&state); err != nil {
				t.Fatal(err)
			}
			if state.Status != "ready" || len(state.PendingRatings) != 0 {
				t.Errorf("Resume after ordinary weight edit = %#v, want ready without Pending rating", state)
			}
		})
	}
}

func TestLowTasteRatingIsAccountScopedIdempotentAndDurablyRejectsDish(t *testing.T) {
	app := openCatalogAppWithDiscovery(t, pendingRatingDiscoveryConfig(3), 4)
	t.Cleanup(func() { app.Close() })
	ownerCookie := registerCandidateEater(t, app, "rating_rejection_owner")
	otherCookie := registerCandidateEater(t, app, "rating_rejection_other")
	poolDishID := "vegetable_dish/番茄炒蛋.md"
	addCandidatePoolDish(t, app, ownerCookie, poolDishID, 5)
	discovery := beginMealDecision(t, app, ownerCookie)
	accepted := acceptDecisionResult(t, app, ownerCookie, discovery)
	if accepted.PendingRating == nil {
		t.Fatal("Discovery Acceptance did not return a Pending rating")
	}
	pendingID := accepted.PendingRating.ID

	otherResume := candidatePoolRequest(
		t,
		app,
		http.MethodGet,
		"/api/meals/resume",
		"",
		otherCookie,
	)
	var otherState mealState
	if otherResume.Code != http.StatusOK {
		t.Fatalf("other Account Resume status = %d, want %d", otherResume.Code, http.StatusOK)
	}
	if err := json.NewDecoder(otherResume.Body).Decode(&otherState); err != nil {
		t.Fatal(err)
	}
	if otherState.Status == "pending_ratings" || len(otherState.PendingRatings) != 0 {
		t.Errorf("other Account Resume = %#v, want no owner's Pending rating", otherState)
	}

	ratePath := "/api/pending-ratings/" + strconv.FormatInt(pendingID, 10) + "/rate"
	otherRating := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		ratePath,
		`{"rating":2}`,
		otherCookie,
	)
	if otherRating.Code != http.StatusNotFound {
		t.Errorf("other Account Taste rating status = %d, want %d", otherRating.Code, http.StatusNotFound)
	}
	invalidRating := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		ratePath,
		`{"rating":0}`,
		ownerCookie,
	)
	if invalidRating.Code != http.StatusBadRequest {
		t.Errorf("invalid Taste rating status = %d, want %d", invalidRating.Code, http.StatusBadRequest)
	}

	addCandidatePoolDish(t, app, ownerCookie, discovery.Dish.ID, 2)
	first := ratePending(t, app, ownerCookie, pendingID, 2)
	second := ratePending(t, app, ownerCookie, pendingID, 2)
	if !reflect.DeepEqual(second, first) {
		t.Errorf("repeated Taste rating = %#v, want original result %#v", second, first)
	}
	if first.Outcome != "rejection_mark" ||
		first.PreferenceWeight != nil ||
		first.Dish.ID != discovery.Dish.ID {
		t.Errorf("Taste rating result = %#v, want NPC Rejection mark", first)
	}

	conflict := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		ratePath,
		`{"rating":1}`,
		ownerCookie,
	)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflicting Taste rating status = %d, want %d; body = %s", conflict.Code, http.StatusConflict, conflict.Body)
	}
	var conflictBody struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(conflict.Body).Decode(&conflictBody); err != nil {
		t.Fatal(err)
	}
	if conflictBody.Error.Code != "rating_conflict" {
		t.Errorf("conflicting Taste rating code = %q, want rating_conflict", conflictBody.Error.Code)
	}
	if afterConflict := ratePending(t, app, ownerCookie, pendingID, 2); !reflect.DeepEqual(afterConflict, first) {
		t.Errorf("Taste rating after conflict = %#v, want unchanged result %#v", afterConflict, first)
	}

	list := candidatePoolRequest(
		t,
		app,
		http.MethodGet,
		"/api/candidate-pool/dishes",
		"",
		ownerCookie,
	)
	var pool struct {
		Dishes []candidateDish `json:"dishes"`
	}
	if list.Code != http.StatusOK {
		t.Fatalf("Candidate pool status = %d, want %d; body = %s", list.Code, http.StatusOK, list.Body)
	}
	if err := json.NewDecoder(list.Body).Decode(&pool); err != nil {
		t.Fatal(err)
	}
	for _, dish := range pool.Dishes {
		if dish.ID == discovery.Dish.ID {
			t.Errorf("Candidate pool still contains rejected Dish %q", dish.ID)
		}
	}

	addCandidatePoolDish(t, app, ownerCookie, "drink/柠檬水.md", 1)
	addCandidatePoolDish(t, app, ownerCookie, "meat_dish/番茄牛腩.md", 1)
	for sequence := int64(2); sequence <= 3; sequence++ {
		acceptMealDecision(t, app, ownerCookie, beginMealDecision(t, app, ownerCookie), sequence)
	}
	removeCandidatePoolDish(t, app, ownerCookie, "drink/柠檬水.md")
	removeCandidatePoolDish(t, app, ownerCookie, "meat_dish/番茄牛腩.md")

	next := beginMealDecision(t, app, ownerCookie)
	if next.Mode != "discovery" {
		t.Fatalf("Decision after Rejection mark = %#v, want Discovery pressure", next)
	}
	if next.Dish.ID == discovery.Dish.ID {
		t.Fatalf("Discovery returned rejected Dish %q after Cooldown elapsed", discovery.Dish.ID)
	}
	rerolled := rerollMealDecision(t, app, ownerCookie, next)
	if rerolled.Dish.ID == discovery.Dish.ID {
		t.Errorf("Discovery Reroll returned rejected Dish %q after Cooldown elapsed", discovery.Dish.ID)
	}
}

func TestResumeRequiresEachOfMultiplePendingRatings(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "what-to-eat.db")
	config := testConfig(t, databasePath, nil)
	app, err := server.New(config)
	if err != nil {
		t.Fatal(err)
	}
	cookie := registerCandidateEater(t, app, "multiple_pending_eater")
	addCandidatePoolDish(t, app, cookie, "vegetable_dish/番茄炒蛋.md", 5)
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	var accountID int64
	if err := db.QueryRow(
		"SELECT id FROM accounts WHERE username_key = ?",
		"multiple_pending_eater",
	).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO meals (id, account_id, status, created_at) VALUES
			(101, ?, 'accepted', 1710000100),
			(102, ?, 'accepted', 1710000200);
		 INSERT INTO decisions (
			id, meal_id, dish_id, mode, reason, status, created_at
		 ) VALUES
			(201, 101, 'vegetable_dish/番茄豆腐.md', 'discovery', '', 'accepted', 1710000100),
			(202, 102, 'vegetable_dish/番茄土豆.md', 'discovery', '', 'accepted', 1710000200);
		 INSERT INTO eating_records (
			account_id, sequence, meal_id, decision_id, dish_id, accepted_at
		 ) VALUES
			(?, 1, 101, 201, 'vegetable_dish/番茄豆腐.md', 1710000110),
			(?, 2, 102, 202, 'vegetable_dish/番茄土豆.md', 1710000210);
		 INSERT INTO pending_ratings (
			account_id, meal_id, decision_id, dish_id, meal_at
		 ) VALUES
			(?, 101, 201, 'vegetable_dish/番茄豆腐.md', 1710000110),
			(?, 102, 202, 'vegetable_dish/番茄土豆.md', 1710000210);`,
		accountID,
		accountID,
		accountID,
		accountID,
		accountID,
		accountID,
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	app, err = server.New(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })

	resume := candidatePoolRequest(t, app, http.MethodGet, "/api/meals/resume", "", cookie)
	if resume.Code != http.StatusOK {
		t.Fatalf("Resume status = %d, want %d; body = %s", resume.Code, http.StatusOK, resume.Body)
	}
	var state mealState
	if err := json.NewDecoder(resume.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	if state.Status != "pending_ratings" ||
		len(state.PendingRatings) != 2 ||
		state.PendingRatings[0].Dish.Name != "番茄豆腐" ||
		state.PendingRatings[0].MealAt != 1710000110 ||
		state.PendingRatings[1].Dish.Name != "番茄土豆" ||
		state.PendingRatings[1].MealAt != 1710000210 {
		t.Fatalf("Resume = %#v, want two Pending ratings in Meal order", state)
	}

	ratePending(t, app, cookie, state.PendingRatings[0].ID, 3)
	resume = candidatePoolRequest(t, app, http.MethodGet, "/api/meals/resume", "", cookie)
	state = mealState{}
	if err := json.NewDecoder(resume.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	if state.Status != "pending_ratings" ||
		len(state.PendingRatings) != 1 ||
		state.PendingRatings[0].Dish.Name != "番茄土豆" {
		t.Errorf("Resume after one rating = %#v, want remaining Pending rating", state)
	}
	blocked := candidatePoolRequest(t, app, http.MethodPost, "/api/meals", "", cookie)
	if blocked.Code != http.StatusConflict {
		t.Errorf("Begin with one Pending rating status = %d, want %d", blocked.Code, http.StatusConflict)
	}

	ratePending(t, app, cookie, state.PendingRatings[0].ID, 2)
	resume = candidatePoolRequest(t, app, http.MethodGet, "/api/meals/resume", "", cookie)
	state = mealState{}
	if err := json.NewDecoder(resume.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	if state.Status != "ready" || len(state.PendingRatings) != 0 {
		t.Errorf("Resume after all ratings = %#v, want ready", state)
	}
}

func TestExplicitAddRevokesRejectionMark(t *testing.T) {
	app := openCatalogAppWithDiscovery(t, pendingRatingDiscoveryConfig(2), 5)
	t.Cleanup(func() { app.Close() })
	cookie := registerCandidateEater(t, app, "rejection_revoke_eater")
	addCandidatePoolDish(t, app, cookie, "vegetable_dish/番茄炒蛋.md", 5)
	discovery := beginMealDecision(t, app, cookie)
	if discovery.Mode != "discovery" {
		t.Fatalf("Decision = %#v, want Discovery", discovery)
	}
	accepted := acceptDecisionResult(t, app, cookie, discovery)
	if accepted.PendingRating == nil {
		t.Fatal("Discovery Acceptance did not return a Pending rating")
	}
	ratePending(t, app, cookie, accepted.PendingRating.ID, 2)
	removeCandidatePoolDish(t, app, cookie, "vegetable_dish/番茄炒蛋.md")

	resumeState := func() mealState {
		t.Helper()
		response := candidatePoolRequest(t, app, http.MethodGet, "/api/meals/resume", "", cookie)
		if response.Code != http.StatusOK {
			t.Fatalf("Resume status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body)
		}
		var state mealState
		if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
			t.Fatal(err)
		}
		return state
	}
	if state := resumeState(); state.Status != "candidate_pool_empty" {
		t.Fatalf("Resume after removing pool = %#v, want candidate_pool_empty", state)
	}

	addCandidatePoolDish(t, app, cookie, discovery.Dish.ID, 2)
	if state := resumeState(); state.Status != "ready" {
		t.Errorf("Resume after re-adding rejected Dish = %#v, want revoked Rejection mark to restore readiness", state)
	}

	list := candidatePoolRequest(t, app, http.MethodGet, "/api/candidate-pool/dishes", "", cookie)
	var pool struct {
		Dishes []candidateDish `json:"dishes"`
	}
	if err := json.NewDecoder(list.Body).Decode(&pool); err != nil {
		t.Fatal(err)
	}
	if len(pool.Dishes) != 1 ||
		pool.Dishes[0].ID != discovery.Dish.ID ||
		pool.Dishes[0].PreferenceWeight != 2 {
		t.Errorf("Candidate pool = %#v, want re-admitted Dish %q at weight 2", pool.Dishes, discovery.Dish.ID)
	}
}

func acceptDecisionResult(
	t *testing.T,
	app http.Handler,
	cookie *http.Cookie,
	decision mealDecision,
) acceptanceResult {
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
	return result
}

func ratePending(
	t *testing.T,
	app http.Handler,
	cookie *http.Cookie,
	pendingRatingID int64,
	rating int,
) tasteRatingResult {
	t.Helper()
	response := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/pending-ratings/"+strconv.FormatInt(pendingRatingID, 10)+"/rate",
		`{"rating":`+strconv.Itoa(rating)+`}`,
		cookie,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("Taste rating status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body)
	}
	var result tasteRatingResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}
