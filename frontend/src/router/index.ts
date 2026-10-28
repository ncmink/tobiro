import { createRouter, createWebHashHistory } from 'vue-router'

import ConnectionDetail from '@/views/ConnectionDetail.vue'
import Dashboard from '@/views/Dashboard.vue'
import Settings from '@/views/Settings.vue'

// Hash history keeps routing self-contained inside the Wails asset server.
export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', name: 'dashboard', component: Dashboard },
    { path: '/connections/:id', name: 'connection', component: ConnectionDetail, props: true },
    { path: '/settings', name: 'settings', component: Settings },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})
