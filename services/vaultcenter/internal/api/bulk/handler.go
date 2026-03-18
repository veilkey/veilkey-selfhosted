package bulk

import (
	"net/http"

	"veilkey-vaultcenter/internal/db"
)

// Deps is the interface that the bulk handler requires from *api.Server.
// It avoids a circular import by not importing the api package itself.
type Deps interface {
	// DB returns the underlying database handle.
	DB() *db.DB

	// BulkApplyDir returns the root directory for bulk-apply file storage.
	BulkApplyDir() string

	// HTTPClient returns the shared HTTP client to use for outbound requests.
	HTTPClient() *http.Client

	// ResolveTemplateValue resolves a single placeholder value for the given
	// vault. kind is either "secret" (VK) or "config" (VE), and name/key is
	// the placeholder name.
	ResolveTemplateValue(vaultHash, kind, name string) (string, bool)

	// FindAgentURL returns the base URL (scheme://host:port) of the agent that
	// is registered for the given hashOrLabel. It returns an error when no
	// live, unblocked agent is found.
	FindAgentURL(hashOrLabel string) (string, error)
}

// Handler holds the bulk-apply HTTP handlers.
type Handler struct {
	deps Deps
}

// NewHandler creates a Handler backed by the provided Deps implementation.
func NewHandler(deps Deps) *Handler {
	return &Handler{deps: deps}
}

// Register adds all bulk-apply routes to mux.
// requireTrustedIP wraps a handler so that only trusted IP addresses may call it.
func (h *Handler) Register(mux *http.ServeMux, requireTrustedIP func(http.HandlerFunc) http.HandlerFunc) {
	// Template routes
	mux.HandleFunc("GET /api/vaults/{vault}/bulk-apply/templates", h.handleBulkApplyTemplates)
	mux.HandleFunc("POST /api/vaults/{vault}/bulk-apply/templates", requireTrustedIP(h.handleBulkApplyTemplates))
	mux.HandleFunc("GET /api/vaults/{vault}/bulk-apply/templates/{name}", h.handleBulkApplyTemplate)
	mux.HandleFunc("PUT /api/vaults/{vault}/bulk-apply/templates/{name}", requireTrustedIP(h.handleBulkApplyTemplate))
	mux.HandleFunc("DELETE /api/vaults/{vault}/bulk-apply/templates/{name}", requireTrustedIP(h.handleBulkApplyTemplate))
	mux.HandleFunc("POST /api/vaults/{vault}/bulk-apply/templates/{name}/preview", h.handleBulkApplyTemplatePreview)

	// Workflow routes
	mux.HandleFunc("GET /api/vaults/{vault}/bulk-apply/workflows", h.handleBulkApplyWorkflows)
	mux.HandleFunc("GET /api/vaults/{vault}/bulk-apply/workflows/{name}", h.handleBulkApplyWorkflow)
	mux.HandleFunc("GET /api/vaults/{vault}/bulk-apply/workflows/{name}/runs", h.handleBulkApplyWorkflowRuns)
	mux.HandleFunc("GET /api/vaults/{vault}/bulk-apply/runs/{run}", h.handleBulkApplyRun)
	mux.HandleFunc("POST /api/vaults/{vault}/bulk-apply/workflows/{name}/precheck", requireTrustedIP(h.handleBulkApplyWorkflowPrecheck))
	mux.HandleFunc("POST /api/vaults/{vault}/bulk-apply/workflows/{name}/run", requireTrustedIP(h.handleBulkApplyWorkflowRun))
}
