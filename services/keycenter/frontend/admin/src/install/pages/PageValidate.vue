<template>
  <div class="validate-page">
    <!-- Loading state -->
    <div v-if="loading" class="validate-loading">
      <div class="spinner"></div>
      <p>{{ t.validating }}</p>
    </div>

    <!-- Results -->
    <div v-else-if="result" class="validate-results">
      <!-- Status banner -->
      <div :class="['status-banner', result.valid ? 'success' : 'error']">
        {{ result.valid ? t.allPassed : t.hasErrors }}
      </div>

      <!-- Checklist -->
      <div class="checklist">
        <div class="check-item success">
          <span class="check-icon">&#x2713;</span>
          {{ t.profileResolved }}: {{ result.resolved_profile }}
        </div>
        <div class="check-item success">
          <span class="check-icon">&#x2713;</span>
          {{ t.rootResolved }}: {{ result.resolved_root }}
        </div>
        <div v-if="result.resolved_script" class="check-item success">
          <span class="check-icon">&#x2713;</span>
          {{ t.scriptResolved }}: {{ result.resolved_script }}
        </div>
        <div v-if="result.resolved_workdir" class="check-item success">
          <span class="check-icon">&#x2713;</span>
          {{ t.workdirResolved }}: {{ result.resolved_workdir }}
        </div>
        <!-- errors -->
        <div v-for="(err, i) in result.errors" :key="'e'+i" class="check-item error">
          <span class="check-icon">&#x2717;</span>
          {{ err }}
        </div>
        <!-- warnings -->
        <div v-for="(warn, i) in result.warnings" :key="'w'+i" class="check-item warning">
          <span class="check-icon">&#x26A0;</span>
          {{ warn }}
        </div>
      </div>

      <!-- Resolved settings -->
      <div class="resolved-settings">
        <h3>{{ t.resolvedSettings }}</h3>
        <div class="resolved-grid">
          <div class="resolved-item">
            <span class="label">{{ t.profile }}</span>
            <span class="value">{{ result.resolved_profile }}</span>
          </div>
          <div class="resolved-item">
            <span class="label">{{ t.root }}</span>
            <span class="value">{{ result.resolved_root }}</span>
          </div>
          <div class="resolved-item">
            <span class="label">{{ t.script }}</span>
            <span class="value">{{ result.resolved_script }}</span>
          </div>
          <div class="resolved-item">
            <span class="label">{{ t.workdir }}</span>
            <span class="value">{{ result.resolved_workdir }}</span>
          </div>
        </div>
      </div>

      <!-- Dangerous root confirmation -->
      <div v-if="result.needs_confirmation" class="dangerous-root">
        <label>
          <input type="checkbox" v-model="confirmDangerous">
          {{ t.confirmDangerousRoot }}
        </label>
        <button @click="revalidate" :disabled="!confirmDangerous">{{ t.revalidate }}</button>
      </div>

      <!-- Command preview -->
      <div class="command-preview">
        <h3>{{ t.commandPreview }}</h3>
        <code>{{ result.command_preview?.join(' ') }}</code>
      </div>

      <!-- Buttons -->
      <div class="btn-row">
        <button class="btn-secondary" @click="$router.push('/configure')">{{ t.back }}</button>
        <button class="btn-primary-install" @click="goApply" :disabled="!canApply">{{ t.applyInstall }}</button>
      </div>
    </div>

    <!-- Error state -->
    <div v-else-if="fetchError" class="validate-error">
      <p>{{ fetchError }}</p>
      <button @click="validate">{{ t.retry }}</button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { store } from '../store'

const router = useRouter()

const loading = ref(false)
const result = ref(null)
const fetchError = ref(null)
const confirmDangerous = ref(false)

const i18n = {
  ko: {
    validating: '설치 구성을 검증하는 중입니다...',
    allPassed: '모든 검증을 통과했습니다.',
    hasErrors: '검증 오류가 있습니다. 확인 후 다시 시도하세요.',
    profileResolved: '프로파일 확인됨',
    rootResolved: '설치 루트 확인됨',
    scriptResolved: '설치 스크립트 확인됨',
    workdirResolved: '작업 디렉토리 확인됨',
    resolvedSettings: '확인된 설정',
    profile: '프로파일',
    root: '설치 루트',
    script: '스크립트',
    workdir: '작업 디렉토리',
    confirmDangerousRoot: '루트(/)에 직접 설치를 확인합니다',
    revalidate: '재검증',
    commandPreview: '실행될 명령',
    back: '이전',
    applyInstall: '설치 실행',
    retry: '다시 시도'
  },
  en: {
    validating: 'Validating install configuration...',
    allPassed: 'All validations passed.',
    hasErrors: 'Validation errors found. Please review and try again.',
    profileResolved: 'Profile resolved',
    rootResolved: 'Install root resolved',
    scriptResolved: 'Install script resolved',
    workdirResolved: 'Work directory resolved',
    resolvedSettings: 'Resolved Settings',
    profile: 'Profile',
    root: 'Install Root',
    script: 'Script',
    workdir: 'Work Directory',
    confirmDangerousRoot: 'I confirm installing directly to root (/)',
    revalidate: 'Re-validate',
    commandPreview: 'Command Preview',
    back: 'Back',
    applyInstall: 'Run Install',
    retry: 'Retry'
  }
}

const t = computed(() => i18n[store.lang] || i18n.ko)

const canApply = computed(() => {
  if (!result.value) return false
  if (!result.value.valid) return false
  if (result.value.needs_confirmation && !confirmDangerous.value) return false
  return true
})

async function validate() {
  loading.value = true
  result.value = null
  fetchError.value = null
  try {
    const res = await fetch('/api/install/validate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ confirm_dangerous_root: confirmDangerous.value })
    })
    if (!res.ok) {
      throw new Error(`HTTP ${res.status}: ${await res.text()}`)
    }
    result.value = await res.json()
  } catch (e) {
    fetchError.value = e.message
  } finally {
    loading.value = false
  }
}

async function revalidate() {
  confirmDangerous.value = true
  await validate()
}

function goApply() {
  router.push('/apply')
}

onMounted(() => {
  validate()
})
</script>
