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
	kek           []byte
	kekMu         sync.RWMutex
	locked        bool
	salt          []byte
	trustedIPs    map[string]bool
	trustedCIDRs  []*net.IPNet
	identity      *NodeIdentity
	unlockLimiter *ratelimit.UnlockRateLimiter
	httpClient    *http.Client

	secretsHandler   *secrets.Handler
	configsHandler   *configs.Handler
	bulkHandler      *bulk.Handler
	functionsHandler *functions.Handler
}

func (s *Server) Close() { s.db.Close() }

func (s *Server) SetIdentity(identity *NodeIdentity) {
	s.identity = identity
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
	if s.identity == nil {
		return ""
	}
	return s.identity.VaultHash
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
		clientIP := strings.Split(r.RemoteAddr, ":")[0]
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
	mux.HandleFunc("GET /api/install/status", s.HandleInstallStatus)
	mux.HandleFunc("PATCH /api/install/vaultcenter-url", s.requireTrustedIP(s.HandlePatchVaultcenterURL))

	// Health + unlock
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("POST /api/unlock", s.requireTrustedIP(s.unlockLimiter.Middleware(s.handleUnlock)))

	// Status
	mux.HandleFunc("GET /api/status", s.requireUnlocked(s.handleStatus))
	mux.HandleFunc("GET /api/node-info", s.requireUnlocked(s.handleStatus))

	// Lifecycle (reencrypt + status transitions — spans VK and VE)
	mux.HandleFunc("POST /api/reencrypt", s.requireUnlocked(s.handleReencrypt))
	mux.HandleFunc("POST /api/activate", s.requireUnlocked(s.handleActivate))
	mux.HandleFunc("POST /api/archive", s.requireUnlocked(s.handleArchive))
	mux.HandleFunc("POST /api/block", s.requireUnlocked(s.handleBlock))
	mux.HandleFunc("POST /api/revoke", s.requireUnlocked(s.handleRevoke))

	// Domain subpackage routes
	s.secretsHandler.Register(mux, s.requireUnlocked, s.requireTrustedIP)
	s.configsHandler.Register(mux, s.requireTrustedIP)
	s.bulkHandler.Register(mux, s.requireUnlocked, s.requireTrustedIP)
	s.functionsHandler.Register(mux, s.requireUnlocked, s.requireTrustedIP)

	return logMiddleware(mux)
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
