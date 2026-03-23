# Changelog

## v0.5.0 (2026-03-23)

### Security — Vault Isolation
- **Agent auth**: Bearer token authentication between VaultCenter ↔ LocalVault (bidirectional)
- **Cross-vault blocked**: agents can only access their own secrets
- **DB encryption required**: `VEILKEY_DB_KEY` mandatory — server refuses to start without SQLCipher
- **Memory-only KEK**: removed `VEILKEY_PASSWORD_FILE` and `VEILKEY_PASSWORD` — unlock via API only
- **Admin password change**: `POST /api/admin/change-password` requires owner password (KEK verification)

### Refactoring
- PTY module split: `masker.rs`, `sync.rs`, `session.rs` (737 → 195 lines in main)
- 11 bug fixes: UB removal (static mut → AtomicI32), UTF-8 safety, long poll timeout, line-clear false positive
- 19 golangci-lint errors resolved in VaultCenter
- CI: auto-sync UI build instead of diff check

### Features
- VE ref display: original value with ref tag in distinct color
- `scripts/vk-bulk-apply-sync.sh`: automated secret sync pipeline with change detection
- Agent auth caching: reduced DB+KEK lookups

### Testing
- Go unit tests: 46 cases (LocalVault API, DB, VaultCenter httputil)
- Bats tests: 17 cases (bulk-apply sync) + 6 (security regression) + 2 (DB encryption)
- Server startup tests: locked mode, no password env vars, DB_KEY required

## v0.4.0 (2026-03-22)

### Security — PTY Masking
- **Fail-closed**: mask_map empty → block all output (no plaintext leakage)
- **Pattern issue failure**: replace with `[REDACTED]` instead of passthrough
- **base64 variants**: auto-add base64-encoded secrets to mask_map
- **Retry**: mask_map fetch retries 3x with exponential backoff

### Security — AI Agent Boundary
- `docs/security-model.md`: AI agent allowed/forbidden actions
- `examples/CLAUDE.md`: drop-in template for Claude Code projects
- Principle: AI interacts with secrets only through VK refs

### VaultCenter
- Fix: `/api/resolve/{ref}` — canonical + raw ref fallback lookup
- Fix: bulk-apply name→ref resolve via `/api/secrets/meta`
- Agent soft delete: `POST /api/agents/by-node/{id}/archive`
- Agent unarchive: `POST /api/agents/by-node/{id}/unarchive`
- Auto-archive: stale agents (no heartbeat 7d) archived by GC
- Agent health: `healthy`/`stale`/`unreachable`/`archived` + `last_seen_ago`
- `?include_archived=true` query parameter

### Frontend
- Health pill (green/yellow/red/gray) replaces status pill
- Last seen ago display
- Archive/unarchive buttons in vault table

### Deployment
- `examples/docker-entrypoint-veilkey.sh` — Docker VK ref resolution at startup
- `examples/Dockerfile.veilkey-app` — multi-stage build example
- `examples/veilkey-app.service` — systemd with veil wrapper
- All deployment modes: direct, systemd, cron, Docker — no plaintext in .env

### veil-cli
- Fix: `fetch_all_secrets_mask_map` uses `self.resolve()` with candidate fallback
- Fix: skip VE: config refs (not secrets)
- Fix: Stats.entries compile error
- cargo fmt + clippy clean

### CI
- Fix: UI build check — pin node version via `.node-version`
- Fix: relative path for `working-directory`
- Fix: remove stale ui_dist assets
- Fix: VaultCenter startup timing in smoke tests

## v0.3.0 (2026-03-21)

### Install
- **Platform-specific install structure** — `install/macos/`, `install/proxmox-lxc-debian/`, `install/common/`
- Each service has its own `install.sh` + `install.md` + `uninstall.sh`
- `install/proxmox-lxc-debian/install-localvault.sh` — TLS auto-gen, bulk-apply paths, update on re-run
- `install/common/install-veil-cli.sh` — works on any Linux (host, LXC, VM)
- Removed old `scripts/install-veil-mac.sh`, `scripts/uninstall-veil-mac.sh`

### Bulk Apply
- Configurable allowed paths via `VEILKEY_BULK_APPLY_ALLOWED_PATHS` env var
- Configurable hooks via `VEILKEY_BULK_APPLY_ALLOWED_HOOKS` env var
- Fix: resolve secret by name→ref lookup (was failing with "workflow execution failed")
- Improved error messages: include actual error in workflow failure response

### Docs
- Split `docs/setup.md` into service-based `docs/setup/` structure
- New: `docs/setup/secrets/bulk-apply.md` — template, workflow, deploy, multi-vault
- New: `docs/setup/env-vars.md` — consolidated environment variable reference
- Platform-specific setup guides: `localvault/macos.md`, `localvault/proxmox-lxc-debian.md`

### CI
- GitHub Actions smoke test workflow (`smoke.yml`)
- bats-core smoke tests: veil-cli (11 tests), localvault (9 tests)
- Retry loops for VaultCenter/LocalVault setup/unlock in CI

### License
- Changed from MIT to AGPL-3.0

### Other
- `.env.example`: uncomment `LOCALVAULT_CHAIN_PEERS=` to suppress warning
- All hardcoded ports/IPs removed from docs and scripts — use env vars/placeholders

## v0.2.0 (2026-03-20)

### veil CLI
- Real-time pattern detection (222 patterns) in PTY output
- Auto-register detected secrets as VK:TEMP
- Length-padded masking (prevents terminal cursor shift)
- PTY chunk boundary handling for split secrets
- npm package (`npm install -g veilkey-cli`)
- macOS Sequoia Gatekeeper bypass via npm + codesign
- Auto-update check on startup
- `.veilkey/env` auto-load from project directory

### VaultCenter
- `/api/refs` endpoint for auth-free ref listing
- Expired TEMP ref filtering
- HTTP connection pooling (MaxIdleConnsPerHost 2→20)
- Configurable TTL for temp refs and admin sessions

### Chain
- CometBFT chain enabled by default in docker-compose
- All DB writes converted to chain TX (veilkey-chain v0.8.0)
- SaveAuditEvent removed from handlers (executor auto-generates)

### Security
- Password file auto-save removed (memory-only)
- Admin login required after master password unlock (2-step auth)
- DEK/key material excluded from chain TX

### Install
- `bash scripts/install-veil-mac.sh` — build + npm install + docker
- Gist one-liner installer
- Port conflict detection
- Idempotent re-runs

## v0.1.0 (2026-03-19)

### Initial Release
- Web-based first-run setup (replaces Docker secrets)
- Auto-generate TLS cert, HTTPS-only
- Admin password authentication (bcrypt + session)
- Keycenter: temp ref CRUD, reveal, promote to vault
- Registration token system for localvault onboarding
- CometBFT ABCI chain layer
- agentDEK-based encryption (VC encrypts, LV stores only)
- Blockchain audit trail (immutable)
- Rate limiting on admin login
