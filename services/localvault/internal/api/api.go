package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"veilkey-localvault/internal/api/bulk"
	"veilkey-localvault/internal/api/configs"
	"veilkey-localvault/internal/api/functions"
	"veilkey-localvault/internal/api/secrets"
	"veilkey-localvault/internal/db"

	"github.com/veilkey/veilkey-go-package/crypto"
	"github.com/veilkey/veilkey-go-package/httputil"
	"github.com/veilkey/veilkey-go-package/ratelimit"
	"github.com/veilkey/veilkey-go-package/tlsutil"
)

type NodeIdentity struct {
	NodeID    string
	Version   int
	VaultHash string
	VaultName string
}

type Server struct {
	db            *db.DB
	dbPath        string // for deferred DB opening (DB opens during Unlock)
	dataDir       string // directory containing salt, agent_secret, vault_key files
	kek           []byte
	kekMu         sync.RWMutex
	locked        bool
	salt          []byte
	trustedIPs    map[string]bool
	trustedCIDRs  []*net.IPNet
	identity      *NodeIdentity
	unlockLimiter *ratelimit.UnlockRateLimiter
	httpClient    *http.Client

	// vaultUnlockKey is the auto-generated password, held in memory until sent to VC.
	vaultUnlockKey string

	// heartbeat params for post-unlock trigger (set once at startup, read-only after)
	heartbeatMu       sync.RWMutex
	heartbeatEndpoint string
	heartbeatLabel    string
	heartbeatPort     int

	// agentAuthCache caches the decrypted agent secret to avoid repeated DB+KEK lookups.
	agentAuthCache   string
	agentAuthCacheAt time.Time

	secretsHandler   *secrets.Handler
	configsHandler   *configs.Handler
	bulkHandler      *bulk.Handler
	functionsHandler *functions.Handler
}

func (s *Server) Close() {
	if s.db != nil {
		_ = s.db.Close()
	}
}

// DeriveDBKeyFromKEK derives a SQLCipher encryption key from the KEK.
// Exported so commands package (init, server) can use the same derivation logic.
func DeriveDBKeyFromKEK(kek []byte) string {
	h := sha256.Sum256(kek)
	return hex.EncodeToString(h[:])
}

// SetDBPath stores the database path and salt for deferred DB opening during Unlock.
func (s *Server) SetDBPath(dbPath string, salt []byte) {
	s.dbPath = dbPath
	s.salt = salt
	s.dataDir = filepath.Dir(dbPath)
}

// SetVaultUnlockKey stores the auto-generated password in memory for sending to VC.
func (s *Server) SetVaultUnlockKey(key string) {
	s.kekMu.Lock()
	defer s.kekMu.Unlock()
	s.vaultUnlockKey = key
}

// VaultUnlockKey returns the pending vault unlock key (empty after sent to VC).
func (s *Server) VaultUnlockKey() string {
	s.kekMu.RLock()
	defer s.kekMu.RUnlock()
	return s.vaultUnlockKey
}

// ClearVaultUnlockKey clears the in-memory vault unlock key after VC confirms storage.
func (s *Server) ClearVaultUnlockKey() {
	s.kekMu.Lock()
	defer s.kekMu.Unlock()
	s.vaultUnlockKey = ""
}

// ReadAgentSecretFile reads the agent_secret from the data directory file.
func (s *Server) ReadAgentSecretFile() string {
	data, err := os.ReadFile(filepath.Join(s.dataDir, "agent_secret"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// WriteAgentSecretFile writes the agent_secret to a file in the data directory.
func (s *Server) WriteAgentSecretFile(secret string) error {
	return os.WriteFile(filepath.Join(s.dataDir, "agent_secret"), []byte(secret), 0600)
}

// AutoUnlockFromVC contacts VaultCenter to fetch the vault unlock key and auto-unlocks.
func (s *Server) AutoUnlockFromVC(vcURL string) error {
	agentSecret := s.ReadAgentSecretFile()
	if agentSecret == "" {
		return fmt.Errorf("no agent_secret file")
	}

	url := strings.TrimRight(vcURL, "/") + "/api/agents/unlock-key"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+agentSecret)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("VC returned status %d", resp.StatusCode)
	}

	var result struct {
		UnlockKey string `json:"unlock_key"`
		Salt      string `json:"salt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if result.UnlockKey == "" {
		return fmt.Errorf("empty unlock_key in response")
	}

	salt := s.salt
	if result.Salt != "" {
		if decoded, err := base64.StdEncoding.DecodeString(result.Salt); err == nil && len(decoded) > 0 {
			salt = decoded
			saltFile := filepath.Join(s.dataDir, "salt")
			if writeErr := os.WriteFile(saltFile, salt, 0600); writeErr != nil {
				log.Printf("AutoUnlockFromVC: failed to cache salt file: %v", writeErr)
			} else {
				s.salt = salt
				log.Println("AutoUnlockFromVC: salt restored from VaultCenter")
			}
		}
	}
	if len(salt) == 0 {
		return fmt.Errorf("no salt available (local file missing and VaultCenter did not provide one)")
	}
	kek := crypto.DeriveKEK(result.UnlockKey, salt)
	return s.Unlock(kek)
}

func (s *Server) Salt() []byte { return s.salt }

func (s *Server) SetIdentity(identity *NodeIdentity) {
	s.kekMu.Lock()
	defer s.kekMu.Unlock()
	s.identity = identity
}

// Identity returns a copy of the current node identity (thread-safe).
func (s *Server) Identity() *NodeIdentity {
	s.kekMu.RLock()
	defer s.kekMu.RUnlock()
	if s.identity == nil {
		return nil
	}
	cp := *s.identity
	return &cp
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
	s := &Server{
		db:            database,
		kek:           kek,
		locked:        kek == nil,
		trustedIPs:    ipMap,
		trustedCIDRs:  cidrs,
		unlockLimiter: ratelimit.New(),
		httpClient:    tlsutil.InitHTTPClientFromEnv(),
	}
	s.secretsHandler = secrets.NewHandler(s)
	s.configsHandler = configs.NewHandler(s)
	s.bulkHandler = bulk.NewHandler()
	s.functionsHandler = functions.NewHandler(s)
	return s
}

func (s *Server) SetSalt(salt []byte) {
	s.salt = salt
}

func (s *Server) Unlock(kek []byte) error {
	// 1. Early check: already unlocked (race between autoUnlock and handleUnlock)
	s.kekMu.RLock()
	alreadyUnlocked := !s.locked
	s.kekMu.RUnlock()
	if alreadyUnlocked {
		return nil
	}

	// 2. Derive DB_KEY from KEK and open database
	dbKey := DeriveDBKeyFromKEK(kek)
	_ = os.Setenv("VEILKEY_DB_KEY", dbKey)

	database, err := db.New(s.dbPath)
	if err != nil {
		// Attempt plaintext → encrypted migration (#360)
		if migErr := db.MigrateToEncrypted(s.dbPath, dbKey); migErr != nil {
			log.Printf("Unlock: encrypted open failed, migration also failed: %v", migErr)
			return fmt.Errorf("invalid password (cannot open database)")
		}
		database, err = db.New(s.dbPath)
		if err != nil {
			return fmt.Errorf("invalid password (cannot open database after migration)")
		}
	}

	// 3. Verify KEK by decrypting DEK
	info, err := database.GetNodeInfo()
	if err != nil {
		_ = database.Close()
		return fmt.Errorf("no node info found: %w", err)
	}
	_, err = crypto.Decrypt(kek, info.DEK, info.DEKNonce)
	if err != nil {
		_ = database.Close()
		return fmt.Errorf("invalid password (KEK decryption failed)")
	}

	// 4. Set DB and unlock — double-check under write lock to prevent DB connection leak
	s.kekMu.Lock()
	if !s.locked {
		// Another goroutine unlocked while we were opening DB — close our connection
		s.kekMu.Unlock()
		_ = database.Close()
		return nil
	}
	s.db = database
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

// ── secrets.Deps implementation ───────────────────────────────────────────────

func (s *Server) DB() *db.DB { return s.db }

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
	dek, err := crypto.Decrypt(s.GetKEK(), info.DEK, info.DEKNonce)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt DEK: %w", err)
	}
	return dek, nil
}

func (s *Server) VaultcenterURL() string {
	return s.resolveVaultcenterTarget().URL
}

func (s *Server) HTTPClient() *http.Client { return s.httpClient }

// ── functions.Deps implementation ─────────────────────────────────────────────

func (s *Server) VaultHash() string {
	id := s.Identity()
	if id == nil {
		return ""
	}
	return id.VaultHash
}

// LoadIdentity reads node info and config from DB and sets the server identity.
// Call after Unlock() to ensure heartbeat has valid identity data.
func (s *Server) LoadIdentity() {
	info, err := s.db.GetNodeInfo()
	if err != nil {
		log.Printf("LoadIdentity: failed to read node info: %v", err)
		return
	}
	vaultHash := s.lookupConfigValue("VEILKEY_VAULT_HASH")
	vaultName := s.lookupConfigValue("VEILKEY_VAULT_NAME")
	// Fallback to env vars if DB config is empty (new vault)
	if vaultName == "" {
		vaultName = os.Getenv("VEILKEY_VAULT_NAME")
	}
	if vaultName == "" {
		vaultName = os.Getenv("VEILKEY_LABEL")
	}
	if vaultName == "" {
		vaultName, _ = os.Hostname()
	}
	if vaultHash == "" {
		vaultHash = os.Getenv("VEILKEY_VAULT_HASH")
	}
	// Generate vault_hash if still empty (first-time registration)
	if vaultHash == "" && vaultName != "" {
		h := sha256.Sum256([]byte(info.NodeID))
		vaultHash = hex.EncodeToString(h[:8])
		_ = s.db.SaveConfig("VEILKEY_VAULT_HASH", vaultHash)
		log.Printf("LoadIdentity: generated vault_hash=%s", vaultHash)
	}
	if vaultName != "" {
		_ = s.db.SaveConfig("VEILKEY_VAULT_NAME", vaultName)
	}
	s.SetIdentity(&NodeIdentity{
		NodeID:    info.NodeID,
		Version:   info.Version,
		VaultHash: vaultHash,
		VaultName: vaultName,
	})
	log.Printf("Identity loaded: node=%s version=%d vault=%s:%s", info.NodeID, info.Version, vaultName, vaultHash)
}

// ── Forwarding methods for cron runner ───────────────────────────────────────

// SyncGlobalFunctions delegates to functionsHandler.SyncGlobalFunctions.
func (s *Server) SyncGlobalFunctions(endpoint string) (int, int, error) {
	return s.functionsHandler.SyncGlobalFunctions(endpoint)
}

// CleanupExpiredTestFunctions delegates to functionsHandler.CleanupExpiredTestFunctions.
func (s *Server) CleanupExpiredTestFunctions(now time.Time) (int, error) {
	return s.functionsHandler.CleanupExpiredTestFunctions(now)
}

// agentAuthHeader returns the Authorization header value for requests to VaultCenter.
// Caches the decrypted secret for 1 minute to avoid repeated DB+KEK lookups.
func (s *Server) agentAuthHeader() string {
	if s.IsLocked() {
		return ""
	}
	if s.agentAuthCache != "" && time.Since(s.agentAuthCacheAt) < time.Minute {
		return s.agentAuthCache
	}
	info, err := s.db.GetNodeInfo()
	if err != nil || len(info.AgentSecret) == 0 {
		return ""
	}
	kek := s.GetKEK()
	decrypted, err := crypto.Decrypt(kek, info.AgentSecret, info.AgentSecretNonce)
	if err != nil {
		return ""
	}
	s.agentAuthCache = "Bearer " + string(decrypted)
	s.agentAuthCacheAt = time.Now()
	return s.agentAuthCache
}

// invalidateAgentAuthCache clears the cached agent secret (call after receiving a new secret).
func (s *Server) invalidateAgentAuthCache() {
	s.agentAuthCache = ""
	s.agentAuthCacheAt = time.Time{}
}

// ── Middleware ────────────────────────────────────────────────────────────────

func (s *Server) requireUnlocked(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.IsLocked() {
			s.respondError(w, http.StatusServiceUnavailable, "server is locked. POST /api/unlock with password to unlock.")
			return
		}
		next(w, r)
	}
}

func (s *Server) handleUnlock(w http.ResponseWriter, r *http.Request) {
	if !s.IsLocked() {
		s.respondJSON(w, http.StatusOK, map[string]interface{}{"status": "already_unlocked"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
		s.respondError(w, http.StatusBadRequest, "password is required")
		return
	}
	if len(req.Password) > 256 {
		s.respondError(w, http.StatusBadRequest, "password too long")
		return
	}

	kek := crypto.DeriveKEK(req.Password, s.salt)
	if err := s.Unlock(kek); err != nil {
		log.Printf("Unlock failed from %s: %v", r.RemoteAddr, err)
		s.respondError(w, http.StatusUnauthorized, "invalid password")
		return
	}

	s.LoadIdentity()

	// Store password for VC migration — next heartbeat will send it to VC
	// so future restarts can auto-unlock without manual password entry.
	s.SetVaultUnlockKey(req.Password)

	log.Printf("Server unlocked by %s", r.RemoteAddr)

	// Trigger immediate heartbeat after unlock
	go func() {
		s.heartbeatMu.RLock()
		ep, lbl, pt := s.heartbeatEndpoint, s.heartbeatLabel, s.heartbeatPort
		s.heartbeatMu.RUnlock()
		if ep != "" {
			log.Println("Post-unlock heartbeat triggered")
			if err := s.SendHeartbeatOnce(ep, lbl, pt); err != nil {
				log.Printf("Post-unlock heartbeat failed: %v", err)
			}
		}
	}()
	s.respondJSON(w, http.StatusOK, map[string]interface{}{"status": "unlocked"})
}

func (s *Server) requireTrustedIP(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		allowed := s.trustedIPs[clientIP]
		if !allowed {
			ip := net.ParseIP(clientIP)
			for _, cidr := range s.trustedCIDRs {
				if cidr.Contains(ip) {
					allowed = true
					break
				}
			}
		}
		if (len(s.trustedIPs) > 0 || len(s.trustedCIDRs) > 0) && !allowed {
			log.Printf("BLOCKED %s %s %s (untrusted IP)", clientIP, r.Method, r.URL.Path)
			s.respondError(w, http.StatusForbidden, "access denied")
			return
		}
		next(w, r)
	}
}

// requireAgentSecret validates the agent secret on incoming requests from VaultCenter.
// If no agent secret is configured yet, requests pass through (allow initial setup).
func (s *Server) requireAgentSecret(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.IsLocked() {
			s.respondError(w, http.StatusServiceUnavailable, "server is locked")
			return
		}
		info, err := s.db.GetNodeInfo()
		if err != nil || len(info.AgentSecret) == 0 {
			// No agent secret configured yet, allow through
			next(w, r)
			return
		}
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			s.respondError(w, http.StatusUnauthorized, "agent secret required")
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader || token == "" {
			s.respondError(w, http.StatusUnauthorized, "agent secret required")
			return
		}
		kek := s.GetKEK()
		decrypted, err := crypto.Decrypt(kek, info.AgentSecret, info.AgentSecretNonce)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, "failed to verify agent secret")
			return
		}
		if subtle.ConstantTimeCompare([]byte(token), decrypted) != 1 {
			s.respondError(w, http.StatusUnauthorized, "invalid agent secret")
			return
		}
		next(w, r)
	}
}

func (s *Server) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	httputil.RespondJSON(w, status, data)
}

func (s *Server) respondError(w http.ResponseWriter, status int, message string) {
	httputil.RespondError(w, status, message)
}

// ── Routes ────────────────────────────────────────────────────────────────────

func (s *Server) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	// Serve install wizard UI at /
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		RenderInstallWizard(w)
	})
	if assets := InstallUIAssets(); assets != nil {
		mux.Handle("/assets/", http.FileServer(http.FS(assets)))
	}

	// Install status/settings APIs
	mux.HandleFunc("GET /api/install/status", s.requireTrustedIP(s.HandleInstallStatus))
	mux.HandleFunc("PATCH /api/install/vaultcenter-url", s.requireTrustedIP(s.HandlePatchVaultcenterURL))

	// Health + unlock
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("POST /api/unlock", s.requireTrustedIP(s.unlockLimiter.Middleware(s.handleUnlock)))

	// Status
	mux.HandleFunc("GET /api/status", s.requireUnlocked(s.handleStatus))
	mux.HandleFunc("GET /api/node-info", s.requireUnlocked(s.handleStatus))

	// Lifecycle (reencrypt + status transitions — spans VK and VE)
	mux.HandleFunc("POST /api/reencrypt", s.requireTrustedIP(s.requireUnlocked(s.handleReencrypt)))
	mux.HandleFunc("POST /api/activate", s.requireTrustedIP(s.requireUnlocked(s.handleActivate)))
	mux.HandleFunc("POST /api/archive", s.requireTrustedIP(s.requireUnlocked(s.handleArchive)))
	mux.HandleFunc("POST /api/block", s.requireTrustedIP(s.requireUnlocked(s.handleBlock)))
	mux.HandleFunc("POST /api/revoke", s.requireTrustedIP(s.requireUnlocked(s.handleRevoke)))

	// Domain subpackage routes
	s.secretsHandler.Register(mux, s.requireUnlocked, s.requireTrustedIP, s.requireAgentSecret)
	s.configsHandler.Register(mux, s.requireTrustedIP)
	s.bulkHandler.Register(mux, s.requireUnlocked, s.requireTrustedIP)
	s.functionsHandler.Register(mux, s.requireUnlocked, s.requireTrustedIP)
	s.registerAdminRoutes(mux)

	return recoveryMiddleware(securityHeadersMiddleware(logMiddleware(mux)))
}

// recoveryMiddleware catches panics in HTTP handlers, logs the stack trace,
// and returns a 500 response instead of crashing the server.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC recovered: %v\n%s", err, debug.Stack())
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
