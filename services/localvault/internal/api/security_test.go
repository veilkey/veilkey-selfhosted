package api

import (
	"os"
	"strings"
	"testing"
)

func extractFn(code, sig string) string {
	i := strings.Index(code, sig)
	if i < 0 {
		return ""
	}
	r := code[i:]
	n := strings.Index(r[1:], "\nfunc ")
	if n < 0 {
		return r
	}
	return r[:n+1]
}

func routeLine(code, path string) string {
	for _, l := range strings.Split(code, "\n") {
		if strings.Contains(l, path) && strings.Contains(l, "HandleFunc") {
			return l
		}
	}
	return ""
}

func TestUnlockMaxBytes(t *testing.T) {
	s, _ := os.ReadFile("api.go")
	if !strings.Contains(extractFn(string(s), "func (s *Server) handleUnlock("), "MaxBytesReader") {
		t.Error("handleUnlock must use MaxBytesReader")
	}
}

func TestUnlockMaxLen(t *testing.T) {
	s, _ := os.ReadFile("api.go")
	if !strings.Contains(extractFn(string(s), "func (s *Server) handleUnlock("), "len(req.Password) >") {
		t.Error("handleUnlock needs max password length")
	}
}

func TestIPv6Parsing(t *testing.T) {
	s, _ := os.ReadFile("api.go")
	b := extractFn(string(s), "func (s *Server) requireTrustedIP(")
	if strings.Contains(b, `strings.Split(r.RemoteAddr, ":")`) {
		t.Error("must use net.SplitHostPort for IPv6")
	}
}

func TestAgentSecretLocked(t *testing.T) {
	s, _ := os.ReadFile("api.go")
	b := extractFn(string(s), "func (s *Server) requireAgentSecret(")
	i := strings.Index(b, "s.IsLocked()")
	if i < 0 {
		t.Fatal("must check IsLocked")
	}
	after := b[i : i+300]
	if strings.Contains(after, "next(w, r)") && !strings.Contains(after, "respondError") {
		t.Error("must not pass-through when locked")
	}
}

func TestHSTS(t *testing.T) {
	s, _ := os.ReadFile("api.go")
	if !strings.Contains(extractFn(string(s), "func securityHeadersMiddleware("), "Strict-Transport-Security") {
		t.Error("missing HSTS header")
	}
}

func TestReencryptAuth(t *testing.T) {
	s, _ := os.ReadFile("api.go")
	if l := routeLine(string(s), "/api/reencrypt"); !strings.Contains(l, "requireTrustedIP") {
		t.Error("/api/reencrypt needs requireTrustedIP")
	}
}

func TestLifecycleAuth(t *testing.T) {
	s, _ := os.ReadFile("api.go")
	c := string(s)
	for _, ep := range []string{"/api/activate", "/api/archive", "/api/block", "/api/revoke"} {
		if l := routeLine(c, ep); !strings.Contains(l, "requireTrustedIP") {
			t.Errorf("%s needs requireTrustedIP", ep)
		}
	}
}

func TestConfigsAuth(t *testing.T) {
	s, _ := os.ReadFile("configs/handler.go")
	for _, l := range strings.Split(string(s), "\n") {
		if strings.Contains(l, `"GET /api/configs"`) && !strings.Contains(l, "{key}") {
			if !strings.Contains(l, "trusted(") {
				t.Error("GET /api/configs needs trusted()")
			}
			return
		}
	}
}

func TestInstallStatusAuth(t *testing.T) {
	s, _ := os.ReadFile("api.go")
	if l := routeLine(string(s), "/api/install/status"); !strings.Contains(l, "requireTrustedIP") {
		t.Error("/api/install/status needs requireTrustedIP")
	}
}

func TestBulkApplyAssertions(t *testing.T) {
	s, _ := os.ReadFile("bulk/apply.go")
	c := string(s)
	if strings.Contains(c, `payload["ServiceSettings"].(map[string]any)`) &&
		!strings.Contains(c, "ok1") && !strings.Contains(c, "ok2") {
		t.Error("type assertions need ok check")
	}
}

// ══ CSP header ══════════════════════════════════════════════════

func TestCSPHeader(t *testing.T) {
	s, _ := os.ReadFile("api.go")
	body := extractFn(string(s), "func securityHeadersMiddleware(")
	if body == "" {
		t.Fatal("securityHeadersMiddleware must exist")
	}
	if !strings.Contains(body, "Content-Security-Policy") {
		t.Error("must set Content-Security-Policy header")
	}
}

// ══ Diagnostics: no raw env leak ════════════════════════════════

func TestDiagnosticsNoRawEnvDump(t *testing.T) {
	s, _ := os.ReadFile("admin_api.go")
	body := extractFn(string(s), "func (s *Server) handleDiagnostics(")
	if body == "" {
		t.Skip("handleDiagnostics not found")
	}
	// Must NOT dump full os.Environ() — only specific safe keys
	if strings.Contains(body, "os.Environ()") {
		t.Error("diagnostics must not dump full os.Environ() — may leak secrets")
	}
	// Must be behind trusted IP (check route)
	routeSrc, _ := os.ReadFile("admin_api.go")
	if routeSrc != nil {
		code := string(routeSrc)
		line := ""
		for _, l := range strings.Split(code, "\n") {
			if strings.Contains(l, "diagnostics") && strings.Contains(l, "HandleFunc") {
				line = l
				break
			}
		}
		if line != "" && !strings.Contains(line, "trusted(") && !strings.Contains(line, "requireTrustedIP") {
			t.Error("diagnostics must be behind trusted IP check")
		}
	}
}

// ══ No secret leaks in log statements ═══════════════════════════

func TestNoPasswordInLogs(t *testing.T) {
	files := []string{"api.go", "admin_api.go"}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for i, line := range strings.Split(string(src), "\n") {
			if !strings.Contains(line, "log.") {
				continue
			}
			lower := strings.ToLower(line)
			if strings.Contains(lower, "req.password") ||
				strings.Contains(lower, "agentsecret") {
				t.Errorf("%s:%d: log may leak secret: %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
}

// ══ Write errors: static file handlers ══════════════════════════

func TestInstallWizardWriteError(t *testing.T) {
	s, _ := os.ReadFile("install_wizard.go")
	if s == nil {
		t.Skip("install_wizard.go not found")
	}
	code := string(s)
	// Check that w.Write errors are not silently swallowed
	// Acceptable: `_, _ = w.Write(body)` for static HTML (nothing to do on error)
	// This test documents the pattern — if Write starts doing something critical, revisit
	if strings.Contains(code, "_, _ = w.Write(") {
		t.Log("NOTE: install_wizard.go silently ignores w.Write errors (acceptable for static HTML)")
	}
}
