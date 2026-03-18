package install

import (
	"net/http"

	"veilkey-vaultcenter/internal/crypto"
	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"
)

type installStatePayload struct {
	SessionID       string   `json:"session_id"`
	Version         int      `json:"version"`
	Language        string   `json:"language"`
	Quickstart      bool     `json:"quickstart"`
	Flow            string   `json:"flow"`
	DeploymentMode  string   `json:"deployment_mode"`
	InstallScope    string   `json:"install_scope"`
	BootstrapMode   string   `json:"bootstrap_mode"`
	MailTransport   string   `json:"mail_transport"`
	PlannedStages   []string `json:"planned_stages"`
	CompletedStages []string `json:"completed_stages"`
	LastStage       string   `json:"last_stage"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

type installStatePatchRequest struct {
	SessionID       string    `json:"session_id"`
	Version         *int      `json:"version"`
	Language        *string   `json:"language"`
	Quickstart      *bool     `json:"quickstart"`
	Flow            *string   `json:"flow"`
	DeploymentMode  *string   `json:"deployment_mode"`
	InstallScope    *string   `json:"install_scope"`
	BootstrapMode   *string   `json:"bootstrap_mode"`
	MailTransport   *string   `json:"mail_transport"`
	PlannedStages   *[]string `json:"planned_stages"`
	CompletedStages *[]string `json:"completed_stages"`
	LastStage       *string   `json:"last_stage"`
}

func installStateToPayload(session *db.InstallSession) installStatePayload {
	return installStatePayload{
		SessionID:       session.SessionID,
		Version:         session.Version,
		Language:        session.Language,
		Quickstart:      session.Quickstart,
		Flow:            session.Flow,
		DeploymentMode:  session.DeploymentMode,
		InstallScope:    session.InstallScope,
		BootstrapMode:   session.BootstrapMode,
		MailTransport:   session.MailTransport,
		PlannedStages:   httputil.DecodeStringList(session.PlannedStagesJSON),
		CompletedStages: httputil.DecodeStringList(session.CompletedStagesJSON),
		LastStage:       session.LastStage,
		CreatedAt:       session.CreatedAt.UTC().Format(http.TimeFormat),
		UpdatedAt:       session.UpdatedAt.UTC().Format(http.TimeFormat),
	}
}

func installStateFromPayload(req installStatePayload) *db.InstallSession {
	return &db.InstallSession{
		SessionID:           req.SessionID,
		Version:             req.Version,
		Language:            req.Language,
		Quickstart:          req.Quickstart,
		Flow:                req.Flow,
		DeploymentMode:      req.DeploymentMode,
		InstallScope:        req.InstallScope,
		BootstrapMode:       req.BootstrapMode,
		MailTransport:       req.MailTransport,
		PlannedStagesJSON:   httputil.EncodeStringList(req.PlannedStages),
		CompletedStagesJSON: httputil.EncodeStringList(req.CompletedStages),
		LastStage:           req.LastStage,
	}
}

func (h *Handler) handleCreateInstallSession(w http.ResponseWriter, r *http.Request) {
	var req installStatePayload
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SessionID == "" {
		req.SessionID = crypto.GenerateUUID()
	}
	session := installStateFromPayload(req)
	if err := h.db.SaveInstallSession(session); err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	saved, err := h.db.GetInstallSession(req.SessionID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to reload install session")
		return
	}
	respond(w, http.StatusCreated, map[string]interface{}{
		"session": installStateToPayload(saved),
	})
}

func (h *Handler) handleGetInstallState(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	var (
		session *db.InstallSession
		err     error
	)
	if sessionID != "" {
		session, err = h.db.GetInstallSession(sessionID)
	} else {
		session, err = h.db.GetLatestInstallSession()
	}
	if err != nil {
		respond(w, http.StatusOK, map[string]interface{}{
			"exists": false,
		})
		return
	}
	respond(w, http.StatusOK, map[string]interface{}{
		"exists":  true,
		"session": installStateToPayload(session),
	})
}

func (h *Handler) handlePatchInstallState(w http.ResponseWriter, r *http.Request) {
	var req installStatePatchRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SessionID == "" {
		respondErr(w, http.StatusBadRequest, "session_id is required")
		return
	}
	session, err := h.db.GetInstallSession(req.SessionID)
	if err != nil {
		respondErr(w, http.StatusNotFound, err.Error())
		return
	}
	if req.Version != nil {
		session.Version = *req.Version
	}
	if req.Language != nil {
		session.Language = *req.Language
	}
	if req.Quickstart != nil {
		session.Quickstart = *req.Quickstart
	}
	if req.Flow != nil {
		session.Flow = *req.Flow
	}
	if req.DeploymentMode != nil {
		session.DeploymentMode = *req.DeploymentMode
	}
	if req.InstallScope != nil {
		session.InstallScope = *req.InstallScope
	}
	if req.BootstrapMode != nil {
		session.BootstrapMode = *req.BootstrapMode
	}
	if req.MailTransport != nil {
		session.MailTransport = *req.MailTransport
	}
	if req.PlannedStages != nil {
		session.PlannedStagesJSON = httputil.EncodeStringList(*req.PlannedStages)
	}
	if req.CompletedStages != nil {
		session.CompletedStagesJSON = httputil.EncodeStringList(*req.CompletedStages)
	}
	if req.LastStage != nil {
		session.LastStage = *req.LastStage
	}
	if err := h.db.SaveInstallSession(session); err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	respond(w, http.StatusOK, map[string]interface{}{
		"session": installStateToPayload(session),
	})
}
