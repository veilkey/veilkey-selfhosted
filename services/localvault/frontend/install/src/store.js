import { reactive } from 'vue'

export const store = reactive({
  lang: 'ko',
  initialized: false,
  keycenterUrl: '',
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
      store.keycenterUrl = store.status.keycenter_url || ''
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
        keycenter_url: store.keycenterUrl
      })
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(text)
    }
    return await res.json()
  } catch (e) {
    store.error = e.message
    return null
  } finally {
    store.loading = false
  }
}

export async function updateKeycenterUrl() {
  store.loading = true
  store.error = null
  try {
    const res = await fetch('/api/install/keycenter-url', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ keycenter_url: store.keycenterUrl })
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
