package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	app := &application{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	app.routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || recorder.Body.String() != "ok\n" {
		t.Fatalf("health response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestRecoverPanicReturnsServerError(t *testing.T) {
	app := &application{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	handler := app.recoverPanic(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestIndexRejectsInvalidYear(t *testing.T) {
	app := &application{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	recorder := httptest.NewRecorder()
	app.indexHandler(recorder, httptest.NewRequest(http.MethodGet, "/?year=not-a-year", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestFeedbackRequestBodyLimit(t *testing.T) {
	app := &application{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	body := "feedback=" + strings.Repeat("x", 65<<10)
	request := httptest.NewRequest(http.MethodPost, "/submitFeedback", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	app.routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestGitHubWebhookRejectsMissingSignature(t *testing.T) {
	app := &application{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	app.config.Github.WebhookSecret = "secret"
	request := httptest.NewRequest(http.MethodPost, "/githubCallback", strings.NewReader(`{}`))
	request.Header.Set("X-GitHub-Event", "push")
	recorder := httptest.NewRecorder()

	app.routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestGitHubWebhookDisabledWithoutSecret(t *testing.T) {
	app := &application{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	request := httptest.NewRequest(http.MethodPost, "/githubCallback", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-GitHub-Event", "push")
	recorder := httptest.NewRecorder()
	app.routes().ServeHTTP(recorder, request)
	app.wg.Wait()
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured webhook accepted a request: %d", recorder.Code)
	}
}
