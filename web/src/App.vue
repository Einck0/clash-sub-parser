<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter, RouterView, RouterLink } from 'vue-router'
import {
  Squares2X2Icon,
  DocumentDuplicateIcon,
  ServerStackIcon,
  BoltIcon,
  CpuChipIcon,
  ArrowDownTrayIcon,
  Cog6ToothIcon,
  SunIcon,
  MoonIcon,
  ArrowPathIcon,
  ChevronDoubleLeftIcon,
  ChevronDoubleRightIcon,
} from '@heroicons/vue/24/outline'
import { api, ApiError } from './api/client'
import { applyTheme, getStoredTheme, THEME_NAMES, type ThemeName } from './theme'
import ToastContainer from './ui/ToastContainer.vue'
import { useAuth } from './features/auth/useAuth'
import AuthGate from './features/auth/AuthGate.vue'
import { useContentInset } from './composables/useContentInset'
import { getRouteTab, setRouteTab, type NavTab } from './navigation'
import { currentLocale, toggleLocale, t } from './locales'
import { router as fallbackRouter } from './router'

const injectedRouter = useRouter()
const injectedRoute = useRoute()
const router = injectedRouter || fallbackRouter

const { dockRef, style: contentInsetStyle } = useContentInset(52)

const currentTheme = ref<ThemeName>('dark')
const themes = THEME_NAMES

const activeTab = ref<string>(getRouteTab())

const isCollapsed = ref<boolean>(false)
if (typeof window !== 'undefined') {
  try {
    isCollapsed.value = localStorage.getItem('csp_sidebar_collapsed') === 'true'
  } catch {
    // ignore
  }
}

function toggleCollapse() {
  isCollapsed.value = !isCollapsed.value
  try {
    localStorage.setItem('csp_sidebar_collapsed', String(isCollapsed.value))
  } catch {
    // ignore
  }
}

const navItems = [
  { id: 'dashboard', path: '/dashboard', labelKey: 'nav.dashboard', defaultLabel: 'Dashboard', icon: Squares2X2Icon },
  { id: 'subscriptions', path: '/subscriptions', labelKey: 'nav.subscriptions', defaultLabel: 'Subscriptions', icon: DocumentDuplicateIcon },
  { id: 'nodes', path: '/nodes', labelKey: 'nav.nodes', defaultLabel: 'Node Ledger', icon: ServerStackIcon },
  { id: 'probes', path: '/probes', labelKey: 'nav.probes', defaultLabel: 'Probe Engine', icon: BoltIcon },
  { id: 'policy', path: '/policy', labelKey: 'nav.policy', defaultLabel: 'Policy Tree', icon: CpuChipIcon },
  { id: 'publications', path: '/publications', labelKey: 'nav.publications', defaultLabel: 'Exports', icon: ArrowDownTrayIcon },
  { id: 'settings', path: '/settings', labelKey: 'nav.settings', defaultLabel: 'Settings', icon: Cog6ToothIcon },
]

const currentTitle = computed(() => {
  const rName = injectedRoute?.name || activeTab.value || 'dashboard'
  return String(rName).replace('-', ' ')
})

const isItemActive = (item: typeof navItems[0]) => {
  if (injectedRoute) {
    return injectedRoute.name === item.id || injectedRoute.path === item.path
  }
  return activeTab.value === item.id
}

const healthStatus = ref<'checking' | 'healthy' | 'unhealthy'>('checking')
const healthDetails = ref<string>('Connecting to control plane...')
const requestId = ref<string>('')

const {
  state: authState,
  status: authStatus,
  errorMessage: authErrorMessage,
  probe: probeAuth,
} = useAuth()

const viewKey = ref(0)

const isThemeOpen = ref(false)
const themeDropdownRef = ref<HTMLElement | null>(null)
const themeToggleBtnRef = ref<HTMLButtonElement | null>(null)

const toggleThemeDropdown = () => {
  if (isThemeOpen.value) {
    closeThemeDropdown()
  } else {
    isThemeOpen.value = true
  }
}

const closeThemeDropdown = () => {
  isThemeOpen.value = false
  if (typeof document !== 'undefined') {
    const active = document.activeElement
    if (active instanceof HTMLElement && themeDropdownRef.value?.contains(active)) {
      active.blur()
    }
  }
}

const switchTheme = (theme: ThemeName) => {
  currentTheme.value = theme
  applyTheme(theme)
  closeThemeDropdown()
}

const handleDocumentClick = (e: MouseEvent) => {
  if (!isThemeOpen.value) return
  const target = e.target as Node | null
  if (themeDropdownRef.value && target && !themeDropdownRef.value.contains(target)) {
    closeThemeDropdown()
  }
}

const handleGlobalKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Escape' && isThemeOpen.value) {
    closeThemeDropdown()
  }
}

const checkHealth = async () => {
  healthStatus.value = 'checking'
  try {
    const res = await api.get<{ status: string }>('/healthz')
    healthStatus.value = 'healthy'
    healthDetails.value = `Service Status: ${res.status}`
  } catch (err) {
    healthStatus.value = 'unhealthy'
    if (err instanceof ApiError) {
      healthDetails.value = `${err.code}: ${err.message}`
      requestId.value = err.requestId || ''
    } else {
      healthDetails.value = String(err)
    }
  }
}

onMounted(() => {
  const initialTab = getRouteTab()
  activeTab.value = initialTab
  if (initialTab && router?.currentRoute?.value?.name !== initialTab) {
    router.replace({ name: initialTab }).catch(() => {})
  }

  if (typeof window !== 'undefined') {
    window.addEventListener('click', handleDocumentClick)
    window.addEventListener('keydown', handleGlobalKeydown)
    window.addEventListener('hashchange', () => {
      activeTab.value = getRouteTab()
      if (activeTab.value && router?.currentRoute?.value?.name !== activeTab.value) {
        router.replace({ name: activeTab.value }).catch(() => {})
      }
    })
    window.addEventListener('csp:auth-success', () => {
      viewKey.value++
      probeAuth()
      checkHealth()
    })
  }
  switchTheme(getStoredTheme())
  checkHealth()
  probeAuth()
})

onUnmounted(() => {
  if (typeof window !== 'undefined') {
    window.removeEventListener('click', handleDocumentClick)
    window.removeEventListener('keydown', handleGlobalKeydown)
  }
})
</script>

<template>
  <div class="min-h-screen bg-base-100 flex flex-col md:flex-row text-base-content overflow-x-hidden">
    <!-- Desktop Collapsible Sidebar -->
    <aside
      class="hidden md:flex flex-col bg-base-200 border-r border-base-300 select-none shrink-0 transition-[width] duration-200 ease-in-out"
      :class="isCollapsed ? 'w-20' : 'w-64'"
    >
      <div class="h-16 flex items-center justify-between px-4 border-b border-base-300">
        <div class="flex items-center gap-3 overflow-hidden">
          <div class="w-8 h-8 rounded-lg bg-primary text-primary-content flex items-center justify-center font-bold text-lg shadow-sm shrink-0">
            C
          </div>
          <div v-if="!isCollapsed" class="min-w-0 transition-opacity duration-150">
            <h1 class="font-bold text-sm tracking-wide truncate">CSP Control Plane</h1>
            <p class="text-xs text-base-content/60 truncate">v1.0 Clean-Slate</p>
          </div>
        </div>

        <button
          type="button"
          @click="toggleCollapse"
          class="btn btn-ghost btn-xs btn-circle text-base-content/60 hover:text-base-content"
          :title="isCollapsed ? t('nav.expand') : t('nav.collapse')"
          aria-label="Toggle Sidebar"
          data-testid="sidebar-collapse-toggle"
        >
          <ChevronDoubleLeftIcon v-if="!isCollapsed" class="w-4 h-4" />
          <ChevronDoubleRightIcon v-else class="w-4 h-4" />
        </button>
      </div>

      <nav class="flex-1 p-3 space-y-1.5 overflow-y-auto">
        <RouterLink
          v-for="item in navItems"
          :key="item.id"
          :to="item.path"
          custom
          v-slot="{ href, navigate, isActive }"
        >
          <button
            :href="href"
            type="button"
            @click="(e) => { activeTab = item.id; setRouteTab(item.id as NavTab); if (navigate) navigate(e); else router.push(item.path); }"
            class="w-full flex items-center gap-3 rounded-lg text-sm font-medium transition-colors"
            :class="[
              isActive || isItemActive(item)
                ? 'bg-primary text-primary-content shadow-sm'
                : 'hover:bg-base-300/60 text-base-content/80',
              isCollapsed ? 'justify-center px-2 py-2.5' : 'px-3.5 py-2.5',
            ]"
            :title="isCollapsed ? t(item.labelKey) : undefined"
          >
            <component :is="item.icon" class="w-5 h-5 flex-shrink-0" />
            <span v-if="!isCollapsed" class="truncate">{{ t(item.labelKey) }}</span>
          </button>
        </RouterLink>
      </nav>

      <div class="p-3 border-t border-base-300">
        <div v-if="!isCollapsed" class="flex items-center justify-between text-xs text-base-content/60">
          <span>Theme: {{ currentTheme }}</span>
          <div class="flex gap-1">
            <button
              v-for="tName in themes.slice(0, 3)"
              :key="tName"
              @click="switchTheme(tName)"
              class="btn btn-xs btn-ghost"
              :class="{ 'btn-active': currentTheme === tName }"
            >
              {{ tName[0].toUpperCase() }}
            </button>
          </div>
        </div>
        <div v-else class="flex justify-center">
          <button
            @click="switchTheme(currentTheme === 'dark' ? 'light' : 'dark')"
            class="btn btn-ghost btn-xs btn-circle"
            :title="`Theme: ${currentTheme}`"
          >
            <SunIcon v-if="currentTheme === 'light'" class="w-4 h-4" />
            <MoonIcon v-else class="w-4 h-4" />
          </button>
        </div>
      </div>
    </aside>

    <!-- Main Container -->
    <div class="flex-1 flex flex-col min-w-0 overflow-x-hidden">
      <!-- Topbar Header -->
      <header class="h-16 bg-base-100 border-b border-base-300 flex items-center justify-between px-3 sm:px-6 sticky top-0 z-10 backdrop-blur bg-base-100/80 w-full overflow-hidden overflow-visible">
        <div class="flex items-center gap-2 sm:gap-3 min-w-0 shrink">
          <RouterLink
            to="/dashboard"
            class="md:hidden w-8 h-8 rounded-lg bg-primary text-primary-content flex items-center justify-center font-bold text-base shadow-sm shrink-0"
            aria-label="Dashboard"
          >
            C
          </RouterLink>
          <span class="font-semibold text-sm sm:text-base capitalize truncate">{{ currentTitle }}</span>
        </div>

        <div class="flex items-center gap-1.5 sm:gap-3 shrink-0">
          <!-- Auth Badge -->
          <RouterLink to="/settings" custom v-slot="{ href, navigate }">
            <button
              :href="href"
              type="button"
              data-testid="header-auth-badge"
              class="flex items-center gap-1 sm:gap-1.5 px-1.5 sm:px-2.5 py-1 rounded-full text-xs font-medium border border-base-300 transition-colors cursor-pointer header-auth-badge shrink-0 touch-manipulation select-none"
              :class="{
                'bg-warning/10 text-warning border-warning/30 hover:bg-warning/20': authState === 'open',
                'bg-success/10 text-success border-success/30 hover:bg-success/20': authState === 'authenticated',
                'bg-error/10 text-error border-error/30 hover:bg-error/20': authState === 'unauthenticated',
                'bg-base-200/50 text-base-content/60': authState === 'probing',
                'bg-error/10 text-error border-error/30': authState === 'error',
              }"
              :title="authState === 'open' ? '当前为零配置私有开放模式' : authState === 'authenticated' ? 'Protected by Admin Token (已认证，点击管理)' : authState === 'unauthenticated' ? 'Protected by Admin Token (未认证)' : '认证探测中...'"
              @click="(e) => { activeTab = 'settings'; setRouteTab('settings'); if (navigate) navigate(e); else router.push('/settings'); }"
            >
              <span
                class="w-2 h-2 rounded-full inline-block shrink-0"
                :class="{
                  'bg-warning': authState === 'open',
                  'bg-success': authState === 'authenticated',
                  'bg-error': authState === 'unauthenticated' || authState === 'error',
                  'bg-base-content/40 animate-pulse': authState === 'probing',
                }"
              />
              <span class="hidden sm:inline whitespace-nowrap">{{ authState === 'open' ? 'Open Mode' : (authState === 'authenticated' || authState === 'unauthenticated') ? 'Protected' : authState === 'probing' ? 'Probing...' : 'Error' }}</span>
            </button>
          </RouterLink>

          <!-- Live Health Indicator -->
          <div class="flex items-center gap-1 sm:gap-1.5 px-1.5 sm:px-2.5 py-1 rounded-full text-xs font-medium border border-base-300 bg-base-200/50 header-health-indicator shrink-0 select-none">
            <span
              class="w-2 h-2 rounded-full shrink-0"
              :class="{
                'bg-warning animate-pulse': healthStatus === 'checking',
                'bg-success': healthStatus === 'healthy',
                'bg-error': healthStatus === 'unhealthy'
              }"
            />
            <span class="hidden sm:inline capitalize whitespace-nowrap">{{ healthStatus }}</span>
            <button @click="checkHealth" class="btn btn-ghost btn-xs btn-circle ml-0.5 touch-manipulation" title="Refresh health" aria-label="Refresh health">
              <ArrowPathIcon class="w-3 h-3" :class="{ 'animate-spin': healthStatus === 'checking' }" />
            </button>
          </div>

          <!-- Language Toggle Button -->
          <button
            type="button"
            class="btn btn-ghost btn-sm px-2 font-mono text-xs touch-manipulation select-none"
            :title="t('topbar.switchLang')"
            aria-label="Switch Language"
            @click="toggleLocale"
          >
            {{ currentLocale === 'zh-CN' ? '中 / EN' : 'EN / 中' }}
          </button>

          <!-- Theme Dropdown -->
          <div
            ref="themeDropdownRef"
            class="dropdown dropdown-end shrink-0"
            :class="{ 'dropdown-open': isThemeOpen }"
            @keydown.esc="closeThemeDropdown"
          >
            <button
              ref="themeToggleBtnRef"
              type="button"
              tabindex="0"
              class="btn btn-ghost btn-sm btn-circle touch-manipulation"
              aria-label="Toggle Theme"
              aria-haspopup="true"
              :aria-expanded="isThemeOpen ? 'true' : 'false'"
              @click="toggleThemeDropdown"
            >
              <SunIcon v-if="currentTheme === 'light'" class="w-5 h-5" />
              <MoonIcon v-else class="w-5 h-5" />
            </button>
            <ul
              tabindex="0"
              class="dropdown-content menu p-2 shadow-lg bg-base-200 rounded-box w-40 z-20 border border-base-300 text-xs mt-1 max-h-[calc(100vh-5rem)] overflow-y-auto"
              role="menu"
              aria-label="Themes"
            >
              <li v-for="tName in themes" :key="tName" role="none">
                <button
                  type="button"
                  role="menuitem"
                  @click="switchTheme(tName)"
                  class="capitalize touch-manipulation w-full text-left"
                  :class="{ 'active': currentTheme === tName }"
                >
                  {{ tName }}
                </button>
              </li>
            </ul>
          </div>
        </div>
      </header>

      <!-- Main Body: routed content scroll container with dynamic clearance -->
      <main
        class="flex-1 p-3 sm:p-6 max-w-7xl w-full mx-auto space-y-6 overflow-x-hidden"
        :style="contentInsetStyle"
      >
        <!-- 1. Probing Skeleton -->
        <div v-if="authState === 'probing'" class="space-y-4 animate-pulse p-4" data-testid="auth-probing-skeleton">
          <div class="h-8 bg-base-300 rounded w-1/4"></div>
          <div class="h-32 bg-base-300 rounded"></div>
          <div class="h-64 bg-base-300 rounded"></div>
        </div>

        <!-- 2. Probing Error -->
        <div v-else-if="authState === 'error'" class="alert alert-error shadow-lg" data-testid="auth-error-alert">
          <div>
            <h3 class="font-bold">认证状态探测失败</h3>
            <div class="text-xs">{{ authErrorMessage || '无法连接控制面认证服务' }}</div>
          </div>
          <div class="flex-none">
            <button class="btn btn-sm btn-outline" @click="probeAuth">重试探测</button>
          </div>
        </div>

        <!-- 3. Unauthenticated: AuthGate -->
        <AuthGate v-else-if="authState === 'unauthenticated'" />

        <!-- 4. Open or Authenticated: Business views via RouterView -->
        <template v-else>
          <RouterView :key="viewKey" />
        </template>
      </main>

      <ToastContainer />

      <!-- Mobile Bottom Navigation (Floating Pill Island) -->
      <nav
        ref="dockRef"
        data-testid="mobile-dock"
        class="md:hidden fixed bottom-3 sm:bottom-4 inset-x-3 sm:inset-x-6 mx-auto max-w-md py-2 px-2.5 bg-base-200/90 backdrop-blur-xl border border-white/10 shadow-2xl rounded-2xl z-20 pb-[calc(0.5rem+env(safe-area-inset-bottom,0px))] touch-manipulation select-none"
        aria-label="Mobile Navigation Dock"
      >
        <div class="grid grid-cols-4 gap-1.5 w-full">
          <RouterLink
            v-for="item in navItems.slice(0, 4)"
            :key="item.id"
            :to="item.path"
            custom
            v-slot="{ href, navigate, isActive }"
          >
            <button
              :href="href"
              type="button"
              :data-testid="`dock-link-${item.id}`"
              @click="(e) => { activeTab = item.id; setRouteTab(item.id as NavTab); if (navigate) navigate(e); else router.push(item.path); }"
              class="flex flex-col items-center justify-center py-1.5 px-1 rounded-xl text-[10px] sm:text-xs font-medium transition-all duration-150 touch-manipulation select-none active:scale-95"
              :class="isActive || isItemActive(item) ? 'bg-primary/15 text-primary font-semibold shadow-sm' : 'text-base-content/70 hover:text-base-content hover:bg-base-300/40'"
            >
              <component :is="item.icon" class="w-4 h-4 sm:w-5 sm:h-5 mb-0.5 shrink-0" />
              <span class="truncate max-w-full leading-tight text-center">{{ t(item.labelKey) }}</span>
            </button>
          </RouterLink>
        </div>
        <div class="grid grid-cols-3 gap-1.5 w-full mt-1.5">
          <RouterLink
            v-for="item in navItems.slice(4)"
            :key="item.id"
            :to="item.path"
            custom
            v-slot="{ href, navigate, isActive }"
          >
            <button
              :href="href"
              type="button"
              :data-testid="`dock-link-${item.id}`"
              @click="(e) => { activeTab = item.id; setRouteTab(item.id as NavTab); if (navigate) navigate(e); else router.push(item.path); }"
              class="flex flex-col items-center justify-center py-1.5 px-1 rounded-xl text-[10px] sm:text-xs font-medium transition-all duration-150 touch-manipulation select-none active:scale-95"
              :class="isActive || isItemActive(item) ? 'bg-primary/15 text-primary font-semibold shadow-sm' : 'text-base-content/70 hover:text-base-content hover:bg-base-300/40'"
            >
              <component :is="item.icon" class="w-4 h-4 sm:w-5 sm:h-5 mb-0.5 shrink-0" />
              <span class="truncate max-w-full leading-tight text-center">{{ t(item.labelKey) }}</span>
            </button>
          </RouterLink>
        </div>
      </nav>
    </div>
  </div>
</template>
