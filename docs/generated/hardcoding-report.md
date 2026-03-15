# Hardcoding Report

This file is generated from `docs/cue/hardcoding.cue`.

## Summary

| Pattern | Files | Hits |
|---|---:|---:|
| `127\\.0\\.0\\.1` | 1 | 1 |
| `localhost` | 8 | 10 |
| `10180` | 62 | 159 |
| `10181` | 11 | 17 |
| `18080` | 5 | 10 |
| `18081` | 10 | 24 |
| `18083` | 5 | 8 |
| `18084` | 5 | 14 |

## `127\\.0\\.0\\.1`

Matches: 1 across 1 files.

```text
/opt/veilkey-selfhosted-repo/client/cli/patterns.yml:946:  - '(?i)^(true|false|null|none|undefined|localhost|127\.0\.0\.1|0\.0\.0\.0)$'

```

## `localhost`

Matches: 10 across 8 files.

```text
/opt/veilkey-selfhosted-repo/services/localvault/internal/api/handle_bulk_apply_test.go:39:				"content":     `{"ServiceSettings":{"SiteURL":"https://mattermost.50.internal.kr"},"SqlSettings":{"DataSource":"postgres://mmuser:***@localhost:5432/mattermost"}}`,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/install_custody.go:205:		from = "veilkey@localhost"
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_veilroot_session.sh:25:export NO_PROXY=127.0.0.1,localhost,.internal.kr,.vhost.kr
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_veilroot_session.sh:34:export NO_PROXY=127.0.0.1,localhost,.internal.kr,.vhost.kr
/opt/veilkey-selfhosted-repo/services/proxy/deploy/host/session-tools.toml.example:4:no_proxy = "127.0.0.1,localhost,.internal.kr,.vhost.kr"
/opt/veilkey-selfhosted-repo/services/proxy/policy/proxy-profiles.toml:4:no_proxy = "127.0.0.1,localhost,.internal.kr,.vhost.kr"
/opt/veilkey-selfhosted-repo/client/cli/tests/test_veilroot_session.sh:25:export NO_PROXY=127.0.0.1,localhost,.internal.kr,.vhost.kr
/opt/veilkey-selfhosted-repo/client/cli/tests/test_veilroot_session.sh:34:export NO_PROXY=127.0.0.1,localhost,.internal.kr,.vhost.kr
/opt/veilkey-selfhosted-repo/client/cli/deploy/host/session-tools.toml.example:4:no_proxy = "127.0.0.1,localhost,.internal.kr,.vhost.kr"
/opt/veilkey-selfhosted-repo/client/cli/patterns.yml:946:  - '(?i)^(true|false|null|none|undefined|localhost|127\.0\.0\.1|0\.0\.0\.0)$'

```

## `10180`

Matches: 159 across 62 files.

```text
/opt/veilkey-selfhosted-repo/services/localvault/cmd/main.go:117:	listenPort := 10180
/opt/veilkey-selfhosted-repo/services/localvault/tests/test_deploy_lxc_env_migration.sh:12:VEILKEY_ADDR=:10180
/opt/veilkey-selfhosted-repo/services/localvault/tests/test_deploy_lxc_env_migration.sh:14:VEILKEY_HUB_URL=http://10.50.2.6:10180
/opt/veilkey-selfhosted-repo/services/localvault/tests/test_deploy_lxc_env_migration.sh:34:grep -q '^VEILKEY_KEYCENTER_URL=http://10.50.2.6:10180$' "${env_file}"
/opt/veilkey-selfhosted-repo/services/localvault/Dockerfile:18:ENV VEILKEY_ADDR=:10180
/opt/veilkey-selfhosted-repo/services/localvault/Dockerfile:19:EXPOSE 10180
/opt/veilkey-selfhosted-repo/services/localvault/internal/api/keycenter_target_test.go:7:	if err := server.db.SaveConfig("VEILKEY_KEYCENTER_URL", "http://db.example:10180"); err != nil {
/opt/veilkey-selfhosted-repo/services/localvault/internal/api/keycenter_target_test.go:10:	t.Setenv("VEILKEY_KEYCENTER_URL", "http://env.example:10180")
/opt/veilkey-selfhosted-repo/services/localvault/internal/api/keycenter_target_test.go:11:	t.Setenv("VEILKEY_HUB_URL", "http://legacy.example:10180")
/opt/veilkey-selfhosted-repo/services/localvault/internal/api/keycenter_target_test.go:14:	if target.URL != "http://env.example:10180" {
/opt/veilkey-selfhosted-repo/services/localvault/internal/api/keycenter_target_test.go:27:	if err := server.db.SaveConfig("VEILKEY_HUB_URL", "http://db-legacy.example:10180"); err != nil {
/opt/veilkey-selfhosted-repo/services/localvault/internal/api/keycenter_target_test.go:32:	if target.URL != "http://db-legacy.example:10180" {
/opt/veilkey-selfhosted-repo/services/localvault/internal/api/heartbeat_test.go:89:	if err := server.SendHeartbeatOnce(ts.URL, "rotate-vault", 10180); err == nil || err.Error() != "rotation_required" {
/opt/veilkey-selfhosted-repo/services/localvault/internal/api/heartbeat_test.go:110:		"port":            10180,
/opt/veilkey-selfhosted-repo/services/localvault/internal/api/heartbeat_test.go:149:	if err := server.SendHeartbeatOnce(ts.URL, "latest-vault", 10180); err != nil {
/opt/veilkey-selfhosted-repo/services/localvault/internal/api/handle_reencrypt_test.go:248:	if err := server.db.SaveConfig("VEILKEY_KEYCENTER_URL", "http://stale.example:10180"); err != nil {
/opt/veilkey-selfhosted-repo/services/keycenter/cmd/main.go:50:	addr := getEnvDefault("VEILKEY_ADDR", ":10180")
/opt/veilkey-selfhosted-repo/services/keycenter/.env.example:12:VEILKEY_ADDR=:10180
/opt/veilkey-selfhosted-repo/services/keycenter/docker-entrypoint.sh:6:ADDR="${VEILKEY_ADDR:-:10180}"
/opt/veilkey-selfhosted-repo/services/keycenter/Dockerfile:18:ENV VEILKEY_ADDR=:10180
/opt/veilkey-selfhosted-repo/services/keycenter/Dockerfile:19:EXPOSE 10180
/opt/veilkey-selfhosted-repo/services/keycenter/scripts/sync-agent-inventory.sh:47:  [[ -n "$port" ]] || port="10180"
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_ref_tracking_test.go:19:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_ref_tracking_test.go:73:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_ref_tracking_test.go:123:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_ref_tracking_test.go:201:		"port":        10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_ref_tracking_test.go:210:		"port":        10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_ref_tracking_test.go:246:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_ref_tracking_test.go:291:		"port":        10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_heartbeat_version_chain_test.go:13:	url := "http://198.51.100.13:10180"
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/open_readiness_rehearsal_test.go:13:		"VEILKEY_KEYCENTER_URL": "http://127.0.0.1:10180",
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/open_readiness_rehearsal_test.go:61:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/open_readiness_rehearsal_test.go:77:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_heartbeat_unknown_child_test.go:13:		"node_id": crypto.GenerateUUID(), "url": "http://198.51.100.99:10180",
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_configs_bulk_safety_test.go:93:	registerMockAgent(t, srv, "svc-1", map[string]string{"HUB_URL": "http://old:10180"}, nil)
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_configs_bulk_safety_test.go:94:	registerMockAgent(t, srv, "svc-2", map[string]string{"HUB_URL": "http://new:10180"}, nil)
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_configs_bulk_safety_test.go:98:		"value": "http://new:10180",
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_operator_catalog_test.go:12:	if err := srv.db.UpsertAgent("node-a", "agent-a", "vh-a", "vault-a", "10.0.0.10", 10180, 1, 1, 1, 1); err != nil {
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_operator_catalog_test.go:39:	if err := srv.db.UpsertAgent("node-a", "agent-a", "vh-a", "vault-a", "10.0.0.10", 10180, 1, 1, 1, 1); err != nil {
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_operator_catalog_test.go:45:	if err := srv.db.UpsertAgent("node-b", "agent-b", "vh-b", "vault-b", "10.0.0.11", 10180, 1, 1, 1, 1); err != nil {
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_status_contract_test.go:40:		"url":     "http://child-status-contract-01:10180",
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_ref_audit_test.go:130:				"port":        10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/secret_input_test.go:140:		"endpoint":    "http://127.0.0.1:10180/api/agents/veilkey-hostvault",
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/secret_input_test.go:150:		"endpoint":    "http://127.0.0.1:10180/api/agents/veilkey-hostvault",
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_configs_bulk_set.go:24:// Request: {"key": "VEILKEY_KEYCENTER_URL", "value": "http://your-hub:10180"}
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_set_parent_test.go:12:	w := postJSON(handler, "/api/set-parent", map[string]string{"parent_url": "http://198.51.100.1:10180"})
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_set_parent_test.go:22:	if resp.ParentURL != "http://198.51.100.1:10180" {
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_unregister_test.go:17:		"port":            10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:35:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:101:		"port":10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:152:		"port":10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:205:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:238:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:254:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:289:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:306:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:327:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:344:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:364:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:381:			"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:398:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:448:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:464:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:498:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:529:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:558:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:578:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:624:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:673:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:689:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:705:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:725:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:742:			"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_agent_heartbeat_vault_fields_test.go:762:		"port":          10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_status_tracked_refs_contract_test.go:19:		"port":            10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_ref_cleanup_test.go:15:		"port":            10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_vault_node_uuid_alias_test.go:14:		"url":             "http://198.51.100.10:10180",
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_vault_node_uuid_alias_test.go:32:	const url = "http://198.51.100.11:10180"
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_vault_node_uuid_alias_test.go:64:		"port":            10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_vault_node_uuid_alias_test.go:105:		"port":            10180,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_heartbeat_direct_match_test.go:15:		"node_id": childNodeID, "label": "heartbeat-child", "url": "http://198.51.100.12:10180",
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_heartbeat_direct_match_test.go:21:	w = postJSON(handler, "/api/heartbeat", map[string]string{"node_id": childNodeID, "url": "http://198.51.100.12:10180"})
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_heartbeat_direct_match_test.go:35:	if child.URL != "http://198.51.100.12:10180" {
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_global_function_run.go:51:		return "http://127.0.0.1:10180"
/opt/veilkey-selfhosted-repo/services/keycenter/internal/db/models.go:167:const DefaultAgentPort = 10180
/opt/veilkey-selfhosted-repo/services/keycenter/internal/db/models.go:190:	Port             int        `gorm:"column:port;default:10180" json:"port"`
/opt/veilkey-selfhosted-repo/services/keycenter/internal/db/db_test.go:167:	if err := d.UpsertAgent("node-a", "vault-a", "vh-a", "vault-a", "127.0.0.1", 10180, 2, 3, 1, 1); err != nil {
/opt/veilkey-selfhosted-repo/services/keycenter/internal/db/db_test.go:205:	if err := d.UpsertAgent("node-host-only", "host-only-agent", "vh-host", "host-only-agent", "127.0.0.1", 10180, 0, 0, 1, 1); err != nil {
/opt/veilkey-selfhosted-repo/services/keycenter/internal/db/db_test.go:272:	if err := d.UpsertAgent("node-a", "agent-a", "vh-a", "vault-a", "10.0.0.10", 10180, 0, 0, 1, 1); err != nil {
/opt/veilkey-selfhosted-repo/services/keycenter/internal/db/db_test.go:304:	if err := d.UpsertAgent("node-a", "agent-a", "vh-a", "vault-a", "10.0.0.10", 10180, 0, 0, 1, 1); err != nil {
/opt/veilkey-selfhosted-repo/services/keycenter/internal/db/db_test.go:327:	if err := d.UpsertAgent("node-a", "agent-a", "vh-a", "vault-a", "10.0.0.10", 10180, 0, 0, 1, 1); err != nil {
/opt/veilkey-selfhosted-repo/services/keycenter/internal/db/db_test.go:826:	if err := d.UpsertAgent("node-a", "agent-a", "vh-a", "vault-a", "127.0.0.1", 10180, 1, 1, 1, 1); err != nil {
/opt/veilkey-selfhosted-repo/services/keycenter/internal/db/db_test.go:957:	if err := first.UpsertAgent("node-hostvault", "veilkey-hostvault", "vault-01", "hostvault", "127.0.0.1", 10180, 0, 0, 1, 1); err != nil {
/opt/veilkey-selfhosted-repo/services/keycenter/install/server-linux.sh:11:#   VEILKEY_ADDR          Listen address (default: :10180)
/opt/veilkey-selfhosted-repo/services/keycenter/install/server-linux.sh:23:ADDR="${VEILKEY_ADDR:-:10180}"
/opt/veilkey-selfhosted-repo/services/keycenter/install/server-macos.sh:11:#   VEILKEY_ADDR          Listen address (default: :10180)
/opt/veilkey-selfhosted-repo/services/keycenter/install/server-macos.sh:23:ADDR="${VEILKEY_ADDR:-:10180}"
/opt/veilkey-selfhosted-repo/services/proxy/cmd/veilkey-session-config/main.go:88:		"http://127.0.0.1:10180",
/opt/veilkey-selfhosted-repo/services/proxy/cmd/veilkey-session-config/main.go:97:		"http://10.50.2.6:10180",
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_session_config.sh:25:assert_contains "$out" "VEILKEY_LOCALVAULT_URL='http://127.0.0.1:10180'"
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_session_config.sh:26:assert_contains "$out" "VEILKEY_HUB_URL='http://10.50.2.6:10180'"
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_doctor_veilkey.sh:15:  *127.0.0.1:10180/health*|*10.50.2.6:10180/health*|*10.50.2.7:10180/health*)
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_doctor_veilkey.sh:47:export VEILKEY_LOCALVAULT_HEALTH_URL="http://127.0.0.1:10180/health"
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_doctor_veilkey.sh:48:export VEILKEY_KEYCENTER_HEALTH_URL="http://10.50.2.6:10180/health"
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_doctor_veilkey.sh:49:export VEILKEY_HOSTVAULT_HEALTH_URL="http://10.50.2.7:10180/health"
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_veilroot_shell_hook.sh:87:export VEILKEY_KEYCENTER_URL='http://127.0.0.1:10180'
/opt/veilkey-selfhosted-repo/services/proxy/deploy/host/session-tools.toml.example:57:localvault_url = "http://127.0.0.1:10180"
/opt/veilkey-selfhosted-repo/services/proxy/deploy/host/session-tools.toml.example:58:hub_url = "http://10.50.2.6:10180"
/opt/veilkey-selfhosted-repo/services/proxy/deploy/host/doctor-veilkey.sh:6:localvault_health_url="${VEILKEY_LOCALVAULT_HEALTH_URL:-http://127.0.0.1:10180/health}"
/opt/veilkey-selfhosted-repo/services/proxy/deploy/host/doctor-veilkey.sh:7:keycenter_health_url="${VEILKEY_KEYCENTER_HEALTH_URL:-http://10.50.2.6:10180/health}"
/opt/veilkey-selfhosted-repo/services/proxy/deploy/host/doctor-veilkey.sh:8:hostvault_health_url="${VEILKEY_HOSTVAULT_HEALTH_URL:-http://10.50.2.7:10180/health}"
/opt/veilkey-selfhosted-repo/services/proxy/deploy/host/doctor-veilkey.sh:94:  resp="$(vibe_lxc_ops "$keycenter_vmid" "curl -fsS -X POST http://127.0.0.1:10180/api/agents/veilkey-hostvault/secrets -H 'Content-Type: application/json' -d '{\"name\":\"doctor-temp-check\",\"value\":\"doctor-temp-check-value\"}'")"
/opt/veilkey-selfhosted-repo/client/cli/install-shell.sh:12:LISTEN_ADDR=":10180"
/opt/veilkey-selfhosted-repo/client/cli/install-shell.sh:89:    VEILKEY_PORT="${VEILKEY_PORT:-10180}"
/opt/veilkey-selfhosted-repo/client/cli/install-shell.sh:113:    read -rp "  포트 [10180]: " VEILKEY_PORT
/opt/veilkey-selfhosted-repo/client/cli/install-shell.sh:114:    VEILKEY_PORT="${VEILKEY_PORT:-10180}"
/opt/veilkey-selfhosted-repo/client/cli/install-shell.sh:122:        read -rp "  Parent URL (예: http://YOUR_HUB_IP:10180): " VEILKEY_PARENT_URL
/opt/veilkey-selfhosted-repo/client/cli/cmd/veilkey-session-config/main.go:87:		"http://127.0.0.1:10180",
/opt/veilkey-selfhosted-repo/client/cli/cmd/veilkey-session-config/main.go:96:		"http://10.50.2.6:10180",
/opt/veilkey-selfhosted-repo/client/cli/examples/.veilkey.yml:62:# server_url: "http://your-veilkey-server:10180"
/opt/veilkey-selfhosted-repo/client/cli/tests/test_session_config.sh:25:assert_contains "$out" "VEILKEY_LOCALVAULT_URL='http://127.0.0.1:10180'"
/opt/veilkey-selfhosted-repo/client/cli/tests/test_session_config.sh:26:assert_contains "$out" "VEILKEY_HUB_URL='http://10.50.2.6:10180'"
/opt/veilkey-selfhosted-repo/client/cli/tests/test_veilroot_shell_hook.sh:87:export VEILKEY_KEYCENTER_URL='http://127.0.0.1:10180'
/opt/veilkey-selfhosted-repo/client/cli/deploy/host/session-tools.toml.example:57:localvault_url = "http://127.0.0.1:10180"
/opt/veilkey-selfhosted-repo/client/cli/deploy/host/session-tools.toml.example:58:hub_url = "http://10.50.2.6:10180"
/opt/veilkey-selfhosted-repo/client/cli/main.go:91:		fmt.Fprintln(os.Stderr, "  export VEILKEY_LOCALVAULT_URL=http://127.0.0.1:10180")
/opt/veilkey-selfhosted-repo/client/cli/docker-compose.hub.yml:6:      - "${VEILKEY_PORT:-10180}:10180"
/opt/veilkey-selfhosted-repo/client/cli/docker-compose.hub.yml:12:      VEILKEY_ADDR: ":10180"
/opt/veilkey-selfhosted-repo/client/cli/install-docker.sh:33:    VEILKEY_PORT="${VEILKEY_PORT:-10180}"
/opt/veilkey-selfhosted-repo/client/cli/install-docker.sh:60:    read -rp "  포트 [10180]: " VEILKEY_PORT
/opt/veilkey-selfhosted-repo/client/cli/install-docker.sh:61:    VEILKEY_PORT="${VEILKEY_PORT:-10180}"
/opt/veilkey-selfhosted-repo/client/cli/install-docker.sh:71:        read -rp "  Parent URL (예: http://YOUR_HUB_IP:10180): " VEILKEY_PARENT_URL
/opt/veilkey-selfhosted-repo/client/cli/install-docker.sh:93:      - "${VEILKEY_PORT:-10180}:10180"
/opt/veilkey-selfhosted-repo/client/cli/install-docker.sh:99:      VEILKEY_ADDR: ":10180"
/opt/veilkey-selfhosted-repo/client/cli/README.md:163:export VEILKEY_LOCALVAULT_URL=http://127.0.0.1:10180
/opt/veilkey-selfhosted-repo/client/cli/install/install.sh:239:echo "  export VEILKEY_LOCALVAULT_URL=http://127.0.0.1:10180"
/opt/veilkey-selfhosted-repo/installer/INSTALL.md:94:- LocalVault: `10180`
/opt/veilkey-selfhosted-repo/installer/INSTALL.md:102:curl http://<lxc-ip>:10180/health
/opt/veilkey-selfhosted-repo/installer/tests/test_proxmox_wrapper_commands.sh:32:grep -F 'VEILKEY_ADDR=:10180' "$tmp_lxc_root/etc/veilkey/localvault.env" >/dev/null
/opt/veilkey-selfhosted-repo/installer/tests/test_proxmox_wrapper_commands.sh:50:VEILKEY_KEYCENTER_URL='http://127.0.0.1:10180' \
/opt/veilkey-selfhosted-repo/installer/tests/test_proxmox_wrapper_commands.sh:57:VEILKEY_KEYCENTER_URL='http://127.0.0.1:10180' \
/opt/veilkey-selfhosted-repo/installer/scripts/proxmox-lxc-allinone-install.sh:73:  export VEILKEY_LOCALVAULT_ADDR="${VEILKEY_LOCALVAULT_ADDR:-:10180}"
/opt/veilkey-selfhosted-repo/installer/scripts/proxmox-host-localvault/purge.sh:14:  status_json="$(curl -sf http://127.0.0.1:10180/api/status 2>/dev/null || true)"
/opt/veilkey-selfhosted-repo/installer/scripts/proxmox-host-localvault/README.md:39:  - 기본값: `0.0.0.0:10180`
/opt/veilkey-selfhosted-repo/installer/scripts/proxmox-host-localvault/install.sh:85:export VEILKEY_LOCALVAULT_ADDR="${VEILKEY_LOCALVAULT_ADDR:-0.0.0.0:10180}"
/opt/veilkey-selfhosted-repo/installer/scripts/ci/proxmox_vm_layout_test.sh:249:      export VEILKEY_KEYCENTER_URL='http://127.0.0.1:10180'
/opt/veilkey-selfhosted-repo/installer/scripts/ci/proxmox_vm_layout_test.sh:250:      export VEILKEY_HOSTVAULT_LOCALVAULT_URL='http://127.0.0.1:10180'
/opt/veilkey-selfhosted-repo/installer/scripts/ci/proxmox_vm_layout_test.sh:274:      curl -fsS http://127.0.0.1:10180/health >/dev/null || curl -fsS http://127.0.0.1:10180/api/status >/dev/null
/opt/veilkey-selfhosted-repo/installer/scripts/ci/darwin_ssh_layout_test.sh:56:    export VEILKEY_KEYCENTER_URL='http://127.0.0.1:10180'
/opt/veilkey-selfhosted-repo/installer/scripts/ci/darwin_ssh_layout_test.sh:57:    export VEILKEY_HOSTVAULT_LOCALVAULT_URL='http://127.0.0.1:10180'
/opt/veilkey-selfhosted-repo/installer/scripts/proxmox-lxc-allinone-health.sh:84:localvault_addr="$(read_addr_from_env /etc/veilkey/localvault.env :10180)"
/opt/veilkey-selfhosted-repo/installer/install.sh:562:VEILKEY_ADDR=:10180
/opt/veilkey-selfhosted-repo/installer/install.sh:628:  default_keycenter_addr=":10180"
/opt/veilkey-selfhosted-repo/installer/install.sh:629:  default_localvault_addr=":10180"
/opt/veilkey-selfhosted-repo/installer/install.sh:630:  default_keycenter_url="http://127.0.0.1:10180"
/opt/veilkey-selfhosted-repo/installer/install.sh:633:    default_localvault_addr=":10180"
/opt/veilkey-selfhosted-repo/installer/install.sh:642:  proxy_localvault_url="${VEILKEY_PROXY_LOCALVAULT_URL:-http://127.0.0.1:10180}"
/opt/veilkey-selfhosted-repo/installer/profiles/proxmox-lxc-allinone.env.example:8:# VEILKEY_LOCALVAULT_ADDR=:10180
/opt/veilkey-selfhosted-repo/installer/profiles/proxmox-host-localvault.env.example:11:# VEILKEY_LOCALVAULT_ADDR=0.0.0.0:10180

```

## `10181`

Matches: 17 across 11 files.

```text
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_ref_audit_test.go:144:				"port":        10181,
/opt/veilkey-selfhosted-repo/services/keycenter/internal/api/hkm_ref_cleanup_test.go:29:		"port":            10181,
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_session_config.sh:27:assert_contains "$out" "VEILKEY_HOSTVAULT_URL='http://10.50.2.7:10181'"
/opt/veilkey-selfhosted-repo/services/proxy/deploy/host/session-tools.toml.example:59:hostvault_url = "http://10.50.2.7:10181"
/opt/veilkey-selfhosted-repo/installer/INSTALL.md:93:- KeyCenter: `10181`
/opt/veilkey-selfhosted-repo/installer/INSTALL.md:101:curl http://<lxc-ip>:10181/health
/opt/veilkey-selfhosted-repo/installer/tests/test_proxmox_wrapper_commands.sh:31:grep -F 'VEILKEY_ADDR=:10181' "$tmp_lxc_root/etc/veilkey/keycenter.env" >/dev/null
/opt/veilkey-selfhosted-repo/installer/tests/test_proxmox_wrapper_commands.sh:33:grep -F 'VEILKEY_KEYCENTER_URL=http://127.0.0.1:10181' "$tmp_lxc_root/etc/veilkey/localvault.env" >/dev/null
/opt/veilkey-selfhosted-repo/installer/scripts/proxmox-lxc-allinone-install.sh:72:  export VEILKEY_KEYCENTER_ADDR="${VEILKEY_KEYCENTER_ADDR:-:10181}"
/opt/veilkey-selfhosted-repo/installer/scripts/proxmox-lxc-allinone-install.sh:74:  export VEILKEY_KEYCENTER_URL="${VEILKEY_KEYCENTER_URL:-http://127.0.0.1:10181}"
/opt/veilkey-selfhosted-repo/installer/scripts/ci/proxmox_vm_layout_test.sh:278:      ssh "${ssh_opts[@]}" "${PROXMOX_TEST_USER}@${host}" "curl -fsS http://127.0.0.1:10181/health >/dev/null"
/opt/veilkey-selfhosted-repo/installer/scripts/proxmox-lxc-allinone-health.sh:83:keycenter_addr="$(read_addr_from_env /etc/veilkey/keycenter.env :10181)"
/opt/veilkey-selfhosted-repo/installer/install.sh:556:VEILKEY_ADDR=:10181
/opt/veilkey-selfhosted-repo/installer/install.sh:632:    default_keycenter_addr=":10181"
/opt/veilkey-selfhosted-repo/installer/install.sh:634:    default_keycenter_url="http://127.0.0.1:10181"
/opt/veilkey-selfhosted-repo/installer/profiles/proxmox-lxc-allinone.env.example:7:# VEILKEY_KEYCENTER_ADDR=:10181
/opt/veilkey-selfhosted-repo/installer/profiles/proxmox-lxc-allinone.env.example:9:# VEILKEY_KEYCENTER_URL=http://127.0.0.1:10181

```

## `18080`

Matches: 10 across 5 files.

```text
/opt/veilkey-selfhosted-repo/services/proxy/deploy/host/session-tools.toml.example:2:listen = "127.0.0.1:18080"
/opt/veilkey-selfhosted-repo/services/proxy/deploy/host/session-tools.toml.example:3:url = "http://127.0.0.1:18080"
/opt/veilkey-selfhosted-repo/services/proxy/deploy/lxc/verify-proxy-lxc.sh:14:ss -ltnp | grep -E '18080|18081|18083|18084' || true
/opt/veilkey-selfhosted-repo/services/proxy/deploy/lxc/verify-proxy-lxc.sh:18:curl -sS -D - -o /dev/null -x http://127.0.0.1:18080 http://github.com/ | sed -n '1,8p'
/opt/veilkey-selfhosted-repo/services/proxy/deploy/lxc/verify-proxy-lxc.sh:22:curl -sS -D - -o /dev/null -x http://127.0.0.1:18080 http://example.com/ | sed -n '1,8p'
/opt/veilkey-selfhosted-repo/services/proxy/README.md:45:  - `default` -> `18080`
/opt/veilkey-selfhosted-repo/services/proxy/policy/proxy-profiles.toml:2:listen = "0.0.0.0:18080"
/opt/veilkey-selfhosted-repo/services/proxy/policy/proxy-profiles.toml:3:url = "http://10.50.2.8:18080"
/opt/veilkey-selfhosted-repo/client/cli/deploy/host/session-tools.toml.example:2:listen = "127.0.0.1:18080"
/opt/veilkey-selfhosted-repo/client/cli/deploy/host/session-tools.toml.example:3:url = "http://127.0.0.1:18080"

```

## `18081`

Matches: 24 across 10 files.

```text
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_verify_proxy_lxc.sh:9:assert_contains "$script" "/dev/tcp/127.0.0.1/18081"
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_veilroot_session.sh:21:export VEILKEY_PROXY_URL=http://10.50.2.8:18081
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_veilroot_session.sh:22:export HTTP_PROXY=http://10.50.2.8:18081
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_veilroot_session.sh:23:export HTTPS_PROXY=http://10.50.2.8:18081
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_veilroot_session.sh:24:export ALL_PROXY=http://10.50.2.8:18081
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_veilroot_session.sh:45:      codex) echo http://10.50.2.8:18081 ;;
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_veilroot_session.sh:121:printf '%s\n' "$out" | grep -q '^VEILKEY_PROXY_URL=http://10.50.2.8:18081$'
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_session_config.sh:17:assert_eq "$out" "http://127.0.0.1:18081"
/opt/veilkey-selfhosted-repo/services/proxy/deploy/host/session-tools.toml.example:10:listen = "127.0.0.1:18081"
/opt/veilkey-selfhosted-repo/services/proxy/deploy/host/session-tools.toml.example:11:url = "http://127.0.0.1:18081"
/opt/veilkey-selfhosted-repo/services/proxy/deploy/lxc/verify-proxy-lxc.sh:14:ss -ltnp | grep -E '18080|18081|18083|18084' || true
/opt/veilkey-selfhosted-repo/services/proxy/deploy/lxc/verify-proxy-lxc.sh:26:exec 3<>/dev/tcp/127.0.0.1/18081
/opt/veilkey-selfhosted-repo/services/proxy/README.md:46:  - `codex` -> `18081`
/opt/veilkey-selfhosted-repo/services/proxy/policy/proxy-profiles.toml:10:listen = "0.0.0.0:18081"
/opt/veilkey-selfhosted-repo/services/proxy/policy/proxy-profiles.toml:11:url = "http://10.50.2.8:18081"
/opt/veilkey-selfhosted-repo/client/cli/tests/test_veilroot_session.sh:21:export VEILKEY_PROXY_URL=http://10.50.2.8:18081
/opt/veilkey-selfhosted-repo/client/cli/tests/test_veilroot_session.sh:22:export HTTP_PROXY=http://10.50.2.8:18081
/opt/veilkey-selfhosted-repo/client/cli/tests/test_veilroot_session.sh:23:export HTTPS_PROXY=http://10.50.2.8:18081
/opt/veilkey-selfhosted-repo/client/cli/tests/test_veilroot_session.sh:24:export ALL_PROXY=http://10.50.2.8:18081
/opt/veilkey-selfhosted-repo/client/cli/tests/test_veilroot_session.sh:45:      codex) echo http://10.50.2.8:18081 ;;
/opt/veilkey-selfhosted-repo/client/cli/tests/test_veilroot_session.sh:121:printf '%s\n' "$out" | grep -q '^VEILKEY_PROXY_URL=http://10.50.2.8:18081$'
/opt/veilkey-selfhosted-repo/client/cli/tests/test_session_config.sh:17:assert_eq "$out" "http://127.0.0.1:18081"
/opt/veilkey-selfhosted-repo/client/cli/deploy/host/session-tools.toml.example:10:listen = "127.0.0.1:18081"
/opt/veilkey-selfhosted-repo/client/cli/deploy/host/session-tools.toml.example:11:url = "http://127.0.0.1:18081"

```

## `18083`

Matches: 8 across 5 files.

```text
/opt/veilkey-selfhosted-repo/services/proxy/deploy/host/session-tools.toml.example:26:listen = "127.0.0.1:18083"
/opt/veilkey-selfhosted-repo/services/proxy/deploy/host/session-tools.toml.example:27:url = "http://127.0.0.1:18083"
/opt/veilkey-selfhosted-repo/services/proxy/deploy/lxc/verify-proxy-lxc.sh:14:ss -ltnp | grep -E '18080|18081|18083|18084' || true
/opt/veilkey-selfhosted-repo/services/proxy/README.md:47:  - `opencode` -> `18083`
/opt/veilkey-selfhosted-repo/services/proxy/policy/proxy-profiles.toml:25:listen = "0.0.0.0:18083"
/opt/veilkey-selfhosted-repo/services/proxy/policy/proxy-profiles.toml:26:url = "http://10.50.2.8:18083"
/opt/veilkey-selfhosted-repo/client/cli/deploy/host/session-tools.toml.example:26:listen = "127.0.0.1:18083"
/opt/veilkey-selfhosted-repo/client/cli/deploy/host/session-tools.toml.example:27:url = "http://127.0.0.1:18083"

```

## `18084`

Matches: 14 across 5 files.

```text
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_veilroot_session.sh:30:export VEILKEY_PROXY_URL=http://10.50.2.8:18084
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_veilroot_session.sh:31:export HTTP_PROXY=http://10.50.2.8:18084
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_veilroot_session.sh:32:export HTTPS_PROXY=http://10.50.2.8:18084
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_veilroot_session.sh:33:export ALL_PROXY=http://10.50.2.8:18084
/opt/veilkey-selfhosted-repo/services/proxy/tests/test_veilroot_session.sh:46:      claude) echo http://10.50.2.8:18084 ;;
/opt/veilkey-selfhosted-repo/services/proxy/deploy/lxc/verify-proxy-lxc.sh:14:ss -ltnp | grep -E '18080|18081|18083|18084' || true
/opt/veilkey-selfhosted-repo/services/proxy/README.md:48:  - `claude` -> `18084`
/opt/veilkey-selfhosted-repo/services/proxy/policy/proxy-profiles.toml:18:listen = "0.0.0.0:18084"
/opt/veilkey-selfhosted-repo/services/proxy/policy/proxy-profiles.toml:19:url = "http://10.50.2.8:18084"
/opt/veilkey-selfhosted-repo/client/cli/tests/test_veilroot_session.sh:30:export VEILKEY_PROXY_URL=http://10.50.2.8:18084
/opt/veilkey-selfhosted-repo/client/cli/tests/test_veilroot_session.sh:31:export HTTP_PROXY=http://10.50.2.8:18084
/opt/veilkey-selfhosted-repo/client/cli/tests/test_veilroot_session.sh:32:export HTTPS_PROXY=http://10.50.2.8:18084
/opt/veilkey-selfhosted-repo/client/cli/tests/test_veilroot_session.sh:33:export ALL_PROXY=http://10.50.2.8:18084
/opt/veilkey-selfhosted-repo/client/cli/tests/test_veilroot_session.sh:46:      claude) echo http://10.50.2.8:18084 ;;

```

