package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jasper0507/what-to-eat/internal/server"
)

var testSessionSecret = []byte("test-session-secret-must-be-at-least-32-bytes")

func TestServerRejectsMissingSessionSecret(t *testing.T) {
	app, err := server.New(server.Config{
		DatabasePath: filepath.Join(t.TempDir(), "what-to-eat.db"),
	})
	if app != nil {
		t.Cleanup(func() { app.Close() })
	}
	if err == nil || !strings.Contains(err.Error(), "SessionSecret") {
		t.Fatalf("New error = %v, want missing SessionSecret error", err)
	}
}

func TestServerRejectsMissingDatabasePath(t *testing.T) {
	app, err := server.New(server.Config{SessionSecret: testSessionSecret})
	if app != nil {
		t.Cleanup(func() { app.Close() })
	}
	if err == nil || !strings.Contains(err.Error(), "DatabasePath") {
		t.Fatalf("New error = %v, want missing DatabasePath error", err)
	}
}

func TestServerReportsDatabaseOpenFailure(t *testing.T) {
	databasePath := t.TempDir()
	app, err := server.New(server.Config{
		DatabasePath:  databasePath,
		SessionSecret: testSessionSecret,
	})
	if app != nil {
		t.Cleanup(func() { app.Close() })
	}
	if err == nil ||
		!strings.Contains(err.Error(), "initialize SQLite database") ||
		!strings.Contains(err.Error(), databasePath) {
		t.Fatalf("New error = %v, want explicit SQLite path failure", err)
	}
}

func newTestApp(t *testing.T, secureCookies bool) *server.App {
	t.Helper()
	app, err := server.New(server.Config{
		DatabasePath:  filepath.Join(t.TempDir(), "what-to-eat.db"),
		SessionSecret: testSessionSecret,
		SecureCookies: secureCookies,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	return app
}

func doRequest(
	t *testing.T,
	app *server.App,
	method string,
	path string,
	body string,
	cookies ...*http.Cookie,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	return response
}

func TestRegistrationCreatesSecureRestorableSession(t *testing.T) {
	app := newTestApp(t, true)
	registerResponse := doRequest(
		t,
		app,
		http.MethodPost,
		"/api/auth/register",
		`{"username":"小明同学","password":"correct horse battery staple"}`,
	)
	if registerResponse.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d", registerResponse.Code, http.StatusCreated)
	}

	cookies := registerResponse.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("session cookies = %d, want 1", len(cookies))
	}
	sessionCookie := cookies[0]
	if !sessionCookie.HttpOnly || !sessionCookie.Secure || sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("unsafe session cookie attributes: %#v", sessionCookie)
	}
	if sessionCookie.Path != "/" {
		t.Errorf("session cookie path = %q, want /", sessionCookie.Path)
	}

	registerBody := registerResponse.Body.String()
	for _, secret := range []string{"correct horse battery staple", sessionCookie.Value, "password", "hash"} {
		if strings.Contains(strings.ToLower(registerBody), strings.ToLower(secret)) {
			t.Errorf("register response contains secret %q", secret)
		}
	}
	var registered struct {
		Account struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"account"`
	}
	if err := json.Unmarshal([]byte(registerBody), &registered); err != nil {
		t.Fatal(err)
	}
	if registered.Account.ID == 0 || registered.Account.Username != "小明同学" {
		t.Errorf("registered Account = %#v, want 小明同学 with a non-zero ID", registered.Account)
	}

	sessionResponse := doRequest(t, app, http.MethodGet, "/api/auth/session", "", sessionCookie)
	if sessionResponse.Code != http.StatusOK {
		t.Fatalf(
			"session status = %d, want %d; body = %s",
			sessionResponse.Code,
			http.StatusOK,
			sessionResponse.Body,
		)
	}
	var result struct {
		Account struct {
			Username string `json:"username"`
		} `json:"account"`
	}
	if err := json.NewDecoder(sessionResponse.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Account.Username != "小明同学" {
		t.Errorf("username = %q, want 小明同学", result.Account.Username)
	}
}

func TestEaterCanLogin(t *testing.T) {
	app := newTestApp(t, false)
	registerResponse := doRequest(
		t,
		app,
		http.MethodPost,
		"/api/auth/register",
		`{"username":"returning_eater","password":"correct horse battery staple"}`,
	)
	if registerResponse.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d", registerResponse.Code, http.StatusCreated)
	}

	loginResponse := doRequest(
		t,
		app,
		http.MethodPost,
		"/api/auth/login",
		`{"username":"returning_eater","password":"correct horse battery staple"}`,
	)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d; body = %s", loginResponse.Code, http.StatusOK, loginResponse.Body)
	}
	if len(loginResponse.Result().Cookies()) != 1 {
		t.Fatalf("session cookies = %d, want 1", len(loginResponse.Result().Cookies()))
	}
	var result struct {
		Account struct {
			Username string `json:"username"`
		} `json:"account"`
	}
	if err := json.NewDecoder(loginResponse.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Account.Username != "returning_eater" {
		t.Errorf("username = %q, want returning_eater", result.Account.Username)
	}
}

func TestInvalidLoginIsNonDisclosingAndRateLimited(t *testing.T) {
	app := newTestApp(t, false)
	registerResponse := doRequest(
		t,
		app,
		http.MethodPost,
		"/api/auth/register",
		`{"username":"known_eater","password":"correct horse battery staple"}`,
	)
	if registerResponse.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d", registerResponse.Code, http.StatusCreated)
	}

	login := func(username, password string) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(map[string]string{"username": username, "password": password})
		if err != nil {
			t.Fatal(err)
		}
		return doRequest(t, app, http.MethodPost, "/api/auth/login", string(body))
	}

	wrongPassword := login("known_eater", "definitely wrong")
	unknownUsername := login("unknown_eater", "definitely wrong")
	if wrongPassword.Code != http.StatusUnauthorized || unknownUsername.Code != http.StatusUnauthorized {
		t.Fatalf(
			"invalid statuses = (%d, %d), want both %d",
			wrongPassword.Code,
			unknownUsername.Code,
			http.StatusUnauthorized,
		)
	}
	if wrongPassword.Body.String() != unknownUsername.Body.String() {
		t.Errorf(
			"known and unknown Account errors differ: %q != %q",
			wrongPassword.Body.String(),
			unknownUsername.Body.String(),
		)
	}
	if len(wrongPassword.Result().Cookies()) != 0 || len(unknownUsername.Result().Cookies()) != 0 {
		t.Error("invalid login must not issue a session cookie")
	}

	for range 2 {
		login("unknown_eater", "definitely wrong")
	}
	successfulLogin := login("known_eater", "correct horse battery staple")
	if successfulLogin.Code != http.StatusOK {
		t.Fatalf("successful login status = %d, want %d", successfulLogin.Code, http.StatusOK)
	}
	fifthFailure := login("unknown_eater", "definitely wrong")
	if fifthFailure.Code != http.StatusUnauthorized {
		t.Fatalf("fifth invalid login status = %d, want %d", fifthFailure.Code, http.StatusUnauthorized)
	}
	rateLimited := login("known_eater", "definitely wrong")
	if rateLimited.Code != http.StatusTooManyRequests {
		t.Errorf("sixth invalid login status = %d, want %d", rateLimited.Code, http.StatusTooManyRequests)
	}
}

func TestAccountSessionIsRequiredAndIsolated(t *testing.T) {
	app := newTestApp(t, false)
	register := func(username string) (*http.Cookie, int64) {
		t.Helper()
		body, err := json.Marshal(map[string]string{
			"username": username,
			"password": "correct horse battery staple",
		})
		if err != nil {
			t.Fatal(err)
		}
		response := doRequest(t, app, http.MethodPost, "/api/auth/register", string(body))
		if response.Code != http.StatusCreated {
			t.Fatalf("register %q status = %d, want %d", username, response.Code, http.StatusCreated)
		}
		var result struct {
			Account struct {
				ID int64 `json:"id"`
			} `json:"account"`
		}
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return response.Result().Cookies()[0], result.Account.ID
	}

	firstCookie, firstID := register("first_eater")
	secondCookie, _ := register("second_eater")

	unauthenticatedResponse := doRequest(t, app, http.MethodGet, "/api/auth/session", "")
	if unauthenticatedResponse.Code != http.StatusUnauthorized {
		t.Errorf(
			"unauthenticated status = %d, want %d",
			unauthenticatedResponse.Code,
			http.StatusUnauthorized,
		)
	}

	secondResponse := doRequest(
		t,
		app,
		http.MethodGet,
		"/api/auth/session?account_id="+strconv.FormatInt(firstID, 10),
		"",
		secondCookie,
	)
	if secondResponse.Code != http.StatusOK {
		t.Fatalf("second Account status = %d, want %d", secondResponse.Code, http.StatusOK)
	}
	var result struct {
		Account struct {
			Username string `json:"username"`
		} `json:"account"`
	}
	if err := json.NewDecoder(secondResponse.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Account.Username != "second_eater" {
		t.Errorf("session resolved username = %q, want second_eater", result.Account.Username)
	}

	firstResponse := doRequest(t, app, http.MethodGet, "/api/auth/session", "", firstCookie)
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("first Account status = %d, want %d", firstResponse.Code, http.StatusOK)
	}
}

func TestRegistrationRejectsInvalidAndDuplicateCredentialsWithoutLeaks(t *testing.T) {
	app := newTestApp(t, false)
	register := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		return doRequest(t, app, http.MethodPost, "/api/auth/register", body)
	}

	shortUsername := register(`{"username":"ab","password":"long enough password"}`)
	shortPassword := register(`{"username":"valid_eater","password":"short"}`)
	shortUnicodePassword := register(`{"username":"valid_eater","password":"饭饭饭"}`)
	if shortUsername.Code != http.StatusBadRequest ||
		shortPassword.Code != http.StatusBadRequest ||
		shortUnicodePassword.Code != http.StatusBadRequest {
		t.Fatalf(
			"invalid statuses = (%d, %d, %d), want all %d",
			shortUsername.Code,
			shortPassword.Code,
			shortUnicodePassword.Code,
			http.StatusBadRequest,
		)
	}
	if shortUsername.Body.String() != shortPassword.Body.String() ||
		shortUsername.Body.String() != shortUnicodePassword.Body.String() {
		t.Errorf(
			"validation errors are unstable: %q, %q, %q",
			shortUsername.Body.String(),
			shortPassword.Body.String(),
			shortUnicodePassword.Body.String(),
		)
	}

	created := register(`{"username":"CaseEater","password":"correct horse battery staple"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("first registration status = %d, want %d", created.Code, http.StatusCreated)
	}
	duplicate := register(`{"username":"caseeater","password":"another valid password"}`)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d, want %d", duplicate.Code, http.StatusConflict)
	}
	for _, leaked := range []string{"CaseEater", "caseeater", "another valid password", "password", "hash"} {
		if strings.Contains(strings.ToLower(duplicate.Body.String()), strings.ToLower(leaked)) {
			t.Errorf("duplicate response leaks %q", leaked)
		}
	}
}

func TestHealthEndpoint(t *testing.T) {
	app := newTestApp(t, false)
	response := doRequest(t, app, http.MethodGet, "/api/healthz", "")
	if response.Code != http.StatusOK || response.Body.String() != `{"status":"ok"}` {
		t.Errorf("health response = (%d, %q), want (200, %q)", response.Code, response.Body, `{"status":"ok"}`)
	}
}
