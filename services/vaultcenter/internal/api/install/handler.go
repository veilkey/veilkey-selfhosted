package install

import (
	"net/http"
	"sync"

	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"
)

// Handler owns all install-flow state and HTTP handlers.
type Handler struct {
	db   *db.DB
	salt []byte

	mu    sync.RWMutex
	state installApplyState
}

func NewHandler(database *db.DB, salt []byte) *Handler {
	return &Handler{db: database, salt: salt}
}

// Register mounts all install routes onto mux.
func (h *Handler) Register(
	mux *http.ServeMux,
	requireTrustedIP func(http.HandlerFunc) http.HandlerFunc,
	requireUnlocked func(http.HandlerFunc) http.HandlerFunc,
) {
	trusted := requireTrustedIP
	unlocked := requireUnlocked
	readyForOps := func(next http.HandlerFunc) http.HandlerFunc {
		return trusted(unlocked(next))
	}

	mux.HandleFunc("GET /api/install/runtime-config", readyForOps(h.handleGetInstallRuntimeConfig))
	mux.HandleFunc("PATCH /api/install/runtime-config", readyForOps(h.handlePatchInstallRuntimeConfig))
	mux.HandleFunc("GET /api/install/apply", readyForOps(h.handleGetInstallApply))
	mux.HandleFunc("GET /api/install/runs", readyForOps(h.handleGetInstallRuns))
	mux.HandleFunc("POST /api/install/validate", readyForOps(h.handleValidateInstallApply))
	mux.HandleFunc("POST /api/install/apply", readyForOps(h.handleRunInstallApply))
	mux.HandleFunc("GET /api/install/state", trusted(h.handleGetInstallState))
	mux.HandleFunc("POST /api/install/session", trusted(h.handleCreateInstallSession))
	mux.HandleFunc("PATCH /api/install/state", trusted(h.handlePatchInstallState))
	mux.HandleFunc("POST /api/install/bootstrap/request", trusted(h.handleCreateInstallBootstrapChallenge))
	mux.HandleFunc("POST /api/install/custody/request", trusted(h.handleCreateInstallCustodyChallenge))
	mux.HandleFunc("GET /approve/install/custody", h.HandleInstallCustodyPage)
	mux.HandleFunc("POST /approve/install/custody", h.HandleSubmitInstallCustody)
	mux.HandleFunc("GET /approve/install/bootstrap", h.handleInstallBootstrapPage)
	mux.HandleFunc("GET /ui/install/custody", h.HandleInstallCustodyPage)
	mux.HandleFunc("POST /ui/install/custody", h.HandleSubmitInstallCustody)
}

func (h *Handler) snapshotState() installApplyState {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.state
}

func (h *Handler) setState(state installApplyState) {
	h.mu.Lock()
	h.state = state
	h.mu.Unlock()
}

func respond(w http.ResponseWriter, status int, data any) {
	httputil.RespondJSON(w, status, data)
}

func respondErr(w http.ResponseWriter, status int, msg string) {
	httputil.RespondError(w, status, msg)
}
