package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
)

func patchJSON(handler http.Handler, path string, body interface{}) *httptest.ResponseRecorder {
	b, err := json.Marshal(body)
	if err != nil {
		panic("patchJSON: json.Marshal: " + err.Error())
	}
	req := httptest.NewRequest(http.MethodPatch, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func patchJSONFromIP(handler http.Handler, path, remoteAddr string, body interface{}) *httptest.ResponseRecorder {
	b, err := json.Marshal(body)
	if err != nil {
		panic("patchJSONFromIP: json.Marshal: " + err.Error())
	}
	req := httptest.NewRequest(http.MethodPatch, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}
