import { ref } from 'vue'
import { api, ApiError } from '../../api/client'
import { clearPublicationCapabilities } from '../publications/usePublications'

export type AuthState = 'probing' | 'open' | 'unauthenticated' | 'authenticated' | 'error'

export interface AuthStatus {
  mode: 'open' | 'protected'
  authenticated: boolean
  subject: string
}

const state = ref<AuthState>('probing')
const status = ref<AuthStatus | null>(null)
const errorMessage = ref<string>('')

// 注册全局失效通知
api.setOnUnauthorized(() => {
  clearPublicationCapabilities()
  api.setCsrfToken(null)
  if (state.value === 'authenticated') {
    state.value = 'unauthenticated'
  }
})

export function useAuth() {
  const probe = async () => {
    state.value = 'probing'
    errorMessage.value = ''
    try {
      const res = await api.get<AuthStatus>('/api/v1/auth/status')
      const wasProtected = status.value?.mode === 'protected'
      const hadStoredAuth = Boolean(typeof window !== 'undefined' && window.localStorage?.getItem('csp_token'))
      status.value = res
      if (res.mode === 'open') {
        // Keep public-mode publication links on reload; clear only a replaced protected/dirty session.
        if (wasProtected || hadStoredAuth) clearPublicationCapabilities()
        api.setAuthToken(null)
        api.setCsrfToken(null)
        state.value = 'open'
      } else {
        // Protected Mode
        if (res.authenticated) {
          state.value = 'authenticated'
        } else {
          clearPublicationCapabilities()
          state.value = 'unauthenticated'
        }
      }
    } catch (err: any) {
      state.value = 'error'
      errorMessage.value = err?.message || '无法连接控制面认证服务'
    }
  }

  const login = async (inputToken: string): Promise<boolean> => {
    errorMessage.value = ''
    const trimmed = inputToken.trim()
    if (!trimmed) {
      errorMessage.value = '令牌不能为空'
      return false
    }

    try {
      const res = await api.post<{ mode: string; authenticated: boolean; token?: string; csrf_token?: string }>('/api/v1/auth/login', {
        token: trimmed,
      })
      if (res && res.authenticated) {
        clearPublicationCapabilities()
        api.setAuthToken(trimmed)
        if (res.csrf_token) {
          api.setCsrfToken(res.csrf_token)
        }
        state.value = 'authenticated'
        status.value = {
          mode: res.mode as 'open' | 'protected',
          authenticated: true,
          subject: 'admin',
        }
        return true
      }
      errorMessage.value = '认证失败'
      return false
    } catch (err: any) {
      if (err instanceof ApiError && err.status === 401) {
        errorMessage.value = '令牌无效或已过期，请检查后重试'
      } else {
        errorMessage.value = err?.message || '登录请求失败'
      }
      return false
    }
  }

  const logout = async () => {
    clearPublicationCapabilities()
    try {
      await api.post('/api/v1/auth/logout')
    } catch {}
    api.setAuthToken(null)
    api.setCsrfToken(null)
    state.value = 'unauthenticated'
    if (status.value) {
      status.value.authenticated = false
    }
  }

  const clearStoredToken = () => {
    clearPublicationCapabilities()
    api.setAuthToken(null)
    api.setCsrfToken(null)
    probe()
  }

  return {
    state,
    status,
    errorMessage,
    probe,
    login,
    logout,
    clearStoredToken,
  }
}
