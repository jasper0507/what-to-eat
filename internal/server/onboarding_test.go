package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jasper0507/what-to-eat/internal/server"
)

func openOnboardingApp(
	t *testing.T,
	databasePath string,
	steps []server.ScriptedNIMStep,
) *server.App {
	t.Helper()
	app, err := server.NewWithScriptedNIMForTest(server.Config{
		DatabasePath: databasePath,
		CatalogDir:   filepath.Join("testdata", "catalog"),
		Discovery:    &server.DiscoveryConfig{Enabled: false},
	}, steps)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func TestOnboardingInterviewRestoresProgressAndBuildsCandidatePool(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "what-to-eat.db")
	app := openOnboardingApp(t, databasePath, []server.ScriptedNIMStep{{
		Reply: "番茄口味记下了。还有哪些具体菜名是你常吃的？",
	}})
	sessionCookie := registerCandidateEater(t, app, "onboarding_eater")

	first := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/onboarding/interview/messages",
		`{"message":"我喜欢番茄炒蛋，也常吃番茄牛腩"}`,
		sessionCookie,
	)
	if first.Code != http.StatusOK {
		t.Fatalf("first message status = %d, want %d; body = %s", first.Code, http.StatusOK, first.Body)
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	app = openOnboardingApp(t, databasePath, []server.ScriptedNIMStep{{
		Reply:    "好了，已经按你的偏好建立 Candidate pool。",
		Complete: true,
		Preferences: map[string]float64{
			"番茄炒蛋": 5,
			"番茄牛腩": 3.5,
			"并不存在": 5,
		},
	}})
	t.Cleanup(func() { app.Close() })

	resume := candidatePoolRequest(
		t,
		app,
		http.MethodGet,
		"/api/onboarding/interview",
		"",
		sessionCookie,
	)
	if resume.Code != http.StatusOK {
		t.Fatalf("resume status = %d, want %d; body = %s", resume.Code, http.StatusOK, resume.Body)
	}
	var resumed struct {
		Status   string `json:"status"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resume.Body).Decode(&resumed); err != nil {
		t.Fatal(err)
	}
	if resumed.Status != "in_progress" || len(resumed.Messages) != 2 {
		t.Fatalf("resumed interview = %#v, want two persisted messages in progress", resumed)
	}
	if resumed.Messages[0].Role != "user" ||
		resumed.Messages[0].Content != "我喜欢番茄炒蛋，也常吃番茄牛腩" ||
		resumed.Messages[1].Role != "assistant" {
		t.Errorf("resumed messages = %#v, want original user and assistant conversation", resumed.Messages)
	}

	completed := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/onboarding/interview/messages",
		`{"message":"就这两道；番茄炒蛋最喜欢"}`,
		sessionCookie,
	)
	if completed.Code != http.StatusOK {
		t.Fatalf(
			"completion status = %d, want %d; body = %s",
			completed.Code,
			http.StatusOK,
			completed.Body,
		)
	}
	var completionState struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(completed.Body).Decode(&completionState); err != nil {
		t.Fatal(err)
	}
	if completionState.Status != "completed" {
		t.Errorf("completion status = %q, want completed", completionState.Status)
	}

	pool := candidatePoolRequest(
		t,
		app,
		http.MethodGet,
		"/api/candidate-pool/dishes",
		"",
		sessionCookie,
	)
	if pool.Code != http.StatusOK {
		t.Fatalf("pool status = %d, want %d; body = %s", pool.Code, http.StatusOK, pool.Body)
	}
	var result struct {
		Dishes []candidateDish `json:"dishes"`
	}
	if err := json.NewDecoder(pool.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Dishes) != 2 {
		t.Fatalf("Candidate pool = %#v, want only two stable Catalog dishes", result.Dishes)
	}
	weights := map[string]float64{}
	for _, dish := range result.Dishes {
		weights[dish.ID] = dish.PreferenceWeight
	}
	if weights["vegetable_dish/番茄炒蛋.md"] != 5 ||
		weights["meat_dish/番茄牛腩.md"] != 3.5 {
		t.Errorf("Candidate pool weights = %#v, want stable Catalog IDs with NIM weights", weights)
	}
}

func TestOnboardingNIMFailureCanRetryOrFallBackToManualPool(t *testing.T) {
	app := openOnboardingApp(t, filepath.Join(t.TempDir(), "what-to-eat.db"), []server.ScriptedNIMStep{
		{Error: "temporary NIM outage"},
		{
			Reply:       "恢复了，Candidate pool 已建立。",
			Complete:    true,
			Preferences: map[string]float64{"番茄炒蛋": 4},
		},
		{Error: "NIM timeout"},
	})
	t.Cleanup(func() { app.Close() })

	retryCookie := registerCandidateEater(t, app, "retry_eater")
	failed := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/onboarding/interview/messages",
		`{"message":"我喜欢番茄炒蛋"}`,
		retryCookie,
	)
	if failed.Code != http.StatusServiceUnavailable {
		t.Fatalf("failed status = %d, want %d; body = %s", failed.Code, http.StatusServiceUnavailable, failed.Body)
	}

	resume := candidatePoolRequest(
		t,
		app,
		http.MethodGet,
		"/api/onboarding/interview",
		"",
		retryCookie,
	)
	var failedState struct {
		Status   string `json:"status"`
		CanRetry bool   `json:"can_retry"`
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resume.Body).Decode(&failedState); err != nil {
		t.Fatal(err)
	}
	if failedState.Status != "failed" || !failedState.CanRetry ||
		len(failedState.Messages) != 1 || failedState.Messages[0].Role != "user" {
		t.Errorf("failed state = %#v, want recoverable persisted user message", failedState)
	}

	retried := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/onboarding/interview/retry",
		"",
		retryCookie,
	)
	if retried.Code != http.StatusOK || !strings.Contains(retried.Body.String(), `"status":"completed"`) {
		t.Errorf("retry response = (%d, %q), want completed", retried.Code, retried.Body)
	}

	manualCookie := registerCandidateEater(t, app, "manual_eater")
	manualFailure := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/onboarding/interview/messages",
		`{"message":"我喜欢柠檬水"}`,
		manualCookie,
	)
	if manualFailure.Code != http.StatusServiceUnavailable {
		t.Fatalf("manual failure status = %d, want %d", manualFailure.Code, http.StatusServiceUnavailable)
	}
	fallback := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/onboarding/interview/manual",
		"",
		manualCookie,
	)
	if fallback.Code != http.StatusOK ||
		!strings.Contains(fallback.Body.String(), `"status":"manual"`) {
		t.Errorf("manual fallback = (%d, %q), want manual state", fallback.Code, fallback.Body)
	}
	pool := candidatePoolRequest(
		t,
		app,
		http.MethodGet,
		"/api/candidate-pool/dishes",
		"",
		manualCookie,
	)
	if pool.Code != http.StatusOK || pool.Body.String() != `{"dishes":[]}` {
		t.Errorf("manual Candidate pool = (%d, %q), want immediately visible empty pool", pool.Code, pool.Body)
	}
}

func TestOnboardingNIMCallsAreRateLimitedPerAccount(t *testing.T) {
	steps := make([]server.ScriptedNIMStep, 0, 11)
	for range 11 {
		steps = append(steps, server.ScriptedNIMStep{Reply: "再告诉我一道喜欢的菜。"})
	}
	app := openOnboardingApp(t, filepath.Join(t.TempDir(), "what-to-eat.db"), steps)
	t.Cleanup(func() { app.Close() })
	firstCookie := registerCandidateEater(t, app, "limited_eater")
	secondCookie := registerCandidateEater(t, app, "other_eater")

	for attempt := 1; attempt <= 10; attempt++ {
		response := candidatePoolRequest(
			t,
			app,
			http.MethodPost,
			"/api/onboarding/interview/messages",
			`{"message":"我还在想，请继续问我"}`,
			firstCookie,
		)
		if response.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d, want %d; body = %s", attempt, response.Code, http.StatusOK, response.Body)
		}
	}
	blocked := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/onboarding/interview/messages",
		`{"message":"第十一次调用"}`,
		firstCookie,
	)
	if blocked.Code != http.StatusTooManyRequests {
		t.Errorf("blocked status = %d, want %d", blocked.Code, http.StatusTooManyRequests)
	}

	otherAccount := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/onboarding/interview/messages",
		`{"message":"我喜欢番茄炒蛋"}`,
		secondCookie,
	)
	if otherAccount.Code != http.StatusOK {
		t.Errorf("other Account status = %d, want %d", otherAccount.Code, http.StatusOK)
	}
}

func TestSlowOnboardingCallDoesNotBlockAnotherAccount(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	app := openOnboardingApp(t, filepath.Join(t.TempDir(), "what-to-eat.db"), []server.ScriptedNIMStep{{
		Reply:   "继续聊聊。",
		Started: started,
		Release: release,
	}})
	t.Cleanup(func() { app.Close() })
	slowCookie := registerCandidateEater(t, app, "slow_eater")
	otherCookie := registerCandidateEater(t, app, "unblocked_eater")

	slowDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/onboarding/interview/messages",
			bytes.NewBufferString(`{"message":"我喜欢番茄炒蛋"}`),
		)
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(slowCookie)
		response := httptest.NewRecorder()
		app.ServeHTTP(response, request)
		slowDone <- response
	}()
	<-started

	otherDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		request := httptest.NewRequest(http.MethodGet, "/api/onboarding/interview", nil)
		request.AddCookie(otherCookie)
		response := httptest.NewRecorder()
		app.ServeHTTP(response, request)
		otherDone <- response
	}()

	select {
	case response := <-otherDone:
		if response.Code != http.StatusOK {
			t.Errorf("other Account status = %d, want %d", response.Code, http.StatusOK)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("slow NIM call blocked another Account")
	}
	close(release)
	if response := <-slowDone; response.Code != http.StatusOK {
		t.Errorf("slow Account status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestNIMAPIKeyStaysAtTheServerBoundary(t *testing.T) {
	const apiKey = "test-nvidia-secret"
	var authorization, providerRequest string
	nim := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		providerRequest = string(body)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"choices":[{
				"message":{
					"content":"{\"reply\":\"已经记下。\",\"complete\":true,\"preferences\":[{\"dish_name\":\"柠檬水\",\"weight\":4}]}"
				}
			}]
		}`))
	}))
	defer nim.Close()

	app, err := server.New(server.Config{
		DatabasePath: filepath.Join(t.TempDir(), "what-to-eat.db"),
		CatalogDir:   filepath.Join("testdata", "catalog"),
		NIM: &server.NIMConfig{
			APIKey:  apiKey,
			BaseURL: nim.URL + "/v1",
			Model:   "test-model",
			Timeout: time.Second,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	sessionCookie := registerCandidateEater(t, app, "secret_eater")

	response := candidatePoolRequest(
		t,
		app,
		http.MethodPost,
		"/api/onboarding/interview/messages",
		`{"message":"我喜欢柠檬水"}`,
		sessionCookie,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("message status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body)
	}
	if authorization != "Bearer "+apiKey {
		t.Errorf("Authorization = %q, want server-side bearer key", authorization)
	}
	if strings.Contains(providerRequest, apiKey) {
		t.Error("NIM request body contains API key")
	}
	if strings.Contains(response.Body.String(), apiKey) {
		t.Error("public response contains API key")
	}
}

func TestNIMAPIKeyRejectsInsecureRemoteEndpoint(t *testing.T) {
	app, err := server.New(server.Config{
		DatabasePath: filepath.Join(t.TempDir(), "what-to-eat.db"),
		NIM: &server.NIMConfig{
			APIKey:  "must-not-cross-plaintext-http",
			BaseURL: "http://nim.example.com/v1",
		},
	})
	if err == nil {
		app.Close()
		t.Fatal("New accepted a non-loopback HTTP NIM endpoint")
	}
	if !strings.Contains(err.Error(), "must use HTTPS") {
		t.Errorf("New error = %q, want HTTPS requirement", err)
	}
}
