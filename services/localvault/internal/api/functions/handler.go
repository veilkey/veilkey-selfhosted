package functions

import (
	"net/http"
	"time"

	"veilkey-localvault/internal/db"
)

// Deps is the interface that functions.Handler requires from *api.Server.
type Deps interface {
	// DB returns the underlying database handle.
	DB() *db.DB

	// VaultHash returns the vault's hash identifier (for synced global functions).
	VaultHash() string

	// HTTPClient returns the shared HTTP client.
	HTTPClient() *http.Client
}

// Handler owns all global-function HTTP handlers.
type Handler struct {
	deps Deps
}

// NewHandler creates a Handler backed by deps.
func NewHandler(deps Deps) *Handler {
	return &Handler{deps: deps}
}

// Register mounts all function routes onto mux.
func (h *Handler) Register(
	mux *http.ServeMux,
	requireUnlocked func(http.HandlerFunc) http.HandlerFunc,
	requireTrustedIP func(http.HandlerFunc) http.HandlerFunc,
) {
	trusted := requireTrustedIP
	ready := requireUnlocked

	mux.HandleFunc("GET /api/functions", ready(h.handleFunctions))
	mux.HandleFunc("POST /api/functions", trusted(ready(h.handleFunctions)))
	mux.HandleFunc("GET /api/functions/{name...}", ready(h.handleFunction))
	mux.HandleFunc("DELETE /api/functions/{name...}", trusted(ready(h.handleFunction)))
}

// CleanupExpiredTestFunctions deletes TEST-scoped functions past their expiry.
// Called by the cron runner via forwarding method on *api.Server.
func (h *Handler) CleanupExpiredTestFunctions(now time.Time) (int, error) {
	return h.deps.DB().CleanupExpiredTestFunctions(now)
}
