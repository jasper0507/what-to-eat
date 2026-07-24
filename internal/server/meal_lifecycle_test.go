package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestResumeReflectsCandidatePoolReadiness(t *testing.T) {
	app := openCatalogApp(t, "")
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCatalogEater(t, app)

	resume := func() struct {
		Status  string `json:"status"`
		Actions []struct {
			Kind string `json:"kind"`
			Href string `json:"href"`
		} `json:"actions"`
	} {
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
			Status  string `json:"status"`
			Actions []struct {
				Kind string `json:"kind"`
				Href string `json:"href"`
			} `json:"actions"`
		}
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}

	empty := resume()
	if empty.Status != "candidate_pool_empty" ||
		len(empty.Actions) != 1 ||
		empty.Actions[0].Kind != "catalog_search" ||
		empty.Actions[0].Href != "/candidate-pool" {
		t.Errorf("empty Resume = %#v, want candidate_pool_empty with Catalog search entry", empty)
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
	if ready.Status != "ready" || len(ready.Actions) != 0 {
		t.Errorf("ready Resume = %#v, want ready without recovery actions", ready)
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
		var blocked struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
			Actions []struct {
				Kind string `json:"kind"`
				Href string `json:"href"`
			} `json:"actions"`
		}
		if err := json.NewDecoder(beginResponse.Body).Decode(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked.Error.Code != "candidate_pool_empty" ||
			len(blocked.Actions) != 1 ||
			blocked.Actions[0].Kind != "catalog_search" ||
			blocked.Actions[0].Href != "/candidate-pool" {
			t.Errorf("blocked Decision = %#v, want Candidate pool error and Catalog entry", blocked)
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
