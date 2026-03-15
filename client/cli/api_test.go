package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVeilKeyClientIssue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/encrypt" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req["plaintext"] == "" {
			t.Error("plaintext should not be empty")
		}

		if err := json.NewEncoder(w).Encode(map[string]string{
			"token": "VK:a1b2c3d4",
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	client := NewVeilKeyClient(server.URL)

	vk, err := client.Issue("my-secret-value")
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}
	if vk != "VK:a1b2c3d4" {
		t.Errorf("expected VK:a1b2c3d4, got %s", vk)
	}

	// Cache hit — second call should not hit server
	vk2, err := client.Issue("my-secret-value")
	if err != nil {
		t.Fatalf("cached Issue failed: %v", err)
	}
	if vk2 != vk {
		t.Errorf("cache should return same value")
	}
}

func TestVeilKeyClientResolve(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Resolve() does GET /api/resolve/{ref}
		expectedPrefix := "/api/resolve/"
		if !strings.HasPrefix(r.URL.Path, expectedPrefix) {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}

		if err := json.NewEncoder(w).Encode(map[string]string{
			"value": "my-secret-value",
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	client := NewVeilKeyClient(server.URL)

	val, err := client.Resolve("VK:a1b2c3d4")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if val != "my-secret-value" {
		t.Errorf("expected my-secret-value, got %s", val)
	}
}

func TestVeilKeyClientResolveScopedRef(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resolve/VK:EXTERNAL:abcd1234" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"value": "my-scoped-secret-value",
		})
	}))
	defer server.Close()

	client := NewVeilKeyClient(server.URL)

	val, err := client.Resolve("VK:EXTERNAL:abcd1234")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if val != "my-scoped-secret-value" {
		t.Errorf("expected my-scoped-secret-value, got %s", val)
	}
}

func TestVeilKeyClientResolveScopedVKFallsBackToRawRef(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/api/resolve/VK:LOCAL:a1b2c3d4":
			http.Error(w, "ref not found: LOCAL:a1b2c3d4", http.StatusNotFound)
		case "/api/resolve/a1b2c3d4":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"value": "fallback-value",
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewVeilKeyClient(server.URL)
	val, err := client.Resolve("VK:LOCAL:a1b2c3d4")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if val != "fallback-value" {
		t.Fatalf("expected fallback-value, got %q", val)
	}
	if len(paths) != 2 || paths[0] != "/api/resolve/VK:LOCAL:a1b2c3d4" || paths[1] != "/api/resolve/a1b2c3d4" {
		t.Fatalf("unexpected resolve path sequence: %#v", paths)
	}
}

func TestVeilKeyClientIssueError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := NewVeilKeyClient(server.URL)

	_, err := client.Issue("test")
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}

func TestVeilKeyClientResolveError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer server.Close()

	client := NewVeilKeyClient(server.URL)

	_, err := client.Resolve("VK:invalid")
	if err == nil {
		t.Fatal("expected error on 404 response")
	}
}

func TestVeilKeyClientResolveURLEncoding(t *testing.T) {
	var receivedRawPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRawPath = r.URL.RawPath
		json.NewEncoder(w).Encode(map[string]string{"value": "ok"})
	}))
	defer server.Close()

	client := NewVeilKeyClient(server.URL)
	_, err := client.Resolve("VK:ref/with/slashes")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if receivedRawPath == "" {
		t.Fatal("RawPath should be set when path contains encoded characters")
	}
	if !strings.Contains(receivedRawPath, "%2F") {
		t.Fatalf("ref slashes should be percent-encoded in raw path, got: %s", receivedRawPath)
	}
}

func TestVeilKeyClientHealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewVeilKeyClient(server.URL)
	if !client.HealthCheck() {
		t.Error("health check should return true")
	}

	// Unreachable server
	client2 := NewVeilKeyClient("http://127.0.0.1:1")
	if client2.HealthCheck() {
		t.Error("health check should return false for unreachable server")
	}
}
