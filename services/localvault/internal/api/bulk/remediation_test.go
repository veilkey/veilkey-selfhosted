package bulk

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestBulkApplyRejectsRequestHookEnvironment(t *testing.T) {
	target := filepath.Join(t.TempDir(), "app.conf")
	extraBulkApplyTargets[target] = struct{}{}
	t.Cleanup(func() { delete(extraBulkApplyTargets, target) })
	step := bulkApplyStep{Name: "config", Format: "raw", TargetPath: target, Content: "enabled=true", HookEnv: map[string]string{"APP_MODE": "test"}}
	for _, endpoint := range []http.HandlerFunc{NewHandler().handleBulkApplyPrecheck, NewHandler().handleBulkApplyExecute} {
		raw, err := json.Marshal(bulkApplyWorkflowRequest{Name: "config", Steps: []bulkApplyStep{step}})
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		endpoint(rec, httptest.NewRequest("POST", "/", bytes.NewReader(raw)))
		if rec.Code != http.StatusConflict {
			t.Errorf("status=%d, want 409", rec.Code)
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Error("invalid request created target")
		}
	}
	env := hookEnv(bulkApplyWorkflowRequest{Steps: []bulkApplyStep{step}}, "")
	if _, ok := env["APP_MODE"]; ok {
		t.Error("request environment forwarded")
	}
}

func TestBulkApplyRejectsRawGitlabConfig(t *testing.T) {
	target := filepath.Join(t.TempDir(), "gitlab.rb")
	extraBulkApplyTargets[target] = struct{}{}
	t.Cleanup(func() { delete(extraBulkApplyTargets, target) })
	if err := validateBulkApplyStep(bulkApplyStep{Name: "config", Format: "raw", TargetPath: target, Content: "enabled=true"}, NewFormatRegistry()); err == nil {
		t.Error("raw gitlab config accepted")
	}
}

func TestBulkApplyRejectsRawGitlabConfigAlias(t *testing.T) {
	target := filepath.Join(t.TempDir(), "gitlab.rb")
	if err := os.WriteFile(target, nil, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(filepath.Dir(target), "config")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	extraBulkApplyTargets[alias] = struct{}{}
	t.Cleanup(func() { delete(extraBulkApplyTargets, alias) })
	if err := validateBulkApplyStep(bulkApplyStep{Name: "config", Format: "raw", TargetPath: alias, Content: "enabled=true"}, NewFormatRegistry()); err == nil {
		t.Error("raw gitlab config alias accepted")
	}
}

func TestBulkApplyExplicitTargetStillWorks(t *testing.T) {
	target := filepath.Join(t.TempDir(), "app.conf")
	extraBulkApplyTargets[target] = struct{}{}
	t.Cleanup(func() { delete(extraBulkApplyTargets, target) })
	raw, err := json.Marshal(bulkApplyWorkflowRequest{Name: "config", Steps: []bulkApplyStep{{Name: "config", Format: "raw", TargetPath: target, Content: "enabled=true"}}})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	NewHandler().handleBulkApplyExecute(rec, httptest.NewRequest("POST", "/", bytes.NewReader(raw)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "enabled=true" {
		t.Error("explicit target not written correctly")
	}
}
