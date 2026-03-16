import { createRouter, createWebHashHistory } from 'vue-router'
import PageSetup from './pages/PageSetup.vue'
import PageSettings from './pages/PageSettings.vue'

const routes = [
  { path: '/', component: PageSetup },
  { path: '/settings', component: PageSettings }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes
})

export default router
