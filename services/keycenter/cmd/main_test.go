package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseDurationEnv(t *testing.T) {
	tests := []struct {
		name       string
		envKey     string
		envVal     string
		defaultVal time.Duration
		want       time.Duration
	}{
		{"empty env uses default", "TEST_DUR_EMPTY", "", 5 * time.Second, 5 * time.Second},
		{"valid seconds", "TEST_DUR_SECS", "10s", 5 * time.Second, 10 * time.Second},
		{"valid minutes", "TEST_DUR_MINS", "2m", 5 * time.Second, 2 * time.Minute},
		{"valid milliseconds", "TEST_DUR_MS", "500ms", 1 * time.Second, 500 * time.Millisecond},
		{"invalid falls back to default", "TEST_DUR_BAD", "not-a-duration", 7 * time.Second, 7 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				os.Setenv(tt.envKey, tt.envVal)
				defer os.Unsetenv(tt.envKey)
			} else {
				os.Unsetenv(tt.envKey)
			}
			got := parseDurationEnv(tt.envKey, tt.defaultVal)
			if got != tt.want {
				t.Errorf("parseDurationEnv(%q) = %v, want %v", tt.envKey, got, tt.want)
			}
		})
	}
}

func TestGetEnvDefault(t *testing.T) {
	os.Setenv("TEST_GED_SET", "custom-value")
	defer os.Unsetenv("TEST_GED_SET")

	if got := getEnvDefault("TEST_GED_SET", "fallback"); got != "custom-value" {
		t.Errorf("set env: got %q, want custom-value", got)
	}
	if got := getEnvDefault("TEST_GED_UNSET", "fallback"); got != "fallback" {
		t.Errorf("unset env: got %q, want fallback", got)
	}
}

func TestReadPasswordFromFileEnv(t *testing.T) {
	t.Run("not set returns empty", func(t *testing.T) {
		t.Setenv("VEILKEY_PASSWORD_FILE", "")
		if got := readPasswordFromFileEnv(); got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("reads password from file", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "pw")
		os.WriteFile(f, []byte("my-secret"), 0600)
		t.Setenv("VEILKEY_PASSWORD_FILE", f)
		if got := readPasswordFromFileEnv(); got != "my-secret" {
			t.Fatalf("got %q, want my-secret", got)
		}
	})

	t.Run("trims trailing newlines", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "pw")
		os.WriteFile(f, []byte("secret\n\r\n"), 0600)
		t.Setenv("VEILKEY_PASSWORD_FILE", f)
		if got := readPasswordFromFileEnv(); got != "secret" {
			t.Fatalf("got %q, want secret", got)
		}
	})
}

func TestDetectExternalIP(t *testing.T) {
	ip := detectExternalIP()
	// On any machine with a network interface, should return something
	// On CI/isolated, might be empty — that's OK
	if ip != "" {
		// Basic IPv4 format check
		if len(ip) < 7 { // "1.2.3.4" = 7 chars minimum
			t.Errorf("detectExternalIP() = %q, doesn't look like IPv4", ip)
		}
	}
}
