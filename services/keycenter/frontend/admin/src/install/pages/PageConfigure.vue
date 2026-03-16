<template>
  <div class="configure-page">
    <h1>{{ t.heading }}</h1>
    <p class="page-subtitle">{{ t.subtitle }}</p>

    <div v-if="store.error" class="error-banner">
      {{ store.error }}
    </div>

    <div v-if="store.targetType === 'lxc'" class="configure-form">
      <div class="form-group">
        <label class="form-label">{{ t.lxcMode }}</label>
        <select class="form-select" v-model="store.config.target_mode">
          <option value="new">{{ t.lxcModeNew }}</option>
          <option value="existing">{{ t.lxcModeExisting }}</option>
        </select>
      </div>

      <div class="form-group">
        <label class="form-label">{{ t.proxmoxNode }}</label>
        <input class="form-input" type="text" v-model="store.config.target_node"
               :placeholder="t.proxmoxNodePlaceholder" />
      </div>

      <div class="form-group">
        <label class="form-label">{{ t.lxcVmid }}</label>
        <input class="form-input" type="text" v-model="store.config.target_vmid"
               :placeholder="t.lxcVmidPlaceholder" />
      </div>

      <div class="form-group">
        <label class="form-label">{{ t.publicBaseUrl }}</label>
        <input class="form-input" type="text" v-model="store.config.public_base_url"
               :placeholder="t.publicBaseUrlPlaceholder" />
      </div>

      <div class="form-group">
        <label class="form-label">{{ t.tlsMode }}</label>
        <select class="form-select" v-model="store.config.tls_mode">
          <option value="none">{{ t.tlsNone }}</option>
          <option value="existing">{{ t.tlsExisting }}</option>
        </select>
      </div>

      <div v-if="store.config.tls_mode === 'existing'" class="tls-fields full-width">
        <div class="form-group">
          <label class="form-label">{{ t.tlsCertPath }}</label>
          <input class="form-input" type="text" v-model="store.config.tls_cert_path"
                 :placeholder="t.tlsCertPathPlaceholder" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t.tlsKeyPath }}</label>
          <input class="form-input" type="text" v-model="store.config.tls_key_path"
                 :placeholder="t.tlsKeyPathPlaceholder" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t.tlsCaPath }}<span class="optional-tag">{{ t.optional }}</span></label>
          <input class="form-input" type="text" v-model="store.config.tls_ca_path"
                 :placeholder="t.tlsCaPathPlaceholder" />
        </div>
      </div>
    </div>

    <div v-else class="configure-form">
      <div class="form-group">
        <label class="form-label">{{ t.installRoot }}</label>
        <input class="form-input" type="text" v-model="store.config.install_root"
               :placeholder="t.installRootPlaceholder" />
      </div>

      <div class="form-group">
        <label class="form-label">{{ t.publicBaseUrl }}</label>
        <input class="form-input" type="text" v-model="store.config.public_base_url"
               :placeholder="t.publicBaseUrlPlaceholder" />
      </div>

      <div class="form-group">
        <label class="form-label">{{ t.tlsMode }}</label>
        <select class="form-select" v-model="store.config.tls_mode">
          <option value="none">{{ t.tlsNone }}</option>
          <option value="existing">{{ t.tlsExisting }}</option>
        </select>
      </div>

      <div class="form-group">
        <label class="form-label">{{ t.localvaultUrl }}<span class="optional-tag">{{ t.optional }}</span></label>
        <input class="form-input" type="text" v-model="store.config.localvault_url"
               :placeholder="t.localvaultUrlPlaceholder" />
      </div>

      <div v-if="store.config.tls_mode === 'existing'" class="tls-fields full-width">
        <div class="form-group">
          <label class="form-label">{{ t.tlsCertPath }}</label>
          <input class="form-input" type="text" v-model="store.config.tls_cert_path"
                 :placeholder="t.tlsCertPathPlaceholder" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t.tlsKeyPath }}</label>
          <input class="form-input" type="text" v-model="store.config.tls_key_path"
                 :placeholder="t.tlsKeyPathPlaceholder" />
        </div>
        <div class="form-group">
          <label class="form-label">{{ t.tlsCaPath }}<span class="optional-tag">{{ t.optional }}</span></label>
          <input class="form-input" type="text" v-model="store.config.tls_ca_path"
                 :placeholder="t.tlsCaPathPlaceholder" />
        </div>
      </div>

      <div v-if="store.config.install_root === '/'" class="form-checkbox-group full-width">
        <input type="checkbox" id="root-confirm" v-model="rootConfirmed" />
        <label for="root-confirm" class="checkbox-label">{{ t.rootConfirm }}</label>
      </div>
    </div>

    <div class="derived-preview">
      <h3>{{ t.derivedTitle }}</h3>
      <dl>
        <dt>{{ t.derivedProfile }}</dt>
        <dd>{{ derivedProfile }}</dd>
        <dt>{{ t.derivedScript }}</dt>
        <dd>{{ derivedScript }}</dd>
        <dt>{{ t.derivedWorkdir }}</dt>
        <dd>{{ derivedWorkdir }}</dd>
        <dt>{{ t.derivedKeycenterUrl }}</dt>
        <dd>{{ derivedKeycenterUrl }}</dd>
        <dt>{{ t.derivedLocalvaultUrl }}</dt>
        <dd>{{ derivedLocalvaultUrl }}</dd>
      </dl>
    </div>

    <div class="btn-row">
      <button class="btn-secondary" @click="goBack">{{ t.btnBack }}</button>
      <button class="btn-primary-install" :disabled="!canProceed || store.loading" @click="goNext">
        {{ store.loading ? t.btnSaving : t.btnNext }}
      </button>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { store, loadRuntimeConfig, saveRuntimeConfig, saveSession } from '../store'

const router = useRouter()
const rootConfirmed = ref(false)

onMounted(() => {
  loadRuntimeConfig()
})

const i18n = {
  ko: {
    heading: '설정 입력',
    subtitle: '설치 환경에 맞는 설정을 입력합니다.',
    lxcMode: 'LXC 모드',
    lxcModeNew: '새 컨테이너 생성',
    lxcModeExisting: '기존 컨테이너 사용',
    proxmoxNode: 'Proxmox 노드',
    proxmoxNodePlaceholder: 'pve1',
    lxcVmid: 'LXC VMID',
    lxcVmidPlaceholder: '200',
    publicBaseUrl: '공개 호스트/도메인',
    publicBaseUrlPlaceholder: 'veilkey.example.com',
    installRoot: '설치 루트 경로',
    installRootPlaceholder: '/',
    tlsMode: 'TLS 모드',
    tlsNone: '나중에 설정',
    tlsExisting: '기존 인증서 사용',
    tlsCertPath: '인증서 경로',
    tlsCertPathPlaceholder: '/etc/ssl/certs/veilkey.crt',
    tlsKeyPath: '키 경로',
    tlsKeyPathPlaceholder: '/etc/ssl/private/veilkey.key',
    tlsCaPath: 'CA 인증서 경로',
    tlsCaPathPlaceholder: '/etc/ssl/certs/ca.crt',
    localvaultUrl: '기존 LocalVault URL',
    localvaultUrlPlaceholder: 'https://localhost:8201',
    optional: ' (선택)',
    rootConfirm: '이 서버의 루트(/)에 직접 설치합니다. 기존 시스템 파일에 영향을 줄 수 있습니다.',
    derivedTitle: '자동 결정 설정',
    derivedProfile: 'Install Profile',
    derivedScript: 'Install Script',
    derivedWorkdir: 'Working Directory',
    derivedKeycenterUrl: 'KeyCenter URL',
    derivedLocalvaultUrl: 'LocalVault URL',
    btnBack: '이전',
    btnNext: '검증으로',
    btnSaving: '저장 중...',
  },
  en: {
    heading: 'Configure',
    subtitle: 'Enter settings for your installation environment.',
    lxcMode: 'LXC Mode',
    lxcModeNew: 'Create new container',
    lxcModeExisting: 'Use existing container',
    proxmoxNode: 'Proxmox Node',
    proxmoxNodePlaceholder: 'pve1',
    lxcVmid: 'LXC VMID',
    lxcVmidPlaceholder: '200',
    publicBaseUrl: 'Public host/domain',
    publicBaseUrlPlaceholder: 'veilkey.example.com',
    installRoot: 'Install Root Path',
    installRootPlaceholder: '/',
    tlsMode: 'TLS Mode',
    tlsNone: 'Configure later',
    tlsExisting: 'Use existing certificates',
    tlsCertPath: 'Certificate path',
    tlsCertPathPlaceholder: '/etc/ssl/certs/veilkey.crt',
    tlsKeyPath: 'Key path',
    tlsKeyPathPlaceholder: '/etc/ssl/private/veilkey.key',
    tlsCaPath: 'CA certificate path',
    tlsCaPathPlaceholder: '/etc/ssl/certs/ca.crt',
    localvaultUrl: 'Existing LocalVault URL',
    localvaultUrlPlaceholder: 'https://localhost:8201',
    optional: ' (optional)',
    rootConfirm: 'Install directly to this server\'s root (/). This may affect existing system files.',
    derivedTitle: 'Auto-determined settings',
    derivedProfile: 'Install Profile',
    derivedScript: 'Install Script',
    derivedWorkdir: 'Working Directory',
    derivedKeycenterUrl: 'KeyCenter URL',
    derivedLocalvaultUrl: 'LocalVault URL',
    btnBack: 'Back',
    btnNext: 'Validate',
    btnSaving: 'Saving...',
  }
}

const t = computed(() => i18n[store.lang] || i18n.ko)

const derivedProfile = computed(() => {
  if (store.targetType === 'lxc') return 'proxmox-lxc-allinone'
  return 'linux-host-service'
})

const derivedScript = computed(() => {
  if (store.targetType === 'lxc') return '/opt/veilkey/scripts/install-lxc.sh'
  return '/opt/veilkey/scripts/install-host.sh'
})

const derivedWorkdir = computed(() => {
  if (store.targetType === 'lxc') return '/opt/veilkey/workdir'
  const root = store.config.install_root || '/'
  return root === '/' ? '/opt/veilkey/workdir' : `${root}/opt/veilkey/workdir`
})

const derivedKeycenterUrl = computed(() => {
  const base = store.config.public_base_url
  if (!base) return 'https://<host>:8443'
  const proto = store.config.tls_mode === 'existing' ? 'https' : 'http'
  const port = store.config.tls_mode === 'existing' ? '8443' : '8080'
  return `${proto}://${base}:${port}`
})

const derivedLocalvaultUrl = computed(() => {
  if (store.config.localvault_url) return store.config.localvault_url
  return 'https://127.0.0.1:8201'
})

const canProceed = computed(() => {
  if (store.targetType === 'lxc') {
    return !!(store.config.target_node && store.config.target_vmid)
  }
  if (store.config.install_root === '/' && !rootConfirmed.value) {
    return false
  }
  return true
})

function goBack() {
  router.push('/')
}

async function goNext() {
  store.error = null
  store.config.install_profile = derivedProfile.value
  store.config.install_script = derivedScript.value
  store.config.install_workdir = derivedWorkdir.value
  store.config.keycenter_url = derivedKeycenterUrl.value
  store.config.localvault_url = derivedLocalvaultUrl.value || 'https://127.0.0.1:8201'

  await saveRuntimeConfig()
  if (store.error) return
  await saveSession('configure')
  if (store.error) return
  router.push('/validate')
}
</script>
