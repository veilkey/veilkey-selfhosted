package configs

import (
	"net/http"

	"veilkey-localvault/internal/db"
)

// Deps is the interface that configs.Handler requires from *api.Server.
type Deps interface {
	// DB returns the underlying database handle.
	DB() *db.DB
}

// Handler owns all config-related HTTP handlers.
type Handler struct {
	deps Deps
}

// NewHandler creates a Handler backed by deps.
func NewHandler(deps Deps) *Handler {
	return &Handler{deps: deps}
}

// Register mounts all config routes onto mux.
func (h *Handler) Register(
	mux *http.ServeMux,
	requireTrustedIP func(http.HandlerFunc) http.HandlerFunc,
) {
	trusted := requireTrustedIP

	mux.HandleFunc("GET /api/configs", trusted(h.handleListConfigs))
	mux.HandleFunc("GET /api/configs/{key}", trusted(h.handleGetConfig))
	mux.HandleFunc("POST /api/configs", trusted(h.handleSaveConfig))
	mux.HandleFunc("PUT /api/configs/bulk", trusted(h.handleSaveConfigsBulk))
	mux.HandleFunc("DELETE /api/configs/{key}", trusted(h.handleDeleteConfig))
}
