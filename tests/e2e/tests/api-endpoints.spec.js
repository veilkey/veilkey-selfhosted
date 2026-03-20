import { test, expect } from '@playwright/test';

test.describe('API Health', () => {
  test('GET /health returns ok', async ({ request }) => {
    const resp = await request.get('/health');
    expect(resp.ok()).toBeTruthy();
    const body = await resp.json();
    expect(body.status).toBe('ok');
  });

  test('GET /ready returns ok', async ({ request }) => {
    const resp = await request.get('/ready');
    expect(resp.ok()).toBeTruthy();
  });
});

test.describe('API Status', () => {
  test('GET /api/status returns node info', async ({ request }) => {
    const resp = await request.get('/api/status');
    expect(resp.ok()).toBeTruthy();
    const body = await resp.json();
    expect(body.locked).toBe(false);
    expect(body.node_id).toBeTruthy();
    expect(body.mode).toBe('hkm');
    expect(body.version).toBeGreaterThanOrEqual(1);
  });

  test('status includes supported features', async ({ request }) => {
    const resp = await request.get('/api/status');
    const body = await resp.json();
    expect(body.supported_features).toContain('status');
    expect(body.supported_features).toContain('secrets');
    expect(body.supported_features).toContain('configs');
  });
});

test.describe('API Data Endpoints', () => {
  test('GET /api/vaults returns vault list', async ({ request }) => {
    const resp = await request.get('/api/vaults?limit=10');
    expect(resp.ok()).toBeTruthy();
    const body = await resp.json();
    expect(body.vaults).toBeDefined();
  });

  test('GET /api/configs/summary returns config data', async ({ request }) => {
    const resp = await request.get('/api/configs/summary');
    expect(resp.ok()).toBeTruthy();
  });

  test('GET /api/functions/global returns function list', async ({ request }) => {
    const resp = await request.get('/api/functions/global');
    expect(resp.ok()).toBeTruthy();
  });
});

test.describe('API Auth Endpoints', () => {
  test('POST /api/unlock when already unlocked', async ({ request }) => {
    const resp = await request.post('/api/unlock', {
      data: { password: 'wrongpassword' },
    });
    expect([200, 401]).toContain(resp.status());
    if (resp.status() === 200) {
      const body = await resp.json();
      expect(body.status).toBe('already_unlocked');
    }
  });

  test('POST /api/admin/session/login without code returns error', async ({ request }) => {
    const resp = await request.post('/api/admin/session/login', {
      data: { password: 'test' },
    });
    const body = await resp.json();
    expect(body.error).toBeTruthy();
  });

  test('GET /api/admin/auth/settings returns config', async ({ request }) => {
    const resp = await request.get('/api/admin/auth/settings');
    expect(resp.ok()).toBeTruthy();
  });
});

test.describe('API Registration Tokens', () => {
  test('validate with fake token returns error', async ({ request }) => {
    const resp = await request.get('/api/registration-tokens/fake-token/validate');
    expect(resp.status()).toBeGreaterThanOrEqual(400);
  });
});

test.describe('API Error Handling', () => {
  test('404 for unknown endpoints', async ({ request }) => {
    const resp = await request.get('/api/nonexistent-endpoint');
    expect(resp.status()).toBe(404);
  });

  test('error responses do not leak internal details', async ({ request }) => {
    const resp = await request.get('/api/registration-tokens/nonexistent/validate');
    const body = await resp.text();
    expect(body).not.toMatch(/\.go:\d+/);
    expect(body).not.toMatch(/runtime error/);
    expect(body).not.toMatch(/goroutine/);
    expect(body).not.toMatch(/sql:/);
    expect(body).not.toMatch(/gorm/);
  });

  test('POST with invalid JSON returns 400 or already_unlocked', async ({ request }) => {
    const resp = await request.post('/api/unlock', {
      headers: { 'Content-Type': 'application/json' },
      data: 'not valid json{{{',
    });
    // If already unlocked, server may return 200 before parsing body
    expect([200, 400]).toContain(resp.status());
  });
});
