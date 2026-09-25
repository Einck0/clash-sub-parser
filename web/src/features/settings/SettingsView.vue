<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  ShieldCheckIcon,
  KeyIcon,
  EyeIcon,
  EyeSlashIcon,
  TrashIcon,
  ArrowPathIcon,
  CommandLineIcon,
  LanguageIcon,
  PaintBrushIcon,
} from '@heroicons/vue/24/outline'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'
import type { AuthStatus } from '../auth/useAuth'
import { t, getLocale, setLocale, type Locale } from '../../locales'
import { applyTheme, getStoredTheme, THEME_NAMES, type ThemeName } from '../../theme'

const authMode = ref<'open' | 'token' | 'loading' | 'error'>('loading')
const authModeError = ref('')
const storedToken = ref<string>('')
const showStoredToken = ref(false)
const inputToken = ref('')
const isTesting = ref(false)
const isSaving = ref(false)
const isClearing = ref(false)
const connectionStatus = ref<'idle' | 'success' | 'error'>('idle')
const connectionMessage = ref('')
const currentLocale = ref<Locale>(getLocale())
const currentTheme = ref<ThemeName>(getStoredTheme())

function syncStoredToken() {
  if (typeof window !== 'undefined' && typeof window.localStorage !== 'undefined') {
    storedToken.value = localStorage.getItem('csp_token') || ''
  }
}

async function probeAuthMode() {
  authMode.value = 'loading'
  authModeError.value = ''
  try {
    const res = await api.get<AuthStatus>('/api/v1/auth/status')
    if (res && (res.mode === 'open' || res.mode === 'protected')) {
      authMode.value = res.mode === 'open' ? 'open' : 'token'
    } else {
      authMode.value = 'open'
    }
  } catch (err: any) {
    authMode.value = 'error'
    authModeError.value = err?.message || 'Failed to probe auth status'
  }
}

const displayToken = computed(() => {
  if (!storedToken.value) {
    return 'None (未配置)'
  }
  if (showStoredToken.value) {
    return storedToken.value
  }
  if (storedToken.value.length <= 8) {
    return '••••••••'
  }
  return `${storedToken.value.slice(0, 3)}••••••••${storedToken.value.slice(-3)}`
})

function toggleTokenVisibility() {
  showStoredToken.value = !showStoredToken.value
}

/**
 * Saves real server Admin Token via POST /api/v1/settings/admin-token.
 * Never a localStorage-only setting. If the backend fails or has not applied the endpoint,
 * it catches and gracefully displays the API error without faking success.
 */
async function handleSaveToken() {
  const trimmed = inputToken.value.trim()
  if (!trimmed) {
    toastStore.push({ message: t('settings.tokenEmpty'), tone: 'warning' })
    return
  }

  isSaving.value = true
  try {
    await api.post('/api/v1/settings/admin-token', { token: trimmed })

    // Successfully applied to backend: update local bearer state
    if (typeof window !== 'undefined') {
      localStorage.setItem('csp_token', trimmed)
    }
    if (api && typeof (api as any).setAuthToken === 'function') {
      (api as any).setAuthToken(trimmed)
    }

    storedToken.value = trimmed
    inputToken.value = ''
    toastStore.push({ message: t('settings.tokenSaved'), tone: 'success' })

    if (typeof window !== 'undefined') {
      window.dispatchEvent(new CustomEvent('csp:auth-success', { detail: { token: trimmed } }))
    }
    await probeAuthMode()
  } catch (err: any) {
    const msg = err?.message || t('settings.tokenSaveFailed')
    toastStore.push({ message: msg, tone: 'error' })
  } finally {
    isSaving.value = false
  }
}

async function handleTestConnection() {
  isTesting.value = true
  connectionStatus.value = 'idle'
  connectionMessage.value = 'Testing connection...'

  try {
    const res = await api.get<AuthStatus>('/api/v1/auth/status')
    connectionStatus.value = 'success'
    connectionMessage.value = `Connected: backend reachable (Mode: ${res?.mode || 'active'})`
    toastStore.push({ message: 'Backend connected successfully', tone: 'success' })
    if (res?.mode === 'open' || res?.mode === 'protected') {
      authMode.value = res.mode === 'open' ? 'open' : 'token'
    }
  } catch (err: any) {
    connectionStatus.value = 'error'
    connectionMessage.value = `Connection failed: ${err?.message || 'Reachable error'}`
    toastStore.push({ message: 'Backend connection failed', tone: 'error' })
  } finally {
    isTesting.value = false
  }
}

/**
 * Clears real server Admin Token via POST /api/v1/settings/admin-token with empty string.
 */
async function handleClearToken() {
  isClearing.value = true
  try {
    await api.post('/api/v1/settings/admin-token', { token: '' })

    if (typeof window !== 'undefined') {
      localStorage.removeItem('csp_token')
    }
    if (api && typeof (api as any).setAuthToken === 'function') {
      (api as any).setAuthToken(null)
    }

    storedToken.value = ''
    inputToken.value = ''
    connectionStatus.value = 'idle'
    connectionMessage.value = ''
    toastStore.push({ message: t('settings.tokenCleared'), tone: 'info' })
    await probeAuthMode()
  } catch (err: any) {
    const msg = err?.message || t('settings.tokenClearFailed')
    toastStore.push({ message: msg, tone: 'error' })
  } finally {
    isClearing.value = false
  }
}

function handleLocaleChange(loc: Locale) {
  currentLocale.value = loc
  setLocale(loc)
}

function handleThemeChange(theme: ThemeName) {
  currentTheme.value = theme
  applyTheme(theme)
}

onMounted(() => {
  syncStoredToken()
  probeAuthMode()
  if (typeof window !== 'undefined') {
    window.addEventListener('csp:auth-success', syncStoredToken)
  }
})
</script>

<template>
  <div class="space-y-6" data-testid="settings-view">
    <!-- View Header -->
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-primary">{{ t('settings.tag') }}</p>
        <h2 class="mt-1 text-2xl font-bold tracking-tight">{{ t('settings.title') }}</h2>
        <p class="mt-1 text-sm text-base-content/70">
          {{ t('settings.subtitle') }}
        </p>
      </div>

      <!-- Auth Mode Status Pill -->
      <div class="flex items-center gap-2">
        <span class="text-xs text-base-content/60 font-medium">Auth Mode:</span>
        <div
          data-testid="auth-mode-badge"
          class="badge gap-1.5 py-3 px-3 text-xs font-semibold uppercase tracking-wider"
          :class="{
            'badge-warning badge-outline': authMode === 'open',
            'badge-success': authMode === 'token',
            'badge-ghost animate-pulse': authMode === 'loading',
            'badge-error': authMode === 'error',
          }"
        >
          <span
            class="w-2 h-2 rounded-full"
            :class="{
              'bg-warning': authMode === 'open',
              'bg-success-content': authMode === 'token',
              'bg-base-content/40': authMode === 'loading',
              'bg-error': authMode === 'error',
            }"
          />
          <span>
            {{
              authMode === 'open'
                ? 'Open Mode'
                : authMode === 'token'
                ? 'Protected'
                : authMode === 'loading'
                ? 'Probing...'
                : 'Error'
            }}
          </span>
        </div>
      </div>
    </div>

    <!-- Credentials Management Card (Real Server POST /api/v1/settings/admin-token) -->
    <section class="card bg-base-200/80 backdrop-blur-xl shadow-sm border border-white/10">
      <div class="card-body p-5 sm:p-6 space-y-5">
        <div class="flex items-start justify-between gap-3">
          <div class="flex items-center gap-3">
            <div class="p-2.5 rounded-xl bg-primary/10 text-primary border border-primary/20 shrink-0">
              <ShieldCheckIcon class="w-6 h-6" />
            </div>
            <div>
              <h3 class="text-base sm:text-lg font-bold">{{ t('settings.authCardTitle') }}</h3>
              <p class="text-xs text-base-content/60 mt-0.5">
                {{ t('settings.authCardDesc') }}
              </p>
            </div>
          </div>
        </div>

        <!-- Current Active Token Display -->
        <div class="p-4 rounded-xl bg-base-300/50 border border-white/5 space-y-2">
          <div class="flex items-center justify-between text-xs font-semibold text-base-content/70">
            <span>{{ t('settings.serverTokenLabel') }}</span>
            <span v-if="storedToken" data-testid="token-status-badge" class="badge badge-xs badge-success gap-1">{{ t('common.active') }}</span>
            <span v-else data-testid="token-status-badge" class="badge badge-xs badge-ghost">{{ t('common.inactive') }}</span>
          </div>

          <div class="flex items-center justify-between gap-2">
            <span
              data-testid="stored-token-display"
              class="font-mono text-sm font-semibold tracking-wider text-primary truncate"
            >
              {{ displayToken }}
            </span>

            <div class="flex items-center gap-1">
              <button
                v-if="storedToken"
                type="button"
                data-testid="toggle-token-visibility-btn"
                class="btn btn-ghost btn-xs btn-square text-base-content/60 hover:text-base-content"
                :title="showStoredToken ? 'Mask token' : 'Reveal token'"
                @click="toggleTokenVisibility"
              >
                <EyeSlashIcon v-if="showStoredToken" class="w-4 h-4" />
                <EyeIcon v-else class="w-4 h-4" />
              </button>

              <button
                v-if="storedToken"
                type="button"
                data-testid="clear-token-btn"
                class="btn btn-ghost btn-xs text-error hover:bg-error/10 gap-1"
                :disabled="isClearing"
                @click="handleClearToken"
              >
                <TrashIcon class="w-3.5 h-3.5" />
                <span>{{ t('settings.clearToken') }}</span>
              </button>
            </div>
          </div>
        </div>

        <!-- Update Admin Token Input & Server Persistence -->
        <div class="space-y-3">
          <label class="block text-xs font-semibold text-base-content/80">
            {{ t('settings.serverTokenLabel') }}
          </label>
          <div class="flex flex-col sm:flex-row gap-2.5">
            <div class="relative flex-1">
              <KeyIcon class="w-5 h-5 absolute left-3 top-1/2 -translate-y-1/2 text-base-content/40" />
              <input
                v-model="inputToken"
                type="password"
                data-testid="settings-token-input"
                :placeholder="t('settings.serverTokenPlaceholder')"
                class="input input-bordered w-full pl-10 text-xs sm:text-sm font-mono bg-base-100/80"
                @keydown.enter="handleSaveToken"
              />
            </div>
            <button
              type="button"
              data-testid="save-token-btn"
              class="btn btn-primary btn-sm sm:btn-md text-xs font-semibold shadow-sm"
              :disabled="isSaving"
              @click="handleSaveToken"
            >
              <span v-if="isSaving" class="loading loading-spinner loading-xs" />
              <span>{{ t('settings.saveAndApply') }}</span>
            </button>
            <button
              type="button"
              data-testid="test-connection-btn"
              class="btn btn-outline btn-sm sm:btn-md text-xs font-semibold gap-1.5"
              :disabled="isTesting"
              @click="handleTestConnection"
            >
              <ArrowPathIcon class="w-4 h-4" :class="{ 'animate-spin': isTesting }" />
              <span>{{ t('settings.testConnection') }}</span>
            </button>
          </div>

          <!-- Connection test feedback -->
          <div
            v-if="connectionMessage"
            data-testid="connection-result"
            class="text-xs p-2.5 rounded-lg font-mono border"
            :class="{
              'bg-success/10 border-success/30 text-success': connectionStatus === 'success',
              'bg-error/10 border-error/30 text-error': connectionStatus === 'error',
              'bg-base-300/50 border-base-300 text-base-content/70': connectionStatus === 'idle',
            }"
          >
            {{ connectionMessage }}
          </div>
        </div>
      </div>
    </section>

    <!-- Language & Visual System Settings -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <!-- Language Selection -->
      <section class="card bg-base-200/80 backdrop-blur-xl shadow-sm border border-white/10">
        <div class="card-body p-5 space-y-4">
          <div class="flex items-center gap-2.5">
            <LanguageIcon class="w-5 h-5 text-primary" />
            <div>
              <h3 class="text-sm sm:text-base font-bold">{{ t('settings.langCardTitle') }}</h3>
              <p class="text-xs text-base-content/60">{{ t('settings.langCardDesc') }}</p>
            </div>
          </div>
          <div class="grid grid-cols-2 gap-2">
            <button
              type="button"
              class="btn btn-sm"
              :class="currentLocale === 'zh-CN' ? 'btn-primary' : 'btn-outline'"
              @click="handleLocaleChange('zh-CN')"
            >
              简体中文 (zh-CN)
            </button>
            <button
              type="button"
              class="btn btn-sm"
              :class="currentLocale === 'en-US' ? 'btn-primary' : 'btn-outline'"
              @click="handleLocaleChange('en-US')"
            >
              English (en-US)
            </button>
          </div>
        </div>
      </section>

      <!-- Theme Selection -->
      <section class="card bg-base-200/80 backdrop-blur-xl shadow-sm border border-white/10">
        <div class="card-body p-5 space-y-4">
          <div class="flex items-center gap-2.5">
            <PaintBrushIcon class="w-5 h-5 text-secondary" />
            <div>
              <h3 class="text-sm sm:text-base font-bold">{{ t('settings.themeCardTitle') }}</h3>
              <p class="text-xs text-base-content/60">{{ t('settings.themeCardDesc') }}</p>
            </div>
          </div>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="theme in THEME_NAMES"
              :key="theme"
              type="button"
              class="btn btn-xs uppercase font-mono"
              :class="currentTheme === theme ? 'btn-secondary' : 'btn-outline'"
              @click="handleThemeChange(theme)"
            >
              {{ theme }}
            </button>
          </div>
        </div>
      </section>
    </div>

    <!-- Runtime Environment Overview Card -->
    <section class="card bg-base-200/80 backdrop-blur-xl shadow-sm border border-white/10" data-testid="runtime-info">
      <div class="card-body p-5 sm:p-6 space-y-4">
        <div class="flex items-center gap-2">
          <CommandLineIcon class="w-5 h-5 text-primary" />
          <h3 class="text-base font-bold">{{ t('settings.runtimeTitle') }}</h3>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 text-xs">
          <div class="p-4 rounded-xl bg-base-100/70 border border-white/5 space-y-1">
            <span class="text-base-content/60 font-medium">Control Plane</span>
            <p class="font-mono text-sm font-semibold text-primary">Single-Binary Go 1.22+</p>
            <p class="text-[11px] text-base-content/50">自包含编译与无依赖单二进制部署</p>
          </div>

          <div class="p-4 rounded-xl bg-base-100/70 border border-white/5 space-y-1">
            <span class="text-base-content/60 font-medium">API Client</span>
            <p class="font-mono text-sm font-semibold text-success">Unified {data} & X-Request-ID</p>
            <p class="text-[11px] text-base-content/50">全链路追踪 ID 与主动握手状态机</p>
          </div>

          <div class="p-4 rounded-xl bg-base-100/70 border border-white/5 space-y-1">
            <span class="text-base-content/60 font-medium">Asset Packaging</span>
            <p class="font-mono text-sm font-semibold text-secondary">internal/webassets (embed.FS)</p>
            <p class="text-[11px] text-base-content/50">前端构建产物零运行时外部文件依赖</p>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
