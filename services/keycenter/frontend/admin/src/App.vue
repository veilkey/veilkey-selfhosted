<template>
<div class="app" data-app="keycenter-admin-shell">
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
                            :href="routePath('vaults', '전체 볼트')"
                            class="nav-item"
                            :class="{ active: activeTab() === '전체 볼트' }"
                            data-action="set-tab"
                            data-tab="전체 볼트"
                        >
                            <span class="nav-item-main"><span>{{ t('all_vaults') }}</span></span>
                        </a>
                        <a
                            :href="routePath('vaults', 'Host Vault')"
                            class="nav-item"
                            :class="{ active: activeTab() === 'Host Vault' }"
                            data-action="set-tab"
                            data-tab="Host Vault"
                        >
                            <span class="nav-item-main"><span>{{ t('host_vault') }}</span></span>
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
                            :class="{ active: activeTab() !== 'Host Vault' && state.selectedVault && vault.vault_runtime_hash === state.selectedVault.vault_runtime_hash }"
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
                    <div class="sidebar-label">{{ t('section_audit_vaults') }}</div>
                    <div class="nav-list">
                        <a
                            v-for="vault in filteredVaults()"
                            :key="vault.vault_runtime_hash"
                            :href="routePath('audit', '감사 로그')"
                            class="nav-item"
                            :class="{ active: state.auditVault === vault.vault_runtime_hash }"
                            data-action="audit-page-select-vault"
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
                                <span>{{ tabName }}</span>
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
                            <template v-if="activeTab() === '전체 볼트'">
                                <div class="toolbar">
                                    <div class="toolbar-group">
                                        <input class="field context-search" id="vault-search" type="search" placeholder="볼트 검색" :value="state.globalQuery">
                                    </div>
                                    <span class="pill">{{ allVaultRows().length }}개</span>
                                    <button class="btn btn-soft" data-action="refresh-vaults">새로고침</button>
                                </div>
                            </template>
                            <template v-else>
                                <div class="toolbar">
                                    <div class="toolbar-group">
                                        <div class="toolbar-group">
                                            <span class="segmented-label">표시 대상</span>
                                            <div class="segmented" role="tablist" aria-label="표시 대상">
                                                <button class="btn" :class="state.vaultItemKind === 'ALL' ? 'btn-primary' : 'btn-soft'" data-action="set-vault-kind" data-kind="ALL" :aria-pressed="state.vaultItemKind === 'ALL' ? 'true' : 'false'">전체</button>
                                                <button class="btn" :class="state.vaultItemKind === 'VE' ? 'btn-primary' : 'btn-soft'" data-action="set-vault-kind" data-kind="VE" :aria-pressed="state.vaultItemKind === 'VE' ? 'true' : 'false'">환경값</button>
                                                <button class="btn" :class="state.vaultItemKind === 'VK' ? 'btn-primary' : 'btn-soft'" data-action="set-vault-kind" data-kind="VK" :aria-pressed="state.vaultItemKind === 'VK' ? 'true' : 'false'">키</button>
                                            </div>
                                        </div>
                                        <input class="field context-search" id="key-search" type="search" :placeholder="activeTab() === 'Host Vault' ? '호스트 볼트 안에서 검색' : '현재 볼트 안에서 검색'" :value="state.globalQuery">
                                    </div>
                                    <span class="pill">{{ vaultVisibleRows().length }}개 항목</span>
                                    <button v-if="activeTab() !== 'Host Vault'" class="btn btn-primary" data-action="new-key">{{ state.vaultItemKind === 'VE' ? '새 환경값' : '새 키' }}</button>
                                </div>
                            </template>
                        </div>
                        <div class="pane-content">
                            <div v-if="activeTab() === '전체 볼트'" class="table-wrap">
                                <table>
                                    <thead>
                                        <tr>
                                            <th>볼트명</th>
                                            <th>식별자</th>
                                            <th>경로</th>
                                            <th>IP</th>
                                            <th>상태</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        <tr
                                            v-for="row in allVaultRows()"
                                            :key="row.is_host ? 'host' : row.vault_runtime_hash"
                                            class="is-clickable"
                                            :class="{ 'is-selected': (activeTab() === 'Host Vault' && row.is_host) || (activeTab() !== 'Host Vault' && !row.is_host && state.selectedVault && row.vault_runtime_hash === state.selectedVault.vault_runtime_hash) }"
                                            :data-action="row.is_host ? 'set-tab' : 'select-vault'"
                                            :data-tab="row.is_host ? 'Host Vault' : null"
                                            :data-key="row.is_host ? null : row.vault_runtime_hash"
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
                            <div v-else class="table-wrap">
                                <table>
                                    <thead>
                                        <tr>
                                            <th>종류</th>
                                            <th>키명</th>
                                            <th>키값</th>
                                            <th v-if="activeTab() === 'Host Vault'">범위</th>
                                            <template v-else>
                                                <th>동기화여부</th>
                                                <th>키 분류</th>
                                                <th>분포</th>
                                            </template>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        <tr
                                            v-for="row in vaultVisibleRows()"
                                            :key="row.item_kind + ':' + row.name"
                                            class="is-clickable"
                                            :class="{ 'is-selected': row.name === currentVaultSelectedName() && row.item_kind === currentVaultSelectedKind() }"
                                            :data-action="activeTab() === 'Host Vault' ? 'select-host-item' : 'select-vault-item'"
                                            :data-kind="row.item_kind"
                                            :data-key="row.name"
                                        >
                                            <td><span class="pill" :class="row.item_kind === 'VE' ? 'kind-ve' : 'kind-vk'">{{ vaultKindLabel(row.item_kind) }}</span></td>
                                            <td>{{ row.name }}</td>
                                            <td><span class="code">{{ vaultItemIdentifier(row) }}</span></td>
                                            <td v-if="activeTab() === 'Host Vault'"><span class="status-pill" :class="scopeClass(row.scope || (row.item_kind === 'VE' ? 'LOCAL' : 'TEMP'))">{{ row.scope || (row.item_kind === 'VE' ? 'LOCAL' : 'TEMP') }}</span></td>
                                            <template v-else>
                                                <td>
                                                    <span
                                                        v-if="vaultSyncStatus(row.item_kind, row.name).loading"
                                                        class="muted"
                                                    >확인중</span>
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
                                                    >확인중</span>
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
                                                    >확인중</span>
                                                    <span
                                                        v-else
                                                        class="status-pill"
                                                        :class="vaultDistributionStatus(row.item_kind, row.name).className"
                                                    >{{ vaultDistributionStatus(row.item_kind, row.name).label }}</span>
                                                </td>
                                            </template>
                                        </tr>
                                        <tr v-if="!vaultVisibleRows().length">
                                            <td :colspan="activeTab() === 'Host Vault' ? 4 : 6"><div class="empty">표시할 행이 없습니다.</div></td>
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
                            <template v-if="activeTab() === '전체 볼트'">
                                <div v-if="selectedInventoryDetail()" class="stack">
                                    <div class="card">
                                        <div class="card-title">선택된 볼트</div>
                                        <div class="inline-grid">
                                            <div class="kv"><span class="label">이름</span><span class="value">{{ selectedInventoryDetail().display_name || selectedInventoryDetail().vault_name || '-' }}</span></div>
                                            <div class="kv"><span class="label">식별자</span><span class="value">{{ selectedInventoryDetail().vault_id || selectedInventoryDetail().vault_runtime_hash || '-' }}</span></div>
                                            <div class="kv"><span class="label">경로</span><span class="value">{{ ((selectedInventoryDetail().managed_paths && selectedInventoryDetail().managed_paths[0]) || '-') }}</span></div>
                                            <div class="kv"><span class="label">IP</span><span class="value">{{ selectedInventoryDetail().ip || '-' }}</span></div>
                                            <div class="kv"><span class="label">상태</span><span class="value">{{ selectedInventoryDetail().status || '-' }}</span></div>
                                        </div>
                                    </div>
                                    <form class="stack" data-form="save-vault-meta">
                                        <div class="card">
                                            <div class="card-title">기본 정보</div>
                                            <div class="stack">
                                                <div class="kv"><span class="label">볼트 이름</span><input class="field" name="display_name" :value="selectedInventoryDetail().display_name || ''"></div>
                                                <div class="kv"><span class="label">설명</span><textarea class="textarea" name="description">{{ selectedInventoryDetail().description || '' }}</textarea></div>
                                                <div class="kv"><span class="label">태그 JSON</span><textarea class="textarea" name="tags_json">{{ selectedInventoryDetail().tags_json || '[]' }}</textarea></div>
                                            </div>
                                        </div>
                                        <div class="toolbar">
                                            <button class="btn btn-primary" type="submit">저장</button>
                                        </div>
                                    </form>
                                </div>
                                <div v-else class="empty">왼쪽에서 볼트를 선택하세요.</div>
                            </template>
                            <template v-else>
                                <div class="stack">
                                    <form v-if="vaultPanel().canMoveItem" class="stack" :data-form="vaultPanel().isConfigItem ? 'promote-config' : 'promote-key'">
                                        <div class="card">
                                            <div class="card-title">{{ vaultPanel().isConfigItem ? '환경값 이동' : '키 이동' }}</div>
                                            <div class="stack">
                                                <div class="kv">
                                                    <span class="label">대상 볼트</span>
                                                    <select class="select" name="target_vault">
                                                        <option v-for="option in vaultTargetOptions(true)" :key="option.value" :value="option.value" :selected="option.value === vaultPanel().targetVaultDefault">{{ option.label }}</option>
                                                    </select>
                                                </div>
                                                <div class="kv">
                                                    <span class="label">대상 범위</span>
                                                    <select class="select" name="target_scope">
                                                        <option v-for="option in vaultPanel().scopeOptions" :key="option" :value="option" :selected="option === vaultPanel().currentScope">{{ option }}</option>
                                                    </select>
                                                </div>
                                                <div class="kv">
                                                    <span class="label">보낼 값</span>
                                                    <textarea class="textarea" name="move_value" placeholder="이동할 값을 입력하거나 확인하세요">{{ vaultPanel().visibleValue || '' }}</textarea>
                                                </div>
                                            </div>
                                            <div class="muted">{{ vaultPanel().moveHelperText }}</div>
                                        </div>
                                        <button class="btn btn-soft" type="submit" :disabled="!vaultPanel().visibleValue">{{ vaultPanel().isConfigItem ? '환경값 이동' : '키 이동' }}</button>
                                    </form>
                                    <form class="stack" :data-form="vaultPanel().saveForm">
                                        <div class="card">
                                            <div class="card-title">{{ vaultPanel().detailName ? '항목 상세' : vaultPanel().createTitle }}</div>
                                            <div class="stack">
                                                <div class="kv">
                                                    <span class="label">{{ vaultPanel().isConfigItem ? '환경값 이름' : '키 이름' }}</span>
                                                    <input class="field" :name="vaultPanel().isConfigItem ? 'key' : 'name'" :value="vaultPanel().detailName || ''" :readonly="vaultPanel().detailName ? true : null">
                                                </div>
                                                <div class="kv">
                                                    <span class="label">{{ vaultPanel().isConfigItem ? '환경값' : '키 값' }}</span>
                                                    <div class="row" style="align-items:center;">
                                                        <button class="btn btn-soft" type="button" data-action="toggle-reveal">{{ state.revealValue ? '가리기' : '보기' }}</button>
                                                        <button v-if="state.revealValue && vaultPanel().visibleValue" class="btn btn-soft" type="button" data-action="copy-value">복사</button>
                                                    </div>
                                                    <textarea class="textarea" name="value" :placeholder="vaultPanel().detailName ? '새 값을 입력하면 덮어씁니다' : '필수'">{{ state.revealValue ? (vaultPanel().visibleValue || '') : '••••••••••••' }}</textarea>
                                                </div>
                                                <template v-if="vaultPanel().showScopeSelect">
                                                    <div class="kv">
                                                        <span class="label">범위</span>
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
                                                    <div class="kv"><span class="label">설명</span><textarea class="textarea" name="description">{{ vaultPanel().description }}</textarea></div>
                                                    <div class="kv"><span class="label">태그 JSON</span><textarea class="textarea" name="tags_json">{{ vaultPanel().tagsJSON }}</textarea></div>
                                                </template>
                                            </div>
                                        </div>
                                        <div class="toolbar">
                                            <button class="btn btn-primary" type="submit">{{ vaultPanel().detailName ? '저장' : '생성' }}</button>
                                            <button v-if="vaultPanel().showDelete" class="btn btn-danger" type="button" :data-action="vaultPanel().deleteAction">삭제</button>
                                        </div>
                                    </form>
                                    <template v-if="vaultPanel().isConfigItem">
                                        <details class="card" open>
                                            <summary class="card-title">LOCAL / EXTERNAL 관계</summary>
                                            <div v-if="!configRelationsByScope().length" class="empty">같은 키의 LOCAL / EXTERNAL 관계가 아직 없습니다.</div>
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
                                            <summary class="card-title">부가 정보</summary>
                                            <div class="inline-grid">
                                                <div class="kv"><span class="label">연결 수</span><span class="value">{{ vaultPanel().bindingsTotal }}</span></div>
                                                <div class="kv"><span class="label">사용 수</span><span class="value">{{ vaultPanel().usageCount }}</span></div>
                                                <div class="kv"><span class="label">범위</span><span class="value">{{ vaultPanel().currentScope }}</span></div>
                                                <div class="kv"><span class="label">상태</span><span class="value">{{ vaultPanel().status }}</span></div>
                                            </div>
                                            <div v-if="vaultPanel().bindings.length" class="stack" style="margin-top:12px;">
                                                <div v-for="item in vaultPanel().bindings" :key="item.binding_type + ':' + item.target_name + ':' + item.field_key" class="value">{{ item.binding_type }} / {{ item.target_name }} / {{ item.field_key || '-' }}</div>
                                            </div>
                                            <div v-else class="empty">연결 정보 없음</div>
                                            <div v-if="vaultPanel().recentAudit.length" class="stack" style="margin-top:12px;">
                                                <div v-for="item in vaultPanel().recentAudit" :key="(item.action || item.event_type) + ':' + (item.created_at || item.timestamp)" class="value">{{ item.action || item.event_type || 'event' }} · {{ item.created_at || item.timestamp || '-' }}</div>
                                            </div>
                                            <div v-else class="empty">감사 로그 없음</div>
                                        </details>
                                        <div v-else class="card">
                                            <div class="card-title">부가 정보</div>
                                            <div class="inline-grid">
                                                <div class="kv"><span class="label">식별자</span><span class="value">{{ vaultPanel().itemRefValue || '-' }}</span></div>
                                                <div class="kv"><span class="label">범위</span><span class="value">{{ vaultPanel().currentScope }}</span></div>
                                                <div class="kv"><span class="label">상태</span><span class="value">{{ vaultPanel().status }}</span></div>
                                                <div class="kv"><span class="label">저장소</span><span class="value">Host Vault</span></div>
                                            </div>
                                        </div>
                                    </template>
                                </div>
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
                                        <input class="field context-search" id="function-search" type="search" placeholder="함수 검색" :value="state.globalQuery">
                                    </div>
                                    <span class="pill">{{ filteredFunctions().length }}개</span>
                                    <button v-if="activeTab() === '함수 목록'" class="btn btn-soft" data-action="refresh-functions">새로고침</button>
                                    <button v-if="activeTab() === '실행'" class="btn btn-sm" type="submit" form="function-run-form">실행</button>
                                </div>
                            </div>
                            <div class="pane-content">
                                <template v-if="activeTab() === '함수 목록'">
                                    <div class="table-wrap">
                                        <table>
                                            <thead>
                                                <tr>
                                                    <th>함수</th>
                                                    <th>분류</th>
                                                    <th>해시</th>
                                                    <th>수정</th>
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
                                                    <td colspan="4"><div class="empty">등록된 글로벌 함수가 없습니다. 먼저 함수를 추가해야 합니다.</div></td>
                                                </tr>
                                            </tbody>
                                        </table>
                                    </div>
                                </template>
                                <template v-else-if="activeTab() === '연결 관리'">
                                    <div class="table-wrap">
                                        <table>
                                            <thead>
                                                <tr>
                                                    <th>Ref</th>
                                                    <th>볼트</th>
                                                    <th>필드</th>
                                                    <th>필수</th>
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
                                                    <td colspan="4"><div class="empty">선택한 함수의 연결이 없습니다.</div></td>
                                                </tr>
                                            </tbody>
                                        </table>
                                    </div>
                                </template>
                                <template v-else-if="activeTab() === '영향도'">
                                    <div v-if="state.functionSummary" class="metrics">
                                        <div class="metric"><span class="label">Bindings</span><strong>{{ state.functionSummary.bindings_total || 0 }}</strong></div>
                                        <div class="metric"><span class="label">Unique Refs</span><strong>{{ state.functionSummary.unique_refs_count || 0 }}</strong></div>
                                        <div class="metric"><span class="label">Vaults</span><strong>{{ state.functionSummary.vaults_count || 0 }}</strong></div>
                                    </div>
                                    <div class="table-wrap">
                                        <table>
                                            <thead>
                                                <tr>
                                                    <th>Ref</th>
                                                    <th>Secret</th>
                                                    <th>Field</th>
                                                    <th>Vault</th>
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
                                                    <td colspan="4"><div class="empty">선택한 함수의 영향도 데이터가 없습니다.</div></td>
                                                </tr>
                                            </tbody>
                                        </table>
                                    </div>
                                </template>
                                <template v-else>
                                    <div v-if="state.selectedFunction" class="stack">
                                        <form class="stack" id="function-run-form" data-form="run-function">
                                            <div class="form-section">
                                                <h4>함수 실행</h4>
                                                <div class="form-row">
                                                    <label>파라미터 (JSON)</label>
                                                    <textarea name="prompt" rows="4" placeholder="{&quot;key&quot;: &quot;value&quot;}"></textarea>
                                                </div>
                                                <input type="hidden" name="system_prompt" value="">
                                                <input type="hidden" name="temperature" value="0.2">
                                                <input type="hidden" name="max_output_tokens" value="2048">
                                                <input type="hidden" name="timeout_seconds" value="120">
                                            </div>
                                        </form>
                                        <div v-if="state.functionRunResult" class="card">
                                            <div class="card-title">결과</div>
                                            <pre class="code">{{ prettyJSON(state.functionRunResult) }}</pre>
                                        </div>
                                    </div>
                                    <div v-else class="empty">등록된 함수가 없어서 실행할 대상을 선택할 수 없습니다.</div>
                                </template>
                            </div>
                        </section>
                        <section class="pane" id="right-pane">
                            <div class="pane-header">
                                <div class="pane-title"><strong>{{ functionRightPaneTitle() }}</strong></div>
                            </div>
                            <div class="pane-content">
                                <template v-if="activeTab() === '함수 목록'">
                                    <div v-if="state.functionDetail" class="stack">
                                        <div class="card">
                                            <div class="card-title">선택된 함수</div>
                                            <div class="inline-grid">
                                                <div class="kv"><span class="label">함수</span><span class="value">{{ state.functionDetail.name || '-' }}</span></div>
                                                <div class="kv"><span class="label">분류</span><span class="value">{{ state.functionDetail.category || '-' }}</span></div>
                                                <div class="kv"><span class="label">해시</span><span class="value">{{ state.functionDetail.function_hash || '-' }}</span></div>
                                                <div class="kv"><span class="label">수정</span><span class="value">{{ state.functionDetail.updated_at || '-' }}</span></div>
                                            </div>
                                        </div>
                                        <div class="card">
                                            <div class="card-title">명령</div>
                                            <pre class="code">{{ state.functionDetail.command || '' }}</pre>
                                        </div>
                                    </div>
                                    <div v-else class="empty">등록된 함수가 없어서 요약을 표시할 수 없습니다.</div>
                                </template>
                                <template v-else-if="activeTab() === '연결 관리'">
                                    <div v-if="state.selectedFunction" class="stack">
                                        <form class="stack" data-form="replace-function-bindings">
                                            <div class="card">
                                                <div class="card-title">Bindings JSON</div>
                                                <textarea class="textarea" name="bindings_json">{{ prettyJSON(functionBindingsPayload()) }}</textarea>
                                            </div>
                                            <div class="toolbar">
                                                <button class="btn btn-primary" type="submit">교체</button>
                                                <button class="btn btn-danger" type="button" data-action="delete-function-bindings">전체 삭제</button>
                                            </div>
                                        </form>
                                    </div>
                                    <div v-else class="empty">등록된 함수가 없어서 연결 정보를 표시할 수 없습니다.</div>
                                </template>
                                <template v-else-if="activeTab() === '영향도'">
                                    <div v-if="state.functionSummary" class="card">
                                        <div class="card-title">요약</div>
                                        <div class="inline-grid">
                                            <div class="kv"><span class="label">Binding Type</span><span class="value">{{ state.functionSummary.binding_type || '-' }}</span></div>
                                            <div class="kv"><span class="label">Target</span><span class="value">{{ state.functionSummary.target_name || '-' }}</span></div>
                                            <div class="kv"><span class="label">Bindings</span><span class="value">{{ state.functionSummary.bindings_total || 0 }}</span></div>
                                            <div class="kv"><span class="label">Unique Refs</span><span class="value">{{ state.functionSummary.unique_refs_count || 0 }}</span></div>
                                            <div class="kv"><span class="label">Vaults</span><span class="value">{{ state.functionSummary.vaults_count || 0 }}</span></div>
                                        </div>
                                    </div>
                                    <div v-else class="empty">등록된 함수가 없어서 영향도 요약을 표시할 수 없습니다.</div>
                                </template>
                                <template v-else>
                                    <div v-if="state.selectedFunction" class="card">
                                        <div class="card-title">요약</div>
                                        <div class="inline-grid">
                                            <div class="kv"><span class="label">함수</span><span class="value">{{ state.selectedFunction.name || '-' }}</span></div>
                                            <div class="kv"><span class="label">상태</span><span class="value">{{ state.selectedFunction.status || 'active' }}</span></div>
                                        </div>
                                    </div>
                                    <div v-else class="empty">등록된 함수가 없어서 요약을 표시할 수 없습니다.</div>
                                </template>
                            </div>
                        </section>
                    </template>
                    <template v-else>
                        <template v-if="state.activePage === 'audit'">
                            <section class="pane" id="center-pane">
                                <div class="pane-header">
                                    <div class="pane-title"><strong>감사 로그</strong></div>
                                    <div class="toolbar">
                                        <span class="pill">{{ state.auditRows.length }}건</span>
                                    </div>
                                </div>
                                <div class="pane-content">
                                    <div class="table-wrap">
                                        <table>
                                            <thead>
                                                <tr>
                                                    <th>시간</th>
                                                    <th>액션</th>
                                                    <th>행위자</th>
                                                    <th>대상</th>
                                                </tr>
                                            </thead>
                                            <tbody>
                                                <tr v-for="row in state.auditRows" :key="(row.created_at || row.timestamp || '-') + ':' + (row.entity_id || '-') + ':' + (row.action || '-')">
                                                    <td>{{ row.created_at || row.timestamp || '-' }}</td>
                                                    <td>{{ row.action || '-' }}</td>
                                                    <td>{{ row.actor_type || row.actor_id || '-' }}</td>
                                                    <td>{{ row.entity_id || '-' }}</td>
                                                </tr>
                                                <tr v-if="!state.auditRows.length">
                                                    <td colspan="4"><div class="empty">표시할 행이 없습니다.</div></td>
                                                </tr>
                                            </tbody>
                                        </table>
                                    </div>
                                </div>
                            </section>
                            <section class="pane" id="right-pane">
                                <div class="pane-header">
                                    <div class="pane-title"><strong>이벤트 상세</strong></div>
                                </div>
                                <div class="pane-content">
                                    <div v-if="state.auditRows.length" class="card">
                                        <div class="card-title">Latest Event</div>
                                        <pre class="code">{{ prettyJSON(state.auditRows[0]) }}</pre>
                                    </div>
                                    <div v-else class="empty">볼트를 선택하면 상세가 표시됩니다.</div>
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
                                        <template v-else-if="activeTab() === '관리자'">
                                            <div class="card">
                                                <div class="card-title">{{ t('system_update') }}</div>
                                                <div class="stack">
                                                    <div class="value">Current deployed version and update target are managed here.</div>
                                                    <div class="value">Update execution only runs when the server has a configured update script.</div>
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
                                        <template v-else-if="activeTab() === '관리자'">
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

<script setup>
import { pageConfig } from './adminConfig';
import { useAdminApp } from './useAdminApp';

const {
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
  vaultSyncStatus,
  vaultDistributionStatus,
  vaultKeyClassStatus,
  renderConfigRelations,
  configRelationsByScope,
  encodeURIComponent
} = useAdminApp();
</script>
