import { computed, ref } from 'vue'
import { api, ApiError } from '../../api/client'
import { clearPublicationCapabilities, publicationSession } from '../publications/usePublications'
import { displayText } from '../../utils/displayNames'
import { uiText } from '../../locales'

export type AuthState = 'probing' | 'open' | 'unauthenticated' | 'authenticated' | 'error'

export interface AuthStatus {
  mode: 'open' | 'protected'
  authenticated: boolean
  subject: string
}

interface LoginResult extends AuthStatus {
  csrf_token?: string
}

type AuthFailure = 'empty' | 'invalid' | 'probe' | 'login' | null
const state = ref<AuthState>('probing')
const status = ref<AuthStatus | null>(null)
const failure = ref<AuthFailure>(null)
const failureDetail = ref('')
const errorMessage = computed({
  get: () => {
    if (failure.value === 'empty') return uiText('令牌不能为空，请输入管理员令牌', 'Enter your administrator token')
    if (failure.value === 'invalid') return uiText('令牌无效或已过期，请检查后重试', 'Your token is invalid or has expired. Check it and try again.')
    if (failure.value === 'probe') return failureDetail.value || uiText('无法连接服务，请检查网络后重试', 'Cannot reach the service. Check your connection and try again.')
    if (failure.value === 'login') return failureDetail.value || uiText('登录失败，请稍后重试', 'Sign-in failed. Please try again.')
    return ''
  },
  set: (value: string) => {
    failure.value = value ? 'login' : null
    failureDetail.value = value ? displayText(value) : ''
  },
})

// Every lifecycle operation replaces earlier results, including across mounted consumers.
let authSeq = 0
function beginAuthOperation(): number {
  api.invalidateAuthRequests()
  failure.value = null
  failureDetail.value = ''
  return ++authSeq
}

function clearPublicationState(): void {
  try {
    clearPublicationCapabilities()
  } catch {
    // A denied sessionStorage getter can fail before publication's in-memory invalidation.
    publicationSession.value++
  }
}

function clearCredentials(): void {
  clearPublicationState()
  api.setAuthToken(null)
  api.setCsrfToken(null)
}

api.setOnUnauthorized(() => {
  ++authSeq
  api.invalidateAuthRequests()
  clearPublicationState()
  api.setCsrfToken(null)
  if (state.value !== 'open') {
    state.value = 'unauthenticated'
    status.value = status.value?.mode === 'protected'
      ? { ...status.value, authenticated: false, subject: '' }
      : null
    failure.value = 'invalid'
  }
})

export function useAuth() {
  const probe = async () => {
    const seq = beginAuthOperation()
    state.value = 'probing'
    try {
      const res = await api.get<AuthStatus>('/api/v1/auth/status')
      if (seq !== authSeq) return
      const wasProtected = status.value?.mode === 'protected'
      const hadStoredAuth = Boolean(api.getAuthToken())
      status.value = res
      if (res.mode === 'open') {
        // Public-mode reloads retain publication links unless replacing a protected session.
        if (wasProtected || hadStoredAuth) clearPublicationState()
        api.setAuthToken(null)
        api.setCsrfToken(null)
        state.value = 'open'
      } else if (res.authenticated) {
        state.value = 'authenticated'
      } else {
        clearPublicationState()
        api.setCsrfToken(null)
        state.value = 'unauthenticated'
      }
    } catch (err) {
      if (seq !== authSeq) return
      state.value = 'error'
      failure.value = 'probe'
      failureDetail.value = err instanceof Error ? displayText(err.message) : ''
    }
  }

  const login = async (inputToken: string): Promise<boolean> => {
    const trimmed = inputToken.trim()
    if (!trimmed) {
      failure.value = 'empty'
      return false
    }
    const seq = beginAuthOperation()
    try {
      const res = await api.post<LoginResult>('/api/v1/auth/login', { token: trimmed })
      if (seq !== authSeq) return false
      if (!res?.authenticated) {
        failure.value = 'invalid'
        return false
      }
      clearPublicationState()
      api.setAuthToken(res.mode === 'open' ? null : trimmed)
      api.setCsrfToken(res.mode === 'open' ? null : res.csrf_token || null)
      status.value = { mode: res.mode, authenticated: true, subject: res.subject || 'admin' }
      state.value = res.mode === 'open' ? 'open' : 'authenticated'
      return true
    } catch (err) {
      if (seq !== authSeq) return false
      if (err instanceof ApiError && err.status === 401) {
        failure.value = 'invalid'
      } else {
        failure.value = 'login'
        failureDetail.value = err instanceof Error ? displayText(err.message) : ''
      }
      return false
    }
  }

  const logout = async () => {
    beginAuthOperation()
    // Dispatch with the current credentials, then lock the local UI immediately.
    const request = api.post('/api/v1/auth/logout')
    clearCredentials()
    state.value = state.value === 'open' ? 'open' : 'unauthenticated'
    if (status.value) status.value = { ...status.value, authenticated: false, subject: '' }
    try {
      await request
    } catch {
      // The local session stays cleared even if the server cannot be reached.
    }
  }

  const clearStoredToken = async () => {
    beginAuthOperation()
    clearCredentials()
    return probe()
  }

  return { state, status, errorMessage, probe, login, logout, clearStoredToken }
}
