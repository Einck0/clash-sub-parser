// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'
import ErrorStateCard from './ErrorStateCard.vue'
import { ApiError } from '../api/client'
import { setLocale } from '../locales'

describe('ErrorStateCard UI Component', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    setLocale('zh-CN')
  })

  afterEach(() => {
    if (app) {
      app.unmount()
      app = null
    }
    container.remove()
    setLocale('zh-CN')
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

    expect(container.textContent).toContain('鉴权已失效或需要登录')
    expect(container.textContent).toContain('HTTP 401')
    expect(container.textContent).toContain('unauthorized')
    expect(container.textContent).toContain('前往设置 / 鉴权')
  })

  it('renders 403 forbidden error with appropriate title and description', async () => {
    const error = new ApiError(403, 'forbidden', 'User is not permitted')
    const { container } = mountComponent({ error })

    expect(container.textContent).toContain('访问受限')
    expect(container.textContent).toContain('HTTP 403')
  })

  it('renders 409 conflict error with appropriate title and description', async () => {
    const error = new ApiError(409, 'conflict', 'State mutation conflict')
    const { container } = mountComponent({ error })

    expect(container.textContent).toContain('状态冲突')
    expect(container.textContent).toContain('HTTP 409')
  })

  it('renders 500 server error and triggers retry event when clicking retry', async () => {
    const error = new ApiError(500, 'internal_error', 'Internal database failure')
    const { container, emitted } = mountComponent({ error })

    expect(container.textContent).toContain('服务暂时不可用')
    expect(container.textContent).toContain('HTTP 500')

    const retryBtn = container.querySelector('button.btn-error') as HTMLButtonElement | null
    expect(retryBtn).not.toBeNull()
    retryBtn?.click()
    await nextTick()

    expect(emitted.retry.length).toBe(1)
  })

  it('maps validation and compiler target capability error codes to Chinese titles', async () => {
    const error = new ApiError(422, 'unsupported_target_capability', 'Target does not support this capability')
    const { container } = mountComponent({ error })

    expect(container.textContent).toContain('目标格式不支持该协议或特性')
    expect(container.textContent).toContain('HTTP 422')
    expect(container.textContent).toContain('unsupported_target_capability')
  })

  it('does not misclassify 422 errors containing "network" keyword as network errors', async () => {
    const error = new ApiError(422, 'unsupported_target_capability', 'sing-box does not support network xhttp')
    const { container } = mountComponent({ error })

    expect(container.textContent).toContain('目标格式不支持该协议或特性')
    expect(container.textContent).not.toContain('网络连接异常')
    expect(container.textContent).toContain('HTTP 422')
  })

  it('parses error string with status code', async () => {
    const { container } = mountComponent({ error: 'Request failed with 401: unauthorized access' })

    expect(container.textContent).toContain('鉴权已失效或需要登录')
    expect(container.textContent).toContain('HTTP 401')
  })

  it('triggers auth-recovery event and hash navigation when clicked', async () => {
    const error = new ApiError(401, 'unauthorized', 'Token expired')
    const { container, emitted } = mountComponent({ error })

    const authBtn = Array.from(container.querySelectorAll('button')).find((b) => b.textContent?.includes('设置') || b.textContent?.includes('Settings'))
    expect(authBtn).toBeDefined()
    authBtn?.click()
    await nextTick()

    expect(emitted['auth-recovery'].length).toBe(1)
    expect(window.location.hash).toBe('#settings')
  })
})
