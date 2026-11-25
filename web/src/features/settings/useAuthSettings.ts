import { computed, ref, type ComputedRef, type Ref } from 'vue'
import { api } from '../../api/client'
import { displayText } from '../../utils/displayNames'
import { toastStore } from '../../ui/toast'
import { t } from '../../locales'
import type { AuthStatus } from '../auth/useAuth'
import { fetchAuthSettings, type AuthSettings } from '../auth/authSettings'
import { settingsDiscardMessage } from './useSettingsDraft'
import { uiText } from '../../locales/redesign'

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
  isBusy: ComputedRef<boolean>
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

export function useAuthSettings(beforeReconnect: (dirty: ComputedRef<boolean>) => boolean = () => true): UseAuthSettingsReturn {
  let authProbeSession = 0
  let updating = false

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
  const isBusy = computed(() => isSavingSwitches.value || isSavingToken.value || isClearingToken.value || isTesting.value)

  function validateSettings(value: AuthSettings) {
    if (!value || typeof value.admin_auth_enabled !== 'boolean' || typeof value.export_auth_enabled !== 'boolean' ||
      typeof value.token_configured !== 'boolean' || !['open', 'protected'].includes(value.admin_mode) ||
      !['open', 'protected'].includes(value.export_mode)) throw new Error(t('settings.invalidSettings'))
  }

  function syncStoredToken() {
    storedToken.value = api.getAuthToken() || ''
  }

  function toggleTokenVisibility() {
    showStoredToken.value = !showStoredToken.value
  }

  async function probeAuthMode() {
    if (isBusy.value || (isDirty.value && !window.confirm(settingsDiscardMessage()))) return
    const currentSession = ++authProbeSession
    authMode.value = 'loading'
    authModeError.value = ''
    settingsError.value = ''
    try {
      const res = await fetchAuthSettings()
      if (currentSession !== authProbeSession) return // Discard late response
      validateSettings(res)
      authSettings.value = res
      adminEnabled.value = res.admin_auth_enabled
      exportEnabled.value = res.export_auth_enabled
      inputToken.value = ''
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
      authMode.value = 'error'
      settingsError.value = displayText(err?.message || t('topbar.authProbeErrorDesc'))
      authModeError.value = displayText(settingsError.value)
    }
  }

  async function updateSettings(change: { token?: string; clear_token?: boolean } = {}) {
    if (!authSettings.value || updating) return
    updating = true
    settingsError.value = ''
    const currentSession = ++authProbeSession
    try {
      const res = await api.put<AuthSettings>('/api/v1/settings/auth', {
        admin_auth_enabled: adminEnabled.value,
        export_auth_enabled: exportEnabled.value,
        ...change,
      })
      if (currentSession !== authProbeSession) return // Discard late response
      try {
        validateSettings(res)
      } catch {
        authMode.value = 'error'
        throw new Error(uiText('请求已完成，但返回的认证配置无法确认。草稿已保留，请重新读取核对。', 'The request completed, but authentication settings could not be verified. Draft retained; reload to verify.'))
      }
      // Token rotation invalidates cookies; the server never returns a plaintext secret.
      if (change.token || change.clear_token) {
        const nextToken = change.token && res.admin_mode === 'protected' ? change.token : null
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
      settingsError.value = displayText(err?.message || t('settings.tokenSaveFailed'))
      toastStore.push({ message: settingsError.value, tone: 'error' })
      throw err
    } finally {
      updating = false
    }
  }

  function confirmExportVisibility() {
    return exportEnabled.value || !authSettings.value?.export_auth_enabled || window.confirm(t('settings.publicWarning'))
  }

  async function saveSwitches() {
    if (!authSettings.value || isBusy.value || authMode.value === 'loading' || authMode.value === 'error') return
    if (inputToken.value.trim()) {
      settingsError.value = uiText(
        '令牌输入仍有草稿，请使用“保存令牌”一并保存安全开关，或清空输入后只保存开关。',
        'A token draft is pending. Save the token to apply it together with these switches, or clear the input to save only the switches.'
      )
      return
    }
    if (!confirmExportVisibility() || !beforeReconnect(isDirty)) return
    isSavingSwitches.value = true
    try {
      await updateSettings()
    } catch {
      // The persistent error remains visible, and the draft stays dirty.
    } finally {
      isSavingSwitches.value = false
    }
  }

  async function saveToken() {
    if (!authSettings.value || isBusy.value || authMode.value === 'loading' || authMode.value === 'error') return
    const trimmed = inputToken.value.trim()
    if (!trimmed) {
      toastStore.push({ message: t('settings.tokenEmpty'), tone: 'warning' })
      return
    }
    if (!confirmExportVisibility() || !beforeReconnect(isDirty)) return
    isSavingToken.value = true
    try {
      await updateSettings({ token: trimmed })
    } catch {
      // Keep the replacement secret only in this mounted draft after rejection.
    } finally {
      isSavingToken.value = false
    }
  }

  async function clearToken() {
    if (!authSettings.value || isBusy.value || authMode.value === 'loading' || authMode.value === 'error') return
    if (!window.confirm(t('settings.clearTokenConfirm')) || !confirmExportVisibility() || !beforeReconnect(isDirty)) return
    isClearingToken.value = true
    try {
      await updateSettings({ clear_token: true })
    } catch {
      // Do not clear a local bearer unless the server acknowledged the change.
    } finally {
      isClearingToken.value = false
    }
  }

  async function testConnection() {
    if (isBusy.value) return
    const currentSession = ++authProbeSession
    isTesting.value = true
    connectionStatus.value = 'idle'
    connectionMessage.value = t('settings.testingConnection')

    try {
      const res = await api.get<AuthStatus>('/api/v1/auth/status')
      if (currentSession !== authProbeSession) return
      if (!res || !['open', 'protected'].includes(res.mode)) throw new Error(t('settings.invalidSettings'))
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
        error: displayText(err?.message || t('common.error')),
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
    isBusy,
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
