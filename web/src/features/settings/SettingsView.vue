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
import { fetchAuthSettings, type AuthSettings } from '../auth/authSettings'
import { t, getLocale, setLocale, type Locale } from '../../locales'
import { applyTheme, getStoredTheme, THEME_NAMES, type ThemeName } from '../../theme'
import GlobalNodeFilterSettings from './GlobalNodeFilterSettings.vue'

const authMode = ref<'open' | 'token' | 'loading' | 'error'>('loading')
const authModeError = ref('')
const storedToken = ref<string>('')
const authSettings = ref<AuthSettings | null>(null)
const adminEnabled = ref(true)
const exportEnabled = ref(true)
const settingsError = ref('')
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
    settingsError.value = ''
    const res = await fetchAuthSettings()
    if (typeof res?.admin_auth_enabled !== 'boolean' || typeof res?.export_auth_enabled !== 'boolean' ||
        typeof res?.token_configured !== 'boolean' || !['open', 'protected'].includes(res.admin_mode) ||
        !['open', 'protected'].includes(res.export_mode)) throw new Error(t('settings.invalidSettings'))
    authSettings.value = res
    adminEnabled.value = res.admin_auth_enabled
    exportEnabled.value = res.export_auth_enabled
    authMode.value = res.admin_mode === 'open' ? 'open' : 'token'
    if (!res.token_configured) {
      storedToken.value = ''
      showStoredToken.value = false
    } else if (res.admin_mode === 'open') {
      // An open admin session must not imply the local bearer is the server's secret.
      storedToken.value = ''
    }
  } catch (err: any) {
    authSettings.value = null
    authMode.value = 'error'
    settingsError.value = err?.message || t('topbar.authProbeErrorDesc')
    authModeError.value = settingsError.value
  }
}

async function updateSettings(change: { token?: string; clear_token?: boolean } = {}) {
  if (!authSettings.value || isSaving.value) return
  isSaving.value = true
  try {
    const res = await api.put<AuthSettings>('/api/v1/settings/auth', {
      admin_auth_enabled: adminEnabled.value,
      export_auth_enabled: exportEnabled.value,
      ...change,
    })
    if (!res || typeof res.token_configured !== 'boolean' || typeof res.admin_auth_enabled !== 'boolean' ||
        typeof res.export_auth_enabled !== 'boolean' || !['open', 'protected'].includes(res.admin_mode) ||
        !['open', 'protected'].includes(res.export_mode)) throw new Error(t('settings.invalidSettings'))
    // Token rotation invalidates cookies; the server never returns a plaintext secret.
    if (change.token || change.clear_token) {
      const nextToken = change.token && res.admin_mode === 'protected' ? change.token : null
      nextToken ? localStorage.setItem('csp_token', nextToken) : localStorage.removeItem('csp_token')
      api.setAuthToken(nextToken)
      api.setCsrfToken(null)
      storedToken.value = nextToken || ''
      showStoredToken.value = false
      inputToken.value = ''
    }
    authSettings.value = res
    adminEnabled.value = res.admin_auth_enabled
    exportEnabled.value = res.export_auth_enabled
    authMode.value = res.admin_mode === 'open' ? 'open' : 'token'
    toastStore.push({ message: t('settings.authSaved'), tone: 'success' })
    window.dispatchEvent(new CustomEvent('csp:auth-success'))
  } catch (err: any) {
    toastStore.push({ message: err?.message || t('settings.tokenSaveFailed'), tone: 'error' })
  } finally {
    isSaving.value = false
  }
}

function saveSwitches() {
  if (!exportEnabled.value && authSettings.value?.export_auth_enabled &&
      !window.confirm(t('settings.publicWarning'))) return
  void updateSettings()
}

const displayToken = computed(() => {
  if (!storedToken.value) {
    return authSettings.value?.token_configured ? t('settings.tokenOnServerOnly') : t('settings.tokenNotConfigured')
  }
  if (showStoredToken.value) {
    return storedToken.value
  }
  if (storedToken.value.length <= 8) {
    return '••••••••'
  }
  return `${storedToken.value.slice(0, 3)}••••••••${storedToken.value.slice(-3)}`
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

function toggleTokenVisibility() {
  showStoredToken.value = !showStoredToken.value
}

/**
 * Saves the shared token together with both independent switches.
 */
async function handleSaveToken() {
  const trimmed = inputToken.value.trim()
  if (!trimmed) {
    toastStore.push({ message: t('settings.tokenEmpty'), tone: 'warning' })
    return
  }

  await updateSettings({ token: trimmed })
}

async function handleTestConnection() {
  isTesting.value = true
  connectionStatus.value = 'idle'
  connectionMessage.value = t('settings.testingConnection')

  try {
    const res = await api.get<AuthStatus>('/api/v1/auth/status')
    connectionStatus.value = 'success'
    const modeText =
      res?.mode === 'open'
        ? t('settings.authModeOpen')
        : res?.mode === 'protected'
        ? t('settings.authModeProtected')
        : res?.mode || t('common.active')
    connectionMessage.value = t('settings.connectedMsg', { mode: modeText })
    toastStore.push({ message: t('settings.connectedToast'), tone: 'success' })
    if (res?.mode === 'open' || res?.mode === 'protected') {
      authMode.value = res.mode === 'open' ? 'open' : 'token'
    }
  } catch (err: any) {
    connectionStatus.value = 'error'
    connectionMessage.value = t('settings.connectionFailedMsg', { error: err?.message || t('common.error') })
    toastStore.push({ message: t('settings.connectionFailedToast'), tone: 'error' })
  } finally {
    isTesting.value = false
  }
}

/**
 * Clears the shared token without changing either independent switch.
 */
async function handleClearToken() {
  if (!window.confirm(t('settings.clearTokenConfirm'))) return
  isClearing.value = true
  try {
    await updateSettings({ clear_token: true })
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
        <span class="text-xs text-base-content/60 font-medium">{{ t('settings.authModeLabel') }}</span>
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
                ? t('settings.authModeOpen')
                : authMode === 'token'
                ? t('settings.authModeProtected')
                : authMode === 'loading'
                ? t('settings.authModeProbing')
                : t('settings.authModeError')
            }}
          </span>
        </div>
      </div>
    </div>

    <section class="card bg-base-200/80 border border-white/10" data-testid="auth-switches">
      <div class="card-body p-5 sm:p-6 space-y-4">
        <h3 class="font-bold">{{ t('settings.protectionTitle') }}</h3>
        <p v-if="settingsError" role="alert" class="text-error">{{ settingsError }}</p>
        <label class="flex items-center justify-between gap-4">
          <span>{{ t('settings.adminProtection') }}</span>
          <input v-model="adminEnabled" data-testid="admin-auth-switch" type="checkbox" class="toggle toggle-primary" :disabled="!authSettings || isSaving" />
        </label>
        <label class="flex items-center justify-between gap-4">
          <span>{{ t('settings.exportProtection') }}</span>
          <input v-model="exportEnabled" data-testid="export-auth-switch" type="checkbox" class="toggle toggle-primary" :disabled="!authSettings || isSaving" />
        </label>
        <p data-testid="export-mode-badge" class="text-sm">{{ t('settings.exportMode') }}: {{ authSettings ? t(authSettings.export_mode === 'open' ? 'settings.exportOpen' : 'settings.exportProtected') : '—' }}</p>
        <p v-if="authSettings?.admin_auth_enabled && !authSettings.token_configured" class="text-warning text-sm">{{ t('settings.zeroConfigWarning') }}</p>
        <p v-if="!exportEnabled" class="text-warning text-sm">{{ t('settings.publicWarning') }}</p>
        <button type="button" class="btn btn-primary btn-sm self-start" data-testid="save-auth-switches-btn" :disabled="!authSettings || isSaving" @click="saveSwitches">{{ t('settings.saveAndApply') }}</button>
      </div>
    </section>

    <!-- Shared credential; the status comes from the server, never from localStorage. -->
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
            <span v-if="authSettings?.token_configured" data-testid="token-status-badge" class="badge badge-xs badge-success gap-1">{{ t('common.active') }}</span>
            <span v-else data-testid="token-status-badge" class="badge badge-xs badge-ghost">{{ t('common.inactive') }}</span>
          </div>

          <p class="text-xs opacity-70">{{ t('settings.sharedTokenHint') }}</p>
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
                :title="showStoredToken ? t('settings.maskToken') : t('settings.revealToken')"
                @click="toggleTokenVisibility"
              >
                <EyeSlashIcon v-if="showStoredToken" class="w-4 h-4" />
                <EyeIcon v-else class="w-4 h-4" />
              </button>

              <button
                v-if="authSettings?.token_configured"
                type="button"
                data-testid="clear-token-btn"
                class="btn btn-ghost btn-xs text-error hover:bg-error/10 gap-1"
                :disabled="isClearing || isSaving"
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
              :disabled="isSaving || !authSettings"
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

    <GlobalNodeFilterSettings />

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
              {{ t('settings.langZhCN') }}
            </button>
            <button
              type="button"
              class="btn btn-sm"
              :class="currentLocale === 'en-US' ? 'btn-primary' : 'btn-outline'"
              @click="handleLocaleChange('en-US')"
            >
              {{ t('settings.langEnUS') }}
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
              class="btn btn-xs font-mono"
              :class="currentTheme === theme ? 'btn-secondary' : 'btn-outline'"
              @click="handleThemeChange(theme)"
            >
              {{ themeLabel(theme) }}
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
            <span class="text-base-content/60 font-medium">{{ t('settings.runtimeControlPlane') }}</span>
            <p class="font-mono text-sm font-semibold text-primary">{{ t('settings.runtimeBinary') }}</p>
            <p class="text-[11px] text-base-content/50">{{ t('settings.runtimeControlPlaneDesc') }}</p>
          </div>

          <div class="p-4 rounded-xl bg-base-100/70 border border-white/5 space-y-1">
            <span class="text-base-content/60 font-medium">{{ t('settings.runtimeApiClient') }}</span>
            <p class="font-mono text-sm font-semibold text-success">{{ t('settings.runtimeClient') }}</p>
            <p class="text-[11px] text-base-content/50">{{ t('settings.runtimeApiClientDesc') }}</p>
          </div>

          <div class="p-4 rounded-xl bg-base-100/70 border border-white/5 space-y-1">
            <span class="text-base-content/60 font-medium">{{ t('settings.runtimeAssetPackaging') }}</span>
            <p class="font-mono text-sm font-semibold text-secondary">{{ t('settings.runtimeAssets') }}</p>
            <p class="text-[11px] text-base-content/50">{{ t('settings.runtimeAssetPackagingDesc') }}</p>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
