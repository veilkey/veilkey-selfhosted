package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"veilkey-keycenter/internal/crypto"
	"veilkey-keycenter/internal/db"
)

type NodeIdentity struct {
	NodeID    string
	ParentURL string
	Version   int
	IsHKM     bool
}

// Timeouts configures HTTP client timeouts for HKM operations
type Timeouts struct {
	CascadeResolve time.Duration // federated resolve to children (default 5s)
	ParentForward  time.Duration // forward to parent node (default 3s)
	Deploy         time.Duration // deploy to children (default 30s)
}

// DefaultTimeouts returns production-safe defaults
func DefaultTimeouts() Timeouts {
	return Timeouts{
		CascadeResolve: 5 * time.Second,
		ParentForward:  3 * time.Second,
		Deploy:         30 * time.Second,
	}
}

type Server struct {
	db            *db.DB
	kek           []byte
	kekMu         sync.RWMutex
	locked        bool
	salt          []byte
	trustedIPs    map[string]bool
	trustedCIDRs  []*net.IPNet
	identity      *NodeIdentity
	timeouts      Timeouts
	unlockLimiter *UnlockRateLimiter
	httpClient    *http.Client
	bulkApplyDir  string
	updateMu      sync.RWMutex
	updateState   systemUpdateState
	installMu     sync.RWMutex
	installState  installApplyState
}

func (s *Server) isTrustedIPString(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if ip := net.ParseIP(value); ip != nil && ip.IsLoopback() {
		return true
	}
	if s.trustedIPs[value] {
		return true
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return false
	}
	for _, cidr := range s.trustedCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func (s *Server) SetIdentity(identity *NodeIdentity) {
	s.identity = identity
}

func (s *Server) IsHKM() bool {
	return s.identity != nil && s.identity.IsHKM
}

func NewServer(database *db.DB, kek []byte, trustedIPs []string) *Server {
	ipMap := make(map[string]bool)
	var cidrs []*net.IPNet
	for _, entry := range trustedIPs {
		entry = strings.TrimSpace(entry)
		if strings.Contains(entry, "/") {
			_, cidr, err := net.ParseCIDR(entry)
			if err == nil {
				cidrs = append(cidrs, cidr)
				continue
			}
		}
		ipMap[entry] = true
	}
	locked := kek == nil
	srv := &Server{
		db:            database,
		kek:           kek,
		locked:        locked,
		trustedIPs:    ipMap,
		trustedCIDRs:  cidrs,
		timeouts:      DefaultTimeouts(),
		unlockLimiter: NewUnlockRateLimiter(),
		httpClient:    InitHTTPClientFromEnv(),
		bulkApplyDir:  strings.TrimSpace(os.Getenv("VEILKEY_BULK_APPLY_DIR")),
	}
	if database.HasNodeInfo() {
		if info, err := database.GetNodeInfo(); err == nil {
			srv.identity = &NodeIdentity{
				NodeID:    info.NodeID,
				ParentURL: info.ParentURL,
				Version:   info.Version,
				IsHKM:     true,
			}
		}
	}
	return srv
}

// SetTimeouts overrides default timeout settings
func (s *Server) SetTimeouts(t Timeouts) {
	s.timeouts = t
}

func (s *Server) SetBulkApplyDir(dir string) {
	s.bulkApplyDir = strings.TrimSpace(dir)
}

func (s *Server) BulkApplyDir() string {
	if strings.TrimSpace(s.bulkApplyDir) != "" {
		return strings.TrimSpace(s.bulkApplyDir)
	}
	return "/etc/veilkey/bulk-apply"
}

func (s *Server) SetSalt(salt []byte) {
	s.salt = salt
}

func (s *Server) Unlock(kek []byte) error {
	info, err := s.db.GetNodeInfo()
	if err != nil {
		return fmt.Errorf("no node info found: %w", err)
	}
	_, err = crypto.Decrypt(kek, info.DEK, info.DEKNonce)
	if err != nil {
		return fmt.Errorf("invalid password (KEK decryption failed)")
	}

	s.kekMu.Lock()
	s.kek = kek
	s.locked = false
	s.kekMu.Unlock()
	return nil
}

func (s *Server) IsLocked() bool {
	s.kekMu.RLock()
	defer s.kekMu.RUnlock()
	return s.locked
}

type installAccessState struct {
	Exists     bool     `json:"exists"`
	Complete   bool     `json:"complete"`
	SessionID  string   `json:"session_id,omitempty"`
	Flow       string   `json:"flow,omitempty"`
	LastStage  string   `json:"last_stage,omitempty"`
	FinalStage string   `json:"final_stage,omitempty"`
	Planned    []string `json:"planned_stages,omitempty"`
	Completed  []string `json:"completed_stages,omitempty"`
}

func (s *Server) installGateState() (bool, *db.InstallSession) {
	session, err := s.db.GetLatestInstallSession()
	if err != nil || session == nil {
		return true, nil
	}

	completed := make(map[string]bool)
	for _, stage := range decodeStringList(session.CompletedStagesJSON) {
		stage = strings.TrimSpace(strings.ToLower(stage))
		if stage != "" {
			completed[stage] = true
		}
	}
	planned := decodeStringList(session.PlannedStagesJSON)
	if len(planned) > 0 {
		allPlannedDone := true
		for _, stage := range planned {
			stage = strings.TrimSpace(strings.ToLower(stage))
			if stage == "" {
				continue
			}
			if !completed[stage] {
				allPlannedDone = false
				break
			}
		}
		if allPlannedDone {
			return true, session
		}
	}

	switch strings.TrimSpace(strings.ToLower(session.LastStage)) {
	case "complete", "completed", "done", "ready", "final_smoke":
		return true, session
	}

	return false, session
}

func (s *Server) currentInstallAccessState() installAccessState {
	complete, session := s.installGateState()
	if session == nil {
		return installAccessState{Exists: false, Complete: complete}
	}

	planned := decodeStringList(session.PlannedStagesJSON)
	completed := decodeStringList(session.CompletedStagesJSON)
	finalStage := ""
	if len(planned) > 0 {
		finalStage = planned[len(planned)-1]
	}

	return installAccessState{
		Exists:     true,
		Complete:   complete,
		SessionID:  session.SessionID,
		Flow:       session.Flow,
		LastStage:  session.LastStage,
		FinalStage: finalStage,
		Planned:    planned,
		Completed:  completed,
	}
}

func (s *Server) requireUnlocked(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.IsLocked() {
			s.respondError(w, http.StatusServiceUnavailable, "server is locked. POST /api/unlock with password to unlock.")
			return
		}
		next(w, r)
	}
}

func (s *Server) requireInstallComplete(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := s.currentInstallAccessState()
		if state.Complete {
			next(w, r)
			return
		}
		message := "install flow is not complete. Use the web-first install flow or install wizard before accessing operational APIs."
		if state.Exists && state.FinalStage != "" {
			message = fmt.Sprintf("%s latest session=%s last_stage=%s final_stage=%s", message, state.SessionID, state.LastStage, state.FinalStage)
		}
		s.respondError(w, http.StatusServiceUnavailable, message)
	}
}

func (s *Server) requireReadyForOps(next http.HandlerFunc) http.HandlerFunc {
	return s.requireUnlocked(s.requireInstallComplete(next))
}

func (s *Server) handleUnlock(w http.ResponseWriter, r *http.Request) {
	if !s.IsLocked() {
		s.respondJSON(w, http.StatusOK, map[string]interface{}{"status": "already_unlocked"})
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
		s.respondError(w, http.StatusBadRequest, "password is required")
		return
	}

	kek := crypto.DeriveKEK(req.Password, s.salt)
	if err := s.Unlock(kek); err != nil {
		log.Printf("Unlock failed from %s: %v", r.RemoteAddr, err)
		s.respondError(w, http.StatusUnauthorized, "invalid password")
		return
	}

	log.Printf("Server unlocked by %s", r.RemoteAddr)
	s.respondJSON(w, http.StatusOK, map[string]interface{}{"status": "unlocked"})
}

func (s *Server) requireTrustedIP(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		allowed := s.isTrustedIPString(clientIP)
		if (len(s.trustedIPs) > 0 || len(s.trustedCIDRs) > 0) && !allowed {
			log.Printf("BLOCKED %s %s %s (untrusted IP)", clientIP, r.Method, r.URL.Path)
			s.respondError(w, http.StatusForbidden, "access denied")
			return
		}
		next(w, r)
	}
}

func (s *Server) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

func (s *Server) respondError(w http.ResponseWriter, status int, message string) {
	s.respondJSON(w, status, map[string]string{"error": message})
}

func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	if s.IsLocked() {
		status = "locked"
	}
	s.respondJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (s *Server) Ready(w http.ResponseWriter, r *http.Request) {
	if s.IsLocked() {
		s.respondError(w, http.StatusServiceUnavailable, "server is locked")
		return
	}
	if complete, _ := s.installGateState(); !complete {
		s.respondError(w, http.StatusServiceUnavailable, "install is not complete")
		return
	}
	if err := s.db.Ping(); err != nil {
		s.respondError(w, http.StatusServiceUnavailable, "database not ready")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		s.handleOperatorShellEntry(w, r)
	})

	mux.HandleFunc("/health", s.Health)
	mux.HandleFunc("/ready", s.Ready)
	mux.HandleFunc("POST /api/unlock", s.requireTrustedIP(s.unlockLimiter.Middleware(s.handleUnlock)))
	s.SetupAPIRoutes(mux)
	s.SetupAdminRoutes(mux)
	if s.IsHKM() {
		s.SetupHKMRoutes(mux)
	}

	return logMiddleware(mux)
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

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
	}
	sessionJSON, _ := json.Marshal(sessionPayload)
	runtimeJSON, _ := json.Marshal(runtimePayload)
	html := fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>VeilKey Install Gate</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,system-ui,sans-serif;background:radial-gradient(circle at top,#16315f 0,#0f172a 42%%,#0a1020 100%%);color:#f9fafb;min-height:100vh;padding:24px}
.shell{max-width:1120px;margin:0 auto;display:grid;grid-template-columns:1.15fr .85fr;gap:18px}
.card{width:100%%;background:rgba(14,23,43,.9);border:1px solid rgba(148,163,184,.16);border-radius:24px;padding:28px;box-shadow:0 18px 60px rgba(0,0,0,.34);backdrop-filter:blur(12px)}
.eyebrow{font-size:12px;letter-spacing:.08em;text-transform:uppercase;color:#93c5fd}
h1{font-size:34px;line-height:1.1;margin-top:8px;margin-bottom:10px}
p{color:#cbd5e1;line-height:1.6}
.row{display:flex;flex-wrap:wrap;gap:10px;margin-top:18px}
.chip{display:inline-block;padding:7px 10px;border-radius:999px;background:#0b1220;border:1px solid #334155;color:#e5e7eb;font-size:13px}
.lang-switch{margin-top:18px;display:flex;gap:8px}
.lang-switch button{border:1px solid #334155;background:#101827;color:#cbd5e1;border-radius:999px;padding:8px 12px;cursor:pointer}
.lang-switch button.active{background:#38bdf8;color:#082f49;border-color:#38bdf8}
.hero{display:grid;grid-template-columns:1fr 220px;gap:18px;align-items:start}
.hero-panel{background:linear-gradient(180deg,rgba(56,189,248,.13),rgba(15,23,42,.08));border:1px solid rgba(56,189,248,.18);border-radius:20px;padding:16px}
.hero-panel strong{display:block;font-size:14px}
.hero-panel span{display:block;margin-top:6px;color:#cbd5e1;font-size:13px;line-height:1.5}
.section-title{font-size:15px;font-weight:700;margin-top:22px;margin-bottom:10px}
.field-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}
.field{display:flex;flex-direction:column;gap:6px}
.field label{font-size:12px;color:#cbd5e1}
.field input,.field select{width:100%%;background:#0b1220;color:#f8fafc;border:1px solid #334155;border-radius:12px;padding:12px}
.field.full{grid-column:1/-1}
.field small{color:#94a3b8;font-size:12px}
.summary{margin-top:16px;padding:14px;border-radius:16px;background:#0b1220;border:1px solid #334155}
.summary strong{display:block;font-size:14px}
.summary ul{margin-top:8px;padding-left:18px;color:#dbeafe}
.summary li{margin:6px 0}
.actions{display:flex;flex-wrap:wrap;gap:12px;margin-top:18px}
.btn{border:0;border-radius:12px;padding:12px 16px;font-weight:700;cursor:pointer}
.btn-primary{background:#38bdf8;color:#082f49}
.btn-soft{background:#1e293b;color:#e2e8f0;border:1px solid #334155}
.note{margin-top:12px;font-size:12px;color:#94a3b8}
.status-box{margin-top:18px;padding:14px;border-radius:16px;background:#0b1220;border:1px solid #334155}
.status-box pre{white-space:pre-wrap;font-size:12px;line-height:1.5;color:#dbeafe}
.links{margin-top:22px;display:flex;flex-wrap:wrap;gap:12px}
.links a{color:#93c5fd;text-decoration:none}
details{margin-top:18px;border:1px solid #334155;border-radius:16px;background:#0b1220}
summary{cursor:pointer;list-style:none;padding:14px 16px;font-weight:700}
summary::-webkit-details-marker{display:none}
.advanced{padding:0 16px 16px}
.advanced-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}
.advanced .field input,.advanced .field select{background:#111827}
@media (max-width: 920px){.shell{grid-template-columns:1fr}.field-grid,.advanced-grid,.hero{grid-template-columns:1fr}}
</style></head><body>
<div class="shell">
<div class="card">
<div class="eyebrow">Linux Quick Install</div>
<h1 id="title">이 리눅스 서버에 VeilKey 설치</h1>
<p id="subtitle">LXC 여부를 가정하지 않고, 일반 리눅스 서버 기준으로 빠르게 시작합니다. 내부용 기본값은 자동으로 채우고 필요한 항목만 먼저 받습니다.</p>
<div class="row">
<span class="chip">flow: %s</span>
<span class="chip">last_stage: %s</span>
<span class="chip" id="target-chip">target: linux-host</span>
</div>
<div class="lang-switch">
<button type="button" id="lang-ko" class="active">한국어</button>
<button type="button" id="lang-en">English</button>
</div>
<div class="hero">
<div>
<div class="section-title" id="quick-title">빠른 설치 정보</div>
<div class="field-grid">
<div class="field">
<label for="public_host" id="public-host-label">접속 주소 또는 도메인</label>
<input id="public_host" placeholder="wizard.example.internal">
<small id="public-host-help">나중에 사용자가 접속할 KeyCenter 주소입니다. 비워두면 현재 호스트를 기준으로 추정합니다.</small>
</div>
<div class="field">
<label for="tls_mode" id="tls-mode-label">TLS 방식</label>
<select id="tls_mode">
<option value="later">나중에 설정</option>
<option value="existing">기존 인증서 사용</option>
</select>
<small id="tls-mode-help">빠른 검증은 HTTP로 시작하고, 운영 전 TLS를 붙일 수 있습니다.</small>
</div>
<div class="field full">
<label for="install_root" id="install-root-label">설치 대상 루트</label>
<input id="install_root" placeholder="/">
<small id="install-root-help">일반 리눅스 서버면 /, chroot나 별도 rootfs면 해당 경로를 입력합니다.</small>
</div>
<div class="field full">
<label for="localvault_url" id="localvault-label">기존 LocalVault URL (선택)</label>
<input id="localvault_url" placeholder="https://localvault.example.internal">
<small id="localvault-help">올인원 설치가 아니고 기존 LocalVault를 연결할 때만 입력합니다.</small>
</div>
</div>
<div class="summary">
<strong id="summary-title">자동으로 결정되는 내부 설정</strong>
<ul>
<li id="summary-profile">설치 프로파일: linux-host 계열 자동 선택</li>
<li id="summary-script">설치 스크립트: 서버 허용 목록에서 자동 사용</li>
<li id="summary-session">설치 단계: language → bootstrap → final_smoke</li>
</ul>
</div>
<div class="actions">
<button class="btn btn-primary" id="save-quick">빠른 설치 저장</button>
<button class="btn btn-primary" id="apply-install">설치 실행</button>
<button class="btn btn-soft" id="reload-state">상태 새로고침</button>
</div>
<div class="note" id="note-text">고급 옵션을 열지 않아도 일반 리눅스 서버 기준 기본값으로 바로 저장하고 실행할 수 있습니다.</div>
<div class="status-box"><pre id="wizard-status">Loading install wizard state…</pre></div>
<div class="links">
<a href="/health">Health</a>
<a href="/ready">Ready</a>
<a href="/approve/install/bootstrap" id="bootstrap-link">Bootstrap Input</a>
<a href="/approve/install/custody" id="custody-link">Custody Input</a>
</div>
<details>
<summary id="advanced-summary">고급 설정 열기</summary>
<div class="advanced">
<div class="advanced-grid">
<div class="field">
<label for="flow">Flow</label>
<select id="flow"><option value="wizard">wizard</option><option value="quickstart">quickstart</option><option value="advanced">advanced</option></select>
</div>
<div class="field">
<label for="deployment_mode">Deployment Mode</label>
<input id="deployment_mode" placeholder="host-service">
</div>
<div class="field">
<label for="install_scope">Install Scope</label>
<input id="install_scope" placeholder="host-only">
</div>
<div class="field">
<label for="bootstrap_mode">Bootstrap Mode</label>
<input id="bootstrap_mode" placeholder="email">
</div>
<div class="field">
<label for="mail_transport">Mail Transport</label>
<input id="mail_transport" placeholder="smtp">
</div>
<div class="field">
<label for="install_profile">Install Profile</label>
<input id="install_profile" placeholder="linux-host">
</div>
<div class="field full">
<label for="install_script">Install Script</label>
<input id="install_script" placeholder="/opt/veilkey-selfhosted-repo/installer/install.sh">
</div>
<div class="field full">
<label for="install_workdir">Install Workdir</label>
<input id="install_workdir" placeholder="/opt/veilkey-selfhosted-repo/installer">
</div>
<div class="field full">
<label for="keycenter_url">KeyCenter URL</label>
<input id="keycenter_url" placeholder="https://keycenter.example.internal">
</div>
<div class="field">
<label for="tls_cert_path">TLS Cert Path</label>
<input id="tls_cert_path" placeholder="/etc/veilkey/tls/server.crt">
</div>
<div class="field">
<label for="tls_key_path">TLS Key Path</label>
<input id="tls_key_path" placeholder="/etc/veilkey/tls/server.key">
</div>
<div class="field full">
<label for="tls_ca_path">TLS CA Path</label>
<input id="tls_ca_path" placeholder="/etc/veilkey/tls/ca.crt">
</div>
<div class="field full">
<label for="planned_stages">Planned Stages</label>
<input id="planned_stages" placeholder="language,bootstrap,final_smoke">
</div>
</div>
<div class="actions">
<button class="btn btn-soft" id="save-session">세션만 저장</button>
<button class="btn btn-soft" id="save-runtime">런타임 설정만 저장</button>
</div>
</div>
</details>
</div>
</div>
<div class="card">
<div class="eyebrow" id="side-eyebrow">Why This Page Exists</div>
<h1 style="font-size:24px" id="side-title">리눅스 서버 설치를 먼저 끝냅니다</h1>
<p id="side-copy">브라우저는 설치 의도와 운영 정책만 저장합니다. 실제 설치 실행은 KeyCenter가 서버 측 runner를 통해 수행합니다.</p>
<div class="summary">
<strong id="steps-title">권장 순서</strong>
<ul>
<li id="step-1">1. 접속 주소와 설치 루트를 입력합니다.</li>
<li id="step-2">2. 필요하면 기존 LocalVault URL을 넣습니다.</li>
<li id="step-3">3. 빠른 설치 저장 후 설치 실행을 누릅니다.</li>
<li id="step-4">4. 완료 후 /ready 가 열리면 운영 콘솔로 진입합니다.</li>
</ul>
</div>
<div class="status-box"><pre id="wizard-preview"></pre></div>
</div>
</div>
<script>
const initialSessionState = %s;
const initialRuntimeConfig = %s;
const copy = {
  ko: {
    title: '이 리눅스 서버에 VeilKey 설치',
    subtitle: 'LXC 여부를 가정하지 않고, 일반 리눅스 서버 기준으로 빠르게 시작합니다. 내부용 기본값은 자동으로 채우고 필요한 항목만 먼저 받습니다.',
    quickTitle: '빠른 설치 정보',
    publicHostLabel: '접속 주소 또는 도메인',
    publicHostHelp: '나중에 사용자가 접속할 KeyCenter 주소입니다. 비워두면 현재 호스트를 기준으로 추정합니다.',
    tlsModeLabel: 'TLS 방식',
    tlsModeHelp: '빠른 검증은 HTTP로 시작하고, 운영 전 TLS를 붙일 수 있습니다.',
    installRootLabel: '설치 대상 루트',
    installRootHelp: '일반 리눅스 서버면 /, chroot나 별도 rootfs면 해당 경로를 입력합니다.',
    localvaultLabel: '기존 LocalVault URL (선택)',
    localvaultHelp: '올인원 설치가 아니고 기존 LocalVault를 연결할 때만 입력합니다.',
    summaryTitle: '자동으로 결정되는 내부 설정',
    summaryProfile: '설치 프로파일: linux-host 계열 자동 선택',
    summaryScript: '설치 스크립트: 서버 허용 목록에서 자동 사용',
    summarySession: '설치 단계: language -> bootstrap -> final_smoke',
    saveQuick: '빠른 설치 저장',
    applyInstall: '설치 실행',
    reload: '상태 새로고침',
    saveSession: '세션만 저장',
    saveRuntime: '런타임 설정만 저장',
    note: '고급 옵션을 열지 않아도 일반 리눅스 서버 기준 기본값으로 바로 저장하고 실행할 수 있습니다.',
    advanced: '고급 설정 열기',
    sideEyebrow: 'Linux Install Guide',
    sideTitle: '리눅스 서버 설치를 먼저 끝냅니다',
    sideCopy: '브라우저는 설치 의도와 운영 정책만 저장합니다. 실제 설치 실행은 KeyCenter가 서버 측 runner를 통해 수행합니다.',
    stepsTitle: '권장 순서',
    step1: '1. 접속 주소와 설치 루트를 입력합니다.',
    step2: '2. 필요하면 기존 LocalVault URL을 넣습니다.',
    step3: '3. 빠른 설치 저장 후 설치 실행을 누릅니다.',
    step4: '4. 완료 후 /ready가 열리면 운영 콘솔로 진입합니다.',
    bootstrap: 'Bootstrap 입력',
    custody: 'Custody 입력',
    loaded: '리눅스 설치 마법사 상태를 불러왔습니다.',
    running: '설치 실행이 진행 중입니다.',
    start: '설치 실행을 시작합니다...',
    saveOk: '빠른 설치 설정을 저장했습니다.',
    saveSessionOk: '설치 세션을 저장했습니다.',
    saveRuntimeOk: '런타임 설정을 저장했습니다.',
    loadFail: '설치 상태를 불러오지 못했습니다: ',
    saveFail: '빠른 설치 저장 실패: ',
    sessionFail: '세션 저장 실패: ',
    runtimeFail: '런타임 설정 저장 실패: ',
    applyFail: '설치 실행 실패: ',
    applyStarted: '설치 실행을 시작했습니다.'
  },
  en: {
    title: 'Install VeilKey on this Linux server',
    subtitle: 'This flow assumes a general Linux host, not LXC. Internal defaults are derived automatically so you only fill the minimum fields first.',
    quickTitle: 'Quick Install',
    publicHostLabel: 'Access host or domain',
    publicHostHelp: 'This becomes the KeyCenter address operators will open later. Leave blank to derive from the current host.',
    tlsModeLabel: 'TLS mode',
    tlsModeHelp: 'Start with HTTP for validation, then attach TLS before production exposure.',
    installRootLabel: 'Install root',
    installRootHelp: 'Use / for a normal Linux host, or another root path for chroot/rootfs installs.',
    localvaultLabel: 'Existing LocalVault URL (optional)',
    localvaultHelp: 'Fill this only when you are connecting an existing LocalVault instead of all-in-one install.',
    summaryTitle: 'Derived internal settings',
    summaryProfile: 'Install profile: auto-select linux-host family',
    summaryScript: 'Install script: auto-use server allowlisted runner',
    summarySession: 'Install stages: language -> bootstrap -> final_smoke',
    saveQuick: 'Save Quick Setup',
    applyInstall: 'Apply Install',
    reload: 'Reload State',
    saveSession: 'Save Session Only',
    saveRuntime: 'Save Runtime Only',
    note: 'You can save and run directly with Linux-host defaults without opening advanced settings.',
    advanced: 'Open Advanced Settings',
    sideEyebrow: 'Why This Page Exists',
    sideTitle: 'Finish Linux host install first',
    sideCopy: 'The browser stores install intent and policy only. Actual installation runs through a fixed server-side runner controlled by KeyCenter.',
    stepsTitle: 'Recommended order',
    step1: '1. Enter the access host and install root.',
    step2: '2. Add an existing LocalVault URL if needed.',
    step3: '3. Save the quick setup, then run install.',
    step4: '4. Enter the operator console after /ready opens.',
    bootstrap: 'Bootstrap Input',
    custody: 'Custody Input',
    loaded: 'Linux install wizard state loaded.',
    running: 'Install apply is running.',
    start: 'Starting install apply...',
    saveOk: 'Quick install settings saved.',
    saveSessionOk: 'Install session saved.',
    saveRuntimeOk: 'Runtime config saved.',
    loadFail: 'Failed to load install state: ',
    saveFail: 'Failed to save quick install: ',
    sessionFail: 'Failed to save install session: ',
    runtimeFail: 'Failed to save runtime config: ',
    applyFail: 'Failed to start install apply: ',
    applyStarted: 'Install apply started.'
  }
};
const statusEl = document.getElementById('wizard-status');
const previewEl = document.getElementById('wizard-preview');
const quickFields = {
  public_host: document.getElementById('public_host'),
  tls_mode: document.getElementById('tls_mode')
};
const fields = {
  flow: document.getElementById('flow'),
  deployment_mode: document.getElementById('deployment_mode'),
  install_scope: document.getElementById('install_scope'),
  bootstrap_mode: document.getElementById('bootstrap_mode'),
  mail_transport: document.getElementById('mail_transport'),
  planned_stages: document.getElementById('planned_stages'),
  install_profile: document.getElementById('install_profile'),
  install_root: document.getElementById('install_root'),
  install_script: document.getElementById('install_script'),
  install_workdir: document.getElementById('install_workdir'),
  keycenter_url: document.getElementById('keycenter_url'),
  localvault_url: document.getElementById('localvault_url'),
  tls_cert_path: document.getElementById('tls_cert_path'),
  tls_key_path: document.getElementById('tls_key_path'),
  tls_ca_path: document.getElementById('tls_ca_path')
};
let currentLang = 'ko';

function guessCurrentHost() {
  const { protocol, hostname, port } = window.location;
  if (!hostname) {
    return '';
  }
  return protocol + '//' + hostname + (port ? ':' + port : '');
}

function deriveInstallProfile() {
  return 'linux-host';
}

function deriveInstallScript(existingValue) {
  return existingValue || '/opt/veilkey-selfhosted-repo/installer/install.sh';
}

function deriveInstallWorkdir(existingValue) {
  return existingValue || '/opt/veilkey-selfhosted-repo/installer';
}

function syncDerivedFields() {
  const rawHost = quickFields.public_host.value.trim();
  const guessed = guessCurrentHost();
  const hasScheme = rawHost.startsWith('http://') || rawHost.startsWith('https://');
  const tlsLater = quickFields.tls_mode.value === 'later';
  let baseURL = rawHost;
  if (baseURL && !hasScheme) {
    baseURL = (tlsLater ? 'http://' : 'https://') + baseURL;
  }
  if (!baseURL) {
    baseURL = guessed;
  }
  fields.install_profile.value = deriveInstallProfile();
  fields.install_script.value = deriveInstallScript(fields.install_script.value.trim());
  fields.install_workdir.value = deriveInstallWorkdir(fields.install_workdir.value.trim());
  fields.keycenter_url.value = baseURL;
  fields.deployment_mode.value = 'host-service';
  fields.install_scope.value = fields.localvault_url.value.trim() ? 'host+existing-localvault' : 'host-only';
  fields.bootstrap_mode.value = 'email';
  fields.mail_transport.value = 'smtp';
  fields.flow.value = 'wizard';
  fields.planned_stages.value = 'language,bootstrap,final_smoke';
  if (tlsLater) {
    fields.tls_cert_path.value = '';
    fields.tls_key_path.value = '';
    fields.tls_ca_path.value = '';
  }
}

function setStatus(message) {
  statusEl.textContent = message;
}

function renderPreview() {
  syncDerivedFields();
  const preview = {
    language: currentLang,
    target: 'linux-host',
    session: {
      flow: fields.flow.value,
      deployment_mode: fields.deployment_mode.value,
      install_scope: fields.install_scope.value,
      bootstrap_mode: fields.bootstrap_mode.value,
      mail_transport: fields.mail_transport.value,
      planned_stages: fields.planned_stages.value.split(',').map((item) => item.trim()).filter(Boolean)
    },
    quick_setup: {
      public_host: quickFields.public_host.value,
      tls_mode: quickFields.tls_mode.value,
      install_root: fields.install_root.value,
      localvault_url: fields.localvault_url.value
    },
    runtime_config: {
      install_profile: fields.install_profile.value,
      install_root: fields.install_root.value,
      install_script: fields.install_script.value,
      install_workdir: fields.install_workdir.value,
      keycenter_url: fields.keycenter_url.value,
      localvault_url: fields.localvault_url.value,
      tls_cert_path: fields.tls_cert_path.value,
      tls_key_path: fields.tls_key_path.value,
      tls_ca_path: fields.tls_ca_path.value
    },
    apply: window.installApplyState || null
  };
  previewEl.textContent = JSON.stringify(preview, null, 2);
}

function setLanguage(lang) {
  currentLang = lang;
  document.getElementById('lang-ko').classList.toggle('active', lang === 'ko');
  document.getElementById('lang-en').classList.toggle('active', lang === 'en');
  const t = copy[lang];
  document.getElementById('title').textContent = t.title;
  document.getElementById('subtitle').textContent = t.subtitle;
  document.getElementById('quick-title').textContent = t.quickTitle;
  document.getElementById('public-host-label').textContent = t.publicHostLabel;
  document.getElementById('public-host-help').textContent = t.publicHostHelp;
  document.getElementById('tls-mode-label').textContent = t.tlsModeLabel;
  document.getElementById('tls-mode-help').textContent = t.tlsModeHelp;
  document.getElementById('install-root-label').textContent = t.installRootLabel;
  document.getElementById('install-root-help').textContent = t.installRootHelp;
  document.getElementById('localvault-label').textContent = t.localvaultLabel;
  document.getElementById('localvault-help').textContent = t.localvaultHelp;
  document.getElementById('summary-title').textContent = t.summaryTitle;
  document.getElementById('summary-profile').textContent = t.summaryProfile;
  document.getElementById('summary-script').textContent = t.summaryScript;
  document.getElementById('summary-session').textContent = t.summarySession;
  document.getElementById('save-quick').textContent = t.saveQuick;
  document.getElementById('apply-install').textContent = t.applyInstall;
  document.getElementById('reload-state').textContent = t.reload;
  document.getElementById('save-session').textContent = t.saveSession;
  document.getElementById('save-runtime').textContent = t.saveRuntime;
  document.getElementById('note-text').textContent = t.note;
  document.getElementById('advanced-summary').textContent = t.advanced;
  document.getElementById('side-eyebrow').textContent = t.sideEyebrow;
  document.getElementById('side-title').textContent = t.sideTitle;
  document.getElementById('side-copy').textContent = t.sideCopy;
  document.getElementById('steps-title').textContent = t.stepsTitle;
  document.getElementById('step-1').textContent = t.step1;
  document.getElementById('step-2').textContent = t.step2;
  document.getElementById('step-3').textContent = t.step3;
  document.getElementById('step-4').textContent = t.step4;
  document.getElementById('bootstrap-link').textContent = t.bootstrap;
  document.getElementById('custody-link').textContent = t.custody;
}

function applySessionState(data) {
  const session = data && data.exists ? data.session : null;
  fields.flow.value = (session && session.flow) || 'wizard';
  fields.deployment_mode.value = (session && session.deployment_mode) || 'host-service';
  fields.install_scope.value = (session && session.install_scope) || 'host-only';
  fields.bootstrap_mode.value = (session && session.bootstrap_mode) || 'email';
  fields.mail_transport.value = (session && session.mail_transport) || 'smtp';
  fields.planned_stages.value = session && Array.isArray(session.planned_stages) ? session.planned_stages.join(',') : 'language,bootstrap,final_smoke';
}

function applyRuntimeConfig(data) {
  fields.install_profile.value = data.install_profile || 'linux-host';
  fields.install_root.value = data.install_root || '/';
  fields.install_script.value = data.install_script || '';
  fields.install_workdir.value = data.install_workdir || '';
  fields.keycenter_url.value = data.keycenter_url || '';
  quickFields.public_host.value = (data.keycenter_url || '').replace(/^https?:\/\//, '');
  fields.localvault_url.value = data.localvault_url || '';
  fields.tls_cert_path.value = data.tls_cert_path || '';
  fields.tls_key_path.value = data.tls_key_path || '';
  fields.tls_ca_path.value = data.tls_ca_path || '';
  quickFields.tls_mode.value = (data.tls_cert_path || data.tls_key_path) ? 'existing' : 'later';
}

async function request(path, options) {
  const response = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...(options || {})
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(body.error || ('HTTP ' + response.status));
  }
  return body;
}

async function reloadState() {
  try {
    const [sessionResp, runtimeResp, applyResp] = await Promise.all([
      request('/api/install/state'),
      request('/api/install/runtime-config'),
      request('/api/install/apply')
    ]);
    applySessionState(sessionResp);
    applyRuntimeConfig(runtimeResp);
    window.installApplyState = applyResp;
    renderPreview();
    setStatus(applyResp.install_running ? copy[currentLang].running : copy[currentLang].loaded);
  } catch (error) {
    setStatus(copy[currentLang].loadFail + error.message);
  }
}

async function saveSession() {
  try {
    syncDerivedFields();
    const existing = await request('/api/install/state');
    const payload = {
      session_id: existing.exists && existing.session ? existing.session.session_id : '',
      version: existing.exists && existing.session ? existing.session.version : 1,
      language: currentLang,
      quickstart: existing.exists && existing.session ? !!existing.session.quickstart : false,
      flow: fields.flow.value || 'wizard',
      deployment_mode: fields.deployment_mode.value || 'host-service',
      install_scope: fields.install_scope.value || 'host-only',
      bootstrap_mode: fields.bootstrap_mode.value || 'email',
      mail_transport: fields.mail_transport.value || 'smtp',
      planned_stages: fields.planned_stages.value.split(',').map((item) => item.trim()).filter(Boolean),
      completed_stages: existing.exists && existing.session ? (existing.session.completed_stages || []) : [],
      last_stage: existing.exists && existing.session && existing.session.last_stage ? existing.session.last_stage : 'language'
    };
    await request('/api/install/session', { method: 'POST', body: JSON.stringify(payload) });
    await reloadState();
    setStatus(copy[currentLang].saveSessionOk);
  } catch (error) {
    setStatus(copy[currentLang].sessionFail + error.message);
  }
}

async function saveRuntimeConfig() {
  try {
    syncDerivedFields();
    const payload = {
      install_profile: fields.install_profile.value,
      install_root: fields.install_root.value,
      install_script: fields.install_script.value,
      install_workdir: fields.install_workdir.value,
      keycenter_url: fields.keycenter_url.value,
      localvault_url: fields.localvault_url.value,
      tls_cert_path: fields.tls_cert_path.value,
      tls_key_path: fields.tls_key_path.value,
      tls_ca_path: fields.tls_ca_path.value
    };
    await request('/api/install/runtime-config', { method: 'PATCH', body: JSON.stringify(payload) });
    renderPreview();
    setStatus(copy[currentLang].saveRuntimeOk);
  } catch (error) {
    setStatus(copy[currentLang].runtimeFail + error.message);
  }
}

async function saveQuickSetup() {
  try {
    await saveSession();
    await saveRuntimeConfig();
    setStatus(copy[currentLang].saveOk);
  } catch (error) {
    setStatus(copy[currentLang].saveFail + error.message);
  }
}

async function applyInstall() {
  try {
    syncDerivedFields();
    setStatus(copy[currentLang].start);
    const resp = await request('/api/install/apply', { method: 'POST', body: JSON.stringify({}) });
    window.installApplyState = resp;
    renderPreview();
    setStatus(copy[currentLang].applyStarted);
    setTimeout(reloadState, 500);
  } catch (error) {
    setStatus(copy[currentLang].applyFail + error.message);
  }
}

document.getElementById('lang-ko').addEventListener('click', () => { setLanguage('ko'); renderPreview(); });
document.getElementById('lang-en').addEventListener('click', () => { setLanguage('en'); renderPreview(); });
document.getElementById('save-quick').addEventListener('click', saveQuickSetup);
document.getElementById('save-session').addEventListener('click', saveSession);
document.getElementById('save-runtime').addEventListener('click', saveRuntimeConfig);
document.getElementById('apply-install').addEventListener('click', applyInstall);
document.getElementById('reload-state').addEventListener('click', reloadState);
Object.values(fields).forEach((el) => el.addEventListener('input', renderPreview));
Object.values(quickFields).forEach((el) => el.addEventListener('input', renderPreview));

setLanguage((initialSessionState.session && initialSessionState.session.language) || 'ko');
applySessionState(initialSessionState);
applyRuntimeConfig(initialRuntimeConfig);
renderPreview();
reloadState();
</script></body></html>`, flow, lastStage, string(sessionJSON), string(runtimeJSON))
	_, _ = w.Write([]byte(html))
}
