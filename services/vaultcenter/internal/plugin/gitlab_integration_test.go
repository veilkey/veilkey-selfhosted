package plugin

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestGitLabPluginWASM(t *testing.T) {
	wasm, err := os.ReadFile("/root/jeonghan/repository/veilkey-plugins/gitlab/gitlab.wasm")
	if err != nil {
		t.Skipf("gitlab.wasm not found: %v", err)
	}

	ctx := context.Background()
	inst, err := LoadInstance(ctx, wasm, HostFunctions{
		ResolveSecret: func(name string) (string, bool) { return "secret-" + name, true },
		ResolveConfig: func(name string) (string, bool) { return "config-" + name, true },
	})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	defer _ = inst.Close(ctx)

	t.Run("plugin_info", func(t *testing.T) {
		info := inst.Info()
		if info.Name != "gitlab" { t.Errorf("name = %q", info.Name) }
		if info.Version != "1.0.0" { t.Errorf("version = %q", info.Version) }
		t.Logf("info: %+v", info)
	})

	t.Run("plugin_init", func(t *testing.T) {
		result, err := inst.Init(ctx, map[string]any{})
		if err != nil { t.Fatal(err) }
		if len(result.Paths) != 1 || result.Paths[0] != "/etc/gitlab/gitlab.rb" { t.Errorf("paths = %v", result.Paths) }
		if len(result.Hooks) != 1 || result.Hooks[0].Name != "reconfigure_gitlab" { t.Errorf("hooks = %v", result.Hooks) }
		t.Logf("init: paths=%v hooks=%d", result.Paths, len(result.Hooks))
	})

	t.Run("plugin_paths", func(t *testing.T) {
		paths, err := inst.Paths(ctx)
		if err != nil { t.Fatal(err) }
		if len(paths) != 1 { t.Errorf("count = %d", len(paths)) }
	})

	t.Run("plugin_hooks", func(t *testing.T) {
		hooks, err := inst.Hooks(ctx)
		if err != nil { t.Fatal(err) }
		if hooks[0].Cmd[0] != "gitlab-ctl" { t.Errorf("cmd = %v", hooks[0].Cmd) }
	})

	t.Run("plugin_validate_ok", func(t *testing.T) {
		result, err := inst.Validate(ctx, "/etc/gitlab/gitlab.rb", "external_url 'http://test'")
		if err != nil { t.Fatal(err) }
		if !result.OK { t.Errorf("expected ok, got: %s", result.Error) }
	})

	t.Run("plugin_validate_fail", func(t *testing.T) {
		result, err := inst.Validate(ctx, "/etc/gitlab/gitlab.rb", "# no url")
		if err != nil { t.Fatal(err) }
		if result.OK { t.Error("expected fail") }
		if !strings.Contains(result.Error, "external_url") { t.Errorf("error = %q", result.Error) }
	})

	t.Run("plugin_render", func(t *testing.T) {
		result, err := inst.Render(ctx, "generate_config", map[string]any{
			"external_url": "http://10.50.0.120",
			"gitlab_rails": map[string]any{"time_zone": "Asia/Seoul"},
			"nginx":        map[string]any{"listen_port": float64(80)},
		})
		if err != nil { t.Fatal(err) }
		if result.Error != "" { t.Fatal(result.Error) }
		if !strings.Contains(result.Output, "10.50.0.120") { t.Error("missing IP in output") }
		if !strings.Contains(result.Output, "Asia/Seoul") { t.Error("missing timezone") }
		t.Logf("rendered:\n%s", result.Output)
	})
}
