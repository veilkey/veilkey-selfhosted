<template>
  <div class="install-shell">
    <header class="install-header">
      <h1 class="install-title">VeilKey Install Wizard</h1>
      <nav class="step-indicator">
        <div
          v-for="(step, i) in steps"
          :key="step.path"
          class="step"
          :class="{ active: currentIndex === i, done: currentIndex > i }"
        >
          <span class="step-number">{{ i + 1 }}</span>
          <span class="step-label">{{ step.label }}</span>
        </div>
      </nav>
    </header>
    <main class="install-body">
      <router-view />
    </main>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import './install.css'

const router = useRouter()

const steps = [
  { path: '/', label: '대상 선택' },
  { path: '/configure', label: '설정 입력' },
  { path: '/validate', label: '검증' },
  { path: '/apply', label: '실행' }
]

const currentIndex = computed(() => {
  const path = router.currentRoute.value.path
  const idx = steps.findIndex(s => s.path === path)
  return idx === -1 ? 0 : idx
})
</script>

<style scoped>
.install-shell {
  min-height: 100vh;
  background: #0f1117;
  color: #e0e0e0;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  display: flex;
  flex-direction: column;
}
.install-header {
  padding: 24px 32px;
  border-bottom: 1px solid #23263a;
}
.install-title {
  font-size: 1.25rem;
  font-weight: 600;
  margin: 0 0 16px 0;
  color: #ffffff;
}
.step-indicator {
  display: flex;
  gap: 8px;
}
.step {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 14px;
  border-radius: 6px;
  background: #1a1d2e;
  font-size: 0.85rem;
  opacity: 0.5;
  transition: opacity 0.2s, background 0.2s;
}
.step.active {
  opacity: 1;
  background: #2a3050;
}
.step.done {
  opacity: 0.75;
}
.step-number {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: #333758;
  font-size: 0.75rem;
  font-weight: 700;
}
.step.active .step-number {
  background: #5b7fff;
  color: #fff;
}
.step.done .step-number {
  background: #3a7d5c;
  color: #fff;
}
.step-label {
  white-space: nowrap;
}
.install-body {
  flex: 1;
  padding: 32px;
}
</style>
