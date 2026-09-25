// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'
import ErrorStateCard from './ErrorStateCard.vue'
import { ApiError } from '../api/client'

describe('ErrorStateCard UI Component', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
  })

  afterEach(() => {
    if (app) {
      app.unmount()
      app = null
    }
    container.remove()
    vi.restoreAllMocks()
  })

  function mountComponent(props: Record<string, any> = {}) {
    const propsState = reactive({ ...props })
    const emitted: Record<string, any[]> = {
      retry: [],
      'auth-recovery': [],
    }

    app = createApp({
      render() {
        return h(ErrorStateCard as any, {
          ...propsState,
          onRetry: () => {
            emitted.retry.push(true)
          },
          onAuthRecovery: () => {
            emitted['auth-recovery'].push(true)
          },
        })
      },
    })

    app.mount(container)
    return { container, propsState, emitted }
  }

  it('renders 401 unauthorized error with auth recovery button and human-readable guidance', async () => {
    const error = new ApiError(401, 'unauthorized', 'Invalid or missing token')
    const { container } = mountComponent({ error })

    expect(container.textContent).toContain('Authentication Required')
    expect(container.textContent).toContain('HTTP 401')
    expect(container.textContent).toContain('unauthorized')
    expect(container.textContent).toContain('Go to Settings / Auth')
  })

  it('renders 403 forbidden error with appropriate title and description', async () => {
    const error = new ApiError(403, 'forbidden', 'User is not permitted')
    const { container } = mountComponent({ error })

    expect(container.textContent).toContain('Access Denied')
    expect(container.textContent).toContain('HTTP 403')
  })

  it('renders 409 conflict error with appropriate title and description', async () => {
    const error = new ApiError(409, 'conflict', 'State mutation conflict')
    const { container } = mountComponent({ error })

    expect(container.textContent).toContain('Resource Conflict')
    expect(container.textContent).toContain('HTTP 409')
  })

  it('renders 500 server error and triggers retry event when clicking retry', async () => {
    const error = new ApiError(500, 'internal_error', 'Internal database failure')
    const { container, emitted } = mountComponent({ error })

    expect(container.textContent).toContain('Service Temporarily Unavailable')
    expect(container.textContent).toContain('HTTP 500')

    const retryBtn = container.querySelector('button.btn-error') as HTMLButtonElement | null
    expect(retryBtn).not.toBeNull()
    retryBtn?.click()
    await nextTick()

    expect(emitted.retry.length).toBe(1)
  })

  it('parses error string with status code', async () => {
    const { container } = mountComponent({ error: 'Request failed with 401: unauthorized access' })

    expect(container.textContent).toContain('Authentication Required')
    expect(container.textContent).toContain('HTTP 401')
  })

  it('triggers auth-recovery event and hash navigation when clicked', async () => {
    const error = new ApiError(401, 'unauthorized', 'Token expired')
    const { container, emitted } = mountComponent({ error })

    const authBtn = Array.from(container.querySelectorAll('button')).find((b) => b.textContent?.includes('Settings'))
    expect(authBtn).toBeDefined()
    authBtn?.click()
    await nextTick()

    expect(emitted['auth-recovery'].length).toBe(1)
    expect(window.location.hash).toBe('#settings')
  })
})
