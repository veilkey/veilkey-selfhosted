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
        keycenter: 'TEMP_REFS',
        vaults: 'ALL_VAULTS',
        functions: 'FUNCTION_LIST',
        audit: 'AUDIT_LOG',
        settings: 'UI'
    },
    message: null,
    status: null,
    uiConfig: null,
    systemUpdate: null,
    authSettings: null,
    totpEnrollStep: null,
    totpSecret: null,
    totpOtpauthURI: null,
    recoveryCodes: [],
    recoveryCopied: false,
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
    auditCountByVault: {},
    trackedRefAudit: null,
    adminAuditRows: [],
    keycenterTempRefs: [],
    selectedTempRef: null,
    revealedValues: {},
    routeSelectedRefCanonical: null,
    regTokens: [],
    createdRegToken: null,
    showRegTokenForm: false,
    showTempRefForm: false,
    passkeys: [],
    passkeyRegistering: false,
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
        twoPane: false,
        adminRequired: false,
        adminSetupRequired: false,
        adminSetupError: '',
        adminLoginError: '',
        locked: false,
        unlockError: '',
        unlockPassword: ''
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
        TEMP_REFS: 'tab_temp_refs',
        ALL_VAULTS: 'tab_all_vaults',
        VAULT_ITEMS: 'tab_vault_items',
        BULK_APPLY: 'tab_bulk_apply',
        FUNCTION_LIST: 'tab_function_list',
        FUNCTION_BINDINGS: 'tab_function_bindings',
        FUNCTION_IMPACT: 'tab_function_impact',
        FUNCTION_RUN: 'tab_function_run',
        AUDIT_LOG: 'tab_audit_log',
        UI: 'tab_ui',
        SECURITY: 'tab_security',
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
    if (page === 'audit' && tab === 'AUDIT_LOG') {
        const vaultHash = state.auditVault;
        return vaultHash ? `/audit/${encodeURIComponent(vaultHash)}` : '/audit';
    }
    if (page === 'keycenter') {
        const canonical = state.selectedTempRef?.ref_canonical;
        return canonical ? `/keycenter/${encodeURIComponent(canonical)}` : '/keycenter';
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
    const auditMatch = normalized.match(/^\/audit(?:\/([^/]+))?$/);
    if (auditMatch) {
        state.activePage = 'audit';
        state.activeTabByPage.audit = 'AUDIT_LOG';
        if (auditMatch[1]) {
            state.auditVault = decodeURIComponent(auditMatch[1]);
        }
        return;
    }
    const keycenterMatch = normalized.match(/^\/keycenter(?:\/(.+))?$/);
    if (keycenterMatch) {
        state.activePage = 'keycenter';
        state.activeTabByPage.keycenter = 'TEMP_REFS';
        state.routeSelectedRefCanonical = keycenterMatch[1] ? decodeURIComponent(keycenterMatch[1]) : null;
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
    if (page === 'keycenter') return state.keycenterTempRefs.length || '';
    if (page === 'vaults') return state.vaults.length;
    if (page === 'functions') return state.functions.length;
    if (page === 'audit') return auditTotalCount();
    if (page === 'settings') return state.uiConfig ? 3 : 0;
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

function vaultTargetOptions() {
    const options = [];
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
        { page: 'keycenter', label: pageLabel('keycenter') },
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
    if (state.activePage === 'keycenter') return t('tab_temp_refs');
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
                </div>
            </div>
            <div class="sidebar-section">
                <div class="sidebar-label">${escapeHTML(t('section_local_vaults'))}</div>
                <div class="nav-list">
                    ${vaults.map((item) => `
                        <a
                            href="/vaults/local/${encodeURIComponent(item.vault_runtime_hash)}"
                            class="nav-item${state.selectedVault && item.vault_runtime_hash === state.selectedVault.vault_runtime_hash ? ' active' : ''}"
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
                            href="/audit/${encodeURIComponent(item.vault_runtime_hash)}"
                            class="nav-item${state.auditVault === item.vault_runtime_hash ? ' active' : ''}"
                            data-action="audit-page-select-vault"
                            data-key="${escapeHTML(item.vault_runtime_hash)}"
                        >
                            <span class="nav-item-main">
                                <span>${escapeHTML(item.display_name || item.vault_name || item.vault_runtime_hash)}</span>
                            </span>
                            <span class="status-pill count-pill">${auditVaultCount(item)}</span>
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
    return renderVaultKeys();
}

function filteredVaults() {
    return state.vaults.filter((vault) => matchesQuery(vault.display_name || vault.vault_name || vault.vault_runtime_hash));
}

function renderVaultInventory() {
    const vaults = filteredVaults();
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
                <span class="pill">${vaults.length} ${escapeHTML(t('count_rows'))}</span>
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
        ], vaults, (row) => {
            const classes = [];
            if (state.selectedVault && row.vault_runtime_hash === state.selectedVault.vault_runtime_hash) classes.push('is-selected');
            classes.push('is-clickable');
            return classes.join(' ');
        }, (row) => `data-action="select-vault" data-key="${escapeHTML(row.vault_runtime_hash)}"`)
    );

    const detail = state.vaultDetail || selectedVaultRecord();
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
                        </div>
                        <div class="muted">${escapeHTML(isConfigItem ? t('vault_config_move_help') : t('vault_key_move_help'))}</div>
                    </div>
                    <button class="btn btn-soft" type="submit"${detailName ? '' : ' disabled'}>${escapeHTML(isConfigItem ? t('move_config') : t('move_key'))}</button>
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
    return filteredVaults();
}

function currentVaultSelectedKind() {
    return state.vaultItemKind === 'VE'
        ? 'VE'
        : state.vaultItemKind === 'VK'
            ? 'VK'
            : state.selectedVaultItemKind;
}

function currentVaultSelectedName() {
    const kind = currentVaultSelectedKind();
    return kind === 'VE' ? state.selectedConfigKey : state.selectedKey?.name;
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
    if (activeTab() === 'VAULT_ITEMS') return localVaultVisibleRows();
    return [];
}

function vaultItemIdentifier(row) {
    if (row.item_kind === 'VE') return row.value || row.name || row.key || row.ref || '-';
    if (row.token) return row.token;
    if (row.ref) return `VK:${row.scope || 'LOCAL'}:${row.ref}`;
    return row.name || '-';
}

function vaultCenterTitle() {
    if (activeTab() === 'ALL_VAULTS') return t('vault_inventory');
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
    const isConfigItem = currentVaultSelectedKind() === 'VE';
    const detail = isConfigItem ? state.configDetail : state.keyDetail;
    const detailName = isConfigItem ? state.selectedConfigKey : state.selectedKey?.name;
    const visibleValue = detail?.value || '';
    const keySummary = !isConfigItem ? state.keySummary : null;
    const keyRecord = !isConfigItem ? (keySummary && keySummary.key ? keySummary.key : state.selectedKey) : null;

    return {
        isConfigItem,
        detail,
        detailName,
        visibleValue,
        canMoveItem: Boolean(detailName || visibleValue || (isConfigItem ? detail?.key : detail?.name)),
        targetVaultDefault: state.selectedVault ? state.selectedVault.vault_runtime_hash : '',
        currentScope: detail?.scope || (isConfigItem ? 'LOCAL' : 'TEMP'),
        scopeOptions: isConfigItem ? ['LOCAL', 'EXTERNAL', 'TEMP'] : ['TEMP', 'LOCAL', 'EXTERNAL'],
        moveHelperText: isConfigItem ? t('vault_config_move_help') : t('vault_key_move_help'),
        saveForm: isConfigItem ? 'save-agent-config' : 'save-key',
        createTitle: isConfigItem ? t('new_config') : t('new_key'),
        showScopeSelect: false,
        showDelete: Boolean(detailName),
        deleteAction: isConfigItem ? 'delete-agent-config' : 'delete-key',
        showKeyMeta: !isConfigItem,
        description: keyRecord && keyRecord.description ? keyRecord.description : '',
        tagsJSON: keyRecord && keyRecord.tags_json ? keyRecord.tags_json : '[]',
        configStatus: isConfigItem ? (detail?.status || 'active') : '',
        itemRefValue: isConfigItem
            ? (detailName || detail?.key || '')
            : (detail?.token || (detail?.ref ? `VK:${detail.scope || 'LOCAL'}:${detail.ref}` : '')),
        bindingsTotal: keySummary ? (keySummary.bindings_total || keySummary.bindings_count || 0) : 0,
        usageCount: keySummary ? (keySummary.usage_count || 0) : 0,
        bindings: keySummary && keySummary.bindings ? keySummary.bindings.slice(0, 5) : [],
        recentAudit: keySummary && keySummary.recent_audit ? keySummary.recent_audit.slice(0, 3) : [],
        status: detail?.status || 'active'
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
    if (activeTab() === 'ADMIN') return t('admin_settings');
    if (activeTab() === 'SECURITY') return t('security_title');
    return t('ui_settings');
}

function settingsRightPaneTitle() {
    if (activeTab() === 'ADMIN') return t('admin_setting_detail');
    if (activeTab() === 'SECURITY') return t('security_title');
    return t('edit_ui_config');
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

function renderAuditPage() {
    state.ui.leftHTML = '';
    state.ui.leftVisible = false;
    state.ui.twoPane = true;
    renderSecondarySidebar();

    const rows = state.auditRows;
    const selected = state.selectedAuditRow;
    const vault = state.vaults.find((v) => v.vault_runtime_hash === state.auditVault) || null;

    state.ui.centerHTML = `
        <div class="pane-header">
            <div class="pane-title"><strong>${escapeHTML(t('tab_audit_log'))}</strong></div>
            <div class="toolbar">
                <span class="pill">${rows.length} ${escapeHTML(t('count_events'))}</span>
            </div>
        </div>
        <div class="pane-content">
            <div class="table-wrap">
                <table>
                    <thead><tr>
                        <th>${escapeHTML(t('time'))}</th>
                        <th>${escapeHTML(t('action'))}</th>
                        <th>${escapeHTML(t('actor'))}</th>
                        <th>${escapeHTML(t('target'))}</th>
                    </tr></thead>
                    <tbody>
                        ${rows.length ? rows.map((row, i) => `
                            <tr class="is-clickable${selected && selected.event_id === row.event_id ? ' is-selected' : ''}" data-action="select-audit-row" data-index="${i}">
                                <td>${escapeHTML(row.created_at || row.timestamp || '-')}</td>
                                <td>${escapeHTML(row.action || '-')}</td>
                                <td>${escapeHTML(row.actor_type || row.actor_id || '-')}</td>
                                <td>${escapeHTML(row.entity_id || '-')}</td>
                            </tr>
                        `).join('') : `<tr><td colspan="4"><div class="empty">${escapeHTML(t('empty_no_rows'))}</div></td></tr>`}
                    </tbody>
                </table>
            </div>
        </div>`;

    const detail = selected || (rows.length ? rows[0] : null);
    const vaultInfo = vault ? `
        <div class="card" style="margin-top:12px">
            <div class="card-title">${escapeHTML(vault.display_name || vault.vault_name)}</div>
            <div class="inline-grid">
                <div class="kv"><span class="label">status</span><span class="value">${escapeHTML(vault.status || '-')}</span></div>
                <div class="kv"><span class="label">secrets</span><span class="value">${vault.secrets_count || 0}</span></div>
                <div class="kv"><span class="label">configs</span><span class="value">${vault.configs_count || 0}</span></div>
                <div class="kv"><span class="label">IP</span><span class="value">${escapeHTML(vault.ip || '-')}</span></div>
                <div class="kv"><span class="label">role</span><span class="value">${escapeHTML(vault.agent_role || '-')}</span></div>
                <div class="kv"><span class="label">last seen</span><span class="value">${escapeHTML(vault.last_seen || '-')}</span></div>
                <div class="kv"><span class="label">version</span><span class="value">${vault.version || '-'}</span></div>
            </div>
        </div>` : '';

    state.ui.rightHTML = `
        <div class="pane-header">
            <div class="pane-title"><strong>${escapeHTML(t('audit_event_detail'))}</strong></div>
        </div>
        <div class="pane-content">
            ${detail ? `
                <div class="card">
                    <div class="card-title">${escapeHTML(detail.action || 'event')} · ${escapeHTML(detail.entity_type || '-')}</div>
                    <pre class="code">${escapeHTML(formatJSON(detail))}</pre>
                </div>
            ` : `<div class="empty">${escapeHTML(t('select_vault_for_detail'))}</div>`}
            ${vaultInfo}
        </div>`;
}

function render() {
    renderTopbarStatus();
    renderSidebar();
    renderHeader();
    if (state.activePage === 'vaults') {
        syncVaultVuePanels();
        return;
    }
    if (state.activePage === 'functions' || state.activePage === 'settings') {
        syncTemplatePageLayout();
        return;
    }
    if (state.activePage === 'audit') {
        renderAuditPage();
        return;
    }
    if (state.activePage === 'keycenter') {
        renderKeycenterPage();
        return;
    }
    renderSecondarySidebar();
    if (state.activePage === 'configs') renderConfigs();
}

function renderKeycenterPage() {
    state.ui.leftHTML = '';
    state.ui.leftVisible = false;
    state.ui.twoPane = true;
    state.ui.secondarySidebarHidden = true;

    const refs = state.keycenterTempRefs;
    const selected = state.selectedTempRef;

    const fmtAbs = (iso) => {
        if (!iso) return '-';
        try { return new Date(iso).toLocaleString(); } catch { return iso; }
    };
    const fmtRemaining = (iso) => {
        if (!iso) return null;
        const ms = new Date(iso) - Date.now();
        if (ms <= 0) return t('keycenter_expired');
        const mins = Math.round(ms / 60000);
        if (mins < 60) return `${mins}${t('keycenter_min_left')}`;
        const hrs = Math.floor(mins / 60);
        const rem = mins % 60;
        return rem > 0 ? `${hrs}${t('keycenter_hr')} ${rem}${t('keycenter_min_left')}` : `${hrs}${t('keycenter_hr_left')}`;
    };

    // Center: list
    state.ui.centerHTML = `
        <div class="pane-header">
            <div class="pane-title"><strong>${escapeHTML(t('keycenter_temp_refs_title'))}</strong></div>
            <div class="toolbar">
                <span class="pill">${refs.length}</span>
                <button class="btn btn-soft" data-action="show-create-temp-ref" style="margin-left:8px;font-size:0.8rem">+ 임시키</button>
                <button class="btn btn-soft" data-action="show-create-reg-token" style="margin-left:8px;font-size:0.8rem">+ 등록 토큰</button>
            </div>
        </div>
        <div class="pane-content">
            <div class="table-wrap">
                <table>
                    <thead><tr>
                        <th>${escapeHTML(t('keycenter_secret_name'))}</th>
                        <th>Value</th>
                        <th>${escapeHTML(t('keycenter_expires_at'))}</th>
                    </tr></thead>
                    <tbody>
                        ${refs.length ? refs.map((ref, i) => {
                            const remaining = fmtRemaining(ref.expires_at);
                            const rv = state.revealedValues[ref.ref_canonical];
                            const isRev = rv !== undefined;
                            return `
                            <tr class="is-clickable${selected && selected.ref_canonical === ref.ref_canonical ? ' is-selected' : ''}"
                                data-action="select-temp-ref" data-index="${i}">
                                <td><strong>${escapeHTML(ref.secret_name || '-')}</strong></td>
                                <td style="white-space:nowrap">
                                    <code style="font-size:0.8rem;color:#c8f0a0">${isRev ? escapeHTML(rv) : '••••••••'}</code>
                                    <button class="action-btn" style="font-size:0.75rem;padding:1px 6px;margin-left:4px"
                                        data-action="toggle-reveal-temp-ref"
                                        data-canonical="${escapeHTML(ref.ref_canonical)}"
                                        onclick="event.stopPropagation()">
                                        ${isRev ? '🙈' : '👁'}
                                    </button>
                                </td>
                                <td>
                                    ${remaining ? `<span style="color:#e0a040;font-weight:500">${escapeHTML(remaining)}</span>` : '-'}
                                </td>
                            </tr>`;
                        }).join('') : `<tr><td colspan="3"><div class="empty">${escapeHTML(t('keycenter_no_temp_refs'))}</div></td></tr>`}
                    </tbody>
                </table>
            </div>

            ${state.vaults.length ? `
            <div style="margin-top:20px">
                <div class="pane-title" style="margin-bottom:8px"><strong>${escapeHTML(t('page_vaults'))}</strong></div>
                <div class="table-wrap">
                    <table>
                        <thead><tr>
                            <th>${escapeHTML(t('name'))}</th>
                            <th>status</th>
                            <th></th>
                        </tr></thead>
                        <tbody>
                            ${state.vaults.map(v => `
                                <tr>
                                    <td>${escapeHTML(v.display_name || v.vault_name || v.vault_runtime_hash)}</td>
                                    <td>${renderStatusPill(v.status || 'active', statusClass(v.status || 'active'))}</td>
                                    <td><button class="action-btn" style="font-size:0.8rem;padding:3px 10px"
                                        data-action="navigate-to-vault"
                                        data-key="${escapeHTML(v.vault_runtime_hash)}">
                                        ${escapeHTML(t('keycenter_goto_vault'))} →
                                    </button></td>
                                </tr>
                            `).join('')}
                        </tbody>
                    </table>
                </div>
            </div>` : ''}

            ${state.showTempRefForm ? `
            <div style="margin-top:20px;padding:12px;border:1px solid #444;border-radius:6px">
                <div class="pane-title" style="margin-bottom:8px"><strong>임시키 등록</strong></div>
                <form data-action="create-temp-ref-form">
                    <div style="margin-bottom:8px">
                        <label style="font-size:0.8rem;color:#999">키 이름</label>
                        <input class="form-input" type="text" name="name" placeholder="MY_SECRET_KEY" style="width:100%" required/>
                    </div>
                    <div style="margin-bottom:8px">
                        <label style="font-size:0.8rem;color:#999">값</label>
                        <input class="form-input" type="password" name="value" placeholder="비밀번호 또는 키 값" style="width:100%" required/>
                    </div>
                    <button type="submit" class="btn btn-primary" style="width:100%">등록 (1시간 후 만료)</button>
                </form>
            </div>` : ''}

            ${state.showRegTokenForm ? `
            <div style="margin-top:20px;padding:12px;border:1px solid #444;border-radius:6px">
                <div class="pane-title" style="margin-bottom:8px"><strong>등록 토큰 발급</strong></div>
                <form data-action="create-reg-token-form">
                    <div style="margin-bottom:8px">
                        <label style="font-size:0.8rem;color:#999">라벨 (선택)</label>
                        <input class="form-input" type="text" name="label" placeholder="my-vault-01" style="width:100%"/>
                    </div>
                    <div style="margin-bottom:8px">
                        <label style="font-size:0.8rem;color:#999">만료</label>
                        <select class="form-input" name="expires" style="width:100%">
                            <option value="60">1시간</option>
                            <option value="360">6시간</option>
                            <option value="1440" selected>24시간</option>
                            <option value="10080">7일</option>
                        </select>
                    </div>
                    <button type="submit" class="btn btn-primary" style="width:100%">토큰 생성</button>
                </form>
                ${state.createdRegToken ? `
                <div style="margin-top:12px;padding:8px;background:#1a2a1a;border-radius:4px">
                    <div style="font-size:0.75rem;color:#999;margin-bottom:4px">생성된 토큰</div>
                    <code style="font-size:0.7rem;word-break:break-all;color:#c8f0a0">${escapeHTML(state.createdRegToken.token)}</code>
                    <div style="margin-top:8px">
                        <button class="btn btn-soft" data-action="copy-reg-token" style="font-size:0.75rem">복사</button>
                    </div>
                    <div style="margin-top:8px;font-size:0.7rem;color:#888">
                        <code>${escapeHTML(state.createdRegToken.command)}</code>
                    </div>
                </div>` : ''}
            </div>` : ''}

            ${state.regTokens.length ? `
            <div style="margin-top:20px">
                <div class="pane-title" style="margin-bottom:8px"><strong>등록 토큰 목록</strong></div>
                <div class="table-wrap">
                    <table>
                        <thead><tr><th>라벨</th><th>상태</th><th>만료</th><th></th></tr></thead>
                        <tbody>
                            ${state.regTokens.map(tk => {
                                const statusColor = tk.status === 'active' ? '#a8f0a0' : tk.status === 'used' ? '#888' : '#f0a0a0';
                                return `<tr>
                                    <td>${escapeHTML(tk.label || '-')}</td>
                                    <td><span style="color:${statusColor}">${escapeHTML(tk.status)}</span></td>
                                    <td style="font-size:0.8rem">${fmtRemaining(tk.expires_at) || fmtAbs(tk.expires_at)}</td>
                                    <td>${tk.status === 'active' ? `<button class="btn btn-soft" data-action="revoke-reg-token" data-token-id="${escapeHTML(tk.token_id)}" style="font-size:0.7rem">폐기</button>` : ''}</td>
                                </tr>`;
                            }).join('')}
                        </tbody>
                    </table>
                </div>
            </div>` : ''}
        </div>`;

    // Right: detail
    if (selected) {
        const linkedVault = selected.agent_hash
            ? state.vaults.find(v => v.vault_runtime_hash === selected.agent_hash)
            : null;
        const remaining = fmtRemaining(selected.expires_at);
        const revealedValue = state.revealedValues[selected.ref_canonical];
        const isRevealed = revealedValue !== undefined;
        const displayValue = isRevealed ? revealedValue : '••••••••';

        state.ui.rightHTML = `
            <div class="pane-header">
                <div class="pane-title"><strong>${escapeHTML(selected.secret_name || selected.ref_canonical)}</strong></div>
            </div>
            <div class="pane-content">
                <div class="card">
                    <div class="card-title">Ref</div>
                    <div style="margin-bottom:12px">
                        <code style="font-size:0.82rem;word-break:break-all;color:#a8d0ff">${escapeHTML(selected.ref_canonical)}</code>
                    </div>
                    <div class="card-title">Value</div>
                    <div style="display:flex;align-items:center;gap:8px;margin-bottom:12px;flex-wrap:wrap">
                        <code style="font-size:0.82rem;word-break:break-all;flex:1;color:#c8f0a0">${isRevealed ? escapeHTML(displayValue) : '••••••••'}</code>
                        <button class="action-btn" data-action="toggle-reveal-temp-ref" style="white-space:nowrap">
                            ${isRevealed ? t('hide') : t('reveal')}
                        </button>
                    </div>
                    <div class="inline-grid">
                        <div class="kv"><span class="label">${escapeHTML(t('keycenter_secret_name'))}</span><span class="value">${escapeHTML(selected.secret_name || '-')}</span></div>
                        <div class="kv"><span class="label">status</span><span class="value">${escapeHTML(selected.status || '-')}</span></div>
                        <div class="kv">
                            <span class="label">${escapeHTML(t('keycenter_expires_at'))}</span>
                            <span class="value">
                                <span style="color:#e0a040">${remaining ? escapeHTML(remaining) : '-'}</span>
                                <span style="color:#8a8fa8;font-size:0.8rem;margin-left:6px">${escapeHTML(fmtAbs(selected.expires_at))}</span>
                            </span>
                        </div>
                        <div class="kv"><span class="label">${escapeHTML(t('keycenter_created_at'))}</span><span class="value">${escapeHTML(fmtAbs(selected.created_at))}</span></div>
                    </div>
                    <div style="margin-top:12px;display:flex;gap:8px;flex-wrap:wrap">
                        <button class="action-btn" data-action="copy-temp-ref" data-ref="${escapeHTML(selected.ref_canonical)}">${escapeHTML(t('copy'))} Ref</button>
                        ${linkedVault ? `
                            <button class="action-btn"
                                data-action="navigate-to-vault"
                                data-key="${escapeHTML(linkedVault.vault_runtime_hash)}">
                                ${escapeHTML(t('keycenter_goto_vault'))} →
                            </button>
                        ` : `
                            <button class="action-btn" data-action="set-page" data-page="vaults">
                                ${escapeHTML(t('keycenter_goto_vault_list'))} →
                            </button>
                        `}
                    </div>
                    ${state.vaults.length ? `
                    <div style="margin-top:16px;padding:12px;border:1px solid #444;border-radius:6px">
                        <div style="font-size:0.8rem;color:#999;margin-bottom:6px">볼트에 저장 (격상)</div>
                        <div style="display:flex;gap:6px;align-items:center">
                            <select class="form-input" id="promote-vault-select" style="flex:1;font-size:0.8rem">
                                ${state.vaults.map(v => `<option value="${escapeHTML(v.vault_runtime_hash)}">${escapeHTML(v.display_name || v.vault_name)}</option>`).join('')}
                            </select>
                            <button class="btn btn-primary" data-action="promote-to-vault"
                                data-ref="${escapeHTML(selected.ref_canonical)}"
                                data-name="${escapeHTML(selected.secret_name || '')}"
                                style="font-size:0.8rem;white-space:nowrap">저장</button>
                        </div>
                    </div>` : ''}
                </div>
            </div>`;
    } else {
        state.ui.rightHTML = `
            <div class="pane-header"><div class="pane-title"><strong>${escapeHTML(t('keycenter_ref'))}</strong></div></div>
            <div class="pane-content"><div class="empty">${escapeHTML(t('keycenter_select_ref'))}</div></div>`;
    }
}

async function loadKeycenterTempRefs() {
    try {
        const data = await request('/api/keycenter/temp-refs');
        state.keycenterTempRefs = data.refs || [];
        // Auto-select ref from URL if set
        if (state.routeSelectedRefCanonical) {
            const found = state.keycenterTempRefs.find(r => r.ref_canonical === state.routeSelectedRefCanonical);
            if (found) state.selectedTempRef = found;
            state.routeSelectedRefCanonical = null;
        }
    } catch (err) {
        state.keycenterTempRefs = [];
    }
}

async function loadRegTokens() {
    try {
        const data = await request('/api/admin/registration-tokens');
        state.regTokens = data.tokens || [];
    } catch (err) {
        state.regTokens = [];
    }
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

async function loadAuthSettings() {
    try {
        state.authSettings = await request('/api/admin/auth/settings');
    } catch (err) {
        // Auth settings may not be available if not authenticated
    }
}

async function startTOTPEnroll() {
    try {
        const data = await request('/api/admin/auth/totp/enroll/start', { method: 'POST' });
        state.totpSecret = data.secret;
        state.totpOtpauthURI = data.otpauth_uri;
        state.totpEnrollStep = 'qr';
        state.recoveryCodes = [];
        state.recoveryCopied = false;
        render();
    } catch (err) {
        setMessage('error', err.message);
    }
}

async function verifyTOTPEnroll(code) {
    try {
        await request('/api/admin/auth/totp/enroll/verify', {
            method: 'POST',
            body: JSON.stringify({ code })
        });
        // Generate recovery codes (client-side, backed by crypto.getRandomValues)
        state.recoveryCodes = Array.from({ length: 5 }, () => {
            const bytes = new Uint8Array(32);
            crypto.getRandomValues(bytes);
            return Array.from(bytes).map(b => b.toString(16).padStart(2, '0')).join('');
        });
        state.totpEnrollStep = 'recovery';
        state.authSettings = await request('/api/admin/auth/settings');
        render();
    } catch (err) {
        setMessage('error', err.message);
    }
}

function copyRecoveryCodes() {
    const text = state.recoveryCodes.join('\n');
    navigator.clipboard.writeText(text).then(() => {
        state.recoveryCopied = true;
        render();
    });
}

function downloadRecoveryCodes() {
    const text = state.recoveryCodes.join('\n');
    const blob = new Blob([text], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'veilkey-recovery-codes.txt';
    a.click();
    URL.revokeObjectURL(url);
}

function printRecoveryCodes() {
    const text = state.recoveryCodes.join('\n');
    const win = window.open('', '_blank');
    win.document.write('<pre style="font-size:14px;line-height:2">' + text + '</pre>');
    win.document.close();
    win.print();
}

function finishTOTPEnroll() {
    state.totpEnrollStep = null;
    state.totpSecret = null;
    state.totpOtpauthURI = null;
    state.recoveryCodes = [];
    state.recoveryCopied = false;
    render();
}

// --- Passkey functions ---

function base64urlToBuffer(base64url) {
    const base64 = base64url.replace(/-/g, '+').replace(/_/g, '/');
    const pad = base64.length % 4 === 0 ? '' : '='.repeat(4 - base64.length % 4);
    const binary = atob(base64 + pad);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    return bytes.buffer;
}

function bufferToBase64url(buffer) {
    const bytes = new Uint8Array(buffer);
    let binary = '';
    for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i]);
    return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
}

async function loadPasskeys() {
    try {
        const data = await request('/api/admin/auth/passkeys');
        state.passkeys = data.passkeys || [];
    } catch (err) { /* ignore */ }
}

async function registerPasskey() {
    state.passkeyRegistering = true;
    render();
    try {
        const options = await request('/api/admin/auth/passkey/register/begin', { method: 'POST' });
        const publicKey = {
            ...options.publicKey,
            challenge: base64urlToBuffer(options.publicKey.challenge),
            user: {
                ...options.publicKey.user,
                id: base64urlToBuffer(options.publicKey.user.id)
            },
            excludeCredentials: (options.publicKey.excludeCredentials || []).map(c => ({
                ...c,
                id: base64urlToBuffer(c.id)
            }))
        };
        const credential = await navigator.credentials.create({ publicKey });
        const body = {
            id: credential.id,
            rawId: bufferToBase64url(credential.rawId),
            type: credential.type,
            response: {
                clientDataJSON: bufferToBase64url(credential.response.clientDataJSON),
                attestationObject: bufferToBase64url(credential.response.attestationObject)
            },
            name: 'Passkey ' + new Date().toLocaleDateString()
        };
        await request('/api/admin/auth/passkey/register/finish', {
            method: 'POST',
            body: JSON.stringify(body)
        });
        await loadPasskeys();
        await loadAuthSettings();
        setMessage('ok', t('security_passkey_register_success'));
    } catch (err) {
        if (err.name !== 'NotAllowedError') {
            setMessage('error', err.message);
        }
    } finally {
        state.passkeyRegistering = false;
        render();
    }
}

async function deletePasskey(credentialId) {
    try {
        await request('/api/admin/auth/passkeys/' + encodeURIComponent(credentialId), { method: 'DELETE' });
        await loadPasskeys();
        await loadAuthSettings();
        setMessage('ok', t('security_passkey_deleted'));
    } catch (err) {
        setMessage('error', err.message);
    }
}

async function loginWithPasskey() {
    try {
        const options = await request('/api/admin/auth/passkey/login/begin', { method: 'POST' });
        const publicKey = {
            ...options.publicKey,
            challenge: base64urlToBuffer(options.publicKey.challenge),
            allowCredentials: (options.publicKey.allowCredentials || []).map(c => ({
                ...c,
                id: base64urlToBuffer(c.id)
            }))
        };
        const credential = await navigator.credentials.get({ publicKey });
        const body = {
            session_key: options.session_key,
            id: credential.id,
            rawId: bufferToBase64url(credential.rawId),
            type: credential.type,
            response: {
                clientDataJSON: bufferToBase64url(credential.response.clientDataJSON),
                authenticatorData: bufferToBase64url(credential.response.authenticatorData),
                signature: bufferToBase64url(credential.response.signature)
            }
        };
        await request('/api/admin/auth/passkey/login/finish', {
            method: 'POST',
            body: JSON.stringify(body)
        });
        state.ui.adminRequired = false;
        state.ui.adminLoginError = '';
        await loadStatus();
        if (state.status?.locked) {
            state.ui.locked = true;
            return;
        }
        await loadUIConfig();
        await loadVaults();
        await loadConfigsSummary();
        await loadFunctions();
        await loadTrackedRefAudit();
        await loadKeycenterTempRefs();
        await syncPageData();
        render();
    } catch (err) {
        if (err.name !== 'NotAllowedError') {
            state.ui.adminLoginError = err.message || 'Passkey login failed.';
        }
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

async function loadAuditCountsPerVault() {
    const counts = {};
    await Promise.all(state.vaults.map(async (v) => {
        try {
            const data = await request('/api/vaults/' + encodeURIComponent(v.vault_runtime_hash) + '/audit?limit=1');
            counts[v.vault_runtime_hash] = data.total_count || 0;
        } catch {
            counts[v.vault_runtime_hash] = 0;
        }
    }));
    state.auditCountByVault = counts;
}

function auditVaultCount(vault) {
    return state.auditCountByVault[vault.vault_runtime_hash] || 0;
}

function auditTotalCount() {
    return Object.values(state.auditCountByVault).reduce((sum, n) => sum + n, 0);
}

function auditSelectedVault() {
    return state.auditVault;
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
        if (state.activePage === 'keycenter') {
            await loadKeycenterTempRefs();
        } else if (state.activePage === 'vaults') {
            if (!state.vaults.length) await loadVaults();
            if (state.selectedVault) {
                await loadSelectedVaultDetail();
                if (activeTab() === 'BULK_APPLY') {
                    await Promise.all([loadBulkApplyTemplates(), loadBulkApplyWorkflows()]);
                } else {
                    await loadSelectedVaultKeys();
                    state.selectedConfigVault = state.selectedVault.vault_runtime_hash;
                    await loadConfigsForVault();
                    await loadVaultItemSyncStatus();
                }
                if (activeTab() === 'VAULT_ITEMS') {
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
            await loadAuditCountsPerVault();
            if (!state.auditVault && state.vaults.length) state.auditVault = state.vaults[0].vault_runtime_hash;
            await loadAuditVaultFeed();
        } else if (state.activePage === 'settings') {
            await loadUIConfig();
            await loadSystemUpdate();
            await loadAuthSettings();
            await loadPasskeys();
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
            const targetVault = String(data.get('target_vault') || '').trim();
            const name = String(data.get('name') || '').trim() || (state.selectedKey?.name || state.keyDetail?.name || '');
            const value = state.keyDetail?.value || '';
            if (!targetVault || !name || !value) throw new Error(t('promote_key_missing'));
            await request('/api/vaults/' + encodeURIComponent(targetVault) + '/keys', {
                method: 'POST',
                body: JSON.stringify({ name, value })
            });
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
        if (formType === 'promote-config') {
            const targetVault = String(data.get('target_vault') || '').trim();
            const key = String(data.get('key') || '').trim() || (state.selectedConfigKey || state.configDetail?.key || '');
            const value = state.configDetail?.value || '';
            if (!targetVault || !key || !value) throw new Error(t('promote_config_missing'));
            const payload = { key, value, scope: 'LOCAL', status: 'active' };
            await request('/api/agents/' + encodeURIComponent(targetVault) + '/configs', {
                method: 'POST',
                body: JSON.stringify(payload)
            });
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
        if (formType === 'verify-totp-enroll') {
            const code = String(data.get('code') || '').trim();
            if (code.length !== 6) {
                setMessage('error', 'Enter a 6-digit code');
                return;
            }
            await verifyTOTPEnroll(code);
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
        if (action === 'archive-vault') {
            await apiFetch(`/api/agents/by-node/${dataset.key}/archive`, { method: 'POST' });
            return syncPageData();
        }
        if (action === 'unarchive-vault') {
            await apiFetch(`/api/agents/by-node/${dataset.key}/unarchive`, { method: 'POST' });
            return syncPageData();
        }
        if (action === 'select-key') return selectKeyByName(dataset.key);
        if (action === 'audit-select-key') {
            state.auditKey = dataset.key;
            return syncPageData();
        }
        if (action === 'clear-audit-key') {
            state.auditKey = null;
            return syncPageData();
        }
        if (action === 'start-totp-enroll') {
            return startTOTPEnroll();
        }
        if (action === 'finish-totp-enroll') {
            return finishTOTPEnroll();
        }
        if (action === 'copy-recovery-codes') {
            return copyRecoveryCodes();
        }
        if (action === 'download-recovery-codes') {
            return downloadRecoveryCodes();
        }
        if (action === 'print-recovery-codes') {
            return printRecoveryCodes();
        }
        if (action === 'register-passkey') {
            return registerPasskey();
        }
        if (action === 'delete-passkey') {
            return deletePasskey(dataset.id);
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
            syncRoute(false);
            return syncPageData();
        }
        if (action === 'select-audit-row') {
            const idx = parseInt(dataset.index, 10);
            state.selectedAuditRow = state.auditRows[idx] || null;
            render();
            return;
        }
        if (action === 'audit-page-select-key') {
            state.auditKey = dataset.key;
            return syncPageData();
        }
        if (action === 'load-admin-audit') return loadAdminAudit().then(render);
        if (action === 'select-temp-ref') {
            const idx = parseInt(dataset.index, 10);
            state.selectedTempRef = state.keycenterTempRefs[idx] || null;
            syncRoute(false);
            render();
            return;
        }
        if (action === 'toggle-reveal-temp-ref') {
            const canonical = dataset.canonical || state.selectedTempRef?.ref_canonical;
            if (!canonical) return;
            if (state.revealedValues[canonical] !== undefined) {
                // already fetched — toggle off
                const next = { ...state.revealedValues };
                delete next[canonical];
                state.revealedValues = next;
                render();
                return;
            }
            try {
                const encodedRef = encodeURIComponent(canonical);
                const data = await request(`/api/keycenter/temp-refs/${encodedRef}/value`);
                state.revealedValues = { ...state.revealedValues, [canonical]: data.value };
            } catch {
                setMessage('warn', '값을 복호화할 수 없습니다.');
            }
            render();
            return;
        }
        if (action === 'copy-temp-ref') {
            const btn = event?.target?.closest('[data-action="copy-temp-ref"]');
            try {
                await navigator.clipboard.writeText(dataset.ref || '');
                if (btn) { btn.textContent = '✓'; setTimeout(() => { btn.textContent = t('copy') + ' Ref'; }, 1500); }
            } catch {
                if (btn) { btn.textContent = '복사 실패'; setTimeout(() => { btn.textContent = t('copy') + ' Ref'; }, 1500); }
            }
            return;
        }
        if (action === 'navigate-to-vault') {
            const vault = state.vaults.find(v => v.vault_runtime_hash === dataset.key);
            if (!vault) {
                setMessage('warn', '연결된 볼트를 찾을 수 없습니다.');
                render();
                return;
            }
            await selectVaultByKey(dataset.key);
            state.activePage = 'vaults';
            syncRoute(false);
            render();
            return;
        }
        if (action === 'promote-to-vault') {
            const vaultSelect = document.getElementById('promote-vault-select');
            const vaultHash = vaultSelect?.value;
            const ref = dataset.ref;
            const name = dataset.name;
            if (!vaultHash || !ref || !name) {
                setMessage('warn', 'ref, name, vault를 모두 선택해주세요.');
                render();
                return;
            }
            try {
                await request('/api/keycenter/promote', {
                    method: 'POST',
                    body: JSON.stringify({ ref, name, vault_hash: vaultHash })
                });
                setMessage('ok', `${name} → 볼트에 저장 완료`);
            } catch (err) {
                setMessage('error', '격상 실패: ' + err.message);
            }
            render();
            return;
        }
        if (action === 'show-create-temp-ref') {
            state.showTempRefForm = !state.showTempRefForm;
            render();
            return;
        }
        if (action === 'show-create-reg-token') {
            state.showRegTokenForm = !state.showRegTokenForm;
            state.createdRegToken = null;
            render();
            return;
        }
        if (action === 'copy-reg-token') {
            if (state.createdRegToken?.token) {
                navigator.clipboard.writeText(state.createdRegToken.token);
                setMessage('ok', '토큰이 복사되었습니다.');
                render();
            }
            return;
        }
        if (action === 'revoke-reg-token') {
            await request(`/api/admin/registration-tokens/${dataset.tokenId}`, { method: 'DELETE' });
            await loadRegTokens();
            render();
            return;
        }
    } catch (err) {
        setMessage('error', err.message);
        render();
    }
}

  function onSubmit(event) {
    const form = event.target.closest('form[data-action]');
    if (form && form.dataset.action === 'create-temp-ref-form') {
        event.preventDefault();
        const name = form.querySelector('[name=name]')?.value || '';
        const value = form.querySelector('[name=value]')?.value || '';
        if (!value) return;
        request('/api/keycenter/temp-refs', {
            method: 'POST',
            body: JSON.stringify({ name, value })
        }).then(() => {
            state.showTempRefForm = false;
            setMessage('ok', '임시키가 등록되었습니다.');
            loadKeycenterTempRefs().then(() => render());
        }).catch(err => {
            setMessage('error', err.message);
            render();
        });
        return;
    }
    if (form && form.dataset.action === 'create-reg-token-form') {
        event.preventDefault();
        const label = form.querySelector('[name=label]')?.value || '';
        const expires = parseInt(form.querySelector('[name=expires]')?.value || '1440', 10);
        request('/api/admin/registration-tokens', {
            method: 'POST',
            body: JSON.stringify({ label, expires_in_minutes: expires })
        }).then(data => {
            state.createdRegToken = data;
            loadRegTokens().then(() => render());
        }).catch(err => {
            setMessage('error', err.message);
            render();
        });
        return;
    }
    const dataForm = event.target.closest('form[data-form]');
    if (!dataForm) return;
    event.preventDefault();
    handleFormSubmit(dataForm);
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

async function adminSetup(ownerPassword, adminPassword) {
    state.ui.adminSetupError = '';
    try {
        await request('/api/admin/setup', {
            method: 'POST',
            body: JSON.stringify({ owner_password: ownerPassword, admin_password: adminPassword })
        });
        state.ui.adminSetupRequired = false;
        state.ui.adminRequired = true; // Now show login screen
    } catch (err) {
        state.ui.adminSetupError = err.message || '설정에 실패했습니다.';
    }
}

async function adminLogin(password) {
    state.ui.adminLoginError = '';
    try {
        await request('/api/admin/login', { method: 'POST', body: JSON.stringify({ password }) });
        state.ui.adminRequired = false;
        await loadStatus();
        if (state.status?.locked) {
            state.ui.locked = true;
            return;
        }
        await loadUIConfig();
        await loadVaults();
        await loadConfigsSummary();
        await loadFunctions();
        await loadTrackedRefAudit();
        await loadKeycenterTempRefs();
        await syncPageData();
        render();
    } catch (err) {
        state.ui.adminLoginError = err.message || '비밀번호가 올바르지 않습니다.';
    }
}

async function unlock(password) {
    state.ui.unlockError = '';
    try {
        await request('/api/unlock', { method: 'POST', body: JSON.stringify({ password }) });
        state.ui.locked = false;
        // After unlock, require admin login before accessing shell
        state.ui.adminRequired = true;
        render();
    } catch (err) {
        state.ui.unlockError = err.message || '비밀번호가 올바르지 않습니다.';
    }
}

async function adminLogout() {
    try {
        await request('/api/admin/logout', { method: 'POST' });
    } catch (_) {}
    state.ui.adminRequired = true;
    state.ui.locked = false;
    render();
}

async function boot() {
    applyRoute(window.location.pathname, window.location.search);
    // 1. Check admin auth
    try {
        const check = await request('/api/admin/check');
        if (check && check.setup_required) {
            state.ui.adminSetupRequired = true;
            return;
        }
    } catch (err) {
        // 401 = not authenticated
        state.ui.adminRequired = true;
        return;
    }
    // 2. Check vault lock
    await loadStatus();
    if (state.status?.locked) {
        state.ui.locked = true;
        return;
    }
    // 3. Load everything
    await loadUIConfig();
    await loadVaults();
    await loadConfigsSummary();
    await loadFunctions();
    await loadTrackedRefAudit();
    await loadKeycenterTempRefs();
    await loadRegTokens();
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
                adminSetup,
                adminLogin,
                loginWithPasskey,
                adminLogout,
                unlock,
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
                auditVaultCount,
                auditTotalCount,
                auditSelectedVault,
                encodeURIComponent
    };
}
