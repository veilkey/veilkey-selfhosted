import { reactive, onMounted, onUnmounted } from 'vue';
import { pageConfig, routeEntries, routeByPath } from './adminConfig';
import {
    apiURL,
    escapeHTML,
    formatJSON,
    isSelected,
    renderStatusPill,
    scopeClass,
    statusClass
} from './adminUtils';
import {
    renderKVGrid,
    renderMiniList,
    renderOptions,
    renderTable,
    renderTargetOptions
} from './renderPrimitives';
import { translate } from './i18n';

const GROUPED_KEYS_TAB = 'GROUPED_KEYS';
const GROUPED_CONFIGS_TAB = 'GROUPED_CONFIGS';
const CONFIG_SEARCH_TAB = 'CONFIG_SEARCH';
const CONFIG_BULK_TAB = 'CONFIG_BULK';
const CONFIG_PER_VAULT_TAB = 'CONFIG_PER_VAULT';

export function useAdminApp() {
const state = reactive({
    activePage: 'vaults',
    activeTabByPage: {
        vaults: 'ALL_VAULTS',
        functions: 'FUNCTION_LIST',
        audit: 'AUDIT_LOG',
        settings: 'UI'
    },
    message: null,
    status: null,
    uiConfig: null,
    systemUpdate: null,
    globalQuery: '',
    vaults: [],
    selectedVault: null,
    vaultDetail: null,
    vaultKeys: [],
    keyDetail: null,
    selectedKey: null,
    keySummary: null,
    keyBindings: [],
    vaultAudit: [],
    keyAudit: [],
    configMatches: [],
    configSearchKey: '',
    selectedConfigVault: null,
    selectedConfigKey: null,
    configVaultItems: [],
    configDetail: null,
    configRelations: [],
    vaultItemKind: 'ALL',
    selectedVaultItemKind: 'VK',
    vaultItemSyncStatus: {},
    bulkApplyView: 'items',
    bulkApplyTemplates: [],
    bulkApplyWorkflows: [],
    selectedBulkApplyTemplateName: null,
    selectedBulkApplyWorkflowName: null,
    routeSelectedVaultHash: null,
    hostVaultKeys: [],
    hostVaultConfigs: [],
    hostSelectedKey: null,
    hostSelectedConfigKey: null,
    hostSelectedItemKind: 'VK',
    hostKeyDetail: null,
    hostConfigDetail: null,
    revealValue: false,
    functions: [],
    selectedFunction: null,
    functionDetail: null,
    functionBindings: [],
    functionSummary: null,
    functionImpact: null,
    functionRunResult: null,
    auditVault: null,
    auditKey: null,
    auditRows: [],
    trackedRefAudit: null,
    adminAuditRows: [],
    busy: {},
    ui: {
        sidebarHTML: '',
        secondarySidebarHTML: '',
        secondarySidebarHidden: false,
        headerHTML: '',
        topbarStatusHTML: '',
        leftHTML: '',
        leftVisible: true,
        centerHTML: '',
        rightHTML: '',
        twoPane: false
    }
});

function currentLocale() {
    return state.uiConfig && state.uiConfig.locale === 'en' ? 'en' : 'ko';
}

function t(key) {
    return translate(currentLocale(), key);
}

async function request(path, options = {}) {
    const resp = await fetch(apiURL(path), {
        headers: {
            'Content-Type': 'application/json',
            ...(options.headers || {})
        },
        ...options
    });
    const text = await resp.text();
    let data = null;
    try {
        data = text ? JSON.parse(text) : null;
    } catch (err) {
        data = { raw: text };
    }
    if (!resp.ok) {
        const message = data && data.error ? data.error : (text || ('HTTP ' + resp.status));
        const err = new Error(message);
        err.status = resp.status;
        err.path = path;
        throw err;
    }
    return data;
}

function isIgnorableDetailError(err) {
    return err && (err.status === 403 || err.status === 404);
}

function setBusy(key, value) {
    state.busy[key] = value;
    render();
}

function setMessage(kind, text) {
    state.message = text ? { kind, text } : null;
    renderHeader();
}

function activeTab() {
    return state.activeTabByPage[state.activePage];
}

function tabLabelKey(tab) {
    return {
        ALL_VAULTS: 'tab_all_vaults',
        HOST_VAULT: 'tab_host_vault',
        VAULT_ITEMS: 'tab_vault_items',
        BULK_APPLY: 'tab_bulk_apply',
        FUNCTION_LIST: 'tab_function_list',
        FUNCTION_BINDINGS: 'tab_function_bindings',
        FUNCTION_IMPACT: 'tab_function_impact',
        FUNCTION_RUN: 'tab_function_run',
        AUDIT_LOG: 'tab_audit_log',
        UI: 'tab_ui',
        ADMIN: 'tab_admin'
    }[tab] || tab;
}

function tabLabel(tab) {
    return t(tabLabelKey(tab));
}

function pageLabel(page) {
    const key = pageConfig[page]?.labelKey;
    return key ? t(key) : page;
}

function onGlobalSearchInput(event) {
    state.globalQuery = event?.target?.value || '';
    render();
}

function routePath(page, tab) {
    if (page === 'vaults' && tab === 'VAULT_ITEMS') {
        const vaultHash = state.selectedVault?.vault_runtime_hash || state.routeSelectedVaultHash;
        return vaultHash ? `/vaults/local/${encodeURIComponent(vaultHash)}` : '/vaults/local';
    }
    if (page === 'vaults' && tab === 'BULK_APPLY') {
        const vaultHash = state.selectedVault?.vault_runtime_hash || state.routeSelectedVaultHash;
        if (!vaultHash) return '/vaults/local';
        const view = state.bulkApplyView === 'workflow' ? 'workflow' : 'items';
        return `/vaults/local/${encodeURIComponent(vaultHash)}?tab=bulk-apply&view=${encodeURIComponent(view)}`;
    }
    const entry = routeEntries.find((item) => item.page === page && item.tab === tab);
    return entry ? entry.path : '/';
}

function applyRoute(pathname, search = window.location.search) {
    const normalized = pathname !== '/' && pathname.endsWith('/') ? pathname.slice(0, -1) : pathname;
    const localVaultMatch = normalized.match(/^\/vaults\/local(?:\/([^/]+))?$/);
    if (localVaultMatch) {
        const params = new URLSearchParams(search || '');
        state.activePage = 'vaults';
        state.activeTabByPage.vaults = params.get('tab') === 'bulk-apply' ? 'BULK_APPLY' : 'VAULT_ITEMS';
        state.bulkApplyView = params.get('view') === 'workflow' ? 'workflow' : 'items';
        state.routeSelectedVaultHash = localVaultMatch[1] ? decodeURIComponent(localVaultMatch[1]) : null;
        return;
    }
    const matched = normalized === '/' ? { page: 'vaults', tab: 'ALL_VAULTS' } : routeByPath[normalized];
    if (!matched) return;
    state.activePage = matched.page;
    state.activeTabByPage[matched.page] = matched.tab;
    state.routeSelectedVaultHash = null;
}

function syncRoute(replace) {
    const nextPath = routePath(state.activePage, activeTab());
    const currentURL = window.location.pathname + window.location.search;
    if (currentURL === nextPath) return;
    const fn = replace ? 'replaceState' : 'pushState';
    window.history[fn]({ page: state.activePage, tab: activeTab() }, '', nextPath);
}

function matchesQuery(text) {
    const query = state.globalQuery.trim().toLowerCase();
    if (!query) return true;
    return String(text || '').toLowerCase().includes(query);
}

function navCount(page) {
    if (page === 'vaults') return state.vaults.length;
    if (page === 'functions') return state.functions.length;
    if (page === 'audit') return state.auditRows.length;
    if (page === 'settings') return state.uiConfig ? 2 : 0;
    return 0;
}

function vaultKindLabel(kind) {
    return kind === 'VE' ? 'ENV' : 'KEY';
}

function vaultItemSyncKey(kind, name) {
    return `${kind}:${name}`;
}

function vaultItemSyncEntry(kind, name) {
    return state.vaultItemSyncStatus[vaultItemSyncKey(kind, name)];
}

function itemValueFingerprint(kind, item) {
    if (!item) return '';
    if (kind === 'VE') {
        return item.ref || item.token || item.value || item.key || item.name || '';
    }
    if (item.token) return item.token;
    if (item.scope && item.ref) return `VK:${item.scope}:${item.ref}`;
    return item.ref || item.value || item.name || '';
}

function renderSyncStatus(kind, name) {
    const item = vaultItemSyncEntry(kind, name);
    if (!item) return `<span class="muted">${escapeHTML(t('sync_checking'))}</span>`;
    const distribution = vaultDistributionStatus(kind, name);
    return renderStatusPill(distribution.label, distribution.className);
}

function vaultSyncStatus(kind, name) {
    const item = vaultItemSyncEntry(kind, name);
    if (!item) {
        return { loading: true, label: t('sync_checking'), className: '' };
    }
    if (item.exactCount >= item.comparableCount && item.comparableCount > 0) {
        return { loading: false, label: t('sync_synced'), className: 'active' };
    }
    if (item.exactCount > 1) {
        return { loading: false, label: t('sync_partial'), className: 'pending' };
    }
    return { loading: false, label: t('sync_unsynced'), className: 'error' };
}

function vaultKeyClassStatus(kind, name) {
    const item = vaultItemSyncEntry(kind, name);
    if (!item) {
        return { loading: true, label: t('sync_checking'), className: '' };
    }
    if (item.presentCount <= 1) {
        return { loading: false, label: t('key_class_unique'), className: 'pending' };
    }
    if (item.presentCount >= item.comparableCount && item.comparableCount > 0) {
        return { loading: false, label: t('key_class_global'), className: 'active' };
    }
    return { loading: false, label: t('key_class_fragmented'), className: 'error' };
}

function vaultDistributionStatus(kind, name) {
    const item = vaultItemSyncEntry(kind, name);
    if (!item) {
        return { loading: true, label: t('sync_checking'), className: '' };
    }
    const label = `${item.exactCount}/${item.comparableCount}`;
    if (item.exactCount >= item.comparableCount && item.comparableCount > 0) {
        return { loading: false, label, className: 'active' };
    }
    if (item.exactCount > 1) {
        return { loading: false, label, className: 'pending' };
    }
    return { loading: false, label, className: 'error' };
}

function vaultTargetOptions(includeHost = true) {
    const options = [];
    if (includeHost) options.push({ value: 'host', label: t('host_vault') });
    (state.vaults || []).forEach((vault) => {
        options.push({
            value: vault.vault_runtime_hash,
            label: vault.display_name || vault.vault_name || vault.label || vault.vault_runtime_hash
        });
    });
    return options;
}

function renderConfigRelations() {
    if (!state.configRelations.length) {
        return `<div class="empty">${escapeHTML(t('empty_no_rows'))}</div>`;
    }
    const sections = ['LOCAL', 'EXTERNAL', 'TEMP'].map((scope) => {
        const rows = state.configRelations.filter((item) => String(item.scope || '').toUpperCase() === scope);
        if (!rows.length) return '';
        return `
            <div class="stack">
                <div class="card-title">${scope}</div>
                ${rows.map((item) => `<div class="value">${escapeHTML(item.vault_name || item.vault_runtime_hash || '-')} · ${escapeHTML(item.value || '-')}</div>`).join('')}
            </div>
        `;
    }).filter(Boolean).join('');
    return sections || `<div class="empty">${escapeHTML(t('empty_no_rows'))}</div>`;
}

function configRelationsByScope() {
    return ['LOCAL', 'EXTERNAL', 'TEMP'].map((scope) => ({
        scope,
        rows: state.configRelations.filter((item) => String(item.scope || '').toUpperCase() === scope)
    })).filter((section) => section.rows.length);
}

function renderSidebar() {
    const sections = [
        { page: 'vaults', label: pageLabel('vaults') },
        { page: 'functions', label: pageLabel('functions') },
        { page: 'audit', label: pageLabel('audit') },
        { page: 'settings', label: pageLabel('settings') }
    ];

    const navItems = sections.map((section) => {
        const active = state.activePage === section.page;
        const currentTab = state.activeTabByPage[section.page] || pageConfig[section.page].tabs[0];
        return `
            <div class="sidebar-section">
                <div class="nav-list">
                    <a href="${routePath(section.page, currentTab)}" class="nav-item${active ? ' active' : ''}" data-action="set-page" data-page="${section.page}">
                        <span class="nav-item-main">
                            <span>${escapeHTML(section.label)}</span>
                        </span>
                        <span class="nav-badge">${escapeHTML(navCount(section.page))}</span>
                    </a>
                </div>
            </div>
        `;
    }).join('');

    state.ui.sidebarHTML = navItems;
}

function renderHeader() {
    const page = pageConfig[state.activePage];
    const context = pageContextText();
    const message = state.message ? `<div class="message ${escapeHTML(state.message.kind)}">${escapeHTML(state.message.text)}</div>` : '';
    const breadcrumb = `${pageLabel(state.activePage)} / ${tabLabel(activeTab())}`;
    const headerContent = state.activePage === 'vaults' && state.selectedVault ? `
        <div class="workspace-title">
            <div class="segmented-label">${escapeHTML(t('current_vault_items'))}</div>
            <h1>${escapeHTML(state.selectedVault.display_name || state.selectedVault.vault_name || '-')}</h1>
            <p>${escapeHTML(state.selectedVault.vault_id || state.selectedVault.vault_runtime_hash || '-')}</p>
        </div>
    ` : `
        <div class="workspace-title">
            <h1>${escapeHTML(breadcrumb)}</h1>
            <p>${escapeHTML(context)}</p>
        </div>
    `;
    state.ui.headerHTML = `
        <div class="workspace-title-row">
            ${headerContent}
        </div>
        ${message}
    `;
}

function renderTopbarStatus() {
    state.ui.topbarStatusHTML = '';
}

function pageContextText() {
    if (state.activePage === 'vaults') return t('vault_inventory');
    if (state.activePage === 'functions') return t('function_list');
    if (state.activePage === 'audit') return t('tab_audit_log');
    return t('ui_settings');
}

function renderSecondarySidebar() {
    const page = pageConfig[state.activePage];
    if (!page || !page.tabs || !page.tabs.length) {
        state.ui.secondarySidebarHidden = true;
        state.ui.secondarySidebarHTML = '';
        return;
    }

    if (state.activePage === 'vaults') {
        const vaults = filteredVaults();
        state.ui.secondarySidebarHidden = false;
        state.ui.secondarySidebarHTML = `
            <div class="sidebar-section">
                <div class="sidebar-label">${escapeHTML(t('section_all'))}</div>
                <div class="nav-list">
                    <a
                        href="${routePath('vaults', 'ALL_VAULTS')}"
                        class="nav-item${activeTab() === 'ALL_VAULTS' ? ' active' : ''}"
                        data-action="set-tab"
                        data-tab="ALL_VAULTS"
                    >
                        <span class="nav-item-main">
                            <span>${escapeHTML(t('all_vaults'))}</span>
                        </span>
                    </a>
                    <a
                        href="${routePath('vaults', 'HOST_VAULT')}"
                        class="nav-item${activeTab() === 'HOST_VAULT' ? ' active' : ''}"
                        data-action="set-tab"
                        data-tab="HOST_VAULT"
                    >
                        <span class="nav-item-main">
                            <span>${escapeHTML(t('host_vault'))}</span>
                        </span>
                    </a>
                </div>
            </div>
            <div class="sidebar-section">
                <div class="sidebar-label">${escapeHTML(t('section_local_vaults'))}</div>
                <div class="nav-list">
                    ${vaults.map((item) => `
                        <a
                            href="/vaults/local/${encodeURIComponent(item.vault_runtime_hash)}"
                            class="nav-item${activeTab() !== 'HOST_VAULT' && state.selectedVault && item.vault_runtime_hash === state.selectedVault.vault_runtime_hash ? ' active' : ''}"
                            data-action="select-vault"
                            data-key="${escapeHTML(item.vault_runtime_hash)}"
                        >
                            <span class="nav-item-main">
                                <span>${escapeHTML(item.display_name || item.vault_name || item.vault_runtime_hash)}</span>
                            </span>
                            ${renderStatusPill(item.status || 'active', statusClass(item.status || 'active'))}
                        </a>
                    `).join('') || `<div class="empty">${escapeHTML(t('no_vaults'))}</div>`}
                </div>
            </div>
        `;
        return;
    }

    if (state.activePage === 'functions') {
        const functions = filteredFunctions();
        state.ui.secondarySidebarHidden = false;
        state.ui.secondarySidebarHTML = `
            <div class="sidebar-section">
                <div class="sidebar-label">${escapeHTML(t('section_functions'))}</div>
                <div class="nav-list">
                    ${functions.map((item) => `
                        <a
                            href="${routePath('functions', activeTab())}"
                            class="nav-item${state.selectedFunction && item.name === state.selectedFunction.name ? ' active' : ''}"
                            data-action="select-function"
                            data-key="${escapeHTML(item.name)}"
                        >
                            <span class="nav-item-main">
                                <span>${escapeHTML(item.name)}</span>
                            </span>
                            ${renderStatusPill(item.status || 'active', statusClass(item.status || 'active'))}
                        </a>
                    `).join('') || `<div class="empty">${escapeHTML(t('no_functions'))}</div>`}
                </div>
            </div>
        `;
        return;
    }

    if (state.activePage === 'audit') {
        const vaults = filteredVaults();
        state.ui.secondarySidebarHidden = false;
        state.ui.secondarySidebarHTML = `
            <div class="sidebar-section">
                <div class="sidebar-label">${escapeHTML(t('section_audit_vaults'))}</div>
                <div class="nav-list">
                    ${vaults.map((item) => `
                        <a
                            href="${routePath('audit', 'AUDIT_LOG')}"
                            class="nav-item${state.auditVault === item.vault_runtime_hash ? ' active' : ''}"
                            data-action="audit-page-select-vault"
                            data-key="${escapeHTML(item.vault_runtime_hash)}"
                        >
                            <span class="nav-item-main">
                                <span>${escapeHTML(item.display_name || item.vault_name || item.vault_runtime_hash)}</span>
                            </span>
                            ${renderStatusPill(item.status || 'active', statusClass(item.status || 'active'))}
                        </a>
                    `).join('') || `<div class="empty">${escapeHTML(t('no_vaults'))}</div>`}
                </div>
            </div>
        `;
        return;
    }

    if (state.activePage === 'settings') {
        state.ui.secondarySidebarHidden = false;
        state.ui.secondarySidebarHTML = `
            <div class="sidebar-section">
                <div class="sidebar-label">${escapeHTML(t('section_settings'))}</div>
                <div class="nav-list">
                    ${page.tabs.map((tabName) => `
                        <a
                            href="${routePath(state.activePage, tabName)}"
                            class="nav-item${tabName === activeTab() ? ' active' : ''}"
                            data-action="set-tab"
                            data-tab="${escapeHTML(tabName)}"
                        >
                            <span class="nav-item-main">
                                <span>${escapeHTML(tabLabel(tabName))}</span>
                            </span>
                        </a>
                    `).join('')}
                </div>
            </div>
        `;
        return;
    }

    state.ui.secondarySidebarHidden = false;
    state.ui.secondarySidebarHTML = `
        <div class="sidebar-section">
            <div class="sidebar-label">${escapeHTML(pageLabel(state.activePage))}</div>
            <div class="nav-list">
                ${page.tabs.map((tabName) => `
                    <a
                        href="${routePath(state.activePage, tabName)}"
                        class="nav-item${tabName === activeTab() ? ' active' : ''}"
                        data-action="set-tab"
                        data-tab="${escapeHTML(tabName)}"
                    >
                        <span class="nav-item-main">
                            <span>${escapeHTML(tabLabel(tabName))}</span>
                        </span>
                    </a>
                `).join('')}
            </div>
        </div>
    `;
}

function renderListPane(title, toolbarHTML, bodyHTML) {
    state.ui.leftVisible = true;
    state.ui.twoPane = false;
    const content = bodyHTML && String(bodyHTML).trim()
        ? `<div class="pane-content">${bodyHTML}</div>`
        : '';
    state.ui.leftHTML = `
        <div class="pane-header">
            <div class="pane-title"><strong>${escapeHTML(title)}</strong></div>
            ${toolbarHTML || ''}
        </div>
        ${content}
    `;
    if (state.ui.leftHTML.includes('<div class="pane-content"></div>')) {
        state.ui.leftHTML = state.ui.leftHTML.replace('<div class="pane-content"></div>', '');
    }
}

function renderCenterPane(title, toolbarHTML, bodyHTML) {
    state.ui.centerHTML = `
        <div class="pane-header">
            <div class="pane-title"><strong>${escapeHTML(title)}</strong></div>
            ${toolbarHTML || ''}
        </div>
        <div class="pane-content">${bodyHTML}</div>
    `;
}

function renderRightPane(title, bodyHTML) {
    state.ui.rightHTML = `
        <div class="pane-header">
            <div class="pane-title"><strong>${escapeHTML(title)}</strong></div>
        </div>
        <div class="pane-content">${bodyHTML}</div>
    `;
}

function selectedVaultRecord() {
    return state.vaults.find((item) => item.vault_runtime_hash === state.selectedVault?.vault_runtime_hash) || null;
}

function selectedKeyRecord() {
    return state.vaultKeys.find((item) => item.name === state.selectedKey?.name) || null;
}

function renderVaults() {
    const tab = activeTab();
    if (tab === 'ALL_VAULTS') return renderVaultInventory();
    if (tab === 'HOST_VAULT') return renderHostVault();
    return renderVaultKeys();
}

function filteredVaults() {
    return state.vaults.filter((vault) => matchesQuery(vault.display_name || vault.vault_name || vault.vault_runtime_hash));
}

function hostVaultRow() {
    const nodeID = state.status?.vault_node_uuid || state.status?.node_id || 'host';
    return {
        is_host: true,
        display_name: 'Host Vault',
        vault_name: 'Host Vault',
        vault_runtime_hash: 'host',
        vault_id: `host:${String(nodeID).slice(0, 8)}`,
        managed_paths: ['-'],
        ip: '127.0.0.1',
        status: 'active'
    };
}

function renderVaultInventory() {
    const vaults = filteredVaults();
    const allVaultRows = [hostVaultRow(), ...vaults];
    state.ui.leftHTML = '';
    state.ui.leftVisible = false;
    state.ui.twoPane = true;

    renderCenterPane(
        t('vault_inventory'),
        `
            <div class="toolbar">
                <div class="toolbar-group">
                    <input class="field context-search" id="vault-search" type="search" placeholder="${escapeHTML(t('search_vaults'))}" value="${escapeHTML(state.globalQuery)}">
                </div>
                <span class="pill">${allVaultRows.length} ${escapeHTML(t('count_rows'))}</span>
                <button class="btn btn-soft" data-action="refresh-vaults">${escapeHTML(t('refresh'))}</button>
            </div>
        `,
        renderTable([
            {
                label: t('table_vault_name'),
                render: (row) => `<span>${escapeHTML(row.display_name || row.vault_name)}</span>`
            },
            { label: t('table_identifier'), render: (row) => `<span class="code">${escapeHTML(row.vault_id || row.vault_runtime_hash)}</span>` },
            { label: t('table_path'), render: (row) => escapeHTML((row.managed_paths && row.managed_paths[0]) || '-') },
            { label: 'IP', render: (row) => escapeHTML(row.ip || '-') },
            { label: t('table_status'), render: (row) => renderStatusPill(row.status, statusClass(row.status)) }
        ], allVaultRows, (row) => {
            const classes = [];
            if ((activeTab() === 'HOST_VAULT' && row.is_host) || (activeTab() !== 'HOST_VAULT' && !row.is_host && state.selectedVault && row.vault_runtime_hash === state.selectedVault.vault_runtime_hash)) classes.push('is-selected');
            classes.push('is-clickable');
            return classes.join(' ');
        }, (row) => row.is_host
            ? `data-action="set-tab" data-tab="HOST_VAULT"`
            : `data-action="select-vault" data-key="${escapeHTML(row.vault_runtime_hash)}"`)
    );

    const detail = activeTab() === 'HOST_VAULT' ? hostVaultRow() : (state.vaultDetail || selectedVaultRecord());
    renderRightPane(t('vault_detail'), detail ? `
        <div class="stack">
            <div class="card">
                <div class="card-title">${escapeHTML(t('selected_vault'))}</div>
                ${renderKVGrid([
                    [t('name'), detail.display_name || detail.vault_name || '-'],
                    [t('table_identifier'), detail.vault_id || detail.vault_runtime_hash || '-'],
                    [t('table_path'), (detail.managed_paths && detail.managed_paths[0]) || '-'],
                    ['IP', detail.ip || '-'],
                    [t('table_status'), detail.status || '-']
                ])}
            </div>
            <form class="stack" data-form="save-vault-meta">
                <div class="card">
                    <div class="card-title">${escapeHTML(t('summary'))}</div>
                    <div class="stack">
                        <div class="kv"><span class="label">${escapeHTML(t('table_vault_name'))}</span><input class="field" name="display_name" value="${escapeHTML(detail.display_name || '')}"></div>
                        <div class="kv"><span class="label">${escapeHTML(t('description'))}</span><textarea class="textarea" name="description">${escapeHTML(detail.description || '')}</textarea></div>
                        <div class="kv"><span class="label">${escapeHTML(t('tags_json'))}</span><textarea class="textarea" name="tags_json">${escapeHTML(detail.tags_json || '[]')}</textarea></div>
                    </div>
                </div>
                <div class="toolbar">
                    <button class="btn btn-primary" type="submit">${escapeHTML(t('save'))}</button>
                </div>
            </form>
        </div>
    ` : `<div class="empty">${escapeHTML(t('selected_vault_prompt'))}</div>`);
}

function renderHostVault() {
    const keyRows = state.hostVaultKeys.map((item) => ({ ...item, item_kind: 'VK' }));
    const configRows = state.hostVaultConfigs.map((item) => ({ ...item, name: item.key, item_kind: 'VE' }));
    const selectedItemKind = state.vaultItemKind === 'VE'
        ? 'VE'
        : state.vaultItemKind === 'VK'
            ? 'VK'
            : state.hostSelectedItemKind;
    const selectedItemName = selectedItemKind === 'VE' ? state.hostSelectedConfigKey : state.hostSelectedKey?.name;
    const sourceRows = state.vaultItemKind === 'VE'
        ? configRows
        : state.vaultItemKind === 'ALL'
            ? [...keyRows, ...configRows]
            : keyRows;
    const itemRows = sourceRows.filter((row) => matchesQuery(row.name));
    if (selectedItemName && !itemRows.some((row) => row.name === selectedItemName && row.item_kind === selectedItemKind)) {
        const selectedRow = sourceRows.find((row) => row.name === selectedItemName && row.item_kind === selectedItemKind);
        if (selectedRow) itemRows.unshift(selectedRow);
    }
    const isConfigItem = selectedItemKind === 'VE';
    const detail = isConfigItem ? state.hostConfigDetail : state.hostKeyDetail;
    const visibleValue = detail?.value;
    const detailName = isConfigItem ? state.hostSelectedConfigKey : state.hostSelectedKey?.name;
    const canMoveItem = Boolean(detailName || visibleValue || (isConfigItem ? detail?.key : detail?.name));
    const itemIdentifier = (row) => {
        if (row.item_kind === 'VE') return row.value || row.ref || row.key || '-';
        if (row.token) return row.token;
        if (row.ref) return `VK:${row.scope || 'TEMP'}:${row.ref}`;
        return row.name || '-';
    };
    state.ui.leftHTML = '';
    state.ui.leftVisible = false;
    state.ui.twoPane = true;

    renderCenterPane(
        t('host_vault_title'),
        `
            <div class="toolbar">
                <div class="toolbar-group">
                    <span class="segmented-label">${escapeHTML(t('toolbar_scope'))}</span>
                    <div class="segmented" role="tablist" aria-label="${escapeHTML(t('toolbar_scope'))}">
                        <button class="btn ${state.vaultItemKind === 'ALL' ? 'btn-primary' : 'btn-soft'}" data-action="set-vault-kind" data-kind="ALL" aria-pressed="${state.vaultItemKind === 'ALL' ? 'true' : 'false'}">${escapeHTML(t('filter_all'))}</button>
                        <button class="btn ${state.vaultItemKind === 'VE' ? 'btn-primary' : 'btn-soft'}" data-action="set-vault-kind" data-kind="VE" aria-pressed="${state.vaultItemKind === 'VE' ? 'true' : 'false'}">${escapeHTML(t('filter_configs'))}</button>
                        <button class="btn ${state.vaultItemKind === 'VK' ? 'btn-primary' : 'btn-soft'}" data-action="set-vault-kind" data-kind="VK" aria-pressed="${state.vaultItemKind === 'VK' ? 'true' : 'false'}">${escapeHTML(t('filter_keys'))}</button>
                    </div>
                    <input class="field context-search" id="key-search" type="search" placeholder="${escapeHTML(t('search_host_vault'))}" value="${escapeHTML(state.globalQuery)}">
                </div>
                <span class="pill">${itemRows.length} ${escapeHTML(t('count_items'))}</span>
            </div>
        `,
        renderTable([
            { label: t('table_kind'), render: (row) => `<span class="pill ${row.item_kind === 'VE' ? 'kind-ve' : 'kind-vk'}">${escapeHTML(vaultKindLabel(row.item_kind))}</span>` },
            { label: t('table_name'), render: (row) => `<span>${escapeHTML(row.name)}</span>` },
            { label: t('table_value'), render: (row) => `<span class="code">${escapeHTML(itemIdentifier(row))}</span>` },
            { label: t('table_scope'), render: (row) => renderStatusPill(row.scope || (row.item_kind === 'VE' ? 'LOCAL' : 'TEMP'), scopeClass(row.scope || (row.item_kind === 'VE' ? 'LOCAL' : 'TEMP'))) }
        ], itemRows, (row) => {
            const classes = [];
            if (row.name === selectedItemName && row.item_kind === selectedItemKind) classes.push('is-selected');
            classes.push('is-clickable');
            return classes.join(' ');
        }, (row) => `data-action="select-host-item" data-kind="${escapeHTML(row.item_kind)}" data-key="${escapeHTML(row.name)}"`)
    );

    renderRightPane(isConfigItem ? t('config_detail') : t('key_detail'), `
        <div class="stack">
            ${canMoveItem ? `
                <form class="stack" data-form="${isConfigItem ? 'promote-config' : 'promote-key'}">
                    <div class="card">
                        <div class="card-title">${escapeHTML(isConfigItem ? t('move_config') : t('move_key'))}</div>
                        <div class="stack">
                            <div class="kv"><span class="label">${escapeHTML(t('target_vault'))}</span><select class="select" name="target_vault">${renderTargetOptions(vaultTargetOptions(true), 'host')}</select></div>
                            <div class="kv"><span class="label">${escapeHTML(t('target_scope'))}</span><select class="select" name="target_scope">${renderOptions(isConfigItem ? ['LOCAL', 'EXTERNAL', 'TEMP'] : ['TEMP', 'LOCAL', 'EXTERNAL'], detail?.scope || (isConfigItem ? 'LOCAL' : 'TEMP'))}</select></div>
                            <div class="kv"><span class="label">${escapeHTML(t('value_to_send'))}</span><textarea class="textarea" name="move_value" placeholder="${escapeHTML(t('move_value_placeholder'))}">${escapeHTML(visibleValue || '')}</textarea></div>
                        </div>
                        <div class="muted">${escapeHTML(isConfigItem ? t('host_config_move_help') : t('host_key_move_help'))}</div>
                    </div>
                    <button class="btn btn-soft" type="submit"${visibleValue ? '' : ' disabled'}>${escapeHTML(isConfigItem ? t('move_config') : t('move_key'))}</button>
                </form>
            ` : ''}
            <form class="stack" data-form="${isConfigItem ? 'save-host-config' : 'save-host-key'}">
                <div class="card">
                    <div class="card-title">${escapeHTML(detailName ? t('selected_item') : (isConfigItem ? t('new_config') : t('new_key')))}</div>
                    <div class="stack">
                        <div class="kv"><span class="label">${escapeHTML(isConfigItem ? t('config_name') : t('key_name'))}</span><input class="field" name="${isConfigItem ? 'key' : 'name'}" value="${escapeHTML(detailName || '')}" ${detailName ? 'readonly' : ''}></div>
                        <div class="kv">
                            <span class="label">${escapeHTML(isConfigItem ? t('config_value') : t('key_value'))}</span>
                            <div class="row" style="align-items:center;">
                                <button class="btn btn-soft" type="button" data-action="toggle-reveal">${escapeHTML(state.revealValue ? t('hide') : t('reveal'))}</button>
                                ${state.revealValue && visibleValue ? `<button class="btn btn-soft" type="button" data-action="copy-value">${escapeHTML(t('copy'))}</button>` : ''}
                            </div>
                            <textarea class="textarea" name="value" placeholder="${escapeHTML(detailName ? t('overwrite_placeholder') : t('required'))}">${escapeHTML(state.revealValue ? (visibleValue || '') : '••••••••••••')}</textarea>
                        </div>
                        <div class="kv"><span class="label">${escapeHTML(t('table_scope'))}</span><select class="select" name="scope">${renderOptions(isConfigItem ? ['LOCAL', 'EXTERNAL', 'TEMP'] : ['TEMP', 'LOCAL', 'EXTERNAL'], detail?.scope || (isConfigItem ? 'LOCAL' : 'TEMP'))}</select></div>
                    </div>
                </div>
                <div class="toolbar">
                    <button class="btn btn-primary" type="submit">${escapeHTML(detailName ? t('save') : t('create'))}</button>
                </div>
            </form>
            ${isConfigItem ? `
                <details class="card" open>
                    <summary class="card-title">${escapeHTML(t('local_external_relations'))}</summary>
                    ${renderConfigRelations()}
                </details>
            ` : `
                <div class="card">
                    <div class="card-title">${escapeHTML(t('additional_info'))}</div>
                    ${renderKVGrid([
                        [t('table_identifier'), detail?.token || detail?.ref || '-'],
                        [t('table_scope'), detail?.scope || 'TEMP'],
                        [t('table_status'), detail?.status || 'temp'],
                        [t('storage'), t('host_vault_storage')]
                    ])}
                </div>
            `}
        </div>
    `);
}

function renderVaultKeys() {
    const keyRows = state.vaultKeys.map((item) => ({ ...item, item_kind: 'VK' }));
    const configRows = state.configVaultItems.map((item) => ({
        ...item,
        name: item.key,
        item_kind: 'VE'
    }));
    const selectedItemKind = state.vaultItemKind === 'VE'
        ? 'VE'
        : state.vaultItemKind === 'VK'
            ? 'VK'
            : state.selectedVaultItemKind;
    const selectedItemName = selectedItemKind === 'VE' ? state.selectedConfigKey : state.selectedKey?.name;
    const sourceRows = state.vaultItemKind === 'VE'
        ? configRows
        : state.vaultItemKind === 'ALL'
            ? [...keyRows, ...configRows]
            : keyRows;
    const itemRows = sourceRows.filter((row) => matchesQuery(row.name));
    if (selectedItemName && !itemRows.some((row) => row.name === selectedItemName && row.item_kind === selectedItemKind)) {
        const selectedRow = sourceRows.find((row) => row.name === selectedItemName && row.item_kind === selectedItemKind);
        if (selectedRow) itemRows.unshift(selectedRow);
    }
    const itemIdentifier = (row) => {
        if (row.item_kind === 'VE') return row.value || row.name || row.key || '-';
        if (row.token) return row.token;
        if (row.ref) return `VK:${row.scope || 'LOCAL'}:${row.ref}`;
        return row.name || '-';
    };
    const isConfigItem = selectedItemKind === 'VE';
    const visibleValue = isConfigItem ? state.configDetail?.value : state.keyDetail?.value;
    const itemTitle = isConfigItem ? t('config_detail') : t('key_detail');
    const detailName = isConfigItem ? state.selectedConfigKey : state.selectedKey?.name;
    const itemRefValue = isConfigItem
        ? (detailName || state.configDetail?.key || '')
        : (state.keyDetail?.token || (state.keyDetail?.ref ? `VK:${state.keyDetail.scope || 'LOCAL'}:${state.keyDetail.ref}` : ''));
    const canMoveItem = Boolean(
        detailName
        || visibleValue
        || (isConfigItem ? state.configDetail?.key : state.keyDetail?.name)
    );

    state.ui.leftHTML = '';
    state.ui.leftVisible = false;
    state.ui.twoPane = true;

    renderCenterPane(
        t('current_vault_items'),
        `
            <div class="toolbar">
                <div class="toolbar-group">
                    <div class="toolbar-group">
                        <span class="segmented-label">${escapeHTML(t('toolbar_scope'))}</span>
                        <div class="segmented" role="tablist" aria-label="${escapeHTML(t('toolbar_scope'))}">
                            <button class="btn ${state.vaultItemKind === 'ALL' ? 'btn-primary' : 'btn-soft'}" data-action="set-vault-kind" data-kind="ALL" aria-pressed="${state.vaultItemKind === 'ALL' ? 'true' : 'false'}">${escapeHTML(t('filter_all'))}</button>
                            <button class="btn ${state.vaultItemKind === 'VE' ? 'btn-primary' : 'btn-soft'}" data-action="set-vault-kind" data-kind="VE" aria-pressed="${state.vaultItemKind === 'VE' ? 'true' : 'false'}">${escapeHTML(t('filter_configs'))}</button>
                            <button class="btn ${state.vaultItemKind === 'VK' ? 'btn-primary' : 'btn-soft'}" data-action="set-vault-kind" data-kind="VK" aria-pressed="${state.vaultItemKind === 'VK' ? 'true' : 'false'}">${escapeHTML(t('filter_keys'))}</button>
                        </div>
                    </div>
                    <input class="field context-search" id="key-search" type="search" placeholder="${escapeHTML(t('search_current_vault'))}" value="${escapeHTML(state.globalQuery)}">
                </div>
                <span class="pill">${itemRows.length} ${escapeHTML(t('count_items'))}</span>
                <button class="btn btn-primary" data-action="new-key">${state.vaultItemKind === 'VE' ? escapeHTML(t('new_config')) : escapeHTML(t('new_key'))}</button>
            </div>
        `,
        renderTable([
            { label: t('table_kind'), render: (row) => `<span class="pill ${row.item_kind === 'VE' ? 'kind-ve' : 'kind-vk'}">${escapeHTML(vaultKindLabel(row.item_kind))}</span>` },
            { label: t('table_name'), render: (row) => `<span>${escapeHTML(row.name)}</span>` },
            { label: t('table_value'), render: (row) => `<span class="code">${escapeHTML(itemIdentifier(row))}</span>` },
            { label: t('table_sync'), render: (row) => renderStatusPill(vaultSyncStatus(row.item_kind, row.name).label, vaultSyncStatus(row.item_kind, row.name).className) },
            { label: t('table_key_class'), render: (row) => renderStatusPill(vaultKeyClassStatus(row.item_kind, row.name).label, vaultKeyClassStatus(row.item_kind, row.name).className) },
            { label: t('table_distribution'), render: (row) => renderSyncStatus(row.item_kind, row.name) }
        ], itemRows, (row) => {
            const classes = [];
            if (row.name === selectedItemName && row.item_kind === selectedItemKind) classes.push('is-selected');
            classes.push('is-clickable');
            return classes.join(' ');
        }, (row) => `data-action="select-vault-item" data-kind="${escapeHTML(row.item_kind)}" data-key="${escapeHTML(row.name)}"`)
    );

    const summary = state.keySummary;
    const key = summary && summary.key ? summary.key : state.selectedKey;
    const usageCount = summary ? summary.usage_count : (key ? (key.usage_count || 0) : 0);
    const recentAudit = summary && summary.recent_audit ? summary.recent_audit.slice(0, 3) : [];
    const bindings = summary && summary.bindings ? summary.bindings.slice(0, 5) : [];
    renderRightPane(itemTitle, state.selectedVault ? `
        <div class="stack">
            ${canMoveItem ? `
                <form class="stack" data-form="${isConfigItem ? 'promote-config' : 'promote-key'}">
                    <div class="card">
                        <div class="card-title">${escapeHTML(isConfigItem ? t('move_config') : t('move_key'))}</div>
                        <div class="stack">
                            <div class="kv"><span class="label">${escapeHTML(t('target_vault'))}</span><select class="select" name="target_vault">${renderTargetOptions(vaultTargetOptions(true), state.selectedVault ? state.selectedVault.vault_runtime_hash : 'host')}</select></div>
                            <div class="kv"><span class="label">${escapeHTML(t('target_scope'))}</span><select class="select" name="target_scope">${renderOptions(isConfigItem ? ['LOCAL', 'EXTERNAL', 'TEMP'] : ['TEMP', 'LOCAL', 'EXTERNAL'], isConfigItem ? (state.configDetail?.scope || 'LOCAL') : (state.keyDetail?.scope || 'TEMP'))}</select></div>
                            <div class="kv"><span class="label">${escapeHTML(t('value_to_send'))}</span><textarea class="textarea" name="move_value" placeholder="${escapeHTML(t('move_value_placeholder'))}">${escapeHTML(visibleValue || '')}</textarea></div>
                        </div>
                        <div class="muted">${escapeHTML(isConfigItem ? t('vault_config_move_help') : t('vault_key_move_help'))}</div>
                    </div>
                    <button class="btn btn-soft" type="submit"${visibleValue ? '' : ' disabled'}>${escapeHTML(isConfigItem ? t('move_config') : t('move_key'))}</button>
                </form>
            ` : ''}
            <form class="stack" data-form="${isConfigItem ? 'save-agent-config' : 'save-key'}">
                <div class="card">
                    <div class="card-title">${escapeHTML(detailName ? t('selected_item') : (isConfigItem ? t('new_config') : t('new_key')))}</div>
                    <div class="stack">
                        <div class="kv"><span class="label">${escapeHTML(isConfigItem ? t('config_name') : t('key_name'))}</span><input class="field" name="${isConfigItem ? 'key' : 'name'}" value="${escapeHTML(detailName || '')}" ${detailName ? 'readonly' : ''}></div>
                        <div class="kv">
                            <span class="label">${escapeHTML(isConfigItem ? t('config_value') : t('key_value'))}</span>
                            <div class="row" style="align-items:center;">
                                <button class="btn btn-soft" type="button" data-action="toggle-reveal">${escapeHTML(state.revealValue ? t('hide') : t('reveal'))}</button>
                                ${state.revealValue && visibleValue ? `<button class="btn btn-soft" type="button" data-action="copy-value">${escapeHTML(t('copy'))}</button>` : ''}
                            </div>
                            <textarea class="textarea" name="value" placeholder="${escapeHTML(detailName ? t('overwrite_placeholder') : t('required'))}">${escapeHTML(state.revealValue ? (visibleValue || '') : '••••••••••••')}</textarea>
                        </div>
                        ${isConfigItem ? `
                            <input type="hidden" name="scope" value="${escapeHTML(state.configDetail?.scope || 'LOCAL')}">
                            <input type="hidden" name="status" value="${escapeHTML(state.configDetail?.status || 'active')}">
                        ` : `
                            <div class="kv"><span class="label">${escapeHTML(t('description'))}</span><textarea class="textarea" name="description">${escapeHTML(key && key.description ? key.description : '')}</textarea></div>
                            <div class="kv"><span class="label">${escapeHTML(t('tags_json'))}</span><textarea class="textarea" name="tags_json">${escapeHTML(key && key.tags_json ? key.tags_json : '[]')}</textarea></div>
                        `}
                    </div>
                </div>
                <div class="toolbar">
                    <button class="btn btn-primary" type="submit">${escapeHTML(detailName ? t('save') : t('create'))}</button>
                    ${detailName ? `<button class="btn btn-danger" type="button" data-action="${isConfigItem ? 'delete-agent-config' : 'delete-key'}">${escapeHTML(t('delete'))}</button>` : ''}
                </div>
            </form>
            ${!isConfigItem ? `
                <details class="card">
                    <summary class="card-title">${escapeHTML(t('additional_info'))}</summary>
                    ${renderKVGrid([
                        [t('bindings_count'), summary ? summary.bindings_total || summary.bindings_count || 0 : 0],
                        [t('usage_count'), usageCount || 0],
                        [t('table_scope'), key ? (key.scope || '-') : '-'],
                        [t('table_status'), key ? (key.status || '-') : '-']
                    ])}
                    ${bindings.length ? `<div class="stack" style="margin-top:12px;">${bindings.map((item) => `<div class="value">${escapeHTML(item.binding_type)} / ${escapeHTML(item.target_name)} / ${escapeHTML(item.field_key || '-')}</div>`).join('')}</div>` : `<div class="empty">${escapeHTML(t('no_binding_info'))}</div>`}
                    ${recentAudit.length ? `<div class="stack" style="margin-top:12px;">${recentAudit.map((item) => `<div class="value">${escapeHTML(item.action || item.event_type || 'event')} · ${escapeHTML(item.created_at || item.timestamp || '-')}</div>`).join('')}</div>` : `<div class="empty">${escapeHTML(t('no_audit_info'))}</div>`}
                </details>
                <form class="stack" data-form="save-key-fields">
                    <details class="card">
                        <summary class="card-title">${escapeHTML(t('field_edit'))}</summary>
                        <textarea class="textarea" name="fields_json">${escapeHTML(key && summary && summary.key && summary.key.fields ? formatJSON(summary.key.fields.map((field) => ({ key: field.key, type: field.type, value: '' }))) : '[]')}</textarea>
                        <button class="btn" type="submit"${key ? '' : ' disabled'}>${escapeHTML(t('save_fields'))}</button>
                    </details>
                </form>
            ` : ''}
            ${isConfigItem ? `
                <details class="card" open>
                    <summary class="card-title">${escapeHTML(t('local_external_relations'))}</summary>
                    ${renderConfigRelations()}
                </details>
            ` : ''}
        </div>
    ` : `<div class="empty">${escapeHTML(t('select_vault_prompt'))}</div>`);
}

function renderVaultBindings() {
    const keys = state.vaultKeys.filter((item) => matchesQuery(item.name));
    renderListPane(
        t('binding_focus'),
        `
            <div class="card">
                <div class="card-title">${escapeHTML(t('vault_context'))}</div>
                <div class="value">${escapeHTML(state.selectedVault ? state.selectedVault.vault_runtime_hash : t('no_vault_selected'))}</div>
            </div>
        `,
        state.selectedVault ? renderMiniList(
            keys,
            (item) => item.name,
            {
                name: 'select-key',
                selected: (item) => state.selectedKey && item.name === state.selectedKey.name
            },
            (item) => `<span>${escapeHTML(item.name)}</span>`,
            (item) => renderStatusPill(item.scope || 'LOCAL', scopeClass(item.scope))
        ) : `<div class="empty">${escapeHTML(t('select_vault_first'))}</div>`
    );

    const bindings = state.keyBindings || [];
    renderCenterPane(
        t('key_bindings'),
        `<div class="toolbar"><span class="pill">${bindings.length} ${escapeHTML(t('binding_count'))}</span></div>`,
        renderTable([
            { label: t('target_type'), render: (row) => escapeHTML(row.binding_type || '-') },
            { label: t('target_name'), render: (row) => escapeHTML(row.target_name || '-') },
            { label: t('field'), render: (row) => escapeHTML(row.field_key || '-') },
            { label: t('required'), render: (row) => escapeHTML(String(row.required)) },
            { label: t('ref'), render: (row) => `<span class="code">${escapeHTML(row.ref_canonical || '-')}</span>` }
        ], bindings, () => '')
    );

    renderRightPane(t('edit_bindings'), state.selectedKey ? `
        <div class="stack">
            <div class="card">
                <div class="card-title">${escapeHTML(t('selected_key'))}</div>
                ${renderKVGrid([
                    [t('name'), state.selectedKey.name],
                    [t('table_scope'), state.selectedKey.scope || '-'],
                    [t('page_vaults'), state.selectedVault ? state.selectedVault.vault_runtime_hash : '-']
                ])}
            </div>
            <form class="stack" data-form="replace-key-bindings">
                <div class="card">
                    <div class="card-title">${escapeHTML(t('bindings_json'))}</div>
                    <textarea class="textarea" name="bindings_json">${escapeHTML(formatJSON((state.keyBindings || []).map((item) => ({
                        binding_id: item.binding_id,
                        binding_type: item.binding_type,
                        target_name: item.target_name,
                        field_key: item.field_key,
                        required: item.required
                    }))))}</textarea>
                </div>
                <div class="toolbar">
                    <button class="btn btn-primary" type="submit">${escapeHTML(t('replace'))}</button>
                    <button class="btn btn-danger" type="button" data-action="delete-all-bindings">${escapeHTML(t('delete_all_bindings'))}</button>
                </div>
            </form>
        </div>
    ` : `<div class="empty">${escapeHTML(t('select_key_manage_bindings'))}</div>`);
}

function renderVaultAudit() {
    const useKeyMode = !!state.auditKey;
    const rows = useKeyMode ? state.keyAudit : state.vaultAudit;
    renderListPane(
        t('audit_scope'),
        `
            <div class="stack">
                <div class="card">
                    <div class="card-title">${escapeHTML(t('page_vaults'))}</div>
                    <div class="value">${escapeHTML(state.selectedVault ? state.selectedVault.display_name || state.selectedVault.vault_name : t('no_vault_selected'))}</div>
                </div>
                <button class="btn btn-soft" data-action="clear-audit-key">${escapeHTML(t('vault_only_feed'))}</button>
            </div>
        `,
        state.selectedVault ? renderMiniList(
            state.vaultKeys,
            (item) => item.name,
            {
                name: 'audit-select-key',
                selected: (item) => state.auditKey === item.name
            },
            (item) => `<span>${escapeHTML(item.name)}</span>`,
            () => ''
        ) : `<div class="empty">${escapeHTML(t('select_vault_first'))}</div>`
    );

    renderCenterPane(
        t('vault_audit'),
        `<div class="toolbar"><span class="pill">${rows.length} ${escapeHTML(t('count_events'))}</span><span class="pill">${escapeHTML(useKeyMode ? t('key_scope') : t('vault_scope'))}</span></div>`,
        renderTable([
            { label: t('time'), render: (row) => escapeHTML(row.created_at || row.timestamp || '-') },
            { label: t('action'), render: (row) => escapeHTML(row.action || row.event_type || '-') },
            { label: t('source'), render: (row) => escapeHTML(row.actor_type || row.source_type || '-') },
            { label: t('object'), render: (row) => escapeHTML(row.entity_id || row.object_id || '-') }
        ], rows, () => '')
    );

    renderRightPane(t('audit_event_detail'), rows.length ? `
        <div class="card">
            <div class="card-title">${escapeHTML(t('latest_event'))}</div>
            <pre class="code">${escapeHTML(formatJSON(rows[0]))}</pre>
        </div>
    ` : `<div class="empty">${escapeHTML(t('no_audit_scope_events'))}</div>`);
}

function renderGrouped() {
    const groupedRows = state.groupedRows.filter((row) => matchesQuery(row.name));
    const selectedGroup = groupedRows.find((row) => row.name === state.selectedGroupedName) || null;
    const entries = selectedGroup ? selectedGroup.entries : [];
    const selectedEntry = state.selectedGroupedEntry || (entries[0] || null);
    const visibleValue = selectedEntry && selectedEntry.kind === 'VE' ? state.configDetail?.value : state.keyDetail?.value;

    renderListPane(
        t('grouped_keys'),
        `
            <div class="stack">
                <div class="toolbar">
                    <a href="${routePath('grouped', GROUPED_KEYS_TAB)}" class="btn ${activeTab() === GROUPED_KEYS_TAB ? 'btn-primary' : 'btn-soft'}" data-action="jump-tab" data-page="grouped" data-tab="${GROUPED_KEYS_TAB}">${escapeHTML(t('filter_keys'))}</a>
                    <a href="${routePath('grouped', GROUPED_CONFIGS_TAB)}" class="btn ${activeTab() === GROUPED_CONFIGS_TAB ? 'btn-primary' : 'btn-soft'}" data-action="jump-tab" data-page="grouped" data-tab="${GROUPED_CONFIGS_TAB}">${escapeHTML(t('filter_configs'))}</a>
                </div>
            </div>
        `,
        renderMiniList(
            groupedRows,
            (item) => item.name,
            {
                name: 'select-grouped-name',
                selected: (item) => item.name === state.selectedGroupedName
            },
            (item) => `
                <div class="stack">
                    <span>${escapeHTML(item.name)}</span>
                    <span class="muted">${escapeHTML(item.count)} ${escapeHTML(t('count_items'))}</span>
                </div>
            `,
            () => ''
        )
    );

    renderCenterPane(
        t('grouped_items'),
        `<div class="toolbar"><span class="pill">${entries.length} ${escapeHTML(t('count_rows'))}</span></div>`,
        renderTable([
            { label: t('table_name_generic'), render: (row) => `<button data-action="select-grouped-entry" data-vault="${escapeHTML(row.vault_runtime_hash)}" data-kind="${escapeHTML(row.kind)}" data-key="${escapeHTML(row.name)}">${escapeHTML(row.name)}</button>` },
            { label: t('page_vaults'), render: (row) => escapeHTML(row.vault_name || row.vault_id || '-') },
            { label: t('table_identifier'), render: (row) => `<span class="code">${escapeHTML(row.vault_id || row.vault_runtime_hash)}</span>` },
            { label: t('table_path'), render: (row) => escapeHTML(row.path || '-') },
            { label: t('ip'), render: (row) => escapeHTML(row.ip || '-') }
        ], entries, (row) => selectedEntry && row.vault_runtime_hash === selectedEntry.vault_runtime_hash && row.kind === selectedEntry.kind && row.name === selectedEntry.name ? 'is-selected' : '')
    );

    renderRightPane(activeTab() === GROUPED_CONFIGS_TAB ? t('config_detail') : t('key_detail'), selectedEntry ? `
        <div class="stack">
            <div class="card">
                <div class="card-title">${escapeHTML(t('selected_item'))}</div>
                ${renderKVGrid([
                    [t('name'), selectedEntry.name || '-'],
                    [t('page_vaults'), selectedEntry.vault_name || '-'],
                    [t('table_identifier'), selectedEntry.vault_id || selectedEntry.vault_runtime_hash || '-'],
                    [t('table_path'), selectedEntry.path || '-'],
                    [t('ip'), selectedEntry.ip || '-']
                ])}
            </div>
            <div class="card">
                <div class="card-title">${escapeHTML(t('value'))}</div>
                <div class="stack">
                    <div class="row" style="align-items:center;">
                        <button class="btn btn-soft" type="button" data-action="toggle-reveal">${escapeHTML(state.revealValue ? t('hide') : t('reveal'))}</button>
                        ${state.revealValue && visibleValue ? `<button class="btn btn-soft" type="button" data-action="copy-value">${escapeHTML(t('copy'))}</button>` : ''}
                    </div>
                    <textarea class="textarea" readonly>${escapeHTML(state.revealValue ? (visibleValue || '') : '••••••••••••')}</textarea>
                </div>
            </div>
        </div>
    ` : `<div class="empty">${escapeHTML(t('select_grouped_entry_prompt'))}</div>`);
}

function renderConfigs() {
    const tab = activeTab();
    if (tab === CONFIG_SEARCH_TAB) return renderConfigSearch();
    if (tab === CONFIG_BULK_TAB) return renderConfigBulk();
    if (tab === CONFIG_PER_VAULT_TAB) return renderConfigPerVault();
    return renderConfigSummary();
}

function renderConfigSummary() {
    renderListPane(
        t('summary_filters'),
        '',
        `
            <div class="card">
                <div class="card-title">${escapeHTML(t('current_summary'))}</div>
                <div class="stack">
                    <span class="value">${escapeHTML(t('config_summary_first_pass'))}</span>
                    <span class="muted">${escapeHTML(t('config_summary_guidance'))}</span>
                </div>
            </div>
        `
    );

    const summary = state.configsSummary || { total_configs: 0, agents_with_configs: 0, total_agents: 0 };
    renderCenterPane(
        t('config_summary'),
        '',
        `
            <div class="metrics">
                <div class="metric"><span class="label">${escapeHTML(t('total_configs'))}</span><strong>${escapeHTML(summary.total_configs)}</strong></div>
                <div class="metric"><span class="label">${escapeHTML(t('agents_with_configs'))}</span><strong>${escapeHTML(summary.agents_with_configs)}</strong></div>
                <div class="metric"><span class="label">${escapeHTML(t('total_agents'))}</span><strong>${escapeHTML(summary.total_agents)}</strong></div>
            </div>
            <div class="empty">${escapeHTML(t('config_summary_backend_notice'))}</div>
        `
    );

    renderRightPane(t('config_summary_detail'), `
        <div class="card">
            <div class="card-title">${escapeHTML(t('guidance'))}</div>
            <div class="stack">
                <div class="value">${escapeHTML(t('guidance_search_single_config'))}</div>
                <div class="value">${escapeHTML(t('guidance_per_vault'))}</div>
                <div class="value">${escapeHTML(t('guidance_bulk_update'))}</div>
            </div>
        </div>
    `);
}

function renderConfigSearch() {
    renderListPane(
        'Config search',
        `
            <form class="stack" data-form="search-configs">
                <input class="field" name="key" placeholder="APP_ENV" value="${escapeHTML(state.configSearchKey)}">
                <button class="btn btn-primary" type="submit">Search</button>
            </form>
        `,
        state.configMatches.length ? renderMiniList(
            state.configMatches,
            (item) => item.vault_runtime_hash + ':' + item.key,
            {
                name: 'select-config-match',
                selected: (item) => state.selectedConfigVault === item.vault_runtime_hash && state.selectedConfigKey === item.key
            },
            (item) => `
                <div class="stack">
                    <span>${escapeHTML(item.key)}</span>
                    <span class="muted">${escapeHTML(item.vault_runtime_hash)}</span>
                </div>
            `,
            (item) => renderStatusPill(item.scope || 'LOCAL', scopeClass(item.scope))
        ) : '<div class="empty">Run a search to inspect config values across vaults.</div>'
    );

    renderCenterPane(
        'Search Results',
        `<div class="toolbar"><span class="pill">${state.configMatches.length} matches</span></div>`,
        renderTable([
            { label: 'Key', render: (row) => `<button data-action="select-config-match" data-vault="${escapeHTML(row.vault_runtime_hash)}" data-key="${escapeHTML(row.key)}">${escapeHTML(row.key)}</button>` },
            { label: 'Vault', render: (row) => escapeHTML(row.vault_runtime_hash) },
            { label: 'Scope', render: (row) => renderStatusPill(row.scope || 'LOCAL', scopeClass(row.scope)) },
            { label: 'Status', render: (row) => renderStatusPill(row.status || 'active', statusClass(row.status || 'active')) },
            { label: 'Value', render: (row) => `<span class="code">${escapeHTML(row.value || '')}</span>` }
        ], state.configMatches, (row) => row.vault_runtime_hash === state.selectedConfigVault && row.key === state.selectedConfigKey ? 'is-selected' : '')
    );

    renderRightPane('Config detail', state.configDetail ? `
        <div class="stack">
            <div class="card">
                <div class="card-title">Selected Config</div>
                ${renderKVGrid([
                    ['Key', state.configDetail.key],
                    ['Vault', state.configDetail.vault_runtime_hash || '-'],
                    ['Scope', state.configDetail.scope || '-'],
                    ['Status', state.configDetail.status || '-']
                ])}
            </div>
            <form class="stack" data-form="save-agent-config">
                <div class="card">
                    <div class="card-title">Edit Config</div>
                    <div class="stack">
                        <div class="kv"><span class="label">Key</span><input class="field" name="key" value="${escapeHTML(state.configDetail.key)}" readonly></div>
                        <div class="kv"><span class="label">Value</span><textarea class="textarea" name="value">${escapeHTML(state.configDetail.value || '')}</textarea></div>
                        <div class="inline-grid">
                            <div class="kv"><span class="label">Scope</span><select class="select" name="scope">${renderOptions(['TEMP','LOCAL','EXTERNAL'], state.configDetail.scope || 'LOCAL')}</select></div>
                            <div class="kv"><span class="label">Status</span><select class="select" name="status">${renderOptions(['active','pending','revoked'], state.configDetail.status || 'active')}</select></div>
                        </div>
                    </div>
                </div>
                <div class="toolbar">
                    <button class="btn btn-primary" type="submit">Save Config</button>
                    <button class="btn btn-danger" type="button" data-action="delete-agent-config">Delete</button>
                </div>
            </form>
        </div>
    ` : '<div class="empty">Select a search result to inspect and edit it.</div>');
}

function renderConfigBulk() {
    renderListPane(
        'Bulk target',
        `
            <div class="stack">
                <div class="card">
                    <div class="card-title">Bulk Operations</div>
                    <div class="value">Use these forms for cross-agent VE changes.</div>
                </div>
            </div>
        `,
        '<div class="empty">Bulk operations apply across agents and refresh summary/search blocks after success.</div>'
    );

    renderCenterPane(
        'Bulk Forms',
        '',
        `
            <div class="stack">
                <form class="stack" data-form="bulk-config-set">
                    <div class="card">
                        <div class="card-title">Bulk Set</div>
                        <div class="stack">
                            <input class="field" name="key" placeholder="KEY_NAME">
                            <textarea class="textarea" name="value" placeholder="Value"></textarea>
                            <div class="inline-grid">
                                <div class="kv"><span class="label">Scope</span><select class="select" name="scope">${renderOptions(['LOCAL','EXTERNAL','TEMP'], 'LOCAL')}</select></div>
                                <div class="kv"><span class="label">Status</span><select class="select" name="status">${renderOptions(['active','pending','revoked'], 'active')}</select></div>
                            </div>
                        </div>
                    </div>
                    <button class="btn btn-primary" type="submit">Run Bulk Set</button>
                </form>
                <form class="stack" data-form="bulk-config-update">
                    <div class="card">
                        <div class="card-title">Bulk Update</div>
                        <div class="stack">
                            <input class="field" name="key" placeholder="KEY_NAME">
                            <textarea class="textarea" name="new_value" placeholder="New value"></textarea>
                            <textarea class="textarea" name="old_value" placeholder="Old value (required when multiple values exist)"></textarea>
                        </div>
                    </div>
                    <button class="btn" type="submit">Run Bulk Update</button>
                </form>
            </div>
        `
    );

    renderRightPane('Bulk result', `
        <div class="card">
            <div class="card-title">Behavior</div>
            <div class="stack">
                <div class="value">Bulk set creates or overwrites a key across agents.</div>
                <div class="value">Bulk update changes values for an existing key across agents.</div>
                <div class="value">Both endpoints are all-or-nothing and may require trusted IP access.</div>
            </div>
        </div>
    `);
}

function renderConfigPerVault() {
    const vaults = state.vaults.filter((item) => matchesQuery(item.display_name || item.vault_name));
    renderListPane(
        'Vault selector',
        `
            <div class="stack">
                <input class="field" id="config-vault-search" type="search" placeholder="${escapeHTML(t('search_vaults'))}" value="${escapeHTML(state.globalQuery)}">
            </div>
        `,
        renderMiniList(
            vaults,
            (item) => item.vault_runtime_hash,
            {
                name: 'select-config-vault',
                selected: (item) => state.selectedConfigVault === item.vault_runtime_hash
            },
            (item) => `<span>${escapeHTML(item.display_name || item.vault_name)}</span>`,
            () => ''
        )
    );

    renderCenterPane(
        'Per-vault configs',
        `<div class="toolbar"><span class="pill">${state.configVaultItems.length} configs</span></div>`,
        renderTable([
            { label: 'Key', render: (row) => `<button data-action="select-config-detail" data-key="${escapeHTML(row.key)}">${escapeHTML(row.key)}</button>` },
            { label: 'Scope', render: (row) => renderStatusPill(row.scope || 'LOCAL', scopeClass(row.scope)) },
            { label: 'Status', render: (row) => renderStatusPill(row.status || 'active', statusClass(row.status || 'active')) },
            { label: 'Value', render: (row) => `<span class="code">${escapeHTML(row.value || '')}</span>` }
        ], state.configVaultItems, (row) => row.key === state.selectedConfigKey ? 'is-selected' : '')
    );

    renderRightPane('Per-vault config detail', state.selectedConfigVault ? `
        <div class="stack">
            <form class="stack" data-form="save-agent-config">
                <div class="card">
                    <div class="card-title">${state.configDetail ? 'Edit Config' : 'Create Config'}</div>
                    <div class="stack">
                        <input class="field" name="key" placeholder="KEY_NAME" value="${escapeHTML(state.configDetail ? state.configDetail.key : '')}">
                        <textarea class="textarea" name="value">${escapeHTML(state.configDetail ? (state.configDetail.value || '') : '')}</textarea>
                        <div class="inline-grid">
                            <div class="kv"><span class="label">Scope</span><select class="select" name="scope">${renderOptions(['TEMP','LOCAL','EXTERNAL'], state.configDetail ? (state.configDetail.scope || 'LOCAL') : 'LOCAL')}</select></div>
                            <div class="kv"><span class="label">Status</span><select class="select" name="status">${renderOptions(['active','pending','revoked'], state.configDetail ? (state.configDetail.status || 'active') : 'active')}</select></div>
                        </div>
                    </div>
                </div>
                <div class="toolbar">
                    <button class="btn btn-primary" type="submit">${state.configDetail ? 'Save Config' : 'Create Config'}</button>
                    ${state.configDetail ? '<button class="btn btn-danger" type="button" data-action="delete-agent-config">Delete</button>' : ''}
                </div>
            </form>
        </div>
    ` : '<div class="empty">Select a vault to inspect per-vault configs.</div>');
}

function filteredFunctions() {
    return state.functions.filter((item) => matchesQuery(item.name));
}

function allVaultRows() {
    return [hostVaultRow(), ...filteredVaults()];
}

function currentVaultSelectedKind() {
    if (activeTab() === 'HOST_VAULT') {
        return state.vaultItemKind === 'VE'
            ? 'VE'
            : state.vaultItemKind === 'VK'
                ? 'VK'
                : state.hostSelectedItemKind;
    }
    return state.vaultItemKind === 'VE'
        ? 'VE'
        : state.vaultItemKind === 'VK'
            ? 'VK'
            : state.selectedVaultItemKind;
}

function currentVaultSelectedName() {
    const kind = currentVaultSelectedKind();
    if (activeTab() === 'HOST_VAULT') {
        return kind === 'VE' ? state.hostSelectedConfigKey : state.hostSelectedKey?.name;
    }
    return kind === 'VE' ? state.selectedConfigKey : state.selectedKey?.name;
}

function hostVaultVisibleRows() {
    const keyRows = state.hostVaultKeys.map((item) => ({ ...item, item_kind: 'VK' }));
    const configRows = state.hostVaultConfigs.map((item) => ({ ...item, name: item.key, item_kind: 'VE' }));
    const sourceRows = state.vaultItemKind === 'VE'
        ? configRows
        : state.vaultItemKind === 'ALL'
            ? [...keyRows, ...configRows]
            : keyRows;
    const selectedName = currentVaultSelectedName();
    const selectedKind = currentVaultSelectedKind();
    const rows = sourceRows.filter((row) => matchesQuery(row.name));
    if (selectedName && !rows.some((row) => row.name === selectedName && row.item_kind === selectedKind)) {
        const selectedRow = sourceRows.find((row) => row.name === selectedName && row.item_kind === selectedKind);
        if (selectedRow) rows.unshift(selectedRow);
    }
    return rows;
}

function localVaultVisibleRows() {
    const keyRows = state.vaultKeys.map((item) => ({ ...item, item_kind: 'VK' }));
    const configRows = state.configVaultItems.map((item) => ({ ...item, name: item.key, item_kind: 'VE' }));
    const sourceRows = state.vaultItemKind === 'VE'
        ? configRows
        : state.vaultItemKind === 'ALL'
            ? [...keyRows, ...configRows]
            : keyRows;
    const selectedName = currentVaultSelectedName();
    const selectedKind = currentVaultSelectedKind();
    const rows = sourceRows.filter((row) => matchesQuery(row.name));
    if (selectedName && !rows.some((row) => row.name === selectedName && row.item_kind === selectedKind)) {
        const selectedRow = sourceRows.find((row) => row.name === selectedName && row.item_kind === selectedKind);
        if (selectedRow) rows.unshift(selectedRow);
    }
    return rows;
}

function vaultVisibleRows() {
    if (activeTab() === 'HOST_VAULT') return hostVaultVisibleRows();
    if (activeTab() === 'VAULT_ITEMS') return localVaultVisibleRows();
    return [];
}

function vaultItemIdentifier(row) {
    if (row.item_kind === 'VE') return row.value || row.name || row.key || row.ref || '-';
    if (row.token) return row.token;
    if (row.ref) return `VK:${row.scope || (activeTab() === 'HOST_VAULT' ? 'TEMP' : 'LOCAL')}:${row.ref}`;
    return row.name || '-';
}

function vaultCenterTitle() {
    if (activeTab() === 'ALL_VAULTS') return t('vault_inventory');
    if (activeTab() === 'HOST_VAULT') return t('host_vault_title');
    if (activeTab() === 'BULK_APPLY') return t('tab_bulk_apply');
    return t('current_vault_items');
}

function vaultRightPaneTitle() {
    if (activeTab() === 'ALL_VAULTS') return t('vault_detail');
    if (activeTab() === 'BULK_APPLY') {
        return state.bulkApplyView === 'workflow' ? t('view_workflows') : t('view_items');
    }
    return currentVaultSelectedKind() === 'VE' ? t('config_detail') : t('key_detail');
}

function selectedBulkApplyTemplate() {
    return state.bulkApplyTemplates.find((item) => item.name === state.selectedBulkApplyTemplateName) || null;
}

function selectedBulkApplyWorkflow() {
    return state.bulkApplyWorkflows.find((item) => item.name === state.selectedBulkApplyWorkflowName) || null;
}

function selectedInventoryDetail() {
    return state.vaultDetail || selectedVaultRecord();
}

function vaultPanel() {
    const isHost = activeTab() === 'HOST_VAULT';
    const isConfigItem = currentVaultSelectedKind() === 'VE';
    const detail = isHost
        ? (isConfigItem ? state.hostConfigDetail : state.hostKeyDetail)
        : (isConfigItem ? state.configDetail : state.keyDetail);
    const detailName = isHost
        ? (isConfigItem ? state.hostSelectedConfigKey : state.hostSelectedKey?.name)
        : (isConfigItem ? state.selectedConfigKey : state.selectedKey?.name);
    const visibleValue = detail?.value || '';
    const keySummary = !isHost && !isConfigItem ? state.keySummary : null;
    const keyRecord = !isHost && !isConfigItem ? (keySummary && keySummary.key ? keySummary.key : state.selectedKey) : null;

    return {
        isHost,
        isConfigItem,
        detail,
        detailName,
        visibleValue,
        canMoveItem: Boolean(detailName || visibleValue || (isConfigItem ? detail?.key : detail?.name)),
        targetVaultDefault: isHost ? 'host' : (state.selectedVault ? state.selectedVault.vault_runtime_hash : 'host'),
        currentScope: detail?.scope || (isConfigItem ? 'LOCAL' : (isHost ? 'TEMP' : 'TEMP')),
        scopeOptions: isConfigItem ? ['LOCAL', 'EXTERNAL', 'TEMP'] : ['TEMP', 'LOCAL', 'EXTERNAL'],
        moveHelperText: isConfigItem
            ? (isHost ? t('host_config_move_help') : t('vault_config_move_help'))
            : (isHost ? t('host_key_move_help') : t('vault_key_move_help')),
        saveForm: isHost
            ? (isConfigItem ? 'save-host-config' : 'save-host-key')
            : (isConfigItem ? 'save-agent-config' : 'save-key'),
        createTitle: isConfigItem ? t('new_config') : t('new_key'),
        showScopeSelect: isHost,
        showDelete: !isHost && Boolean(detailName),
        deleteAction: isConfigItem ? 'delete-agent-config' : 'delete-key',
        showKeyMeta: !isHost && !isConfigItem,
        description: keyRecord && keyRecord.description ? keyRecord.description : '',
        tagsJSON: keyRecord && keyRecord.tags_json ? keyRecord.tags_json : '[]',
        configStatus: isConfigItem ? (detail?.status || 'active') : '',
        itemRefValue: isConfigItem
            ? (detailName || detail?.key || '')
            : (detail?.token || (detail?.ref ? `VK:${detail.scope || (isHost ? 'TEMP' : 'LOCAL')}:${detail.ref}` : '')),
        bindingsTotal: keySummary ? (keySummary.bindings_total || keySummary.bindings_count || 0) : 0,
        usageCount: keySummary ? (keySummary.usage_count || 0) : 0,
        bindings: keySummary && keySummary.bindings ? keySummary.bindings.slice(0, 5) : [],
        recentAudit: keySummary && keySummary.recent_audit ? keySummary.recent_audit.slice(0, 3) : [],
        status: detail?.status || (isHost ? 'temp' : 'active')
    };
}

function functionCenterTitle() {
    if (activeTab() === 'FUNCTION_BINDINGS') return t('bindings');
    if (activeTab() === 'FUNCTION_IMPACT') return t('impact');
    if (activeTab() === 'FUNCTION_RUN') return t('function_run');
    return t('function_list');
}

function functionRightPaneTitle() {
    return t('summary');
}

function functionImpactRefs() {
    return state.functionImpact && state.functionImpact.refs ? state.functionImpact.refs : [];
}

function functionBindingsPayload() {
    return (state.functionBindings || []).map((item) => ({
        binding_id: item.binding_id,
        ref_canonical: item.ref_canonical,
        vault_hash: item.vault_hash,
        secret_name: item.secret_name,
        field_key: item.field_key,
        required: item.required
    }));
}

function prettyJSON(value) {
    return formatJSON(value);
}

function settingsCenterTitle() {
    return activeTab() === 'ADMIN' ? t('admin_settings') : t('ui_settings');
}

function settingsRightPaneTitle() {
    return activeTab() === 'ADMIN' ? t('admin_setting_detail') : t('edit_ui_config');
}

function syncVaultVuePanels() {
    state.ui.leftHTML = '';
    state.ui.leftVisible = false;
    state.ui.twoPane = true;
    state.ui.rightHTML = '';
}

function syncTemplatePageLayout() {
    state.ui.leftHTML = '';
    state.ui.leftVisible = false;
    state.ui.twoPane = true;
    state.ui.centerHTML = '';
    state.ui.rightHTML = '';
}

function render() {
    renderTopbarStatus();
    renderSidebar();
    renderHeader();
    if (state.activePage === 'vaults') {
        syncVaultVuePanels();
        return;
    }
    if (state.activePage === 'functions' || state.activePage === 'audit' || state.activePage === 'settings') {
        syncTemplatePageLayout();
        return;
    }
    renderSecondarySidebar();
    if (state.activePage === 'configs') renderConfigs();
}

async function loadStatus() {
    try {
        state.status = await request('/api/status');
    } catch (err) {
        setMessage('error', 'Failed to load runtime status: ' + err.message);
    }
}

async function loadUIConfig() {
    try {
        state.uiConfig = await request('/api/ui/config');
    } catch (err) {
        setMessage('error', 'Failed to load UI config: ' + err.message);
    }
}

async function loadSystemUpdate() {
    try {
        state.systemUpdate = await request('/api/system/update');
    } catch (err) {
        setMessage('error', 'Failed to load update status: ' + err.message);
    }
}

async function loadVaults() {
    try {
        const data = await request('/api/vaults?limit=200');
        state.vaults = data.vaults || [];
        if (state.routeSelectedVaultHash) {
            const routedVault = state.vaults.find((item) => item.vault_runtime_hash === state.routeSelectedVaultHash);
            if (routedVault) {
                state.selectedVault = routedVault;
            }
        }
        if (!state.selectedVault && state.vaults.length) {
            state.selectedVault = state.vaults[0];
        }
        if (!state.selectedConfigVault && state.selectedVault) {
            state.selectedConfigVault = state.selectedVault.vault_runtime_hash;
        }
        if (!state.auditVault && state.selectedVault) {
            state.auditVault = state.selectedVault.vault_runtime_hash;
        }
    } catch (err) {
        setMessage('error', 'Failed to load vaults: ' + err.message);
    }
}

async function loadSelectedVaultDetail() {
    if (!state.selectedVault) return;
    state.vaultDetail = await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash));
}

async function loadSelectedVaultKeys() {
    if (!state.selectedVault) return;
    const data = await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash) + '/keys');
    state.vaultKeys = data.secrets || [];
    if (state.vaultKeys.length && (!state.selectedKey || !state.vaultKeys.some((item) => item.name === state.selectedKey.name))) {
        state.selectedKey = state.vaultKeys[0];
    } else if (!state.vaultKeys.length || !state.vaultKeys.some((item) => item.name === state.selectedKey?.name)) {
        state.selectedKey = null;
    }
}

async function loadSelectedKeyDetail() {
    if (!state.selectedVault || !state.selectedKey) {
        state.keyDetail = null;
        return;
    }
    try {
        state.keyDetail = await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash) + '/keys/' + encodeURIComponent(state.selectedKey.name));
    } catch (err) {
        if (!isIgnorableDetailError(err)) throw err;
        state.keyDetail = null;
    }
}

async function loadSelectedKeySummary() {
    if (!state.selectedVault || !state.selectedKey) {
        state.keySummary = null;
        return;
    }
    try {
        state.keySummary = await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash) + '/keys/' + encodeURIComponent(state.selectedKey.name) + '/summary');
    } catch (err) {
        if (!isIgnorableDetailError(err)) throw err;
        state.keySummary = null;
    }
}

async function loadSelectedKeyBindings() {
    if (!state.selectedVault || !state.selectedKey) {
        state.keyBindings = [];
        return;
    }
    try {
        const data = await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash) + '/keys/' + encodeURIComponent(state.selectedKey.name) + '/bindings');
        state.keyBindings = data.bindings || [];
    } catch (err) {
        if (!isIgnorableDetailError(err)) throw err;
        state.keyBindings = [];
    }
}

async function loadVaultAudit() {
    if (!state.selectedVault) {
        state.vaultAudit = [];
        return;
    }
    const data = await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash) + '/audit?limit=50');
    state.vaultAudit = data.events || [];
}

async function loadKeyAudit() {
    if (!state.selectedVault || !state.auditKey) {
        state.keyAudit = [];
        return;
    }
    const data = await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash) + '/keys/' + encodeURIComponent(state.auditKey) + '/audit?limit=50');
    state.keyAudit = data.events || [];
}

async function loadConfigsSummary() {
    state.configsSummary = await request('/api/configs/summary');
}

async function searchConfigs(key) {
    state.configSearchKey = key;
    if (!key) {
        state.configMatches = [];
        state.configDetail = null;
        return;
    }
    const data = await request('/api/configs/search/' + encodeURIComponent(key));
    state.configMatches = data.matches || [];
    if (state.configMatches.length) {
        state.selectedConfigVault = state.configMatches[0].vault_runtime_hash;
        state.selectedConfigKey = state.configMatches[0].key;
        await loadSelectedConfigDetail();
    }
}

async function loadConfigsForVault() {
    if (!state.selectedConfigVault) {
        state.configVaultItems = [];
        return;
    }
    const data = await request('/api/agents/' + encodeURIComponent(state.selectedConfigVault) + '/configs');
    state.configVaultItems = data.configs || [];
    if (state.configVaultItems.length && (!state.selectedConfigKey || !state.configVaultItems.some((item) => item.key === state.selectedConfigKey))) {
        state.selectedConfigKey = state.configVaultItems[0].key;
    } else if (!state.configVaultItems.length || !state.configVaultItems.some((item) => item.key === state.selectedConfigKey)) {
        state.selectedConfigKey = null;
    }
}

async function loadSelectedConfigDetail() {
    if (!state.selectedConfigVault || !state.selectedConfigKey) {
        state.configDetail = null;
        return;
    }
    try {
        state.configDetail = await request('/api/agents/' + encodeURIComponent(state.selectedConfigVault) + '/configs/' + encodeURIComponent(state.selectedConfigKey));
    } catch (err) {
        if (!isIgnorableDetailError(err)) throw err;
        state.configDetail = null;
    }
}

async function loadConfigRelations() {
    if (!state.selectedConfigKey) {
        state.configRelations = [];
        return;
    }
    try {
        const data = await request('/api/configs/search/' + encodeURIComponent(state.selectedConfigKey));
        state.configRelations = (data.matches || []).map((item) => ({
            key: item.key,
            value: item.value || '',
            scope: item.scope || 'LOCAL',
            vault_runtime_hash: item.vault_runtime_hash,
            vault_name: item.vault || item.vault_name || item.vault_runtime_hash
        }));
    } catch (_) {
        state.configRelations = [];
    }
}

async function loadHostVaultKeys() {
    const data = await request('/api/host-vault/keys');
    state.hostVaultKeys = data.secrets || [];
    if (state.hostVaultKeys.length && (!state.hostSelectedKey || !state.hostVaultKeys.some((item) => item.name === state.hostSelectedKey.name))) {
        state.hostSelectedKey = state.hostVaultKeys[0];
    } else if (!state.hostVaultKeys.length || !state.hostVaultKeys.some((item) => item.name === state.hostSelectedKey?.name)) {
        state.hostSelectedKey = null;
    }
}

async function loadHostVaultConfigs() {
    const data = await request('/api/host-vault/configs');
    state.hostVaultConfigs = data.configs || [];
    if (state.hostVaultConfigs.length && (!state.hostSelectedConfigKey || !state.hostVaultConfigs.some((item) => item.key === state.hostSelectedConfigKey))) {
        state.hostSelectedConfigKey = state.hostVaultConfigs[0].key;
    } else if (!state.hostVaultConfigs.length || !state.hostVaultConfigs.some((item) => item.key === state.hostSelectedConfigKey)) {
        state.hostSelectedConfigKey = null;
    }
}

async function loadHostVaultKeyDetail() {
    if (!state.hostSelectedKey) {
        state.hostKeyDetail = null;
        return;
    }
    try {
        state.hostKeyDetail = await request('/api/host-vault/keys/' + encodeURIComponent(state.hostSelectedKey.name));
    } catch (err) {
        if (!isIgnorableDetailError(err)) throw err;
        state.hostKeyDetail = null;
    }
}

async function loadHostVaultConfigDetail() {
    if (!state.hostSelectedConfigKey) {
        state.hostConfigDetail = null;
        return;
    }
    try {
        state.hostConfigDetail = await request('/api/host-vault/configs/' + encodeURIComponent(state.hostSelectedConfigKey));
    } catch (err) {
        if (!isIgnorableDetailError(err)) throw err;
        state.hostConfigDetail = null;
    }
}

async function loadVaultItemSyncStatus() {
    if (!state.selectedVault || activeTab() !== 'VAULT_ITEMS') {
        state.vaultItemSyncStatus = {};
        return;
    }

    const nextStatus = {};
    const vaults = state.vaults || [];
    const currentVaultHash = state.selectedVault.vault_runtime_hash;
    const normalizedConfigsByVault = new Map();
    const normalizedKeysByVault = new Map();

    await Promise.all(vaults.map(async (vault) => {
        const vaultHash = vault.vault_runtime_hash;

        try {
            const supportsConfigs = vaultHash === currentVaultHash || (vault.local_enabled !== false && (vault.configs_count || 0) > 0);
            if (!supportsConfigs) {
                normalizedConfigsByVault.set(vaultHash, null);
            } else {
                const data = await request('/api/agents/' + encodeURIComponent(vaultHash) + '/configs');
                const configs = new Map();
                (data.configs || []).forEach((item) => {
                    configs.set(item.key, itemValueFingerprint('VE', item));
                });
                normalizedConfigsByVault.set(vaultHash, configs);
            }
        } catch (_) {
            normalizedConfigsByVault.set(vaultHash, vaultHash === currentVaultHash ? new Map() : null);
        }

        try {
            const data = await request('/api/vaults/' + encodeURIComponent(vaultHash) + '/keys');
            const keys = new Map();
            (data.keys || data.secrets || []).forEach((item) => {
                keys.set(item.name, itemValueFingerprint('VK', item));
            });
            normalizedKeysByVault.set(vaultHash, keys);
        } catch (_) {
            normalizedKeysByVault.set(vaultHash, null);
        }
    }));

    state.configVaultItems.forEach((item) => {
        const configKey = item.key || item.name;
        const currentFingerprint = itemValueFingerprint('VE', item);
        let comparableCount = 0;
        let presentCount = 0;
        let exactCount = 0;
        vaults.forEach((vault) => {
            const entry = normalizedConfigsByVault.get(vault.vault_runtime_hash);
            if (!(entry instanceof Map)) return;
            comparableCount += 1;
            if (entry.has(configKey)) {
                presentCount += 1;
                if (entry.get(configKey) === currentFingerprint) {
                    exactCount += 1;
                }
            }
        });
        nextStatus[vaultItemSyncKey('VE', configKey)] = {
            presentCount,
            exactCount,
            comparableCount: Math.max(comparableCount, 1)
        };
    });

    state.vaultKeys.forEach((item) => {
        const currentFingerprint = itemValueFingerprint('VK', item);
        let comparableCount = 0;
        let presentCount = 0;
        let exactCount = 0;
        vaults.forEach((vault) => {
            const entry = normalizedKeysByVault.get(vault.vault_runtime_hash);
            if (!(entry instanceof Map)) return;
            comparableCount += 1;
            if (entry.has(item.name)) {
                presentCount += 1;
                if (entry.get(item.name) === currentFingerprint) {
                    exactCount += 1;
                }
            }
        });
        nextStatus[vaultItemSyncKey('VK', item.name)] = {
            presentCount,
            exactCount,
            comparableCount: Math.max(comparableCount, 1)
        };
    });

    state.vaultItemSyncStatus = nextStatus;
}

async function loadBulkApplyTemplates() {
    if (!state.selectedVault) {
        state.bulkApplyTemplates = [];
        state.selectedBulkApplyTemplateName = null;
        return;
    }
    const data = await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash) + '/bulk-apply/templates');
    state.bulkApplyTemplates = data.templates || [];
    if (state.bulkApplyTemplates.length && !state.bulkApplyTemplates.some((item) => item.name === state.selectedBulkApplyTemplateName)) {
        state.selectedBulkApplyTemplateName = state.bulkApplyTemplates[0].name;
    } else if (!state.bulkApplyTemplates.length) {
        state.selectedBulkApplyTemplateName = null;
    }
}

async function loadBulkApplyWorkflows() {
    if (!state.selectedVault) {
        state.bulkApplyWorkflows = [];
        state.selectedBulkApplyWorkflowName = null;
        return;
    }
    const data = await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash) + '/bulk-apply/workflows');
    state.bulkApplyWorkflows = data.workflows || [];
    if (state.bulkApplyWorkflows.length && !state.bulkApplyWorkflows.some((item) => item.name === state.selectedBulkApplyWorkflowName)) {
        state.selectedBulkApplyWorkflowName = state.bulkApplyWorkflows[0].name;
    } else if (!state.bulkApplyWorkflows.length) {
        state.selectedBulkApplyWorkflowName = null;
    }
}

async function loadFunctions() {
    const data = await request('/api/functions/global');
    state.functions = data.functions || [];
    if (!state.selectedFunction && state.functions.length) {
        state.selectedFunction = state.functions[0];
    }
}

async function loadSelectedFunctionDetail() {
    if (!state.selectedFunction) {
        state.functionDetail = null;
        return;
    }
    state.functionDetail = await request('/api/functions/global/' + encodeURIComponent(state.selectedFunction.name));
}

async function loadSelectedFunctionBindings() {
    if (!state.selectedFunction) {
        state.functionBindings = [];
        return;
    }
    const data = await request('/api/targets/function/' + encodeURIComponent(state.selectedFunction.name) + '/bindings');
    state.functionBindings = data.bindings || [];
}

async function loadSelectedFunctionImpact() {
    if (!state.selectedFunction) {
        state.functionImpact = null;
        state.functionSummary = null;
        return;
    }
    state.functionImpact = await request('/api/targets/function/' + encodeURIComponent(state.selectedFunction.name) + '/impact');
    state.functionSummary = await request('/api/targets/function/' + encodeURIComponent(state.selectedFunction.name) + '/summary');
}

async function loadTrackedRefAudit() {
    state.trackedRefAudit = await request('/api/tracked-refs/audit');
}

async function loadAuditVaultFeed() {
    if (!state.auditVault) {
        state.auditRows = [];
        return;
    }
    const data = await request('/api/vaults/' + encodeURIComponent(state.auditVault) + '/audit?limit=50');
    state.auditRows = data.events || [];
}

async function loadAuditKeyFeed() {
    if (!state.auditVault || !state.auditKey) {
        state.auditRows = [];
        return;
    }
    const data = await request('/api/vaults/' + encodeURIComponent(state.auditVault) + '/keys/' + encodeURIComponent(state.auditKey) + '/audit?limit=50');
    state.auditRows = data.events || [];
}

async function loadCatalogAudit() {
    if (!state.catalogAuditEntityType || !state.catalogAuditEntityID) {
        state.catalogAuditRows = [];
        return;
    }
    const data = await request('/api/catalog/audit?entity_type=' + encodeURIComponent(state.catalogAuditEntityType) + '&entity_id=' + encodeURIComponent(state.catalogAuditEntityID) + '&limit=50');
    state.catalogAuditRows = data.events || [];
}

async function loadAdminAudit() {
    try {
        const data = await request('/api/admin/audit/recent?limit=50');
        state.adminAuditRows = data.events || data.rows || [];
    } catch (err) {
        state.adminAuditRows = [];
        setMessage('warn', 'Admin audit requires an admin session: ' + err.message);
    }
}

async function loadGroupedRows() {
            const kind = activeTab() === 'VE' ? 'VE' : 'VK';
    const groupedMap = new Map();
    const tasks = state.vaults.map(async (vault) => {
        if (kind === 'VK') {
            const data = await request('/api/vaults/' + encodeURIComponent(vault.vault_runtime_hash) + '/keys');
            (data.secrets || []).forEach((item) => {
                const key = item.name;
                const entry = {
                    kind: 'VK',
                    name: item.name,
                    scope: item.scope || 'LOCAL',
                    status: item.status || 'active',
                    vault_name: vault.display_name || vault.vault_name || vault.label || vault.vault_runtime_hash,
                    vault_runtime_hash: vault.vault_runtime_hash,
                    vault_id: vault.vault_id || vault.vault_runtime_hash,
                    path: (vault.managed_paths && vault.managed_paths[0]) || '-',
                    ip: vault.ip || '-',
                    updated_at: item.updated_at || '',
                    usage_count: item.usage_count || item.binding_count || 0
                };
                if (!groupedMap.has(key)) groupedMap.set(key, []);
                groupedMap.get(key).push(entry);
            });
            return;
        }

        const data = await request('/api/agents/' + encodeURIComponent(vault.vault_runtime_hash) + '/configs');
        (data.configs || []).forEach((item) => {
            const key = item.key;
            const entry = {
                kind: 'VE',
                name: item.key,
                scope: item.scope || 'LOCAL',
                status: item.status || 'active',
                vault_name: vault.display_name || vault.vault_name || vault.label || vault.vault_runtime_hash,
                vault_runtime_hash: vault.vault_runtime_hash,
                vault_id: vault.vault_id || vault.vault_runtime_hash,
                path: (vault.managed_paths && vault.managed_paths[0]) || '-',
                ip: vault.ip || '-',
                updated_at: item.updated_at || '',
                usage_count: 0
            };
            if (!groupedMap.has(key)) groupedMap.set(key, []);
            groupedMap.get(key).push(entry);
        });
    });

    await Promise.all(tasks);
    state.groupedRows = Array.from(groupedMap.entries()).map(([name, entries]) => {
        const currentVaultMatch = state.selectedVault ? entries.find((entry) => entry.vault_runtime_hash === state.selectedVault.vault_runtime_hash) : null;
        const localMatch = entries.find((entry) => entry.scope === 'LOCAL');
        const representative = currentVaultMatch || localMatch || entries[0];
        return {
            kind,
            name,
            count: entries.length,
            representative,
            entries
        };
    }).sort((a, b) => a.name.localeCompare(b.name));
    if (!state.selectedGroupedName && state.groupedRows.length) {
        state.selectedGroupedName = state.groupedRows[0].name;
        state.selectedGroupedEntries = state.groupedRows[0].entries;
        state.selectedGroupedEntry = state.groupedRows[0].entries[0] || null;
    }
}

async function loadGroupedEntryDetail() {
    if (!state.selectedGroupedEntry) return;
    state.revealValue = false;
    if (state.selectedGroupedEntry.kind === 'VK') {
        state.selectedVault = state.vaults.find((item) => item.vault_runtime_hash === state.selectedGroupedEntry.vault_runtime_hash) || state.selectedVault;
        state.selectedKey = { name: state.selectedGroupedEntry.name };
        await loadSelectedKeyDetail();
        await loadSelectedKeySummary();
        return;
    }
    state.selectedConfigVault = state.selectedGroupedEntry.vault_runtime_hash;
    state.selectedConfigKey = state.selectedGroupedEntry.name;
    await loadSelectedConfigDetail();
}

async function syncPageData() {
    try {
        if (state.activePage === 'vaults') {
            if (!state.vaults.length) await loadVaults();
            if (state.selectedVault) {
                await loadSelectedVaultDetail();
                if (activeTab() === 'BULK_APPLY') {
                    await Promise.all([loadBulkApplyTemplates(), loadBulkApplyWorkflows()]);
                } else if (activeTab() !== 'Host Vault') {
                    await loadSelectedVaultKeys();
                    state.selectedConfigVault = state.selectedVault.vault_runtime_hash;
                    await loadConfigsForVault();
                    await loadVaultItemSyncStatus();
                }
                if (activeTab() === 'HOST_VAULT') {
                    await loadHostVaultKeys();
                    await loadHostVaultConfigs();
                    const useHostConfigDetail = state.vaultItemKind === 'VE'
                        || (state.vaultItemKind === 'ALL' && state.hostSelectedItemKind === 'VE');
                    if (useHostConfigDetail) {
                        await loadHostVaultConfigDetail();
                    } else {
                        await loadHostVaultKeyDetail();
                    }
                } else if (activeTab() === 'VAULT_ITEMS') {
                    const useConfigDetail = state.vaultItemKind === 'VE'
                        || (state.vaultItemKind === 'ALL' && state.selectedVaultItemKind === 'VE');
                    if (useConfigDetail) {
                        if (state.selectedConfigKey) {
                            await loadSelectedConfigDetail();
                            await loadConfigRelations();
                        } else {
                            state.configDetail = null;
                            state.configRelations = [];
                        }
                        state.revealValue = false;
                    } else {
                        await loadSelectedKeyDetail();
                        await loadSelectedKeySummary();
                        state.configRelations = [];
                        state.revealValue = false;
                    }
                }
            }
        } else if (state.activePage === 'functions') {
            await loadFunctions();
            if (state.selectedFunction) {
                await loadSelectedFunctionDetail();
                if (activeTab() === 'FUNCTION_BINDINGS') {
                    await loadSelectedFunctionBindings();
                } else if (activeTab() === 'FUNCTION_IMPACT') {
                    await loadSelectedFunctionImpact();
                }
            }
        } else if (state.activePage === 'audit') {
            if (!state.vaults.length) await loadVaults();
            await loadTrackedRefAudit();
            if (!state.auditVault && state.vaults.length) state.auditVault = state.vaults[0].vault_runtime_hash;
            await loadAuditVaultFeed();
        } else if (state.activePage === 'settings') {
            await loadUIConfig();
            await loadSystemUpdate();
        }
        setMessage(null, null);
    } catch (err) {
        setMessage('error', err.message);
    }
    render();
}

async function setPage(page) {
    state.activePage = page;
    if (page !== 'vaults') {
        state.routeSelectedVaultHash = null;
    }
    syncRoute(false);
    await syncPageData();
}

async function setTab(tab) {
    state.activeTabByPage[state.activePage] = tab;
    if (state.activePage === 'vaults' && tab === 'BULK_APPLY' && !state.bulkApplyView) {
        state.bulkApplyView = 'items';
    }
    if (!(state.activePage === 'vaults' && (tab === 'VAULT_ITEMS' || tab === 'BULK_APPLY'))) {
        state.routeSelectedVaultHash = null;
    }
    syncRoute(false);
    await syncPageData();
}

async function selectVaultByKey(key) {
    const vault = state.vaults.find((item) => item.vault_runtime_hash === key);
    if (!vault) return;
    state.selectedVault = vault;
    state.activeTabByPage.vaults = 'VAULT_ITEMS';
    state.routeSelectedVaultHash = vault.vault_runtime_hash;
    state.selectedConfigVault = vault.vault_runtime_hash;
    state.auditVault = vault.vault_runtime_hash;
    state.selectedKey = null;
    state.configDetail = null;
    syncRoute(false);
    await syncPageData();
}

async function selectKeyByName(name) {
    const key = state.vaultKeys.find((item) => item.name === name);
    if (!key) return;
    state.selectedVaultItemKind = 'VK';
    state.selectedKey = key;
    state.auditKey = name;
    if (state.activePage === 'vaults') {
        await syncPageData();
    } else if (state.activePage === 'audit' && activeTab() === 'VE') {
        await syncPageData();
    } else {
        render();
    }
}

async function selectConfigMatch(vaultHash, key) {
    state.selectedConfigVault = vaultHash;
    state.selectedConfigKey = key;
    await loadSelectedConfigDetail();
    render();
}

async function selectFunctionByName(name) {
    const item = state.functions.find((fn) => fn.name === name);
    if (!item) return;
    state.selectedFunction = item;
    await syncPageData();
}

async function handleFormSubmit(form) {
    const formType = form.dataset.form;
    const data = new FormData(form);
    try {
        if (formType === 'save-vault-meta') {
            const payload = {
                display_name: data.get('display_name'),
                description: data.get('description'),
                tags_json: data.get('tags_json')
            };
            await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash), {
                method: 'PATCH',
                body: JSON.stringify(payload)
            });
            setMessage('ok', 'Vault metadata saved.');
            await syncPageData();
            return;
        }
        if (formType === 'save-key') {
            const name = String(data.get('name') || '').trim();
            const value = String(data.get('value') || '').trim();
            const description = String(data.get('description') || '');
            const tagsJSON = String(data.get('tags_json') || '[]');
            if (!state.selectedKey) {
                await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash) + '/keys', {
                    method: 'POST',
                    body: JSON.stringify({ name, value })
                });
                state.selectedKey = { name };
            } else {
                if (value) {
                    await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash) + '/keys/' + encodeURIComponent(state.selectedKey.name), {
                        method: 'PUT',
                        body: JSON.stringify({ name: state.selectedKey.name, value })
                    });
                }
                await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash) + '/keys/' + encodeURIComponent(state.selectedKey.name) + '/meta', {
                    method: 'PATCH',
                    body: JSON.stringify({ display_name: state.selectedKey.name, description, tags_json: tagsJSON })
                });
            }
            setMessage('ok', state.selectedKey ? 'Key saved.' : 'Key created.');
            await syncPageData();
            return;
        }
        if (formType === 'save-host-key') {
            const payload = {
                name: String(data.get('name') || '').trim(),
                value: String(data.get('value') || '').trim(),
                scope: String(data.get('scope') || 'TEMP')
            };
            await request('/api/host-vault/keys', {
                method: 'POST',
                body: JSON.stringify(payload)
            });
            state.hostSelectedItemKind = 'VK';
            state.hostSelectedKey = { name: payload.name };
            setMessage('ok', 'Host key saved.');
            await syncPageData();
            return;
        }
        if (formType === 'save-key-fields') {
            const raw = String(data.get('fields_json') || '[]');
            const parsed = JSON.parse(raw);
            await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash) + '/keys/' + encodeURIComponent(state.selectedKey.name) + '/fields', {
                method: 'PUT',
                body: JSON.stringify({ fields: parsed })
            });
            setMessage('ok', 'Key fields saved.');
            await syncPageData();
            return;
        }
        if (formType === 'replace-key-bindings') {
            const parsed = JSON.parse(String(data.get('bindings_json') || '[]'));
            await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash) + '/keys/' + encodeURIComponent(state.selectedKey.name) + '/bindings', {
                method: 'PUT',
                body: JSON.stringify({ bindings: parsed })
            });
            setMessage('ok', 'Key bindings replaced.');
            await syncPageData();
            return;
        }
        if (formType === 'promote-key') {
            const isHostVault = activeTab() === 'HOST_VAULT';
            const targetVault = String(data.get('target_vault') || '').trim();
            const targetScope = String(data.get('target_scope') || 'TEMP').trim();
            const value = String(data.get('move_value') || '').trim() || (isHostVault ? (state.hostKeyDetail?.value || '') : (state.keyDetail?.value || ''));
            const name = String(data.get('name') || '').trim() || (isHostVault ? (state.hostSelectedKey?.name || state.hostKeyDetail?.name || '') : (state.selectedKey?.name || state.keyDetail?.name || ''));
            if (!targetVault || !name || !value) throw new Error(t('promote_key_missing'));
            if (targetVault === 'host') {
                await request('/api/host-vault/keys', {
                    method: 'POST',
                    body: JSON.stringify({ name, value, scope: targetScope })
                });
            } else {
                if (targetScope !== 'TEMP') {
                    throw new Error(t('promote_key_scope_restricted'));
                }
                await request('/api/vaults/' + encodeURIComponent(targetVault) + '/keys', {
                    method: 'POST',
                    body: JSON.stringify({ name, value })
                });
            }
            setMessage('ok', t('key_sent'));
            await syncPageData();
            return;
        }
        if (formType === 'search-configs') {
            await searchConfigs(String(data.get('key') || '').trim());
            render();
            return;
        }
        if (formType === 'save-agent-config') {
            const payload = {
                key: String(data.get('key') || '').trim(),
                value: String(data.get('value') || '').trim(),
                scope: String(data.get('scope') || 'LOCAL'),
                status: String(data.get('status') || 'active')
            };
            await request('/api/agents/' + encodeURIComponent(state.selectedConfigVault) + '/configs', {
                method: 'POST',
                body: JSON.stringify(payload)
            });
            state.selectedConfigKey = payload.key;
            setMessage('ok', 'Config saved.');
            await syncPageData();
            return;
        }
        if (formType === 'save-host-config') {
            const payload = {
                key: String(data.get('key') || '').trim(),
                value: String(data.get('value') || '').trim(),
                scope: String(data.get('scope') || 'LOCAL'),
                status: 'active'
            };
            await request('/api/host-vault/configs', {
                method: 'POST',
                body: JSON.stringify(payload)
            });
            state.hostSelectedItemKind = 'VE';
            state.hostSelectedConfigKey = payload.key;
            setMessage('ok', 'Host config saved.');
            await syncPageData();
            return;
        }
        if (formType === 'promote-config') {
            const isHostVault = activeTab() === 'HOST_VAULT';
            const targetVault = String(data.get('target_vault') || '').trim();
            const targetScope = String(data.get('target_scope') || 'LOCAL').trim();
            const key = String(data.get('key') || '').trim() || (isHostVault ? (state.hostSelectedConfigKey || state.hostConfigDetail?.key || '') : (state.selectedConfigKey || state.configDetail?.key || ''));
            const value = String(data.get('move_value') || '').trim() || (isHostVault ? (state.hostConfigDetail?.value || '') : (state.configDetail?.value || ''));
            if (!targetVault || !key || !value) throw new Error(t('promote_config_missing'));
            const payload = {
                key,
                value,
                scope: targetScope,
                status: 'active'
            };
            if (targetVault === 'host') {
                await request('/api/host-vault/configs', {
                    method: 'POST',
                    body: JSON.stringify(payload)
                });
            } else {
                await request('/api/agents/' + encodeURIComponent(targetVault) + '/configs', {
                    method: 'POST',
                    body: JSON.stringify(payload)
                });
            }
            setMessage('ok', t('config_sent'));
            await syncPageData();
            return;
        }
        if (formType === 'bulk-config-set') {
            const payload = {
                key: String(data.get('key') || '').trim(),
                value: String(data.get('value') || '').trim(),
                scope: String(data.get('scope') || 'LOCAL'),
                status: String(data.get('status') || 'active')
            };
            await request('/api/configs/bulk-set', {
                method: 'POST',
                body: JSON.stringify(payload)
            });
            setMessage('ok', 'Bulk set completed.');
            await syncPageData();
            return;
        }
        if (formType === 'bulk-config-update') {
            const payload = {
                key: String(data.get('key') || '').trim(),
                old_value: String(data.get('old_value') || '').trim(),
                new_value: String(data.get('new_value') || '').trim()
            };
            await request('/api/configs/bulk-update', {
                method: 'POST',
                body: JSON.stringify(payload)
            });
            setMessage('ok', 'Bulk update completed.');
            await syncPageData();
            return;
        }
        if (formType === 'replace-function-bindings') {
            const parsed = JSON.parse(String(data.get('bindings_json') || '[]'));
            await request('/api/targets/function/' + encodeURIComponent(state.selectedFunction.name) + '/bindings', {
                method: 'PUT',
                body: JSON.stringify({ bindings: parsed })
            });
            setMessage('ok', 'Function bindings replaced.');
            await syncPageData();
            return;
        }
        if (formType === 'run-function') {
            const payload = {
                prompt: String(data.get('prompt') || ''),
                system_prompt: String(data.get('system_prompt') || ''),
                temperature: Number(data.get('temperature') || 0.2),
                max_output_tokens: Number(data.get('max_output_tokens') || 2048),
                timeout_seconds: Number(data.get('timeout_seconds') || 120)
            };
            state.functionRunResult = await request('/api/functions/global/' + encodeURIComponent(state.selectedFunction.name) + '/run', {
                method: 'POST',
                body: JSON.stringify(payload)
            });
            setMessage('ok', 'Function run completed.');
            render();
            return;
        }
        if (formType === 'load-catalog-audit') {
            state.catalogAuditEntityType = String(data.get('entity_type') || 'secret');
            state.catalogAuditEntityID = String(data.get('entity_id') || '').trim();
            await loadCatalogAudit();
            render();
            return;
        }
        if (formType === 'save-settings') {
            const payload = {
                locale: String(data.get('locale') || state.uiConfig.locale || 'ko'),
                default_email: String(data.get('default_email') || ''),
                target_version: String(state.uiConfig.target_version || ''),
                release_channel: String(state.uiConfig.release_channel || 'stable')
            };
            state.uiConfig = await request('/api/ui/config', {
                method: 'PATCH',
                body: JSON.stringify(payload)
            });
            setMessage('ok', t('settings_saved'));
            await syncPageData();
            return;
        }
        if (formType === 'save-admin-update-settings') {
            const payload = {
                locale: String(state.uiConfig.locale || 'ko'),
                default_email: String(state.uiConfig.default_email || ''),
                target_version: String(data.get('target_version') || ''),
                release_channel: String(data.get('release_channel') || state.uiConfig.release_channel || 'stable')
            };
            state.uiConfig = await request('/api/ui/config', {
                method: 'PATCH',
                body: JSON.stringify(payload)
            });
            await loadSystemUpdate();
            setMessage('ok', t('update_settings_saved'));
            await syncPageData();
            return;
        }
    } catch (err) {
        setMessage('error', err.message);
        render();
    }
}

async function handleAction(action, dataset) {
    try {
        if (action === 'set-page') return setPage(dataset.page);
        if (action === 'set-tab') return setTab(dataset.tab);
        if (action === 'jump-tab') {
            state.activePage = dataset.page;
            state.activeTabByPage[dataset.page] = dataset.tab;
            syncRoute(false);
            return syncPageData();
        }
        if (action === 'refresh-vaults') {
            await loadVaults();
            return syncPageData();
        }
        if (action === 'select-vault') return selectVaultByKey(dataset.key);
        if (action === 'select-key') return selectKeyByName(dataset.key);
        if (action === 'audit-select-key') {
            state.auditKey = dataset.key;
            return syncPageData();
        }
        if (action === 'clear-audit-key') {
            state.auditKey = null;
            return syncPageData();
        }
        if (action === 'run-system-update') {
            await request('/api/system/update', {
                method: 'POST',
                body: JSON.stringify({})
            });
            await loadSystemUpdate();
            setMessage('ok', t('update_started'));
            return syncPageData();
        }
        if (action === 'new-key') {
            state.revealValue = false;
            if (state.vaultItemKind === 'VE') {
                state.selectedVaultItemKind = 'VE';
                state.selectedConfigKey = null;
                state.configDetail = null;
            } else {
                state.selectedVaultItemKind = 'VK';
                state.selectedKey = null;
                state.keyDetail = null;
                state.keySummary = null;
            }
            render();
            return;
        }
        if (action === 'set-vault-kind') {
            state.vaultItemKind = dataset.kind || 'VK';
            state.revealValue = false;
            if (activeTab() === 'HOST_VAULT') {
                if (state.vaultItemKind === 'VK') {
                    state.hostSelectedItemKind = 'VK';
                    state.hostSelectedConfigKey = null;
                    state.hostConfigDetail = null;
                    if (!state.hostSelectedKey && state.hostVaultKeys.length) {
                        state.hostSelectedKey = state.hostVaultKeys[0];
                    }
                } else if (state.vaultItemKind === 'VE') {
                    state.hostSelectedItemKind = 'VE';
                    state.hostKeyDetail = null;
                    if (!state.hostSelectedConfigKey && state.hostVaultConfigs.length) {
                        state.hostSelectedConfigKey = state.hostVaultConfigs[0].key;
                    }
                }
                return syncPageData();
            }
            if (state.vaultItemKind === 'VK') {
                state.selectedVaultItemKind = 'VK';
                state.selectedConfigKey = null;
                state.configDetail = null;
                if (!state.selectedKey && state.vaultKeys.length) {
                    state.selectedKey = state.vaultKeys[0];
                }
            } else if (state.vaultItemKind === 'VE') {
                state.selectedVaultItemKind = 'VE';
                state.keyDetail = null;
                state.keySummary = null;
                if (!state.selectedConfigKey && state.configVaultItems.length) {
                    state.selectedConfigKey = state.configVaultItems[0].key;
                }
            }
            return syncPageData();
        }
        if (action === 'set-bulk-apply-view') {
            state.bulkApplyView = dataset.view === 'workflow' ? 'workflow' : 'items';
            syncRoute(false);
            render();
            return;
        }
        if (action === 'select-bulk-template') {
            state.selectedBulkApplyTemplateName = dataset.key || null;
            render();
            return;
        }
        if (action === 'select-bulk-workflow') {
            state.selectedBulkApplyWorkflowName = dataset.key || null;
            render();
            return;
        }
        if (action === 'select-vault-item') {
            if ((dataset.kind || 'VK') === 'VE') {
                state.selectedVaultItemKind = 'VE';
                state.selectedConfigKey = dataset.key;
                state.revealValue = false;
                return syncPageData();
            }
            return selectKeyByName(dataset.key);
        }
        if (action === 'select-host-item') {
            if ((dataset.kind || 'VK') === 'VE') {
                state.hostSelectedItemKind = 'VE';
                state.hostSelectedConfigKey = dataset.key;
                return syncPageData();
            }
            state.hostSelectedItemKind = 'VK';
            state.hostSelectedKey = state.hostVaultKeys.find((item) => item.name === dataset.key) || { name: dataset.key };
            return syncPageData();
        }
        if (action === 'toggle-reveal') {
            state.revealValue = !state.revealValue;
            render();
            return;
        }
        if (action === 'copy-value') {
            const value = state.selectedGroupedEntry && state.selectedGroupedEntry.kind === 'VE'
                ? (state.configDetail?.value || '')
                : (state.configDetail && state.vaultItemKind === 'VE' ? (state.configDetail.value || '') : (state.keyDetail?.value || ''));
            if (value) {
                await navigator.clipboard.writeText(value);
                setMessage('ok', t('value_copied'));
                renderHeader();
            }
            return;
        }
        if (action === 'delete-key') {
            if (!state.selectedVault || !state.selectedKey) return;
            await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash) + '/keys/' + encodeURIComponent(state.selectedKey.name), {
                method: 'DELETE'
            });
            state.selectedKey = null;
            setMessage('ok', 'Key deleted.');
            return syncPageData();
        }
        if (action === 'delete-all-bindings') {
            if (!state.selectedVault || !state.selectedKey) return;
            await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash) + '/keys/' + encodeURIComponent(state.selectedKey.name) + '/bindings', {
                method: 'DELETE'
            });
            setMessage('ok', 'Bindings deleted.');
            return syncPageData();
        }
        if (action === 'select-config-match') return selectConfigMatch(dataset.vault, dataset.key);
        if (action === 'select-config-vault') {
            state.selectedConfigVault = dataset.key;
            state.selectedConfigKey = null;
            return syncPageData();
        }
        if (action === 'select-config-detail') {
            state.selectedConfigKey = dataset.key;
            return syncPageData();
        }
        if (action === 'delete-agent-config') {
            if (!state.selectedConfigVault || !state.selectedConfigKey) return;
            await request('/api/agents/' + encodeURIComponent(state.selectedConfigVault) + '/configs/' + encodeURIComponent(state.selectedConfigKey), {
                method: 'DELETE'
            });
            state.configDetail = null;
            setMessage('ok', 'Config deleted.');
            return syncPageData();
        }
        if (action === 'refresh-functions') {
            await loadFunctions();
            return syncPageData();
        }
        if (action === 'select-function') return selectFunctionByName(dataset.key);
        if (action === 'select-grouped-name') {
            const row = state.groupedRows.find((item) => item.name === dataset.key);
            state.selectedGroupedName = dataset.key;
            state.selectedGroupedEntries = row ? row.entries : [];
            state.selectedGroupedEntry = row && row.entries.length ? row.entries[0] : null;
            return syncPageData();
        }
        if (action === 'select-grouped-entry') {
            const row = state.groupedRows.find((item) => item.name === dataset.key);
            const entry = row ? row.entries.find((item) => item.vault_runtime_hash === dataset.vault && item.kind === dataset.kind) : null;
            state.selectedGroupedName = dataset.key;
            state.selectedGroupedEntries = row ? row.entries : [];
            state.selectedGroupedEntry = entry || null;
            return syncPageData();
        }
        if (action === 'delete-function-bindings') {
            if (!state.selectedFunction) return;
            await request('/api/targets/function/' + encodeURIComponent(state.selectedFunction.name) + '/bindings', {
                method: 'DELETE'
            });
            setMessage('ok', 'Function bindings deleted.');
            return syncPageData();
        }
        if (action === 'audit-page-select-vault') {
            state.auditVault = dataset.key;
            state.auditKey = null;
            await selectVaultByKey(dataset.key);
            return syncPageData();
        }
        if (action === 'audit-page-select-key') {
            state.auditKey = dataset.key;
            return syncPageData();
        }
        if (action === 'load-admin-audit') return loadAdminAudit().then(render);
    } catch (err) {
        setMessage('error', err.message);
        render();
    }
}

  function onSubmit(event) {
    const form = event.target.closest('form[data-form]');
    if (!form) return;
    event.preventDefault();
    handleFormSubmit(form);
  }

  function onClick(event) {
    const target = event.target.closest('[data-action]');
    if (!target) return;
    event.preventDefault();
    handleAction(target.dataset.action, target.dataset);
  }

  function onInput(event) {
    const target = event.target;
    if (!target || !target.id) return;
    if (!['global-search', 'vault-search', 'key-search', 'config-vault-search', 'function-search'].includes(target.id)) return;
    state.globalQuery = target.value;
    render();
  }

  function onPopState() {
    applyRoute(window.location.pathname, window.location.search);
    syncPageData();
  }

async function boot() {
    applyRoute(window.location.pathname, window.location.search);
    await loadStatus();
    await loadUIConfig();
    await loadVaults();
    await loadConfigsSummary();
    await loadFunctions();
    await loadTrackedRefAudit();
    await syncPageData();
    render();
}


  onMounted(() => {
    document.addEventListener('submit', onSubmit);
    document.addEventListener('click', onClick);
    document.addEventListener('input', onInput);
    window.addEventListener('popstate', onPopState);
    boot();
  });

  onUnmounted(() => {
    document.removeEventListener('submit', onSubmit);
    document.removeEventListener('click', onClick);
    document.removeEventListener('input', onInput);
    window.removeEventListener('popstate', onPopState);
  });

return {
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
  renderSyncStatus,
  vaultSyncStatus,
  vaultDistributionStatus,
  vaultKeyClassStatus,
  selectedBulkApplyTemplate,
  selectedBulkApplyWorkflow,
                renderConfigRelations,
                configRelationsByScope,
                encodeURIComponent
    };
}
