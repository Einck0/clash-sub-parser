import { ref } from 'vue'
import { api, ApiError } from '../../api/client'

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
      status.value = res
      if (res.mode === 'open') {
        // Open Mode 自动净化历史脏 Token，彻底避免污染
        api.setAuthToken(null)
        state.value = 'open'
      } else {
        // Protected Mode
        if (res.authenticated) {
          state.value = 'authenticated'
        } else {
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
      const res = await api.post<{ mode: string; authenticated: boolean; token?: string }>('/api/v1/auth/login', {
        token: trimmed,
      })
      if (res && res.authenticated) {
        api.setAuthToken(trimmed)
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
    try {
      await api.post('/api/v1/auth/logout')
    } catch {}
    api.setAuthToken(null)
    state.value = 'unauthenticated'
    if (status.value) {
      status.value.authenticated = false
    }
  }

  const clearStoredToken = () => {
    api.setAuthToken(null)
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
