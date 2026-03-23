package plugin

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestSystemdSyncPlugin(t *testing.T) {
	wasm, err := os.ReadFile("/root/jeonghan/repository/veilkey-plugins/systemd-sync/systemd-sync.wasm")
	if err != nil { t.Skipf("wasm not found: %v", err) }
	ctx := context.Background()
	inst, err := LoadInstance(ctx, wasm, HostFunctions{})
	if err != nil { t.Fatalf("load: %v", err) }
	defer func() { _ = inst.Close(ctx) }()

	t.Run("info", func(t *testing.T) {
		if inst.Info().Name != "systemd-sync" { t.Errorf("name = %q", inst.Info().Name) }
	})
	t.Run("hooks", func(t *testing.T) {
		hooks, _ := inst.Hooks(ctx)
		if hooks[0].Name != "reload_systemd" { t.Errorf("hook = %q", hooks[0].Name) }
	})
	t.Run("validate_ok", func(t *testing.T) {
		r, _ := inst.Validate(ctx, "/etc/systemd/system/x.service.d/override.conf", "[Service]\nEnvironment=X=1")
		if !r.OK { t.Errorf("expected ok: %s", r.Error) }
	})
	t.Run("validate_fail", func(t *testing.T) {
		r, _ := inst.Validate(ctx, "/etc/systemd/system/x.service", "no section")
		if r.OK { t.Error("expected fail") }
	})
	t.Run("render_override", func(t *testing.T) {
		r, _ := inst.Render(ctx, "generate_override", map[string]any{"environment": map[string]any{"DB": "localhost"}, "limit_nofile": float64(65536)})
		if !strings.Contains(r.Output, "LimitNOFILE=65536") { t.Error("missing limit") }
	})
	t.Run("render_service", func(t *testing.T) {
		r, _ := inst.Render(ctx, "generate_service", map[string]any{"description": "Test", "exec_start": "/usr/bin/test"})
		if !strings.Contains(r.Output, "ExecStart=/usr/bin/test") { t.Error("missing ExecStart") }
	})
}
