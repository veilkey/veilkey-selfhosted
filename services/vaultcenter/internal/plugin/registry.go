package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Registry manages plugin lifecycle: install, load, unload, remove.
type Registry struct {
	mu        sync.RWMutex
	pluginDir string
	instances map[string]*Instance
	hostFns   HostFunctions
}

func NewRegistry(pluginDir string, hostFns HostFunctions) *Registry {
	return &Registry{pluginDir: pluginDir, instances: make(map[string]*Instance), hostFns: hostFns}
}

func (r *Registry) PluginDir() string { return r.pluginDir }

type PluginStatus struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Loaded      bool   `json:"loaded"`
	WasmFile    string `json:"wasm_file"`
	InstalledAt string `json:"installed_at,omitempty"`
}

func (r *Registry) Install(name string, wasmBytes []byte, manifest *PluginManifest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	dir := filepath.Join(r.pluginDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil { return fmt.Errorf("create plugin dir: %w", err) }
	wasmFile := name + ".wasm"
	if manifest.WasmFile != "" { wasmFile = manifest.WasmFile }
	manifest.WasmFile = wasmFile
	manifest.APIVersion = "veilkey.io/v1"
	manifest.Kind = "Plugin"
	manifest.Name = name
	if err := os.WriteFile(filepath.Join(dir, wasmFile), wasmBytes, 0o644); err != nil { return fmt.Errorf("write wasm: %w", err) }
	raw, _ := json.MarshalIndent(manifest, "", "  ")
	raw = append(raw, '\n')
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), raw, 0o644); err != nil { return fmt.Errorf("write manifest: %w", err) }
	_ = os.WriteFile(filepath.Join(dir, ".installed_at"), []byte(time.Now().UTC().Format(time.RFC3339)), 0o644)
	return nil
}

func (r *Registry) Load(ctx context.Context, name string) (*Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if inst, ok := r.instances[name]; ok { return inst, nil }
	manifest, err := r.loadManifest(name)
	if err != nil { return nil, err }
	wasmBytes, err := os.ReadFile(filepath.Join(r.pluginDir, name, manifest.WasmFile))
	if err != nil { return nil, fmt.Errorf("read wasm: %w", err) }
	inst, err := LoadInstance(ctx, wasmBytes, r.hostFns)
	if err != nil { return nil, fmt.Errorf("load instance: %w", err) }
	r.instances[name] = inst
	return inst, nil
}

func (r *Registry) Unload(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	inst, ok := r.instances[name]
	if !ok { return nil }
	if err := inst.Close(ctx); err != nil { return fmt.Errorf("close plugin: %w", err) }
	delete(r.instances, name)
	return nil
}

func (r *Registry) Remove(ctx context.Context, name string) error {
	if err := r.Unload(ctx, name); err != nil { return err }
	r.mu.Lock()
	defer r.mu.Unlock()
	dir := filepath.Join(r.pluginDir, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) { return fmt.Errorf("plugin %q not found", name) }
	return os.RemoveAll(dir)
}

func (r *Registry) Get(name string) (*Instance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	inst, ok := r.instances[name]
	return inst, ok
}

func (r *Registry) List() ([]PluginStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := os.MkdirAll(r.pluginDir, 0o755); err != nil { return nil, err }
	entries, err := os.ReadDir(r.pluginDir)
	if err != nil { return nil, err }
	var out []PluginStatus
	for _, e := range entries {
		if !e.IsDir() { continue }
		name := e.Name()
		manifest, err := r.loadManifest(name)
		if err != nil { continue }
		_, loaded := r.instances[name]
		installedAt := ""
		if raw, err := os.ReadFile(filepath.Join(r.pluginDir, name, ".installed_at")); err == nil { installedAt = strings.TrimSpace(string(raw)) }
		out = append(out, PluginStatus{Name: manifest.Name, Version: manifest.Version, Description: manifest.Description, Loaded: loaded, WasmFile: manifest.WasmFile, InstalledAt: installedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (r *Registry) LoadAll(ctx context.Context) []error {
	entries, err := os.ReadDir(r.pluginDir)
	if err != nil { return nil }
	var errs []error
	for _, e := range entries {
		if !e.IsDir() { continue }
		if _, err := r.Load(ctx, e.Name()); err != nil { errs = append(errs, fmt.Errorf("load %s: %w", e.Name(), err)) }
	}
	return errs
}

func (r *Registry) CloseAll(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, inst := range r.instances { _ = inst.Close(ctx); delete(r.instances, name) }
}

func (r *Registry) loadManifest(name string) (*PluginManifest, error) {
	raw, err := os.ReadFile(filepath.Join(r.pluginDir, name, "plugin.json"))
	if err != nil { return nil, fmt.Errorf("read manifest: %w", err) }
	var m PluginManifest
	if err := json.Unmarshal(raw, &m); err != nil { return nil, fmt.Errorf("parse manifest: %w", err) }
	return &m, nil
}
