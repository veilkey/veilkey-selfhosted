# Changelog

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
