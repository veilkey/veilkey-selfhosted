package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecVKHashResolution(t *testing.T) {
	// Mock VeilKey server — Resolve() does GET /api/resolve/{ref}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Map known refs to plaintext
		resolveMap := map[string]string{
			"a1b2c3d4": "real-api-key-12345",
			"e5f6a7b8": "real-password-67890",
		}

		prefix := "/api/resolve/"
		if strings.HasPrefix(r.URL.Path, prefix) {
			ref := strings.TrimPrefix(r.URL.Path, prefix)
			if plain, ok := resolveMap[ref]; ok {
				if err := json.NewEncoder(w).Encode(map[string]string{"value": plain}); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer server.Close()

	client := NewVeilKeyClient(server.URL)

	// Test argument resolution
	args := []string{
		"curl",
		"-H", "Authorization: Bearer VK:a1b2c3d4",
		"--data", "password=VK:e5f6a7b8",
	}

	resolved := make([]string, len(args))
	for i, arg := range args {
		resolved[i] = veilkeyRE.ReplaceAllStringFunc(arg, func(hash string) string {
			val, err := client.Resolve(hash)
			if err != nil {
				return hash
			}
			return val
		})
	}

	if resolved[0] != "curl" {
		t.Errorf("command should be unchanged: %s", resolved[0])
	}
	if !strings.Contains(resolved[2], "real-api-key-12345") {
		t.Errorf("VK hash should be resolved to real value: %s", resolved[2])
	}
	if strings.Contains(resolved[2], "VK:a1b2c3d4") {
		t.Error("VK hash should be replaced")
	}
	if !strings.Contains(resolved[4], "real-password-67890") {
		t.Errorf("second VK hash should be resolved: %s", resolved[4])
	}
}

func TestExecVKHashRegex(t *testing.T) {

	tests := []struct {
		input   string
		matches []string
	}{
		{"VK:a1b2c3d4", []string{"VK:a1b2c3d4"}},
		{"VK:00112233", []string{"VK:00112233"}},
		{"prefix VK:aabbccdd suffix", []string{"VK:aabbccdd"}},
		{"no match here", nil},
		{"VK:a1b2c3d4 and VK:e5f6a7b8", []string{"VK:a1b2c3d4", "VK:e5f6a7b8"}},
		{"VK:short", nil}, // too short, not 8 hex chars
		// Scoped VK tokens
		{"VK:TEMP:abcd1234", []string{"VK:TEMP:abcd1234"}},
		{"VK:LOCAL:a1b2c3d4", []string{"VK:LOCAL:a1b2c3d4"}},
		{"VK:EXTERNAL:abcd1234abcd1234", []string{"VK:EXTERNAL:abcd1234abcd1234"}},
		{"token=VK:TEMP:AABB1234 end", []string{"VK:TEMP:AABB1234"}},
	}

	for _, tt := range tests {
		matches := veilkeyRE.FindAllString(tt.input, -1)
		if len(matches) != len(tt.matches) {
			t.Errorf("input %q: expected %d matches, got %d (%v)", tt.input, len(tt.matches), len(matches), matches)
			continue
		}
		for i, m := range matches {
			if m != tt.matches[i] {
				t.Errorf("input %q: match[%d] expected %s, got %s", tt.input, i, tt.matches[i], m)
			}
		}
	}
}

func TestExecResolutionFailureKeepsOriginal(t *testing.T) {
	// Server always returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer server.Close()

	client := NewVeilKeyClient(server.URL)

	arg := "token=VK:deadbeef"
	resolved := veilkeyRE.ReplaceAllStringFunc(arg, func(hash string) string {
		val, err := client.Resolve(hash)
		if err != nil {
			return hash // Keep original on failure
		}
		return val
	})

	if resolved != "token=VK:deadbeef" {
		t.Errorf("on resolution failure, should keep original: %s", resolved)
	}
}
