package api

import (
	"net/http"
)

const lockedLandingHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>VeilKey</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,system-ui,sans-serif;background:#0f172a;color:#e2e8f0;min-height:100vh;display:flex;align-items:center;justify-content:center}
.card{background:#1e293b;border-radius:12px;padding:48px;max-width:600px;width:100%%;box-shadow:0 4px 24px rgba(0,0,0,.3)}
h1{font-size:28px;margin-bottom:8px;color:#38bdf8}
.sub{color:#94a3b8;margin-bottom:32px}
.links a{display:inline-block;margin-right:16px;color:#38bdf8;text-decoration:none;font-size:14px}
.links a:hover{text-decoration:underline}
.badge.locked{background:#7c2d12;color:#fdba74}
</style></head><body>
<div class="card">
<h1>VeilKey<span class="badge locked">locked</span></h1>
<p class="sub">KeyCenter is locked. Unlock first to enter the operator console.</p>
<div class="links">
<a href="/health">Health</a>
<a href="/api/status">Status</a>
</div>
</div></body></html>` + "\n"

func (s *Server) handleOperatorShellEntry(w http.ResponseWriter, r *http.Request) {
	if s.IsLocked() {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(lockedLandingHTML))
		return
	}
	// TODO(install-wizard): absorb lockedLandingHTML into Vue wizard as /#/locked
	if complete, _ := s.installGateState(); !complete {
		renderInstallWizard(s, w)
		return
	}
	s.handleDashboard(w, r)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if body, ok := devUIIndex(); ok {
		_, _ = w.Write(body)
		return
	}
	if body, ok := embeddedUIIndex(); ok {
		_, _ = w.Write(body)
		return
	}
	http.Error(w, "admin ui build is not available", http.StatusServiceUnavailable)
}
