// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from '../../api/client'
import { useAuth, type AuthStatus } from './useAuth'
import { usePublications } from '../publications/usePublications'

describe('useAuth state machine', () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
    api.setAuthToken(null)
    vi.restoreAllMocks()
  })

  it('probes in open mode and auto-cleans dirty token from localStorage', async () => {
    localStorage.setItem('csp_token', 'stale-dirty-token-123')
    api.setAuthToken('stale-dirty-token-123')

    const getSpy = vi.spyOn(api, 'get').mockResolvedValue({
      mode: 'open',
      authenticated: true,
      subject: 'admin',
    } as AuthStatus)

    const auth = useAuth()
    await auth.probe()

    expect(getSpy).toHaveBeenCalledWith('/api/v1/auth/status')
    expect(auth.state.value).toBe('open')
    expect(auth.status.value).toEqual({
      mode: 'open',
      authenticated: true,
      subject: 'admin',
    })
    // Auto-cleansed
    expect(localStorage.getItem('csp_token')).toBeNull()
    expect(api.getAuthToken()).toBeNull()
    expect(auth.errorMessage.value).toBe('')
  })

  it('preserves a publication capability on a fresh open-mode page reload', async () => {
    sessionStorage.setItem('csp_publication_active_mihomo', '{"id":"public","target":"mihomo","export_url":"/publish/v1/public?token=public"}')
    vi.spyOn(api, 'get').mockResolvedValue({ mode: 'open', authenticated: true, subject: 'admin' } as AuthStatus)
    await useAuth().probe()
    expect(sessionStorage.getItem('csp_publication_active_mihomo')).toContain('public')
  })

  it('probes in protected mode when authenticated', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      mode: 'protected',
      authenticated: true,
      subject: 'admin',
    } as AuthStatus)

    const auth = useAuth()
    await auth.probe()

    expect(auth.state.value).toBe('authenticated')
    expect(auth.status.value?.authenticated).toBe(true)
  })

  it('probes in protected mode when unauthenticated', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      mode: 'protected',
      authenticated: false,
      subject: '',
    } as AuthStatus)

    const auth = useAuth()
    await auth.probe()

    expect(auth.state.value).toBe('unauthenticated')
    expect(auth.status.value?.authenticated).toBe(false)
  })

  it('handles probe error gracefully', async () => {
    vi.spyOn(api, 'get').mockRejectedValue(new Error('Network unreachable'))

    const auth = useAuth()
    await auth.probe()

    expect(auth.state.value).toBe('error')
    expect(auth.errorMessage.value).toContain('Network unreachable')
  })

  it('rejects empty token during login without calling API', async () => {
    const postSpy = vi.spyOn(api, 'post')
    const auth = useAuth()

    const success = await auth.login('   ')
    expect(success).toBe(false)
    expect(auth.errorMessage.value).toContain('令牌不能为空')
    expect(postSpy).not.toHaveBeenCalled()
  })

  it('successfully logs in, saves token, and transitions to authenticated', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({
      mode: 'protected',
      authenticated: true,
      token: 'valid-secret-token',
    })

    const auth = useAuth()
    const success = await auth.login('valid-secret-token')

    expect(success).toBe(true)
    expect(auth.state.value).toBe('authenticated')
    expect(auth.status.value?.authenticated).toBe(true)
    expect(localStorage.getItem('csp_token')).toBe('valid-secret-token')
    expect(api.getAuthToken()).toBe('valid-secret-token')
  })

  it('handles 401 unauthorized login failure with human-readable error', async () => {
    vi.spyOn(api, 'post').mockRejectedValue(new ApiError(401, 'unauthorized', 'Invalid credentials'))

    const auth = useAuth()
    const success = await auth.login('wrong-token')

    expect(success).toBe(false)
    expect(auth.errorMessage.value).toContain('令牌无效或已过期')
  })

  it('logs out and transitions to unauthenticated', async () => {
    const postSpy = vi.spyOn(api, 'post').mockResolvedValue({ message: 'Logged out' })
    localStorage.setItem('csp_token', 'valid-token')
    api.setAuthToken('valid-token')

    const auth = useAuth()
    await auth.logout()

    expect(postSpy).toHaveBeenCalledWith('/api/v1/auth/logout')
    expect(auth.state.value).toBe('unauthenticated')
    expect(localStorage.getItem('csp_token')).toBeNull()
    expect(api.getAuthToken()).toBeNull()
  })

  it('clearStoredToken purges localStorage and re-probes', async () => {
    localStorage.setItem('csp_token', 'stale-token')
    const getSpy = vi.spyOn(api, 'get').mockResolvedValue({
      mode: 'open',
      authenticated: true,
      subject: 'admin',
    } as AuthStatus)

    const auth = useAuth()
    auth.clearStoredToken()

    expect(localStorage.getItem('csp_token')).toBeNull()
    expect(getSpy).toHaveBeenCalledWith('/api/v1/auth/status')
  })

  it('transitions from authenticated to unauthenticated on 401 signal from api client', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      mode: 'protected',
      authenticated: true,
      subject: 'admin',
    } as AuthStatus)

    const auth = useAuth()
    await auth.probe()
    expect(auth.state.value).toBe('authenticated')

    // Simulate 401 triggering api.onUnauthorizedCallback
    ;(api as any).onUnauthorizedCallback?.()
    expect(auth.state.value).toBe('unauthenticated')
    expect(api.getCsrfToken()).toBeNull()
  })

  it('clears publication capabilities across all auth invalidation paths', async () => {
    sessionStorage.setItem('csp_publication_latest', '{"id":"pub-1","target":"mihomo","export_url":"/url"}')
    sessionStorage.setItem('csp_publication_active_mihomo', '{"id":"pub-1","target":"mihomo","export_url":"/url"}')
    sessionStorage.setItem('unrelated_session', 'preserve')

    const auth = useAuth()

    // 1. clearStoredToken
    auth.clearStoredToken()
    expect(sessionStorage.getItem('csp_publication_latest')).toBeNull()
    expect(sessionStorage.getItem('csp_publication_active_mihomo')).toBeNull()

    // 2. logout
    sessionStorage.setItem('csp_publication_latest', '{"id":"pub-2","target":"mihomo","export_url":"/url"}')
    sessionStorage.setItem('csp_publication_active_mihomo', '{"id":"pub-2","target":"mihomo","export_url":"/url"}')
    vi.spyOn(api, 'post').mockResolvedValueOnce({ message: 'Logged out' })
    await auth.logout()
    expect(sessionStorage.getItem('csp_publication_latest')).toBeNull()
    expect(sessionStorage.getItem('csp_publication_active_mihomo')).toBeNull()

    // 3. 401 callback also clears a mounted publication view's in-memory capability
    sessionStorage.setItem('csp_publication_latest', '{"id":"pub-3","target":"mihomo","export_url":"/url"}')
    sessionStorage.setItem('csp_publication_active_mihomo', '{"id":"pub-3","target":"mihomo","export_url":"/url"}')
    const publication = usePublications('mihomo')
    expect(publication.activePublication.value?.id).toBe('pub-3')
    ;(api as any).onUnauthorizedCallback?.()
    expect(publication.activePublication.value).toBeNull()
    expect(sessionStorage.getItem('csp_publication_latest')).toBeNull()
    expect(sessionStorage.getItem('csp_publication_active_mihomo')).toBeNull()

    // 4. login replacement
    sessionStorage.setItem('csp_publication_latest', '{"id":"pub-4","target":"mihomo","export_url":"/url"}')
    sessionStorage.setItem('csp_publication_active_mihomo', '{"id":"pub-4","target":"mihomo","export_url":"/url"}')
    vi.spyOn(api, 'post').mockResolvedValueOnce({ mode: 'protected', authenticated: true, token: 'new-token' })
    await auth.login('new-token')
    expect(sessionStorage.getItem('csp_publication_latest')).toBeNull()
    expect(sessionStorage.getItem('csp_publication_active_mihomo')).toBeNull()
    expect(sessionStorage.getItem('unrelated_session')).toBe('preserve')
  })

  it('stores and clears CSRF token alongside login and logout lifecycle', async () => {
    sessionStorage.clear()
    api.setCsrfToken(null)

    const postSpy = vi.spyOn(api, 'post').mockResolvedValue({
      mode: 'protected',
      authenticated: true,
      token: 'valid-secret-token',
      csrf_token: 'csrf-server-token-123',
    })

    const auth = useAuth()
    const success = await auth.login('valid-secret-token')

    expect(success).toBe(true)
    expect(api.getCsrfToken()).toBe('csrf-server-token-123')
    expect(sessionStorage.getItem('csp_csrf_token')).toBe('csrf-server-token-123')

    // Logout should clear CSRF token
    postSpy.mockResolvedValueOnce({ message: 'Logged out' })
    await auth.logout()
    expect(api.getCsrfToken()).toBeNull()
    expect(sessionStorage.getItem('csp_csrf_token')).toBeNull()
  })
})
