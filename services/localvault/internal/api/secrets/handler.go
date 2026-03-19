package secrets

import (
	"net/http"

	"veilkey-localvault/internal/db"
)

// Deps is the interface that secrets.Handler requires from *api.Server.
// It avoids a circular import by not importing the api package itself.
type Deps interface {
	// DB returns the underlying database handle.
	DB() *db.DB

	// GetKEK returns a copy of the current KEK bytes.
	GetKEK() []byte

	// GetLocalDEK decrypts and returns the local node's DEK using the KEK.
	GetLocalDEK() ([]byte, error)

	// VaultcenterURL returns the resolved vaultcenter base URL (empty if not configured).
	VaultcenterURL() string

	// HTTPClient returns the shared HTTP client.
	HTTPClient() *http.Client
}

// Handler owns all secret-related HTTP handlers.
type Handler struct {
	deps Deps
}

// NewHandler creates a Handler backed by deps.
func NewHandler(deps Deps) *Handler {
	return &Handler{deps: deps}
}

// Register mounts all secret routes onto mux.
func (h *Handler) Register(
	mux *http.ServeMux,
	requireUnlocked func(http.HandlerFunc) http.HandlerFunc,
	requireTrustedIP func(http.HandlerFunc) http.HandlerFunc,
) {
	trusted := requireTrustedIP
	ready := requireUnlocked

	mux.HandleFunc("POST /api/secrets", trusted(ready(h.handleSaveSecret)))
	mux.HandleFunc("GET /api/secrets", ready(h.handleListSecrets))
	mux.HandleFunc("GET /api/secrets/{name}", ready(h.handleGetSecret))
	mux.HandleFunc("DELETE /api/secrets/{name}", trusted(ready(h.handleDeleteSecret)))

	mux.HandleFunc("GET /api/resolve/{ref}", ready(h.handleResolveSecret))
	mux.HandleFunc("POST /api/rekey", trusted(ready(h.handleRekey)))

	mux.HandleFunc("GET /api/cipher/{ref}", trusted(ready(h.handleCipher)))
	mux.HandleFunc("GET /api/cipher/{ref}/fields/{field}", trusted(ready(h.handleCipherField)))
	mux.HandleFunc("POST /api/cipher", trusted(ready(h.handleSaveCipher)))
	mux.HandleFunc("POST /api/decrypt", trusted(ready(h.handleDecrypt)))
	mux.HandleFunc("POST /api/promote", trusted(ready(h.handlePromote)))

	mux.HandleFunc("POST /api/encrypt", ready(h.handleEncrypt))

	mux.HandleFunc("GET /api/secrets/meta/{name}", ready(h.handleGetSecretMeta))
	mux.HandleFunc("POST /api/secrets/fields", trusted(ready(h.handleSaveSecretFields)))
	mux.HandleFunc("DELETE /api/secrets/{name}/fields/{field}", trusted(ready(h.handleDeleteSecretField)))
}
