package api

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPanicRecoveryMiddleware(t *testing.T) {
	// A handler that panics
	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic: nil pointer dereference")
	})

	handler := recoveryMiddleware(panicking)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	// Should not panic — recovery middleware catches it
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}
	body := strings.TrimSpace(rec.Body.String())
	if body != "internal server error" {
		t.Errorf("expected 'internal server error' body, got %q", body)
	}
}

func TestPanicRecoveryLogsStack(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil)

	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("stack trace test")
	})

	handler := recoveryMiddleware(panicking)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	logOutput := buf.String()
	if !strings.Contains(logOutput, "PANIC recovered") {
		t.Error("log output should contain 'PANIC recovered'")
	}
	if !strings.Contains(logOutput, "stack trace test") {
		t.Error("log output should contain the panic value")
	}
	if !strings.Contains(logOutput, "goroutine") {
		t.Error("log output should contain stack trace (goroutine info)")
	}
}

func TestRecoveryMiddlewareNormalRequest(t *testing.T) {
	// Non-panicking handler should pass through normally
	normal := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	handler := recoveryMiddleware(normal)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("expected 'ok' body, got %q", rec.Body.String())
	}
}
