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

export function useAdminApp() {
const state = reactive({
    activePage: 'vaults',
    activeTabByPage: {
        vaults: '전체 볼트',
        functions: '함수 목록',
        audit: '감사 로그',
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
        throw new Error(message);
    }
    return data;
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

function onGlobalSearchInput(event) {
    state.globalQuery = event?.target?.value || '';
    render();
}

function routePath(page, tab) {
    if (page === 'vaults' && tab === '키 / 환경값') {
        const vaultHash = state.selectedVault?.vault_runtime_hash || state.routeSelectedVaultHash;
        return vaultHash ? `/vaults/local/${encodeURIComponent(vaultHash)}` : '/vaults/local';
    }
    const entry = routeEntries.find((item) => item.page === page && item.tab === tab);
    return entry ? entry.path : '/';
}

function applyRoute(pathname) {
    const normalized = pathname !== '/' && pathname.endsWith('/') ? pathname.slice(0, -1) : pathname;
    const localVaultMatch = normalized.match(/^\/vaults\/local(?:\/([^/]+))?$/);
    if (localVaultMatch) {
        state.activePage = 'vaults';
        state.activeTabByPage.vaults = '키 / 환경값';
        state.routeSelectedVaultHash = localVaultMatch[1] ? decodeURIComponent(localVaultMatch[1]) : null;
        return;
    }
    const matched = normalized === '/' ? { page: 'vaults', tab: '전체 볼트' } : routeByPath[normalized];
    if (!matched) return;
    state.activePage = matched.page;
    state.activeTabByPage[matched.page] = matched.tab;
    state.routeSelectedVaultHash = null;
}

function syncRoute(replace) {
    const nextPath = routePath(state.activePage, activeTab());
    if (window.location.pathname === nextPath) return;
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
    if (page === 'audit') return state.trackedRefAudit && state.trackedRefAudit.counts ? state.trackedRefAudit.counts.total_refs : 0;
    if (page === 'settings') return state.uiConfig ? 2 : 0;
    return 0;
}

function vaultKindLabel(kind) {
    return kind === 'VE' ? 'ENV' : 'KEY';
}

function vaultItemSyncKey(kind, name) {
    return `${kind}:${name}`;
}

function renderSyncStatus(kind, name) {
    const item = state.vaultItemSyncStatus[vaultItemSyncKey(kind, name)];
    if (!item) return '<span class="muted">확인중</span>';
    if (item.total === 1 && item.matched === 1) {
        return renderStatusPill('유일키', 'active');
    }
    const klass = item.matched === item.total ? 'active' : 'pending';
    return renderStatusPill(`${item.matched}/${item.total} ${item.matched === item.total ? '동기화중' : '동기화안됨'}`, klass);
}

function vaultSyncStatus(kind, name) {
    const item = state.vaultItemSyncStatus[vaultItemSyncKey(kind, name)];
    if (!item) {
        return { loading: true, label: '확인중', className: '' };
    }
    if (item.total === 1 && item.matched === 1) {
        return { loading: false, label: '유일키', className: 'active' };
    }
    const className = item.matched === item.total ? 'active' : 'pending';
    const label = `${item.matched}/${item.total} ${item.matched === item.total ? '동기화중' : '동기화안됨'}`;
    return { loading: false, label, className };
}

function vaultTargetOptions(includeHost = true) {
    const options = [];
    if (includeHost) options.push({ value: 'host', label: 'Host Vault' });
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
        return '<div class="empty">같은 키의 LOCAL / EXTERNAL 관계가 아직 없습니다.</div>';
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
    return sections || '<div class="empty">같은 키의 LOCAL / EXTERNAL 관계가 아직 없습니다.</div>';
}

function configRelationsByScope() {
    return ['LOCAL', 'EXTERNAL', 'TEMP'].map((scope) => ({
        scope,
        rows: state.configRelations.filter((item) => String(item.scope || '').toUpperCase() === scope)
    })).filter((section) => section.rows.length);
}

function renderSidebar() {
    const sections = [
        { page: 'vaults', label: '볼트' },
        { page: 'functions', label: '함수' },
        { page: 'audit', label: '감사' },
        { page: 'settings', label: '설정' }
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
    const breadcrumb = `${page.label} / ${activeTab()}`;
    const headerContent = state.activePage === 'vaults' && state.selectedVault ? `
        <div class="workspace-title">
            <div class="segmented-label">현재 볼트</div>
            <h1>${escapeHTML(state.selectedVault.display_name || state.selectedVault.vault_name || '볼트 미선택')}</h1>
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
    if (state.activePage === 'vaults') return '볼트를 먼저 고르고, 그 안의 키와 환경값을 관리합니다.';
    if (state.activePage === 'functions') return '함수 목록과 실행 흐름은 보조 작업으로 유지합니다.';
    if (state.activePage === 'audit') return '감사 로그를 하나의 화면에서 조회합니다.';
    return '설정은 작고 명확하게 유지합니다.';
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
                <div class="sidebar-label">전체</div>
                <div class="nav-list">
                    <a
                        href="${routePath('vaults', '전체 볼트')}"
                        class="nav-item${activeTab() === '전체 볼트' ? ' active' : ''}"
                        data-action="set-tab"
                        data-tab="전체 볼트"
                    >
                        <span class="nav-item-main">
                            <span>전체 볼트</span>
                        </span>
                    </a>
                    <a
                        href="${routePath('vaults', 'Host Vault')}"
                        class="nav-item${activeTab() === 'Host Vault' ? ' active' : ''}"
                        data-action="set-tab"
                        data-tab="Host Vault"
                    >
                        <span class="nav-item-main">
                            <span>호스트 볼트</span>
                        </span>
                    </a>
                </div>
            </div>
            <div class="sidebar-section">
                <div class="sidebar-label">로컬 볼트</div>
                <div class="nav-list">
                    ${vaults.map((item) => `
                        <a
                            href="/vaults/local/${encodeURIComponent(item.vault_runtime_hash)}"
                            class="nav-item${activeTab() !== 'Host Vault' && state.selectedVault && item.vault_runtime_hash === state.selectedVault.vault_runtime_hash ? ' active' : ''}"
                            data-action="select-vault"
                            data-key="${escapeHTML(item.vault_runtime_hash)}"
                        >
                            <span class="nav-item-main">
                                <span>${escapeHTML(item.display_name || item.vault_name || item.vault_runtime_hash)}</span>
                            </span>
                            ${renderStatusPill(item.status || 'active', statusClass(item.status || 'active'))}
                        </a>
                    `).join('') || '<div class="empty">표시할 볼트가 없습니다.</div>'}
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
                <div class="sidebar-label">함수 목록</div>
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
                    `).join('') || '<div class="empty">표시할 함수가 없습니다.</div>'}
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
                <div class="sidebar-label">감사 볼트</div>
                <div class="nav-list">
                    ${vaults.map((item) => `
                        <a
                            href="${routePath('audit', '감사 로그')}"
                            class="nav-item${state.auditVault === item.vault_runtime_hash ? ' active' : ''}"
                            data-action="audit-page-select-vault"
                            data-key="${escapeHTML(item.vault_runtime_hash)}"
                        >
                            <span class="nav-item-main">
                                <span>${escapeHTML(item.display_name || item.vault_name || item.vault_runtime_hash)}</span>
                            </span>
                            ${renderStatusPill(item.status || 'active', statusClass(item.status || 'active'))}
                        </a>
                    `).join('') || '<div class="empty">표시할 볼트가 없습니다.</div>'}
                </div>
            </div>
        `;
        return;
    }

    if (state.activePage === 'settings') {
        state.ui.secondarySidebarHidden = false;
        state.ui.secondarySidebarHTML = `
            <div class="sidebar-section">
                <div class="sidebar-label">설정</div>
                <div class="nav-list">
                    ${page.tabs.map((tabName) => `
                        <a
                            href="${routePath(state.activePage, tabName)}"
                            class="nav-item${tabName === activeTab() ? ' active' : ''}"
                            data-action="set-tab"
                            data-tab="${escapeHTML(tabName)}"
                        >
                            <span class="nav-item-main">
                                <span>${escapeHTML(tabName)}</span>
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
            <div class="sidebar-label">${escapeHTML(page.label)}</div>
            <div class="nav-list">
                ${page.tabs.map((tabName) => `
                    <a
                        href="${routePath(state.activePage, tabName)}"
                        class="nav-item${tabName === activeTab() ? ' active' : ''}"
                        data-action="set-tab"
                        data-tab="${escapeHTML(tabName)}"
                    >
                        <span class="nav-item-main">
                            <span>${escapeHTML(tabName)}</span>
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
    if (tab === '전체 볼트') return renderVaultInventory();
    if (tab === 'Host Vault') return renderHostVault();
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
        '볼트 현황',
        `
            <div class="toolbar">
                <div class="toolbar-group">
                    <input class="field context-search" id="vault-search" type="search" placeholder="볼트 검색" value="${escapeHTML(state.globalQuery)}">
                </div>
                <span class="pill">${allVaultRows.length}개</span>
                <button class="btn btn-soft" data-action="refresh-vaults">새로고침</button>
            </div>
        `,
        renderTable([
            {
                label: '볼트명',
                render: (row) => `<span>${escapeHTML(row.display_name || row.vault_name)}</span>`
            },
            { label: '식별자', render: (row) => `<span class="code">${escapeHTML(row.vault_id || row.vault_runtime_hash)}</span>` },
            { label: '경로', render: (row) => escapeHTML((row.managed_paths && row.managed_paths[0]) || '-') },
            { label: 'IP', render: (row) => escapeHTML(row.ip || '-') },
            { label: '상태', render: (row) => renderStatusPill(row.status, statusClass(row.status)) }
        ], allVaultRows, (row) => {
            const classes = [];
            if ((activeTab() === 'Host Vault' && row.is_host) || (activeTab() !== 'Host Vault' && !row.is_host && state.selectedVault && row.vault_runtime_hash === state.selectedVault.vault_runtime_hash)) classes.push('is-selected');
            classes.push('is-clickable');
            return classes.join(' ');
        }, (row) => row.is_host
            ? `data-action="set-tab" data-tab="Host Vault"`
            : `data-action="select-vault" data-key="${escapeHTML(row.vault_runtime_hash)}"`)
    );

    const detail = activeTab() === 'Host Vault' ? hostVaultRow() : (state.vaultDetail || selectedVaultRecord());
    renderRightPane('볼트 상세', detail ? `
        <div class="stack">
            <div class="card">
                <div class="card-title">선택된 볼트</div>
                ${renderKVGrid([
                    ['이름', detail.display_name || detail.vault_name || '-'],
                    ['식별자', detail.vault_id || detail.vault_runtime_hash || '-'],
                    ['경로', (detail.managed_paths && detail.managed_paths[0]) || '-'],
                    ['IP', detail.ip || '-'],
                    ['상태', detail.status || '-']
                ])}
            </div>
            <form class="stack" data-form="save-vault-meta">
                <div class="card">
                    <div class="card-title">기본 정보</div>
                    <div class="stack">
                        <div class="kv"><span class="label">볼트 이름</span><input class="field" name="display_name" value="${escapeHTML(detail.display_name || '')}"></div>
                        <div class="kv"><span class="label">설명</span><textarea class="textarea" name="description">${escapeHTML(detail.description || '')}</textarea></div>
                        <div class="kv"><span class="label">태그 JSON</span><textarea class="textarea" name="tags_json">${escapeHTML(detail.tags_json || '[]')}</textarea></div>
                    </div>
                </div>
                <div class="toolbar">
                    <button class="btn btn-primary" type="submit">저장</button>
                </div>
            </form>
        </div>
    ` : '<div class="empty">왼쪽에서 볼트를 선택하세요.</div>');
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
        '호스트 볼트',
        `
            <div class="toolbar">
                <div class="toolbar-group">
                    <span class="segmented-label">표시 대상</span>
                    <div class="segmented" role="tablist" aria-label="표시 대상">
                        <button class="btn ${state.vaultItemKind === 'ALL' ? 'btn-primary' : 'btn-soft'}" data-action="set-vault-kind" data-kind="ALL" aria-pressed="${state.vaultItemKind === 'ALL' ? 'true' : 'false'}">전체</button>
                        <button class="btn ${state.vaultItemKind === 'VE' ? 'btn-primary' : 'btn-soft'}" data-action="set-vault-kind" data-kind="VE" aria-pressed="${state.vaultItemKind === 'VE' ? 'true' : 'false'}">환경값</button>
                        <button class="btn ${state.vaultItemKind === 'VK' ? 'btn-primary' : 'btn-soft'}" data-action="set-vault-kind" data-kind="VK" aria-pressed="${state.vaultItemKind === 'VK' ? 'true' : 'false'}">키</button>
                    </div>
                    <input class="field context-search" id="key-search" type="search" placeholder="호스트 볼트 안에서 검색" value="${escapeHTML(state.globalQuery)}">
                </div>
                <span class="pill">${itemRows.length}개 항목</span>
            </div>
        `,
        renderTable([
            { label: '종류', render: (row) => `<span class="pill ${row.item_kind === 'VE' ? 'kind-ve' : 'kind-vk'}">${escapeHTML(vaultKindLabel(row.item_kind))}</span>` },
            { label: '키명', render: (row) => `<span>${escapeHTML(row.name)}</span>` },
            { label: '키값', render: (row) => `<span class="code">${escapeHTML(itemIdentifier(row))}</span>` },
            { label: '범위', render: (row) => renderStatusPill(row.scope || (row.item_kind === 'VE' ? 'LOCAL' : 'TEMP'), scopeClass(row.scope || (row.item_kind === 'VE' ? 'LOCAL' : 'TEMP'))) }
        ], itemRows, (row) => {
            const classes = [];
            if (row.name === selectedItemName && row.item_kind === selectedItemKind) classes.push('is-selected');
            classes.push('is-clickable');
            return classes.join(' ');
        }, (row) => `data-action="select-host-item" data-kind="${escapeHTML(row.item_kind)}" data-key="${escapeHTML(row.name)}"`)
    );

    renderRightPane(isConfigItem ? '환경값 상세' : '키 상세', `
        <div class="stack">
            ${canMoveItem ? `
                <form class="stack" data-form="${isConfigItem ? 'promote-config' : 'promote-key'}">
                    <div class="card">
                        <div class="card-title">${isConfigItem ? '환경값 이동' : '키 이동'}</div>
                        <div class="stack">
                            <div class="kv"><span class="label">대상 볼트</span><select class="select" name="target_vault">${renderTargetOptions(vaultTargetOptions(true), 'host')}</select></div>
                            <div class="kv"><span class="label">대상 범위</span><select class="select" name="target_scope">${renderOptions(isConfigItem ? ['LOCAL', 'EXTERNAL', 'TEMP'] : ['TEMP', 'LOCAL', 'EXTERNAL'], detail?.scope || (isConfigItem ? 'LOCAL' : 'TEMP'))}</select></div>
                            <div class="kv"><span class="label">보낼 값</span><textarea class="textarea" name="move_value" placeholder="이동할 값을 입력하거나 확인하세요">${escapeHTML(visibleValue || '')}</textarea></div>
                        </div>
                        <div class="muted">${isConfigItem ? 'Host Vault 환경값을 다른 볼트로 보냅니다. LOCAL / EXTERNAL 관계도 여기서 정리합니다.' : 'Host Vault 키를 다른 볼트로 보냅니다.'}</div>
                    </div>
                    <button class="btn btn-soft" type="submit"${visibleValue ? '' : ' disabled'}>${isConfigItem ? '환경값 이동' : '키 이동'}</button>
                </form>
            ` : ''}
            <form class="stack" data-form="${isConfigItem ? 'save-host-config' : 'save-host-key'}">
                <div class="card">
                    <div class="card-title">${detailName ? '항목 상세' : (isConfigItem ? '새 환경값' : '새 키')}</div>
                    <div class="stack">
                        <div class="kv"><span class="label">${isConfigItem ? '환경값 이름' : '키 이름'}</span><input class="field" name="${isConfigItem ? 'key' : 'name'}" value="${escapeHTML(detailName || '')}" ${detailName ? 'readonly' : ''}></div>
                        <div class="kv">
                            <span class="label">${isConfigItem ? '환경값' : '키 값'}</span>
                            <div class="row" style="align-items:center;">
                                <button class="btn btn-soft" type="button" data-action="toggle-reveal">${state.revealValue ? '가리기' : '보기'}</button>
                                ${state.revealValue && visibleValue ? `<button class="btn btn-soft" type="button" data-action="copy-value">복사</button>` : ''}
                            </div>
                            <textarea class="textarea" name="value" placeholder="${detailName ? '새 값을 입력하면 덮어씁니다' : '필수'}">${escapeHTML(state.revealValue ? (visibleValue || '') : '••••••••••••')}</textarea>
                        </div>
                        <div class="kv"><span class="label">범위</span><select class="select" name="scope">${renderOptions(isConfigItem ? ['LOCAL', 'EXTERNAL', 'TEMP'] : ['TEMP', 'LOCAL', 'EXTERNAL'], detail?.scope || (isConfigItem ? 'LOCAL' : 'TEMP'))}</select></div>
                    </div>
                </div>
                <div class="toolbar">
                    <button class="btn btn-primary" type="submit">${detailName ? '저장' : '생성'}</button>
                </div>
            </form>
            ${isConfigItem ? `
                <details class="card" open>
                    <summary class="card-title">LOCAL / EXTERNAL 관계</summary>
                    ${renderConfigRelations()}
                </details>
            ` : `
                <div class="card">
                    <div class="card-title">부가 정보</div>
                    ${renderKVGrid([
                        ['식별자', detail?.token || detail?.ref || '-'],
                        ['범위', detail?.scope || 'TEMP'],
                        ['상태', detail?.status || 'temp'],
                        ['저장소', 'Host Vault']
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
    const itemTitle = isConfigItem ? '환경값 상세' : '키 상세';
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
        '현재 볼트 항목',
        `
            <div class="toolbar">
                <div class="toolbar-group">
                    <div class="toolbar-group">
                        <span class="segmented-label">표시 대상</span>
                        <div class="segmented" role="tablist" aria-label="표시 대상">
                            <button class="btn ${state.vaultItemKind === 'ALL' ? 'btn-primary' : 'btn-soft'}" data-action="set-vault-kind" data-kind="ALL" aria-pressed="${state.vaultItemKind === 'ALL' ? 'true' : 'false'}">전체</button>
                            <button class="btn ${state.vaultItemKind === 'VE' ? 'btn-primary' : 'btn-soft'}" data-action="set-vault-kind" data-kind="VE" aria-pressed="${state.vaultItemKind === 'VE' ? 'true' : 'false'}">환경값</button>
                            <button class="btn ${state.vaultItemKind === 'VK' ? 'btn-primary' : 'btn-soft'}" data-action="set-vault-kind" data-kind="VK" aria-pressed="${state.vaultItemKind === 'VK' ? 'true' : 'false'}">키</button>
                        </div>
                    </div>
                    <input class="field context-search" id="key-search" type="search" placeholder="현재 볼트 안에서 검색" value="${escapeHTML(state.globalQuery)}">
                </div>
                <span class="pill">${itemRows.length}개 항목</span>
                <button class="btn btn-primary" data-action="new-key">${state.vaultItemKind === 'VE' ? '새 환경값' : '새 키'}</button>
            </div>
        `,
        renderTable([
            { label: '종류', render: (row) => `<span class="pill ${row.item_kind === 'VE' ? 'kind-ve' : 'kind-vk'}">${escapeHTML(vaultKindLabel(row.item_kind))}</span>` },
            { label: '키명', render: (row) => `<span>${escapeHTML(row.name)}</span>` },
            { label: '키값', render: (row) => `<span class="code">${escapeHTML(itemIdentifier(row))}</span>` },
            { label: '동기화', render: (row) => renderSyncStatus(row.item_kind, row.name) }
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
                        <div class="card-title">${isConfigItem ? '환경값 이동' : '키 이동'}</div>
                        <div class="stack">
                            <div class="kv"><span class="label">대상 볼트</span><select class="select" name="target_vault">${renderTargetOptions(vaultTargetOptions(true), state.selectedVault ? state.selectedVault.vault_runtime_hash : 'host')}</select></div>
                            <div class="kv"><span class="label">대상 범위</span><select class="select" name="target_scope">${renderOptions(isConfigItem ? ['LOCAL', 'EXTERNAL', 'TEMP'] : ['TEMP', 'LOCAL', 'EXTERNAL'], isConfigItem ? (state.configDetail?.scope || 'LOCAL') : (state.keyDetail?.scope || 'TEMP'))}</select></div>
                            <div class="kv"><span class="label">보낼 값</span><textarea class="textarea" name="move_value" placeholder="이동할 값을 입력하거나 확인하세요">${escapeHTML(visibleValue || '')}</textarea></div>
                        </div>
                        <div class="muted">${isConfigItem ? '선택한 환경값을 다른 볼트나 Host Vault로 보냅니다. LOCAL / EXTERNAL 관계도 여기서 정리합니다.' : '선택한 키를 다른 볼트나 Host Vault로 보냅니다.'}</div>
                    </div>
                    <button class="btn btn-soft" type="submit"${visibleValue ? '' : ' disabled'}>${isConfigItem ? '환경값 이동' : '키 이동'}</button>
                </form>
            ` : ''}
            <form class="stack" data-form="${isConfigItem ? 'save-agent-config' : 'save-key'}">
                <div class="card">
                    <div class="card-title">${detailName ? '항목 상세' : (isConfigItem ? '새 환경값' : '새 키')}</div>
                    <div class="stack">
                        <div class="kv"><span class="label">${isConfigItem ? '환경값 이름' : '키 이름'}</span><input class="field" name="${isConfigItem ? 'key' : 'name'}" value="${escapeHTML(detailName || '')}" ${detailName ? 'readonly' : ''}></div>
                        <div class="kv">
                            <span class="label">${isConfigItem ? '환경값' : '키 값'}</span>
                            <div class="row" style="align-items:center;">
                                <button class="btn btn-soft" type="button" data-action="toggle-reveal">${state.revealValue ? '가리기' : '보기'}</button>
                                ${state.revealValue && visibleValue ? `<button class="btn btn-soft" type="button" data-action="copy-value">복사</button>` : ''}
                            </div>
                            <textarea class="textarea" name="value" placeholder="${detailName ? '새 값을 입력하면 덮어씁니다' : '필수'}">${escapeHTML(state.revealValue ? (visibleValue || '') : '••••••••••••')}</textarea>
                        </div>
                        ${isConfigItem ? `
                            <input type="hidden" name="scope" value="${escapeHTML(state.configDetail?.scope || 'LOCAL')}">
                            <input type="hidden" name="status" value="${escapeHTML(state.configDetail?.status || 'active')}">
                        ` : `
                            <div class="kv"><span class="label">설명</span><textarea class="textarea" name="description">${escapeHTML(key && key.description ? key.description : '')}</textarea></div>
                            <div class="kv"><span class="label">태그 JSON</span><textarea class="textarea" name="tags_json">${escapeHTML(key && key.tags_json ? key.tags_json : '[]')}</textarea></div>
                        `}
                    </div>
                </div>
                <div class="toolbar">
                    <button class="btn btn-primary" type="submit">${detailName ? '저장' : '생성'}</button>
                    ${detailName ? `<button class="btn btn-danger" type="button" data-action="${isConfigItem ? 'delete-agent-config' : 'delete-key'}">삭제</button>` : ''}
                </div>
            </form>
            ${!isConfigItem ? `
                <details class="card">
                    <summary class="card-title">부가 정보</summary>
                    ${renderKVGrid([
                        ['연결 수', summary ? summary.bindings_total || summary.bindings_count || 0 : 0],
                        ['사용 수', usageCount || 0],
                        ['범위', key ? (key.scope || '-') : '-'],
                        ['상태', key ? (key.status || '-') : '-']
                    ])}
                    ${bindings.length ? `<div class="stack" style="margin-top:12px;">${bindings.map((item) => `<div class="value">${escapeHTML(item.binding_type)} / ${escapeHTML(item.target_name)} / ${escapeHTML(item.field_key || '-')}</div>`).join('')}</div>` : '<div class="empty">연결 정보 없음</div>'}
                    ${recentAudit.length ? `<div class="stack" style="margin-top:12px;">${recentAudit.map((item) => `<div class="value">${escapeHTML(item.action || item.event_type || 'event')} · ${escapeHTML(item.created_at || item.timestamp || '-')}</div>`).join('')}</div>` : '<div class="empty">감사 로그 없음</div>'}
                </details>
                <form class="stack" data-form="save-key-fields">
                    <details class="card">
                        <summary class="card-title">필드 편집</summary>
                        <textarea class="textarea" name="fields_json">${escapeHTML(key && summary && summary.key && summary.key.fields ? formatJSON(summary.key.fields.map((field) => ({ key: field.key, type: field.type, value: '' }))) : '[]')}</textarea>
                        <button class="btn" type="submit"${key ? '' : ' disabled'}>필드 저장</button>
                    </details>
                </form>
            ` : ''}
            ${isConfigItem ? `
                <details class="card" open>
                    <summary class="card-title">LOCAL / EXTERNAL 관계</summary>
                    ${renderConfigRelations()}
                </details>
            ` : ''}
        </div>
    ` : '<div class="empty">볼트를 먼저 선택하세요.</div>');
}

function renderVaultBindings() {
    const keys = state.vaultKeys.filter((item) => matchesQuery(item.name));
    renderListPane(
        'Binding focus',
        `
            <div class="card">
                <div class="card-title">Vault Context</div>
                <div class="value">${escapeHTML(state.selectedVault ? state.selectedVault.vault_runtime_hash : 'No vault selected')}</div>
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
        ) : '<div class="empty">Select a vault first.</div>'
    );

    const bindings = state.keyBindings || [];
    renderCenterPane(
        'Key Bindings',
        `<div class="toolbar"><span class="pill">${bindings.length} bindings</span></div>`,
        renderTable([
            { label: 'Target Type', render: (row) => escapeHTML(row.binding_type || '-') },
            { label: 'Target Name', render: (row) => escapeHTML(row.target_name || '-') },
            { label: 'Field', render: (row) => escapeHTML(row.field_key || '-') },
            { label: 'Required', render: (row) => escapeHTML(String(row.required)) },
            { label: 'Ref', render: (row) => `<span class="code">${escapeHTML(row.ref_canonical || '-')}</span>` }
        ], bindings, () => '')
    );

    renderRightPane('Edit bindings', state.selectedKey ? `
        <div class="stack">
            <div class="card">
                <div class="card-title">Selected Key</div>
                ${renderKVGrid([
                    ['Name', state.selectedKey.name],
                    ['Scope', state.selectedKey.scope || '-'],
                    ['Vault', state.selectedVault ? state.selectedVault.vault_runtime_hash : '-']
                ])}
            </div>
            <form class="stack" data-form="replace-key-bindings">
                <div class="card">
                    <div class="card-title">Bindings JSON</div>
                    <textarea class="textarea" name="bindings_json">${escapeHTML(formatJSON((state.keyBindings || []).map((item) => ({
                        binding_id: item.binding_id,
                        binding_type: item.binding_type,
                        target_name: item.target_name,
                        field_key: item.field_key,
                        required: item.required
                    }))))}</textarea>
                </div>
                <div class="toolbar">
                    <button class="btn btn-primary" type="submit">Replace Bindings</button>
                    <button class="btn btn-danger" type="button" data-action="delete-all-bindings">Delete All</button>
                </div>
            </form>
        </div>
    ` : '<div class="empty">Select a key to manage bindings.</div>');
}

function renderVaultAudit() {
    const useKeyMode = !!state.auditKey;
    const rows = useKeyMode ? state.keyAudit : state.vaultAudit;
    renderListPane(
        'Audit scope',
        `
            <div class="stack">
                <div class="card">
                    <div class="card-title">Vault</div>
                    <div class="value">${escapeHTML(state.selectedVault ? state.selectedVault.display_name || state.selectedVault.vault_name : 'No vault selected')}</div>
                </div>
                <button class="btn btn-soft" data-action="clear-audit-key">Vault-only feed</button>
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
        ) : '<div class="empty">Select a vault first.</div>'
    );

    renderCenterPane(
        'Vault Audit',
        `<div class="toolbar"><span class="pill">${rows.length} events</span><span class="pill">${useKeyMode ? 'Key scope' : 'Vault scope'}</span></div>`,
        renderTable([
            { label: 'Time', render: (row) => escapeHTML(row.created_at || row.timestamp || '-') },
            { label: 'Action', render: (row) => escapeHTML(row.action || row.event_type || '-') },
            { label: 'Source', render: (row) => escapeHTML(row.actor_type || row.source_type || '-') },
            { label: 'Object', render: (row) => escapeHTML(row.entity_id || row.object_id || '-') }
        ], rows, () => '')
    );

    renderRightPane('Event detail', rows.length ? `
        <div class="card">
            <div class="card-title">Latest Event</div>
            <pre class="code">${escapeHTML(formatJSON(rows[0]))}</pre>
        </div>
    ` : '<div class="empty">No audit events for the current scope.</div>');
}

function renderGrouped() {
    const groupedRows = state.groupedRows.filter((row) => matchesQuery(row.name));
    const selectedGroup = groupedRows.find((row) => row.name === state.selectedGroupedName) || null;
    const entries = selectedGroup ? selectedGroup.entries : [];
    const selectedEntry = state.selectedGroupedEntry || (entries[0] || null);
    const visibleValue = selectedEntry && selectedEntry.kind === 'VE' ? state.configDetail?.value : state.keyDetail?.value;

    renderListPane(
        '키 묶음',
        `
            <div class="stack">
                <div class="toolbar">
                    <a href="${routePath('grouped', '키')}" class="btn ${activeTab() === '키' ? 'btn-primary' : 'btn-soft'}" data-action="jump-tab" data-page="grouped" data-tab="키">키</a>
                    <a href="${routePath('grouped', '환경값')}" class="btn ${activeTab() === '환경값' ? 'btn-primary' : 'btn-soft'}" data-action="jump-tab" data-page="grouped" data-tab="환경값">환경값</a>
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
                    <span class="muted">${escapeHTML(item.count)}개 항목</span>
                </div>
            `,
            () => ''
        )
    );

    renderCenterPane(
        '묶음 항목',
        `<div class="toolbar"><span class="pill">${entries.length}개</span></div>`,
        renderTable([
            { label: '이름', render: (row) => `<button data-action="select-grouped-entry" data-vault="${escapeHTML(row.vault_runtime_hash)}" data-kind="${escapeHTML(row.kind)}" data-key="${escapeHTML(row.name)}">${escapeHTML(row.name)}</button>` },
            { label: '볼트', render: (row) => escapeHTML(row.vault_name || row.vault_id || '-') },
            { label: '식별자', render: (row) => `<span class="code">${escapeHTML(row.vault_id || row.vault_runtime_hash)}</span>` },
            { label: '경로', render: (row) => escapeHTML(row.path || '-') },
            { label: 'IP', render: (row) => escapeHTML(row.ip || '-') }
        ], entries, (row) => selectedEntry && row.vault_runtime_hash === selectedEntry.vault_runtime_hash && row.kind === selectedEntry.kind && row.name === selectedEntry.name ? 'is-selected' : '')
    );

    renderRightPane(activeTab() === '환경값' ? '환경값 상세' : '키 상세', selectedEntry ? `
        <div class="stack">
            <div class="card">
                <div class="card-title">선택 항목</div>
                ${renderKVGrid([
                    ['이름', selectedEntry.name || '-'],
                    ['볼트', selectedEntry.vault_name || '-'],
                    ['식별자', selectedEntry.vault_id || selectedEntry.vault_runtime_hash || '-'],
                    ['경로', selectedEntry.path || '-'],
                    ['IP', selectedEntry.ip || '-']
                ])}
            </div>
            <div class="card">
                <div class="card-title">값</div>
                <div class="stack">
                    <div class="row" style="align-items:center;">
                        <button class="btn btn-soft" type="button" data-action="toggle-reveal">${state.revealValue ? '가리기' : '보기'}</button>
                        ${state.revealValue && visibleValue ? `<button class="btn btn-soft" type="button" data-action="copy-value">복사</button>` : ''}
                    </div>
                    <textarea class="textarea" readonly>${escapeHTML(state.revealValue ? (visibleValue || '') : '••••••••••••')}</textarea>
                </div>
            </div>
        </div>
    ` : '<div class="empty">왼쪽에서 묶음을 선택하세요.</div>');
}

function renderConfigs() {
    const tab = activeTab();
    if (tab === '검색') return renderConfigSearch();
    if (tab === '일괄 수정') return renderConfigBulk();
    if (tab === '볼트별 설정') return renderConfigPerVault();
    return renderConfigSummary();
}

function renderConfigSummary() {
    renderListPane(
        'Summary filters',
        '',
        `
            <div class="card">
                <div class="card-title">Current Summary</div>
                <div class="stack">
                    <span class="value">Cross-agent summary only in first pass.</span>
                    <span class="muted">Use Search or Per Vault for concrete config records.</span>
                </div>
            </div>
        `
    );

    const summary = state.configsSummary || { total_configs: 0, agents_with_configs: 0, total_agents: 0 };
    renderCenterPane(
        'Config Summary',
        '',
        `
            <div class="metrics">
                <div class="metric"><span class="label">Total Configs</span><strong>${escapeHTML(summary.total_configs)}</strong></div>
                <div class="metric"><span class="label">Agents With Configs</span><strong>${escapeHTML(summary.agents_with_configs)}</strong></div>
                <div class="metric"><span class="label">Total Agents</span><strong>${escapeHTML(summary.total_agents)}</strong></div>
            </div>
            <div class="empty">Detailed scope summary is not returned by the backend yet. Use Search or Per Vault for concrete records.</div>
        `
    );

    renderRightPane('Config summary detail', `
        <div class="card">
            <div class="card-title">Guidance</div>
            <div class="stack">
                <div class="value">Use <strong>Search</strong> to inspect a single config key across vaults.</div>
                <div class="value">Use <strong>Per Vault</strong> to edit one vault's configs.</div>
                <div class="value">Use <strong>Bulk Update</strong> for cross-agent changes.</div>
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
                <input class="field" id="config-vault-search" type="search" placeholder="볼트 검색" value="${escapeHTML(state.globalQuery)}">
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
    if (activeTab() === 'Host Vault') {
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
    if (activeTab() === 'Host Vault') {
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
    if (activeTab() === 'Host Vault') return hostVaultVisibleRows();
    if (activeTab() === '키 / 환경값') return localVaultVisibleRows();
    return [];
}

function vaultItemIdentifier(row) {
    if (row.item_kind === 'VE') return row.value || row.name || row.key || row.ref || '-';
    if (row.token) return row.token;
    if (row.ref) return `VK:${row.scope || (activeTab() === 'Host Vault' ? 'TEMP' : 'LOCAL')}:${row.ref}`;
    return row.name || '-';
}

function vaultCenterTitle() {
    if (activeTab() === '전체 볼트') return t('vault_inventory');
    if (activeTab() === 'Host Vault') return t('host_vault_title');
    return t('current_vault_items');
}

function vaultRightPaneTitle() {
    if (activeTab() === '전체 볼트') return currentLocale() === 'en' ? 'Vault Detail' : '볼트 상세';
    return currentVaultSelectedKind() === 'VE' ? t('config_detail') : t('key_detail');
}

function selectedInventoryDetail() {
    return state.vaultDetail || selectedVaultRecord();
}

function vaultPanel() {
    const isHost = activeTab() === 'Host Vault';
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
            ? (isHost ? 'Host Vault 환경값을 다른 볼트로 보냅니다. LOCAL / EXTERNAL 관계도 여기서 정리합니다.' : '선택한 환경값을 다른 볼트나 Host Vault로 보냅니다. LOCAL / EXTERNAL 관계도 여기서 정리합니다.')
            : (isHost ? 'Host Vault 키를 다른 볼트로 보냅니다.' : '선택한 키를 다른 볼트나 Host Vault로 보냅니다.'),
        saveForm: isHost
            ? (isConfigItem ? 'save-host-config' : 'save-host-key')
            : (isConfigItem ? 'save-agent-config' : 'save-key'),
        createTitle: isConfigItem ? '새 환경값' : '새 키',
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
    if (activeTab() === '연결 관리') return t('bindings');
    if (activeTab() === '영향도') return t('impact');
    if (activeTab() === '실행') return t('function_run');
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
    return activeTab() === '관리자' ? t('admin_settings') : t('ui_settings');
}

function settingsRightPaneTitle() {
    return activeTab() === '관리자' ? t('admin_setting_detail') : t('edit_ui_config');
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
    }
}

async function loadSelectedKeyDetail() {
    if (!state.selectedVault || !state.selectedKey) {
        state.keyDetail = null;
        return;
    }
    state.keyDetail = await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash) + '/keys/' + encodeURIComponent(state.selectedKey.name));
}

async function loadSelectedKeySummary() {
    if (!state.selectedVault || !state.selectedKey) {
        state.keySummary = null;
        return;
    }
    state.keySummary = await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash) + '/keys/' + encodeURIComponent(state.selectedKey.name) + '/summary');
}

async function loadSelectedKeyBindings() {
    if (!state.selectedVault || !state.selectedKey) {
        state.keyBindings = [];
        return;
    }
    const data = await request('/api/vaults/' + encodeURIComponent(state.selectedVault.vault_runtime_hash) + '/keys/' + encodeURIComponent(state.selectedKey.name) + '/bindings');
    state.keyBindings = data.bindings || [];
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
    if (state.configVaultItems.length && !state.selectedConfigKey) {
        state.selectedConfigKey = state.configVaultItems[0].key;
    }
}

async function loadSelectedConfigDetail() {
    if (!state.selectedConfigVault || !state.selectedConfigKey) {
        state.configDetail = null;
        return;
    }
    state.configDetail = await request('/api/agents/' + encodeURIComponent(state.selectedConfigVault) + '/configs/' + encodeURIComponent(state.selectedConfigKey));
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
    }
}

async function loadHostVaultConfigs() {
    const data = await request('/api/host-vault/configs');
    state.hostVaultConfigs = data.configs || [];
    if (state.hostVaultConfigs.length && !state.hostSelectedConfigKey) {
        state.hostSelectedConfigKey = state.hostVaultConfigs[0].key;
    }
}

async function loadHostVaultKeyDetail() {
    if (!state.hostSelectedKey) {
        state.hostKeyDetail = null;
        return;
    }
    state.hostKeyDetail = await request('/api/host-vault/keys/' + encodeURIComponent(state.hostSelectedKey.name));
}

async function loadHostVaultConfigDetail() {
    if (!state.hostSelectedConfigKey) {
        state.hostConfigDetail = null;
        return;
    }
    state.hostConfigDetail = await request('/api/host-vault/configs/' + encodeURIComponent(state.hostSelectedConfigKey));
}

async function loadVaultItemSyncStatus() {
    if (!state.selectedVault || activeTab() !== '키 / 환경값') {
        state.vaultItemSyncStatus = {};
        return;
    }

    const nextStatus = {};
    const vaults = state.vaults || [];
    const normalizedConfigsByVault = new Map();
    const normalizedKeysByVault = new Map();

    await Promise.all(vaults.map(async (vault) => {
        const vaultHash = vault.vault_runtime_hash;

        try {
            const data = await request('/api/agents/' + encodeURIComponent(vaultHash) + '/configs');
            const configs = new Map();
            (data.configs || []).forEach((item) => {
                configs.set(item.key, {
                    exists: true,
                    value: item.value ?? null,
                    scope: item.scope || 'LOCAL',
                    status: item.status || 'active'
                });
            });
            normalizedConfigsByVault.set(vaultHash, configs);
        } catch (_) {
            normalizedConfigsByVault.set(vaultHash, new Map());
        }

        try {
            const data = await request('/api/vaults/' + encodeURIComponent(vaultHash) + '/keys');
            const keys = new Map();
            (data.secrets || []).forEach((item) => {
                keys.set(item.name, {
                    exists: true,
                    token: item.token || null,
                    ref: item.ref || null,
                    scope: item.scope || 'LOCAL',
                    status: item.status || 'active'
                });
            });
            normalizedKeysByVault.set(vaultHash, keys);
        } catch (_) {
            normalizedKeysByVault.set(vaultHash, new Map());
        }
    }));

    state.configVaultItems.forEach((item) => {
        let matched = 0;
        let present = 0;
        const baseline = {
            value: item.value ?? null,
            scope: item.scope || 'LOCAL',
            status: item.status || 'active'
        };
        vaults.forEach((vault) => {
            const entry = normalizedConfigsByVault.get(vault.vault_runtime_hash)?.get(item.key);
            if (!entry) return;
            present += 1;
            if (entry.value === baseline.value && entry.scope === baseline.scope && entry.status === baseline.status) {
                matched += 1;
            }
        });
        nextStatus[vaultItemSyncKey('VE', item.key)] = { matched, total: Math.max(present, 1) };
    });

    state.vaultKeys.forEach((item) => {
        let matched = 0;
        let present = 0;
        const baseline = {
            token: item.token || null,
            ref: item.ref || null,
            scope: item.scope || 'LOCAL',
            status: item.status || 'active'
        };
        vaults.forEach((vault) => {
            const entry = normalizedKeysByVault.get(vault.vault_runtime_hash)?.get(item.name);
            if (!entry) return;
            present += 1;
            if (entry.token === baseline.token && entry.ref === baseline.ref && entry.scope === baseline.scope && entry.status === baseline.status) {
                matched += 1;
            }
        });
        nextStatus[vaultItemSyncKey('VK', item.name)] = { matched, total: Math.max(present, 1) };
    });

    state.vaultItemSyncStatus = nextStatus;
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
    const kind = activeTab() === '환경값' ? 'VE' : 'VK';
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
                if (activeTab() !== 'Host Vault') {
                    await loadSelectedVaultKeys();
                    state.selectedConfigVault = state.selectedVault.vault_runtime_hash;
                    await loadConfigsForVault();
                    await loadVaultItemSyncStatus();
                }
                if (activeTab() === 'Host Vault') {
                    await loadHostVaultKeys();
                    await loadHostVaultConfigs();
                    const useHostConfigDetail = state.vaultItemKind === 'VE'
                        || (state.vaultItemKind === 'ALL' && state.hostSelectedItemKind === 'VE');
                    if (useHostConfigDetail) {
                        await loadHostVaultConfigDetail();
                    } else {
                        await loadHostVaultKeyDetail();
                    }
                } else if (activeTab() === '키 / 환경값') {
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
                if (activeTab() === '연결 관리') {
                    await loadSelectedFunctionBindings();
                } else if (activeTab() === '영향도') {
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
    if (!(state.activePage === 'vaults' && tab === '키 / 환경값')) {
        state.routeSelectedVaultHash = null;
    }
    syncRoute(false);
    await syncPageData();
}

async function selectVaultByKey(key) {
    const vault = state.vaults.find((item) => item.vault_runtime_hash === key);
    if (!vault) return;
    state.selectedVault = vault;
    state.activeTabByPage.vaults = '키 / 환경값';
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
    } else if (state.activePage === 'audit' && activeTab() === '키') {
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
            const isHostVault = activeTab() === 'Host Vault';
            const targetVault = String(data.get('target_vault') || '').trim();
            const targetScope = String(data.get('target_scope') || 'TEMP').trim();
            const value = String(data.get('move_value') || '').trim() || (isHostVault ? (state.hostKeyDetail?.value || '') : (state.keyDetail?.value || ''));
            const name = String(data.get('name') || '').trim() || (isHostVault ? (state.hostSelectedKey?.name || state.hostKeyDetail?.name || '') : (state.selectedKey?.name || state.keyDetail?.name || ''));
            if (!targetVault || !name || !value) throw new Error('격상할 키 값이 없습니다.');
            if (targetVault === 'host') {
                await request('/api/host-vault/keys', {
                    method: 'POST',
                    body: JSON.stringify({ name, value, scope: targetScope })
                });
            } else {
                if (targetScope !== 'TEMP') {
                    throw new Error('로컬 볼트 VK 격상은 아직 TEMP만 지원합니다. Host Vault를 선택하면 LOCAL / EXTERNAL도 사용할 수 있습니다.');
                }
                await request('/api/vaults/' + encodeURIComponent(targetVault) + '/keys', {
                    method: 'POST',
                    body: JSON.stringify({ name, value })
                });
            }
            setMessage('ok', '키를 선택한 볼트로 보냈습니다.');
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
            const isHostVault = activeTab() === 'Host Vault';
            const targetVault = String(data.get('target_vault') || '').trim();
            const targetScope = String(data.get('target_scope') || 'LOCAL').trim();
            const key = String(data.get('key') || '').trim() || (isHostVault ? (state.hostSelectedConfigKey || state.hostConfigDetail?.key || '') : (state.selectedConfigKey || state.configDetail?.key || ''));
            const value = String(data.get('move_value') || '').trim() || (isHostVault ? (state.hostConfigDetail?.value || '') : (state.configDetail?.value || ''));
            if (!targetVault || !key || !value) throw new Error('격상할 환경값이 없습니다.');
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
            setMessage('ok', '환경값을 선택한 볼트로 보냈습니다.');
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
            if (activeTab() === 'Host Vault') {
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
                setMessage('ok', '값을 복사했습니다.');
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
    applyRoute(window.location.pathname);
    syncPageData();
  }

async function boot() {
    applyRoute(window.location.pathname);
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
                renderConfigRelations,
                configRelationsByScope,
                encodeURIComponent
    };
}
