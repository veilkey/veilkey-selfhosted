<template>
  <div class="lv-shell">
    <header class="lv-header">
      <h1 class="lv-logo">VeilKey <span class="lv-badge">LocalVault</span></h1>
      <div class="lang-toggle">
        <button :class="{ active: store.lang === 'ko' }" @click="store.lang = 'ko'">한국어</button>
        <button :class="{ active: store.lang === 'en' }" @click="store.lang = 'en'">English</button>
      </div>
    </header>
    <main class="lv-main">
      <router-view />
    </main>
  </div>
</template>

<script setup>
import { store } from './store'
</script>
