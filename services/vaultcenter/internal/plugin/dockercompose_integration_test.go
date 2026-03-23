package plugin

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestDockerComposeSyncPlugin(t *testing.T) {
	wasm, err := os.ReadFile("/root/jeonghan/repository/veilkey-plugins/docker-compose-sync/docker-compose-sync.wasm")
	if err != nil { t.Skipf("wasm not found: %v", err) }
	ctx := context.Background()
	inst, err := LoadInstance(ctx, wasm, HostFunctions{})
	if err != nil { t.Fatalf("load: %v", err) }
	defer inst.Close(ctx)

	t.Run("info", func(t *testing.T) {
		if inst.Info().Name != "docker-compose-sync" { t.Errorf("name = %q", inst.Info().Name) }
	})
	t.Run("init_custom_dir", func(t *testing.T) {
		r, _ := inst.Init(ctx, map[string]any{"service_dir": "/opt/activepieces", "project": "activepieces"})
		found := false
		for _, p := range r.Paths { if strings.Contains(p, "activepieces") { found = true } }
		if !found { t.Errorf("paths = %v", r.Paths) }
		if r.Hooks[0].Name != "recreate_activepieces" { t.Errorf("hook = %q", r.Hooks[0].Name) }
	})
	t.Run("validate_env_ok", func(t *testing.T) {
		r, _ := inst.Validate(ctx, "/opt/service/.env", "KEY=val\nFOO=bar")
		if !r.OK { t.Errorf("error: %s", r.Error) }
	})
	t.Run("validate_env_bad", func(t *testing.T) {
		r, _ := inst.Validate(ctx, "/opt/service/.env", "BROKEN LINE")
		if r.OK { t.Error("expected fail") }
	})
	t.Run("render_env", func(t *testing.T) {
		r, _ := inst.Render(ctx, "generate_env", map[string]any{
			"vars": map[string]any{"AP_DB_PASSWORD": "VK:LOCAL:db-pw", "AP_FRONTEND_URL": "http://10.50.0.103:8080"},
		})
		if !strings.Contains(r.Output, "AP_DB_PASSWORD=VK:LOCAL:db-pw") { t.Error("missing var") }
	})
}
