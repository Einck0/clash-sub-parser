// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from '../../api/client'
import { useAuth, type AuthStatus } from './useAuth'
import { setLocale } from '../../locales'
import { usePublications } from '../publications/usePublications'

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(done => { resolve = done })
  return { promise, resolve }
}

describe('useAuth state machine', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
    sessionStorage.clear()
    api.setAuthToken(null)
    api.setCsrfToken(null)
    setLocale('zh-CN')
  })

  afterEach(() => {
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
    await auth.clearStoredToken()

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
    vi.spyOn(api, 'get').mockResolvedValue({ mode: 'protected', authenticated: false, subject: '' })
    sessionStorage.setItem('csp_publication_latest', '{"id":"pub-1","target":"mihomo","export_url":"/url"}')
    sessionStorage.setItem('csp_publication_active_mihomo', '{"id":"pub-1","target":"mihomo","export_url":"/url"}')
    sessionStorage.setItem('unrelated_session', 'preserve')

    const auth = useAuth()

    // 1. clearStoredToken
    await auth.clearStoredToken()
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

  it('ignores a slow unauthenticated probe after successful login', async () => {
    const oldStatus = deferred<AuthStatus>()
    vi.spyOn(api, 'get').mockReturnValue(oldStatus.promise)
    vi.spyOn(api, 'post').mockResolvedValue({ mode: 'protected', authenticated: true, subject: 'admin', csrf_token: 'new-csrf' })
    const auth = useAuth()
    const probing = auth.probe()
    await auth.login('new-token')
    oldStatus.resolve({ mode: 'protected', authenticated: false, subject: '' })
    await probing
    expect(auth.state.value).toBe('authenticated')
    expect(auth.status.value?.authenticated).toBe(true)
    expect(api.getAuthToken()).toBe('new-token')
    expect(api.getCsrfToken()).toBe('new-csrf')
  })

  it('does not restore authentication from a late probe after logout', async () => {
    const oldStatus = deferred<AuthStatus>()
    vi.spyOn(api, 'get').mockReturnValue(oldStatus.promise)
    vi.spyOn(api, 'post').mockResolvedValue({})
    const auth = useAuth()
    const probing = auth.probe()
    await auth.logout()
    oldStatus.resolve({ mode: 'protected', authenticated: true, subject: 'admin' })
    await probing
    expect(auth.state.value).toBe('unauthenticated')
    expect(api.getAuthToken()).toBeNull()
    expect(api.getCsrfToken()).toBeNull()
  })

  it('does not save a late successful login after local logout', async () => {
    const oldLogin = deferred<any>()
    vi.spyOn(api, 'post').mockImplementation((path: string) => path.endsWith('/login') ? oldLogin.promise : Promise.resolve({}))
    const auth = useAuth()
    const signingIn = auth.login('old-token')
    await auth.logout()
    oldLogin.resolve({ mode: 'protected', authenticated: true, subject: 'admin', csrf_token: 'old-csrf' })
    expect(await signingIn).toBe(false)
    expect(auth.state.value).toBe('unauthenticated')
    expect(api.getAuthToken()).toBeNull()
    expect(api.getCsrfToken()).toBeNull()
  })

  it('clears local credentials immediately but does not erase a newer login when logout finishes', async () => {
    const oldLogout = deferred<any>()
    vi.spyOn(api, 'post').mockImplementation((path: string) => path.endsWith('/logout') ? oldLogout.promise : Promise.resolve({
      mode: 'protected', authenticated: true, subject: 'admin', csrf_token: 'new-csrf',
    }))
    const auth = useAuth()
    api.setAuthToken('old-token')
    const loggingOut = auth.logout()
    expect(api.getAuthToken()).toBeNull()
    await auth.login('new-token')
    oldLogout.resolve({})
    await loggingOut
    expect(auth.state.value).toBe('authenticated')
    expect(api.getAuthToken()).toBe('new-token')
    expect(api.getCsrfToken()).toBe('new-csrf')
  })

  it('uses only the newest overlapping login response', async () => {
    const first = deferred<any>()
    vi.spyOn(api, 'post').mockReturnValueOnce(first.promise).mockResolvedValueOnce({ mode: 'protected', authenticated: true, subject: 'admin' })
    const auth = useAuth()
    const firstLogin = auth.login('first-token')
    expect(await auth.login('second-token')).toBe(true)
    first.resolve({ mode: 'protected', authenticated: true, subject: 'admin', csrf_token: 'outdated' })
    expect(await firstLogin).toBe(false)
    expect(api.getAuthToken()).toBe('second-token')
    expect(api.getCsrfToken()).toBeNull()
  })

  it('does not let an older probe success overwrite the current connection error', async () => {
    const first = deferred<AuthStatus>()
    vi.spyOn(api, 'get').mockReturnValueOnce(first.promise).mockRejectedValueOnce(new Error('Current connection failed'))
    const auth = useAuth()
    const firstProbe = auth.probe()
    await auth.probe()
    first.resolve({ mode: 'protected', authenticated: true, subject: 'admin' })
    await firstProbe
    expect(auth.state.value).toBe('error')
    expect(auth.errorMessage.value).toContain('Current connection failed')
  })

  it('invalidates an outstanding login when clearing the local token and probing again', async () => {
    const oldLogin = deferred<any>()
    vi.spyOn(api, 'post').mockReturnValue(oldLogin.promise)
    vi.spyOn(api, 'get').mockResolvedValue({ mode: 'open', authenticated: true, subject: 'admin' })
    const auth = useAuth()
    const signingIn = auth.login('outdated-token')
    await auth.clearStoredToken()
    oldLogin.resolve({ mode: 'protected', authenticated: true, subject: 'admin', csrf_token: 'outdated-csrf' })
    expect(await signingIn).toBe(false)
    expect(auth.state.value).toBe('open')
    expect(api.getAuthToken()).toBeNull()
    expect(api.getCsrfToken()).toBeNull()
  })

  it('does not restore a pending login after a current unauthorized signal', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ mode: 'protected', authenticated: false, subject: '' })
    const pending = deferred<any>()
    vi.spyOn(api, 'post').mockReturnValue(pending.promise)
    const auth = useAuth()
    await auth.probe()
    const signingIn = auth.login('pending-token')
    ;(api as any).onUnauthorizedCallback()
    pending.resolve({ mode: 'protected', authenticated: true, subject: 'admin', csrf_token: 'stale-csrf' })
    expect(await signingIn).toBe(false)
    expect(auth.state.value).toBe('unauthenticated')
    expect(auth.status.value?.authenticated).toBe(false)
    expect(api.getCsrfToken()).toBeNull()
    expect(api.getAuthToken()).toBeNull()
  })

  it('allows an in-memory login with denied localStorage getters and storage writes', async () => {
    vi.spyOn(window, 'localStorage', 'get').mockImplementation(() => { throw new DOMException('Denied', 'SecurityError') })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('Write denied') })
    vi.spyOn(api, 'post').mockResolvedValue({ mode: 'protected', authenticated: true, subject: 'admin', csrf_token: 'memory-csrf' })
    const auth = useAuth()
    expect(await auth.login('memory-token')).toBe(true)
    expect(auth.state.value).toBe('authenticated')
    expect(api.getAuthToken()).toBe('memory-token')
    expect(api.getCsrfToken()).toBe('memory-csrf')
    await auth.logout()
    expect(api.getAuthToken()).toBeNull()
    expect(api.getCsrfToken()).toBeNull()
  })

  it('invalidates mounted publication capabilities even when the sessionStorage getter is denied', async () => {
    sessionStorage.setItem('csp_publication_active_mihomo', '{"id":"fixture-capability","target":"mihomo","export_url":"/fixture"}')
    const publication = usePublications('mihomo')
    expect(publication.activePublication.value?.id).toBe('fixture-capability')
    vi.spyOn(window, 'sessionStorage', 'get').mockImplementation(() => { throw new DOMException('Denied', 'SecurityError') })
    vi.spyOn(api, 'post').mockResolvedValue({ mode: 'protected', authenticated: true, subject: 'admin', csrf_token: 'memory-csrf' })
    const auth = useAuth()
    expect(await auth.login('memory-token')).toBe(true)
    expect(publication.activePublication.value).toBeNull()
    expect(api.getCsrfToken()).toBe('memory-csrf')
    ;(api as any).onUnauthorizedCallback()
    expect(auth.state.value).toBe('unauthenticated')
    expect(api.getCsrfToken()).toBeNull()
    await auth.logout()
    expect(api.getAuthToken()).toBeNull()
  })

  it('keeps open-mode login semantics and does not store a bearer or CSRF token', async () => {
    api.setCsrfToken('old-csrf')
    vi.spyOn(api, 'post').mockResolvedValue({ mode: 'open', authenticated: true, subject: 'admin' })
    const auth = useAuth()
    expect(await auth.login('unused-fixture-token')).toBe(true)
    expect(auth.state.value).toBe('open')
    expect(api.getAuthToken()).toBeNull()
    expect(api.getCsrfToken()).toBeNull()
  })

  it('clears an old CSRF token when a replacement login does not return one', async () => {
    api.setCsrfToken('previous-csrf')
    vi.spyOn(api, 'post').mockResolvedValue({ mode: 'protected', authenticated: true, subject: 'admin' })
    expect(await useAuth().login('new-token')).toBe(true)
    expect(api.getCsrfToken()).toBeNull()
  })

  it('keeps empty-token guidance reactive across locale changes', async () => {
    const auth = useAuth()
    await auth.login(' ')
    expect(auth.errorMessage.value).toContain('令牌不能为空')
    setLocale('en-US')
    expect(auth.errorMessage.value).toBe('Enter your administrator token')
  })
})
