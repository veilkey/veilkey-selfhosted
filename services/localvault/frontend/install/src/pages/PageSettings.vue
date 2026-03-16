<template>
  <div class="settings-page">
    <h2>{{ t.heading }}</h2>

    <div v-if="store.error" class="error-banner">{{ store.error }}</div>
    <div v-if="saved" class="success-banner">{{ t.saved }}</div>

    <!-- Status Card -->
    <div class="status-card">
      <h3>{{ t.statusHeading }}</h3>
      <div class="status-item">
        <span class="status-label">{{ t.initialized }}</span>
        <span class="connection-status connected">{{ t.yes }}</span>
      </div>
      <div class="status-item">
        <span class="status-label">KeyCenter URL</span>
        <span class="status-value">{{ store.status?.keycenter_url || '-' }}</span>
      </div>
      <div class="status-item">
        <span class="status-label">{{ t.connection }}</span>
        <span
          class="connection-status"
          :class="store.status?.keycenter_connected ? 'connected' : 'disconnected'"
        >
          <template v-if="store.status?.keycenter_connected">{{ t.connected }}</template>
          <template v-else>{{ t.notConnected }}</template>
        </span>
      </div>
      <div v-if="store.status?.keycenter_error" class="status-item error-detail">
        <span class="status-label">{{ t.errorDetail }}</span>
        <span class="status-value error-text">{{ store.status.keycenter_error }}</span>
      </div>
    </div>

    <!-- Edit Section -->
    <div class="edit-section">
      <h3>{{ t.editHeading }}</h3>
      <form @submit.prevent="handleSave">
        <div class="form-group">
          <label class="form-label">KeyCenter URL</label>
          <input
            class="form-input"
            type="text"
            v-model="store.keycenterUrl"
            placeholder="https://keycenter.example.com:10181"
          />
        </div>
        <div class="btn-row">
          <button
            type="submit"
            class="btn-primary"
            :disabled="store.loading"
          >
            <span v-if="store.loading" class="spinner"></span>
            {{ t.saveBtn }}
          </button>
          <button
            type="button"
            class="btn-secondary"
            :disabled="store.loading"
            @click="handleTest"
          >
            {{ t.testBtn }}
          </button>
        </div>
      </form>
    </div>

    <!-- Health Links -->
    <div class="health-links">
      <h3>{{ t.healthHeading }}</h3>
      <ul>
        <li><a href="/health" target="_blank">/health</a></li>
        <li><a href="/ready" target="_blank">/ready</a></li>
        <li><a href="/api/status" target="_blank">/api/status</a></li>
      </ul>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, onMounted } from 'vue'
import { store, loadStatus, updateKeycenterUrl } from '../store'

const saved = ref(false)

const i18n = {
  ko: {
    heading: 'LocalVault 설정',
    statusHeading: '현재 상태',
    initialized: '초기화 상태',
    yes: '완료',
    connection: '연결 상태',
    connected: '연결됨',
    notConnected: '연결 안 됨',
    errorDetail: '오류 상세',
    editHeading: 'KeyCenter URL 변경',
    saveBtn: '저장',
    testBtn: '연결 테스트',
    saved: '저장되었습니다.',
    healthHeading: '상태 확인 링크'
  },
  en: {
    heading: 'LocalVault Settings',
    statusHeading: 'Current Status',
    initialized: 'Initialized',
    yes: 'Yes',
    connection: 'Connection',
    connected: 'Connected',
    notConnected: 'Not connected',
    errorDetail: 'Error Detail',
    editHeading: 'Change KeyCenter URL',
    saveBtn: 'Save',
    testBtn: 'Test Connection',
    saved: 'Saved successfully.',
    healthHeading: 'Health Check Links'
  }
}

const t = computed(() => i18n[store.lang] || i18n.ko)

onMounted(() => {
  loadStatus()
})

async function handleSave() {
  saved.value = false
  const ok = await updateKeycenterUrl()
  if (ok) {
    saved.value = true
    await loadStatus()
    setTimeout(() => { saved.value = false }, 3000)
  }
}

async function handleTest() {
  saved.value = false
  store.error = null
  await loadStatus()
}
</script>
