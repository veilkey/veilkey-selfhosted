<template>
  <div class="locked-page">
    <div class="locked-card">
      <h1>VeilKey <span class="locked-badge">locked</span></h1>
      <p class="locked-message">{{ t.message }}</p>
      <div class="locked-links">
        <a href="/health">Health</a>
        <a href="/api/status">Status</a>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { store } from '../store'

const t = computed(() => store.lang === 'en' ? {
  message: 'KeyCenter is locked. Unlock first to enter the operator console.'
} : {
  message: 'KeyCenter가 잠겨 있습니다. 운영 콘솔에 접근하려면 먼저 잠금을 해제하세요.'
})
</script>
