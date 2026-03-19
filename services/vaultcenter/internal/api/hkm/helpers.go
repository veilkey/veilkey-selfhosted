package hkm

import (
	"fmt"
	"net/http"

	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"

	"github.com/veilkey/veilkey-go-package/crypto"
)

func joinPath(base string, elem ...string) string { return httputil.JoinPath(base, elem...) }

func respondJSON(w http.ResponseWriter, status int, data any) {
	httputil.RespondJSON(w, status, data)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	httputil.RespondError(w, status, msg)
}


// federatedSecretEntry represents a secret found on a child node.
type federatedSecretEntry struct {
	NodeID  string `json:"node_id"`
	Label   string `json:"label"`
	URL     string `json:"url"`
	Name    string `json:"name"`
	Ref     string `json:"ref,omitempty"`
	Token   string `json:"token,omitempty"`
	Version int    `json:"version"`
	Value   string `json:"value,omitempty"`
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
