import { createApp } from 'vue'
import InstallApp from './InstallApp.vue'
import router from './router.js'

const app = createApp(InstallApp)
app.use(router)
app.mount('#install-app')

// Detect locked state and redirect to /#/locked
fetch('/api/status').then(r => {
  if (r.status === 403 || r.status === 423) {
    router.replace('/locked')
  } else if (r.ok) {
    return r.json()
  }
}).then(data => {
  if (data && data.locked) {
    router.replace('/locked')
  }
}).catch(() => {})
