package hkm

import (
	"log"
	"net/http"
	"veilkey-vaultcenter/internal/httputil"
	"net/url"
	"strings"
)

// handleSetParent sets the parent_url in node_info
func (h *Handler) handleSetParent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ParentURL string `json:"parent_url"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ParentURL == "" {
		respondError(w, http.StatusBadRequest, "parent_url is required")
		return
	}
	parsed, err := url.Parse(strings.TrimSpace(req.ParentURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		respondError(w, http.StatusBadRequest, "parent_url must be a valid http(s) URL")
		return
	}
	req.ParentURL = strings.TrimRight(parsed.String(), "/")
	_, err = h.deps.DB().SetParentURL(req.ParentURL)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("Parent URL set to %s", req.ParentURL)
	respondJSON(w, http.StatusOK, map[string]interface{}{"status": "ok", "parent_url": req.ParentURL})
}
