<template>
  <div class="setup-page">
    <template v-if="!done">
      <h2>{{ t.heading }}</h2>
      <p class="page-desc">{{ t.desc }}</p>

      <div v-if="store.error" class="error-banner">{{ store.error }}</div>

      <form @submit.prevent="handleInit">
        <div class="form-group">
          <label class="form-label">KeyCenter URL <span class="required">*</span></label>
          <input
            class="form-input"
            type="text"
            v-model="store.keycenterUrl"
            placeholder="https://keycenter.example.com:10181"
            required
          />
        </div>

        <div class="form-group">
          <label class="form-label">{{ t.password }} <span class="required">*</span></label>
          <input
            class="form-input"
            type="password"
            v-model="store.password"
            :placeholder="t.passwordPh"
            required
            minlength="8"
          />
          <span v-if="store.password && store.password.length < 8" class="field-hint error">
            {{ t.minChars }}
          </span>
        </div>

        <div class="form-group">
          <label class="form-label">{{ t.passwordConfirm }} <span class="required">*</span></label>
          <input
            class="form-input"
            type="password"
            v-model="store.passwordConfirm"
            :placeholder="t.passwordConfirmPh"
            required
          />
          <span v-if="store.passwordConfirm && store.password !== store.passwordConfirm" class="field-hint error">
            {{ t.mismatch }}
          </span>
        </div>

        <button
          type="submit"
          class="btn-primary"
          :disabled="!canSubmit || store.loading"
        >
          <span v-if="store.loading" class="spinner"></span>
          {{ store.loading ? t.initializing : t.initBtn }}
        </button>
      </form>
    </template>

    <template v-else>
      <div class="success-banner">
        <h2>{{ t.successHeading }}</h2>
        <p>{{ t.successMsg }}</p>
        <p v-if="polling" class="polling-msg">{{ t.polling }}</p>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { store, runInit } from '../store'

const router = useRouter()
const done = ref(false)
const polling = ref(false)

const i18n = {
  ko: {
    heading: 'LocalVault 초기 설정',
    desc: 'KeyCenter URL과 비밀번호를 설정하면 LocalVault가 초기화됩니다.',
    password: '비밀번호',
    passwordPh: '8자 이상 입력',
    passwordConfirm: '비밀번호 확인',
    passwordConfirmPh: '비밀번호를 다시 입력',
    minChars: '8자 이상 입력해야 합니다.',
    mismatch: '비밀번호가 일치하지 않습니다.',
    initBtn: '초기화 실행',
    initializing: '초기화 중...',
    successHeading: '초기화 완료',
    successMsg: '초기화 완료. 서비스가 재시작됩니다.',
    polling: '서비스 재시작 대기 중...'
  },
  en: {
    heading: 'LocalVault Initial Setup',
    desc: 'Set the KeyCenter URL and password to initialize LocalVault.',
    password: 'Password',
    passwordPh: 'Minimum 8 characters',
    passwordConfirm: 'Confirm Password',
    passwordConfirmPh: 'Re-enter password',
    minChars: 'Must be at least 8 characters.',
    mismatch: 'Passwords do not match.',
    initBtn: 'Initialize',
    initializing: 'Initializing...',
    successHeading: 'Initialization Complete',
    successMsg: 'Initialization complete. The service will restart.',
    polling: 'Waiting for service restart...'
  }
}

const t = computed(() => i18n[store.lang] || i18n.ko)

const canSubmit = computed(() => {
  return (
    store.keycenterUrl.trim() !== '' &&
    store.password.length >= 8 &&
    store.password === store.passwordConfirm
  )
})

async function pollHealth() {
  polling.value = true
  const maxAttempts = 30
  for (let i = 0; i < maxAttempts; i++) {
    try {
      const res = await fetch('/health')
      if (res.ok) {
        polling.value = false
        router.replace('/settings')
        return
      }
    } catch {
      // service not ready yet
    }
    await new Promise(resolve => setTimeout(resolve, 2000))
  }
  polling.value = false
  router.replace('/settings')
}

async function handleInit() {
  if (!canSubmit.value) return
  const result = await runInit()
  if (result) {
    done.value = true
    pollHealth()
  }
}
</script>
