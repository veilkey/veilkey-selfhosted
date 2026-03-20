export const pageConfig = {
    keycenter: { labelKey: 'page_keycenter', tabs: ['TEMP_REFS'] },
    vaults: { labelKey: 'page_vaults', tabs: ['ALL_VAULTS', 'VAULT_ITEMS', 'BULK_APPLY'] },
    functions: { labelKey: 'page_functions', tabs: ['FUNCTION_LIST', 'FUNCTION_BINDINGS', 'FUNCTION_IMPACT', 'FUNCTION_RUN'] },
    audit: { labelKey: 'page_audit', tabs: ['AUDIT_LOG'] },
    settings: { labelKey: 'page_settings', tabs: ['UI', 'SECURITY', 'ADMIN'] }
};

export const routeEntries = [
    { page: 'keycenter', tab: 'TEMP_REFS', path: '/keycenter' },
    { page: 'vaults', tab: 'ALL_VAULTS', path: '/vaults/all' },
    { page: 'functions', tab: 'FUNCTION_LIST', path: '/functions/list' },
    { page: 'functions', tab: 'FUNCTION_BINDINGS', path: '/functions/bindings' },
    { page: 'functions', tab: 'FUNCTION_IMPACT', path: '/functions/impact' },
    { page: 'functions', tab: 'FUNCTION_RUN', path: '/functions/run' },
    { page: 'audit', tab: 'AUDIT_LOG', path: '/audit' },
    { page: 'settings', tab: 'UI', path: '/settings/ui' },
    { page: 'settings', tab: 'SECURITY', path: '/settings/security' },
    { page: 'settings', tab: 'ADMIN', path: '/settings/admin' }
];

export const routeByPath = Object.fromEntries(routeEntries.map((entry) => [entry.path, entry]));
