package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"veilkey-vaultcenter/internal/api/admin"
	"veilkey-vaultcenter/internal/api/approval"
	"veilkey-vaultcenter/internal/api/bulk"
	"veilkey-vaultcenter/internal/api/hkm"
	chain "github.com/veilkey/veilkey-chain"
	"github.com/veilkey/veilkey-go-package/agentapi"
	"github.com/veilkey/veilkey-go-package/crypto"
	"github.com/veilkey/veilkey-go-package/ratelimit"
	"github.com/veilkey/veilkey-go-package/tlsutil"
	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"
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
	db             *db.DB
	kek            []byte
	kekMu          sync.RWMutex
	locked         bool
	salt           []byte
	trustedIPs     map[string]bool
	trustedCIDRs   []*net.IPNet
	identity       *NodeIdentity
	timeouts       Timeouts
	unlockLimiter  *ratelimit.UnlockRateLimiter
	httpClient     *http.Client
	chainClient    *chain.Client
	chainStore     chain.Store
	chainHome      string
	chainNodeID    string
	bulkApplyDir   string
	updateMu       sync.RWMutex
	updateState    systemUpdateState
	approvalHandler *approval.Handler
	adminHandler    *admin.Handler
	hkmHandler      *hkm.Handler
	bulkHandler     *bulk.Handler
}

// ── hkm.Deps implementation ──────────────────────────────────────────────────

func (s *Server) DB() *db.DB { return s.db }

func (s *Server) HTTPClient() *http.Client { return s.httpClient }

func (s *Server) GetKEK() []byte {
	s.kekMu.RLock()
	defer s.kekMu.RUnlock()
	k := make([]byte, len(s.kek))
	copy(k, s.kek)
	return k
}

func (s *Server) GetLocalDEK() ([]byte, error) {
	info, err := s.db.GetNodeInfo()
	if err != nil {
		return nil, fmt.Errorf("no node info: %w", err)
	}
	kek := s.GetKEK()
	dek, err := crypto.Decrypt(kek, info.DEK, info.DEKNonce)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt DEK: %w", err)
	}
	return dek, nil
}

func (s *Server) CascadeResolveTimeout() time.Duration { return s.timeouts.CascadeResolve }

func (s *Server) ParentForwardTimeout() time.Duration { return s.timeouts.ParentForward }

func (s *Server) DeployTimeout() time.Duration { return s.timeouts.Deploy }

func (s *Server) IsTrustedIPString(ip string) bool { return s.isTrustedIPString(ip) }

func (s *Server) SaveAuditEvent(entityType, entityID, action, actorType, actorID, reason, source string, before, after map[string]any) {
	s.saveAuditEvent(entityType, entityID, action, actorType, actorID, reason, source, before, after)
}

// ── chain TX submission ─────────────────────────────────────────────────────

// SetChainClient sets the CometBFT chain client. nil disables chain mode (DB fallback).
func (s *Server) SetChainClient(c *chain.Client)      { s.chainClient = c }
func (s *Server) SetChainHome(home string)             { s.chainHome = home }
func (s *Server) SetChainNodeID(nodeID string)         { s.chainNodeID = nodeID }

// ChainInfo returns genesis JSON and persistent_peers for child nodes joining the chain.
// Returns nil, "" if chain is not enabled.
func (s *Server) ChainInfo() (genesisJSON []byte, persistentPeers string) {
	if s.chainHome == "" {
		return nil, ""
	}
	genesis, err := os.ReadFile(filepath.Join(s.chainHome, "config", "genesis.json"))
	if err != nil {
		return nil, ""
	}
	if s.chainNodeID != "" {
		// peer format: nodeID@host:port — host is the vaultcenter's listen address
		addr := strings.TrimSpace(os.Getenv("VEILKEY_CHAIN_P2P_ADDR"))
		if addr == "" {
			addr = "127.0.0.1:26656"
		}
		persistentPeers = s.chainNodeID + "@" + addr
	}
	return genesis, persistentPeers
}

// SubmitTx submits a write TX and blocks until committed.
func (s *Server) SubmitTx(ctx context.Context, txType chain.TxType, payload any) (string, error) {
	actor := txActorFromCtx(ctx)
	if s.chainClient != nil {
		env, err := chain.BuildEnvelope(txType, payload, actor)
		if err != nil {
			return "", err
		}
		txBytes, err := chain.MarshalEnvelope(env)
		if err != nil {
			return "", err
		}
		result, err := s.chainClient.BroadcastTxCommitRaw(ctx, txBytes)
		if err != nil {
			return "", err
		}
		if result.CheckTx.Code != 0 {
			return "", fmt.Errorf("chain check: %s", result.CheckTx.Log)
		}
		if result.TxResult.Code != 0 {
			return "", fmt.Errorf("chain exec: %s", result.TxResult.Log)
		}
		return result.TxResult.Log, nil
	}
	// fallback: executor 직접 호출
	env, err := chain.BuildEnvelope(txType, payload, actor)
	if err != nil {
		return "", err
	}
	code, resultLog := chain.Execute(s.chainStore, env)
	if code != 0 {
		return "", fmt.Errorf(resultLog)
	}
	return resultLog, nil
}

// SubmitTxAsync submits a write TX without waiting for block inclusion.
func (s *Server) SubmitTxAsync(ctx context.Context, txType chain.TxType, payload any) error {
	actor := txActorFromCtx(ctx)
	if s.chainClient != nil {
		env, err := chain.BuildEnvelope(txType, payload, actor)
		if err != nil {
			return err
		}
		txBytes, err := chain.MarshalEnvelope(env)
		if err != nil {
			return err
		}
		return s.chainClient.BroadcastTxSyncRaw(ctx, txBytes)
	}
	// fallback
	env, err := chain.BuildEnvelope(txType, payload, actor)
	if err != nil {
		return err
	}
	code, resultLog := chain.Execute(s.chainStore, env)
	if code != 0 {
		return fmt.Errorf(resultLog)
	}
	return nil
}

// ── admin.Deps implementation ────────────────────────────────────────────────

func (s *Server) DecryptAgentDEK(encDEK, encNonce []byte) ([]byte, error) {
	if len(encDEK) == 0 {
		return nil, fmt.Errorf("agent has no DEK assigned")
	}
	return crypto.Decrypt(s.GetKEK(), encDEK, encNonce)
}

func (s *Server) FindAgentRecord(hashOrLabel string) (*db.Agent, error) {
	agent, err := s.db.GetAgentRecord(hashOrLabel)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %s", hashOrLabel)
	}
	return agent, nil
}

func (s *Server) FetchAgentCiphertext(agentURL, ref string) (name string, ciphertext []byte, nonce []byte, err error) {
	resp, httpErr := s.httpClient.Get(httputil.JoinPath(agentURL, httputil.AgentPathCipher, ref))
	if httpErr != nil {
		return "", nil, nil, fmt.Errorf("agent unreachable: %w", httpErr)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", nil, nil, fmt.Errorf("agent returned %d", resp.StatusCode)
	}
	var data struct {
		Name       string `json:"name"`
		Ciphertext []byte `json:"ciphertext"`
		Nonce      []byte `json:"nonce"`
	}
	if decErr := json.NewDecoder(resp.Body).Decode(&data); decErr != nil {
		return "", nil, nil, fmt.Errorf("invalid agent response: %w", decErr)
	}
	return data.Name, data.Ciphertext, data.Nonce, nil
}

func (s *Server) AgentURL(ip string, port int) string {
	if port == 0 {
		port = agentapi.DefaultPort
	}
	return fmt.Sprintf("%s://%s:%d", httputil.AgentScheme(), ip, port)
}

// ── bulk.Deps implementation ─────────────────────────────────────────────────

func (s *Server) FindAgentURL(hashOrLabel string) (string, error) {
	agent, err := s.FindAgentRecord(hashOrLabel)
	if err != nil {
		return "", err
	}
	return s.AgentURL(agent.IP, agent.Port), nil
}

func (s *Server) ResolveTemplateValue(vaultHash, kind, name string) (string, bool) {
	agent, err := s.FindAgentRecord(vaultHash)
	if err != nil {
		return "", false
	}
	if agent.BlockedAt != nil || agent.RebindRequired {
		return "", false
	}
	agentURL := s.AgentURL(agent.IP, agent.Port)
	if kind == "secret" {
		return s.resolveBulkApplySecretValue(agentURL, agent.DEK, agent.DEKNonce, name)
	}
	return s.resolveBulkApplyConfigValue(agentURL, name)
}

func (s *Server) resolveBulkApplySecretValue(agentURL string, encDEK, encNonce []byte, name string) (string, bool) {
	// Try to fetch the ciphertext directly and decrypt with the agent DEK.
	_, ciphertext, nonce, err := s.FetchAgentCiphertext(agentURL, name)
	if err == nil {
		agentDEK, dekErr := s.DecryptAgentDEK(encDEK, encNonce)
		if dekErr == nil {
			plaintext, decErr := crypto.Decrypt(agentDEK, ciphertext, nonce)
			if decErr == nil {
				return string(plaintext), true
			}
		}
	}
	// Fall back to the agent's own resolve endpoint.
	resp, resolveErr := s.httpClient.Get(httputil.JoinPath(agentURL, httputil.AgentPathResolve, name))
	if resolveErr != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", false
	}
	var data struct {
		Value string `json:"value"`
	}
	if json.NewDecoder(resp.Body).Decode(&data) != nil {
		return "", false
	}
	return data.Value, strings.TrimSpace(data.Value) != ""
}

func (s *Server) resolveBulkApplyConfigValue(agentURL, key string) (string, bool) {
	resp, err := s.httpClient.Get(httputil.JoinPath(agentURL, httputil.AgentPathConfigs, key))
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", false
	}
	var data map[string]any
	if json.NewDecoder(resp.Body).Decode(&data) != nil {
		return "", false
	}
	value, _ := data["value"].(string)
	return value, strings.TrimSpace(value) != ""
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
		unlockLimiter: ratelimit.New(),
		httpClient:    tlsutil.InitHTTPClientFromEnv(),
		chainStore:    &db.ChainStoreAdapter{DB: database},
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
	srv.adminHandler = admin.NewHandler(srv)
	srv.hkmHandler = hkm.NewHandler(srv)
	srv.bulkHandler = bulk.NewHandler(srv)
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
	return s.bulkApplyDir
}

func (s *Server) SetSalt(salt []byte) {
	s.salt = salt
	s.approvalHandler = approval.NewHandler(s.db, salt, s.httpClient)
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

func (s *Server) requireUnlocked(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.IsLocked() {
			s.respondError(w, http.StatusServiceUnavailable, "server is locked. POST /api/unlock with password to unlock.")
			return
		}
		next(w, r)
	}
}

func (s *Server) requireReadyForOps(next http.HandlerFunc) http.HandlerFunc {
	return s.requireUnlocked(next)
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

func decodeJSON(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func (s *Server) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	httputil.RespondJSON(w, status, data)
}

func (s *Server) respondError(w http.ResponseWriter, status int, message string) {
	httputil.RespondError(w, status, message)
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
	if err := s.db.Ping(); err != nil {
		s.respondError(w, http.StatusServiceUnavailable, "database not ready")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) SetupRoutes() http.Handler {
	if s.approvalHandler == nil {
		panic("api: SetSalt must be called before SetupRoutes")
	}
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
	mux.HandleFunc("GET /api/chain/info", s.handleChainInfo)
	mux.HandleFunc("POST /api/unlock", s.requireTrustedIP(s.unlockLimiter.Middleware(s.handleUnlock)))
	s.approvalHandler.Register(mux, s.requireTrustedIP)
	s.SetupAPIRoutes(mux)
	s.adminHandler.Register(mux, s.requireReadyForOps, s.requireTrustedIP)
	// tracked-ref cleanup routes delegate to the hkm handler (registered after IsHKM check below)
	if s.IsHKM() {
		s.hkmHandler.Register(mux, s.requireTrustedIP, s.requireReadyForOps)
		s.bulkHandler.Register(mux, s.requireTrustedIP)
		// Admin tracked-ref cleanup routes require an active hkm handler.
		mux.HandleFunc("POST /api/admin/tracked-refs/cleanup", s.requireReadyForOps(s.adminHandler.RequireAdminSession(s.hkmHandler.HandleTrackedRefCleanup)))
	}

	return logMiddleware(TxActorMiddleware(mux))
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
