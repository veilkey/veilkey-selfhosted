package approval

import (
	"net/http"

	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"
)

// Handler owns all approval-flow HTTP handlers.
type Handler struct {
	db         *db.DB
	salt       []byte
	httpClient *http.Client
}

func NewHandler(database *db.DB, salt []byte, httpClient *http.Client) *Handler {
	return &Handler{
		db:         database,
		salt:       salt,
		httpClient: httpClient,
	}
}

// Register mounts all approval routes onto mux.
func (h *Handler) Register(
	mux *http.ServeMux,
	requireTrustedIP func(http.HandlerFunc) http.HandlerFunc,
) {
	mux.HandleFunc("GET /ui/approvals", h.handleHub)
	mux.HandleFunc("GET /ui/approvals/", h.handleHub)
	mux.HandleFunc("GET /ui/approvals/email-otp", h.handleEmailOTPPage)
	mux.HandleFunc("POST /ui/approvals/email-otp", h.handleSubmitEmailOTP)
	mux.HandleFunc("GET /ui/approvals/secret-input", h.handleSecretInputPage)
	mux.HandleFunc("POST /ui/approvals/secret-input", h.handleSubmitSecretInput)
	mux.HandleFunc("GET /approve/t/{token}", h.handleApprovalTokenPage)
	mux.HandleFunc("POST /approve/t/{token}", h.handleApprovalTokenSubmit)
	mux.HandleFunc("POST /api/approvals/email-otp/request", requireTrustedIP(h.handleCreateEmailOTPChallenge))
	mux.HandleFunc("GET /api/approvals/email-otp/state", h.handleEmailOTPState)
	mux.HandleFunc("POST /api/approvals/secret-input/request", requireTrustedIP(h.handleCreateSecretInputChallenge))
}

func respond(w http.ResponseWriter, status int, data any) {
	httputil.RespondJSON(w, status, data)
}

func respondErr(w http.ResponseWriter, status int, msg string) {
	httputil.RespondError(w, status, msg)
}
