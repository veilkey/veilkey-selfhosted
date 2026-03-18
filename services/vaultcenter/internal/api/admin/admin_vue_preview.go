package admin

import "net/http"

func (h *Handler) handleAdminVuePreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if body, ok := DevUIIndex(); ok {
		_, _ = w.Write(body)
		return
	}
	if body, ok := EmbeddedUIIndex(); ok {
		_, _ = w.Write(body)
		return
	}
	http.Error(w, "admin ui build is not available", http.StatusServiceUnavailable)
}

// HandleAdminVuePreview is the exported entry point for the Vue preview route.
func (h *Handler) HandleAdminVuePreview(w http.ResponseWriter, r *http.Request) {
	h.handleAdminVuePreview(w, r)
}
