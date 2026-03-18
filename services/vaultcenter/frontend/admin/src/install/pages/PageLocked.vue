<template>
  <div class="locked-page">
    <div class="locked-card">
      <h1>VeilKey <span class="locked-badge">locked</span></h1>
      <p class="locked-message">{{ t.message }}</p>

      <div v-if="error" class="error-banner">{{ error }}</div>
      <div v-if="unlocked" class="success-banner">{{ t.unlocked }}</div>

      <form v-if="!unlocked" @submit.prevent="handleUnlock" class="unlock-form">
        <div class="form-group">
          <label class="form-label">{{ t.passwordLabel }}</label>
          <input
            class="form-input"
            type="password"
            v-model="password"
            :placeholder="t.passwordPh"
            autocomplete="current-password"
            required
          />
        </div>
        <button type="submit" class="btn-unlock" :disabled="!password || loading">
          <span v-if="loading" class="spinner"></span>
          {{ loading ? t.unlocking : t.unlockBtn }}
        </button>
      </form>

      <div class="locked-links">
        <a href="/health">Health</a>
        <a href="/api/status">Status</a>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { store } from '../store'

const router = useRouter()
const password = ref('')
const loading = ref(false)
const error = ref(null)
const unlocked = ref(false)

const t = computed(() => store.lang === 'en' ? {
  message: 'VaultCenter is locked. Enter the master password to unlock.',
  passwordLabel: 'Master Password',
  passwordPh: 'Enter password',
  unlockBtn: 'Unlock',
  unlocking: 'Unlocking...',
  unlocked: 'Unlocked. Redirecting...'
} : {
  message: 'VaultCenter가 잠겨 있습니다. 마스터 비밀번호를 입력해 잠금을 해제하세요.',
  passwordLabel: '마스터 비밀번호',
  passwordPh: '비밀번호 입력',
  unlockBtn: '잠금 해제',
  unlocking: '잠금 해제 중...',
  unlocked: '잠금 해제됨. 이동 중...'
})

async function handleUnlock() {
  if (!password.value || loading.value) return
  loading.value = true
  error.value = null

  try {
    const res = await fetch('/api/unlock', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password: password.value })
    })
    if (res.ok) {
      unlocked.value = true
      password.value = ''
      setTimeout(() => { window.location.href = '/' }, 1000)
    } else {
      const data = await res.json().catch(() => ({}))
      error.value = data.error || `Error ${res.status}`
    }
  } catch (e) {
    error.value = String(e)
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.locked-page {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 60vh;
}
.locked-card {
  background: #161921;
  border: 1px solid #23263a;
  border-radius: 12px;
  padding: 40px 36px;
  width: 100%;
  max-width: 420px;
  text-align: center;
}
h1 {
  font-size: 1.4rem;
  font-weight: 700;
  color: #fff;
  margin: 0 0 12px;
}
.locked-badge {
  display: inline-block;
  background: #5a2020;
  color: #f08080;
  font-size: 0.7rem;
  font-weight: 700;
  text-transform: uppercase;
  padding: 2px 8px;
  border-radius: 4px;
  vertical-align: middle;
  margin-left: 6px;
  letter-spacing: 0.05em;
}
.locked-message {
  color: #8a8fa8;
  font-size: 0.9rem;
  margin: 0 0 24px;
  line-height: 1.5;
}

.error-banner {
  background: #2a1a1a;
  border: 1px solid #7b2f2f;
  color: #f08080;
  border-radius: 6px;
  padding: 10px 14px;
  font-size: 0.875rem;
  margin-bottom: 14px;
  text-align: left;
}
.success-banner {
  background: #1a2a1a;
  border: 1px solid #3a7d5c;
  color: #4caf7d;
  border-radius: 6px;
  padding: 10px 14px;
  font-size: 0.875rem;
  margin-bottom: 14px;
}

.unlock-form {
  display: flex;
  flex-direction: column;
  gap: 14px;
  text-align: left;
  margin-bottom: 20px;
}
.form-group { display: flex; flex-direction: column; gap: 5px; }
.form-label { font-size: 0.82rem; font-weight: 500; color: #b0b5cc; }
.form-input {
  background: #1e2131;
  border: 1px solid #2e3150;
  border-radius: 6px;
  color: #e0e0e0;
  font-size: 0.95rem;
  padding: 10px 12px;
  outline: none;
  box-sizing: border-box;
  width: 100%;
  transition: border-color 0.15s;
}
.form-input:focus { border-color: #5b7fff; }

.btn-unlock {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  width: 100%;
  background: #5b7fff;
  color: #fff;
  border: none;
  border-radius: 6px;
  font-size: 0.95rem;
  font-weight: 600;
  padding: 11px;
  cursor: pointer;
  transition: background 0.15s, opacity 0.15s;
}
.btn-unlock:hover:not(:disabled) { background: #4a6ee8; }
.btn-unlock:disabled { opacity: 0.5; cursor: not-allowed; }

.spinner {
  display: inline-block;
  width: 14px; height: 14px;
  border: 2px solid rgba(255,255,255,0.3);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }

.locked-links {
  display: flex;
  gap: 16px;
  justify-content: center;
  margin-top: 20px;
}
.locked-links a {
  color: #5b7fff;
  font-size: 0.82rem;
  text-decoration: none;
}
.locked-links a:hover { text-decoration: underline; }
</style>
