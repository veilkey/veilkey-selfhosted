package plugin

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestTraefikSyncPlugin(t *testing.T) {
	wasm, err := os.ReadFile("/root/jeonghan/repository/veilkey-plugins/traefik-sync/traefik-sync.wasm")
	if err != nil { t.Skipf("wasm not found: %v", err) }
	ctx := context.Background()
	inst, err := LoadInstance(ctx, wasm, HostFunctions{})
	if err != nil { t.Fatalf("load: %v", err) }
	defer inst.Close(ctx)

	t.Run("info", func(t *testing.T) {
		if inst.Info().Name != "traefik-sync" { t.Errorf("name = %q", inst.Info().Name) }
	})
	t.Run("validate_tabs", func(t *testing.T) {
		r, _ := inst.Validate(ctx, "/etc/traefik/conf.d/test.yml", "http:\n\trouters:")
		if r.OK { t.Error("tabs should fail") }
	})
	t.Run("render_routes", func(t *testing.T) {
		r, _ := inst.Render(ctx, "generate_routes", map[string]any{
			"routes": []any{
				map[string]any{"name": "gitlab", "domain": "gitlab.prelik.kr", "backend": "http://10.50.0.101:8080", "tls": map[string]any{"certResolver": "letsencrypt"}},
				map[string]any{"name": "soulflow", "domain": "soulflow.prelik.kr", "backend": "http://10.50.0.104:4200"},
				map[string]any{"name": "mm", "domain": "mm.prelik.kr", "backend": "http://10.50.0.202:8065", "auth": map[string]any{"basicAuth": "admin:$apr1$hash"}},
			},
		})
		if r.Error != "" { t.Fatal(r.Error) }
		if !strings.Contains(r.Output, "gitlab.prelik.kr") { t.Error("missing gitlab") }
		if !strings.Contains(r.Output, "websecure") { t.Error("missing tls entrypoint") }
		if !strings.Contains(r.Output, "mm-auth") { t.Error("missing auth middleware") }
		if !strings.Contains(r.Output, "10.50.0.104:4200") { t.Error("missing soulflow backend") }
		t.Logf("routes:\n%s", r.Output)
	})
	t.Run("render_static", func(t *testing.T) {
		r, _ := inst.Render(ctx, "generate_static", map[string]any{"acme_email": "admin@prelik.kr"})
		if !strings.Contains(r.Output, "cloudflare") { t.Error("missing dns provider") }
		if !strings.Contains(r.Output, "admin@prelik.kr") { t.Error("missing email") }
	})
	t.Run("render_env", func(t *testing.T) {
		r, _ := inst.Render(ctx, "generate_env", map[string]any{"cf_api_email": "a@b.com", "cf_dns_api_token": "VK:LOCAL:cf-token"})
		if !strings.Contains(r.Output, "CF_DNS_API_TOKEN=VK:LOCAL:cf-token") { t.Error("missing token") }
	})
}
