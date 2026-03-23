package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallAndList(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir, HostFunctions{})
	manifest := &PluginManifest{Name: "test-plugin", Version: "1.0.0", Description: "A test plugin"}
	if err := reg.Install("test-plugin", []byte("fake-wasm"), manifest); err != nil { t.Fatalf("install: %v", err) }
	if _, err := os.Stat(filepath.Join(dir, "test-plugin", "test-plugin.wasm")); err != nil { t.Fatal("wasm missing") }
	if _, err := os.Stat(filepath.Join(dir, "test-plugin", "plugin.json")); err != nil { t.Fatal("manifest missing") }
	plugins, err := reg.List()
	if err != nil { t.Fatalf("list: %v", err) }
	if len(plugins) != 1 { t.Fatalf("expected 1, got %d", len(plugins)) }
	if plugins[0].Name != "test-plugin" { t.Errorf("name = %q", plugins[0].Name) }
	if plugins[0].Loaded { t.Error("should not be loaded") }
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir, HostFunctions{})
	_ = reg.Install("removable", []byte("wasm"), &PluginManifest{Name: "removable", Version: "1.0.0"})
	if err := reg.Remove(nil, "nonexistent"); err == nil { t.Error("expected error") }
	if err := reg.Remove(nil, "removable"); err != nil { t.Fatalf("remove: %v", err) }
	plugins, _ := reg.List()
	if len(plugins) != 0 { t.Errorf("expected 0, got %d", len(plugins)) }
}

func TestProtocolTypes(t *testing.T) {
	if r := (ValidateResult{OK: true}); !r.OK { t.Error("expected ok") }
	if r := (ValidateResult{OK: false, Error: "bad"}); r.OK { t.Error("expected not ok") }
	info := PluginInfo{Name: "traefik", Version: "1.0.0"}
	if info.Name != "traefik" { t.Errorf("name = %q", info.Name) }
	hook := HookDef{Name: "restart", Cmd: []string{"systemctl", "restart", "traefik"}, Depends: []string{"systemd:reload"}}
	if hook.Depends[0] != "systemd:reload" { t.Errorf("depends = %v", hook.Depends) }
	m := PluginManifest{APIVersion: "veilkey.io/v1", Kind: "Plugin", Name: "x", WasmFile: "x.wasm"}
	if m.Kind != "Plugin" { t.Errorf("kind = %q", m.Kind) }
}
