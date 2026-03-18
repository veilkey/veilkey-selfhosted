package admin

import "net/http"

func (h *Handler) handleAdminHTMLOneShotPreview(w http.ResponseWriter, r *http.Request) {
	h.handleAdminVuePreview(w, r)
}

// HandleAdminHTMLOneShotPreview is the exported entry point for the HTML-only preview route.
func (h *Handler) HandleAdminHTMLOneShotPreview(w http.ResponseWriter, r *http.Request) {
	h.handleAdminHTMLOneShotPreview(w, r)
}
