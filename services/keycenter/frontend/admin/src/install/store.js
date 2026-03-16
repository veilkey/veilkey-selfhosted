import { reactive } from 'vue'

export const store = reactive({
  lang: 'ko',
  targetType: null, // 'linux' or 'lxc' — set by landing
  flow: 'wizard',
  // runtime config (loaded from API)
  config: {
    target_type: '',
    target_mode: 'new',
    target_node: '',
    target_vmid: '',
    public_base_url: '',
    install_root: '/',
    install_profile: '',
    install_script: '',
    install_workdir: '',
    keycenter_url: '',
    localvault_url: '',
    tls_mode: 'none', // UI-only: 'none' or 'existing'
    tls_cert_path: '',
    tls_key_path: '',
    tls_ca_path: '',
  },
  sessionId: null,
  loading: false,
  error: null,
})

export async function loadRuntimeConfig() {
  try {
    const res = await fetch('/api/install/runtime-config')
    if (res.ok) {
      const data = await res.json()
      Object.assign(store.config, data)
      // Derive targetType from config if not set
      if (!store.targetType && data.target_type) {
        store.targetType = data.target_type.includes('lxc') ? 'lxc' : 'linux'
      }
    }
  } catch (e) { store.error = e.message }
}

export async function saveRuntimeConfig() {
  // Map targetType to API target_type
  const target_type = store.targetType === 'lxc' ? 'lxc-allinone' : 'linux-host'
  const payload = { ...store.config, target_type }
  try {
    store.loading = true
    const res = await fetch('/api/install/runtime-config', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (!res.ok) throw new Error(await res.text())
  } catch (e) { store.error = e.message }
  finally { store.loading = false }
}

export async function saveSession(lastStage) {
  const payload = {
    session_id: store.sessionId || crypto.randomUUID(),
    language: store.lang,
    flow: store.flow,
    deployment_mode: 'host-service',
    install_scope: 'host-only',
    bootstrap_mode: 'email',
    mail_transport: 'none',
    planned_stages: ['target_select', 'configure', 'validate', 'apply'],
    last_stage: lastStage,
  }
  store.sessionId = payload.session_id
  try {
    const res = await fetch('/api/install/session', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (!res.ok) throw new Error(await res.text())
  } catch (e) { store.error = e.message }
}
