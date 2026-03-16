import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import './install.css'

const app = createApp(App)
app.use(router)
app.mount('#install-app')

// Detect initialized state and redirect to settings
fetch('/api/install/status')
  .then(r => r.json())
  .then(data => {
    if (data.initialized && router.currentRoute.value.path === '/') {
      router.replace('/settings')
    }
  })
  .catch(() => {})
