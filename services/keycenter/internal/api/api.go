package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
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
	updateMu      sync.RWMutex
	updateState   systemUpdateState
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
	srv := &Server{db: database, kek: kek, locked: locked, trustedIPs: ipMap, trustedCIDRs: cidrs, timeouts: DefaultTimeouts(), unlockLimiter: NewUnlockRateLimiter(), httpClient: InitHTTPClientFromEnv()}
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

func renderInstallGate(w http.ResponseWriter, session *db.InstallSession) {
	lastStage := "pending"
	flow := "wizard"
	if session != nil {
		if session.LastStage != "" {
			lastStage = session.LastStage
		}
		if session.Flow != "" {
			flow = session.Flow
		}
	}
	html := fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>VeilKey Install Gate</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,system-ui,sans-serif;background:#111827;color:#f9fafb;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:20px}
.card{max-width:720px;width:100%%;background:#1f2937;border:1px solid #374151;border-radius:20px;padding:32px;box-shadow:0 18px 50px rgba(0,0,0,.32)}
.eyebrow{font-size:12px;letter-spacing:.08em;text-transform:uppercase;color:#9ca3af}
h1{font-size:32px;margin-top:8px;margin-bottom:10px}
p{color:#cbd5e1;line-height:1.6}
.row{display:flex;flex-wrap:wrap;gap:10px;margin-top:18px}
.chip{display:inline-block;padding:7px 10px;border-radius:999px;background:#111827;border:1px solid #4b5563;color:#e5e7eb;font-size:13px}
.links{margin-top:22px;display:flex;flex-wrap:wrap;gap:12px}
.links a{color:#93c5fd;text-decoration:none}
</style></head><body>
<div class="card">
<div class="eyebrow">Install Gate</div>
<h1>Finish KeyCenter install before operator access</h1>
<p>This instance is unlocked but the canonical install flow is not complete yet. Operator console routes stay gated until the install session finishes through the web-first bootstrap path.</p>
<div class="row">
<span class="chip">flow: %s</span>
<span class="chip">last_stage: %s</span>
</div>
<div class="links">
<a href="/health">Health</a>
<a href="/ready">Ready</a>
<a href="/approve/install/bootstrap">Bootstrap Input</a>
<a href="/approve/install/custody">Custody Input</a>
</div>
</div></body></html>`, flow, lastStage)
	_, _ = w.Write([]byte(html))
}
