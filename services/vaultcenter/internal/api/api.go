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
	"veilkey-vaultcenter/internal/crypto"
	"veilkey-vaultcenter/internal/db"
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
	return os.Getenv("VEILKEY_BULK_APPLY_DIR")
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

func pathVal(r *http.Request, key string) string {
	return strings.TrimSpace(r.PathValue(key))
}

func decodeJSON(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
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
