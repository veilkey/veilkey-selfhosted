package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"veilkey-localvault/internal/crypto"
	"veilkey-localvault/internal/db"
)

type NodeIdentity struct {
	NodeID    string
	Version   int
	VaultHash string
	VaultName string
}

type Server struct {
	db              *db.DB
	kek             []byte
	kekMu           sync.RWMutex
	locked          bool
	salt            []byte
	trustedIPs      map[string]bool
	trustedCIDRs    []*net.IPNet
	identity        *NodeIdentity
	unlockLimiter   *UnlockRateLimiter
	httpClient      *http.Client
}

const keycenterOnlyDecryptMessage = "localvault direct plaintext handling is disabled; use keycenter"

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
	return &Server{db: database, kek: kek, locked: kek == nil, trustedIPs: ipMap, trustedCIDRs: cidrs, unlockLimiter: NewUnlockRateLimiter(), httpClient: InitHTTPClientFromEnv()}
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("failed to encode JSON response: %v", err)
	}
}

func (s *Server) respondError(w http.ResponseWriter, status int, message string) {
	s.respondJSON(w, status, map[string]string{"error": message})
}

func (s *Server) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("POST /api/unlock", s.requireTrustedIP(s.unlockLimiter.Middleware(s.handleUnlock)))

	// Status
	mux.HandleFunc("GET /api/status", s.requireUnlocked(s.handleStatus))
	mux.HandleFunc("GET /api/node-info", s.requireUnlocked(s.handleStatus))

	// Secrets
	mux.HandleFunc("POST /api/secrets", s.requireTrustedIP(s.requireUnlocked(s.handleSaveSecret)))
	mux.HandleFunc("GET /api/secrets", s.requireUnlocked(s.handleListSecrets))
	mux.HandleFunc("GET /api/secrets/{name}", s.requireUnlocked(s.handleGetSecret))
	mux.HandleFunc("DELETE /api/secrets/{name}", s.requireTrustedIP(s.requireUnlocked(s.handleDeleteSecret)))

	// Resolve scoped canonical refs like VK:{SCOPE}:{REF}
	mux.HandleFunc("GET /api/resolve/{ref}", s.requireUnlocked(s.handleResolveSecret))

	// Rekey (called by hub/parent)
	mux.HandleFunc("POST /api/rekey", s.requireTrustedIP(s.requireUnlocked(s.handleRekey)))

	// Cipher endpoint: return raw ciphertext+nonce for Hub-only decryption
	mux.HandleFunc("GET /api/cipher/{ref}", s.requireTrustedIP(s.requireUnlocked(s.handleCipher)))
	mux.HandleFunc("GET /api/cipher/{ref}/fields/{field}", s.requireTrustedIP(s.requireUnlocked(s.handleCipherField)))
	mux.HandleFunc("POST /api/cipher", s.requireTrustedIP(s.requireUnlocked(s.handleSaveCipher)))
	mux.HandleFunc("POST /api/decrypt", s.requireTrustedIP(s.requireUnlocked(s.handleDecrypt)))

	// Encrypt (CLI-compatible: plaintext → VK:TEMP:ref token)
	mux.HandleFunc("POST /api/encrypt", s.requireUnlocked(s.handleEncrypt))
	mux.HandleFunc("POST /api/reencrypt", s.requireUnlocked(s.handleReencrypt))
	mux.HandleFunc("POST /api/activate", s.requireUnlocked(s.handleActivate))
	mux.HandleFunc("POST /api/archive", s.requireUnlocked(s.handleArchive))
	mux.HandleFunc("POST /api/block", s.requireUnlocked(s.handleBlock))
	mux.HandleFunc("POST /api/revoke", s.requireUnlocked(s.handleRevoke))

	// Secret metadata (no plaintext exposure)
	mux.HandleFunc("GET /api/secrets/meta/{name}", s.requireUnlocked(s.handleGetSecretMeta))
	mux.HandleFunc("POST /api/secrets/fields", s.requireTrustedIP(s.requireUnlocked(s.handleSaveSecretFields)))
	mux.HandleFunc("DELETE /api/secrets/{name}/fields/{field}", s.requireTrustedIP(s.requireUnlocked(s.handleDeleteSecretField)))

	// Configs (plaintext key-value, no encryption — no unlock required)
	mux.HandleFunc("GET /api/configs", s.handleListConfigs)
	mux.HandleFunc("GET /api/configs/{key}", s.handleGetConfig)
	mux.HandleFunc("POST /api/configs", s.requireTrustedIP(s.handleSaveConfig))
	mux.HandleFunc("PUT /api/configs/bulk", s.requireTrustedIP(s.handleSaveConfigsBulk))
	mux.HandleFunc("DELETE /api/configs/{key}", s.requireTrustedIP(s.handleDeleteConfig))

	// Bulk apply (trusted orchestration from KeyCenter)
	mux.HandleFunc("POST /api/bulk-apply/precheck", s.requireTrustedIP(s.requireUnlocked(s.handleBulkApplyPrecheck)))
	mux.HandleFunc("POST /api/bulk-apply/execute", s.requireTrustedIP(s.requireUnlocked(s.handleBulkApplyExecute)))

	// Functions (vault-local rows, no plaintext secret storage)
	mux.HandleFunc("GET /api/functions", s.requireUnlocked(s.handleFunctions))
	mux.HandleFunc("POST /api/functions", s.requireTrustedIP(s.requireUnlocked(s.handleFunctions)))
	mux.HandleFunc("GET /api/functions/{name...}", s.requireUnlocked(s.handleFunction))
	mux.HandleFunc("DELETE /api/functions/{name...}", s.requireTrustedIP(s.requireUnlocked(s.handleFunction)))

	return logMiddleware(mux)
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
