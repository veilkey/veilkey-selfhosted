package integration_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func deleteJSON(handler http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func TestIntegration_RemovedEndpoints(t *testing.T) {
	_, handler := setupTestServer(t)

	endpoints := []struct{ method, path string }{
		{"POST", "/api/decrypt"},
		{"POST", "/api/reencrypt"},
		{"POST", "/api/secrets"},
		{"GET", "/api/secrets"},
		{"GET", "/api/secrets/example"},
		{"DELETE", "/api/secrets/example"},
		{"GET", "/api/federation/secrets"},
		{"GET", "/api/federation/search/example"},
		{"POST", "/api/federation/update"},
	}
	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			var w *httptest.ResponseRecorder
			switch ep.method {
			case "POST":
				w = postJSON(handler, ep.path, map[string]string{"plaintext": "test"})
			case "DELETE":
				w = deleteJSON(handler, ep.path)
			default:
				w = getJSON(handler, ep.path)
			}
			if w.Code != http.StatusNotFound {
				t.Errorf("expected 404, got %d", w.Code)
			}
		})
	}
}
