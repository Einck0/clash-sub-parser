export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly requestId?: string
  readonly details?: unknown

  constructor(status: number, code: string, message: string, requestId?: string, details?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.requestId = requestId
    this.details = details
  }
}

export interface ApiClientOptions {
  baseUrl?: string
  fetcher?: typeof fetch
  requestId?: () => string
  getAuthToken?: () => string | null | undefined
}

export interface RequestOptions extends RequestInit {
  params?: Record<string, string | number | boolean | undefined | null>
  requestId?: string
}

export class ApiClient {
  private baseUrl: string
  private fetcher: typeof fetch
  private requestIdGenerator: () => string
  private getAuthTokenFn?: () => string | null | undefined
  private authToken: string | null | undefined = undefined
  private onUnauthorizedCallback?: () => void

  constructor(options: ApiClientOptions = {}) {
    this.baseUrl = options.baseUrl ? options.baseUrl.replace(/\/+$/, '') : ''
    this.fetcher = options.fetcher ?? globalThis.fetch.bind(globalThis)
    this.requestIdGenerator = options.requestId ?? (() => {
      const entropy = Math.random().toString(36).slice(2, 10)
      return `csp-${entropy}-${Date.now().toString(36)}`
    })
    this.getAuthTokenFn = options.getAuthToken
  }

  /**
   * Registers a single reactive unauthorized callback managed by useAuth state machine.
   */
  setOnUnauthorized(cb: () => void): void {
    this.onUnauthorizedCallback = cb
  }

  /**
   * Updates in-memory authentication token and synchronizes with localStorage if available.
   */
  setAuthToken(token: string | null): void {
    this.authToken = token
    if (typeof window !== 'undefined' && typeof window.localStorage !== 'undefined') {
      try {
        if (token) {
          window.localStorage.setItem('csp_token', token)
        } else {
          window.localStorage.removeItem('csp_token')
        }
      } catch {
        // ignore localStorage access errors
      }
    }
  }

  /**
   * Resolves the current active authentication token.
   */
  getAuthToken(): string | null {
    if (this.authToken !== undefined) {
      return this.authToken
    }
    return this.getAuthTokenFn?.() ?? null
  }

  async request<T = unknown>(path: string, options: RequestOptions = {}): Promise<T> {
    const { params, requestId, ...init } = options
    let url = path.startsWith('http://') || path.startsWith('https://') ? path : `${this.baseUrl}${path.startsWith('/') ? '' : '/'}${path}`

    if (params) {
      const searchParams = new URLSearchParams()
      for (const [key, value] of Object.entries(params)) {
        if (value !== undefined && value !== null) {
          searchParams.set(key, String(value))
        }
      }
      const query = searchParams.toString()
      if (query) {
        url += (url.includes('?') ? '&' : '?') + query
      }
    }

    const headers = new Headers(init.headers)
    if (!headers.has('X-Request-ID')) {
      headers.set('X-Request-ID', requestId ?? this.requestIdGenerator())
    }
    if (!headers.has('Accept')) {
      headers.set('Accept', 'application/json')
    }

    if (!headers.has('Authorization')) {
      const token = this.getAuthToken()
      if (token) {
        headers.set('Authorization', `Bearer ${token}`)
      }
    }

    if (init.body && typeof init.body === 'object' && !(init.body instanceof FormData) && !(init.body instanceof Blob)) {
      if (!headers.has('Content-Type')) {
        headers.set('Content-Type', 'application/json')
      }
      init.body = JSON.stringify(init.body)
    }

    const res = await this.fetcher(url, {
      ...init,
      headers,
    })

    const effectiveRequestId = res.headers.get('X-Request-ID') ?? headers.get('X-Request-ID') ?? undefined

    if (!res.ok) {
      if (res.status === 401) {
        try {
          this.onUnauthorizedCallback?.()
        } catch (err) {
          console.error('Error in onUnauthorized callback:', err)
        }
      }

      let code = 'http_error'
      let message = `Request failed with status ${res.status}`
      let errRequestId = effectiveRequestId
      let details: unknown

      try {
        const text = await res.text()
        if (text) {
          const payload = JSON.parse(text)
          if (payload && typeof payload === 'object') {
            if (typeof payload.code === 'string') code = payload.code
            if (typeof payload.message === 'string') message = payload.message
            if (typeof payload.request_id === 'string') errRequestId = payload.request_id
            details = payload.details ?? payload
          }
        }
      } catch {
        // use default fallback
      }

      throw new ApiError(res.status, code, message, errRequestId, details)
    }

    if (res.status === 204) {
      return undefined as T
    }

    const contentType = res.headers.get('content-type') || ''
    if (contentType.includes('application/json')) {
      const payload = await res.json()
      if (payload && typeof payload === 'object' && 'data' in payload) {
        return payload.data as T
      }
      return payload as T
    }

    const text = await res.text()
    if (text && (text.startsWith('{') || text.startsWith('['))) {
      try {
        const payload = JSON.parse(text)
        if (payload && typeof payload === 'object' && 'data' in payload) {
          return payload.data as T
        }
        return payload as T
      } catch {
        // fallback to returning text
      }
    }

    return text as unknown as T
  }

  get<T = unknown>(path: string, options?: RequestOptions): Promise<T> {
    return this.request<T>(path, { ...options, method: 'GET' })
  }

  post<T = unknown>(path: string, body?: unknown, options?: RequestOptions): Promise<T> {
    return this.request<T>(path, { ...options, method: 'POST', body: body as any })
  }

  put<T = unknown>(path: string, body?: unknown, options?: RequestOptions): Promise<T> {
    return this.request<T>(path, { ...options, method: 'PUT', body: body as any })
  }

  patch<T = unknown>(path: string, body?: unknown, options?: RequestOptions): Promise<T> {
    return this.request<T>(path, { ...options, method: 'PATCH', body: body as any })
  }

  delete<T = unknown>(path: string, options?: RequestOptions): Promise<T> {
    return this.request<T>(path, { ...options, method: 'DELETE' })
  }
}

export const api = new ApiClient({
  baseUrl: '',
  getAuthToken: () => {
    if (typeof window !== 'undefined') {
      return localStorage.getItem('csp_token')
    }
    return null
  },
})

export const setAuthToken = (token: string | null): void => api.setAuthToken(token)
export const setOnUnauthorized = (cb: () => void): void => api.setOnUnauthorized(cb)
