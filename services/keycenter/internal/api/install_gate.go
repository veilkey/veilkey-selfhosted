package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"veilkey-keycenter/internal/db"
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

func renderInstallGate(s *Server, w http.ResponseWriter, session *db.InstallSession) {
	lastStage := "pending"
	flow := "wizard"
	sessionPayload := map[string]any{
		"exists": false,
	}
	runtimePayload := installRuntimeConfigPayload{}
	if session != nil {
		if session.LastStage != "" {
			lastStage = session.LastStage
		}
		if session.Flow != "" {
			flow = session.Flow
		}
		sessionPayload = map[string]any{
			"exists":  true,
			"session": installStateToPayload(session),
		}
	}
	if cfg, err := s.db.GetOrCreateUIConfig(); err == nil && cfg != nil {
		runtimePayload = installRuntimeConfigFromUI(cfg)
		if _, err := os.Stat("/usr/local/bin/veilkey-keycenter"); err == nil {
			if _, err := os.Stat("/usr/local/bin/veilkey-localvault"); err == nil {
				if _, err := os.Stat("/usr/local/bin/veilroot-shell"); err != nil {
					runtimePayload.RuntimeWarning = "partial runtime detected: keycenter/localvault binaries exist on this host, but all-in-one boundary assets are incomplete"
				}
			}
		}
	}
	sessionJSON, _ := json.Marshal(sessionPayload)
	runtimeJSON, _ := json.Marshal(runtimePayload)
	targetNodePlaceholder := installGateTargetNodePlaceholder()
	targetVMIDPlaceholder := installGateTargetVMIDPlaceholder()
	html := fmt.Sprintf(installGateTemplate, flow, lastStage, targetNodePlaceholder, targetVMIDPlaceholder, string(sessionJSON), string(runtimeJSON))
	_, _ = w.Write([]byte(html))
}

func installGateTargetNodePlaceholder() string {
	if value := strings.TrimSpace(os.Getenv("VEILKEY_PROXMOX_DEFAULT_NODE")); value != "" {
		return value
	}
	if host, err := os.Hostname(); err == nil {
		host = strings.TrimSpace(host)
		if host != "" {
			return host
		}
	}
	return "proxmox-node"
}

func installGateTargetVMIDPlaceholder() string {
	if value := strings.TrimSpace(os.Getenv("VEILKEY_PROXMOX_DEFAULT_VMID")); value != "" {
		return value
	}
	return "next available VMID"
}
