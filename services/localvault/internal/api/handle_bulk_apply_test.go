package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleBulkApplyExecuteReturnsPostchecks(t *testing.T) {
	server := setupReencryptTestServer(t)
	handler := server.SetupRoutes()

	configDir := "/opt/mattermost/config"
	overrideDir := "/etc/systemd/system/mattermost.service.d"
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.MkdirAll(overrideDir, 0o755); err != nil {
		t.Fatalf("mkdir override dir: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	overridePath := filepath.Join(overrideDir, "override.conf")
	t.Cleanup(func() {
		_ = os.Remove(configPath)
		_ = os.Remove(overridePath)
	})

	body, _ := json.Marshal(map[string]any{
		"name": "mattermost-apply",
		"steps": []map[string]any{
			{
				"name":        "mattermost-core-config",
				"format":      "json",
				"target_path": configPath,
				"content":     `{"ServiceSettings":{"SiteURL":"https://mattermost.test.internal"},"SqlSettings":{"DataSource":"postgres://mmuser:***@localhost:5432/mattermost"}}`,
				"hook":        "",
			},
			{
				"name":        "mattermost-systemd-override",
				"format":      "raw",
				"target_path": overridePath,
				"content":     "[Service]\nEnvironment=\"MM_SERVICESETTINGS_SITEURL=https://mattermost.test.internal\"\n",
				"hook":        "",
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/bulk-apply/execute", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Status     string `json:"status"`
		Results    []struct {
			Step       string `json:"step"`
			Status     string `json:"status"`
			Postchecks []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"postchecks"`
		} `json:"results"`
		Postchecks []struct {
			Step string `json:"step"`
			Checks []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"checks"`
		} `json:"postchecks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "applied" {
		t.Fatalf("status = %q", resp.Status)
	}
	if len(resp.Results) != 2 || len(resp.Postchecks) != 2 {
		t.Fatalf("unexpected result counts: %+v", resp)
	}
	if len(resp.Results[0].Postchecks) == 0 || len(resp.Results[1].Postchecks) == 0 {
		t.Fatalf("postchecks not attached to results: %+v", resp.Results)
	}
	for _, result := range resp.Results {
		if result.Status != "applied" {
			t.Fatalf("result status = %q", result.Status)
		}
		for _, check := range result.Postchecks {
			if check.Status != "ok" {
				t.Fatalf("postcheck status = %q", check.Status)
			}
		}
	}
}

func TestHandleBulkApplyPrecheckAcceptsGitLabConfig(t *testing.T) {
	server := setupReencryptTestServer(t)
	handler := server.SetupRoutes()

	gitlabDir := "/etc/gitlab"
	if err := os.MkdirAll(gitlabDir, 0o755); err != nil {
		t.Fatalf("mkdir gitlab dir: %v", err)
	}
	gitlabPath := filepath.Join(gitlabDir, "gitlab.rb")
	t.Cleanup(func() {
		_ = os.Remove(gitlabPath)
	})

	body, _ := json.Marshal(map[string]any{
		"name": "gitlab-phase1-apply",
		"steps": []map[string]any{
			{
				"name":        "gitlab-url-authority",
				"format":      "raw",
				"target_path": gitlabPath,
				"content":     "external_url 'https://gitlab.test.internal'\nregistry_external_url 'https://registry.test.internal'\n",
				"hook":        "reconfigure_gitlab",
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/bulk-apply/precheck", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Status string `json:"status"`
		Checks []struct {
			Step    string `json:"step"`
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "ready" {
		t.Fatalf("status = %q", resp.Status)
	}
	if len(resp.Checks) != 1 || resp.Checks[0].Status != "ok" {
		t.Fatalf("unexpected checks: %+v", resp.Checks)
	}
}
