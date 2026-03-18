<template>
  <div class="setup-shell">
    <div class="setup-card">
      <div class="setup-brand">
        <span class="brand-mark">VK</span>
        <span class="brand-name">VeilKey</span>
      </div>

      <!-- Init form -->
      <template v-if="phase === 'form'">
        <h1 class="setup-heading">{{ t.heading }}</h1>
        <p class="setup-desc">{{ t.desc }}</p>

        <div v-if="error" class="error-banner">{{ error }}</div>

        <form @submit.prevent="handleSubmit" class="setup-form">
          <div class="form-group">
            <label class="form-label">{{ t.password }}</label>
            <input
              class="form-input"
              type="password"
              v-model="password"
              :placeholder="t.passwordPh"
              autocomplete="new-password"
              required
              minlength="8"
            />
            <span v-if="password && password.length < 8" class="field-hint error">
              {{ t.minChars }}
            </span>
          </div>

          <div class="form-group">
            <label class="form-label">{{ t.passwordConfirm }}</label>
            <input
              class="form-input"
              type="password"
              v-model="passwordConfirm"
              :placeholder="t.passwordConfirmPh"
              autocomplete="new-password"
              required
            />
            <span v-if="passwordConfirm && password !== passwordConfirm" class="field-hint error">
              {{ t.mismatch }}
            </span>
          </div>

          <button
            type="submit"
            class="btn-primary"
            :disabled="!canSubmit || loading"
          >
            <span v-if="loading" class="spinner"></span>
            {{ loading ? t.initializing : t.initBtn }}
          </button>
        </form>
      </template>

      <!-- Success / VK:TEMP ref display -->
      <template v-else-if="phase === 'done'">
        <div class="success-icon">✓</div>
        <h1 class="setup-heading">{{ t.successHeading }}</h1>
        <p class="setup-desc">{{ t.successDesc }}</p>

        <div class="ref-box">
          <div class="ref-label">{{ t.tempRefLabel }}</div>
          <div class="ref-value">
            <code>{{ tempRef }}</code>
            <button class="copy-btn" @click="copyRef" :title="t.copy">
              {{ copied ? '✓' : t.copy }}
            </button>
          </div>
          <div class="ref-expiry">{{ t.expiresAt }}: {{ expiresAt }}</div>
        </div>

        <div class="retrieve-box">
          <div class="retrieve-label">{{ t.retrieveLabel }}</div>
          <code class="retrieve-cmd">curl {{ origin }}/api/resolve/{{ tempRef }}</code>
        </div>

        <div class="warning-box">
          <strong>{{ t.warningTitle }}</strong>
          <p>{{ t.warningBody }}</p>
        </div>

        <div v-if="restarting" class="restart-banner">
          <span class="spinner"></span> {{ t.restarting }}
        </div>
        <div v-else-if="ready" class="ready-banner">
          {{ t.ready }}
          <a :href="origin" class="goto-btn">{{ t.gotoAdmin }}</a>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'

const phase = ref('form')
const password = ref('')
const passwordConfirm = ref('')
const loading = ref(false)
const error = ref(null)
const tempRef = ref('')
const expiresAt = ref('')
const copied = ref(false)
const restarting = ref(false)
const ready = ref(false)
const origin = window.location.origin

const lang = navigator.language?.startsWith('ko') ? 'ko' : 'en'

const i18n = {
  ko: {
    heading: 'VaultCenter 초기 설정',
    desc: '마스터 비밀번호를 설정합니다. 이 비밀번호는 모든 암호화 키를 보호합니다.',
    password: '마스터 비밀번호',
    passwordPh: '8자 이상 입력',
    passwordConfirm: '비밀번호 확인',
    passwordConfirmPh: '비밀번호를 다시 입력',
    minChars: '8자 이상 입력해야 합니다.',
    mismatch: '비밀번호가 일치하지 않습니다.',
    initBtn: '초기화 실행',
    initializing: '초기화 중...',
    successHeading: '초기화 완료',
    successDesc: '비밀번호가 VK:TEMP ref로 저장되었습니다. 아래 ref는 1시간 후 만료됩니다.',
    tempRefLabel: '임시 비밀번호 참조 (VK:TEMP Ref)',
    copy: '복사',
    expiresAt: '만료',
    retrieveLabel: '비밀번호 조회 명령어',
    warningTitle: '⚠️ 중요',
    warningBody: '이 ref는 1시간 후 영구 삭제됩니다. 비밀번호를 반드시 안전한 곳에 보관하세요. 비밀번호를 잃으면 모든 데이터가 복구 불가능합니다.',
    restarting: '서비스 재시작 중...',
    ready: '서비스가 준비되었습니다.',
    gotoAdmin: '관리 콘솔로 이동 →'
  },
  en: {
    heading: 'VaultCenter First-Run Setup',
    desc: 'Set a master password. This password protects all encryption keys.',
    password: 'Master Password',
    passwordPh: 'Minimum 8 characters',
    passwordConfirm: 'Confirm Password',
    passwordConfirmPh: 'Re-enter password',
    minChars: 'Must be at least 8 characters.',
    mismatch: 'Passwords do not match.',
    initBtn: 'Initialize',
    initializing: 'Initializing...',
    successHeading: 'Initialization Complete',
    successDesc: 'Your password has been stored as a VK:TEMP ref. This ref expires in 1 hour.',
    tempRefLabel: 'Temporary Password Ref (VK:TEMP)',
    copy: 'Copy',
    expiresAt: 'Expires',
    retrieveLabel: 'Retrieve password command',
    warningTitle: '⚠️ Important',
    warningBody: 'This ref will be permanently deleted in 1 hour. Store your password in a secure location (e.g. password manager). If you lose your password, all data is unrecoverable.',
    restarting: 'Service restarting...',
    ready: 'Service is ready.',
    gotoAdmin: 'Go to Admin Console →'
  }
}

const t = computed(() => i18n[lang] || i18n.ko)

const canSubmit = computed(() =>
  password.value.length >= 8 && password.value === passwordConfirm.value
)

async function handleSubmit() {
  if (!canSubmit.value) return
  loading.value = true
  error.value = null

  try {
    const res = await fetch('/api/setup/init', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password: password.value })
    })
    const data = await res.json()
    if (!res.ok) {
      error.value = data.error || `Error ${res.status}`
      return
    }
    tempRef.value = data.temp_ref || ''
    expiresAt.value = data.expires_at ? new Date(data.expires_at).toLocaleString() : ''
    phase.value = 'done'
    pollRestart()
  } catch (e) {
    error.value = String(e)
  } finally {
    loading.value = false
  }
}

async function copyRef() {
  try {
    await navigator.clipboard.writeText(tempRef.value)
    copied.value = true
    setTimeout(() => { copied.value = false }, 2000)
  } catch {}
}

async function pollRestart() {
  restarting.value = true
  // Wait a moment for the server to begin restarting
  await new Promise(r => setTimeout(r, 1500))

  const maxAttempts = 60
  for (let i = 0; i < maxAttempts; i++) {
    await new Promise(r => setTimeout(r, 2000))
    try {
      const res = await fetch('/health')
      if (res.ok) {
        const data = await res.json().catch(() => ({}))
        // Server is back up — not in setup mode
        if (data.status !== 'setup') {
          restarting.value = false
          ready.value = true
          return
        }
      }
    } catch {
      // still restarting
    }
  }
  restarting.value = false
  ready.value = true
}
</script>

<style scoped>
* { box-sizing: border-box; }

.setup-shell {
  min-height: 100vh;
  background: #0f1117;
  color: #e0e0e0;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
}

.setup-card {
  width: 100%;
  max-width: 480px;
  background: #161921;
  border: 1px solid #23263a;
  border-radius: 12px;
  padding: 40px 36px;
}

.setup-brand {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 28px;
}
.brand-mark {
  background: #5b7fff;
  color: #fff;
  font-weight: 700;
  font-size: 0.8rem;
  padding: 3px 7px;
  border-radius: 4px;
  letter-spacing: 1px;
}
.brand-name {
  font-size: 1.1rem;
  font-weight: 600;
  color: #fff;
}

.setup-heading {
  font-size: 1.35rem;
  font-weight: 700;
  color: #ffffff;
  margin: 0 0 8px;
}
.setup-desc {
  color: #8a8fa8;
  font-size: 0.9rem;
  margin: 0 0 28px;
  line-height: 1.5;
}

.error-banner {
  background: #2a1a1a;
  border: 1px solid #7b2f2f;
  color: #f08080;
  border-radius: 6px;
  padding: 10px 14px;
  font-size: 0.875rem;
  margin-bottom: 16px;
}

.setup-form { display: flex; flex-direction: column; gap: 18px; }

.form-group { display: flex; flex-direction: column; gap: 6px; }
.form-label { font-size: 0.85rem; font-weight: 500; color: #b0b5cc; }
.form-input {
  background: #1e2131;
  border: 1px solid #2e3150;
  border-radius: 6px;
  color: #e0e0e0;
  font-size: 0.95rem;
  padding: 10px 12px;
  outline: none;
  transition: border-color 0.15s;
}
.form-input:focus { border-color: #5b7fff; }
.field-hint { font-size: 0.8rem; }
.field-hint.error { color: #f08080; }

.btn-primary {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  background: #5b7fff;
  color: #fff;
  border: none;
  border-radius: 6px;
  font-size: 0.95rem;
  font-weight: 600;
  padding: 11px 20px;
  cursor: pointer;
  transition: background 0.15s, opacity 0.15s;
  margin-top: 4px;
}
.btn-primary:hover:not(:disabled) { background: #4a6ee8; }
.btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }

.spinner {
  display: inline-block;
  width: 14px; height: 14px;
  border: 2px solid rgba(255,255,255,0.3);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }

/* Success state */
.success-icon {
  width: 48px; height: 48px;
  background: #1a3a2a;
  border: 2px solid #3a7d5c;
  border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  font-size: 1.4rem;
  color: #4caf7d;
  margin-bottom: 16px;
}

.ref-box {
  background: #1a1d2e;
  border: 1px solid #2e3150;
  border-radius: 8px;
  padding: 16px;
  margin: 20px 0;
}
.ref-label {
  font-size: 0.78rem;
  color: #8a8fa8;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 8px;
}
.ref-value {
  display: flex;
  align-items: center;
  gap: 10px;
}
.ref-value code {
  flex: 1;
  font-family: 'Courier New', Courier, monospace;
  font-size: 0.95rem;
  color: #a8d0ff;
  word-break: break-all;
}
.copy-btn {
  background: #2e3150;
  border: 1px solid #3e4270;
  border-radius: 4px;
  color: #b0b5cc;
  font-size: 0.75rem;
  padding: 4px 10px;
  cursor: pointer;
  white-space: nowrap;
  transition: background 0.15s;
}
.copy-btn:hover { background: #3a3d60; }
.ref-expiry {
  margin-top: 8px;
  font-size: 0.8rem;
  color: #e0a040;
}

.retrieve-box {
  background: #14181f;
  border: 1px solid #1e2233;
  border-radius: 6px;
  padding: 14px;
  margin-bottom: 16px;
}
.retrieve-label {
  font-size: 0.78rem;
  color: #8a8fa8;
  margin-bottom: 8px;
}
.retrieve-cmd {
  font-family: 'Courier New', Courier, monospace;
  font-size: 0.82rem;
  color: #a0e0a0;
  word-break: break-all;
  display: block;
}

.warning-box {
  background: #1f1a10;
  border: 1px solid #5a4010;
  border-radius: 6px;
  padding: 14px;
  font-size: 0.85rem;
  line-height: 1.5;
  color: #d4a040;
  margin-bottom: 20px;
}
.warning-box strong { display: block; margin-bottom: 4px; color: #e0b050; }
.warning-box p { margin: 0; }

.restart-banner {
  display: flex;
  align-items: center;
  gap: 10px;
  color: #8a8fa8;
  font-size: 0.875rem;
  padding: 10px 0;
}

.ready-banner {
  display: flex;
  align-items: center;
  gap: 16px;
  font-size: 0.9rem;
  color: #4caf7d;
  padding: 10px 0;
}
.goto-btn {
  background: #5b7fff;
  color: #fff;
  border: none;
  border-radius: 6px;
  padding: 8px 16px;
  font-size: 0.875rem;
  font-weight: 600;
  text-decoration: none;
  cursor: pointer;
  transition: background 0.15s;
}
.goto-btn:hover { background: #4a6ee8; }
</style>
