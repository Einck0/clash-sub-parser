<template>
  <AuthGate v-if="showAuthGate" :authenticate="handleAuthSubmit" />

  <div v-else class="app-shell">
    <header class="topbar">
      <div class="brand-block">
        <div class="brand">
          <span class="brand-dot" aria-hidden="true"></span>
          <div>
            <p class="eyebrow app-eyebrow">Config Studio</p>
            <h1>Clash Subscription Parser</h1>
          </div>
        </div>
        <p class="brand-sub">统一管理订阅、策略组、规则、DNS 与最终导出配置。</p>
      </div>
      <div class="topbar-actions" aria-label="快捷输出">
        <a class="quick-link" :href="withAuthToken('/yaml', exportNeedsToken)" target="_blank" rel="noreferrer">YAML</a>
        <a class="quick-link" :href="withAuthToken('/script', exportNeedsToken)" target="_blank" rel="noreferrer">Script.js</a>
      </div>
    </header>

    <nav class="tabs" aria-label="主导航">
      <router-link v-for="item in navItems" :key="item.to" :to="item.to" class="tab">
        <span class="tab-icon" aria-hidden="true">{{ item.icon }}</span>
        <span class="tab-label">{{ item.label }}</span>
        <small>{{ item.hint }}</small>
      </router-link>
    </nav>

    <main class="page-wrap" aria-live="polite">
      <router-view v-slot="{ Component, route }">
        <Transition name="page-fade" mode="out-in">
          <component :is="Component" :key="route.path" />
        </Transition>
      </router-view>
    </main>
  </div>

  <ToastContainer />
  <ConfirmDialog />
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { checkAuthSession, getSecuritySettings, loginAuthToken } from './api'
import { setAuthToken, withAuthToken } from './auth'
import AuthGate from './components/AuthGate.vue'
import ToastContainer from './components/ToastContainer.vue'
import ConfirmDialog from './components/ConfirmDialog.vue'
import { useAppStore } from './stores/app'

const store = useAppStore()

const navItems = [
  { to: '/', label: 'Subscriptions', hint: '订阅', icon: '📡' },
  { to: '/node-groups', label: 'Node Groups', hint: '策略组', icon: '🔀' },
  { to: '/rules', label: 'Rules', hint: '规则', icon: '📋' },
  { to: '/dns', label: 'DNS', hint: '解析', icon: '🌐' },
  { to: '/generate', label: 'Generate', hint: '导出', icon: '⚙' },
  { to: '/settings', label: 'Settings', hint: '设置', icon: '🔒' },
]

const showAuthGate = ref(false)
const exportNeedsToken = ref(false)

onMounted(() => {
  window.addEventListener('auth:unauthorized', handleUnauthorized)
  checkFrontendAccess()
})

async function checkFrontendAccess() {
  try {
    const { data } = await getSecuritySettings()
    exportNeedsToken.value = Boolean(data.auth_enabled && data.protect_exports)
    if (data.auth_enabled && data.protect_frontend) {
      try {
        await checkAuthSession()
      } catch {
        setAuthToken('')
        showAuthGate.value = true
        return
      }
    }
    showAuthGate.value = false
  } catch (err) {
    // If 401 and protect_frontend is on, show auth gate; otherwise allow access
    if (err?.response?.status === 401) {
      setAuthToken('')
      exportNeedsToken.value = true
      showAuthGate.value = true
      return
    }
    // Non-auth error (e.g. server down) - don't block the UI
    showAuthGate.value = false
  }
}

async function handleAuthSubmit(token) {
  const { data } = await loginAuthToken(token)
  if (!data?.ok) throw new Error('Token 无效')
  setAuthToken(token)
  await checkFrontendAccess()
}

function handleUnauthorized() {
  setAuthToken('')
  exportNeedsToken.value = true
  showAuthGate.value = true
}
</script>

<style scoped>
/* Page transition */
.page-fade-enter-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}
.page-fade-leave-active {
  transition: opacity 0.12s ease;
}
.page-fade-enter-from {
  opacity: 0;
  transform: translateY(6px);
}
.page-fade-leave-to {
  opacity: 0;
}

/* Tab icons */
.tab-icon {
  display: inline-block;
  margin-right: 4px;
  font-size: 15px;
  line-height: 1;
}

.tab-label {
  display: inline;
}
</style>
