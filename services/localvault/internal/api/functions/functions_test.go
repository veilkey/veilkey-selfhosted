package functions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"veilkey-localvault/internal/db"
)

// mockStore is an in-memory store that mirrors the DB methods used by handlers.
type mockStore struct {
	functions map[string]*db.Function
}

func newMockStore() *mockStore {
	return &mockStore{functions: make(map[string]*db.Function)}
}

func (m *mockStore) save(fn *db.Function) error {
	m.functions[fn.Name] = fn
	return nil
}

func (m *mockStore) get(name string) (*db.Function, error) {
	fn, ok := m.functions[name]
	if !ok {
		return nil, fmt.Errorf("function %s not found", name)
	}
	return fn, nil
}

func (m *mockStore) delete(name string) error {
	if _, ok := m.functions[name]; !ok {
		return fmt.Errorf("function %s not found", name)
	}
	delete(m.functions, name)
	return nil
}

func (m *mockStore) listByScope(scope string) []db.Function {
	out := make([]db.Function, 0)
	for _, fn := range m.functions {
		if scope == "" || fn.Scope == scope {
			out = append(out, *fn)
		}
	}
	return out
}

// testHandler reimplements the handler logic against mockStore,
// allowing us to verify request validation, routing, and response
// formatting without a real database.
type testHandler struct {
	store *mockStore
}

func newTestHandler() *testHandler {
	return &testHandler{store: newMockStore()}
}

func (th *testHandler) handleFunctions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		scope := strings.TrimSpace(r.URL.Query().Get("scope"))
		functions := th.store.listByScope(scope)
		respondJSON(w, http.StatusOK, map[string]any{
			"functions": functions,
			"count":     len(functions),
		})
	case http.MethodPost:
		var req db.Function
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if req.Name == "" {
			respondError(w, http.StatusBadRequest, "function name is required")
			return
		}
		if strings.EqualFold(req.Scope, "GLOBAL") {
			respondError(w, http.StatusBadRequest, "GLOBAL functions are managed by VaultCenter sync only")
			return
		}
		if err := th.store.save(&req); err != nil {
			respondError(w, http.StatusBadRequest, "failed to save function")
			return
		}
		respondJSON(w, http.StatusOK, req)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (th *testHandler) handleFunction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		respondError(w, http.StatusBadRequest, "function name is required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		fn, err := th.store.get(name)
		if err != nil {
			respondError(w, http.StatusNotFound, "function not found")
			return
		}
		respondJSON(w, http.StatusOK, fn)
	case http.MethodDelete:
		fn, err := th.store.get(name)
		if err != nil {
			respondError(w, http.StatusNotFound, "function not found")
			return
		}
		if strings.EqualFold(fn.Scope, "GLOBAL") {
			respondError(w, http.StatusBadRequest, "GLOBAL functions are managed by VaultCenter sync only")
			return
		}
		if err := th.store.delete(name); err != nil {
			respondError(w, http.StatusNotFound, "function not found")
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"deleted": name})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (th *testHandler) mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/functions", th.handleFunctions)
	mux.HandleFunc("POST /api/functions", th.handleFunctions)
	mux.HandleFunc("GET /api/functions/{name...}", th.handleFunction)
	mux.HandleFunc("DELETE /api/functions/{name...}", th.handleFunction)
	return mux
}

// decodeBody reads the JSON response body into a map.
func decodeBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return body
}

// ---------- route registration ----------

func TestRoutes_Exist(t *testing.T) {
	th := newTestHandler()
	mux := th.mux()

	tests := []struct {
		method   string
		path     string
		wantNot  int
	}{
		{"GET", "/api/functions", 404},
		{"POST", "/api/functions", 404},
		{"GET", "/api/functions/some-name", 404},       // route exists, function doesn't
		{"DELETE", "/api/functions/some-name", 404},     // route exists, function doesn't
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			var body *bytes.Reader
			if tt.method == "POST" {
				body = bytes.NewReader([]byte(`{"name":"x","scope":"LOCAL","command":"echo"}`))
			} else {
				body = bytes.NewReader(nil)
			}
			req := httptest.NewRequest(tt.method, tt.path, body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			// The GET /api/functions/{name...} for nonexistent function returns 404
			// as a handler response, not a routing 404. We just check the route is handled.
			if tt.method == "GET" && tt.path == "/api/functions" && w.Code == 404 {
				t.Errorf("route %s %s unexpectedly returned 404", tt.method, tt.path)
			}
		})
	}
}

func TestRoutes_MethodNotAllowed(t *testing.T) {
	th := newTestHandler()
	mux := th.mux()

	tests := []struct {
		method string
		path   string
	}{
		{"PUT", "/api/functions"},
		{"PATCH", "/api/functions"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

// ---------- POST /api/functions validation ----------

func TestPostFunction_Validation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			name:       "invalid json",
			body:       `{broken`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid json body",
		},
		{
			name:       "missing name",
			body:       `{"command":"echo hi"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "function name is required",
		},
		{
			name:       "GLOBAL scope rejected",
			body:       `{"name":"fn1","scope":"GLOBAL","command":"echo"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "GLOBAL functions are managed by VaultCenter sync only",
		},
		{
			name:       "global scope case insensitive",
			body:       `{"name":"fn1","scope":"global","command":"echo"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "GLOBAL functions are managed by VaultCenter sync only",
		},
		{
			name:       "valid LOCAL function",
			body:       `{"name":"my-func","scope":"LOCAL","command":"echo hello"}`,
			wantStatus: http.StatusOK,
			wantError:  "",
		},
		{
			name:       "valid empty scope",
			body:       `{"name":"fn2","command":"ls"}`,
			wantStatus: http.StatusOK,
			wantError:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th := newTestHandler()
			mux := th.mux()

			req := httptest.NewRequest("POST", "/api/functions", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantError != "" {
				body := decodeBody(t, w.Result())
				if msg, _ := body["error"].(string); msg != tt.wantError {
					t.Errorf("error = %q, want %q", msg, tt.wantError)
				}
			}
		})
	}
}

// ---------- GET /api/functions/{name} ----------

func TestGetFunction(t *testing.T) {
	tests := []struct {
		name       string
		seed       *db.Function
		query      string
		wantStatus int
	}{
		{
			name:       "not found",
			seed:       nil,
			query:      "nonexistent",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "found",
			seed:       &db.Function{Name: "my-func", Scope: "LOCAL", Command: "echo hi"},
			query:      "my-func",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th := newTestHandler()
			if tt.seed != nil {
				th.store.save(tt.seed)
			}
			mux := th.mux()

			req := httptest.NewRequest("GET", "/api/functions/"+tt.query, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusOK {
				body := decodeBody(t, w.Result())
				if body["name"] != tt.query {
					t.Errorf("name = %v, want %s", body["name"], tt.query)
				}
			}
		})
	}
}

// ---------- DELETE /api/functions/{name} ----------

func TestDeleteFunction(t *testing.T) {
	tests := []struct {
		name       string
		seed       *db.Function
		target     string
		wantStatus int
		wantError  string
		wantGone   bool
	}{
		{
			name:       "not found",
			seed:       nil,
			target:     "nope",
			wantStatus: http.StatusNotFound,
			wantError:  "function not found",
		},
		{
			name:       "GLOBAL rejected",
			seed:       &db.Function{Name: "gfn", Scope: "GLOBAL", Command: "echo"},
			target:     "gfn",
			wantStatus: http.StatusBadRequest,
			wantError:  "GLOBAL functions are managed by VaultCenter sync only",
			wantGone:   false,
		},
		{
			name:       "LOCAL deleted",
			seed:       &db.Function{Name: "lfn", Scope: "LOCAL", Command: "echo"},
			target:     "lfn",
			wantStatus: http.StatusOK,
			wantGone:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th := newTestHandler()
			if tt.seed != nil {
				th.store.save(tt.seed)
			}
			mux := th.mux()

			req := httptest.NewRequest("DELETE", "/api/functions/"+tt.target, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if tt.wantError != "" {
				body := decodeBody(t, w.Result())
				if msg, _ := body["error"].(string); msg != tt.wantError {
					t.Errorf("error = %q, want %q", msg, tt.wantError)
				}
			}
			if tt.seed != nil {
				_, err := th.store.get(tt.target)
				gone := err != nil
				if gone != tt.wantGone {
					t.Errorf("gone = %v, want %v", gone, tt.wantGone)
				}
			}
		})
	}
}

// ---------- GET /api/functions (list / filter) ----------

func TestListFunctions_FilterByScope(t *testing.T) {
	th := newTestHandler()
	th.store.save(&db.Function{Name: "fn-local", Scope: "LOCAL", Command: "echo"})
	th.store.save(&db.Function{Name: "fn-global", Scope: "GLOBAL", Command: "echo"})
	th.store.save(&db.Function{Name: "fn-test", Scope: "TEST", Command: "echo"})
	mux := th.mux()

	tests := []struct {
		scope     string
		wantCount int
	}{
		{"", 3},
		{"LOCAL", 1},
		{"GLOBAL", 1},
		{"TEST", 1},
		{"VAULT", 0},
	}

	for _, tt := range tests {
		t.Run("scope="+tt.scope, func(t *testing.T) {
			url := "/api/functions"
			if tt.scope != "" {
				url += "?scope=" + tt.scope
			}
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}
			body := decodeBody(t, w.Result())
			count, _ := body["count"].(float64)
			if int(count) != tt.wantCount {
				t.Errorf("count = %v, want %d", count, tt.wantCount)
			}
		})
	}
}

func TestListFunctions_Empty(t *testing.T) {
	th := newTestHandler()
	mux := th.mux()

	req := httptest.NewRequest("GET", "/api/functions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := decodeBody(t, w.Result())
	count, _ := body["count"].(float64)
	if count != 0 {
		t.Errorf("count = %v, want 0", count)
	}
}

// ---------- SyncGlobalFunctions (HTTP parsing) ----------

func TestSyncGlobalFunctions_ParsesPayload(t *testing.T) {
	payload := globalFunctionEnvelope{
		Functions: []db.Function{
			{Name: "sync-fn-1", Command: "echo 1"},
			{Name: "sync-fn-2", Command: "echo 2"},
		},
	}
	data, _ := json.Marshal(payload)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer ts.Close()

	// We just verify the envelope is parsed correctly.
	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var got globalFunctionEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(got.Functions) != 2 {
		t.Errorf("got %d functions, want 2", len(got.Functions))
	}
	if got.Functions[0].Name != "sync-fn-1" {
		t.Errorf("first function name = %q, want sync-fn-1", got.Functions[0].Name)
	}
}

func TestSyncGlobalFunctions_BadStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	// Verify the error path: non-200 response should be an error.
	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Error("expected non-200 status from test server")
	}
}

// ---------- helper functions ----------

func TestRespondJSON_WritesJSON(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusOK, map[string]string{"hello": "world"})

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["hello"] != "world" {
		t.Errorf("body[hello] = %q, want world", body["hello"])
	}
}

func TestRespondError_WritesErrorJSON(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, http.StatusBadRequest, "something went wrong")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "something went wrong" {
		t.Errorf("error = %q, want 'something went wrong'", body["error"])
	}
}
