<template>
  <div class="install-landing">
    <!-- Scope banner -->
    <div class="scope-banner">
      {{ t.scopeNotice }}
    </div>

    <!-- Language toggle -->
    <div class="lang-toggle">
      <button :class="{ active: store.lang === 'ko' }" @click="store.lang = 'ko'">한국어</button>
      <button :class="{ active: store.lang === 'en' }" @click="store.lang = 'en'">English</button>
    </div>

    <!-- Main heading -->
    <h1>{{ t.heading }}</h1>
    <p class="subtitle">{{ t.subtitle }}</p>

    <!-- Selection cards -->
    <div class="install-cards">
      <div class="install-card" @click="select('linux')">
        <h2>{{ t.linuxTitle }}</h2>
        <p>{{ t.linuxDesc }}</p>
      </div>
      <div class="install-card recommended" @click="select('lxc')">
        <span class="badge">{{ t.recommended }}</span>
        <h2>{{ t.lxcTitle }}</h2>
        <p>{{ t.lxcDesc }}</p>
      </div>
    </div>

    <!-- Flow summary -->
    <p class="flow-summary">{{ t.flowSummary }}</p>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { store } from '../store'

const router = useRouter()

const i18n = {
  ko: {
    scopeNotice: '이 설치기는 Service Stack + Orchestration만 다룹니다. Host Boundary와 Operator CLI는 별도입니다.',
    heading: 'VeilKey 설치 시작',
    subtitle: '어디에 설치할지 먼저 선택합니다.',
    linuxTitle: 'Linux에 직접 설치',
    linuxDesc: 'Service Stack을 Linux 호스트에 직접 설치합니다.',
    lxcTitle: 'LXC 컨테이너에 설치',
    lxcDesc: 'Proxmox LXC에 격리된 Service Stack을 설치합니다.',
    recommended: '추천',
    flowSummary: '대상 선택 → 설정 입력 → 검증 → 실행'
  },
  en: {
    scopeNotice: 'This wizard covers Service Stack + Orchestration only. Host Boundary and Operator CLI are separate.',
    heading: 'Start VeilKey Install',
    subtitle: 'Choose where to install first.',
    linuxTitle: 'Install on Linux directly',
    linuxDesc: 'Install Service Stack directly on a Linux host.',
    lxcTitle: 'Install in LXC container',
    lxcDesc: 'Install an isolated Service Stack in a Proxmox LXC.',
    recommended: 'Recommended',
    flowSummary: 'Select target → Configure → Validate → Apply'
  }
}

const t = computed(() => i18n[store.lang] || i18n.ko)

function select(type) {
  store.targetType = type
  router.push('/configure')
}
</script>
