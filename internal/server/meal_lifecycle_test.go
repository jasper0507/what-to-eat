package server_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/jasper0507/what-to-eat/internal/server"
)

type mealDecision struct {
	ID     int64  `json:"id"`
	MealID int64  `json:"meal_id"`
	Mode   string `json:"mode"`
	Reason string `json:"reason"`
	Dish   struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"dish"`
}

type mealState struct {
	Status           string          `json:"status"`
	Decision         mealDecision    `json:"decision"`
	RerollsRemaining *int            `json:"rerolls_remaining"`
	PendingRatings   []pendingRating `json:"pending_ratings"`
}

type acceptanceResult struct {
	EatingRecord struct {
		Sequence int64 `json:"sequence"`
	} `json:"eating_record"`
	Recipe struct {
		Dish candidateDish `json:"dish"`
	} `json:"recipe"`
	PendingRating *pendingRating `json:"pending_rating"`
}

type wireError struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

func decodeWireError(t *testing.T, body *strings.Reader) string {
	t.Helper()
	var result wireError
	if err := json.NewDecoder(body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result.Error.Code
}

const (
	tomatoEgg   = "vegetable_dish/番茄炒蛋.md"
	tomatoBeef  = "meat_dish/番茄牛腩.md"
	winterSoup  = "soup/冬瓜排骨汤.md"
	lemonWater  = "drink/柠檬水.md"
	plainCongee = "breakfast/白粥.md"
)

// ---- 就绪与揭示基本盘 ----

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
		var result struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result.Status
	}

	if empty := resume(); empty != "candidate_pool_empty" {
		t.Errorf("empty Resume = %q, want candidate_pool_empty", empty)
	}
	addCandidatePoolDish(t, app, sessionCookie, tomatoEgg, 4)
	if ready := resume(); ready != "ready" {
		t.Errorf("ready Resume = %q, want ready", ready)
	}
}

func TestBeginCreatesOnePoolDecisionWithReasonAndBudget(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCatalogEater(t, app)
	addCandidatePoolDish(t, app, sessionCookie, tomatoEgg, 4)

	beginResponse := candidatePoolRequest(t, app, http.MethodPost, "/api/meals", "", sessionCookie)
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
		begun.Decision.Dish.ID != tomatoEgg ||
		begun.Decision.Dish.Name != "番茄炒蛋" {
		t.Fatalf("Begin = %#v, want one active Pool Decision for 番茄炒蛋", begun)
	}
	// ADR-0022：每次揭示必有理由行；State 携带 Reroll budget 余量
	if begun.Decision.Reason == "" {
		t.Errorf("Begin reason is empty, want a mandatory reason line")
	}
	if begun.RerollsRemaining == nil || *begun.RerollsRemaining != 3 {
		t.Errorf("Begin rerolls_remaining = %v, want 3", begun.RerollsRemaining)
	}

	repeatedBegin := candidatePoolRequest(t, app, http.MethodPost, "/api/meals", "", sessionCookie)
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

	resumeResponse := candidatePoolRequest(t, app, http.MethodGet, "/api/meals/resume", "", sessionCookie)
	var resumed mealState
	if err := json.NewDecoder(resumeResponse.Body).Decode(&resumed); err != nil {
		t.Fatal(err)
	}
	if resumed.Status != "active_decision" ||
		resumed.Decision.ID != begun.Decision.ID ||
		resumed.RerollsRemaining == nil ||
		*resumed.RerollsRemaining != 3 {
		t.Errorf("Resume = %#v, want active Decision with budget 3", resumed)
	}
}

func TestBeginRejectsInvalidLocalHour(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCatalogEater(t, app)
	addCandidatePoolDish(t, app, sessionCookie, tomatoEgg, 4)

	response := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/meals",
		`{"local_hour":24}`,
		sessionCookie,
	)
	if response.Code != http.StatusBadRequest {
		t.Errorf("Begin with local_hour 24 status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

// ---- Reroll 与本顿否决 ----

func TestRerollReplacesCurrentDecisionWithoutCreatingEatingRecord(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCatalogEater(t, app)
	addCandidatePoolDish(t, app, sessionCookie, tomatoEgg, 4)
	addCandidatePoolDish(t, app, sessionCookie, tomatoBeef, 4)

	begun := beginMealState(t, app, sessionCookie)
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
	if replacement.RerollsRemaining == nil || *replacement.RerollsRemaining != 2 {
		t.Errorf("Reroll rerolls_remaining = %v, want 2", replacement.RerollsRemaining)
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

func TestConsecutiveRerollsVetoShownDishesAndRejectNonCurrentDecisions(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	ownerCookie := registerCandidateEater(t, app, "reroll_owner")
	otherCookie := registerCandidateEater(t, app, "reroll_other")

	for _, dishID := range []string{tomatoEgg, tomatoBeef, winterSoup} {
		addCandidatePoolDish(t, app, ownerCookie, dishID, 4)
	}

	current := beginMealState(t, app, ownerCookie)
	firstDecision := current.Decision
	firstRerollPath := "/api/decisions/" + strconv.FormatInt(firstDecision.ID, 10) + "/reroll"
	otherReroll := candidatePoolRequest(t, app, http.MethodPost, firstRerollPath, "", otherCookie)
	if otherReroll.Code != http.StatusNotFound {
		t.Errorf("other Account Reroll status = %d, want %d", otherReroll.Code, http.StatusNotFound)
	}

	// 本顿否决：连续 Reroll 必然轮完三道菜，不重复端已展示的
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
			t.Fatalf("consecutive Reroll returned shown Dish %q before exhausting pool", current.Decision.Dish.ID)
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

// ---- Reroll budget 与三出口 ----

func TestRerollBudgetIsThreePerMealAndSettledServerSide(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	cookie := registerCandidateEater(t, app, "budget_eater")
	for _, dishID := range []string{tomatoEgg, tomatoBeef, winterSoup} {
		addCandidatePoolDish(t, app, cookie, dishID, 4)
	}

	state := beginMealState(t, app, cookie)
	for want := 2; want >= 0; want-- {
		response := candidatePoolRequest(
			t,
			app,
			http.MethodPost,
			"/api/decisions/"+strconv.FormatInt(state.Decision.ID, 10)+"/reroll",
			"",
			cookie,
		)
		if response.Code != http.StatusOK {
			t.Fatalf("Reroll (want remaining %d) status = %d; body = %s", want, response.Code, response.Body)
		}
		if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
			t.Fatal(err)
		}
		if state.RerollsRemaining == nil || *state.RerollsRemaining != want {
			t.Fatalf("rerolls_remaining = %v, want %d", state.RerollsRemaining, want)
		}
		if state.Decision.Reason == "" {
			t.Errorf("Reroll reason is empty, want a mandatory reason line")
		}
	}

	exhausted := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/decisions/"+strconv.FormatInt(state.Decision.ID, 10)+"/reroll",
		"",
		cookie,
	)
	if exhausted.Code != http.StatusConflict {
		t.Fatalf("exhausted Reroll status = %d, want %d; body = %s", exhausted.Code, http.StatusConflict, exhausted.Body)
	}
	if code := decodeWireError(t, strings.NewReader(exhausted.Body.String())); code != "reroll_budget_exhausted" {
		t.Errorf("exhausted Reroll code = %q, want reroll_budget_exhausted", code)
	}
}

func TestHandPickUnlocksOnlyAtBudgetExhaustion(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	cookie := registerCandidateEater(t, app, "hand_pick_eater")
	for _, dishID := range []string{tomatoEgg, tomatoBeef, winterSoup} {
		addCandidatePoolDish(t, app, cookie, dishID, 4)
	}

	state := beginMealState(t, app, cookie)
	locked := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/meals/hand-pick",
		`{"dish_id":`+strconv.Quote(tomatoEgg)+`}`,
		cookie,
	)
	if locked.Code != http.StatusConflict {
		t.Fatalf("locked hand-pick status = %d, want %d; body = %s", locked.Code, http.StatusConflict, locked.Body)
	}
	if code := decodeWireError(t, strings.NewReader(locked.Body.String())); code != "hand_pick_locked" {
		t.Errorf("locked hand-pick code = %q, want hand_pick_locked", code)
	}

	for range 3 {
		response := candidatePoolRequest(
			t,
			app,
			http.MethodPost,
			"/api/decisions/"+strconv.FormatInt(state.Decision.ID, 10)+"/reroll",
			"",
			cookie,
		)
		if response.Code != http.StatusOK {
			t.Fatalf("Reroll status = %d; body = %s", response.Code, response.Body)
		}
		if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
			t.Fatal(err)
		}
	}

	outside := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/meals/hand-pick",
		`{"dish_id":`+strconv.Quote(lemonWater)+`}`,
		cookie,
	)
	if outside.Code != http.StatusNotFound {
		t.Errorf("out-of-pool hand-pick status = %d, want %d", outside.Code, http.StatusNotFound)
	}

	picked := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/meals/hand-pick",
		`{"dish_id":`+strconv.Quote(tomatoBeef)+`}`,
		cookie,
	)
	if picked.Code != http.StatusOK {
		t.Fatalf("hand-pick status = %d, want %d; body = %s", picked.Code, http.StatusOK, picked.Body)
	}
	var acceptance acceptanceResult
	if err := json.NewDecoder(picked.Body).Decode(&acceptance); err != nil {
		t.Fatal(err)
	}
	if acceptance.EatingRecord.Sequence != 1 ||
		acceptance.Recipe.Dish.ID != tomatoBeef ||
		acceptance.PendingRating != nil {
		t.Errorf("hand-pick acceptance = %#v, want first Eating record for 番茄牛腩 without Pending rating", acceptance)
	}

	// 手选后站着的最后一次揭示被标记取代：陈旧 accept 得到干净的 404，
	// 而不是撞 eating_records.meal_id 唯一约束的 500
	staleAccept := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/decisions/"+strconv.FormatInt(state.Decision.ID, 10)+"/accept",
		"",
		cookie,
	)
	if staleAccept.Code != http.StatusNotFound {
		t.Errorf("stale accept after hand-pick status = %d, want %d; body = %s", staleAccept.Code, http.StatusNotFound, staleAccept.Body)
	}

	// 本顿已了结：可以开始新的一顿
	next := beginMealState(t, app, cookie)
	if next.Status != "active_decision" {
		t.Errorf("Begin after hand-pick = %#v, want a fresh Meal", next)
	}
}

func TestAbandonSettlesMealWithoutEatingRecordOrCooldown(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	cookie := registerCandidateEater(t, app, "abandon_eater")

	noMeal := candidatePoolRequest(t, app, http.MethodPost, "/api/meals/abandon", "", cookie)
	if noMeal.Code != http.StatusConflict {
		t.Fatalf("abandon without Meal status = %d, want %d", noMeal.Code, http.StatusConflict)
	}
	if code := decodeWireError(t, strings.NewReader(noMeal.Body.String())); code != "meal_not_found" {
		t.Errorf("abandon without Meal code = %q, want meal_not_found", code)
	}

	addCandidatePoolDish(t, app, cookie, tomatoEgg, 4)
	begun := beginMealState(t, app, cookie)

	abandoned := candidatePoolRequest(t, app, http.MethodPost, "/api/meals/abandon", "", cookie)
	if abandoned.Code != http.StatusOK {
		t.Fatalf("abandon status = %d, want %d; body = %s", abandoned.Code, http.StatusOK, abandoned.Body)
	}
	var after mealState
	if err := json.NewDecoder(abandoned.Body).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if after.Status != "ready" {
		t.Errorf("state after abandon = %#v, want ready", after)
	}

	// 无吃饭记录
	records := candidatePoolRequest(t, app, http.MethodGet, "/api/eating-records", "", cookie)
	if records.Code != http.StatusOK || records.Body.String() != `{"records":[]}` {
		t.Errorf("Eating records after abandon = (%d, %q), want empty", records.Code, records.Body)
	}

	// 不进冷却：下一顿正常揭示同一道菜，且不是放宽路径
	next := beginMealState(t, app, cookie)
	if next.Decision.Dish.ID != begun.Decision.Dish.ID {
		t.Errorf("Decision after abandon = %q, want %q outside cooldown", next.Decision.Dish.ID, begun.Decision.Dish.ID)
	}
	if strings.Contains(next.Decision.Reason, "破例") {
		t.Errorf("reason after abandon = %q, want a non-relaxation reason", next.Decision.Reason)
	}

	// 已放弃的 Meal 上的旧 Decision 不可再操作——Reroll 与 Accept 同一口径，
	// 否则陈旧客户端能给放弃的这顿落下幽灵吃饭记录
	staleReroll := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/decisions/"+strconv.FormatInt(begun.Decision.ID, 10)+"/reroll",
		"",
		cookie,
	)
	if staleReroll.Code != http.StatusNotFound {
		t.Errorf("Reroll on abandoned Meal status = %d, want %d", staleReroll.Code, http.StatusNotFound)
	}
	staleAccept := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/decisions/"+strconv.FormatInt(begun.Decision.ID, 10)+"/accept",
		"",
		cookie,
	)
	if staleAccept.Code != http.StatusNotFound {
		t.Errorf("Accept on abandoned Meal status = %d, want %d", staleAccept.Code, http.StatusNotFound)
	}
	// 第二次开的这顿仍站着 active Decision，历史必须只有零条记录
	records = candidatePoolRequest(t, app, http.MethodGet, "/api/eating-records", "", cookie)
	if records.Body.String() != `{"records":[]}` {
		t.Errorf("Eating records after stale accept = %q, want empty", records.Body)
	}
}

// ---- 四因子：喜爱、新鲜感、场合 ----

func TestTierAffectsRepeatedOnDemandDecisions(t *testing.T) {
	seed := int64(2)
	app := openCatalogAppWithDecisionSeed(t, "", &seed)
	t.Cleanup(func() { app.Close() })

	counts := map[string]int{}
	for sample := range 30 {
		cookie := registerCandidateEater(t, app, "preference_eater_"+strconv.Itoa(sample))
		addCandidatePoolDish(t, app, cookie, tomatoEgg, 3)
		addCandidatePoolDish(t, app, cookie, tomatoBeef, 5)
		counts[beginMealDecision(t, app, cookie).Dish.ID]++
	}

	// 夯:人上人 = 4:1
	if counts[tomatoBeef] <= counts[tomatoEgg] {
		t.Errorf("Decision counts = %#v, want 夯 Dish more often than 人上人 Dish", counts)
	}
}

func TestFreshnessCooldownExcludesJustAcceptedDish(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	cookie := registerCandidateEater(t, app, "freshness_eater")
	addCandidatePoolDish(t, app, cookie, tomatoEgg, 5)
	addCandidatePoolDish(t, app, cookie, tomatoBeef, 3)

	first := beginMealDecision(t, app, cookie)
	acceptMealDecision(t, app, cookie, first, 1)

	second := beginMealDecision(t, app, cookie)
	if second.Dish.ID == first.Dish.ID {
		t.Errorf("second Decision = %q, want the other Dish while %q cools down", second.Dish.ID, first.Dish.ID)
	}
}

func TestFreshnessCurveRestoresDishAfterTwoFurtherRecords(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	cookie := registerCandidateEater(t, app, "freshness_curve_eater")

	addCandidatePoolDish(t, app, cookie, tomatoEgg, 4)
	acceptMealDecision(t, app, cookie, beginMealDecision(t, app, cookie), 1)
	removeCandidatePoolDish(t, app, cookie, tomatoEgg)
	addCandidatePoolDish(t, app, cookie, tomatoBeef, 4)
	acceptMealDecision(t, app, cookie, beginMealDecision(t, app, cookie), 2)

	// 炒蛋 d=1（×0）、牛腩 d=0（×0）、冬瓜汤 从未吃过（×1）→ 必出冬瓜汤
	addCandidatePoolDish(t, app, cookie, tomatoEgg, 5)
	addCandidatePoolDish(t, app, cookie, winterSoup, 3)
	third := beginMealDecision(t, app, cookie)
	if third.Dish.ID != winterSoup {
		t.Fatalf("third Decision = %q, want only fresh Dish %q", third.Dish.ID, winterSoup)
	}
	acceptMealDecision(t, app, cookie, third, 3)

	// 现在 炒蛋 d=2（×0.35）复活，牛腩 d=1、冬瓜汤 d=0 仍为零 → 必出炒蛋
	fourth := beginMealDecision(t, app, cookie)
	if fourth.Dish.ID != tomatoEgg {
		t.Errorf("fourth Decision = %q, want %q back at distance 2", fourth.Dish.ID, tomatoEgg)
	}
}

func TestAllCoolingRelaxesFreshnessWithHumanReason(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	cookie := registerCandidateEater(t, app, "relaxation_eater")
	addCandidatePoolDish(t, app, cookie, tomatoEgg, 4)
	acceptMealDecision(t, app, cookie, beginMealDecision(t, app, cookie), 1)

	relaxed := beginMealDecision(t, app, cookie)
	if relaxed.Dish.ID != tomatoEgg {
		t.Fatalf("relaxed Decision = %q, want the only Dish %q", relaxed.Dish.ID, tomatoEgg)
	}
	if relaxed.Reason != "都在冷却，破例放行。" {
		t.Errorf("relaxed reason = %q, want 都在冷却，破例放行。", relaxed.Reason)
	}
}

func TestOccasionNeverClassIsNeverRevealed(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	cookie := registerCandidateEater(t, app, "occasion_never_eater")

	// 池里只有饮料：视同空池——Resume 与 Begin 必须同一口径，
	// 否则 ready 状态下的开始按钮成死键
	addCandidatePoolDish(t, app, cookie, lemonWater, 5)
	resume := candidatePoolRequest(t, app, http.MethodGet, "/api/meals/resume", "", cookie)
	if !strings.Contains(resume.Body.String(), `"status":"candidate_pool_empty"`) {
		t.Errorf("drink-only Resume = %q, want candidate_pool_empty", resume.Body)
	}
	blocked := candidatePoolRequest(t, app, http.MethodPost, "/api/meals", "", cookie)
	if blocked.Code != http.StatusConflict {
		t.Fatalf("drink-only Begin status = %d, want %d; body = %s", blocked.Code, http.StatusConflict, blocked.Body)
	}
	if code := decodeWireError(t, strings.NewReader(blocked.Body.String())); code != "candidate_pool_empty" {
		t.Errorf("drink-only Begin code = %q, want candidate_pool_empty", code)
	}

	// 加一道正餐后永远揭示正餐
	addCandidatePoolDish(t, app, cookie, tomatoEgg, 3)
	for range 3 {
		decision := beginMealDecision(t, app, cookie)
		if decision.Dish.ID != tomatoEgg {
			t.Fatalf("Decision = %q, want drink to stay unrevealable", decision.Dish.ID)
		}
		abandonActiveMeal(t, app, cookie)
	}
}

func TestBreakfastClassFollowsReportedLocalHour(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	cookie := registerCandidateEater(t, app, "breakfast_eater")
	addCandidatePoolDish(t, app, cookie, plainCongee, 4)

	morning := beginMealStateAt(t, app, cookie, 7)
	if morning.Decision.Dish.ID != plainCongee || morning.Decision.Reason != "早上就该来这口。" {
		t.Errorf("morning Decision = %#v, want breakfast boost reason", morning.Decision)
	}
	abandonActiveMeal(t, app, cookie)

	evening := beginMealStateAt(t, app, cookie, 22)
	if evening.Decision.Dish.ID != plainCongee {
		t.Fatalf("evening Decision = %q, want the only Dish despite penalty", evening.Decision.Dish.ID)
	}
	if evening.Decision.Reason == "早上就该来这口。" {
		t.Errorf("evening reason = %q, want no morning boost at 22:00", evening.Decision.Reason)
	}
}

// ---- 自动降档 ----

func TestFourSwapsDemoteOneTierAcceptResetsAndFloorHolds(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	cookie := registerCandidateEater(t, app, "demotion_eater")
	addCandidatePoolDish(t, app, cookie, tomatoEgg, 5)

	poolTier := func() int {
		t.Helper()
		list := candidatePoolRequest(t, app, http.MethodGet, "/api/candidate-pool/dishes", "", cookie)
		var pool struct {
			Dishes []candidateDish `json:"dishes"`
		}
		if err := json.NewDecoder(list.Body).Decode(&pool); err != nil {
			t.Fatal(err)
		}
		if len(pool.Dishes) != 1 {
			t.Fatalf("pool = %#v, want exactly 番茄炒蛋", pool.Dishes)
		}
		return pool.Dishes[0].Tier
	}
	reroll := func(state *mealState) {
		t.Helper()
		response := candidatePoolRequest(
			t,
			app,
			http.MethodPost,
			"/api/decisions/"+strconv.FormatInt(state.Decision.ID, 10)+"/reroll",
			"",
			cookie,
		)
		if response.Code != http.StatusOK {
			t.Fatalf("Reroll status = %d; body = %s", response.Code, response.Body)
		}
		if err := json.NewDecoder(response.Body).Decode(state); err != nil {
			t.Fatal(err)
		}
	}

	// 3 次被换（放弃的 Meal 里站着的那道不计）
	state := beginMealState(t, app, cookie)
	for range 3 {
		reroll(&state)
	}
	abandonActiveMeal(t, app, cookie)
	if tier := poolTier(); tier != 5 {
		t.Fatalf("tier after 3 swaps = %d, want still 夯", tier)
	}

	// 第 4 次被换 → 降到顶尖
	state = beginMealState(t, app, cookie)
	reroll(&state)
	if tier := poolTier(); tier != 4 {
		t.Fatalf("tier after 4th swap = %d, want 顶尖", tier)
	}

	// 接受清零：先攒 2 次被换再接受，之后 3 次被换不够触发
	reroll(&state)
	reroll(&state)
	acceptMealDecision(t, app, cookie, state.Decision, 1)
	state = beginMealState(t, app, cookie)
	for range 3 {
		reroll(&state)
	}
	abandonActiveMeal(t, app, cookie)
	if tier := poolTier(); tier != 4 {
		t.Fatalf("tier after accept-reset and 3 swaps = %d, want still 顶尖", tier)
	}

	// 再凑满 4 次 → 人上人；随后任何连换都不再往下降（地板）
	state = beginMealState(t, app, cookie)
	reroll(&state)
	if tier := poolTier(); tier != 3 {
		t.Fatalf("tier after next 4th swap = %d, want 人上人", tier)
	}
	for range 2 {
		reroll(&state)
	}
	abandonActiveMeal(t, app, cookie)
	state = beginMealState(t, app, cookie)
	for range 2 {
		reroll(&state)
	}
	abandonActiveMeal(t, app, cookie)
	if tier := poolTier(); tier != 3 {
		t.Errorf("tier after further swaps = %d, want floor 人上人", tier)
	}
}

// ---- 探索臂 ----

func TestDiscoveryDrawsFromTasteProfileSimilarityWithReason(t *testing.T) {
	app := openCatalogAppWithDiscovery(t, server.DiscoveryConfig{
		Enabled:               true,
		MaxPoolSize:           1,
		MaxEligibleDishes:     1,
		MinRerolls:            99,
		RecentMealWindow:      3,
		MaxDiscoveriesPerMeal: 2,
	}, 1)
	t.Cleanup(func() { app.Close() })
	cookie := registerCandidateEater(t, app, "discovery_similarity_eater")
	addCandidatePoolDish(t, app, cookie, tomatoEgg, 5)

	discovery := beginDiscoveryDecision(t, app, cookie)
	// 口味画像相似度：与番茄炒蛋共享主料「番茄」的菜才有非零权重
	similar := map[string]bool{
		"vegetable_dish/番茄豆腐.md": true,
		"vegetable_dish/番茄土豆.md": true,
		tomatoBeef:               true,
	}
	if !similar[discovery.Dish.ID] {
		t.Errorf("Discovery Dish = %q, want a Dish sharing 番茄 with 番茄炒蛋", discovery.Dish.ID)
	}
	if !strings.Contains(discovery.Reason, "番茄炒蛋") ||
		!strings.Contains(discovery.Reason, "试试新的") {
		t.Errorf("Discovery reason = %q, want reference Dish and 试试新的", discovery.Reason)
	}
}

// ---- 轻历史与补评分 ----

func TestHistoryListsRecordsAndBackfillRatingNeverBlocks(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	cookie := registerCandidateEater(t, app, "history_eater")
	addCandidatePoolDish(t, app, cookie, tomatoEgg, 4)
	addCandidatePoolDish(t, app, cookie, tomatoBeef, 4)

	first := beginMealDecision(t, app, cookie)
	acceptMealDecision(t, app, cookie, first, 1)
	second := beginMealDecision(t, app, cookie)
	acceptMealDecision(t, app, cookie, second, 2)

	list := candidatePoolRequest(t, app, http.MethodGet, "/api/eating-records", "", cookie)
	if list.Code != http.StatusOK {
		t.Fatalf("history status = %d, want %d; body = %s", list.Code, http.StatusOK, list.Body)
	}
	var history struct {
		Records []struct {
			ID       int64 `json:"id"`
			Sequence int64 `json:"sequence"`
			Dish     struct {
				ID string `json:"id"`
			} `json:"dish"`
			Mode       string `json:"mode"`
			AcceptedAt int64  `json:"accepted_at"`
			Rating     *int   `json:"rating"`
			PoolTier   *int   `json:"pool_tier"`
		} `json:"records"`
	}
	if err := json.NewDecoder(list.Body).Decode(&history); err != nil {
		t.Fatal(err)
	}
	if len(history.Records) != 2 ||
		history.Records[0].Sequence != 2 ||
		history.Records[1].Sequence != 1 ||
		history.Records[0].Mode != "pool" ||
		history.Records[0].Rating != nil ||
		history.Records[0].AcceptedAt == 0 ||
		history.Records[0].PoolTier == nil {
		t.Fatalf("history = %#v, want two pool records newest first with pool tiers", history.Records)
	}

	// 补评分：升到夯
	latest := history.Records[0]
	ratePath := "/api/eating-records/" + strconv.FormatInt(latest.ID, 10) + "/rate"
	rated := candidatePoolRequest(t, app, http.MethodPost, ratePath, `{"rating":5}`, cookie)
	if rated.Code != http.StatusOK {
		t.Fatalf("backfill rating status = %d, want %d; body = %s", rated.Code, http.StatusOK, rated.Body)
	}
	var result tasteRatingResult
	if err := json.NewDecoder(rated.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Outcome != "pool_admission" || result.Tier == nil || *result.Tier != 5 {
		t.Errorf("backfill rating = %#v, want pool admission at 夯", result)
	}

	// 幂等与冲突
	repeat := candidatePoolRequest(t, app, http.MethodPost, ratePath, `{"rating":5}`, cookie)
	if repeat.Code != http.StatusOK {
		t.Errorf("repeated backfill status = %d, want %d", repeat.Code, http.StatusOK)
	}
	conflict := candidatePoolRequest(t, app, http.MethodPost, ratePath, `{"rating":3}`, cookie)
	if conflict.Code != http.StatusConflict {
		t.Errorf("conflicting backfill status = %d, want %d", conflict.Code, http.StatusConflict)
	}

	// 低分补评分 → 拒绝标记；但绝不产生拦截
	older := history.Records[1]
	rejected := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/eating-records/"+strconv.FormatInt(older.ID, 10)+"/rate",
		`{"rating":2}`,
		cookie,
	)
	if rejected.Code != http.StatusOK {
		t.Fatalf("low backfill status = %d, want %d; body = %s", rejected.Code, http.StatusOK, rejected.Body)
	}
	resume := candidatePoolRequest(t, app, http.MethodGet, "/api/meals/resume", "", cookie)
	if !strings.Contains(resume.Body.String(), `"status":"ready"`) {
		t.Errorf("Resume after backfill ratings = %q, want ready (never a gate)", resume.Body)
	}
}

// ---- 账号隔离与空池 ----

func TestPoolDecisionSelectionIsIsolatedByAccount(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	firstCookie := registerCandidateEater(t, app, "first_meal_eater")
	secondCookie := registerCandidateEater(t, app, "second_meal_eater")

	addCandidatePoolDish(t, app, firstCookie, winterSoup, 4)
	addCandidatePoolDish(t, app, secondCookie, tomatoEgg, 4)

	for _, fixture := range []struct {
		cookie *http.Cookie
		dishID string
	}{
		{firstCookie, winterSoup},
		{secondCookie, tomatoEgg},
	} {
		decision := beginMealDecision(t, app, fixture.cookie)
		if decision.Dish.ID != fixture.dishID {
			t.Errorf("Decision Dish = %q, want own Account Dish %q", decision.Dish.ID, fixture.dishID)
		}
	}
}

func TestEatingHistoryIsAccountScoped(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	historyOwner := registerCandidateEater(t, app, "history_owner")
	otherCookie := registerCandidateEater(t, app, "history_other")

	addCandidatePoolDish(t, app, historyOwner, tomatoEgg, 4)
	acceptMealDecision(t, app, historyOwner, beginMealDecision(t, app, historyOwner), 1)

	// 他人的冷却不影响本账号
	addCandidatePoolDish(t, app, otherCookie, tomatoEgg, 4)
	decision := beginMealDecision(t, app, otherCookie)
	if decision.Dish.ID != tomatoEgg {
		t.Errorf("other Account Decision = %q, want %q despite owner's cooldown", decision.Dish.ID, tomatoEgg)
	}
}

func TestCandidatePoolEmptyBlocksDecisionWithoutCreatingMeal(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCatalogEater(t, app)

	for range 2 {
		beginResponse := candidatePoolRequest(t, app, http.MethodPost, "/api/meals", "", sessionCookie)
		if beginResponse.Code != http.StatusConflict {
			t.Fatalf("begin status = %d, want %d; body = %s", beginResponse.Code, http.StatusConflict, beginResponse.Body)
		}
		if code := decodeWireError(t, strings.NewReader(beginResponse.Body.String())); code != "candidate_pool_empty" {
			t.Errorf("blocked Decision code = %q, want candidate_pool_empty", code)
		}
	}

	resumeResponse := candidatePoolRequest(t, app, http.MethodGet, "/api/meals/resume", "", sessionCookie)
	if resumeResponse.Code != http.StatusOK ||
		!strings.Contains(resumeResponse.Body.String(), `"status":"candidate_pool_empty"`) {
		t.Errorf("Resume after blocked attempts = (%d, %q), want candidate_pool_empty", resumeResponse.Code, resumeResponse.Body)
	}
}

func TestRerollPreservesFreshnessCooldown(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	cookie := registerCandidateEater(t, app, "reroll_cooldown_eater")

	addCandidatePoolDish(t, app, cookie, tomatoEgg, 4)
	acceptMealDecision(t, app, cookie, beginMealDecision(t, app, cookie), 1)
	addCandidatePoolDish(t, app, cookie, tomatoBeef, 4)
	addCandidatePoolDish(t, app, cookie, winterSoup, 4)

	current := beginMealDecision(t, app, cookie)
	if current.Dish.ID == tomatoEgg {
		t.Fatalf("Begin Dish = %q, want accepted Dish to stay in cooldown", tomatoEgg)
	}
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
		t.Errorf("Reroll Dish = %q, want accepted Dish to remain in cooldown", tomatoEgg)
	}
	if replacement.Decision.Dish.ID == current.Dish.ID {
		t.Errorf("Reroll Dish = %q, want the other eligible Dish", current.Dish.ID)
	}
}

// ---- 助手 ----

func openCatalogAppWithDiscovery(
	t *testing.T,
	discovery server.DiscoveryConfig,
	seed int64,
) *server.App {
	t.Helper()
	app, err := server.NewWithDecisionRandomSeedForTest(testConfig(t, "", &discovery), seed)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func addCandidatePoolDish(t *testing.T, app http.Handler, cookie *http.Cookie, dishID string, tier int) {
	t.Helper()
	response := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/candidate-pool/dishes",
		`{"dish_id":`+strconv.Quote(dishID)+`,"tier":`+strconv.Itoa(tier)+`}`,
		cookie,
	)
	if response.Code != http.StatusCreated {
		t.Fatalf("add Dish %q status = %d, want %d; body = %s", dishID, response.Code, http.StatusCreated, response.Body)
	}
}

func updateCandidatePoolDish(t *testing.T, app http.Handler, cookie *http.Cookie, dishID string, tier int) {
	t.Helper()
	response := candidatePoolRequest(
		t,
		app,
		http.MethodPatch,
		"/api/candidate-pool/dishes",
		`{"dish_id":`+strconv.Quote(dishID)+`,"tier":`+strconv.Itoa(tier)+`}`,
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

func beginMealState(t *testing.T, app http.Handler, cookie *http.Cookie) mealState {
	t.Helper()
	response := candidatePoolRequest(t, app, http.MethodPost, "/api/meals", "", cookie)
	if response.Code != http.StatusCreated {
		t.Fatalf("Begin status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body)
	}
	var result mealState
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func beginMealStateAt(t *testing.T, app http.Handler, cookie *http.Cookie, hour int) mealState {
	t.Helper()
	response := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/meals",
		`{"local_hour":`+strconv.Itoa(hour)+`}`,
		cookie,
	)
	if response.Code != http.StatusCreated {
		t.Fatalf("Begin at %d:00 status = %d, want %d; body = %s", hour, response.Code, http.StatusCreated, response.Body)
	}
	var result mealState
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func beginMealDecision(t *testing.T, app http.Handler, cookie *http.Cookie) mealDecision {
	t.Helper()
	return beginMealState(t, app, cookie).Decision
}

func abandonActiveMeal(t *testing.T, app http.Handler, cookie *http.Cookie) {
	t.Helper()
	response := candidatePoolRequest(t, app, http.MethodPost, "/api/meals/abandon", "", cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("abandon status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body)
	}
}

// beginDiscoveryDecision 反复开顿直到探索臂命中（概率触发，ADR-0022）；
// 撞到 pool 揭示就放弃这顿重来。60 次全 miss 的概率在信号 ≥2 时低于 1e-18。
func beginDiscoveryDecision(t *testing.T, app http.Handler, cookie *http.Cookie) mealDecision {
	t.Helper()
	for range 60 {
		decision := beginMealDecision(t, app, cookie)
		if decision.Mode == "discovery" {
			return decision
		}
		abandonActiveMeal(t, app, cookie)
	}
	t.Fatal("Discovery never triggered in 60 attempts")
	return mealDecision{}
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

func rerollMealDecision(
	t *testing.T,
	app http.Handler,
	cookie *http.Cookie,
	current mealDecision,
) mealDecision {
	t.Helper()
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
	var result mealState
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result.Decision
}
