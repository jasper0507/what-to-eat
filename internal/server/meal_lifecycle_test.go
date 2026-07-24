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
