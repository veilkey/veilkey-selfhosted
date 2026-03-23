package hkm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"

	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"

	"github.com/veilkey/veilkey-go-package/crypto"
)

// agentAuthContextKey is the context key type for authenticated agent hash.
type agentAuthContextKey struct{}

// agentAuthKey is the context key for the authenticated agent hash.
var agentAuthKey = agentAuthContextKey{}

// requireAgentAuth is a middleware that validates agent Bearer tokens.
// Phase 1: if no Authorization header is present, warn but allow through.
func (h *Handler) requireAgentAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// Phase 1: warn but allow
			log.Printf("WARNING: unauthenticated agent request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
			next(w, r)
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader {
			respondError(w, http.StatusUnauthorized, "invalid authorization format")
			return
		}
		agent, err := h.authenticateAgentBySecret(token)
		if err != nil {
			respondError(w, http.StatusUnauthorized, "invalid agent secret")
			return
		}
		// Store authenticated agent hash in request context
		ctx := context.WithValue(r.Context(), agentAuthKey, agent.AgentHash)
		next(w, r.WithContext(ctx))
	}
}

// authenticateAgentBySecret validates an agent secret token by hashing it and looking up the agent.
func (h *Handler) authenticateAgentBySecret(token string) (*db.Agent, error) {
	hash := sha256.Sum256([]byte(token))
	secretHash := hex.EncodeToString(hash[:])
	return h.deps.DB().GetAgentBySecretHash(secretHash)
}

// verifyAgentAccess checks that the authenticated agent matches the URL path agent.
// Returns true if access is allowed (either no auth present in Phase 1, or agent matches).
func (h *Handler) verifyAgentAccess(r *http.Request) bool {
	authedAgent, ok := r.Context().Value(agentAuthKey).(string)
	if !ok {
		return true // Phase 1: no auth present, allow
	}
	urlAgent := r.PathValue("agent")
	return authedAgent == urlAgent
}

func joinPath(base string, elem ...string) string { return httputil.JoinPath(base, elem...) }

func respondJSON(w http.ResponseWriter, status int, data any) {
	httputil.RespondJSON(w, status, data)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	httputil.RespondError(w, status, msg)
}


// AgentScheme returns the URL scheme for agent communication.
func AgentScheme() string { return httputil.AgentScheme() }

func isValidResourceName(name string) bool { return httputil.IsValidResourceName(name) }

// getLocalDEK retrieves and decrypts the local node's DEK using the server KEK.
func (h *Handler) getLocalDEK() ([]byte, error) {
	return h.deps.GetLocalDEK()
}

// resolveTempRef decrypts a temporary (session-scoped) encrypted ref.
func (h *Handler) resolveTempRef(tracked *db.TokenRef) (string, error) {
	ciphertext, nonce, err := crypto.DecodeCiphertext(tracked.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode temp ciphertext: %w", err)
	}
	dek, err := h.getLocalDEK()
	if err != nil {
		return "", fmt.Errorf("get DEK: %w", err)
	}
	plaintext, err := crypto.Decrypt(dek, ciphertext, nonce)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}
