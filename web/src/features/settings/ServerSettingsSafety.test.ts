// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { api } from '../../api/client'
import { setLocale } from '../../locales'
import { toastStore } from '../../ui/toast'
import PublicBaseUrlSettings from './PublicBaseUrlSettings.vue'
import ProbeTimeoutSettings from './ProbeTimeoutSettings.vue'
import ProbeScheduleSettings from './ProbeScheduleSettings.vue'
import SubscriptionFetchSettings from './SubscriptionFetchSettings.vue'
import GlobalNodeFilterSettings from './GlobalNodeFilterSettings.vue'
import AuthSecuritySettings from './AuthSecuritySettings.vue'
import { useAuthSettings } from './useAuthSettings'

const flush = async () => { await nextTick(); await Promise.resolve(); await nextTick(); await Promise.resolve(); await nextTick() }
const schedule = { enabled: false, interval_seconds: 3600, kinds: ['baseline'], generation: 1, next_due_at: null, updated_at: 'now' }
const fetchSettings = {
  mode: 'environment', effective_mode: 'direct', effective_source: 'environment',
  configured_proxy_endpoint: '', effective_proxy_endpoint: '', credentials_present: false,
  effective_credentials_present: false,
}
const cases = [
  {
    name: 'public URL', component: PublicBaseUrlSettings, endpoint: '/api/v1/settings/public-base-url',
    input: 'settings-public-base-url-input', save: 'settings-save-public-base-url-btn', draft: 'https://draft.example.test',
    initial: { public_base_url: '' }, canonical: { public_base_url: 'https://canonical.example.test' }, expected: 'https://canonical.example.test',
  },
  {
    name: 'probe timeout', component: ProbeTimeoutSettings, endpoint: '/api/v1/settings/probe-timeout',
    input: 'probe-timeout-input', save: 'save-probe-timeout-btn', draft: '9',
    initial: { probe_timeout_seconds: 5 }, canonical: { probe_timeout_seconds: 10 }, expected: '10',
  },
  {
    name: 'probe schedule', component: ProbeScheduleSettings, endpoint: '/api/v1/probes/schedule',
    input: 'probe-schedule-interval-input', save: 'save-probe-schedule-btn', draft: '7200',
    initial: schedule, canonical: { ...schedule, interval_seconds: 14400 }, expected: '14400',
  },
  {
    name: 'subscription fetch', component: SubscriptionFetchSettings, endpoint: '/api/v1/settings/subscription-fetch',
    input: 'fetch-proxy-url', save: 'fetch-save', draft: 'http://fixture:replacement@proxy.example:7890',
    initial: fetchSettings, canonical: fetchSettings, expected: '',
  },
  {
    name: 'global filter', component: GlobalNodeFilterSettings, endpoint: '/api/v1/policies/global-node-filter',
    input: 'global-empty-pass', save: 'filter-save', draft: false,
    initial: { spec: { conditions: [] }, empty_fallback_pass: true, updated_at: 'now' },
    canonical: { spec: { conditions: [] }, empty_fallback_pass: true, updated_at: 'later' }, expected: true,
  },
]

describe('server settings persistence boundary', () => {
  let root: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null
  const find = (id: string) => root.querySelector(`[data-testid="${id}"]`)
  const unsaved = () => {
    const event = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(event)
    return event.defaultPrevented
  }

  beforeEach(() => {
    vi.restoreAllMocks()
    toastStore.clear()
    setLocale('zh-CN')
    root = document.createElement('div')
    document.body.append(root)
  })
  afterEach(() => {
    app?.unmount()
    app = null
    root.remove()
    toastStore.clear()
    vi.restoreAllMocks()
  })

  it.each(cases)('$name hides unconfirmed defaults, retains failed drafts and accepts only canonical server state', async fixture => {
    let failRead = true
    let readValue: unknown = fixture.initial
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path !== fixture.endpoint) return { items: [], total: 0 }
      if (failRead) throw new Error('Read unavailable')
      return structuredClone(readValue)
    })
    const put = vi.spyOn(api, 'put')
    app = createApp({ render: () => h(fixture.component) })
    app.mount(root)
    await flush()
    expect(find(fixture.input)).toBeNull()
    expect(root.textContent).toContain('Read unavailable')
    expect(put).not.toHaveBeenCalled()

    failRead = false
    const retry = root.querySelector('[role="alert"] button') ?? find('filter-refresh')
    ;(retry as HTMLButtonElement).click()
    await flush()
    const input = find(fixture.input) as HTMLInputElement
    expect(input).not.toBeNull()
    if (input.type === 'checkbox') input.click()
    else {
      input.value = String(fixture.draft)
      input.dispatchEvent(new Event('input', { bubbles: true }))
    }
    await nextTick()
    expect(unsaved()).toBe(true)

    put.mockRejectedValueOnce(new Error('Write rejected'))
    ;(find(fixture.save) as HTMLButtonElement).click()
    await flush()
    expect(root.textContent).toContain('Write rejected')
    expect(input.type === 'checkbox' ? input.checked : input.value).toBe(fixture.draft)
    expect(unsaved()).toBe(true)

    put.mockResolvedValueOnce({})
    ;(find(fixture.save) as HTMLButtonElement).click()
    await flush()
    expect(root.textContent).toContain('无法确认')
    expect(input.type === 'checkbox' ? input.checked : input.value).toBe(fixture.draft)
    expect(unsaved()).toBe(true)
    expect(toastStore.items.value.some(item => item.tone === 'success')).toBe(false)
    const writeCount = put.mock.calls.length
    ;(find(fixture.save) as HTMLButtonElement).click()
    await flush()
    expect(put).toHaveBeenCalledTimes(writeCount)

    readValue = fixture.canonical
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    ;((root.querySelector('[role="alert"] button') ?? find('filter-refresh')) as HTMLButtonElement).click()
    await flush()
    expect(input.type === 'checkbox' ? input.checked : input.value).toBe(fixture.expected)
    expect(unsaved()).toBe(false)

    // A fresh edit can now be persisted using the confirmed server baseline.
    if (input.type === 'checkbox') input.click()
    else {
      input.value = String(fixture.draft)
      input.dispatchEvent(new Event('input', { bubbles: true }))
    }
    await nextTick()
    put.mockResolvedValueOnce(structuredClone(fixture.canonical))
    ;(find(fixture.save) as HTMLButtonElement).click()
    await flush()
    expect(input.type === 'checkbox' ? input.checked : input.value).toBe(fixture.expected)
    expect(unsaved()).toBe(false)
  })

  it('keeps authentication recovery enabled after a failed read and blocks writes until confirmed', async () => {
    const confirmed = {
      admin_auth_enabled: true, export_auth_enabled: true, token_configured: true,
      admin_mode: 'protected' as const, export_mode: 'protected' as const,
    }
    const get = vi.spyOn(api, 'get').mockRejectedValueOnce(new Error('Auth read unavailable')).mockResolvedValue(confirmed)
    const put = vi.spyOn(api, 'put').mockResolvedValueOnce({})
    const auth = useAuthSettings()
    app = createApp({ render: () => h(AuthSecuritySettings, { auth }) })
    app.mount(root)
    await auth.probeAuthMode()
    await flush()
    const retry = root.querySelector('[role="alert"] button') as HTMLButtonElement
    expect(retry.disabled).toBe(false)
    retry.click()
    await flush()
    expect(get).toHaveBeenCalledTimes(2)
    expect(auth.authMode.value).toBe('token')

    auth.exportEnabled.value = false
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    await auth.saveSwitches()
    await flush()
    expect(auth.settingsError.value).toContain('无法确认')
    expect(auth.isDirty.value).toBe(true)
    await auth.saveSwitches()
    expect(put).toHaveBeenCalledTimes(1)
    expect((root.querySelector('[role="alert"] button') as HTMLButtonElement).disabled).toBe(false)
    await auth.probeAuthMode()
    expect(auth.isDirty.value).toBe(false)
  })
})
