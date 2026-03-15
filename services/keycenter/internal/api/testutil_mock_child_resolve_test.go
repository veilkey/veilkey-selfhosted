package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newMockChildResolve(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/resolve/test_ref" {
			if r.Header.Get("X-VeilKey-Cascade") != "true" {
				t.Error("child did not receive X-VeilKey-Cascade header")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"ref": "test_ref", "name": "CHILD_SECRET", "value": "from-child",
			})
			return
		}
		http.NotFound(w, r)
	}))
}
