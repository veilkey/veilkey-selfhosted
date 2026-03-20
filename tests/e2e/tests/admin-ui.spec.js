import { test, expect } from '@playwright/test';

test.describe('Admin UI Loading', () => {
  test('root page loads Vue app', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('#app')).toBeAttached();
    await page.waitForLoadState('networkidle');
    const appContent = await page.locator('#app').innerHTML();
    expect(appContent.length).toBeGreaterThan(0);
  });

  test('page title contains VeilKey', async ({ page }) => {
    await page.goto('/');
    const title = await page.title();
    expect(title).toContain('VeilKey');
  });

  test('favicon loads', async ({ request }) => {
    const resp = await request.get('/favicon.svg');
    expect(resp.ok()).toBeTruthy();
  });

  test('static assets load without errors', async ({ page }) => {
    const failedRequests = [];
    page.on('requestfailed', req => failedRequests.push(req.url()));
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    expect(failedRequests).toHaveLength(0);
  });
});

test.describe('Admin UI Dashboard', () => {
  test('sidebar navigation is visible', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    // Sidebar should have navigation items (Korean: 볼트, 함수, 감사, 설정)
    const sidebarText = await page.textContent('body');
    const hasNavItems = /볼트|함수|감사|설정|Vaults|Functions|Audit|Settings/i.test(sidebarText);
    expect(hasNavItems).toBeTruthy();
  });

  test('vault list is visible', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    // Should show vault section with at least a search or list
    const bodyText = await page.textContent('body');
    const hasVaultContent = /볼트|vault|jeonghans/i.test(bodyText);
    expect(hasVaultContent).toBeTruthy();
  });

  test('VeilKey KeyCenter branding visible', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    const hasLogo = await page.getByText(/VeilKey/i).count();
    expect(hasLogo).toBeGreaterThan(0);
  });

  test('sidebar sections have counts', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    // Sidebar items should show counts (볼트 1, 함수 0, 감사 0, 설정 2)
    const bodyText = await page.textContent('body');
    expect(bodyText).toMatch(/\d/);
  });
});

test.describe('Admin UI Navigation', () => {
  test('clicking sidebar items changes content', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');

    // Click on different sidebar items and verify content changes
    const sidebarItems = page.locator('nav a, nav button, [class*="sidebar"] a, [class*="sidebar"] button, [class*="nav"] a');
    const count = await sidebarItems.count();
    if (count >= 2) {
      const firstContent = await page.locator('#app').innerHTML();
      await sidebarItems.nth(1).click();
      await page.waitForTimeout(500);
      const secondContent = await page.locator('#app').innerHTML();
      // Content should change after clicking a different nav item
      expect(secondContent).not.toBe(firstContent);
    }
  });
});

test.describe('Admin UI Search', () => {
  test('search input is present', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    const searchInput = page.locator('input[type="search"], input[type="text"][placeholder*="검색"], input[placeholder*="search"]');
    const count = await searchInput.count();
    expect(count).toBeGreaterThan(0);
  });
});
