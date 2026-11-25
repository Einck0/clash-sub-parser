// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import { setLocale } from '../../locales'
import AuthGate from './AuthGate.vue'
import * as useAuthModule from './useAuth'

describe('AuthGate component', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  const mockLogin = vi.fn()
  const mockClearStoredToken = vi.fn()
  const mockErrorMessage = ref('')

  beforeEach(() => {
    vi.restoreAllMocks()
    setLocale('zh-CN')
    container = document.createElement('div')
    document.body.appendChild(container)

    mockLogin.mockReset()
    mockClearStoredToken.mockReset()
    mockErrorMessage.value = ''

    vi.spyOn(useAuthModule, 'useAuth').mockReturnValue({
      state: { value: 'unauthenticated' } as any,
      status: { value: { mode: 'protected', authenticated: false, subject: '' } } as any,
      errorMessage: mockErrorMessage as any,
      probe: vi.fn(),
      login: mockLogin,
      logout: vi.fn(),
      clearStoredToken: mockClearStoredToken,
    })
  })

  function mountComponent() {
    app = createApp({
      render() {
        return h(AuthGate)
      },
    })
    app.mount(container)

    return {
      getGate: () => container.querySelector('[data-testid=\"auth-gate\"]'),
      getInput: () => container.querySelector('input[data-testid=\"auth-gate-token-input\"]') as HTMLInputElement,
      getToggleBtn: () => container.querySelector('button[data-testid=\"auth-gate-toggle-visibility\"]') as HTMLButtonElement,
      getSubmitBtn: () => container.querySelector('button[data-testid=\"auth-gate-submit\"]') as HTMLButtonElement,
      getResetBtn: () => container.querySelector('button[data-testid=\"auth-gate-reset\"]') as HTMLButtonElement,
      getError: () => container.querySelector('[data-testid=\"auth-gate-error\"]'),
    }
  }

  afterEach(() => {
    app?.unmount()
    app = null
    container.remove()
    vi.restoreAllMocks()
  })

  it('renders auth gate form elements properly', () => {
    const { getGate, getInput, getSubmitBtn, getResetBtn } = mountComponent()
    expect(getGate()).not.toBeNull()
    expect(getInput()).not.toBeNull()
    expect(getSubmitBtn()).not.toBeNull()
    expect(getResetBtn()).not.toBeNull()
  })

  it('toggles password visibility', async () => {
    const { getInput, getToggleBtn } = mountComponent()
    expect(getInput().type).toBe('password')

    getToggleBtn().click()
    await nextTick()
    expect(getInput().type).toBe('text')

    getToggleBtn().click()
    await nextTick()
    expect(getInput().type).toBe('password')
  })

  it('submits token on button click or form submit and invokes auth.login', async () => {
    mockLogin.mockResolvedValue(true)
    const { getInput, getSubmitBtn } = mountComponent()

    getInput().value = 'my-admin-token'
    getInput().dispatchEvent(new Event('input'))
    await nextTick()

    getSubmitBtn().click()
    await nextTick()

    expect(mockLogin).toHaveBeenCalledWith('my-admin-token')
  })

  it('displays error alert when errorMessage is set', async () => {
    mockErrorMessage.value = '令牌无效或已过期，请检查后重试'
    const { getError } = mountComponent()
    await nextTick()

    const errorEl = getError()
    expect(errorEl).not.toBeNull()
    expect(errorEl?.textContent).toContain('令牌无效或已过期')
  })

  it('calls clearStoredToken when escape reset button is clicked', async () => {
    const { getResetBtn } = mountComponent()
    getResetBtn().click()
    await nextTick()

    expect(mockClearStoredToken).toHaveBeenCalledTimes(1)
  })

  it('updates human-readable labels and privacy controls when the locale changes', async () => {
    const { getGate, getInput, getToggleBtn, getResetBtn } = mountComponent()
    expect(getGate()?.textContent).toContain('登录控制台')
    setLocale('en-US')
    await nextTick()
    expect(getGate()?.textContent).toContain('Sign in to the console')
    expect(getInput().placeholder).toBe('Enter your administrator token')
    expect(getToggleBtn().getAttribute('aria-label')).toBe('Show token')
    getToggleBtn().click()
    await nextTick()
    expect(getToggleBtn().getAttribute('aria-label')).toBe('Hide token')
    expect(getToggleBtn().getAttribute('aria-pressed')).toBe('true')
    expect(getResetBtn().textContent).toContain('Clear token on this device')
  })

  it('prevents duplicate login and reset while pending, hides the token, and clears it after success', async () => {
    let finish!: (success: boolean) => void
    mockLogin.mockImplementation(() => new Promise<boolean>(resolve => { finish = resolve }))
    const { getInput, getToggleBtn, getSubmitBtn, getResetBtn } = mountComponent()
    getInput().value = 'fixture-admin-token'
    getInput().dispatchEvent(new Event('input'))
    getToggleBtn().click()
    await nextTick()
    getSubmitBtn().click()
    await nextTick()
    expect(getInput().disabled).toBe(true)
    expect(getInput().type).toBe('password')
    expect(getToggleBtn().disabled).toBe(true)
    expect(getSubmitBtn().disabled).toBe(true)
    expect(getResetBtn().disabled).toBe(true)
    container.querySelector('form')?.dispatchEvent(new Event('submit', { cancelable: true }))
    getResetBtn().click()
    expect(mockLogin).toHaveBeenCalledTimes(1)
    expect(mockClearStoredToken).not.toHaveBeenCalled()
    finish(true)
    await Promise.resolve()
    await nextTick()
    expect(getInput().value).toBe('')
    expect(getSubmitBtn().disabled).toBe(false)
  })

  it('keeps a failed login retryable and associates full errors with the input', async () => {
    mockLogin.mockResolvedValue(false)
    const { getInput, getSubmitBtn, getError } = mountComponent()
    getInput().value = 'wrong-fixture-token'
    getInput().dispatchEvent(new Event('input'))
    getSubmitBtn().click()
    await Promise.resolve()
    mockErrorMessage.value = `Please retry: ${'server-unavailable-'.repeat(30)}`
    await nextTick()
    expect(getInput().value).toBe('wrong-fixture-token')
    expect(getSubmitBtn().disabled).toBe(false)
    expect(getInput().getAttribute('aria-invalid')).toBe('true')
    expect(getInput().getAttribute('aria-describedby')).toBe(getError()?.id)
    expect(getError()?.textContent).toContain(mockErrorMessage.value)
  })
})
