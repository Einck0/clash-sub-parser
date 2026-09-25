// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import AuthGate from './AuthGate.vue'
import * as useAuthModule from './useAuth'

describe('AuthGate component', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  const mockLogin = vi.fn()
  const mockClearStoredToken = vi.fn()
  const mockErrorMessage = { value: '' }

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    vi.restoreAllMocks()

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
})
