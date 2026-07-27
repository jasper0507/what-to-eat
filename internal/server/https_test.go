package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestForceHTTPSRedirectsWhenForwardedProtoIsHTTP(t *testing.T) {
	app := newTestApp(t, true)

	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	request.Host = "what2eat.example.test"
	request.Header.Set("X-Forwarded-Proto", "http")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)

	if response.Code != http.StatusPermanentRedirect {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusPermanentRedirect)
	}
	location := response.Header().Get("Location")
	if location != "https://what2eat.example.test/login" {
		t.Fatalf("Location = %q, want https redirect", location)
	}
}

func TestForceHTTPSSetsHSTSOnForwardedHTTPS(t *testing.T) {
	app := newTestApp(t, true)

	request := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	request.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	hsts := response.Header().Get("Strict-Transport-Security")
	if hsts == "" {
		t.Fatal("expected Strict-Transport-Security on https-forwarded request")
	}
}

func TestForceHTTPSSkipsWhenSecureCookiesDisabled(t *testing.T) {
	app := newTestApp(t, false)

	request := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	request.Header.Set("X-Forwarded-Proto", "http")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (no redirect without SecureCookies)", response.Code, http.StatusOK)
	}
}

func TestHealthzWithoutForwardedProtoStillOK(t *testing.T) {
	// 容器内健康检查走 127.0.0.1，没有 X-Forwarded-Proto。
	app := newTestApp(t, true)
	response := doRequest(t, app, http.MethodGet, "/api/healthz", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}
