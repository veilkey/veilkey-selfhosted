# Secrets — Create, Promote, Change

## Create (Web UI)

In the keycenter UI:

1. "+ 임시키" — enter name and value
2. Select the key — "볼트에 저장 (격상)" — select vault
3. Secret is now encrypted with agentDEK and stored in LocalVault

## Create (CLI)

```bash
# Login as admin
curl -sk -X POST <VC_URL>/api/admin/login \
  -H 'Content-Type: application/json' \
  -d '{"password":"<ADMIN_PASSWORD>"}' \
  -c /tmp/vk-cookies.txt

# Create temp ref
curl -sk -X POST <VC_URL>/api/keycenter/temp-refs \
  -H 'Content-Type: application/json' \
  -b /tmp/vk-cookies.txt \
  -d '{"name":"<SECRET_NAME>","value":"<SECRET_VALUE>"}'
# Returns: {"ref":"VK:TEMP:xxxxxxxx", ...}

# List agents to get vault_hash
curl -sk <VC_URL>/api/agents
# Returns: {"agents":[{"agent_hash":"xxxxxxxx", ...}]}

# Promote to vault
curl -sk -X POST <VC_URL>/api/keycenter/promote \
  -H 'Content-Type: application/json' \
  -b /tmp/vk-cookies.txt \
  -d '{"ref":"VK:TEMP:xxxxxxxx","name":"<SECRET_NAME>","vault_hash":"<AGENT_HASH>"}'
# Returns: {"token":"VK:LOCAL:yyyyyyyy", "status":"active", ...}
```

## Change a secret

Promote the same name with a new value — it overwrites the existing secret:

```bash
# Create temp ref with new value
curl -sk -X POST <VC_URL>/api/keycenter/temp-refs \
  -H 'Content-Type: application/json' \
  -b /tmp/vk-cookies.txt \
  -d '{"name":"<SECRET_NAME>","value":"<NEW_VALUE>"}'

# Promote (same name, same vault — overwrites)
curl -sk -X POST <VC_URL>/api/keycenter/promote \
  -H 'Content-Type: application/json' \
  -b /tmp/vk-cookies.txt \
  -d '{"ref":"VK:TEMP:xxxxxxxx","name":"<SECRET_NAME>","vault_hash":"<AGENT_HASH>"}'
```

The `VK:LOCAL:` ref stays the same — all references auto-resolve to the new value.
