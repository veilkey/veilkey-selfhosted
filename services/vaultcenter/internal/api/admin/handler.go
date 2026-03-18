package admin

import (
	"net/http"
	"veilkey-vaultcenter/internal/db"
)

// Deps is the interface the admin Handler uses to access server capabilities.
// *api.Server implements this interface, but it is declared here to avoid
// a circular import.
type Deps interface {
	// DB returns the underlying database handle.
	DB() *db.DB

	// GetLocalDEK retrieves and decrypts the local node's DEK.
	GetLocalDEK() ([]byte, error)

	// DecryptAgentDEK decrypts an agent's DEK using the server KEK.
	DecryptAgentDEK(encDEK, encNonce []byte) ([]byte, error)

	// FindAgentRecord returns the db.Agent for the given hash or label.
	FindAgentRecord(hashOrLabel string) (*db.Agent, error)

	// FetchAgentCiphertext retrieves the named secret ciphertext from the agent.
	FetchAgentCiphertext(agentURL, ref string) (name string, ciphertext []byte, nonce []byte, err error)

	// AgentURL builds the HTTP base URL for an agent (e.g. "http://1.2.3.4:10180").
	AgentURL(ip string, port int) string

	// SaveAuditEvent records an audit event.
	SaveAuditEvent(entityType, entityID, action, actorType, actorID, reason, source string, before, after map[string]any)
}

// Handler owns all admin HTTP handlers.
type Handler struct {
	deps Deps
}

// NewHandler creates an admin Handler backed by the provided Deps.
func NewHandler(deps Deps) *Handler {
	return &Handler{deps: deps}
}

// Register mounts all admin routes onto mux.
//
// requireReadyForOps must wrap each handler that needs the server to be
// unlocked and fully installed (mirrors api.Server.requireReadyForOps).
// requireTrustedIP restricts handlers to callers from trusted IP ranges.
func (h *Handler) Register(
	mux *http.ServeMux,
	requireReadyForOps func(http.HandlerFunc) http.HandlerFunc,
	requireTrustedIP func(http.HandlerFunc) http.HandlerFunc,
) {
	readyForOps := requireReadyForOps
	trusted := requireTrustedIP

	mux.HandleFunc("GET /api/admin/auth/settings", readyForOps(h.handleAdminAuthSettings))
	mux.HandleFunc("POST /api/admin/auth/totp/enroll/start", trusted(readyForOps(h.handleAdminTOTPEnrollStart)))
	mux.HandleFunc("POST /api/admin/auth/totp/enroll/verify", trusted(readyForOps(h.handleAdminTOTPEnrollVerify)))
	mux.HandleFunc("POST /api/admin/session/login", readyForOps(h.handleAdminSessionLogin))
	mux.HandleFunc("GET /api/admin/session", readyForOps(h.handleAdminSessionGet))
	mux.HandleFunc("DELETE /api/admin/session", readyForOps(h.handleAdminSessionDelete))
	mux.HandleFunc("GET /api/admin/approval-challenges", readyForOps(h.RequireAdminSession(h.handleAdminListApprovalChallenges)))
	mux.HandleFunc("POST /api/admin/approval-challenges/secure-input", trusted(readyForOps(h.RequireAdminSession(h.handleAdminCreateSecureInputChallenge))))
	mux.HandleFunc("GET /api/admin/audit/recent", readyForOps(h.RequireAdminSession(h.handleAdminRecentAudit)))
	mux.HandleFunc("GET /api/admin/approvals/rebind", readyForOps(h.RequireAdminSession(h.handleAdminRebindApprovalsList)))
	mux.HandleFunc("GET /api/admin/approvals/rebind/{agent}", readyForOps(h.RequireAdminSession(h.handleAdminRebindPlan)))
	mux.HandleFunc("POST /api/admin/approvals/rebind/{agent}/approve", readyForOps(h.RequireAdminSession(h.handleAdminApproveRebind)))
	mux.HandleFunc("POST /api/admin/rotations/{agent}/schedule", readyForOps(h.RequireAdminSession(h.handleAdminScheduleRotation)))
	mux.HandleFunc("POST /api/admin/rotations/schedule-all", readyForOps(h.RequireAdminSession(h.handleAdminScheduleAllRotations)))
	mux.HandleFunc("POST /api/admin/reveal-authorize", readyForOps(h.RequireAdminSession(h.handleAdminRevealAuthorize)))
	mux.HandleFunc("POST /api/admin/reveal", readyForOps(h.RequireAdminSession(h.handleAdminReveal)))
}

// RequireAdminSession returns middleware that enforces a valid admin session cookie.
func (h *Handler) RequireAdminSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := h.currentAdminSession(r); err != nil {
			respondError(w, http.StatusUnauthorized, "admin session required")
			return
		}
		next(w, r)
	}
}
