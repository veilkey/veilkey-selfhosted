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

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "mattermost", "config", "config.json")
	overridePath := filepath.Join(tmpDir, "systemd", "mattermost.service.d", "override.conf")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(overridePath), 0o755); err != nil {
		t.Fatal(err)
	}
	allowedBulkApplyTargets[configPath] = struct{}{}
	allowedBulkApplyTargets[overridePath] = struct{}{}
	t.Cleanup(func() {
		delete(allowedBulkApplyTargets, configPath)
		delete(allowedBulkApplyTargets, overridePath)
	})

	body, err := json.Marshal(map[string]any{
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
	if err != nil {
		t.Fatal(err)
	}
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

	tmpDir := t.TempDir()
	gitlabPath := filepath.Join(tmpDir, "gitlab", "gitlab.rb")
	if err := os.MkdirAll(filepath.Dir(gitlabPath), 0o755); err != nil {
		t.Fatal(err)
	}
	allowedBulkApplyTargets[gitlabPath] = struct{}{}
	t.Cleanup(func() { delete(allowedBulkApplyTargets, gitlabPath) })

	body, err := json.Marshal(map[string]any{
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
	if err != nil {
		t.Fatal(err)
	}
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
