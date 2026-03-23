// Package plugin defines the VeilKey plugin system built on wazero.
package plugin

// PluginInfo is returned by plugin_info().
type PluginInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

// PluginInitResult is returned by plugin_init().
type PluginInitResult struct {
	Paths     []string   `json:"paths"`
	Hooks     []HookDef  `json:"hooks"`
	APIRoutes []APIDef   `json:"api_routes,omitempty"`
}

// HookDef defines a hook that a plugin provides.
type HookDef struct {
	Name    string   `json:"name"`
	Cmd     []string `json:"cmd"`
	Depends []string `json:"depends,omitempty"`
}

// APIDef defines a custom API route the plugin exposes.
type APIDef struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// ValidateResult is returned by plugin_validate().
type ValidateResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// RenderRequest is the input to plugin_render().
type RenderRequest struct {
	Action string `json:"action"`
	Input  any    `json:"input"`
}

// RenderResult is returned by plugin_render().
type RenderResult struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// PluginManifest is the on-disk metadata stored alongside .wasm files.
type PluginManifest struct {
	APIVersion  string `json:"apiVersion"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	WasmFile    string `json:"wasmFile"`
	License     string `json:"license,omitempty"`
}
