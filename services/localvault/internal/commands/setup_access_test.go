package commands

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"veilkey-localvault/internal/api"
)

func TestSetupRoutesRequireExplicitTrustedIP(t *testing.T) {
	for _, trusted := range [][]string{nil, {""}, {"192.0.2.2"}} {
		server := api.NewServer(nil, nil, trusted)
		initialized := false
		handler := setupRoutes(server, func(w http.ResponseWriter, r *http.Request) { initialized = true; w.WriteHeader(http.StatusNoContent) })
		for _, route := range []struct{ method, path string }{
			{"GET", "/"}, {"GET", "/api/install/status"}, {"PATCH", "/api/install/vaultcenter-url"}, {"POST", "/api/install/init"},
		} {
			req := httptest.NewRequest(route.method, route.path, nil)
			req.Header.Set("X-Forwarded-For", "192.0.2.2")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s %s status=%d, want 403", route.method, route.path, rec.Code)
			}
		}
		if initialized {
			t.Error("untrusted initialization handler ran")
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/health", nil))
		if rec.Code != http.StatusOK {
			t.Errorf("health status=%d", rec.Code)
		}
	}
	server := api.NewServer(nil, nil, []string{"192.0.2.1"})
	initialized := false
	handler := setupRoutes(server, func(w http.ResponseWriter, r *http.Request) { initialized = true; w.WriteHeader(http.StatusNoContent) })
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("POST", "/api/install/init", nil))
	if rec.Code != http.StatusNoContent || !initialized {
		t.Error("explicitly trusted setup request rejected")
	}
}
