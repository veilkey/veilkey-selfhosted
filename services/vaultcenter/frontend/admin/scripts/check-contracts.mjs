import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const appVue = fs.readFileSync(path.join(root, 'src', 'App.vue'), 'utf8');
const useAdminApp = fs.readFileSync(path.join(root, 'src', 'useAdminApp.js'), 'utf8');
const adminConfig = fs.readFileSync(path.join(root, 'src', 'adminConfig.js'), 'utf8');

function fail(message) {
  console.error(message);
  process.exit(1);
}

const template = appVue.split('<template>')[1]?.split('</template>')[0] || '';
const scriptSetup = appVue.split('<script setup>')[1]?.split('</script>')[0] || '';

if (!template || !scriptSetup) {
  fail('App.vue contract check failed: missing template or <script setup>.');
}

const destructureMatch = scriptSetup.match(/const\s*\{([\s\S]*?)\}\s*=\s*useAdminApp\(\)/);
if (!destructureMatch) {
  fail('App.vue contract check failed: useAdminApp() destructure block not found.');
}

const destructured = new Set(
  destructureMatch[1]
    .split('\n')
    .map((line) => line.trim().replace(/,$/, ''))
    .filter(Boolean)
);

const importedPageConfig = /import\s*\{\s*pageConfig\s*\}\s*from\s*['"]\.\/adminConfig['"]/.test(scriptSetup);
if (template.includes('pageConfig.settings.tabs') && !importedPageConfig) {
  fail('App.vue contract check failed: template uses pageConfig.settings.tabs but pageConfig is not imported.');
}

const returnMatch = useAdminApp.match(/return\s*\{([\s\S]*?)\n\s*\};?\s*$/);
if (!returnMatch) {
  fail('useAdminApp contract check failed: return block not found.');
}

const returned = new Set(
  returnMatch[1]
    .split('\n')
    .map((line) => line.trim().replace(/,$/, ''))
    .filter(Boolean)
);

for (const name of destructured) {
  if (name === 'encodeURIComponent') continue;
  if (name === 'pageConfig') continue;
  if (!returned.has(name)) {
    fail(`useAdminApp contract check failed: App.vue destructures "${name}" but useAdminApp() does not return it.`);
  }
}

const helperCalls = new Set(
  [...template.matchAll(/\b([A-Za-z_][A-Za-z0-9_]*)\s*\(/g)]
    .map((match) => match[1])
    .filter((name) => !['if', 'for', 'encodeURIComponent'].includes(name))
);

const importedHelpers = new Set(
  [...useAdminApp.matchAll(/import\s*\{\s*([\s\S]*?)\s*\}\s*from\s*['"][^'"]+['"]/g)]
    .flatMap((match) =>
      match[1]
        .split(',')
        .map((name) => name.trim())
        .filter(Boolean)
    )
);

for (const helper of helperCalls) {
  if (helper === 'pageConfig') continue;
  if (!destructured.has(helper)) {
    fail(`App.vue contract check failed: template calls "${helper}()" but it is not destructured from useAdminApp().`);
  }
  const functionPattern = new RegExp(`function\\s+${helper}\\s*\\(`);
  const arrowPattern = new RegExp(`(?:const|let|var)\\s+${helper}\\s*=\\s*(?:async\\s*)?\\(`);
  if (!functionPattern.test(useAdminApp) && !arrowPattern.test(useAdminApp) && !importedHelpers.has(helper)) {
    fail(`useAdminApp contract check failed: template calls "${helper}()" but no function definition for "${helper}" exists in useAdminApp.js.`);
  }
}

function extractLiteralActions(source) {
  return new Set([
    ...[...source.matchAll(/data-action="([^"]+)"/g)].map((match) => match[1]),
    ...[...source.matchAll(/'((?:set|select|refresh|delete|toggle|clear|copy|run|jump|load|audit)-[^']+)'/g)].map((match) => match[1])
  ]);
}

const templateActions = extractLiteralActions(template);
const handledActions = new Set(
  [...useAdminApp.matchAll(/if \(action === '([^']+)'\)/g)].map((match) => match[1])
);

for (const action of templateActions) {
  if (!handledActions.has(action)) {
    fail(`useAdminApp contract check failed: template uses data-action="${action}" but handleAction() does not handle it.`);
  }
}

if (template.includes('data-tab="BULK_APPLY"') || template.includes('activeTab() === \'BULK_APPLY\'')) {
  if (!/bulkApplyView/.test(useAdminApp) || !/bulkApplyTemplates/.test(useAdminApp) || !/bulkApplyWorkflows/.test(useAdminApp)) {
    fail('useAdminApp contract check failed: bulk-apply UI exists but bulk apply state is not fully initialized.');
  }
  if (!/tab === 'BULK_APPLY'/.test(useAdminApp) || !/bulk-apply/.test(useAdminApp)) {
    fail('useAdminApp contract check failed: bulk-apply tab exists but route/query handling is missing.');
  }
}

const pageConfigMatch = adminConfig.match(/export const pageConfig = (\{[\s\S]*?\n\});/);
const routeEntriesMatch = adminConfig.match(/export const routeEntries = (\[[\s\S]*?\n\]);/);
if (!pageConfigMatch || !routeEntriesMatch) {
  fail('adminConfig contract check failed: unable to parse pageConfig or routeEntries.');
}

const pageConfig = Function(`"use strict"; return (${pageConfigMatch[1]});`)();
const routeEntries = Function(`"use strict"; return (${routeEntriesMatch[1]});`)();
const routeSet = new Set(routeEntries.map((entry) => `${entry.page}::${entry.tab}`));

for (const [page, config] of Object.entries(pageConfig)) {
  for (const tab of config.tabs || []) {
    if (page === 'vaults' && ['VAULT_ITEMS', 'BULK_APPLY'].includes(tab)) {
      continue;
    }
    if (!routeSet.has(`${page}::${tab}`)) {
      fail(`adminConfig contract check failed: missing route entry for ${page} / ${tab}.`);
    }
  }
}

const stateMatch = useAdminApp.match(/const state = reactive\((\{[\s\S]*?\n\})\);/);
if (!stateMatch) {
  fail('useAdminApp contract check failed: reactive state block not found.');
}

const stateObject = Function(`"use strict"; return (${stateMatch[1]});`)();
const stateRefs = new Set([...template.matchAll(/\bstate\.([A-Za-z_][A-Za-z0-9_]*)/g)].map((match) => match[1]));
const stateUIRefs = new Set([...template.matchAll(/\bstate\.ui\.([A-Za-z_][A-Za-z0-9_]*)/g)].map((match) => match[1]));

for (const key of stateRefs) {
  if (!(key in stateObject)) {
    fail(`useAdminApp contract check failed: template reads state.${key} but it is not initialized in reactive state.`);
  }
}

if (!stateObject.ui || typeof stateObject.ui !== 'object') {
  fail('useAdminApp contract check failed: state.ui is not initialized as an object.');
}

for (const key of stateUIRefs) {
  if (!(key in stateObject.ui)) {
    fail(`useAdminApp contract check failed: template reads state.ui.${key} but it is not initialized in state.ui.`);
  }
}

console.log('Admin UI contract check passed.');
