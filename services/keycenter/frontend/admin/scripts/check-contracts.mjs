import fs from 'node:fs';
import path from 'node:path';

const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..');
const appVue = fs.readFileSync(path.join(root, 'src', 'App.vue'), 'utf8');
const useAdminApp = fs.readFileSync(path.join(root, 'src', 'useAdminApp.js'), 'utf8');

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

for (const helper of helperCalls) {
  if (helper === 'pageConfig') continue;
  if (!destructured.has(helper)) {
    fail(`App.vue contract check failed: template calls "${helper}()" but it is not destructured from useAdminApp().`);
  }
}

console.log('Admin UI contract check passed.');
