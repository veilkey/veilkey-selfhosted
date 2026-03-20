import { test, expect } from '@playwright/test';

test.describe('Health & Status', () => {
  test('GET /health returns ok', async ({ request }) => {
    const resp = await request.get('/health');
    expect(resp.ok()).toBeTruthy();
    const body = await resp.json();
    expect(body.status).toBe('ok');
  });

  test('GET /api/status returns unlocked node info', async ({ request }) => {
    const resp = await request.get('/api/status');
    expect(resp.ok()).toBeTruthy();
    const body = await resp.json();
    expect(body.locked).toBe(false);
    expect(body.node_id).toBeTruthy();
    expect(body.mode).toBe('hkm');
  });

  test('GET /ready returns ok', async ({ request }) => {
    const resp = await request.get('/ready');
    expect(resp.ok()).toBeTruthy();
  });
});
