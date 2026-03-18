package admin

import "net/http"

func (h *Handler) handleAdminMockupDark(w http.ResponseWriter, r *http.Request) {
	h.handleAdminVuePreview(w, r)
}

func (h *Handler) handleAdminMockupAmber(w http.ResponseWriter, r *http.Request) {
	h.handleAdminVuePreview(w, r)
}

func (h *Handler) handleAdminMockupMono(w http.ResponseWriter, r *http.Request) {
	h.handleAdminVuePreview(w, r)
}

// HandleAdminMockupDark is the exported entry point for the dark mockup route.
func (h *Handler) HandleAdminMockupDark(w http.ResponseWriter, r *http.Request) {
	h.handleAdminMockupDark(w, r)
}

// HandleAdminMockupAmber is the exported entry point for the amber mockup route.
func (h *Handler) HandleAdminMockupAmber(w http.ResponseWriter, r *http.Request) {
	h.handleAdminMockupAmber(w, r)
}

// HandleAdminMockupMono is the exported entry point for the mono mockup route.
func (h *Handler) HandleAdminMockupMono(w http.ResponseWriter, r *http.Request) {
	h.handleAdminMockupMono(w, r)
}
