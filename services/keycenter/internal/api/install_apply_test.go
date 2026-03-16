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

func TestInstallApplyStatusReflectsRuntimeConfig(t *testing.T) {
	srv, handler := setupTrustedIPServer(t, []string{"10.10.10.10"})
	t.Setenv("VEILKEY_INSTALL_SCRIPT_ALLOWLIST", "/tmp/allowed-install.sh")

	cfg, err := srv.db.GetOrCreateUIConfig()
	if err != nil {
		t.Fatalf("GetOrCreateUIConfig: %v", err)
	}
	cfg.InstallProfile = "proxmox-lxc-allinone"
	cfg.InstallRoot = "/"
	cfg.InstallScript = "/tmp/allowed-install.sh"
	cfg.InstallWorkdir = "/tmp"
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
	if resp.Profile != "proxmox-lxc-allinone" {
		t.Fatalf("profile = %q, want proxmox-lxc-allinone", resp.Profile)
	}
	if resp.Workdir != "/tmp" {
		t.Fatalf("workdir = %q, want /tmp", resp.Workdir)
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
	localvault := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer localvault.Close()

	cfg.InstallProfile = "proxmox-lxc-allinone"
	cfg.InstallRoot = "/tmp/lxc-root"
	cfg.InstallScript = script
	cfg.KeycenterURL = keycenter.URL
	cfg.LocalvaultURL = localvault.URL
	if err := srv.db.SaveUIConfig(cfg); err != nil {
		t.Fatalf("SaveUIConfig: %v", err)
	}

	saveInstallSessionForTest(t, srv, []string{"language", "bootstrap", "final_smoke"}, []string{"language"}, "language")

	post := postJSONFromIP(handler, "/api/install/apply", "10.10.10.10:1234", map[string]any{})
	if post.Code != 202 {
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
	if got != "profile=proxmox-lxc-allinone\nroot=/tmp/lxc-root\nkeycenter="+keycenter.URL+"\nlocalvault="+localvault.URL+"\n" {
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

	latest := getJSONFromIP(handler, "/api/install/state", "10.10.10.10:1234")
	if latest.Code != 200 {
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
	if len(installResp.Session.CompletedStages) != 3 {
		t.Fatalf("completed_stages length = %d, want 3", len(installResp.Session.CompletedStages))
	}
}
