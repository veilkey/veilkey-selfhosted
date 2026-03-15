package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
)

func newMockRekeyServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/rekey" && r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "rekeyed", "secrets_updated": 5, "version": 2,
			})
			return
		}
		http.NotFound(w, r)
	}))
}
