// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiClient, ApiError, setAuthToken, setOnUnauthorized } from './client'

afterEach(() => {
  vi.restoreAllMocks()
  localStorage.clear()
  sessionStorage.clear()
})

describe('ApiClient', () => {
  it('adds a request id and unwraps successful data', async () => {
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      expect(new Headers(init?.headers).get('X-Request-ID')).toMatch(/^csp-/)
      return new Response(JSON.stringify({ data: { status: 'ok' } }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      })
    })

    const client = new ApiClient({ fetcher })
    await expect(client.get<{ status: string }>('/healthz')).resolves.toEqual({ status: 'ok' })
  })

  it('preserves an explicit request id', async () => {
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      expect(new Headers(init?.headers).get('X-Request-ID')).toBe('request-from-ui')
      return new Response(JSON.stringify({ data: true }), { status: 200 })
    })

    const client = new ApiClient({ fetcher, requestId: () => 'request-from-ui' })
    await expect(client.get<boolean>('/readyz')).resolves.toBe(true)
  })

  it('turns the unified error envelope into ApiError', async () => {
    const fetcher = vi.fn(async () => new Response(JSON.stringify({
      code: 'unauthorized',
      message: 'Sign in required',
      request_id: 'server-request-id',
    }), { status: 401 }))

    const client = new ApiClient({ fetcher })
    await expect(client.get('/api/v1/nodes')).rejects.toEqual(expect.objectContaining<Partial<ApiError>>({
      code: 'unauthorized',
      message: 'Sign in required',
      requestId: 'server-request-id',
      status: 401,
    }))
  })

  it('handles query parameters correctly', async () => {
    let requestedUrl = ''
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      requestedUrl = String(input)
      return new Response(JSON.stringify({ data: [] }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      })
    })

    const client = new ApiClient({ fetcher, baseUrl: 'http://localhost:8080' })
    await client.get('/api/v1/nodes', {
      params: { page: 1, page_size: 20, active: true, unused: undefined },
    })

    expect(requestedUrl).toBe('http://localhost:8080/api/v1/nodes?page=1&page_size=20&active=true')
  })

  it('attaches auth bearer token and automatically serializes json body', async () => {
    let capturedHeaders: Headers | undefined
    let capturedBody: any
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      capturedHeaders = new Headers(init?.headers)
      capturedBody = init?.body
      return new Response(JSON.stringify({ data: { id: 'node-1' } }), {
        status: 201,
        headers: { 'content-type': 'application/json' },
      })
    })

    const client = new ApiClient({
      fetcher,
      getAuthToken: () => 'test-bearer-token',
    })

    const result = await client.post('/api/v1/nodes', { name: 'Node 1' })
    expect(result).toEqual({ id: 'node-1' })
    expect(capturedHeaders?.get('Authorization')).toBe('Bearer test-bearer-token')
    expect(capturedHeaders?.get('Content-Type')).toBe('application/json')
    expect(capturedBody).toBe(JSON.stringify({ name: 'Node 1' }))
  })

  it('handles 204 no content responses', async () => {
    const fetcher = vi.fn(async () => new Response(null, { status: 204 }))
    const client = new ApiClient({ fetcher })
    const result = await client.delete('/api/v1/nodes/node-1')
    expect(result).toBeUndefined()
  })

  it('handles non-json error responses', async () => {
    const fetcher = vi.fn(async () => new Response('Internal Server Error 500', { status: 500 }))
    const client = new ApiClient({ fetcher })
    await expect(client.get('/healthz')).rejects.toThrow('Request failed with status 500')
  })

  it('notifies setOnUnauthorized callback on 401 response and propagates ApiError', async () => {
    const fetcher = vi.fn(async () => new Response(JSON.stringify({ code: 'unauthorized', message: 'Auth required' }), { status: 401 }))
    const client = new ApiClient({ fetcher })
    const handler = vi.fn()
    client.setOnUnauthorized(handler)

    await expect(client.get('/api/v1/protected')).rejects.toThrow()
    expect(handler).toHaveBeenCalledTimes(1)
  })

  it('dynamically attaches and clears Bearer token via setAuthToken', async () => {
    let capturedHeaders: Headers | undefined
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      capturedHeaders = new Headers(init?.headers)
      return new Response(JSON.stringify({ data: 'ok' }), { status: 200 })
    })

    const client = new ApiClient({ fetcher })

    // Initially no token
    await client.get('/test')
    expect(capturedHeaders?.has('Authorization')).toBe(false)

    // Set token
    client.setAuthToken('dynamic-jwt-abc')
    await client.get('/test')
    expect(capturedHeaders?.get('Authorization')).toBe('Bearer dynamic-jwt-abc')

    // Clear token
    client.setAuthToken(null)
    await client.get('/test')
    expect(capturedHeaders?.has('Authorization')).toBe(false)
  })

  it('safely catches errors thrown inside onUnauthorized callback without breaking request lifecycle', async () => {
    const fetcher = vi.fn(async () => new Response(JSON.stringify({ code: 'unauthorized', message: 'Auth required' }), { status: 401 }))
    const client = new ApiClient({ fetcher })
    const crashingHandler = vi.fn(() => {
      throw new Error('Explosion in callback')
    })
    client.setOnUnauthorized(crashingHandler)

    // Request should still reject with ApiError as expected, without crashing the call stack
    await expect(client.get('/api/v1/nodes')).rejects.toEqual(expect.objectContaining<Partial<ApiError>>({
      status: 401,
      code: 'unauthorized',
    }))
    expect(crashingHandler).toHaveBeenCalledTimes(1)
  })

  it('exports module-level setOnUnauthorized and setAuthToken bound to default api instance', () => {
    expect(typeof setOnUnauthorized).toBe('function')
    expect(typeof setAuthToken).toBe('function')
  })

  it('attaches X-CSRF-Token on unsafe HTTP methods (POST, PUT, PATCH, DELETE) but not GET', async () => {
    let capturedHeaders: Headers | undefined
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      capturedHeaders = new Headers(init?.headers)
      return new Response(JSON.stringify({ data: 'ok' }), { status: 200 })
    })

    const client = new ApiClient({
      fetcher,
      getAuthToken: () => 'auth-secret-123',
    })
    client.setCsrfToken('csrf-token-abc')

    // GET request (safe method) - should NOT include X-CSRF-Token, only Authorization
    await client.get('/test')
    expect(capturedHeaders?.get('Authorization')).toBe('Bearer auth-secret-123')
    expect(capturedHeaders?.has('X-CSRF-Token')).toBe(false)

    // POST request (unsafe method) - MUST include X-CSRF-Token and Authorization without cross-leaking
    await client.post('/test', { hello: 'world' })
    expect(capturedHeaders?.get('Authorization')).toBe('Bearer auth-secret-123')
    expect(capturedHeaders?.get('X-CSRF-Token')).toBe('csrf-token-abc')

    // PUT request
    await client.put('/test', { hello: 'world' })
    expect(capturedHeaders?.get('X-CSRF-Token')).toBe('csrf-token-abc')

    // PATCH request
    await client.patch('/test', { hello: 'world' })
    expect(capturedHeaders?.get('X-CSRF-Token')).toBe('csrf-token-abc')

    // DELETE request
    await client.delete('/test')
    expect(capturedHeaders?.get('X-CSRF-Token')).toBe('csrf-token-abc')

    // After clearing CSRF token
    client.setCsrfToken(null)
    await client.post('/test', { hello: 'world' })
    expect(capturedHeaders?.has('X-CSRF-Token')).toBe(false)
  })

  it('preserves an explicit X-CSRF-Token in request options', async () => {
    let capturedHeaders: Headers | undefined
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      capturedHeaders = new Headers(init?.headers)
      return new Response(JSON.stringify({ data: 'ok' }), { status: 200 })
    })

    const client = new ApiClient({ fetcher })
    client.setCsrfToken('default-csrf')

    await client.post('/test', {}, {
      headers: { 'X-CSRF-Token': 'custom-csrf-override' },
    })

    expect(capturedHeaders?.get('X-CSRF-Token')).toBe('custom-csrf-override')
  })

  it('keeps relative API requests safe when browser storage property access is denied', async () => {
    const denied = () => { throw new DOMException('Storage is disabled', 'SecurityError') }
    vi.spyOn(window, 'localStorage', 'get').mockImplementation(denied)
    vi.spyOn(window, 'sessionStorage', 'get').mockImplementation(denied)
    expect(api.getAuthToken()).toBeNull()
    expect(api.getCsrfToken()).toBeNull()
    const fetcher = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response('{"data":true}', { status: 200 }))
    const client = new ApiClient({ fetcher, getAuthToken: denied, getCsrfToken: denied })
    await expect(client.get('/api/v1/auth/status')).resolves.toBe(true)
    expect(() => client.setAuthToken('memory-token')).not.toThrow()
    expect(() => client.setCsrfToken('memory-csrf')).not.toThrow()
    await client.post('/api/v1/settings', { enabled: true })
    const headers = new Headers(fetcher.mock.calls[1]?.[1]?.headers)
    expect(headers.get('Authorization')).toBe('Bearer memory-token')
    expect(headers.get('X-CSRF-Token')).toBe('memory-csrf')
    client.setAuthToken(null)
    client.setCsrfToken(null)
    expect(client.getAuthToken()).toBeNull()
    expect(client.getCsrfToken()).toBeNull()
  })

  it('handles storage methods that throw, not just restricted property getters', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('read denied') })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('write denied') })
    vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => { throw new Error('remove denied') })
    const client = new ApiClient({ fetcher: vi.fn(), getAuthToken: () => localStorage.getItem('csp_token') })
    expect(client.getAuthToken()).toBeNull()
    expect(client.getCsrfToken()).toBeNull()
    client.setAuthToken('memory-only')
    client.setCsrfToken('csrf-only')
    expect(client.getAuthToken()).toBe('memory-only')
    expect(client.getCsrfToken()).toBe('csrf-only')
    client.setAuthToken(null)
    client.setCsrfToken(null)
    expect(client.getAuthToken()).toBeNull()
    expect(client.getCsrfToken()).toBeNull()
  })

  it.each(['replacement', 'clearing', 'new auth operation'] as const)('does not invalidate a new session on an old 401 after %s', async (change) => {
    let respond!: (response: Response) => void
    const fetcher = vi.fn(() => new Promise<Response>(resolve => { respond = resolve }))
    const client = new ApiClient({ fetcher, requestId: () => 'old-request' })
    client.setAuthToken('old-session')
    const unauthorized = vi.fn()
    client.setOnUnauthorized(unauthorized)
    const pending = client.get('/api/v1/nodes')
    const rejection = expect(pending).rejects.toMatchObject({ status: 401, requestId: 'old-request', details: { reason: 'expired' } })
    if (change === 'new auth operation') client.invalidateAuthRequests()
    else client.setAuthToken(change === 'replacement' ? 'new-session' : null)
    respond(new Response(JSON.stringify({ error: { code: 'unauthorized', message: 'Expired', details: { reason: 'expired' } }, request_id: 'old-request' }), { status: 401 }))
    await rejection
    expect(unauthorized).not.toHaveBeenCalled()
    expect(fetcher).toHaveBeenCalledTimes(1)
  })

  it('ignores an old 401 when an external auth getter has moved to another token', async () => {
    let token = 'first-token'
    let respond!: (response: Response) => void
    const client = new ApiClient({
      getAuthToken: () => token,
      fetcher: vi.fn(() => new Promise<Response>(resolve => { respond = resolve })),
    })
    const unauthorized = vi.fn()
    client.setOnUnauthorized(unauthorized)
    const pending = client.get('/api/v1/nodes')
    const rejection = expect(pending).rejects.toMatchObject({ status: 401 })
    token = 'replacement-token'
    respond(new Response(null, { status: 401 }))
    await rejection
    expect(unauthorized).not.toHaveBeenCalled()
  })

  it('never retries an unauthorized mutation automatically', async () => {
    const fetcher = vi.fn(async () => new Response('{"error":{"code":"unauthorized","message":"Sign in again"}}', { status: 401 }))
    const client = new ApiClient({ fetcher })
    const unauthorized = vi.fn()
    client.setOnUnauthorized(unauthorized)
    await expect(client.post('/api/v1/subscriptions', { name: 'fixture' })).rejects.toMatchObject({ status: 401, code: 'unauthorized' })
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(unauthorized).toHaveBeenCalledTimes(1)
  })
})
