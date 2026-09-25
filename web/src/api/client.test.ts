import { describe, expect, it, vi } from 'vitest'
import { ApiClient, ApiError, setAuthToken, setOnUnauthorized } from './client'

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
})
