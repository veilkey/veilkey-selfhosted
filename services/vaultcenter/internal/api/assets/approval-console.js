function approvalEsc(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function approvalBadge(value) {
  const normalized = String(value || "").toLowerCase();
  let klass = "warn";
  if (normalized === "approved" || normalized === "plan") klass = "ok";
  if (normalized === "error" || normalized === "blocked") klass = "err";
  return '<span class="badge ' + klass + '">' + approvalEsc(value || "unknown") + '</span>';
}

function approvalCardRows(rows) {
  return rows.map(([label, value]) =>
    '<div class="event"><div class="event-top"><strong>' + approvalEsc(label) + '</strong></div><div class="mono">' + value + '</div></div>'
  ).join("");
}

async function approvalFetchJSON(path, options = {}) {
  const res = await fetch(path, {
    headers: {accept: "application/json", ...(options.headers || {})},
    ...options,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(path + " -> " + res.status + " " + text);
  }
  return res.json();
}

function approvalSetResult(text, ok) {
  const el = document.getElementById("approval-result");
  el.textContent = text;
  el.className = "result show " + (ok ? "ok" : "err");
}

function approvalSetResultHTML(html, ok) {
  const el = document.getElementById("approval-result");
  el.innerHTML = html;
  el.className = "result show " + (ok ? "ok" : "err");
}

function approvalSetChip(text) {
  const el = document.getElementById("approval-chip");
  if (el) el.textContent = text;
}

function approvalChecked(id) {
  return Boolean(document.getElementById(id)?.checked);
}

function custodySetChip(text) {
  const el = document.getElementById("custody-chip");
  if (el) el.textContent = text;
}

function custodySetResult(text, ok) {
  const el = document.getElementById("custody-result");
  if (!el) return;
  el.textContent = text;
  el.className = "result show " + (ok ? "ok" : "err");
}

function custodySetResultHTML(html, ok) {
  const el = document.getElementById("custody-result");
  if (!el) return;
  el.innerHTML = html;
  el.className = "result show " + (ok ? "ok" : "err");
}

function approvalLink(path, label) {
  return '<a href="' + path + '">' + approvalEsc(label) + '</a>';
}

function renderRebindResult(result) {
  return approvalCardRows([
    ["Status", approvalBadge(result.status || "approved")],
    ["Vault ID", '<span class="mono">' + approvalEsc(result.vault_id || "") + '</span>'],
    ["Approval Route", approvalLink("/ui/approvals?agent=" + encodeURIComponent(result.vault_runtime_hash || ""), result.vault_runtime_hash || "")],
    ["Key Version", approvalEsc(result.key_version ?? "")],
    ["Managed Paths", '<span class="mono">' + approvalEsc((result.managed_paths || []).join(", ") || "-") + '</span>'],
  ]);
}

function approvalRenderPlan(plan) {
  const el = document.getElementById("approval-plan");
  el.innerHTML = approvalCardRows([
    ["Status", approvalBadge(plan.status || "plan")],
    ["Vault ID", '<span class="mono">' + approvalEsc(plan.vault_id || "") + '</span>'],
    ["Vault Runtime Hash", '<span class="mono">' + approvalLink('/ui/approvals?agent=' + encodeURIComponent(plan.vault_runtime_hash || ""), plan.vault_runtime_hash || "") + '</span>'],
    ["Current Key Version", approvalEsc(plan.current_key_version ?? "")],
    ["Next Key Version", approvalEsc(plan.next_key_version ?? "")],
    ["Managed Paths", '<span class="mono">' + approvalEsc((plan.managed_paths || []).join(", ") || "-") + '</span>'],
    ["Flags", approvalBadge(plan.blocked ? "blocked" : (plan.rebind_required ? "rebind_required" : "clear"))],
  ]);
}

function renderCustodyOutput(resp) {
  const el = document.getElementById("custody-output");
  if (!el) return;
  el.innerHTML = approvalCardRows([
    ["Token", '<span class="mono">' + approvalEsc(resp.token || "") + '</span>'],
    ["Custody Link", approvalLink(resp.link || "#", resp.link || "")],
    ["Open Custody Page", approvalLink(resp.link || "#", "Open install custody flow")],
  ]);
}

async function loadRebindPlan() {
  const agent = document.getElementById("agent-input").value.trim();
  if (!agent) {
    approvalSetResult("agent runtime hash or label is required", false);
    return null;
  }
  approvalSetChip("Loading plan");
  const plan = await approvalFetchJSON("/api/agents/" + encodeURIComponent(agent) + "/rebind-plan");
  approvalRenderPlan(plan);
  approvalSetResultHTML(approvalCardRows([
    ["Result", approvalBadge("plan loaded")],
    ["Vault Runtime Hash", approvalLink("/ui/approvals?agent=" + encodeURIComponent(plan.vault_runtime_hash || ""), plan.vault_runtime_hash || "")],
    ["Managed Paths", '<span class="mono">' + approvalEsc((plan.managed_paths || []).join(", ") || "-") + '</span>'],
  ]), true);
  approvalSetChip("Plan loaded");
  return {agent, plan};
}

async function approveRebind() {
  const agent = document.getElementById("agent-input").value.trim();
  if (!agent) {
    approvalSetResult("agent runtime hash or label is required", false);
    return;
  }
  if (!approvalChecked("approve-confirm")) {
    approvalSetResult("confirm rebind approval before submitting the action", false);
    return;
  }
  const loaded = await loadRebindPlan();
  if (!loaded) return;
  const runtimeHash = loaded.plan.vault_runtime_hash || "";
  const confirm = window.prompt("Type APPROVE " + runtimeHash.toUpperCase());
  if (!confirm) return;
  const reason = window.prompt("Reason for approving this rebind");
  if (!reason || !reason.trim()) return;
  approvalSetChip("Approving");
  const result = await approvalFetchJSON("/api/admin/approvals/rebind/" + encodeURIComponent(agent) + "/approve", {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify({confirm, reason}),
  });
  approvalRenderPlan({
    status: result.status,
    vault_id: result.vault_id,
    vault_runtime_hash: result.vault_runtime_hash,
    current_key_version: result.key_version,
    next_key_version: result.key_version,
    managed_paths: result.managed_paths || [],
    rebind_required: false,
    blocked: false,
  });
  approvalSetResultHTML(renderRebindResult(result), true);
  approvalSetChip("Approved");
}

async function createCustodyChallenge() {
  const sessionID = document.getElementById("custody-session-id")?.value.trim() || "";
  const email = document.getElementById("custody-email")?.value.trim() || "";
  const secretName = document.getElementById("custody-secret-name")?.value.trim() || "";
  if (!sessionID || !secretName) {
    custodySetResult("session id and secret name are required", false);
    return;
  }
  custodySetChip("Creating");
  const resp = await approvalFetchJSON("/api/install/custody/request", {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify({
      session_id: sessionID,
      email,
      secret_name: secretName,
      base_url: window.location.origin,
    }),
  });
  renderCustodyOutput(resp);
  custodySetResultHTML(approvalCardRows([
    ["Result", approvalBadge("created")],
    ["Token", '<span class="mono">' + approvalEsc(resp.token || "") + '</span>'],
    ["Open Flow", approvalLink(resp.link || "#", "Open custody approval page")],
  ]), true);
  custodySetChip("Created");
}

function loadAgentFromQuery() {
  const params = new URLSearchParams(window.location.search);
  const agent = params.get("agent");
  if (!agent) return;
  const input = document.getElementById("agent-input");
  if (input) input.value = agent;
  loadRebindPlan().catch((err) => {
    approvalSetResult(err.message, false);
    approvalSetChip("Load failed");
  });
}

document.getElementById("load-plan-btn")?.addEventListener("click", async () => {
  try {
    await loadRebindPlan();
  } catch (err) {
    approvalSetResult(err.message, false);
    approvalSetChip("Load failed");
  }
});

document.getElementById("approve-btn")?.addEventListener("click", async () => {
  try {
    await approveRebind();
  } catch (err) {
    approvalSetResult(err.message, false);
    approvalSetChip("Approval failed");
  }
});

document.getElementById("custody-create-btn")?.addEventListener("click", async () => {
  try {
    await createCustodyChallenge();
  } catch (err) {
    custodySetResult(err.message, false);
    custodySetChip("Create failed");
  }
});

loadAgentFromQuery();
