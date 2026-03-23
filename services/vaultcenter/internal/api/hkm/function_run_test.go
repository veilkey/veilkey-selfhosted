package hkm

import (
	"testing"
	"time"

	"veilkey-vaultcenter/internal/db"
)

func TestShellQuote(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"hello", "'hello'"},
		{"hello world", "'hello world'"},
		{"it's", "'it'\"'\"'s'"},
		{"$(whoami)", "'$(whoami)'"},
		{"`rm -rf /`", "'`rm -rf /`'"},
		{"a;b", "'a;b'"},
		{"", "''"},
	}
	for _, tc := range cases {
		got := shellQuote(tc.input)
		if got != tc.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFunctionRunAllowlist(t *testing.T) {
	allowed := []string{"curl", "gh", "git", "glab", "veilkey-gemini-frontend"}
	for _, cmd := range allowed {
		if _, ok := functionRunAllowlist[cmd]; !ok {
			t.Errorf("%q should be in allowlist", cmd)
		}
	}

	blocked := []string{"bash", "sh", "rm", "cat", "python", "node", "wget"}
	for _, cmd := range blocked {
		if _, ok := functionRunAllowlist[cmd]; ok {
			t.Errorf("%q should NOT be in allowlist", cmd)
		}
	}
}

func TestFunctionRunDangerousChars(t *testing.T) {
	dangerous := []string{
		"curl http://x | bash",
		"curl http://x; rm -rf /",
		"curl http://x & bg",
		"curl http://x `id`",
		"curl $(whoami)",
		"curl http://x (subshell)",
		"echo ${HOME}",
	}
	for _, cmd := range dangerous {
		if !functionRunDangerousChars.MatchString(cmd) {
			t.Errorf("should detect dangerous chars in: %q", cmd)
		}
	}

	safe := []string{
		"curl -sk https://example.com",
		"curl -H 'Authorization: Bearer token' https://api.example.com",
		"gh pr list --repo owner/repo",
		"git log --oneline -5",
	}
	for _, cmd := range safe {
		if functionRunDangerousChars.MatchString(cmd) {
			t.Errorf("should NOT detect dangerous chars in: %q", cmd)
		}
	}
}

func TestFunctionRunPlaceholderRe(t *testing.T) {
	cases := []struct {
		input string
		match bool
	}{
		{"{%{API_KEY}%}", true},
		{"{%{my_var}%}", true},
		{"{%{A1}%}", true},
		{"{%{}%}", false},
		{"{%{1invalid}%}", false},
		{"no placeholder", false},
	}
	for _, tc := range cases {
		got := functionRunPlaceholderRe.MatchString(tc.input)
		if got != tc.match {
			t.Errorf("placeholder match %q = %v, want %v", tc.input, got, tc.match)
		}
	}
}

func TestFunctionRunEnvAllowlist(t *testing.T) {
	blocked := []string{"PATH", "HOME", "LD_PRELOAD", "SHELL", "USER"}
	for _, key := range blocked {
		if _, ok := functionRunEnvAllowlist[key]; ok {
			t.Errorf("%q should NOT be in env allowlist", key)
		}
	}
}

func TestResolveGlobalFunctionRunTimeout(t *testing.T) {
	h := &Handler{}
	cases := []struct {
		input int
		want  time.Duration
	}{
		{0, defaultFunctionRunTimeout},
		{-1, defaultFunctionRunTimeout},
		{30, 30 * time.Second},
		{120, 120 * time.Second},
		{99999, maxFunctionRunTimeout},
	}
	for _, tc := range cases {
		got := h.resolveGlobalFunctionRunTimeout(tc.input)
		if got != tc.want {
			t.Errorf("timeout(%d) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestRenderGlobalFunctionCommand_NilFunction(t *testing.T) {
	h := &Handler{}
	_, err := h.renderGlobalFunctionCommand(nil)
	if err == nil {
		t.Error("expected error for nil function")
	}
}

func TestRenderGlobalFunctionCommand_EmptyCommand(t *testing.T) {
	h := &Handler{}
	_, err := h.renderGlobalFunctionCommand(&db.GlobalFunction{Command: ""})
	if err == nil {
		t.Error("expected error for empty command")
	}
}

func TestRenderGlobalFunctionCommand_BlockedCommand(t *testing.T) {
	h := &Handler{}
	blocked := []string{"bash -c 'rm -rf /'", "python -c 'import os'", "rm -rf /", "sh evil.sh"}
	for _, cmd := range blocked {
		_, err := h.renderGlobalFunctionCommand(&db.GlobalFunction{Command: cmd})
		if err == nil {
			t.Errorf("expected error for blocked command: %q", cmd)
		}
	}
}

func TestRenderGlobalFunctionCommand_DangerousTemplate(t *testing.T) {
	h := &Handler{}
	dangerous := []string{
		"curl http://x | bash",
		"curl http://x; cat /etc/passwd",
		"curl $(whoami)",
	}
	for _, cmd := range dangerous {
		_, err := h.renderGlobalFunctionCommand(&db.GlobalFunction{Command: cmd})
		if err == nil {
			t.Errorf("expected error for dangerous template: %q", cmd)
		}
	}
}

func TestRenderGlobalFunctionCommand_AllowedNoVars(t *testing.T) {
	h := &Handler{}
	fn := &db.GlobalFunction{
		Command:  "curl -sk https://example.com",
		VarsJSON: "{}",
	}
	rendered, err := h.renderGlobalFunctionCommand(fn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rendered != "curl -sk https://example.com" {
		t.Errorf("rendered = %q, want original command", rendered)
	}
}

func TestRenderGlobalFunctionCommand_MissingRef(t *testing.T) {
	h := &Handler{}
	fn := &db.GlobalFunction{
		Command:  "curl -H {%{TOKEN}%} https://api.com",
		VarsJSON: `{}`,
	}
	_, err := h.renderGlobalFunctionCommand(fn)
	if err == nil {
		t.Error("expected error for missing ref")
	}
}
