package hkm

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"regexp"

	"encoding/base64"
	"fmt"
	"strings"

	"veilkey-vaultcenter/internal/crypto"
	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"
)

func respondJSON(w http.ResponseWriter, status int, data any) {
	httputil.RespondJSON(w, status, data)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	httputil.RespondError(w, status, msg)
}

// GenerateSecretRef generates a random hex ref of given length (exported for use by api package).
func GenerateSecretRef(length int) (string, error) {
	return generateSecretRef(length)
}

// generateSecretRef generates a random hex ref of given length.
func generateSecretRef(length int) (string, error) {
	b := make([]byte, (length+1)/2)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b)[:length], nil
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
func AgentScheme() string {
	if scheme := os.Getenv("VEILKEY_AGENT_SCHEME"); scheme != "" {
		return scheme
	}
	if os.Getenv("VEILKEY_TLS_CERT") != "" {
		return "https"
	}
	return "http"
}

var validResourceName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

func isValidResourceName(name string) bool {
	return validResourceName.MatchString(name)
}

// getLocalDEK retrieves and decrypts the local node's DEK using the server KEK.
func (h *Handler) getLocalDEK() ([]byte, error) {
	return h.deps.GetLocalDEK()
}

// resolveTempRef decrypts a temporary (session-scoped) encrypted ref.
func (h *Handler) resolveTempRef(tracked *db.TokenRef) (string, error) {
	parts := strings.SplitN(tracked.Ciphertext, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid temp ciphertext format")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode nonce: %w", err)
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
