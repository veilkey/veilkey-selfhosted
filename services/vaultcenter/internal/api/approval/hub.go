package approval

import "net/http"

func (h *Handler) handleHub(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(approvalHubHTML))
}

const approvalHubHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>VeilKey Approvals</title>
<link rel="stylesheet" href="/assets/operator-console.css">
</head>
<body>
<div class="shell">
  <section class="hero">
    <div class="hero-main">
      <div class="eyebrow">Approval surface</div>
      <h1>VaultCenter Approvals</h1>
      <p class="hero-copy">
        This page is the canonical entry for approval-oriented flows that VaultCenter serves directly.
        The current native scope is rebind approval, secure secret input, and install custody.
      </p>
      <div class="quick-links">
        <a href="/">Operator console</a>
        <a href="#rebind">Rebind approval</a>
        <a href="/ui/approvals/secret-input?token=example">Secret input</a>
        <a href="#custody">Install custody</a>
      </div>
    </div>
    <aside class="hero-side">
      <h2>Current scope</h2>
      <ul>
        <li>Rebind approval runs against VaultCenter APIs directly.</li>
        <li>Secure secret input now lands on VaultCenter-owned token pages.</li>
        <li>Install custody remains VaultCenter-native at <span class="mono">/ui/install/custody</span>.</li>
        <li>Legacy approval proxy routes are removed from VaultCenter.</li>
      </ul>
    </aside>
  </section>

  <main class="content">
    <section class="panel" id="rebind">
      <div class="panel-head">
        <div>
          <h2 class="panel-title">Rebind Approval</h2>
          <div class="panel-sub">Load a rebind plan by runtime hash or label, then approve it from VaultCenter.</div>
        </div>
        <div class="chip" id="approval-chip">Idle</div>
      </div>
      <div class="grid-2">
        <div class="panel" style="padding:16px;background:#0c1822">
          <label for="agent-input" class="metric-label">Agent Runtime Hash Or Label</label>
          <input id="agent-input" type="text" placeholder="example: abcd1234 or vault-label" style="width:100%;padding:12px 14px;border-radius:12px;border:1px solid #24384a;background:#08111a;color:#d9e6ef">
          <label style="display:flex;gap:10px;align-items:flex-start;margin-top:12px;color:#d9e6ef">
            <input id="approve-confirm" type="checkbox" style="margin-top:3px">
            <span>I understand rebind approval changes the active runtime key context for the selected vault.</span>
          </label>
          <div style="display:flex;gap:10px;margin-top:12px;flex-wrap:wrap">
            <button id="load-plan-btn" class="btn" style="background:#19b394">Load plan</button>
            <button id="approve-btn" class="btn">Approve rebind</button>
          </div>
          <div id="approval-result" class="result" style="display:block;margin-top:14px"></div>
        </div>
        <div class="stack" id="approval-plan">
          <div class="empty">Enter an agent runtime hash or label to load its rebind plan.</div>
        </div>
      </div>
    </section>

    <section class="panel" id="custody">
      <div class="panel-head">
        <div>
          <h2 class="panel-title">Install Custody</h2>
          <div class="panel-sub">Create a VaultCenter-native custody challenge and copy the generated approval link.</div>
        </div>
        <div class="chip" id="custody-chip">Idle</div>
      </div>
      <div class="grid-2">
        <div class="panel" style="padding:16px;background:#0c1822">
          <label for="custody-session-id" class="metric-label">Session ID</label>
          <input id="custody-session-id" type="text" placeholder="example: install-session-123" style="width:100%;padding:12px 14px;border-radius:12px;border:1px solid #24384a;background:#08111a;color:#d9e6ef">
          <label for="custody-email" class="metric-label" style="display:block;margin-top:12px">Operator Email (optional)</label>
          <input id="custody-email" type="email" placeholder="operator@example.com" style="width:100%;padding:12px 14px;border-radius:12px;border:1px solid #24384a;background:#08111a;color:#d9e6ef">
          <label for="custody-secret-name" class="metric-label" style="display:block;margin-top:12px">Secret Name</label>
          <input id="custody-secret-name" type="text" placeholder="VEILKEY_PASSWORD" style="width:100%;padding:12px 14px;border-radius:12px;border:1px solid #24384a;background:#08111a;color:#d9e6ef">
          <div style="display:flex;gap:10px;margin-top:12px;flex-wrap:wrap">
            <button id="custody-create-btn" class="btn" style="background:#19b394">Create custody link</button>
          </div>
          <div id="custody-result" class="result" style="display:block;margin-top:14px"></div>
        </div>
        <div class="stack" id="custody-output">
          <div class="empty">Enter an install session and secret name to create a custody challenge. Email is optional.</div>
        </div>
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <div>
          <h2 class="panel-title">Native Approval APIs</h2>
          <div class="panel-sub">Current VaultCenter approval surface without legacy proxy fallback.</div>
        </div>
      </div>
      <div class="stack">
        <div class="event">
          <div class="event-top"><strong>Secret input</strong></div>
          <div class="mono">POST /api/approvals/secret-input/request</div>
          <div class="mono">/ui/approvals/secret-input?token=...</div>
        </div>
        <div class="event">
          <div class="event-top"><strong>Install custody</strong></div>
          <div class="mono">/ui/install/custody?token=...</div>
          <div class="mono">POST /api/install/custody/request</div>
        </div>
        <div class="event">
          <div class="event-top"><strong>Rebind APIs</strong></div>
          <div class="mono">GET /api/agents/{agent}/rebind-plan</div>
          <div class="mono">POST /api/agents/{agent}/approve-rebind</div>
        </div>
      </div>
    </section>
  </main>
</div>
<script src="/assets/approval-console.js"></script>
</body>
</html>
`
