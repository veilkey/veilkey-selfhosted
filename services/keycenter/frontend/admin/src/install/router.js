import { createRouter, createWebHashHistory } from 'vue-router'
import PageLanding from './pages/PageLanding.vue'
import PageConfigure from './pages/PageConfigure.vue'
import PageValidate from './pages/PageValidate.vue'
import PageApply from './pages/PageApply.vue'

const routes = [
  { path: '', component: PageLanding },
  { path: '/configure', component: PageConfigure },
  { path: '/validate', component: PageValidate },
  { path: '/apply', component: PageApply }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes
})

export default router
