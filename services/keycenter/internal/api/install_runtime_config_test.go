package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInstallRuntimeConfigRoundTrip(t *testing.T) {
	_, handler := setupTestServer(t)

	patch := httptest.NewRequest(http.MethodPatch, "/api/install/runtime-config", strings.NewReader(`{
		"target_type":"lxc-allinone",
		"target_mode":"new",
		"target_node":"proxmox-node-a",
		"target_vmid":"220",
		"host_companion":true,
		"public_base_url":"https://keycenter.example.internal",
		"install_profile":"proxmox-lxc-allinone",
		"install_root":"/",
		"install_script":"/opt/veilkey-selfhosted-repo/installer/install.sh",
		"install_workdir":"/opt/veilkey-selfhosted-repo/installer",
		"keycenter_url":"https://keycenter.example.internal",
		"localvault_url":"https://localvault.example.internal",
		"tls_cert_path":"/etc/veilkey/tls/server.crt",
		"tls_key_path":"/etc/veilkey/tls/server.key",
		"tls_ca_path":"/etc/veilkey/tls/ca.crt"
	}`))
	patch.RemoteAddr = "127.0.0.1:12345"
	patch.Header.Set("Content-Type", "application/json")
	patchW := httptest.NewRecorder()
	handler.ServeHTTP(patchW, patch)
	if patchW.Code != http.StatusOK {
		t.Fatalf("patch runtime config: expected 200, got %d: %s", patchW.Code, patchW.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/api/install/runtime-config", nil)
	get.RemoteAddr = "127.0.0.1:12345"
	getW := httptest.NewRecorder()
	handler.ServeHTTP(getW, get)
	if getW.Code != http.StatusOK {
		t.Fatalf("get runtime config: expected 200, got %d: %s", getW.Code, getW.Body.String())
	}

	var payload installRuntimeConfigPayload
	if err := json.Unmarshal(getW.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode runtime config: %v", err)
	}
	if payload.TargetType != "lxc-allinone" || payload.TargetVMID != "220" || !payload.HostCompanion || payload.InstallProfile != "proxmox-lxc-allinone" || payload.PublicBaseURL != "https://keycenter.example.internal" {
		t.Fatalf("unexpected runtime config payload: %+v", payload)
	}
}

func TestInstallRuntimeConfigRejectsPartialTLSPaths(t *testing.T) {
	_, handler := setupTestServer(t)

	req := httptest.NewRequest(http.MethodPatch, "/api/install/runtime-config", strings.NewReader(`{
		"tls_cert_path":"/etc/veilkey/tls/server.crt"
	}`))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
