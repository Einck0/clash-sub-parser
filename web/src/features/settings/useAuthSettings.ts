import { computed, ref, type ComputedRef, type Ref } from 'vue'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { t } from '../../locales'
import type { AuthStatus } from '../auth/useAuth'
import { fetchAuthSettings, type AuthSettings } from '../auth/authSettings'

export type AuthMode = 'open' | 'token' | 'loading' | 'error'
export type ConnectionStatus = 'idle' | 'success' | 'error'

export interface UseAuthSettingsReturn {
  authSettings: Ref<AuthSettings | null>
  authMode: Ref<AuthMode>
  authModeError: Ref<string>
  settingsError: Ref<string>
  storedToken: Ref<string>
  inputToken: Ref<string>
  showStoredToken: Ref<boolean>
  adminEnabled: Ref<boolean>
  exportEnabled: Ref<boolean>
  isSavingSwitches: Ref<boolean>
  isSavingToken: Ref<boolean>
  isClearingToken: Ref<boolean>
  isTesting: Ref<boolean>
  connectionStatus: Ref<ConnectionStatus>
  connectionMessage: Ref<string>
  isDirty: ComputedRef<boolean>
  displayToken: ComputedRef<string>
  syncStoredToken: () => void
  toggleTokenVisibility: () => void
  probeAuthMode: () => Promise<void>
  updateSettings: (change?: { token?: string; clear_token?: boolean }) => Promise<void>
  saveSwitches: () => Promise<void>
  saveToken: () => Promise<void>
  clearToken: () => Promise<void>
  testConnection: () => Promise<void>
}

export function useAuthSettings(): UseAuthSettingsReturn {
  let authProbeSession = 0

  const authMode = ref<AuthMode>('loading')
  const authModeError = ref('')
  const settingsError = ref('')
  const storedToken = ref('')
  const inputToken = ref('')
  const showStoredToken = ref(false)
  const authSettings = ref<AuthSettings | null>(null)
  const adminEnabled = ref(true)
  const exportEnabled = ref(true)
  const isSavingSwitches = ref(false)
  const isSavingToken = ref(false)
  const isClearingToken = ref(false)
  const isTesting = ref(false)
  const connectionStatus = ref<ConnectionStatus>('idle')
  const connectionMessage = ref('')

  function syncStoredToken() {
    if (typeof window !== 'undefined' && typeof window.localStorage !== 'undefined') {
      storedToken.value = localStorage.getItem('csp_token') || ''
    }
  }

  function toggleTokenVisibility() {
    showStoredToken.value = !showStoredToken.value
  }

  async function probeAuthMode() {
    const currentSession = ++authProbeSession
    authMode.value = 'loading'
    authModeError.value = ''
    settingsError.value = ''
    try {
      const res = await fetchAuthSettings()
      if (currentSession !== authProbeSession) return // Discard late response
      if (
        !res ||
        typeof res.admin_auth_enabled !== 'boolean' ||
        typeof res.export_auth_enabled !== 'boolean' ||
        typeof res.token_configured !== 'boolean' ||
        !['open', 'protected'].includes(res.admin_mode) ||
        !['open', 'protected'].includes(res.export_mode)
      ) {
        throw new Error(t('settings.invalidSettings'))
      }
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
      if (currentSession !== authProbeSession) return
      authSettings.value = null
      authMode.value = 'error'
      settingsError.value = err?.message || t('topbar.authProbeErrorDesc')
      authModeError.value = settingsError.value
    }
  }

  async function updateSettings(change: { token?: string; clear_token?: boolean } = {}) {
    if (!authSettings.value) return
    const currentSession = ++authProbeSession
    try {
      const res = await api.put<AuthSettings>('/api/v1/settings/auth', {
        admin_auth_enabled: adminEnabled.value,
        export_auth_enabled: exportEnabled.value,
        ...change,
      })
      if (currentSession !== authProbeSession) return // Discard late response
      if (
        !res ||
        typeof res.token_configured !== 'boolean' ||
        typeof res.admin_auth_enabled !== 'boolean' ||
        typeof res.export_auth_enabled !== 'boolean' ||
        !['open', 'protected'].includes(res.admin_mode) ||
        !['open', 'protected'].includes(res.export_mode)
      ) {
        throw new Error(t('settings.invalidSettings'))
      }
      // Token rotation invalidates cookies; the server never returns a plaintext secret.
      if (change.token || change.clear_token) {
        const nextToken = change.token && res.admin_mode === 'protected' ? change.token : null
        if (nextToken) {
          localStorage.setItem('csp_token', nextToken)
        } else {
          localStorage.removeItem('csp_token')
        }
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
      if (currentSession !== authProbeSession) return
      toastStore.push({ message: err?.message || t('settings.tokenSaveFailed'), tone: 'error' })
      throw err
    }
  }

  async function saveSwitches() {
    if (isSavingSwitches.value) return
    if (
      !exportEnabled.value &&
      authSettings.value?.export_auth_enabled &&
      !window.confirm(t('settings.publicWarning'))
    ) {
      return
    }
    isSavingSwitches.value = true
    try {
      await updateSettings()
    } finally {
      isSavingSwitches.value = false
    }
  }

  async function saveToken() {
    if (isSavingToken.value) return
    const trimmed = inputToken.value.trim()
    if (!trimmed) {
      toastStore.push({ message: t('settings.tokenEmpty'), tone: 'warning' })
      return
    }
    isSavingToken.value = true
    try {
      await updateSettings({ token: trimmed })
    } finally {
      isSavingToken.value = false
    }
  }

  async function clearToken() {
    if (isClearingToken.value) return
    if (!window.confirm(t('settings.clearTokenConfirm'))) return
    isClearingToken.value = true
    try {
      await updateSettings({ clear_token: true })
    } finally {
      isClearingToken.value = false
    }
  }

  async function testConnection() {
    const currentSession = ++authProbeSession
    isTesting.value = true
    connectionStatus.value = 'idle'
    connectionMessage.value = t('settings.testingConnection')

    try {
      const res = await api.get<AuthStatus>('/api/v1/auth/status')
      if (currentSession !== authProbeSession) return
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
      if (currentSession !== authProbeSession) return
      connectionStatus.value = 'error'
      connectionMessage.value = t('settings.connectionFailedMsg', {
        error: err?.message || t('common.error'),
      })
      toastStore.push({ message: t('settings.connectionFailedToast'), tone: 'error' })
    } finally {
      if (currentSession === authProbeSession) {
        isTesting.value = false
      }
    }
  }

  const isDirty = computed(() => {
    if (inputToken.value.trim().length > 0) return true
    if (authSettings.value) {
      if (adminEnabled.value !== authSettings.value.admin_auth_enabled) return true
      if (exportEnabled.value !== authSettings.value.export_auth_enabled) return true
    }
    return false
  })

  const displayToken = computed(() => {
    if (!storedToken.value) {
      return authSettings.value?.token_configured
        ? t('settings.tokenOnServerOnly')
        : t('settings.tokenNotConfigured')
    }
    if (showStoredToken.value) {
      return storedToken.value
    }
    if (storedToken.value.length <= 8) {
      return '••••••••'
    }
    return `${storedToken.value.slice(0, 3)}••••••••${storedToken.value.slice(-3)}`
  })

  return {
    authSettings,
    authMode,
    authModeError,
    settingsError,
    storedToken,
    inputToken,
    showStoredToken,
    adminEnabled,
    exportEnabled,
    isSavingSwitches,
    isSavingToken,
    isClearingToken,
    isTesting,
    connectionStatus,
    connectionMessage,
    isDirty,
    displayToken,
    syncStoredToken,
    toggleTokenVisibility,
    probeAuthMode,
    updateSettings,
    saveSwitches,
    saveToken,
    clearToken,
    testConnection,
  }
}
