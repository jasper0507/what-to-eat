package server_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/jasper0507/what-to-eat/internal/server"
)

func TestSQLiteBackupRestoresEntireEaterJourneyState(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "what-to-eat.db")
	app := openPersistentApp(t, sourcePath)
	sessionCookie := registerCandidateEater(t, app, "restored_eater")
	addCandidatePoolDish(t, app, sessionCookie, "vegetable_dish/番茄炒蛋.md", 5)

	decision := beginMealDecision(t, app, sessionCookie)
	if decision.Mode != "discovery" {
		t.Fatalf("Decision mode = %q, want discovery", decision.Mode)
	}
	accepted := acceptDecisionResult(t, app, sessionCookie, decision)
	if accepted.EatingRecord.Sequence != 1 || accepted.PendingRating == nil {
		t.Fatalf("Acceptance = %#v, want first Eating record and Pending rating", accepted)
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	restoredPath := filepath.Join(t.TempDir(), "what-to-eat.db")
	backup, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(restoredPath, backup, 0o600); err != nil {
		t.Fatal(err)
	}

	app = openPersistentApp(t, restoredPath)
	session := candidatePoolRequest(
		t,
		app,
		http.MethodGet,
		"/api/auth/session",
		"",
		sessionCookie,
	)
	if session.Code != http.StatusOK {
		t.Fatalf("restored session status = %d, want %d; body = %s", session.Code, http.StatusOK, session.Body)
	}
	resume := candidatePoolRequest(
		t,
		app,
		http.MethodGet,
		"/api/meals/resume",
		"",
		sessionCookie,
	)
	var state mealState
	if err := json.NewDecoder(resume.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	if state.Status != "pending_ratings" ||
		len(state.PendingRatings) != 1 ||
		state.PendingRatings[0].Dish.ID != decision.Dish.ID {
		t.Fatalf("restored Meal state = %#v, want original Pending rating", state)
	}
	rated := ratePending(t, app, sessionCookie, state.PendingRatings[0].ID, 5)
	if rated.Outcome != "pool_admission" {
		t.Fatalf("Taste rating outcome = %q, want pool_admission", rated.Outcome)
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	app = openPersistentApp(t, restoredPath)
	t.Cleanup(func() { app.Close() })
	resume = candidatePoolRequest(
		t,
		app,
		http.MethodGet,
		"/api/meals/resume",
		"",
		sessionCookie,
	)
	state = mealState{}
	if err := json.NewDecoder(resume.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	if state.Status != "ready" {
		t.Fatalf("Meal state after restored Taste rating = %#v, want ready", state)
	}
	pool := candidatePoolRequest(
		t,
		app,
		http.MethodGet,
		"/api/candidate-pool/dishes",
		"",
		sessionCookie,
	)
	var poolState struct {
		Dishes []candidateDish `json:"dishes"`
	}
	if err := json.NewDecoder(pool.Body).Decode(&poolState); err != nil {
		t.Fatal(err)
	}
	if len(poolState.Dishes) != 2 {
		t.Fatalf("restored Candidate pool = %#v, want original and rated Discovery Dish", poolState.Dishes)
	}
}

func openPersistentApp(t *testing.T, databasePath string) *server.App {
	t.Helper()
	app, err := server.NewWithDecisionRandomSeedForTest(server.Config{
		DatabasePath:  databasePath,
		SessionSecret: testSessionSecret,
		CatalogDir:    filepath.Join("testdata", "catalog"),
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	return app
}
