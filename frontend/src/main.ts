import { createApp } from 'vue'
import { createPinia } from 'pinia'
import router from './router'
import App from './App.vue'
import { syncTokenFromUrl } from './auth'
import './style.css'

syncTokenFromUrl()

const app = createApp(App)
app.use(createPinia())
app.use(router)

// Global error handler - prevent white screen on uncaught render errors
app.config.errorHandler = (err, instance, info) => {
  console.error('Vue error:', err, info)
}

app.mount('#app')
