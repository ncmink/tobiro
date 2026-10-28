import { createApp } from 'vue'
import { createPinia } from 'pinia'

import App from './App.vue'
import { router } from './router'
import './style.css'
import { initTheme } from './lib/theme'

const disposeTheme = initTheme()
if (import.meta.hot) import.meta.hot.dispose(disposeTheme)

createApp(App).use(createPinia()).use(router).mount('#app')
