package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestUIConfigDefaultsAndUpdatesLocale(t *testing.T) {
	_, handler := setupTestServer(t)

	get := getJSON(handler, "/api/ui/config")
	if get.Code != http.StatusOK {
		t.Fatalf("get ui config: expected 200, got %d: %s", get.Code, get.Body.String())
	}
	var initial struct {
		Locale         string `json:"locale"`
		DefaultEmail   string `json:"default_email"`
		TargetVersion  string `json:"target_version"`
		ReleaseChannel string `json:"release_channel"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode initial ui config: %v", err)
	}
	if initial.Locale != "ko" {
		t.Fatalf("initial locale = %q, want ko", initial.Locale)
	}
	if initial.DefaultEmail != "" {
		t.Fatalf("initial default_email = %q, want empty", initial.DefaultEmail)
	}
	if initial.TargetVersion != "" {
		t.Fatalf("initial target_version = %q, want empty", initial.TargetVersion)
	}
	if initial.ReleaseChannel != "stable" {
		t.Fatalf("initial release_channel = %q, want stable", initial.ReleaseChannel)
	}

	patch := patchJSON(handler, "/api/ui/config", map[string]any{
		"locale":          "en",
		"default_email":   "ops@example.com",
		"target_version":  "0.2.0",
		"release_channel": "candidate",
	})
	if patch.Code != http.StatusOK {
		t.Fatalf("patch ui config: expected 200, got %d: %s", patch.Code, patch.Body.String())
	}
	var updated struct {
		Locale         string `json:"locale"`
		DefaultEmail   string `json:"default_email"`
		TargetVersion  string `json:"target_version"`
		ReleaseChannel string `json:"release_channel"`
	}
	if err := json.Unmarshal(patch.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode patched ui config: %v", err)
	}
	if updated.Locale != "en" {
		t.Fatalf("patched locale = %q, want en", updated.Locale)
	}
	if updated.DefaultEmail != "ops@example.com" {
		t.Fatalf("patched default_email = %q, want ops@example.com", updated.DefaultEmail)
	}
	if updated.TargetVersion != "0.2.0" {
		t.Fatalf("patched target_version = %q, want 0.2.0", updated.TargetVersion)
	}
	if updated.ReleaseChannel != "candidate" {
		t.Fatalf("patched release_channel = %q, want candidate", updated.ReleaseChannel)
	}

	getAgain := getJSON(handler, "/api/ui/config")
	if getAgain.Code != http.StatusOK {
		t.Fatalf("get ui config again: expected 200, got %d: %s", getAgain.Code, getAgain.Body.String())
	}
	if err := json.Unmarshal(getAgain.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode get-again ui config: %v", err)
	}
	if updated.Locale != "en" {
		t.Fatalf("persisted locale = %q, want en", updated.Locale)
	}
	if updated.DefaultEmail != "ops@example.com" {
		t.Fatalf("persisted default_email = %q, want ops@example.com", updated.DefaultEmail)
	}
	if updated.TargetVersion != "0.2.0" {
		t.Fatalf("persisted target_version = %q, want 0.2.0", updated.TargetVersion)
	}
	if updated.ReleaseChannel != "candidate" {
		t.Fatalf("persisted release_channel = %q, want candidate", updated.ReleaseChannel)
	}
}

func TestUIConfigRejectsInvalidDefaultEmail(t *testing.T) {
	_, handler := setupTestServer(t)

	patch := patchJSON(handler, "/api/ui/config", map[string]any{
		"locale":        "ko",
		"default_email": "not-an-email",
	})
	if patch.Code != http.StatusBadRequest {
		t.Fatalf("invalid default_email: expected 400, got %d: %s", patch.Code, patch.Body.String())
	}
}

func TestUIConfigRejectsInvalidReleaseChannel(t *testing.T) {
	_, handler := setupTestServer(t)

	patch := patchJSON(handler, "/api/ui/config", map[string]any{
		"locale":          "ko",
		"default_email":   "ops@example.com",
		"release_channel": "beta",
	})
	if patch.Code != http.StatusBadRequest {
		t.Fatalf("invalid release_channel: expected 400, got %d: %s", patch.Code, patch.Body.String())
	}
}
