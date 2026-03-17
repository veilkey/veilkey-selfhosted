function vhEsc(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function vhBadge(value) {
  const normalized = String(value || "").toLowerCase();
  let klass = "warn";
  if (normalized === "ok" || normalized === "active" || normalized === "localvault") klass = "ok";
  if (normalized === "blocked" || normalized === "error") klass = "err";
  return '<span class="badge ' + klass + '">' + vhEsc(value || "unknown") + '</span>';
}

function vhCards(rows) {
  return rows.map(([label, value]) =>
    '<div class="event"><div class="event-top"><strong>' + vhEsc(label) + '</strong></div><div class="mono">' + value + '</div></div>'
  ).join("");
}

async function vhFetchJSON(path) {
  const res = await fetch(path, {headers: {accept: "application/json"}});
  if (!res.ok) {
    const text = await res.text();
    throw new Error(path + " -> " + res.status + " " + text);
  }
  return res.json();
}

function renderVaultInventory(rows) {
  const el = document.getElementById("vaults-page-table");
  if (!rows.length) {
    el.innerHTML = '<tr><td colspan="6"><div class="empty">No vault inventory rows available.</div></td></tr>';
    return;
  }
  el.innerHTML = rows.map((row) =>
    '<tr>' +
      '<td class="mono"><a href="/ui/vaults/' + encodeURIComponent(row.vault_hash || row.vault_runtime_hash || "") + '">' + vhEsc(row.vault_hash || row.vault_runtime_hash || "") + '</a></td>' +
      '<td>' + vhBadge(row.vault_type || "unknown") + '</td>' +
      '<td class="mono">' + vhEsc(row.vault_node_uuid || row.node_id || "") + '</td>' +
      '<td class="mono">' + vhEsc(row.managed_path || "-") + '</td>' +
      '<td>' + vhEsc(row.ref_count ?? 0) + '</td>' +
      '<td>' + vhEsc(row.config_count ?? 0) + '</td>' +
    '</tr>'
  ).join("");
}

async function loadVaultsPage() {
  const list = await vhFetchJSON("/api/vault-inventory?limit=50");
  const rows = list.vaults || [];
  document.getElementById("vaults-page-chip").textContent = (list.count ?? rows.length) + " rows";
  renderVaultInventory(rows);

  const preview = rows[0];
  if (!preview) {
    document.getElementById("vaults-preview-chip").textContent = "Empty";
    document.getElementById("vaults-preview-summary").innerHTML = '<div class="empty">No vault preview available.</div>';
    document.getElementById("vaults-preview-context").innerHTML = '<div class="empty">No vault context available.</div>';
    return;
  }

  document.getElementById("vaults-preview-chip").innerHTML = vhBadge(preview.vault_type || "vault");
  document.getElementById("vaults-preview-summary").innerHTML = vhCards([
    ["Vault Hash", '<a href="/ui/vaults/' + encodeURIComponent(preview.vault_hash || preview.vault_runtime_hash || "") + '">' + vhEsc(preview.vault_hash || preview.vault_runtime_hash || "") + '</a>'],
    ["Vault Type", vhBadge(preview.vault_type || "unknown")],
    ["Ref Count", vhEsc(preview.ref_count ?? 0)],
    ["Config Count", vhEsc(preview.config_count ?? 0)],
  ]);
  document.getElementById("vaults-preview-context").innerHTML = vhCards([
    ["Vault Node UUID", '<span class="mono">' + vhEsc(preview.vault_node_uuid || preview.node_id || "") + '</span>'],
    ["Managed Path", '<span class="mono">' + vhEsc(preview.managed_path || "-") + '</span>'],
    ["Runtime Hash", '<span class="mono">' + vhEsc(preview.vault_runtime_hash || "") + '</span>'],
    ["Approval Route", preview.vault_runtime_hash ? '<a href="/ui/approvals?agent=' + encodeURIComponent(preview.vault_runtime_hash) + '">' + vhEsc(preview.vault_runtime_hash) + '</a>' : '<span class="muted">n/a</span>'],
  ]);
}

loadVaultsPage().catch((err) => {
  document.getElementById("vaults-page-chip").textContent = "Load failed";
  document.getElementById("vaults-page-table").innerHTML = '<tr><td colspan="6"><div class="empty">' + vhEsc(err.message) + '</div></td></tr>';
  document.getElementById("vaults-preview-summary").innerHTML = '<div class="empty">Vault preview failed to load.</div>';
  document.getElementById("vaults-preview-context").innerHTML = '<div class="empty">Vault context failed to load.</div>';
});
