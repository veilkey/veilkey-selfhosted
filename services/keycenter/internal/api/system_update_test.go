package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSystemUpdateStatusReflectsUIConfig(t *testing.T) {
	t.Setenv("VEILKEY_PRODUCT_VERSION", "0.1.0")
	t.Setenv("VEILKEY_UPDATE_SCRIPT", "")

	srv, handler := setupTestServer(t)
	cfg, err := srv.db.GetOrCreateUIConfig()
	if err != nil {
		t.Fatalf("GetOrCreateUIConfig: %v", err)
	}
	cfg.TargetVersion = "0.2.0"
	cfg.ReleaseChannel = "candidate"
	if err := srv.db.SaveUIConfig(cfg); err != nil {
		t.Fatalf("SaveUIConfig: %v", err)
	}

	get := getJSON(handler, "/api/system/update")
	if get.Code != 200 {
		t.Fatalf("get system update: expected 200, got %d: %s", get.Code, get.Body.String())
	}
	var resp struct {
		CurrentVersion  string `json:"current_version"`
		TargetVersion   string `json:"target_version"`
		ReleaseChannel  string `json:"release_channel"`
		UpdateEnabled   bool   `json:"update_enabled"`
		UpdateAvailable bool   `json:"update_available"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode system update: %v", err)
	}
	if resp.CurrentVersion != "0.1.0" {
		t.Fatalf("current_version = %q, want 0.1.0", resp.CurrentVersion)
	}
	if resp.TargetVersion != "0.2.0" {
		t.Fatalf("target_version = %q, want 0.2.0", resp.TargetVersion)
	}
	if resp.ReleaseChannel != "candidate" {
		t.Fatalf("release_channel = %q, want candidate", resp.ReleaseChannel)
	}
	if resp.UpdateEnabled {
		t.Fatalf("update_enabled = true, want false")
	}
	if !resp.UpdateAvailable {
		t.Fatalf("update_available = false, want true")
	}
}

func TestRunSystemUpdateExecutesConfiguredScript(t *testing.T) {
	srv, handler := setupTrustedIPServer(t, []string{"10.10.10.10"})
	t.Setenv("VEILKEY_PRODUCT_VERSION", "0.1.0")
	t.Setenv("VEILKEY_UPDATE_TIMEOUT", "5s")

	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "update-env.txt")
	script := filepath.Join(tmpDir, "run-update.sh")
	body := "#!/usr/bin/env bash\nset -euo pipefail\nprintf 'target=%s\\nchannel=%s\\ncurrent=%s\\n' \"$VEILKEY_UPDATE_TARGET_VERSION\" \"$VEILKEY_UPDATE_RELEASE_CHANNEL\" \"$VEILKEY_UPDATE_CURRENT_VERSION\" > \"" + outputFile + "\"\n"
	if err := os.WriteFile(script, []byte(body), 0755); err != nil {
		t.Fatalf("WriteFile(script): %v", err)
	}
	t.Setenv("VEILKEY_UPDATE_SCRIPT", script)

	cfg, err := srv.db.GetOrCreateUIConfig()
	if err != nil {
		t.Fatalf("GetOrCreateUIConfig: %v", err)
	}
	cfg.TargetVersion = "0.2.0"
	cfg.ReleaseChannel = "stable"
	if err := srv.db.SaveUIConfig(cfg); err != nil {
		t.Fatalf("SaveUIConfig: %v", err)
	}

	post := postJSONFromIP(handler, "/api/system/update", "10.10.10.10:1234", map[string]any{})
	if post.Code != 202 {
		t.Fatalf("run system update: expected 202, got %d: %s", post.Code, post.Body.String())
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(outputFile); err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("ReadFile(outputFile): %v", err)
	}
	got := string(data)
	if got != "target=0.2.0\nchannel=stable\ncurrent=0.1.0\n" {
		t.Fatalf("script output = %q", got)
	}
}
