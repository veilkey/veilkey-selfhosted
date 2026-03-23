package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Handler struct {
	registry *Registry
}

func NewHandler(registry *Registry) *Handler {
	return &Handler{registry: registry}
}

func (h *Handler) Register(mux *http.ServeMux, requireTrustedIP func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/plugins", h.handleList)
	mux.HandleFunc("POST /api/plugins", requireTrustedIP(h.handleInstall))
	mux.HandleFunc("GET /api/plugins/{name}", h.handleGet)
	mux.HandleFunc("DELETE /api/plugins/{name}", requireTrustedIP(h.handleRemove))
	mux.HandleFunc("POST /api/plugins/{name}/load", requireTrustedIP(h.handleLoad))
	mux.HandleFunc("POST /api/plugins/{name}/unload", requireTrustedIP(h.handleUnload))
	mux.HandleFunc("GET /api/plugins/{name}/api/{rest...}", h.handlePluginAPI)
	mux.HandleFunc("POST /api/plugins/{name}/api/{rest...}", requireTrustedIP(h.handlePluginAPI))
	mux.HandleFunc("PUT /api/plugins/{name}/api/{rest...}", requireTrustedIP(h.handlePluginAPI))
	mux.HandleFunc("DELETE /api/plugins/{name}/api/{rest...}", requireTrustedIP(h.handlePluginAPI))
}

func (h *Handler) handleList(w http.ResponseWriter, _ *http.Request) {
	plugins, err := h.registry.List()
	if err != nil { respondError(w, http.StatusInternalServerError, err.Error()); return }
	respondJSON(w, http.StatusOK, map[string]any{"count": len(plugins), "plugins": plugins})
}

func (h *Handler) handleInstall(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil { respondError(w, http.StatusBadRequest, "multipart required: "+err.Error()); return }
	manifestStr := r.FormValue("manifest")
	if manifestStr == "" { respondError(w, http.StatusBadRequest, "manifest field required"); return }
	var manifest PluginManifest
	if err := json.Unmarshal([]byte(manifestStr), &manifest); err != nil { respondError(w, http.StatusBadRequest, "invalid manifest JSON: "+err.Error()); return }
	if strings.TrimSpace(manifest.Name) == "" { respondError(w, http.StatusBadRequest, "manifest.name is required"); return }
	file, _, err := r.FormFile("wasm")
	if err != nil { respondError(w, http.StatusBadRequest, "wasm file required: "+err.Error()); return }
	defer file.Close()
	wasmBytes, err := io.ReadAll(file)
	if err != nil { respondError(w, http.StatusInternalServerError, "read wasm: "+err.Error()); return }
	if err := h.registry.Install(manifest.Name, wasmBytes, &manifest); err != nil { respondError(w, http.StatusInternalServerError, err.Error()); return }
	respondJSON(w, http.StatusCreated, map[string]string{"status": "installed", "name": manifest.Name})
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	plugins, err := h.registry.List()
	if err != nil { respondError(w, http.StatusInternalServerError, err.Error()); return }
	for _, p := range plugins {
		if p.Name == name { respondJSON(w, http.StatusOK, p); return }
	}
	respondError(w, http.StatusNotFound, fmt.Sprintf("plugin %q not found", name))
}

func (h *Handler) handleRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if inst, ok := h.registry.Get(name); ok { _ = inst.Destroy(r.Context()) }
	if err := h.registry.Remove(r.Context(), name); err != nil { respondError(w, http.StatusNotFound, err.Error()); return }
	respondJSON(w, http.StatusOK, map[string]string{"status": "removed", "name": name})
}

func (h *Handler) handleLoad(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	inst, err := h.registry.Load(r.Context(), name)
	if err != nil { respondError(w, http.StatusInternalServerError, err.Error()); return }
	respondJSON(w, http.StatusOK, map[string]any{"status": "loaded", "info": inst.Info()})
}

func (h *Handler) handleUnload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.registry.Unload(r.Context(), name); err != nil { respondError(w, http.StatusInternalServerError, err.Error()); return }
	respondJSON(w, http.StatusOK, map[string]string{"status": "unloaded", "name": name})
}

func (h *Handler) handlePluginAPI(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	rest := r.PathValue("rest")
	inst, ok := h.registry.Get(name)
	if !ok { respondError(w, http.StatusNotFound, fmt.Sprintf("plugin %q not loaded", name)); return }
	body, _ := io.ReadAll(r.Body)
	input := map[string]any{"method": r.Method, "path": "/" + rest, "body": string(body), "query": r.URL.Query()}
	result, err := inst.Render(r.Context(), "api_request", input)
	if err != nil { respondError(w, http.StatusInternalServerError, "plugin error: "+err.Error()); return }
	if result.Error != "" { respondError(w, http.StatusBadRequest, result.Error); return }
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(result.Output))
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
