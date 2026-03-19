package bulk

import "net/http"

// Handler owns all bulk-apply HTTP handlers.
// It has no external dependencies — bulk-apply operates on the local filesystem.
type Handler struct{}

// NewHandler creates a bulk Handler.
func NewHandler() *Handler {
	return &Handler{}
}

// Register mounts all bulk-apply routes onto mux.
func (h *Handler) Register(
	mux *http.ServeMux,
	requireUnlocked func(http.HandlerFunc) http.HandlerFunc,
	requireTrustedIP func(http.HandlerFunc) http.HandlerFunc,
) {
	trusted := requireTrustedIP
	ready := requireUnlocked

	mux.HandleFunc("POST /api/bulk-apply/precheck", trusted(ready(h.handleBulkApplyPrecheck)))
	mux.HandleFunc("POST /api/bulk-apply/execute", trusted(ready(h.handleBulkApplyExecute)))
}
