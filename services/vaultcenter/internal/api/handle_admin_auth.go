package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"time"

	"veilkey-vaultcenter/internal/db"
)

const adminSessionCookie = "vk_session"
const adminSessionDuration = 24 * time.Hour
const adminSessionIdleDuration = 2 * time.Hour

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Password == "" {
		s.respondError(w, http.StatusBadRequest, "password is required")
		return
	}

	if !s.db.VerifyAdminPassword(req.Password) {
		s.respondError(w, http.StatusUnauthorized, "invalid password")
		return
	}

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
