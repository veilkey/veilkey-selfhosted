import { reactive } from 'vue'

export const store = reactive({
  lang: 'ko',
  initialized: false,
  vaultcenterUrl: '',
  registrationToken: '',
  password: '',
  passwordConfirm: '',
  loading: false,
  error: null,
  status: null  // from /api/install/status
})

export async function loadStatus() {
  try {
    const res = await fetch('/api/install/status')
    if (res.ok) {
      store.status = await res.json()
      store.initialized = store.status.initialized
      store.vaultcenterUrl = store.status.vaultcenter_url || ''
    }
  } catch (e) {
    store.error = e.message
  }
}

export async function runInit() {
  store.loading = true
  store.error = null
  try {
    const res = await fetch('/api/install/init', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        password: store.password,
        vaultcenter_url: store.vaultcenterUrl,
        registration_token: store.registrationToken.trim()
      })
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(text)
    }
    const result = await res.json()
    store.registrationToken = ''
    return result
  } catch (e) {
    store.error = e.message
    return null
  } finally {
    store.loading = false
  }
}

export async function updateVaultcenterUrl() {
  store.loading = true
  store.error = null
  try {
    const res = await fetch('/api/install/vaultcenter-url', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ vaultcenter_url: store.vaultcenterUrl })
    })
    if (!res.ok) throw new Error(await res.text())
    store.status = await res.json()
    return true
  } catch (e) {
    store.error = e.message
    return false
  } finally {
    store.loading = false
  }
}
