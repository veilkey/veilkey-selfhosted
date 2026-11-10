import assert from 'node:assert/strict'
import { randomUUID } from 'node:crypto'
import test from 'node:test'
import { runInit, store } from './store.js'

test('initialization sends the registration token without retaining it after success', async (t) => {
  const token = randomUUID()
  store.registrationToken = ` ${token} `
  store.vaultcenterUrl = 'https://vaultcenter.example.com'
  store.password = randomUUID()
  let received = false
  t.mock.method(globalThis, 'fetch', async (url, options) => {
    const body = JSON.parse(options.body)
    received = url === '/api/install/init' &&
      body.registration_token === token &&
      body.vaultcenter_url === store.vaultcenterUrl
    return { ok: true, json: async () => ({ initialized: true }) }
  })
  t.after(() => {
    store.registrationToken = ''
    store.password = ''
    store.vaultcenterUrl = ''
  })
  const result = await runInit()
  assert.ok(received, 'initialization must transmit the supplied registration token')
  assert.equal(result.initialized, true)
  assert.equal(store.registrationToken, '')
  assert.equal(store.loading, false)
})
