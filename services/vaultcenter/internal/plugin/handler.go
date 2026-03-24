package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// SyncDeps provides access to VaultCenter services needed for plugin sync.
type SyncDeps interface {
	FindAgentURL(hashOrLabel string) (string, error)
	HTTPClient() *http.Client
	ResolveTemplateValue(vaultHash, kind, name string) (string, bool)
}

type Handler struct {
	registry       *Registry
	sync           SyncDeps
	domainRegistry *DomainRegistry
}

func NewHandler(registry *Registry, syncDeps ...SyncDeps) *Handler {
	h := &Handler{registry: registry, domainRegistry: NewDomainRegistry()}
	if len(syncDeps) > 0 {
		h.sync = syncDeps[0]
	}
	return h
}

func (h *Handler) Register(mux *http.ServeMux, requireTrustedIP func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/plugins", h.handleList)
	mux.HandleFunc("POST /api/plugins", requireTrustedIP(h.handleInstall))
	mux.HandleFunc("GET /api/plugins/{name}", h.handleGet)
	mux.HandleFunc("DELETE /api/plugins/{name}", requireTrustedIP(h.handleRemove))
	mux.HandleFunc("POST /api/plugins/{name}/load", requireTrustedIP(h.handleLoad))
	mux.HandleFunc("POST /api/plugins/{name}/unload", requireTrustedIP(h.handleUnload))
	mux.HandleFunc("POST /api/vaults/{vault}/plugins/{name}/sync", requireTrustedIP(h.handleSync))
	mux.HandleFunc("POST /api/plugins/traefik-sync/domain-check", requireTrustedIP(h.handleDomainCheck))
	mux.HandleFunc("GET /api/plugins/{name}/api/{rest...}", h.handlePluginAPI)
	mux.HandleFunc("POST /api/plugins/{name}/api/{rest...}", requireTrustedIP(h.handlePluginAPI))
	mux.HandleFunc("PUT /api/plugins/{name}/api/{rest...}", requireTrustedIP(h.handlePluginAPI))
	mux.HandleFunc("DELETE /api/plugins/{name}/api/{rest...}", requireTrustedIP(h.handlePluginAPI))
}

func (h *Handler) handleList(w http.ResponseWriter, _ *http.Request) {
	plugins, err := h.registry.List()
	if err != nil {
		log.Printf("plugin list: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to list plugins")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"count": len(plugins), "plugins": plugins})
}

func (h *Handler) handleInstall(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		respondError(w, http.StatusBadRequest, "multipart form required")
		return
	}
	manifestStr := r.FormValue("manifest")
	if manifestStr == "" {
		respondError(w, http.StatusBadRequest, "manifest field required")
		return
	}
	var manifest PluginManifest
	if err := json.Unmarshal([]byte(manifestStr), &manifest); err != nil {
		respondError(w, http.StatusBadRequest, "invalid manifest JSON")
		return
	}
	if strings.TrimSpace(manifest.Name) == "" {
		respondError(w, http.StatusBadRequest, "manifest.name is required")
		return
	}
	file, _, err := r.FormFile("wasm")
	if err != nil {
		respondError(w, http.StatusBadRequest, "wasm file required")
		return
	}
	defer file.Close()
	wasmBytes, err := io.ReadAll(file)
	if err != nil {
		log.Printf("plugin install read wasm: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to read wasm file")
		return
	}
	if err := h.registry.Install(manifest.Name, wasmBytes, &manifest); err != nil {
		log.Printf("plugin install: %v", err)
		respondError(w, http.StatusInternalServerError, "plugin installation failed")
		return
	}
	respondJSON(w, http.StatusCreated, map[string]string{"status": "installed", "name": manifest.Name})
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	plugins, err := h.registry.List()
	if err != nil {
		log.Printf("plugin get: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to list plugins")
		return
	}
	for _, p := range plugins {
		if p.Name == name {
			respondJSON(w, http.StatusOK, p)
			return
		}
	}
	respondError(w, http.StatusNotFound, fmt.Sprintf("plugin %q not found", name))
}

func (h *Handler) handleRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if inst, ok := h.registry.Get(name); ok {
		_ = inst.Destroy(r.Context())
	}
	if err := h.registry.Remove(r.Context(), name); err != nil {
		log.Printf("plugin remove %s: %v", name, err)
		respondError(w, http.StatusNotFound, "plugin not found")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "removed", "name": name})
}

func (h *Handler) handleLoad(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	inst, err := h.registry.Load(r.Context(), name)
	if err != nil {
		log.Printf("plugin load %s: %v", name, err)
		respondError(w, http.StatusInternalServerError, "failed to load plugin")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"status": "loaded", "info": inst.Info()})
}

func (h *Handler) handleUnload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.registry.Unload(r.Context(), name); err != nil {
		log.Printf("plugin unload %s: %v", name, err)
		respondError(w, http.StatusInternalServerError, "failed to unload plugin")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "unloaded", "name": name})
}

func (h *Handler) handlePluginAPI(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	rest := r.PathValue("rest")
	inst, ok := h.registry.Get(name)
	if !ok {
		respondError(w, http.StatusNotFound, fmt.Sprintf("plugin %q not loaded", name))
		return
	}
	body, _ := io.ReadAll(r.Body)
	input := map[string]any{"method": r.Method, "path": "/" + rest, "body": string(body), "query": r.URL.Query()}
	result, err := inst.Render(r.Context(), "api_request", input)
	if err != nil {
		log.Printf("plugin api %s: %v", name, err)
		respondError(w, http.StatusInternalServerError, "plugin execution failed")
		return
	}
	if result.Error != "" {
		respondError(w, http.StatusBadRequest, result.Error)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(result.Output))
}

func (h *Handler) handleSync(w http.ResponseWriter, r *http.Request) {
	vault := r.PathValue("vault")
	name := r.PathValue("name")
	if h.sync == nil {
		respondError(w, http.StatusInternalServerError, "sync not configured")
		return
	}
	inst, ok := h.registry.Get(name)
	if !ok {
		respondError(w, http.StatusNotFound, fmt.Sprintf("plugin %q not loaded", name))
		return
	}
	body, _ := io.ReadAll(r.Body)
	var input struct {
		Action string         `json:"action"`
		Input  map[string]any `json:"input"`
	}
	if err := json.Unmarshal(body, &input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if input.Action == "" {
		respondError(w, http.StatusBadRequest, "action is required")
		return
	}
	ctx := r.Context()
	rendered, err := inst.Render(ctx, input.Action, input.Input)
	if err != nil {
		log.Printf("plugin sync render %s/%s: %v", name, input.Action, err)
		respondError(w, http.StatusInternalServerError, "render failed")
		return
	}
	if rendered.Error != "" {
		respondError(w, http.StatusBadRequest, "render validation failed")
		return
	}
	output := h.resolvePlaceholders(vault, rendered.Output)
	paths, _ := inst.Paths(ctx)
	if len(paths) == 0 {
		respondError(w, http.StatusInternalServerError, "no target paths")
		return
	}
	valid, _ := inst.Validate(ctx, paths[0], output)
	if valid != nil && !valid.OK {
		respondError(w, http.StatusBadRequest, "validation failed")
		return
	}
	hooks, _ := inst.Hooks(ctx)
	hookName := ""
	if len(hooks) > 0 {
		hookName = hooks[0].Name
	}
	agentURL, err := h.sync.FindAgentURL(vault)
	if err != nil {
		log.Printf("plugin sync agent %s: %v", vault, err)
		respondError(w, http.StatusBadGateway, "vault not reachable")
		return
	}
	step := map[string]any{"name": "plugin-sync", "format": "raw", "target_path": paths[0], "content": output}
	if hookName != "" {
		step["hook"] = hookName
	}
	payload, _ := json.Marshal(map[string]any{"name": "plugin-sync", "steps": []any{step}})
	req, _ := http.NewRequest("POST", strings.TrimRight(agentURL, "/")+"/api/bulk-apply/execute", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.sync.HTTPClient().Do(req)
	if err != nil {
		log.Printf("plugin sync push %s: %v", vault, err)
		respondError(w, http.StatusBadGateway, "push to vault failed")
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		respondError(w, resp.StatusCode, "agent rejected sync request")
		return
	}
	var result map[string]any
	_ = json.Unmarshal(respBody, &result)

	// Register domains from traefik-sync after successful sync.
	if name == "traefik-sync" {
		h.registerDomainsFromSync(vault, input.Input)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status": "synced", "plugin": name, "vault": vault,
		"target_path": paths[0], "hook": hookName, "result": result,
	})
}

func (h *Handler) handleDomainCheck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain string `json:"domain"`
		Vault  string `json:"vault"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Domain == "" || req.Vault == "" {
		respondError(w, http.StatusBadRequest, "domain and vault are required")
		return
	}
	conflict, ok := h.domainRegistry.Check(req.Domain, req.Vault)
	if !ok {
		respondJSON(w, http.StatusConflict, map[string]string{
			"error":          "domain already registered",
			"domain":         req.Domain,
			"conflict_vault": conflict,
		})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{
		"status": "available",
		"domain": req.Domain,
	})
}

// registerDomainsFromSync extracts domains from traefik-sync input and registers them.
func (h *Handler) registerDomainsFromSync(vault string, input map[string]any) {
	routesRaw, ok := input["routes"]
	if !ok {
		return
	}
	routeBytes, err := json.Marshal(routesRaw)
	if err != nil {
		return
	}
	var routes []struct {
		Domain string `json:"domain"`
	}
	if err := json.Unmarshal(routeBytes, &routes); err != nil {
		return
	}
	// Clear old domains for this vault before re-registering.
	h.domainRegistry.RemoveByVault(vault)
	for _, r := range routes {
		if r.Domain != "" {
			h.domainRegistry.Register(r.Domain, vault)
		}
	}
}

func (h *Handler) resolvePlaceholders(vault, text string) string {
	if h.sync == nil {
		return text
	}
	result := text
	for {
		start := strings.Index(result, "{{ ")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], " }}")
		if end == -1 {
			break
		}
		placeholder := result[start+3 : start+end]
		parts := strings.SplitN(placeholder, ".", 2)
		if len(parts) != 2 {
			result = result[:start] + result[start+end+3:]
			continue
		}
		kind := strings.ToLower(parts[0])
		switch kind {
		case "vk":
			kind = "secret"
		case "ve":
			kind = "config"
		}
		resolved, ok := h.sync.ResolveTemplateValue(vault, kind, parts[1])
		if ok {
			result = result[:start] + resolved + result[start+end+3:]
		} else {
			result = result[:start] + result[start+end+3:]
		}
	}
	return result
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
