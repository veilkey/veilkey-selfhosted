package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"

	"github.com/veilkey/veilkey-go-package/cmdutil"
	"github.com/veilkey/veilkey-go-package/crypto"
)

const adminSessionCookie = "vk_session"

var (
	adminSessionDuration     = cmdutil.ParseDurationEnv("VEILKEY_ADMIN_SESSION_TTL", 8*time.Hour)
	adminSessionIdleDuration = cmdutil.ParseDurationEnv("VEILKEY_ADMIN_SESSION_IDLE_TIMEOUT", 1*time.Hour)
)

const loginMaxAttempts = 10

var (
	loginLockDuration    = cmdutil.ParseDurationEnv("VEILKEY_LOGIN_LOCK_DURATION", 15*time.Minute)
	loginCleanupInterval = cmdutil.ParseDurationEnv("VEILKEY_LOGIN_CLEANUP_INTERVAL", 10*time.Minute)
)

type loginAttempt struct {
	count    int
	lockedAt time.Time
}

var (
	loginMu       sync.Mutex
	loginAttempts = map[string]*loginAttempt{}
)

// privateRanges are RFC1918 + loopback used for proxy detection.
var privateRanges []*net.IPNet

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8", "::1/128",
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"fc00::/7",
	} {
		_, network, _ := net.ParseCIDR(cidr)
		privateRanges = append(privateRanges, network)
	}

	// Background cleanup: remove stale entries every loginCleanupInterval.
	go func() {
		for {
			time.Sleep(loginCleanupInterval)
			cutoff := time.Now().Add(-2 * loginLockDuration)
			loginMu.Lock()
			for ip, a := range loginAttempts {
				if !a.lockedAt.IsZero() && a.lockedAt.Before(cutoff) {
					delete(loginAttempts, ip)
				}
			}
			loginMu.Unlock()
		}
	}()
}

func isPrivateIP(ip net.IP) bool {
	for _, network := range privateRanges {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// remoteIP returns the client IP address.
// When the direct connection comes from a private/loopback address (i.e. a
// reverse proxy on the same host or local network), it trusts X-Real-IP and
// then the leftmost entry of X-Forwarded-For. This is safe for self-hosted
// deployments; do not expose this server directly to the internet without a
// trusted proxy in front.
func remoteIP(r *http.Request) string {
	addr, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		addr = r.RemoteAddr
	}
	if ip := net.ParseIP(addr); ip != nil && isPrivateIP(ip) {
		if h := r.Header.Get(httputil.HeaderXRealIP); h != "" {
			if parsed := net.ParseIP(strings.TrimSpace(h)); parsed != nil && !parsed.IsLoopback() {
				return parsed.String()
			}
		}
		if h := r.Header.Get(httputil.HeaderXForwardedFor); h != "" {
			first := strings.TrimSpace(strings.SplitN(h, ",", 2)[0])
			if parsed := net.ParseIP(first); parsed != nil && !parsed.IsLoopback() {
				return parsed.String()
			}
		}
	}
	return addr
}

func checkLoginRateLimit(ip string) bool {
	loginMu.Lock()
	defer loginMu.Unlock()
	a := loginAttempts[ip]
	if a == nil {
		return true
	}
	if !a.lockedAt.IsZero() {
		if time.Since(a.lockedAt) > loginLockDuration {
			delete(loginAttempts, ip)
			return true
		}
		return false
	}
	return true
}

func recordLoginFailure(ip string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	a := loginAttempts[ip]
	if a == nil {
		a = &loginAttempt{}
		loginAttempts[ip] = a
	}
	a.count++
	if a.count >= loginMaxAttempts {
		a.lockedAt = time.Now()
		log.Printf("admin login: too many failures from %s, locked for %v", ip, loginLockDuration)
	}
}

func clearLoginAttempts(ip string) {
	loginMu.Lock()
	delete(loginAttempts, ip)
	loginMu.Unlock()
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	ip := remoteIP(r)
	if !checkLoginRateLimit(ip) {
		s.respondError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Password == "" {
		s.respondError(w, http.StatusBadRequest, "password is required")
		return
	}

	if !s.db.VerifyAdminPassword(req.Password) {
		recordLoginFailure(ip)
		s.respondError(w, http.StatusUnauthorized, "invalid password")
		return
	}
	clearLoginAttempts(ip)

	token, err := generateSecureToken(32)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to generate session")
		return
	}

	tokenHash := hashToken(token)
	now := time.Now().UTC()
	session := &db.AdminSession{
		SessionID:     generateSessionID(),
		TokenHash:     tokenHash,
		AuthMethod:    "password",
		RemoteAddr:    r.RemoteAddr,
		ExpiresAt:     now.Add(adminSessionDuration),
		IdleExpiresAt: now.Add(adminSessionIdleDuration),
		LastSeenAt:    now,
	}
	if err := s.db.SaveAdminSession(session); err != nil {
		log.Printf("admin login: failed to save session: %v", err)
		s.respondError(w, http.StatusInternalServerError, "failed to save session")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(adminSessionDuration.Seconds()),
	})

	log.Printf("Admin login from %s", r.RemoteAddr)
	s.respondJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(adminSessionCookie)
	if err == nil {
		tokenHash := hashToken(cookie.Value)
		session, err := s.db.GetAdminSessionByTokenHash(tokenHash)
		if err == nil {
			_ = s.db.RevokeAdminSession(session.SessionID, time.Now().UTC())
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	s.respondJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

func (s *Server) handleAdminSetup(w http.ResponseWriter, r *http.Request) {
	if s.db.HasAdminPassword() {
		s.respondError(w, http.StatusConflict, "admin password already configured")
		return
	}
	var req struct {
		OwnerPassword string `json:"owner_password"`
		AdminPassword string `json:"admin_password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.AdminPassword) < 8 {
		s.respondError(w, http.StatusBadRequest, "admin_password must be at least 8 characters")
		return
	}
	if len(req.AdminPassword) > 256 {
		s.respondError(w, http.StatusBadRequest, "admin_password must not exceed 256 characters")
		return
	}
	// Verify owner password (KEK)
	kek := crypto.DeriveKEK(req.OwnerPassword, s.salt)
	info, err := s.db.GetNodeInfo()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "node info not available")
		return
	}
	if _, err := crypto.Decrypt(kek, info.DEK, info.DEKNonce); err != nil {
		s.respondError(w, http.StatusUnauthorized, "invalid owner password")
		return
	}
	// Set admin password
	if err := s.db.SetAdminPassword(req.AdminPassword); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to set admin password")
		return
	}
	log.Printf("admin password configured by %s", r.RemoteAddr)
	s.respondJSON(w, http.StatusOK, map[string]interface{}{"status": "configured"})
}

func (s *Server) handleAdminChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OwnerPassword    string `json:"owner_password"`
		NewAdminPassword string `json:"new_admin_password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.OwnerPassword == "" {
		s.respondError(w, http.StatusBadRequest, "owner_password is required")
		return
	}
	if len(req.NewAdminPassword) < 8 {
		s.respondError(w, http.StatusBadRequest, "new_admin_password must be at least 8 characters")
		return
	}
	if len(req.NewAdminPassword) > 256 {
		s.respondError(w, http.StatusBadRequest, "admin_password must not exceed 256 characters")
		return
	}
	// Verify owner password (KEK) — the ONLY way to change admin password
	kek := crypto.DeriveKEK(req.OwnerPassword, s.salt)
	info, err := s.db.GetNodeInfo()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "node info not available")
		return
	}
	if _, err := crypto.Decrypt(kek, info.DEK, info.DEKNonce); err != nil {
		s.respondError(w, http.StatusUnauthorized, "invalid owner password")
		return
	}
	if err := s.db.SetAdminPassword(req.NewAdminPassword); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to change admin password")
		return
	}
	log.Printf("admin password changed by %s (owner-verified)", r.RemoteAddr)
	s.respondJSON(w, http.StatusOK, map[string]interface{}{"status": "changed"})
}

func (s *Server) handleAdminCheck(w http.ResponseWriter, r *http.Request) {
	if !s.db.HasAdminPassword() {
		s.respondJSON(w, http.StatusOK, map[string]interface{}{"setup_required": true})
		return
	}
	session := s.resolveAdminSession(r)
	if session == nil {
		s.respondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

func (s *Server) resolveAdminSession(r *http.Request) *db.AdminSession {
	cookie, err := r.Cookie(adminSessionCookie)
	if err != nil {
		return nil
	}
	tokenHash := hashToken(cookie.Value)
	session, err := s.db.GetAdminSessionByTokenHash(tokenHash)
	if err != nil {
		return nil
	}
	now := time.Now().UTC()
	if now.After(session.ExpiresAt) || now.After(session.IdleExpiresAt) {
		return nil
	}
	_ = s.db.TouchAdminSession(session.SessionID, now, now.Add(adminSessionIdleDuration))
	return session
}

func (s *Server) requireAdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := s.resolveAdminSession(r)
		if session == nil {
			s.respondError(w, http.StatusUnauthorized, "admin session required")
			return
		}
		next(w, r)
	}
}

func generateSecureToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func generateSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
