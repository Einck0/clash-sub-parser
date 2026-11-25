<script setup lang="ts">
import { ref, computed, nextTick, watch, onMounted, onUnmounted, type Component } from 'vue'
import { useRoute, useRouter, RouterView, RouterLink } from 'vue-router'
import {
  Squares2X2Icon,
  DocumentDuplicateIcon,
  ServerStackIcon,
  GlobeAltIcon,
  CpuChipIcon,
  ShieldCheckIcon,
  ArrowDownTrayIcon,
  Cog6ToothIcon,
  LinkIcon,
  SunIcon,
  MoonIcon,
  ArrowPathIcon,
  ChevronDoubleLeftIcon,
  ChevronDoubleRightIcon,
  EllipsisHorizontalIcon,
} from '@heroicons/vue/24/outline'
import { api, ApiError } from './api/client'
import { applyTheme, getStoredTheme, THEME_NAMES, type ThemeName } from './theme'
import ToastContainer from './ui/ToastContainer.vue'
import { useAuth } from './features/auth/useAuth'
import AuthGate from './features/auth/AuthGate.vue'
import { useContentInset } from './composables/useContentInset'
import Drawer from './ui/Drawer.vue'
import { drawerStack } from './ui/drawerStack'
import {
  getRouteTab,
  isMobileMoreTab,
  isLegacyHash,
  type NavTab,
} from './navigation'
import { currentLocale, toggleLocale, t } from './locales'
import { uiText } from './locales/redesign'
import { displayText } from './utils/displayNames'
import { router as fallbackRouter } from './router'

const injectedRouter = useRouter()
const injectedRoute = useRoute()
const router = injectedRouter || fallbackRouter

const { dockRef, style: contentInsetStyle } = useContentInset(52)

const currentTheme = ref<ThemeName>('dark')
const themes = THEME_NAMES

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

interface NavItemDef {
  id: NavTab
  path: string
  labelKey: string
  defaultLabel: string
  labelZh?: string
  labelEn?: string
  icon: Component
}

function navItemLabel(item: NavItemDef): string {
  if (item.labelZh && item.labelEn) {
    return uiText(item.labelZh, item.labelEn)
  }
  const translated = t(item.labelKey)
  return translated !== item.labelKey ? translated : item.defaultLabel
}

// 8 Routes grouped logically for desktop sidebar
const navGroups: Array<{
  id: string
  titleZh: string
  titleEn: string
  items: NavItemDef[]
}> = [
  {
    id: 'core',
    titleZh: '核心路由',
    titleEn: 'Routing Core',
    items: [
      { id: 'nodes', path: '/nodes', labelKey: 'nav.nodes', defaultLabel: 'Node Ledger', icon: ServerStackIcon },
      { id: 'policy', path: '/policy', labelKey: 'nav.policy', defaultLabel: 'Policy Tree', icon: CpuChipIcon },
      { id: 'subscriptions', path: '/subscriptions', labelKey: 'nav.subscriptions', defaultLabel: 'Subscriptions', icon: DocumentDuplicateIcon },
    ],
  },
  {
    id: 'rules-dns',
    titleZh: '规则与解析',
    titleEn: 'Rules & DNS',
    items: [
      { id: 'rules', path: '/rules', labelKey: 'nav.rules', defaultLabel: 'Rule Desk', icon: ShieldCheckIcon },
      { id: 'dns', path: '/dns', labelKey: 'nav.dns', defaultLabel: 'DNS', icon: GlobeAltIcon },
      { id: 'chains', path: '/chains', labelKey: 'nav.chains', defaultLabel: 'Proxy Chains', labelZh: '链式代理', labelEn: 'Proxy Chains', icon: LinkIcon },
    ],
  },
  {
    id: 'system',
    titleZh: '系统与发布',
    titleEn: 'System & Exports',
    items: [
      { id: 'dashboard', path: '/dashboard', labelKey: 'nav.dashboard', defaultLabel: 'Dashboard', icon: Squares2X2Icon },
      { id: 'publications', path: '/publications', labelKey: 'nav.publications', defaultLabel: 'Exports', icon: ArrowDownTrayIcon },
      { id: 'settings', path: '/settings', labelKey: 'nav.settings', defaultLabel: 'Settings', icon: Cog6ToothIcon },
    ],
  },
]

// Flattened list for lookup
const allNavItems: NavItemDef[] = navGroups.flatMap((g) => g.items)

// Mobile primary items: Nodes, Policy, Subscriptions
const mobilePrimaryItems: NavItemDef[] = [
  allNavItems.find((i) => i.id === 'nodes')!,
  allNavItems.find((i) => i.id === 'policy')!,
  allNavItems.find((i) => i.id === 'subscriptions')!,
]

// Mobile more items: Dashboard, DNS, Chains, Rules, Publications, Settings
const mobileMoreItems: NavItemDef[] = [
  allNavItems.find((i) => i.id === 'dashboard')!,
  allNavItems.find((i) => i.id === 'dns')!,
  allNavItems.find((i) => i.id === 'chains')!,
  allNavItems.find((i) => i.id === 'rules')!,
  allNavItems.find((i) => i.id === 'publications')!,
  allNavItems.find((i) => i.id === 'settings')!,
]

const currentActiveId = computed<string>(() => {
  return String(injectedRoute?.name || getRouteTab() || 'dashboard')
})

const isMoreActive = computed<boolean>(() => {
  return isMobileMoreTab(currentActiveId.value)
})

const isMoreOpen = ref<boolean>(false)

function toggleMore() {
  closeThemeDropdown()
  isMoreOpen.value = !isMoreOpen.value
}

function closeMore() {
  isMoreOpen.value = false
}

const currentTitle = computed(() => {
  const rName = currentActiveId.value
  const matched = allNavItems.find((item) => item.id === rName)
  if (matched) {
    return navItemLabel(matched)
  }
  return rName.replace('-', ' ')
})

function themeLabel(theme: ThemeName): string {
  switch (theme) {
    case 'light':
      return t('themes.light')
    case 'dark':
      return t('themes.dark')
    case 'dim':
      return t('themes.dim')
    case 'cyberpunk':
      return t('themes.cyberpunk')
    case 'cupcake':
      return t('themes.cupcake')
    case 'dracula':
      return t('themes.dracula')
    case 'nord':
      return t('themes.nord')
    default:
      return theme
  }
}

const isItemActive = (item: NavItemDef) => {
  if (injectedRoute && injectedRoute.name) {
    return injectedRoute.name === item.id || injectedRoute.path === item.path
  }
  return currentActiveId.value === item.id
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

const toggleThemeDropdown = async () => {
  if (isThemeOpen.value) {
    closeThemeDropdown()
  } else {
    closeMore()
    isThemeOpen.value = true
    await nextTick()
    themeDropdownRef.value?.querySelector<HTMLButtonElement>('[aria-checked="true"]')?.focus()
  }
}

const closeThemeDropdown = (restoreFocus = true) => {
  const ownedFocus = themeDropdownRef.value?.contains(document.activeElement)
  isThemeOpen.value = false
  if (restoreFocus && ownedFocus) themeToggleBtnRef.value?.focus()
}

function handleThemeKeydown(event: KeyboardEvent) {
  if (!isThemeOpen.value) return
  const options = Array.from(themeDropdownRef.value?.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]') || [])
  const current = options.indexOf(document.activeElement as HTMLButtonElement)
  if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
    event.preventDefault()
    const index = event.key === 'Home' ? 0 : event.key === 'End' ? options.length - 1
      : (current + (event.key === 'ArrowDown' ? 1 : -1) + options.length) % options.length
    options[index]?.focus()
  } else if (event.key === 'Tab') closeThemeDropdown(false)
}

function focusMain() {
  document.getElementById('main-content')?.focus()
}

function handleViewportResize() {
  if (window.innerWidth >= 768) closeMore()
}

watch(() => injectedRoute?.fullPath, () => {
  closeMore()
  closeThemeDropdown(false)
})

const switchTheme = (theme: ThemeName) => {
  currentTheme.value = theme
  applyTheme(theme)
  closeThemeDropdown()
}

const handleDocumentClick = (e: MouseEvent) => {
  const target = e.target as Node | null
  if (isThemeOpen.value) {
    if (themeDropdownRef.value && target && !themeDropdownRef.value.contains(target)) {
      closeThemeDropdown(false)
    }
  }
}

const handleGlobalKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Escape' && isThemeOpen.value && !e.defaultPrevented) {
    e.preventDefault()
    closeThemeDropdown()
  }
}

let healthRequest = 0
const checkHealth = async () => {
  const request = ++healthRequest
  healthStatus.value = 'checking'
  requestId.value = ''
  try {
    const res = await api.get<{ status: string }>('/healthz')
    if (request !== healthRequest) return
    healthStatus.value = 'healthy'
    healthDetails.value = displayText(res.status)
  } catch (err) {
    if (request !== healthRequest) return
    healthStatus.value = 'unhealthy'
    if (err instanceof ApiError) {
      healthDetails.value = displayText(`${err.code}: ${err.message}`)
      requestId.value = err.requestId || ''
    } else {
      healthDetails.value = displayText(String(err))
    }
  }
}

function handleNavigation(tabId: NavTab, path: string, event?: MouseEvent) {
  if (event && (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)) return
  event?.preventDefault()
  closeMore()
  if (router.currentRoute.value.name === tabId) return
  return router.push(path).catch(() => {})
}

const handleHashChange = () => {
  if (typeof window === 'undefined') return
  const rawHash = window.location.hash || ''
  // Only normalize non-canonical legacy hashes (e.g. #nodes, #subscriptions without leading slash).
  // Canonical hashes (#/nodes, #/settings) and empty hashes are governed exclusively by VueRouter.
  if (isLegacyHash(rawHash)) {
    const targetTab = getRouteTab(rawHash)
    if (targetTab && router?.currentRoute?.value?.name !== targetTab) {
      router.replace({ name: targetTab }).catch(() => {})
    }
  }
}

const handleAuthSuccess = () => {
  viewKey.value++
  probeAuth()
  checkHealth()
}

onMounted(() => {
  if (typeof window !== 'undefined') {
    window.addEventListener('resize', handleViewportResize)
    window.addEventListener('click', handleDocumentClick)
    window.addEventListener('keydown', handleGlobalKeydown)
    window.addEventListener('hashchange', handleHashChange)
    window.addEventListener('csp:auth-success', handleAuthSuccess)
    // If mounted with a legacy non-canonical hash (e.g. #nodes), normalize it once
    handleHashChange()
  }
  switchTheme(getStoredTheme())
  checkHealth()
  probeAuth()
})

onUnmounted(() => {
  healthRequest++
  if (typeof window !== 'undefined') {
    window.removeEventListener('resize', handleViewportResize)
    window.removeEventListener('click', handleDocumentClick)
    window.removeEventListener('keydown', handleGlobalKeydown)
    window.removeEventListener('hashchange', handleHashChange)
    window.removeEventListener('csp:auth-success', handleAuthSuccess)
  }
})
</script>

<template>
  <div class="csp-app min-h-screen bg-base-100 flex flex-col md:flex-row text-base-content">
    <a href="#main-content" class="csp-skip-link" @click.prevent="focusMain">{{ uiText('跳到主要内容', 'Skip to main content') }}</a>
    <!-- Desktop Collapsible Grouped Sidebar -->
    <aside
      :inert="drawerStack.length > 0"
      class="csp-sidebar hidden md:flex flex-col bg-base-200 border-r border-base-300 select-none shrink-0 sticky top-0 h-dvh overflow-y-auto transition-[width] duration-200 ease-in-out z-20"
      :class="isCollapsed ? 'w-20' : 'w-64'"
    >
      <!-- Sidebar Header / Logo -->
      <div class="h-16 flex items-center justify-between px-4 border-b border-base-300 shrink-0">
        <div class="flex items-center gap-3 overflow-hidden">
          <div class="w-8 h-8 rounded-lg bg-primary text-primary-content flex items-center justify-center font-bold text-lg shadow-sm shrink-0">
            C
          </div>
          <div v-if="!isCollapsed" class="min-w-0 transition-opacity duration-150">
            <p class="font-bold text-sm tracking-wide truncate">{{ t('topbar.sidebarTitle') }}</p>
            <p class="text-xs text-base-content/60 truncate">{{ t('topbar.sidebarVersion') }}</p>
          </div>
        </div>

        <button
          type="button"
          @click="toggleCollapse"
          class="btn btn-ghost btn-xs btn-circle text-base-content/60 hover:text-base-content shrink-0"
          :title="isCollapsed ? t('nav.expand') : t('nav.collapse')"
          :aria-label="isCollapsed ? t('nav.expand') : t('nav.collapse')"
          :aria-expanded="!isCollapsed"
          data-testid="sidebar-collapse-toggle"
        >
          <ChevronDoubleLeftIcon v-if="!isCollapsed" class="w-4 h-4" />
          <ChevronDoubleRightIcon v-else class="w-4 h-4" />
        </button>
      </div>

      <!-- Sidebar Grouped Navigation Links -->
      <nav class="flex-1 p-3 space-y-4 overflow-y-auto" :aria-label="uiText('主导航', 'Main navigation')">
        <div v-for="group in navGroups" :key="group.id" class="space-y-1">
          <div
            v-if="!isCollapsed"
            class="px-3 pt-2 pb-1 text-[11px] font-semibold uppercase tracking-wider text-base-content/40 truncate select-none"
          >
            {{ uiText(group.titleZh, group.titleEn) }}
          </div>
          <div v-else class="h-px bg-base-300/40 my-2 mx-2" />

          <RouterLink
            v-for="item in group.items"
            :key="item.id"
            :to="item.path"
            custom
            v-slot="{ href, isActive }"
          >
            <a
              :href="href"
              :aria-label="navItemLabel(item)"
              :aria-current="(isActive || isItemActive(item)) ? 'page' : undefined"
              @click="handleNavigation(item.id, item.path, $event)"
              class="csp-nav-link w-full flex items-center gap-3 rounded-lg text-sm font-medium transition-colors cursor-pointer"
              :class="[
                isActive || isItemActive(item)
                  ? 'bg-primary text-primary-content shadow-sm'
                  : 'hover:bg-base-300/60 text-base-content/80',
                isCollapsed ? 'justify-center px-2 py-2.5' : 'px-3.5 py-2.5',
              ]"
              :title="isCollapsed ? navItemLabel(item) : undefined"
            >
              <component :is="item.icon" class="w-5 h-5 shrink-0" />
              <span v-if="!isCollapsed" class="truncate">{{ navItemLabel(item) }}</span>
            </a>
          </RouterLink>
        </div>
      </nav>

      <!-- Desktop Sidebar Footer (Themes) -->
      <div class="p-3 border-t border-base-300 shrink-0">
        <div v-if="!isCollapsed" class="flex items-center justify-between text-xs text-base-content/60">
          <span>{{ t('themes.label') }}：{{ themeLabel(currentTheme) }}</span>
          <div class="flex gap-1">
            <button
              v-for="tName in themes.slice(0, 3)"
              :key="tName"
              @click="switchTheme(tName)"
              class="btn btn-xs btn-ghost"
              :class="{ 'btn-active': currentTheme === tName }"
              :aria-label="themeLabel(tName)"
              :aria-pressed="currentTheme === tName"
            >
              {{ tName[0].toUpperCase() }}
            </button>
          </div>
        </div>
        <div v-else class="flex justify-center">
          <button
            @click="switchTheme(currentTheme === 'dark' ? 'light' : 'dark')"
            class="btn btn-ghost btn-xs btn-circle"
            :title="`${t('themes.label')}：${themeLabel(currentTheme)}`"
            :aria-label="uiText('切换明暗主题', 'Switch light and dark theme')"
          >
            <SunIcon v-if="currentTheme === 'light'" class="w-4 h-4" />
            <MoonIcon v-else class="w-4 h-4" />
          </button>
        </div>
      </div>
    </aside>

    <!-- Main Content Container -->
    <div class="flex-1 flex flex-col min-w-0 overflow-x-hidden">
      <!-- Topbar Header -->
      <header :inert="drawerStack.length > 0" class="csp-topbar h-16 bg-base-100 border-b border-base-300 flex items-center justify-between px-3 sm:px-6 sticky top-0 z-20 backdrop-blur bg-base-100/80 w-full overflow-visible">
        <div class="flex items-center gap-2 sm:gap-3 min-w-0 shrink">
          <RouterLink
            to="/dashboard"
            class="md:hidden w-8 h-8 rounded-lg bg-primary text-primary-content flex items-center justify-center font-bold text-base shadow-sm shrink-0"
            :aria-label="t('nav.dashboard')"
          >
            C
          </RouterLink>
          <!-- Single visible title level -->
          <span class="font-semibold text-sm sm:text-base capitalize truncate">{{ currentTitle }}</span>
        </div>

        <div class="flex items-center gap-1.5 sm:gap-3 shrink-0">
          <!-- Auth Badge -->
          <RouterLink to="/settings" custom v-slot="{ href }">
            <a
              :href="href"
              data-testid="header-auth-badge"
              :aria-label="uiText('查看访问保护设置', 'View access protection settings')"
              class="flex items-center gap-1 sm:gap-1.5 px-1.5 sm:px-2.5 py-1 rounded-full text-xs font-medium border border-base-300 transition-colors cursor-pointer header-auth-badge shrink-0 touch-manipulation select-none"
              :class="{
                'bg-warning/10 text-warning border-warning/30 hover:bg-warning/20': authState === 'open',
                'bg-success/10 text-success border-success/30 hover:bg-success/20': authState === 'authenticated',
                'bg-error/10 text-error border-error/30 hover:bg-error/20': authState === 'unauthenticated',
                'bg-base-200/50 text-base-content/60': authState === 'probing',
                'bg-error/10 text-error border-error/30': authState === 'error',
              }"
              :title="authState === 'open' ? t('topbar.openModeTooltip') : authState === 'authenticated' ? t('topbar.protectedAuthTooltip') : authState === 'unauthenticated' ? t('topbar.protectedUnauthTooltip') : t('topbar.probingTooltip')"
              @click="handleNavigation('settings', '/settings', $event)"
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
              <span class="hidden sm:inline whitespace-nowrap">{{ authState === 'open' ? t('topbar.openMode') : (authState === 'authenticated' || authState === 'unauthenticated') ? t('topbar.protected') : authState === 'probing' ? t('topbar.probing') : t('topbar.authError') }}</span>
            </a>
          </RouterLink>

          <!-- Live Health Indicator -->
          <div :title="healthStatus === 'unhealthy' ? healthDetails : uiText('服务响应状态，不代表节点或配置已验证', 'Service response, not node or configuration validation')" class="flex items-center gap-1 sm:gap-1.5 px-1.5 sm:px-2.5 py-1 rounded-full text-xs font-medium border border-base-300 bg-base-200/50 header-health-indicator shrink-0 select-none">
            <span
              class="w-2 h-2 rounded-full shrink-0"
              :class="{
                'bg-warning animate-pulse': healthStatus === 'checking',
                'bg-success': healthStatus === 'healthy',
                'bg-error': healthStatus === 'unhealthy'
              }"
            />
            <span class="hidden sm:inline capitalize whitespace-nowrap">{{ healthStatus === 'healthy' ? uiText('服务在线', 'Service online') : healthStatus === 'unhealthy' ? t('topbar.connectionError') : t('topbar.checking') }}</span>
            <button type="button" :disabled="healthStatus === 'checking'" @click="checkHealth" class="btn btn-ghost btn-xs btn-circle ml-0.5 touch-manipulation" :title="t('topbar.refreshHealth')" :aria-label="t('topbar.refreshHealth')">
              <ArrowPathIcon class="w-3 h-3" :class="{ 'animate-spin': healthStatus === 'checking' }" />
            </button>
          </div>

          <!-- Language Toggle Button -->
          <button
            type="button"
            class="btn btn-ghost btn-sm px-2 font-mono text-xs touch-manipulation select-none"
            :title="t('topbar.switchLang')"
            :aria-label="t('topbar.switchLang')"
            data-testid="language-toggle"
            @click="toggleLocale"
          >
            {{ currentLocale === 'zh-CN' ? '中 / EN' : 'EN / 中' }}
          </button>

          <!-- Theme Dropdown (with focus and menu-active support) -->
          <div
            ref="themeDropdownRef"
            class="dropdown dropdown-end shrink-0"
            :class="{ 'dropdown-open': isThemeOpen }"
            @keydown="handleThemeKeydown"
          >
            <button
              ref="themeToggleBtnRef"
              type="button"
              tabindex="0"
              class="btn btn-ghost btn-sm btn-circle touch-manipulation"
              :aria-label="uiText('切换主题', 'Choose theme')"
              data-testid="theme-toggle"
              aria-haspopup="menu"
              aria-controls="theme-options"
              :aria-expanded="isThemeOpen ? 'true' : 'false'"
              @click="toggleThemeDropdown"
            >
              <SunIcon v-if="currentTheme === 'light'" class="w-5 h-5" />
              <MoonIcon v-else class="w-5 h-5" />
            </button>
            <ul
              v-show="isThemeOpen"
              id="theme-options"
              class="dropdown-content menu p-2 shadow-lg bg-base-200 rounded-box w-44 z-30 border border-base-300 text-sm mt-1 max-h-[calc(100dvh-5rem)] overflow-y-auto"
              role="menu"
              :aria-label="t('themes.label')"
            >
              <li v-for="tName in themes" :key="tName" role="none">
                <button
                  type="button"
                  role="menuitemradio"
                  :aria-checked="currentTheme === tName"
                  tabindex="-1"
                  @click="switchTheme(tName)"
                  class="capitalize touch-manipulation w-full text-left"
                  :class="{ 'menu-active': currentTheme === tName, 'active': currentTheme === tName }"
                >
                  {{ themeLabel(tName) }}
                </button>
              </li>
            </ul>
          </div>
        </div>
      </header>

      <!-- Main Body: routed content scroll container with dynamic clearance -->
      <main
        :inert="drawerStack.length > 0"
        id="main-content"
        tabindex="-1"
        :aria-label="currentTitle"
        class="csp-main flex-1 p-3 sm:p-6 max-w-7xl w-full mx-auto space-y-6 overflow-x-hidden"
        :style="contentInsetStyle"
      >
        <h1 class="sr-only">{{ currentTitle }}</h1>
        <!-- 1. Probing Skeleton -->
        <div v-if="authState === 'probing'" role="status" :aria-label="uiText('正在检查访问状态', 'Checking access')" class="space-y-4 animate-pulse p-4" data-testid="auth-probing-skeleton">
          <div class="h-8 bg-base-300 rounded w-1/4"></div>
          <div class="h-32 bg-base-300 rounded"></div>
          <div class="h-64 bg-base-300 rounded"></div>
        </div>

        <!-- 2. Probing Error -->
        <div v-else-if="authState === 'error'" role="alert" class="alert alert-error shadow-lg flex flex-wrap [overflow-wrap:anywhere]" data-testid="auth-error-alert">
          <div>
            <h3 class="font-bold">{{ t('topbar.authProbeError') }}</h3>
            <div class="text-xs">{{ authErrorMessage || t('topbar.authProbeErrorDesc') }}</div>
          </div>
          <div class="flex-none">
            <button class="btn btn-sm btn-outline" @click="probeAuth">{{ t('topbar.retryProbe') }}</button>
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

      <!-- Mobile Bottom Navigation: 4 items (Nodes, Policy, Subscriptions, More) -->
      <nav
        :inert="drawerStack.length > 0"
        ref="dockRef"
        data-testid="mobile-dock"
        class="csp-mobile-dock md:hidden fixed bottom-3 sm:bottom-4 inset-x-3 sm:inset-x-6 mx-auto max-w-md py-2 px-2.5 bg-base-200/90 backdrop-blur-xl border border-white/10 shadow-2xl rounded-2xl z-20 pb-[calc(0.5rem+env(safe-area-inset-bottom,0px))] touch-manipulation select-none"
        :aria-label="uiText('移动导航', 'Mobile navigation')"
      >
        <!-- Single 4-column row -->
        <div class="grid grid-cols-4 gap-1.5 w-full">
          <!-- 1. Node Ledger -->
          <RouterLink
            v-for="item in mobilePrimaryItems"
            :key="item.id"
            :to="item.path"
            custom
            v-slot="{ href, isActive }"
          >
            <a
              :href="href"
              :aria-current="(isActive || isItemActive(item)) ? 'page' : undefined"
              :data-testid="`dock-link-${item.id}`"
              @click="handleNavigation(item.id, item.path, $event)"
              class="flex flex-col items-center justify-center py-1.5 px-1 rounded-xl text-xs font-medium transition-all duration-150 touch-manipulation select-none active:scale-95 cursor-pointer"
              :class="isActive || isItemActive(item) ? 'bg-primary/15 text-primary font-semibold shadow-sm' : 'text-base-content/70 hover:text-base-content hover:bg-base-300/40'"
            >
              <component :is="item.icon" class="w-4 h-4 sm:w-5 sm:h-5 mb-0.5 shrink-0" />
              <span class="truncate max-w-full leading-tight text-center">{{ navItemLabel(item) }}</span>
            </a>
          </RouterLink>

          <!-- 4. More Button -->
          <button
            type="button"
            data-testid="dock-link-more"
            @click="toggleMore"
            class="flex flex-col items-center justify-center py-1.5 px-1 rounded-xl text-[10px] sm:text-xs font-medium transition-all duration-150 touch-manipulation select-none active:scale-95 cursor-pointer"
            :class="isMoreActive || isMoreOpen ? 'bg-primary/15 text-primary font-semibold shadow-sm' : 'text-base-content/70 hover:text-base-content hover:bg-base-300/40'"
            :aria-expanded="isMoreOpen ? 'true' : 'false'"
            :aria-label="uiText('更多页面', 'More pages')"
            aria-haspopup="dialog"
          >
            <EllipsisHorizontalIcon class="w-4 h-4 sm:w-5 sm:h-5 mb-0.5 shrink-0" />
            <span class="truncate max-w-full leading-tight text-center">{{ uiText('更多', 'More') }}</span>
          </button>
        </div>

        <Drawer v-model="isMoreOpen" :title="uiText('更多页面', 'More pages')" width-class="md:max-w-md">
          <div data-testid="mobile-more-sheet" class="grid grid-cols-2 gap-3">
            <RouterLink v-for="item in mobileMoreItems" :key="item.id" :to="item.path" custom v-slot="{ href, isActive }">
              <a :href="href" :aria-current="(isActive || isItemActive(item)) ? 'page' : undefined"
                :data-testid="`dock-link-${item.id}`" @click="handleNavigation(item.id, item.path, $event)"
                class="flex items-center gap-3 p-3 rounded-xl text-sm font-medium transition-colors"
                :class="isActive || isItemActive(item) ? 'bg-primary/15 text-primary' : 'bg-base-200 hover:bg-base-300'">
                <component :is="item.icon" class="w-5 h-5 shrink-0" />
                <span class="break-words min-w-0">{{ navItemLabel(item) }}</span>
              </a>
            </RouterLink>
          </div>
        </Drawer>
      </nav>
    </div>
  </div>
</template>
