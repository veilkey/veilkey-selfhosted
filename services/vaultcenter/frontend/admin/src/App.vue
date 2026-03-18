<template>
<div v-if="state.ui.locked" class="unlock-shell">
    <div class="unlock-card">
        <div class="unlock-brand"><span class="brand-mark">VK</span><span class="brand-name">VeilKey</span></div>
        <h1 class="unlock-heading">잠금 해제</h1>
        <p class="unlock-desc">마스터 비밀번호를 입력하세요.</p>
        <div v-if="state.ui.unlockError" class="unlock-error">{{ state.ui.unlockError }}</div>
        <form class="unlock-form" @submit.prevent="(e) => unlock(e.target.password.value)">
            <input class="unlock-input" type="password" name="password" placeholder="비밀번호" autocomplete="current-password" autofocus required />
            <button class="unlock-btn" type="submit">잠금 해제</button>
        </form>
    </div>
</div>
<div v-else class="app" data-app="vaultcenter-admin-shell">
    <header class="topbar">
        <div class="topbar-brand">
            <span class="topbar-brand-mark">VK</span>
            <span>{{ t('app_title') }}</span>
        </div>
        <div class="topbar-center">
            <div class="global-search">
                <input id="global-search" type="search" :placeholder="t('search_current_view')" :value="state.globalQuery" @input="onGlobalSearchInput">
            </div>
        </div>
        <div class="topbar-status" id="topbar-status" v-html="state.ui.topbarStatusHTML"></div>
    </header>
    <div class="shell">
        <aside class="sidebar" id="sidebar" v-html="state.ui.sidebarHTML"></aside>
        <aside class="sidebar" id="secondary-sidebar" v-show="state.activePage === 'vaults' || !state.ui.secondarySidebarHidden">
            <template v-if="state.activePage === 'vaults'">
                <div class="sidebar-section">
                    <div class="sidebar-label">{{ t('section_all') }}</div>
                    <div class="nav-list">
                        <a
                            :href="routePath('vaults', 'ALL_VAULTS')"
                            class="nav-item"
                            :class="{ active: activeTab() === 'ALL_VAULTS' }"
                            data-action="set-tab"
                            data-tab="ALL_VAULTS"
                        >
                            <span class="nav-item-main"><span>{{ t('all_vaults') }}</span></span>
                        </a>
                    </div>
                </div>
                <div class="sidebar-section">
                    <div class="sidebar-label">{{ t('section_local_vaults') }}</div>
                    <div class="nav-list">
                        <a
                            v-for="vault in filteredVaults()"
                            :key="vault.vault_runtime_hash"
                            :href="'/vaults/local/' + encodeURIComponent(vault.vault_runtime_hash)"
                            class="nav-item"
                            :class="{ active: state.selectedVault && vault.vault_runtime_hash === state.selectedVault.vault_runtime_hash }"
                            data-action="select-vault"
                            :data-key="vault.vault_runtime_hash"
                        >
                            <span class="nav-item-main">
                                <span>{{ vault.display_name || vault.vault_name || vault.vault_runtime_hash }}</span>
                            </span>
                            <span class="status-pill" :class="statusClass(vault.status || 'active')">{{ vault.status || 'active' }}</span>
                        </a>
                        <div v-if="!filteredVaults().length" class="empty">{{ t('no_vaults') }}</div>
                    </div>
                </div>
            </template>
            <template v-else-if="state.activePage === 'functions'">
                <div class="sidebar-section">
                    <div class="sidebar-label">{{ t('section_functions') }}</div>
                    <div class="nav-list">
                        <a
                            v-for="fn in filteredFunctions()"
                            :key="fn.name"
                            :href="routePath('functions', activeTab())"
                            class="nav-item"
                            :class="{ active: state.selectedFunction && fn.name === state.selectedFunction.name }"
                            data-action="select-function"
                            :data-key="fn.name"
                        >
                            <span class="nav-item-main">
                                <span>{{ fn.name }}</span>
                            </span>
                            <span class="status-pill" :class="statusClass(fn.status || 'active')">{{ fn.status || 'active' }}</span>
                        </a>
                        <div v-if="!filteredFunctions().length" class="empty">{{ t('no_functions') }}</div>
                    </div>
                </div>
            </template>
            <template v-else-if="state.activePage === 'audit'">
                <div class="sidebar-section">
                    <div class="sidebar-label">{{ t('section_audit_overview') || 'Audit Overview' }}</div>
                    <div class="nav-list">
                        <div class="sidebar-stat-row">
                            <span class="muted">{{ t('total_events') || 'Total events' }}</span>
                            <span class="status-pill count-pill">{{ auditTotalCount() }}</span>
                        </div>
                        <div class="sidebar-stat-row">
                            <span class="muted">{{ t('tracked_refs') || 'Tracked refs' }}</span>
                            <span class="status-pill count-pill">{{ state.trackedRefAudit && state.trackedRefAudit.counts ? state.trackedRefAudit.counts.total_refs : 0 }}</span>
                        </div>
                        <div v-if="state.trackedRefAudit && state.trackedRefAudit.counts && state.trackedRefAudit.counts.stale > 0" class="sidebar-stat-row">
                            <span class="muted">{{ t('stale_refs') || 'Stale refs' }}</span>
                            <span class="status-pill" :class="'error'">{{ state.trackedRefAudit.counts.stale }}</span>
                        </div>
                    </div>
                </div>
                <div class="sidebar-section">
                    <div class="sidebar-label">{{ t('section_audit_vaults') }}</div>
                    <div class="nav-list">
                        <a
                            v-for="vault in filteredVaults()"
                            :key="vault.vault_runtime_hash"
                            :href="'/audit/' + encodeURIComponent(vault.vault_runtime_hash)"
                            class="nav-item"
                            :class="{ active: state.auditVault === vault.vault_runtime_hash }"
                            data-action="audit-page-select-vault"
                            :data-key="vault.vault_runtime_hash"
                        >
                            <span class="nav-item-main">
                                <span>{{ vault.display_name || vault.vault_name || vault.vault_runtime_hash }}</span>
                            </span>
                            <span class="status-pill count-pill">{{ auditVaultCount(vault) }}</span>
                        </a>
                        <div v-if="!filteredVaults().length" class="empty">{{ t('no_vaults') }}</div>
                    </div>
                </div>
            </template>
            <template v-else-if="state.activePage === 'settings'">
                <div class="sidebar-section">
                    <div class="sidebar-label">{{ t('section_settings') }}</div>
                    <div class="nav-list">
                        <a
                            v-for="tabName in pageConfig.settings.tabs"
                            :key="tabName"
                            :href="routePath('settings', tabName)"
                            class="nav-item"
                            :class="{ active: activeTab() === tabName }"
                            data-action="set-tab"
                            :data-tab="tabName"
                        >
                            <span class="nav-item-main">
                                <span>{{ t('tab_' + tabName.toLowerCase()) }}</span>
                            </span>
                        </a>
                    </div>
                </div>
            </template>
            <template v-else>
                <div v-html="state.ui.secondarySidebarHTML"></div>
            </template>
        </aside>
        <section class="workspace">
            <header class="workspace-header" id="workspace-header" v-html="state.ui.headerHTML"></header>
            <div class="workspace-body" :class="{ 'is-two-pane': state.activePage === 'vaults' ? true : state.ui.twoPane }">
                <template v-if="state.activePage === 'vaults'">
                    <section class="pane" id="center-pane">
                        <div class="pane-header">
                            <div class="pane-title"><strong>{{ vaultCenterTitle() }}</strong></div>
                            <template v-if="activeTab() === 'ALL_VAULTS'">
                                <div class="toolbar">
                                    <div class="toolbar-group">
                                        <input class="field context-search" id="vault-search" type="search" :placeholder="t('search_vaults')" :value="state.globalQuery">
                                    </div>
                                    <span class="pill">{{ allVaultRows().length }} {{ t('count_rows') }}</span>
                                    <button class="btn btn-soft" data-action="refresh-vaults">{{ t('refresh') }}</button>
                                </div>
                            </template>
                            <template v-else>
                                <div class="toolbar">
                                    <div class="toolbar-group">
                                        <div class="toolbar-group">
                                            <span class="segmented-label">{{ t('toolbar_work') }}</span>
                                            <div class="segmented" role="tablist" :aria-label="t('toolbar_work')">
                                                <button class="btn" :class="activeTab() === 'VAULT_ITEMS' ? 'btn-primary' : 'btn-soft'" data-action="set-tab" data-tab="VAULT_ITEMS">{{ t('tab_vault_items') }}</button>
                                                <button class="btn" :class="activeTab() === 'BULK_APPLY' ? 'btn-primary' : 'btn-soft'" data-action="set-tab" data-tab="BULK_APPLY">{{ t('tab_bulk_apply') }}</button>
                                            </div>
                                        </div>
                                        <template v-if="activeTab() === 'BULK_APPLY'">
                                            <div class="toolbar-group">
                                                <span class="segmented-label">{{ t('toolbar_view') }}</span>
                                                <div class="segmented" role="tablist" :aria-label="t('toolbar_view')">
                                                    <button class="btn" :class="state.bulkApplyView === 'items' ? 'btn-primary' : 'btn-soft'" data-action="set-bulk-apply-view" data-view="items">{{ t('view_items') }}</button>
                                                    <button class="btn" :class="state.bulkApplyView === 'workflow' ? 'btn-primary' : 'btn-soft'" data-action="set-bulk-apply-view" data-view="workflow">{{ t('view_workflows') }}</button>
                                                </div>
                                            </div>
                                        </template>
                                        <div class="toolbar-group">
                                            <span class="segmented-label">{{ t('toolbar_scope') }}</span>
                                            <div class="segmented" role="tablist" :aria-label="t('toolbar_scope')">
                                                <button class="btn" :class="state.vaultItemKind === 'ALL' ? 'btn-primary' : 'btn-soft'" data-action="set-vault-kind" data-kind="ALL" :aria-pressed="state.vaultItemKind === 'ALL' ? 'true' : 'false'">{{ t('filter_all') }}</button>
                                                <button class="btn" :class="state.vaultItemKind === 'VE' ? 'btn-primary' : 'btn-soft'" data-action="set-vault-kind" data-kind="VE" :aria-pressed="state.vaultItemKind === 'VE' ? 'true' : 'false'">{{ t('filter_configs') }}</button>
                                                <button class="btn" :class="state.vaultItemKind === 'VK' ? 'btn-primary' : 'btn-soft'" data-action="set-vault-kind" data-kind="VK" :aria-pressed="state.vaultItemKind === 'VK' ? 'true' : 'false'">{{ t('filter_keys') }}</button>
                                            </div>
                                        </div>
                                        <input class="field context-search" id="key-search" type="search" :placeholder="t('search_current_vault')" :value="state.globalQuery">
                                    </div>
                                    <span class="pill">{{ activeTab() === 'BULK_APPLY' ? (state.bulkApplyView === 'workflow' ? state.bulkApplyWorkflows.length : state.bulkApplyTemplates.length) : vaultVisibleRows().length }} {{ t('count_items') }}</span>
                                    <button v-if="activeTab() !== 'BULK_APPLY'" class="btn btn-primary" data-action="new-key">{{ state.vaultItemKind === 'VE' ? t('new_config') : t('new_key') }}</button>
                                </div>
                            </template>
                        </div>
                        <div class="pane-content">
                            <div v-if="activeTab() === 'ALL_VAULTS'" class="table-wrap">
                                <table>
                                    <thead>
                                        <tr>
                                            <th>{{ t('table_vault_name') }}</th>
                                            <th>{{ t('table_identifier') }}</th>
                                            <th>{{ t('table_path') }}</th>
                                            <th>IP</th>
                                            <th>{{ t('table_status') }}</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        <tr
                                            v-for="row in allVaultRows()"
                                            :key="row.vault_runtime_hash"
                                            class="is-clickable"
                                            :class="{ 'is-selected': state.selectedVault && row.vault_runtime_hash === state.selectedVault.vault_runtime_hash }"
                                            data-action="select-vault"
                                            :data-key="row.vault_runtime_hash"
                                        >
                                            <td>{{ row.display_name || row.vault_name }}</td>
                                            <td><span class="code">{{ row.vault_id || row.vault_runtime_hash }}</span></td>
                                            <td>{{ ((row.managed_paths && row.managed_paths[0]) || '-') }}</td>
                                            <td>{{ row.ip || '-' }}</td>
                                            <td><span class="status-pill" :class="statusClass(row.status || 'active')">{{ row.status || 'active' }}</span></td>
                                        </tr>
                                    </tbody>
                                </table>
                            </div>
                            <div v-else-if="activeTab() !== 'BULK_APPLY'" class="table-wrap">
                                <table>
                                    <thead>
                                        <tr>
                                            <th>{{ t('table_kind') }}</th>
                                            <th>{{ t('table_name') }}</th>
                                            <th>{{ t('table_value') }}</th>
                                            <th>{{ t('table_sync') }}</th>
                                            <th>{{ t('table_key_class') }}</th>
                                            <th>{{ t('table_distribution') }}</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        <tr
                                            v-for="row in vaultVisibleRows()"
                                            :key="row.item_kind + ':' + row.name"
                                            class="is-clickable"
                                            :class="{ 'is-selected': row.name === currentVaultSelectedName() && row.item_kind === currentVaultSelectedKind() }"
                                            data-action="select-vault-item"
                                            :data-kind="row.item_kind"
                                            :data-key="row.name"
                                        >
                                            <td><span class="pill" :class="row.item_kind === 'VE' ? 'kind-ve' : 'kind-vk'">{{ vaultKindLabel(row.item_kind) }}</span></td>
                                            <td>{{ row.name }}</td>
                                            <td><span class="code">{{ vaultItemIdentifier(row) }}</span></td>
                                            <td>
                                                <span
                                                    v-if="vaultSyncStatus(row.item_kind, row.name).loading"
                                                    class="muted"
                                                >{{ t('sync_checking') }}</span>
                                                <span
                                                    v-else
                                                    class="status-pill"
                                                    :class="vaultSyncStatus(row.item_kind, row.name).className"
                                                >{{ vaultSyncStatus(row.item_kind, row.name).label }}</span>
                                            </td>
                                            <td>
                                                <span
                                                    v-if="vaultKeyClassStatus(row.item_kind, row.name).loading"
                                                    class="muted"
                                                >{{ t('sync_checking') }}</span>
                                                <span
                                                    v-else
                                                    class="status-pill"
                                                    :class="vaultKeyClassStatus(row.item_kind, row.name).className"
                                                >{{ vaultKeyClassStatus(row.item_kind, row.name).label }}</span>
                                            </td>
                                            <td>
                                                <span
                                                    v-if="vaultDistributionStatus(row.item_kind, row.name).loading"
                                                    class="muted"
                                                >{{ t('sync_checking') }}</span>
                                                <span
                                                    v-else
                                                    class="status-pill"
                                                    :class="vaultDistributionStatus(row.item_kind, row.name).className"
                                                >{{ vaultDistributionStatus(row.item_kind, row.name).label }}</span>
                                            </td>
                                        </tr>
                                        <tr v-if="!vaultVisibleRows().length">
                                            <td :colspan="6"><div class="empty">{{ t('empty_no_rows') }}</div></td>
                                        </tr>
                                    </tbody>
                                </table>
                            </div>
                            <div v-else class="table-wrap">
                                <table v-if="state.bulkApplyView === 'items'">
                                    <thead>
                                        <tr>
                                            <th>{{ t('table_name_generic') }}</th>
                                            <th>{{ t('table_type') }}</th>
                                            <th>{{ t('table_target_path') }}</th>
                                            <th>{{ t('table_definition_status') }}</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        <tr
                                            v-for="row in state.bulkApplyTemplates"
                                            :key="row.name"
                                            class="is-clickable"
                                            :class="{ 'is-selected': state.selectedBulkApplyTemplateName === row.name }"
                                            data-action="select-bulk-template"
                                            :data-key="row.name"
                                        >
                                            <td>{{ row.name }}</td>
                                            <td>{{ row.format || '-' }}</td>
                                            <td><span class="code">{{ row.target_path || '-' }}</span></td>
                                            <td><span class="status-pill" :class="statusClass(row.validation_status || 'active')">{{ row.validation_status || '-' }}</span></td>
                                        </tr>
                                        <tr v-if="!state.bulkApplyTemplates.length">
                                            <td colspan="4"><div class="empty">{{ t('empty_no_bulk_items') }}</div></td>
                                        </tr>
                                    </tbody>
                                </table>
                                <table v-else>
                                    <thead>
                                        <tr>
                                            <th>{{ t('table_name_generic') }}</th>
                                            <th>{{ t('table_label') }}</th>
                                            <th>{{ t('table_steps') }}</th>
                                            <th>{{ t('table_definition_status') }}</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        <tr
                                            v-for="row in state.bulkApplyWorkflows"
                                            :key="row.name"
                                            class="is-clickable"
                                            :class="{ 'is-selected': state.selectedBulkApplyWorkflowName === row.name }"
                                            data-action="select-bulk-workflow"
                                            :data-key="row.name"
                                        >
                                            <td>{{ row.name }}</td>
                                            <td>{{ row.label || '-' }}</td>
                                            <td>{{ row.step_count || ((row.steps && row.steps.length) || 0) }}</td>
                                            <td><span class="status-pill" :class="statusClass(row.validation_status || 'active')">{{ row.validation_status || '-' }}</span></td>
                                        </tr>
                                        <tr v-if="!state.bulkApplyWorkflows.length">
                                            <td colspan="4"><div class="empty">{{ t('empty_no_workflows') }}</div></td>
                                        </tr>
                                    </tbody>
                                </table>
                            </div>
                        </div>
                    </section>
                    <section class="pane" id="right-pane">
                        <div class="pane-header">
                            <div class="pane-title"><strong>{{ vaultRightPaneTitle() }}</strong></div>
                        </div>
                        <div class="pane-content">
                            <template v-if="activeTab() === 'ALL_VAULTS'">
                                <div v-if="selectedInventoryDetail()" class="stack">
                                    <div class="card">
                                        <div class="card-title">{{ t('selected_vault') }}</div>
                                        <div class="inline-grid">
                                            <div class="kv"><span class="label">{{ t('name') }}</span><span class="value">{{ selectedInventoryDetail().display_name || selectedInventoryDetail().vault_name || '-' }}</span></div>
                                            <div class="kv"><span class="label">{{ t('table_identifier') }}</span><span class="value">{{ selectedInventoryDetail().vault_id || selectedInventoryDetail().vault_runtime_hash || '-' }}</span></div>
                                            <div class="kv"><span class="label">{{ t('table_path') }}</span><span class="value">{{ ((selectedInventoryDetail().managed_paths && selectedInventoryDetail().managed_paths[0]) || '-') }}</span></div>
                                            <div class="kv"><span class="label">IP</span><span class="value">{{ selectedInventoryDetail().ip || '-' }}</span></div>
                                            <div class="kv"><span class="label">{{ t('table_status') }}</span><span class="value">{{ selectedInventoryDetail().status || '-' }}</span></div>
                                        </div>
                                    </div>
                                    <form class="stack" data-form="save-vault-meta">
                                        <div class="card">
                                            <div class="card-title">{{ t('summary') }}</div>
                                            <div class="stack">
                                                <div class="kv"><span class="label">{{ t('table_vault_name') }}</span><input class="field" name="display_name" :value="selectedInventoryDetail().display_name || ''"></div>
                                                <div class="kv"><span class="label">{{ t('description') }}</span><textarea class="textarea" name="description">{{ selectedInventoryDetail().description || '' }}</textarea></div>
                                                <div class="kv"><span class="label">{{ t('tags_json') }}</span><textarea class="textarea" name="tags_json">{{ selectedInventoryDetail().tags_json || '[]' }}</textarea></div>
                                            </div>
                                        </div>
                                        <div class="toolbar">
                                            <button class="btn btn-primary" type="submit">{{ t('save') }}</button>
                                        </div>
                                    </form>
                                </div>
                                <div v-else class="empty">{{ t('selected_vault_prompt') }}</div>
                            </template>
                            <template v-else-if="activeTab() !== 'BULK_APPLY'">
                                <div class="stack">
                                    <form v-if="vaultPanel().canMoveItem" class="stack" :data-form="vaultPanel().isConfigItem ? 'promote-config' : 'promote-key'">
                                        <div class="card">
                                            <div class="card-title">{{ vaultPanel().isConfigItem ? t('move_config') : t('move_key') }}</div>
                                            <div class="stack">
                                                <div class="kv">
                                                    <span class="label">{{ t('target_vault') }}</span>
                                                    <select class="select" name="target_vault">
                                                        <option v-for="option in vaultTargetOptions(true)" :key="option.value" :value="option.value" :selected="option.value === vaultPanel().targetVaultDefault">{{ option.label }}</option>
                                                    </select>
                                                </div>
                                            </div>
                                            <div class="muted">{{ vaultPanel().moveHelperText }}</div>
                                        </div>
                                        <button class="btn btn-soft" type="submit" :disabled="!vaultPanel().detailName">{{ vaultPanel().isConfigItem ? t('move_config') : t('move_key') }}</button>
                                    </form>
                                    <form class="stack" :data-form="vaultPanel().saveForm">
                                        <div class="card">
                                            <div class="card-title">{{ vaultPanel().detailName ? t('selected_item') : vaultPanel().createTitle }}</div>
                                            <div class="stack">
                                                <div class="kv">
                                                    <span class="label">{{ vaultPanel().isConfigItem ? t('config_name') : t('key_name') }}</span>
                                                    <input class="field" :name="vaultPanel().isConfigItem ? 'key' : 'name'" :value="vaultPanel().detailName || ''" :readonly="vaultPanel().detailName ? true : null">
                                                </div>
                                                <div class="kv">
                                                    <span class="label">{{ vaultPanel().isConfigItem ? t('config_value') : t('key_value') }}</span>
                                                    <div class="row" style="align-items:center;">
                                                        <button class="btn btn-soft" type="button" data-action="toggle-reveal">{{ state.revealValue ? t('hide') : t('reveal') }}</button>
                                                        <button v-if="state.revealValue && vaultPanel().visibleValue" class="btn btn-soft" type="button" data-action="copy-value">{{ t('copy') }}</button>
                                                    </div>
                                                    <textarea class="textarea" name="value" :placeholder="vaultPanel().detailName ? t('overwrite_placeholder') : t('required')">{{ state.revealValue ? (vaultPanel().visibleValue || '') : '••••••••••••' }}</textarea>
                                                </div>
                                                <template v-if="vaultPanel().showScopeSelect">
                                                    <div class="kv">
                                                        <span class="label">{{ t('table_scope') }}</span>
                                                        <select class="select" name="scope">
                                                            <option v-for="option in vaultPanel().scopeOptions" :key="option" :value="option" :selected="option === vaultPanel().currentScope">{{ option }}</option>
                                                        </select>
                                                    </div>
                                                </template>
                                                <template v-else>
                                                    <input type="hidden" name="scope" :value="vaultPanel().currentScope">
                                                    <input v-if="vaultPanel().isConfigItem" type="hidden" name="status" :value="vaultPanel().configStatus">
                                                </template>
                                                <template v-if="vaultPanel().showKeyMeta">
                                                    <div class="kv"><span class="label">{{ t('description') }}</span><textarea class="textarea" name="description">{{ vaultPanel().description }}</textarea></div>
                                                    <div class="kv"><span class="label">{{ t('tags_json') }}</span><textarea class="textarea" name="tags_json">{{ vaultPanel().tagsJSON }}</textarea></div>
                                                </template>
                                            </div>
                                        </div>
                                        <div class="toolbar">
                                            <button class="btn btn-primary" type="submit">{{ vaultPanel().detailName ? t('save') : t('create') }}</button>
                                            <button v-if="vaultPanel().showDelete" class="btn btn-danger" type="button" :data-action="vaultPanel().deleteAction">{{ t('delete') }}</button>
                                        </div>
                                    </form>
                                    <template v-if="vaultPanel().isConfigItem">
                                        <details class="card" open>
                                            <summary class="card-title">{{ t('local_external_relations') }}</summary>
                                            <div v-if="!configRelationsByScope().length" class="empty">{{ t('no_relation_info') }}</div>
                                            <div v-else class="stack">
                                                <div v-for="section in configRelationsByScope()" :key="section.scope" class="stack">
                                                    <div class="card-title">{{ section.scope }}</div>
                                                    <div v-for="item in section.rows" :key="section.scope + ':' + (item.vault_runtime_hash || item.vault_name) + ':' + (item.value || '')" class="value">
                                                        {{ item.vault_name || item.vault_runtime_hash || '-' }} · {{ item.value || '-' }}
                                                    </div>
                                                </div>
                                            </div>
                                        </details>
                                    </template>
                                    <template v-else>
                                        <details v-if="vaultPanel().showKeyMeta" class="card">
                                            <summary class="card-title">{{ t('additional_info') }}</summary>
                                            <div class="inline-grid">
                                                <div class="kv"><span class="label">{{ t('bindings_count') }}</span><span class="value">{{ vaultPanel().bindingsTotal }}</span></div>
                                                <div class="kv"><span class="label">{{ t('usage_count') }}</span><span class="value">{{ vaultPanel().usageCount }}</span></div>
                                                <div class="kv"><span class="label">{{ t('table_scope') }}</span><span class="value">{{ vaultPanel().currentScope }}</span></div>
                                                <div class="kv"><span class="label">{{ t('table_status') }}</span><span class="value">{{ vaultPanel().status }}</span></div>
                                            </div>
                                            <div v-if="vaultPanel().bindings.length" class="stack" style="margin-top:12px;">
                                                <div v-for="item in vaultPanel().bindings" :key="item.binding_type + ':' + item.target_name + ':' + item.field_key" class="value">{{ item.binding_type }} / {{ item.target_name }} / {{ item.field_key || '-' }}</div>
                                            </div>
                                            <div v-else class="empty">{{ t('no_binding_info') }}</div>
                                            <div v-if="vaultPanel().recentAudit.length" class="stack" style="margin-top:12px;">
                                                <div v-for="item in vaultPanel().recentAudit" :key="(item.action || item.event_type) + ':' + (item.created_at || item.timestamp)" class="value">{{ item.action || item.event_type || 'event' }} · {{ item.created_at || item.timestamp || '-' }}</div>
                                            </div>
                                            <div v-else class="empty">{{ t('no_audit_info') }}</div>
                                        </details>
                                        <div v-else class="card">
                                            <div class="card-title">{{ t('additional_info') }}</div>
                                            <div class="inline-grid">
                                                <div class="kv"><span class="label">{{ t('table_identifier') }}</span><span class="value">{{ vaultPanel().itemRefValue || '-' }}</span></div>
                                                <div class="kv"><span class="label">{{ t('table_scope') }}</span><span class="value">{{ vaultPanel().currentScope }}</span></div>
                                                <div class="kv"><span class="label">{{ t('table_status') }}</span><span class="value">{{ vaultPanel().status }}</span></div>
                                                <div class="kv"><span class="label">{{ t('storage') }}</span><span class="value">Local Vault</span></div>
                                            </div>
                                        </div>
                                    </template>
                                </div>
                            </template>
                            <template v-else>
                                <div v-if="state.bulkApplyView === 'items' && selectedBulkApplyTemplate()" class="stack">
                                    <div class="card">
                                        <div class="card-title">{{ t('selected_item') }}</div>
                                        <div class="inline-grid">
                                            <div class="kv"><span class="label">{{ t('table_name_generic') }}</span><span class="value">{{ selectedBulkApplyTemplate().name || '-' }}</span></div>
                                            <div class="kv"><span class="label">{{ t('table_type') }}</span><span class="value">{{ selectedBulkApplyTemplate().format || '-' }}</span></div>
                                            <div class="kv"><span class="label">{{ t('table_target_path') }}</span><span class="value">{{ selectedBulkApplyTemplate().target_path || '-' }}</span></div>
                                            <div class="kv"><span class="label">{{ t('hook') }}</span><span class="value">{{ selectedBulkApplyTemplate().hook || '-' }}</span></div>
                                            <div class="kv"><span class="label">{{ t('table_definition_status') }}</span><span class="value">{{ selectedBulkApplyTemplate().validation_status || '-' }}</span></div>
                                        </div>
                                    </div>
                                    <div class="card">
                                        <div class="card-title">{{ t('template_body') }}</div>
                                        <pre class="code">{{ selectedBulkApplyTemplate().body || '' }}</pre>
                                    </div>
                                </div>
                                <div v-else-if="state.bulkApplyView === 'workflow' && selectedBulkApplyWorkflow()" class="stack">
                                    <div class="card">
                                        <div class="card-title">{{ t('workflow_detail') }}</div>
                                        <div class="inline-grid">
                                            <div class="kv"><span class="label">{{ t('table_name_generic') }}</span><span class="value">{{ selectedBulkApplyWorkflow().name || '-' }}</span></div>
                                            <div class="kv"><span class="label">{{ t('table_label') }}</span><span class="value">{{ selectedBulkApplyWorkflow().label || '-' }}</span></div>
                                            <div class="kv"><span class="label">{{ t('hook') }}</span><span class="value">{{ selectedBulkApplyWorkflow().hook || '-' }}</span></div>
                                            <div class="kv"><span class="label">{{ t('table_steps') }}</span><span class="value">{{ selectedBulkApplyWorkflow().step_count || ((selectedBulkApplyWorkflow().steps && selectedBulkApplyWorkflow().steps.length) || 0) }}</span></div>
                                            <div class="kv"><span class="label">{{ t('table_definition_status') }}</span><span class="value">{{ selectedBulkApplyWorkflow().validation_status || '-' }}</span></div>
                                        </div>
                                    </div>
                                    <div class="card">
                                        <div class="card-title">{{ t('steps') }}</div>
                                        <div v-if="selectedBulkApplyWorkflow().steps && selectedBulkApplyWorkflow().steps.length" class="stack">
                                            <div v-for="step in selectedBulkApplyWorkflow().steps" :key="step.name" class="value">
                                                {{ step.name }} · {{ step.format || '-' }} · {{ step.target_path || '-' }}
                                            </div>
                                        </div>
                                        <div v-else class="empty">{{ t('no_steps_defined') }}</div>
                                    </div>
                                </div>
                                <div v-else class="empty">{{ t('select_bulk_target_prompt') }}</div>
                            </template>
                        </div>
                    </section>
                </template>
                <template v-else>
                    <template v-if="state.activePage === 'functions'">
                        <section class="pane" id="center-pane">
                            <div class="pane-header">
                                <div class="pane-title"><strong>{{ functionCenterTitle() }}</strong></div>
                                <div class="toolbar">
                                    <div class="toolbar-group">
                                        <input class="field context-search" id="function-search" type="search" :placeholder="t('section_functions')" :value="state.globalQuery">
                                    </div>
                                    <span class="pill">{{ filteredFunctions().length }} {{ t('count_rows') }}</span>
                                    <button v-if="activeTab() === 'FUNCTION_LIST'" class="btn btn-soft" data-action="refresh-functions">{{ t('refresh') }}</button>
                                    <button v-if="activeTab() === 'FUNCTION_RUN'" class="btn btn-sm" type="submit" form="function-run-form">{{ t('tab_function_run') }}</button>
                                </div>
                            </div>
                            <div class="pane-content">
                                <template v-if="activeTab() === 'FUNCTION_LIST'">
                                    <div class="table-wrap">
                                        <table>
                                            <thead>
                                                <tr>
                                                    <th>{{ t('function_list') }}</th>
                                                    <th>{{ t('category') }}</th>
                                                    <th>{{ t('hash') }}</th>
                                                    <th>Updated</th>
                                                </tr>
                                            </thead>
                                            <tbody>
                                                <tr
                                                    v-for="fn in filteredFunctions()"
                                                    :key="fn.name"
                                                    class="is-clickable"
                                                    :class="{ 'is-selected': state.selectedFunction && fn.name === state.selectedFunction.name }"
                                                    data-action="select-function"
                                                    :data-key="fn.name"
                                                >
                                                    <td>{{ fn.name }}</td>
                                                    <td>{{ fn.category || '-' }}</td>
                                                    <td><span class="code">{{ fn.function_hash || '-' }}</span></td>
                                                    <td>{{ fn.updated_at || '-' }}</td>
                                                </tr>
                                                <tr v-if="!filteredFunctions().length">
                                                    <td colspan="4"><div class="empty">{{ t('no_functions') }}</div></td>
                                                </tr>
                                            </tbody>
                                        </table>
                                    </div>
                                </template>
                                <template v-else-if="activeTab() === 'FUNCTION_BINDINGS'">
                                    <div class="table-wrap">
                                        <table>
                                            <thead>
                                                <tr>
                                                    <th>{{ t('ref') }}</th>
                                                    <th>{{ t('page_vaults') }}</th>
                                                    <th>{{ t('field') }}</th>
                                                    <th>{{ t('required') }}</th>
                                                </tr>
                                            </thead>
                                            <tbody>
                                                <tr v-for="row in state.functionBindings" :key="(row.binding_id || row.ref_canonical) + ':' + (row.field_key || '')">
                                                    <td><span class="code">{{ row.ref_canonical || '-' }}</span></td>
                                                    <td>{{ row.vault_hash || '-' }}</td>
                                                    <td>{{ row.field_key || '-' }}</td>
                                                    <td>{{ String(row.required) }}</td>
                                                </tr>
                                                <tr v-if="!state.functionBindings.length">
                                                    <td colspan="4"><div class="empty">{{ t('empty_no_rows') }}</div></td>
                                                </tr>
                                            </tbody>
                                        </table>
                                    </div>
                                </template>
                                <template v-else-if="activeTab() === 'FUNCTION_IMPACT'">
                                    <div v-if="state.functionSummary" class="metrics">
                                        <div class="metric"><span class="label">{{ t('bindings') }}</span><strong>{{ state.functionSummary.bindings_total || 0 }}</strong></div>
                                        <div class="metric"><span class="label">{{ t('unique_refs') }}</span><strong>{{ state.functionSummary.unique_refs_count || 0 }}</strong></div>
                                        <div class="metric"><span class="label">{{ t('vaults_metric') }}</span><strong>{{ state.functionSummary.vaults_count || 0 }}</strong></div>
                                    </div>
                                    <div class="table-wrap">
                                        <table>
                                            <thead>
                                                <tr>
                                                    <th>{{ t('ref') }}</th>
                                                    <th>{{ t('secret') }}</th>
                                                    <th>{{ t('field') }}</th>
                                                    <th>{{ t('page_vaults') }}</th>
                                                </tr>
                                            </thead>
                                            <tbody>
                                                <tr v-for="row in functionImpactRefs()" :key="(row.ref_canonical || '-') + ':' + (row.field_key || '') + ':' + (row.vault_hash || '-')">
                                                    <td><span class="code">{{ row.ref_canonical || '-' }}</span></td>
                                                    <td>{{ row.secret_name || '-' }}</td>
                                                    <td>{{ row.field_key || '-' }}</td>
                                                    <td>{{ row.vault_hash || '-' }}</td>
                                                </tr>
                                                <tr v-if="!functionImpactRefs().length">
                                                    <td colspan="4"><div class="empty">{{ t('empty_no_rows') }}</div></td>
                                                </tr>
                                            </tbody>
                                        </table>
                                    </div>
                                </template>
                                <template v-else>
                                    <div v-if="state.selectedFunction" class="stack">
                                        <form class="stack" id="function-run-form" data-form="run-function">
                                            <div class="form-section">
                                                <h4>{{ t('function_run') }}</h4>
                                                <div class="form-row">
                                                    <label>{{ t('parameters_json') }}</label>
                                                    <textarea name="prompt" rows="4" placeholder="{&quot;key&quot;: &quot;value&quot;}"></textarea>
                                                </div>
                                                <input type="hidden" name="system_prompt" value="">
                                                <input type="hidden" name="temperature" value="0.2">
                                                <input type="hidden" name="max_output_tokens" value="2048">
                                                <input type="hidden" name="timeout_seconds" value="120">
                                            </div>
                                        </form>
                                        <div v-if="state.functionRunResult" class="card">
                                            <div class="card-title">{{ t('summary') }}</div>
                                            <pre class="code">{{ prettyJSON(state.functionRunResult) }}</pre>
                                        </div>
                                    </div>
                                    <div v-else class="empty">{{ t('no_functions') }}</div>
                                </template>
                            </div>
                        </section>
                        <section class="pane" id="right-pane">
                            <div class="pane-header">
                                <div class="pane-title"><strong>{{ functionRightPaneTitle() }}</strong></div>
                            </div>
                            <div class="pane-content">
                                <template v-if="activeTab() === 'FUNCTION_LIST'">
                                    <div v-if="state.functionDetail" class="stack">
                                        <div class="card">
                                            <div class="card-title">{{ t('selected_function') }}</div>
                                            <div class="inline-grid">
                                                <div class="kv"><span class="label">{{ t('function_list') }}</span><span class="value">{{ state.functionDetail.name || '-' }}</span></div>
                                                <div class="kv"><span class="label">{{ t('category') }}</span><span class="value">{{ state.functionDetail.category || '-' }}</span></div>
                                                <div class="kv"><span class="label">{{ t('hash') }}</span><span class="value">{{ state.functionDetail.function_hash || '-' }}</span></div>
                                                <div class="kv"><span class="label">{{ t('updated') }}</span><span class="value">{{ state.functionDetail.updated_at || '-' }}</span></div>
                                            </div>
                                        </div>
                                        <div class="card">
                                            <div class="card-title">{{ t('command') }}</div>
                                            <pre class="code">{{ state.functionDetail.command || '' }}</pre>
                                        </div>
                                    </div>
                                    <div v-else class="empty">{{ t('no_function_summary') }}</div>
                                </template>
                                <template v-else-if="activeTab() === 'FUNCTION_BINDINGS'">
                                    <div v-if="state.selectedFunction" class="stack">
                                        <form class="stack" data-form="replace-function-bindings">
                                            <div class="card">
                                                <div class="card-title">{{ t('bindings_json') }}</div>
                                                <textarea class="textarea" name="bindings_json">{{ prettyJSON(functionBindingsPayload()) }}</textarea>
                                            </div>
                                            <div class="toolbar">
                                                <button class="btn btn-primary" type="submit">{{ t('replace') }}</button>
                                                <button class="btn btn-danger" type="button" data-action="delete-function-bindings">{{ t('delete_all') }}</button>
                                            </div>
                                        </form>
                                    </div>
                                    <div v-else class="empty">{{ t('no_function_bindings_info') }}</div>
                                </template>
                                <template v-else-if="activeTab() === 'FUNCTION_IMPACT'">
                                    <div v-if="state.functionSummary" class="card">
                                        <div class="card-title">{{ t('summary') }}</div>
                                        <div class="inline-grid">
                                            <div class="kv"><span class="label">{{ t('binding_type') }}</span><span class="value">{{ state.functionSummary.binding_type || '-' }}</span></div>
                                            <div class="kv"><span class="label">{{ t('target_name') }}</span><span class="value">{{ state.functionSummary.target_name || '-' }}</span></div>
                                            <div class="kv"><span class="label">{{ t('bindings') }}</span><span class="value">{{ state.functionSummary.bindings_total || 0 }}</span></div>
                                            <div class="kv"><span class="label">{{ t('unique_refs') }}</span><span class="value">{{ state.functionSummary.unique_refs_count || 0 }}</span></div>
                                            <div class="kv"><span class="label">{{ t('vaults_metric') }}</span><span class="value">{{ state.functionSummary.vaults_count || 0 }}</span></div>
                                        </div>
                                    </div>
                                    <div v-else class="empty">{{ t('no_functions') }}</div>
                                </template>
                                <template v-else>
                                    <div v-if="state.selectedFunction" class="card">
                                        <div class="card-title">{{ t('summary') }}</div>
                                        <div class="inline-grid">
                                            <div class="kv"><span class="label">{{ t('function_list') }}</span><span class="value">{{ state.selectedFunction.name || '-' }}</span></div>
                                            <div class="kv"><span class="label">{{ t('table_status') }}</span><span class="value">{{ state.selectedFunction.status || 'active' }}</span></div>
                                        </div>
                                    </div>
                                    <div v-else class="empty">{{ t('no_functions') }}</div>
                                </template>
                            </div>
                        </section>
                    </template>
                    <template v-else>
                        <template v-if="state.activePage === 'audit'">
                            <section class="pane" id="center-pane">
                                <div class="pane-header">
                                    <div class="pane-title"><strong>{{ t('tab_audit_log') }}</strong></div>
                                    <div class="toolbar">
                                        <span class="pill">{{ state.auditRows.length }} {{ t('count_events') }}</span>
                                    </div>
                                </div>
                                <div class="pane-content">
                                    <div class="table-wrap">
                                        <table>
                                            <thead>
                                                <tr>
                                                    <th>{{ t('time') }}</th>
                                                    <th>{{ t('action') }}</th>
                                                    <th>{{ t('actor') }}</th>
                                                    <th>{{ t('target') }}</th>
                                                </tr>
                                            </thead>
                                            <tbody>
                                                <tr v-for="(row, idx) in state.auditRows" class="is-clickable" :class="{ 'is-selected': state.selectedAuditRow && state.selectedAuditRow.event_id === row.event_id }" data-action="select-audit-row" :data-index="idx" :key="(row.created_at || row.timestamp || '-') + ':' + (row.entity_id || '-') + ':' + (row.action || '-')">
                                                    <td>{{ row.created_at || row.timestamp || '-' }}</td>
                                                    <td>{{ row.action || '-' }}</td>
                                                    <td>{{ row.actor_type || row.actor_id || '-' }}</td>
                                                    <td>{{ row.entity_id || '-' }}</td>
                                                </tr>
                                                <tr v-if="!state.auditRows.length">
                                                    <td colspan="4"><div class="empty">{{ t('empty_no_rows') }}</div></td>
                                                </tr>
                                            </tbody>
                                        </table>
                                    </div>
                                </div>
                            </section>
                            <section class="pane" id="right-pane">
                                <div class="pane-header">
                                    <div class="pane-title"><strong>{{ t('audit_event_detail') }}</strong></div>
                                </div>
                                <div class="pane-content">
                                    <div v-if="state.auditRows.length" class="card">
                                        <div class="card-title">{{ (state.selectedAuditRow || state.auditRows[0]).action || 'event' }} · {{ (state.selectedAuditRow || state.auditRows[0]).entity_type || '-' }}</div>
                                        <pre class="code">{{ prettyJSON(state.selectedAuditRow || state.auditRows[0]) }}</pre>
                                    </div>
                                    <div v-else class="empty">{{ t('select_vault_for_detail') }}</div>
                                </div>
                            </section>
                        </template>
                        <template v-else>
                            <template v-if="state.activePage === 'settings'">
                                <section class="pane" id="center-pane">
                                    <div class="pane-header">
                                        <div class="pane-title"><strong>{{ settingsCenterTitle() }}</strong></div>
                                    </div>
                                    <div class="pane-content">
                                        <template v-if="!state.uiConfig">
                                            <div class="empty">{{ t('loading_ui_config') }}</div>
                                        </template>
                                        <template v-else-if="activeTab() === 'ADMIN'">
                                            <div class="card">
                                                <div class="card-title">{{ t('system_update') }}</div>
                                                <div class="stack">
                                                    <div class="value">{{ t('settings_admin_intro_line1') }}</div>
                                                    <div class="value">{{ t('settings_admin_intro_line2') }}</div>
                                                </div>
                                            </div>
                                            <div class="card" v-if="state.systemUpdate">
                                                <div class="card-title">{{ t('update_status') }}</div>
                                                <div class="inline-grid">
                                                    <div class="kv"><span class="label">{{ t('current_version') }}</span><span class="value">{{ state.systemUpdate.current_version || '-' }}</span></div>
                                                    <div class="kv"><span class="label">{{ t('target_version') }}</span><span class="value">{{ state.systemUpdate.target_version || '-' }}</span></div>
                                                    <div class="kv"><span class="label">{{ t('release_channel') }}</span><span class="value">{{ state.systemUpdate.release_channel || '-' }}</span></div>
                                                    <div class="kv"><span class="label">{{ t('update_available') }}</span><span class="value">{{ state.systemUpdate.update_available ? t('yes') : t('no') }}</span></div>
                                                    <div class="kv"><span class="label">{{ t('update_enabled') }}</span><span class="value">{{ state.systemUpdate.update_enabled ? t('yes') : t('no') }}</span></div>
                                                    <div class="kv"><span class="label">{{ t('last_status') }}</span><span class="value">{{ (state.systemUpdate.state && state.systemUpdate.state.status) || '-' }}</span></div>
                                                </div>
                                            </div>
                                        </template>
                                        <template v-else>
                                            <div class="card">
                                                <div class="card-title">{{ t('ui_config') }}</div>
                                                <div class="inline-grid">
                                                    <div class="kv"><span class="label">{{ t('locale') }}</span><span class="value">{{ state.uiConfig.locale || '-' }}</span></div>
                                                    <div class="kv"><span class="label">{{ t('default_email') }}</span><span class="value">{{ state.uiConfig.default_email || '-' }}</span></div>
                                                </div>
                                            </div>
                                        </template>
                                    </div>
                                </section>
                                <section class="pane" id="right-pane">
                                    <div class="pane-header">
                                        <div class="pane-title"><strong>{{ settingsRightPaneTitle() }}</strong></div>
                                    </div>
                                    <div class="pane-content">
                                        <template v-if="!state.uiConfig">
                                            <div class="empty">{{ t('loading') }}</div>
                                        </template>
                                        <template v-else-if="activeTab() === 'ADMIN'">
                                            <div class="card">
                                                <div class="card-title">{{ t('update_control') }}</div>
                                                <div class="inline-grid">
                                                    <div class="kv"><span class="label">{{ t('configured_script') }}</span><span class="value">{{ (state.systemUpdate && state.systemUpdate.script_path) || t('not_configured') }}</span></div>
                                                    <div class="kv"><span class="label">{{ t('last_error') }}</span><span class="value">{{ (state.systemUpdate && state.systemUpdate.state && state.systemUpdate.state.last_error) || '-' }}</span></div>
                                                </div>
                                            </div>
                                            <form class="stack" data-form="save-admin-update-settings">
                                                <div class="card">
                                                    <div class="card-title">{{ t('update_settings') }}</div>
                                                    <div class="stack">
                                                        <div class="kv"><span class="label">{{ t('target_version') }}</span><input class="field" name="target_version" :value="state.uiConfig.target_version || ''" placeholder="0.2.0"></div>
                                                        <div class="kv">
                                                            <span class="label">{{ t('release_channel') }}</span>
                                                            <select class="select" name="release_channel">
                                                                <option value="stable" :selected="(state.uiConfig.release_channel || 'stable') === 'stable'">stable</option>
                                                                <option value="candidate" :selected="(state.uiConfig.release_channel || 'stable') === 'candidate'">candidate</option>
                                                                <option value="nightly" :selected="(state.uiConfig.release_channel || 'stable') === 'nightly'">nightly</option>
                                                            </select>
                                                        </div>
                                                    </div>
                                                </div>
                                                <div class="inline-actions">
                                                    <button class="btn btn-primary" type="submit">{{ t('save_update_settings') }}</button>
                                                    <button class="btn" type="button" data-action="run-system-update">{{ t('run_update') }}</button>
                                                </div>
                                            </form>
                                        </template>
                                        <template v-else>
                                            <form class="stack" data-form="save-settings">
                                                <div class="card">
                                                    <div class="card-title">{{ t('ui_config') }}</div>
                                                    <div class="stack">
                                                        <div class="kv">
                                                            <span class="label">{{ t('locale') }}</span>
                                                            <select class="select" name="locale">
                                                                <option value="ko" :selected="(state.uiConfig.locale || 'ko') === 'ko'">ko</option>
                                                                <option value="en" :selected="(state.uiConfig.locale || 'ko') === 'en'">en</option>
                                                            </select>
                                                        </div>
                                                        <div class="kv"><span class="label">{{ t('default_email') }}</span><input class="field" name="default_email" :value="state.uiConfig.default_email || ''"></div>
                                                    </div>
                                                </div>
                                                <button class="btn btn-primary" type="submit">{{ t('save_settings') }}</button>
                                            </form>
                                        </template>
                                    </div>
                                </section>
                            </template>
                            <template v-else>
                                <section class="pane" id="left-pane" v-show="state.ui.leftVisible" v-html="state.ui.leftHTML"></section>
                                <section class="pane" id="center-pane" v-html="state.ui.centerHTML"></section>
                                <section class="pane" id="right-pane" v-html="state.ui.rightHTML"></section>
                            </template>
                        </template>
                    </template>
                </template>
            </div>
        </section>
    </div>
</div>
</template>

<style scoped>
.unlock-shell {
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    background: #0f1117;
}
.unlock-card {
    background: #1a1d27;
    border: 1px solid #2a2d3a;
    border-radius: 12px;
    padding: 40px;
    width: 360px;
    display: flex;
    flex-direction: column;
    gap: 16px;
}
.unlock-brand {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 4px;
}
.brand-mark {
    background: #3b82f6;
    color: #fff;
    font-weight: 700;
    font-size: 13px;
    padding: 3px 7px;
    border-radius: 6px;
}
.brand-name { color: #e2e8f0; font-weight: 600; font-size: 15px; }
.unlock-heading { color: #f1f5f9; font-size: 20px; font-weight: 700; margin: 0; }
.unlock-desc { color: #94a3b8; font-size: 14px; margin: 0; }
.unlock-error {
    background: #3b1212;
    border: 1px solid #7f1d1d;
    color: #fca5a5;
    border-radius: 6px;
    padding: 10px 14px;
    font-size: 13px;
}
.unlock-form { display: flex; flex-direction: column; gap: 10px; }
.unlock-input {
    background: #0f1117;
    border: 1px solid #2a2d3a;
    border-radius: 8px;
    color: #f1f5f9;
    font-size: 14px;
    padding: 10px 14px;
    outline: none;
    width: 100%;
    box-sizing: border-box;
}
.unlock-input:focus { border-color: #3b82f6; }
.unlock-btn {
    background: #3b82f6;
    border: none;
    border-radius: 8px;
    color: #fff;
    cursor: pointer;
    font-size: 14px;
    font-weight: 600;
    padding: 10px;
    width: 100%;
}
.unlock-btn:hover { background: #2563eb; }
</style>

<script setup>
import { pageConfig } from './adminConfig';
import { useAdminApp } from './useAdminApp';

const {
  state,
  onGlobalSearchInput,
  routePath,
  activeTab,
  filteredVaults,
  filteredFunctions,
  statusClass,
  allVaultRows,
  vaultCenterTitle,
  vaultRightPaneTitle,
  vaultVisibleRows,
  currentVaultSelectedName,
  currentVaultSelectedKind,
  vaultKindLabel,
  vaultItemIdentifier,
  selectedInventoryDetail,
  vaultPanel,
  vaultTargetOptions,
  functionCenterTitle,
  functionRightPaneTitle,
  functionImpactRefs,
  functionBindingsPayload,
  prettyJSON,
  settingsCenterTitle,
  settingsRightPaneTitle,
  t,
  scopeClass,
  vaultSyncStatus,
  vaultDistributionStatus,
  vaultKeyClassStatus,
  selectedBulkApplyTemplate,
  selectedBulkApplyWorkflow,
  renderConfigRelations,
  configRelationsByScope,
  auditVaultCount,
  auditTotalCount,
  auditSelectedVault,
  unlock,
  encodeURIComponent
} = useAdminApp();
</script>
