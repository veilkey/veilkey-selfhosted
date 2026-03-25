package hkm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
// Rejects requests without valid agent authentication.
func (h *Handler) requireAgentAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			respondError(w, http.StatusUnauthorized, "agent authentication required")
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

// vaultProxyContextKey marks a request as proxied from a /api/vaults/ handler.
type vaultProxyContextKey struct{}

// vaultProxyKey is the context key for vault-proxy bypass.
var vaultProxyKey = vaultProxyContextKey{}

// asVaultProxy injects the vault-proxy flag and sets the "agent" path value,
// so that agent handlers accept the request without Bearer-token auth.
func asVaultProxy(r *http.Request) *http.Request {
	vault := r.PathValue("vault")
	r.SetPathValue("agent", vault)
	ctx := context.WithValue(r.Context(), vaultProxyKey, true)
	return r.WithContext(ctx)
}

// verifyAgentAccess checks that the authenticated agent matches the URL path agent.
// Requests flagged by asVaultProxy() are always allowed (admin-initiated via vault route).
func (h *Handler) verifyAgentAccess(r *http.Request) bool {
	if bypass, _ := r.Context().Value(vaultProxyKey).(bool); bypass {
		return true
	}
	authedAgent, ok := r.Context().Value(agentAuthKey).(string)
	if !ok {
		return false
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
