package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
)

func postJSON(handler http.Handler, path string, body interface{}) *httptest.ResponseRecorder {
	b, err := json.Marshal(body)
	if err != nil {
		panic("postJSON: json.Marshal: " + err.Error())
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}
