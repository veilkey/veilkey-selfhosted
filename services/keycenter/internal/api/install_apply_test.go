package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInstallApplyStatusReflectsResolvedProfile(t *testing.T) {
	srv, handler := setupTrustedIPServer(t, []string{"10.10.10.10"})
	t.Setenv("VEILKEY_INSTALL_SCRIPT_ALLOWLIST", "/tmp/allowed-install.sh")

	cfg, err := srv.db.GetOrCreateUIConfig()
	if err != nil {
		t.Fatalf("GetOrCreateUIConfig: %v", err)
	}
	cfg.InstallProfile = "linux-host"
	cfg.InstallRoot = "/tmp/rootfs"
	cfg.InstallScript = "/tmp/allowed-install.sh"
	cfg.InstallWorkdir = "/tmp"
	cfg.LocalvaultURL = ""
	if err := srv.db.SaveUIConfig(cfg); err != nil {
		t.Fatalf("SaveUIConfig: %v", err)
	}

	get := getJSONFromIP(handler, "/api/install/apply", "10.10.10.10:1234")
	if get.Code != 200 {
		t.Fatalf("get install apply: expected 200, got %d: %s", get.Code, get.Body.String())
	}
	var resp installApplyPayload
	if err := json.Unmarshal(get.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode install apply: %v", err)
	}
	if !resp.InstallEnabled {
		t.Fatalf("install_enabled = false, want true")
	}
	if resp.Profile != "proxmox-host" {
		t.Fatalf("profile = %q, want proxmox-host", resp.Profile)
	}
}

func TestInstallValidateRejectsDangerousRootWithoutConfirmation(t *testing.T) {
	srv, handler := setupTrustedIPServer(t, []string{"10.10.10.10"})
	t.Setenv("VEILKEY_INSTALL_SCRIPT_ALLOWLIST", "/tmp/allowed-install.sh")

	cfg, err := srv.db.GetOrCreateUIConfig()
	if err != nil {
		t.Fatalf("GetOrCreateUIConfig: %v", err)
	}
	cfg.InstallProfile = "linux-host"
	cfg.InstallRoot = "/"
	cfg.InstallScript = "/tmp/allowed-install.sh"
	cfg.InstallWorkdir = "/tmp"
	if err := srv.db.SaveUIConfig(cfg); err != nil {
		t.Fatalf("SaveUIConfig: %v", err)
	}

	post := postJSONFromIP(handler, "/api/install/validate", "10.10.10.10:1234", map[string]any{})
	if post.Code != http.StatusBadRequest {
		t.Fatalf("validate install: expected 400, got %d: %s", post.Code, post.Body.String())
	}
	var resp struct {
		Validation installValidationResult `json:"validation"`
	}
	if err := json.Unmarshal(post.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode validate response: %v", err)
	}
	if !resp.Validation.DangerousRoot {
		t.Fatalf("dangerous_root = false, want true")
	}
	if !resp.Validation.NeedsConfirmation {
		t.Fatalf("needs_confirmation = false, want true")
	}
}

func TestRunInstallApplyExecutesConfiguredScript(t *testing.T) {
	srv, handler := setupTrustedIPServer(t, []string{"10.10.10.10"})
	t.Setenv("VEILKEY_INSTALL_TIMEOUT", "5s")

	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "install-env.txt")
	script := filepath.Join(tmpDir, "apply-install.sh")
	body := "#!/usr/bin/env bash\nset -euo pipefail\nprintf 'profile=%s\\nroot=%s\\nkeycenter=%s\\nlocalvault=%s\\n' \"$VEILKEY_INSTALL_PROFILE\" \"$VEILKEY_INSTALL_ROOT\" \"$VEILKEY_KEYCENTER_URL\" \"$VEILKEY_LOCALVAULT_URL\" > \"" + outputFile + "\"\n"
	if err := os.WriteFile(script, []byte(body), 0755); err != nil {
		t.Fatalf("WriteFile(script): %v", err)
	}
	t.Setenv("VEILKEY_INSTALL_SCRIPT_ALLOWLIST", script)

	cfg, err := srv.db.GetOrCreateUIConfig()
	if err != nil {
		t.Fatalf("GetOrCreateUIConfig: %v", err)
	}
	keycenter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer keycenter.Close()

	cfg.InstallProfile = "linux-host"
	cfg.InstallRoot = "/tmp/host-root"
	cfg.InstallScript = script
	cfg.InstallWorkdir = tmpDir
	cfg.KeycenterURL = keycenter.URL
	cfg.LocalvaultURL = ""
	if err := srv.db.SaveUIConfig(cfg); err != nil {
		t.Fatalf("SaveUIConfig: %v", err)
	}

	saveInstallSessionForTest(t, srv, []string{"language", "bootstrap", "final_smoke"}, []string{"language"}, "language")

	post := postJSONFromIP(handler, "/api/install/apply", "10.10.10.10:1234", map[string]any{
		"confirm_dangerous_root": false,
	})
	if post.Code != http.StatusAccepted {
		t.Fatalf("run install apply: expected 202, got %d: %s", post.Code, post.Body.String())
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
	if got != "profile=proxmox-host\nroot=/tmp/host-root\nkeycenter="+keycenter.URL+"\nlocalvault=\n" {
		t.Fatalf("script output = %q", got)
	}

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state := getJSONFromIP(handler, "/api/install/apply", "10.10.10.10:1234")
		var resp installApplyPayload
		if err := json.Unmarshal(state.Body.Bytes(), &resp); err == nil && resp.State.Status == "succeeded" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	runs := getJSONFromIP(handler, "/api/install/runs", "10.10.10.10:1234")
	if runs.Code != http.StatusOK {
		t.Fatalf("get install runs: expected 200, got %d: %s", runs.Code, runs.Body.String())
	}
	var runsResp struct {
		Runs []installRunPayload `json:"runs"`
	}
	if err := json.Unmarshal(runs.Body.Bytes(), &runsResp); err != nil {
		t.Fatalf("decode install runs: %v", err)
	}
	if len(runsResp.Runs) == 0 {
		t.Fatalf("expected persisted install runs")
	}
	if runsResp.Runs[0].RunKind != "apply" {
		t.Fatalf("run_kind = %q, want apply", runsResp.Runs[0].RunKind)
	}
	if runsResp.Runs[0].InstallProfile != "proxmox-host" {
		t.Fatalf("install_profile = %q, want proxmox-host", runsResp.Runs[0].InstallProfile)
	}

	latest := getJSONFromIP(handler, "/api/install/state", "10.10.10.10:1234")
	if latest.Code != http.StatusOK {
		t.Fatalf("get install state: expected 200, got %d: %s", latest.Code, latest.Body.String())
	}
	var installResp struct {
		Exists  bool                `json:"exists"`
		Session installStatePayload `json:"session"`
	}
	if err := json.Unmarshal(latest.Body.Bytes(), &installResp); err != nil {
		t.Fatalf("decode install state: %v", err)
	}
	if !installResp.Exists {
		t.Fatalf("install session missing after apply")
	}
	if installResp.Session.LastStage != "final_smoke" {
		t.Fatalf("last_stage = %q, want final_smoke", installResp.Session.LastStage)
	}
}
