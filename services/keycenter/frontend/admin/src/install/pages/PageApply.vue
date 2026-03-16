<template>
  <div class="apply-page">
    <!-- Starting state -->
    <div v-if="starting" class="apply-starting">
      <div class="spinner"></div>
      <p>{{ t.starting }}</p>
    </div>

    <!-- Running state -->
    <div v-if="running" class="apply-running">
      <div class="progress-header">
        <div class="spinner"></div>
        <h2>{{ t.installing }}</h2>
        <p>{{ t.profile }}: {{ state?.profile || state?.last_profile || '' }}</p>
      </div>
      <div class="output-box" ref="outputBox">
        <pre>{{ state?.state?.last_output || '' }}</pre>
      </div>
    </div>

    <!-- Success state -->
    <div v-if="succeeded" class="apply-success">
      <div class="status-banner success">
        <h2>{{ t.installComplete }}</h2>
      </div>
      <p>{{ t.successMessage }}</p>
      <div class="next-steps">
        <h3>{{ t.nextSteps }}</h3>
        <ul>
          <li><a href="/approve/install/bootstrap">{{ t.bootstrapLink }}</a></li>
          <li><a href="/approve/install/custody">{{ t.custodyLink }}</a></li>
        </ul>
      </div>
      <button class="btn-primary" @click="markComplete">{{ t.goToDashboard }}</button>
    </div>

    <!-- Failed state -->
    <div v-if="failed" class="apply-failed">
      <div class="status-banner error">
        <h2>{{ t.installFailed }}</h2>
      </div>
      <p class="error-message">{{ state?.state?.last_error }}</p>
      <div class="output-box" ref="outputBox">
        <pre>{{ state?.state?.last_output || '' }}</pre>
      </div>
      <div class="btn-row">
        <button class="btn-secondary" @click="$router.push('/validate')">{{ t.backToValidate }}</button>
        <button class="btn-primary" @click="retry">{{ t.retry }}</button>
      </div>
    </div>

    <!-- Recent runs history -->
    <div v-if="runs.length > 0" class="run-history">
      <h3>{{ t.recentRuns }}</h3>
      <div v-for="run in runs" :key="run.run_id" class="run-item">
        <span :class="['run-status', run.status]">{{ run.status }}</span>
        <span class="run-profile">{{ run.install_profile }}</span>
        <span class="run-time">{{ formatTime(run.started_at) }}</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { store } from '../store'
import { useRouter } from 'vue-router'

const router = useRouter()

const state = ref(null)
const runs = ref([])
const starting = ref(true)
const error = ref(null)
const pollTimer = ref(null)
const outputBox = ref(null)

const running = computed(() => !starting.value && state.value?.state?.status === 'running')
const succeeded = computed(() => !starting.value && state.value?.state?.status === 'succeeded')
const failed = computed(() => !starting.value && state.value?.state?.status === 'failed')

const i18n = {
  ko: {
    starting: '설치를 시작하고 있습니다...',
    installing: '설치 진행 중',
    profile: '프로파일',
    installComplete: '설치가 완료되었습니다',
    successMessage: 'VeilKey Service Stack이 성공적으로 설치되었습니다. 아래 단계를 진행하여 초기 설정을 완료하세요.',
    nextSteps: '다음 단계',
    bootstrapLink: 'Bootstrap 초기 설정',
    custodyLink: 'Custody 키 설정',
    goToDashboard: '대시보드로 이동',
    installFailed: '설치에 실패했습니다',
    backToValidate: '검증으로 돌아가기',
    retry: '재시도',
    recentRuns: '최근 실행 기록',
  },
  en: {
    starting: 'Starting installation...',
    installing: 'Installation in progress',
    profile: 'Profile',
    installComplete: 'Installation Complete',
    successMessage: 'VeilKey Service Stack has been installed successfully. Proceed with the steps below to complete initial setup.',
    nextSteps: 'Next Steps',
    bootstrapLink: 'Bootstrap Initial Setup',
    custodyLink: 'Custody Key Setup',
    goToDashboard: 'Go to Dashboard',
    installFailed: 'Installation Failed',
    backToValidate: 'Back to Validate',
    retry: 'Retry',
    recentRuns: 'Recent Runs',
  }
}

const t = computed(() => i18n[store.lang] || i18n.ko)

async function startApply() {
  starting.value = true
  error.value = null
  try {
    const res = await fetch('/api/install/apply', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ confirm_dangerous_root: false })
    })
    if (!res.ok) throw new Error(await res.text())
    const data = await res.json()
    // Map POST response into the GET response shape for consistency
    state.value = {
      profile: data.install_profile,
      state: {
        status: data.status,
        last_error: data.last_error || '',
        last_output: data.output_tail || '',
        last_run_id: data.run_id,
        last_started_at: data.started_at,
        last_finished_at: data.finished_at,
      }
    }
    starting.value = false
    startPolling()
  } catch (e) {
    error.value = e.message
    // Even on error, try to fetch current status
    await pollStatus()
    starting.value = false
  }
}

function startPolling() {
  stopPolling()
  pollTimer.value = setInterval(pollStatus, 3000)
}

function stopPolling() {
  if (pollTimer.value) {
    clearInterval(pollTimer.value)
    pollTimer.value = null
  }
}

async function pollStatus() {
  try {
    const res = await fetch('/api/install/apply')
    if (!res.ok) throw new Error(await res.text())
    const data = await res.json()
    state.value = data
    if (data.state?.status !== 'running') {
      stopPolling()
      await loadRuns()
    }
  } catch (e) {
    error.value = e.message
  }
}

async function loadRuns() {
  try {
    const res = await fetch('/api/install/runs')
    if (!res.ok) return
    const data = await res.json()
    runs.value = data.runs || []
  } catch (_e) {
    // silently ignore
  }
}

async function markComplete() {
  try {
    await fetch('/api/install/state', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        session_id: store.sessionId || '',
        last_stage: 'complete'
      })
    })
  } catch (_e) {
    // best-effort
  }
  window.location.href = '/'
}

function retry() {
  runs.value = []
  startApply()
}

function formatTime(iso) {
  if (!iso) return ''
  try {
    const d = new Date(iso)
    const pad = (n) => String(n).padStart(2, '0')
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  } catch (_e) {
    return iso
  }
}

function scrollOutputToBottom() {
  if (outputBox.value) {
    outputBox.value.scrollTop = outputBox.value.scrollHeight
  }
}

// Auto-scroll output box when content changes
watch(
  () => state.value?.state?.last_output,
  () => { nextTick(scrollOutputToBottom) }
)

onMounted(() => {
  startApply()
})

onUnmounted(() => {
  stopPolling()
})
</script>
