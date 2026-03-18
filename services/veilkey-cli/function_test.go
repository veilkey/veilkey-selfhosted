package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateFunctionSpecRequiresScopedRefs(t *testing.T) {
	spec := &FunctionSpec{
		Name:        "bad-ref",
		Description: "bad",
		Command:     `curl -sS "https://example.test/{%{TOKEN}%}"`,
		Vars: map[string]string{
			"TOKEN": "VK:abcd",
		},
	}
	if err := validateFunctionSpec(spec); err == nil {
		t.Fatalf("expected scoped ref validation error")
	}
}

func TestValidateFunctionSpecRejectsNonAllowlistedCommand(t *testing.T) {
	spec := &FunctionSpec{
		Name:        "bad-cmd",
		Description: "bad",
		Command:     `bash -lc "echo {%{TOKEN}%}"`,
		Vars: map[string]string{
			"TOKEN": "VK:LOCAL:abcd",
		},
	}
	if err := validateFunctionSpec(spec); err == nil {
		t.Fatalf("expected allowlist validation error")
	}
}

func TestFunctionRegisterListShowDelete(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VEILKEY_FUNCTION_DIR", dir)

	spec := &FunctionSpec{
		Name:        "gitlab-project-get",
		Description: "GitLab project fetch",
		Command:     `curl -sS -H "PRIVATE-TOKEN: {%{TOKEN}%}" "https://example.test/projects/{%{PROJECT_ID}%}"`,
		Vars: map[string]string{
			"TOKEN":      "VK:EXTERNAL:abcd1234",
			"PROJECT_ID": "VE:LOCAL:GITLAB_PROJECT_ID",
		},
	}
	if err := writeFunctionSpec(functionSelector{Name: spec.Name}, spec); err != nil {
		t.Fatalf("writeFunctionSpec: %v", err)
	}

	names, err := listFunctionNames()
	if err != nil {
		t.Fatalf("listFunctionNames: %v", err)
	}
	if len(names) != 1 || names[0] != spec.Name {
		t.Fatalf("unexpected function names: %#v", names)
	}

	loaded, err := loadFunctionSpec(functionSelector{Name: spec.Name})
	if err != nil {
		t.Fatalf("loadFunctionSpec: %v", err)
	}
	if loaded.Description != spec.Description {
		t.Fatalf("description mismatch: got %q", loaded.Description)
	}

	path := filepath.Join(dir, spec.Name+".toml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected TOML file to exist: %v", err)
	}

	if err := deleteFunctionSpec(functionSelector{Name: spec.Name}); err != nil {
		t.Fatalf("deleteFunctionSpec: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected TOML file to be deleted, got %v", err)
	}
}

func TestFunctionPreviewAndRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VEILKEY_FUNCTION_DIR", dir)

	curlDir := t.TempDir()
	curlPath := filepath.Join(curlDir, "curl")
	if err := os.WriteFile(curlPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0o755); err != nil {
		t.Fatalf("write fake curl: %v", err)
	}
	t.Setenv("PATH", curlDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	spec := &FunctionSpec{
		Name:        "gitlab-project-get",
		Description: "GitLab project fetch",
		Command:     `curl -sS -H "PRIVATE-TOKEN: {%{TOKEN}%}" "https://example.test/projects/{%{PROJECT_ID}%}"`,
		Vars: map[string]string{
			"TOKEN":      "VK:EXTERNAL:abcd1234",
			"PROJECT_ID": "VE:LOCAL:GITLAB_PROJECT_ID",
		},
	}
	if err := writeFunctionSpec(functionSelector{Name: spec.Name}, spec); err != nil {
		t.Fatalf("writeFunctionSpec: %v", err)
	}

	preview, err := renderFunctionCommand(spec, previewResolver(spec))
	if err != nil {
		t.Fatalf("render preview: %v", err)
	}
	if !strings.Contains(preview, "<masked:TOKEN>") || !strings.Contains(preview, "<masked:PROJECT_ID>") {
		t.Fatalf("expected masked preview, got %q", preview)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/resolve/") {
			http.NotFound(w, r)
			return
		}
		ref := strings.TrimPrefix(r.URL.Path, "/api/resolve/")
		var value string
		switch ref {
		case "VK:EXTERNAL:abcd1234":
			value = "secret-token"
		case "VE:LOCAL:GITLAB_PROJECT_ID":
			value = "42"
		default:
			http.Error(w, "unknown ref", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"value": value})
	}))
	defer server.Close()

	client := NewVeilKeyClient(server.URL)
	rendered, err := renderFunctionCommand(spec, realResolver(client))
	if err != nil {
		t.Fatalf("render runtime: %v", err)
	}
	if !strings.Contains(rendered, "'secret-token'") || !strings.Contains(rendered, "'42'") {
		t.Fatalf("expected resolved values to be shell-quoted, got %q", rendered)
	}
}

func TestCmdFunctionTestPrintsContextVaultHash(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VEILKEY_FUNCTION_DIR", dir)

	spec := &FunctionSpec{
		Name:        "demo",
		Description: "demo",
		Command:     `curl -sS "https://example.test/{%{TOKEN}%}"`,
		Vars: map[string]string{
			"TOKEN": "VK:LOCAL:abcd1234",
		},
	}
	if err := writeFunctionSpec(functionSelector{Name: spec.Name}, spec); err != nil {
		t.Fatalf("writeFunctionSpec: %v", err)
	}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	cmdFunctionTest(functionSelector{Name: "demo"}, functionRunOptions{VaultHash: "56093730"})
	_ = w.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "context_vault_hash=56093730") {
		t.Fatalf("expected context_vault_hash in output, got %q", out)
	}
	if !strings.Contains(out, "<masked:TOKEN>") {
		t.Fatalf("expected masked preview, got %q", out)
	}
}

func TestDomainFunctionPathAndRunSelection(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VEILKEY_FUNCTION_DIR", dir)

	spec := &FunctionSpec{
		Name:        "project-get",
		Description: "domain test",
		Command:     `curl -sS "https://example.test/{%{TOKEN}%}"`,
		Vars: map[string]string{
			"TOKEN": "VK:LOCAL:abcd1234",
		},
	}
	sel := functionSelector{Domain: "gitlab", Name: "project-get"}
	if err := writeFunctionSpec(sel, spec); err != nil {
		t.Fatalf("writeFunctionSpec(domain): %v", err)
	}
	path := filepath.Join(dir, "gitlab", "project-get.toml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected domain TOML file to exist: %v", err)
	}
	names, err := listFunctionNames()
	if err != nil {
		t.Fatalf("listFunctionNames: %v", err)
	}
	if len(names) != 1 || names[0] != "gitlab/project-get" {
		t.Fatalf("unexpected function names: %#v", names)
	}
	loaded, err := loadFunctionSpec(sel)
	if err != nil {
		t.Fatalf("loadFunctionSpec(domain): %v", err)
	}
	if loaded.Name != "project-get" {
		t.Fatalf("unexpected loaded name: %q", loaded.Name)
	}
	parsed, rest, err := parseFunctionSelector([]string{"gitlab", "project-get", "56093730"})
	if err != nil {
		t.Fatalf("parseFunctionSelector(domain): %v", err)
	}
	if parsed.Domain != "gitlab" || parsed.Name != "project-get" || len(rest) != 1 || rest[0] != "56093730" {
		t.Fatalf("unexpected parsed selector: %#v %#v", parsed, rest)
	}
	parsed, rest, err = parseFunctionSelector([]string{"project-get", "56093730"})
	if err != nil {
		t.Fatalf("parseFunctionSelector(simple): %v", err)
	}
	if parsed.Domain != "" || parsed.Name != "project-get" || len(rest) != 1 || rest[0] != "56093730" {
		t.Fatalf("unexpected parsed simple selector: %#v %#v", parsed, rest)
	}
}
