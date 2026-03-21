# Secrets — Bulk Apply

Deploy secrets to files on LocalVault hosts. VaultCenter renders templates with real values and pushes to LocalVault, which writes files atomically.

## Prerequisites

- Secrets promoted to the target vault (see [manage.md](./manage.md))
- `VEILKEY_BULK_APPLY_ALLOWED_PATHS` set on LocalVault (see [install-localvault.md](../../../install/proxmox-lxc-debian/install-localvault.md))
- Target directory exists on LocalVault host

## 1. Create template

```bash
curl -sk -X PUT <VC_URL>/api/vaults/<AGENT_HASH>/bulk-apply/templates/<TEMPLATE_NAME> \
  -H 'Content-Type: application/json' \
  -H 'Cookie: <SESSION>' \
  -d '{
    "format": "raw",
    "target_path": "<TARGET_PATH>",
    "body": "# VeilKey managed\nDB_PASSWORD={{ VK.DB_PASSWORD }}\nAPI_KEY={{ VK.API_KEY }}\n",
    "enabled": true
  }'
```

Placeholders:
- `{{ VK.<SECRET_NAME> }}` — replaced with secret value
- `{{ VE.<CONFIG_NAME> }}` — replaced with config value

Formats: `raw`, `env`, `json`, `json_merge`

## 2. Create workflow

Workflow files are stored in VaultCenter's bulk-apply directory. From inside the VaultCenter container:

```bash
docker compose exec -T vaultcenter sh -c '
mkdir -p /data/bulk-apply/workflows/<AGENT_HASH>
cat > /data/bulk-apply/workflows/<AGENT_HASH>/<WORKFLOW_NAME>.json << EOF
{
  "apiVersion": "v1",
  "kind": "BulkApplyWorkflow",
  "name": "<WORKFLOW_NAME>",
  "vaultRuntimeHash": "<AGENT_HASH>",
  "label": "<DISPLAY_NAME>",
  "steps": [
    {"template": "<TEMPLATE_NAME>"}
  ]
}
EOF'
```

## 3. Preview (optional)

```bash
curl -sk -X POST <VC_URL>/api/vaults/<AGENT_HASH>/bulk-apply/templates/<TEMPLATE_NAME>/preview \
  -H 'Cookie: <SESSION>'
```

Shows placeholders without revealing values.

## 4. Deploy

```bash
# Precheck
curl -sk -X POST <VC_URL>/api/vaults/<AGENT_HASH>/bulk-apply/workflows/<WORKFLOW_NAME>/precheck \
  -H 'Cookie: <SESSION>'

# Run
curl -sk -X POST <VC_URL>/api/vaults/<AGENT_HASH>/bulk-apply/workflows/<WORKFLOW_NAME>/run \
  -H 'Cookie: <SESSION>'
```

Response:
```json
{
  "status": "applied",
  "results": [{"step": "...", "status": "applied", "target": "/path/to/file"}]
}
```

## 5. Change and re-deploy

1. Change the secret value (see [manage.md](./manage.md))
2. Re-run the workflow — the template re-renders with the new value

```bash
curl -sk -X POST <VC_URL>/api/vaults/<AGENT_HASH>/bulk-apply/workflows/<WORKFLOW_NAME>/run \
  -H 'Cookie: <SESSION>'
```

## Multi-vault deployment

Run the same workflow across multiple vaults:

```bash
for AGENT in <AGENT_1> <AGENT_2> <AGENT_3>; do
  curl -sk -X POST <VC_URL>/api/vaults/$AGENT/bulk-apply/workflows/<WORKFLOW_NAME>/run \
    -H 'Cookie: <SESSION>'
done
```

Each vault must have the same template name and secrets.

## Allowed paths

LocalVault only writes to allowed paths. Configure via:

```bash
VEILKEY_BULK_APPLY_ALLOWED_PATHS=/etc/myapp/.env,/opt/service/config.json
```

Default allowed: `/opt/mattermost/...`, `/etc/gitlab/gitlab.rb`

## Hooks

Templates can trigger post-deploy hooks:

```json
{"format": "raw", "target_path": "/etc/gitlab/gitlab.rb", "hook": "reconfigure_gitlab", ...}
```

Default hooks: `reload_systemd`, `restart_mattermost`, `reconfigure_gitlab`

Custom hooks via `VEILKEY_BULK_APPLY_ALLOWED_HOOKS`:

```bash
VEILKEY_BULK_APPLY_ALLOWED_HOOKS=restart_nginx:systemctl restart nginx
```
