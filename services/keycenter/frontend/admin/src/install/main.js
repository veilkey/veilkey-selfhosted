import { createApp } from 'vue'
import InstallApp from './InstallApp.vue'
import router from './router.js'

createApp(InstallApp).use(router).mount('#install-app')
