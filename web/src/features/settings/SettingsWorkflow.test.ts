// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, onMounted, ref } from 'vue'
import { createRouter, createWebHashHistory, RouterView } from 'vue-router'
import { api } from '../../api/client'
import { setLocale } from '../../locales'
import SettingsView from './SettingsView.vue'
import { useServerSettings } from './useServerSettings'

const flush = async () => {
  await nextTick()
  await Promise.resolve()
  await new Promise(resolve => setTimeout(resolve, 0))
  await nextTick()
}

describe('settings workflow and draft safety', () => {
  let app: ReturnType<typeof createApp> | null = null
  let root: HTMLDivElement

  beforeEach(() => {
    vi.restoreAllMocks()
    setLocale('zh-CN')
    root = document.createElement('div')
    document.body.append(root)
  })

  afterEach(() => {
    app?.unmount()
    app = null
    root.remove()
    vi.restoreAllMocks()
    setLocale('zh-CN')
  })

  it('tracks section navigation in history, retains hidden drafts and prompts once on route exit', async () => {
    const apiGet = vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/settings/auth') return {
        admin_auth_enabled: true, export_auth_enabled: true, token_configured: true,
        admin_mode: 'protected', export_mode: 'protected',
      }
      if (path === '/api/v1/settings/public-base-url') return { public_base_url: 'https://example.test' }
      if (path === '/api/v1/settings/subscription-fetch') return {
        mode: 'environment', effective_mode: 'direct', effective_source: 'environment',
        configured_proxy_endpoint: '', effective_proxy_endpoint: '', credentials_present: false,
        effective_credentials_present: false,
      }
      if (path === '/api/v1/probes/schedule') return {
        enabled: false, interval_seconds: 3600, kinds: ['baseline'], next_due_at: null,
        generation: 1, updated_at: '2026-11-25T00:00:00Z',
      }
      if (path === '/api/v1/settings/probe-timeout') return { probe_timeout_seconds: 5 }
      if (path === '/api/v1/policies/global-node-filter') return {
        spec: { conditions: [] }, empty_fallback_pass: true, updated_at: '2026-11-25T00:00:00Z',
      }
      return { items: [], total: 0 }
    })
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const put = vi.spyOn(api, 'put')
    const router = createRouter({
      history: createWebHashHistory(),
      routes: [
        { path: '/settings', component: SettingsView },
        { path: '/outside', component: { render: () => h('div', 'Outside') } },
      ],
    })
    await router.push('/settings?section=display')
    app = createApp({ render: () => h(RouterView) })
    app.use(router)
    app.mount(root)
    await flush()

    expect(apiGet).toHaveBeenCalledWith('/api/v1/settings/auth')
    expect((root.querySelector('[data-testid="settings-category-select"]') as HTMLSelectElement).value).toBe('display')
    expect((root.querySelector('[data-testid="settings-panel-display"]') as HTMLElement).style.display).not.toBe('none')
    expect(window.location.hash).toContain('section=display')
    await router.replace('/settings')
    await flush()
    expect(root.querySelectorAll('[data-testid^="settings-tab-"]')).toHaveLength(5)
    const urlInput = root.querySelector('[data-testid="settings-public-base-url-input"]') as HTMLInputElement
    urlInput.value = 'https://draft.example.test'
    urlInput.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()

    ;(root.querySelector('[data-testid="settings-tab-probe"]') as HTMLAnchorElement).click()
    await flush()
    expect(router.currentRoute.value.query.section).toBe('probe')
    expect(confirm).not.toHaveBeenCalled()
    expect(root.querySelector('[data-testid="settings-unsaved-summary"]')).not.toBeNull()

    router.back()
    await new Promise(resolve => setTimeout(resolve, 20))
    await flush()
    expect(router.currentRoute.value.query.section).toBeUndefined()
    expect((root.querySelector('[data-testid="settings-public-base-url-input"]') as HTMLInputElement).value).toBe('https://draft.example.test')

    await router.push('/settings?section=security')
    await flush()
    ;(root.querySelector('[data-testid="save-auth-switches-btn"]') as HTMLButtonElement).click()
    await flush()
    expect(put).not.toHaveBeenCalled()
    expect(confirm).toHaveBeenCalledOnce()
    expect(urlInput.value).toBe('https://draft.example.test')
    confirm.mockClear()

    await router.push('/outside')
    await flush()
    expect(router.currentRoute.value.path).toBe('/settings')
    expect(confirm).toHaveBeenCalledTimes(1)

    confirm.mockReturnValue(true)
    await router.push('/outside')
    await flush()
    expect(router.currentRoute.value.path).toBe('/outside')
    expect(confirm).toHaveBeenCalledTimes(2)
  })

  it('does not expose defaults on read failure and preserves edits after failed save', async () => {
    const read = vi.fn<() => Promise<number>>()
      .mockRejectedValueOnce(new Error('Read unavailable'))
      .mockResolvedValue(5)
    const write = vi.fn<() => Promise<number>>()
    let state!: ReturnType<typeof useServerSettings<number>>

    const Harness = defineComponent({
      setup() {
        const value = ref(0)
        state = useServerSettings({ read, apply: result => { value.value = result }, snapshot: () => value.value })
        onMounted(() => { void state.load() })
        return () => h('section', [
          state.loaded.value
            ? h('input', { 'data-testid': 'draft', value: value.value, onInput: (event: Event) => { value.value = Number((event.target as HTMLInputElement).value) } })
            : null,
          h('p', { role: 'alert' }, state.error.value),
          h('button', { 'data-testid': 'retry', onClick: () => { void state.load() } }, 'Retry'),
          h('button', { 'data-testid': 'save', onClick: () => { void state.save(write) } }, 'Save'),
          h('output', { 'data-testid': 'dirty' }, String(state.dirty.value)),
        ])
      },
    })
    app = createApp(Harness)
    app.mount(root)
    await flush()

    expect(root.querySelector('[data-testid="draft"]')).toBeNull()
    expect(root.querySelector('[role="alert"]')?.textContent).toContain('Read unavailable')

    ;(root.querySelector('[data-testid="retry"]') as HTMLButtonElement).click()
    await flush()
    expect((root.querySelector('[data-testid="draft"]') as HTMLInputElement).value).toBe('5')

    const draft = root.querySelector('[data-testid="draft"]') as HTMLInputElement
    draft.value = '9'
    draft.dispatchEvent(new Event('input', { bubbles: true }))
    write.mockRejectedValueOnce(new Error('Write rejected'))
    ;(root.querySelector('[data-testid="save"]') as HTMLButtonElement).click()
    await flush()
    expect((root.querySelector('[data-testid="draft"]') as HTMLInputElement).value).toBe('9')
    expect(root.querySelector('[role="alert"]')?.textContent).toContain('Write rejected')
    expect(root.querySelector('[data-testid="dirty"]')?.textContent).toBe('true')

    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const readsBeforeCancel = read.mock.calls.length
    expect(await state.load()).toBe(false)
    expect(confirm).toHaveBeenCalledOnce()
    expect(read).toHaveBeenCalledTimes(readsBeforeCancel)
    expect(draft.value).toBe('9')
    confirm.mockReturnValue(true)
    read.mockRejectedValueOnce(new Error('Read failed again'))
    expect(await state.load()).toBe(false)
    await flush()
    expect(draft.value).toBe('9')
    expect(root.querySelector('[role="alert"]')?.textContent).toContain('Read failed again')
    expect(state.dirty.value).toBe(true)

    write.mockResolvedValueOnce(11)
    expect(await state.save(write)).toBe(false)
    expect(write).toHaveBeenCalledTimes(1)
    // A successful explicit read is required after the last read failed.
    expect(await state.load()).toBe(true)
    draft.value = '12'
    draft.dispatchEvent(new Event('input', { bubbles: true }))
    ;(root.querySelector('[data-testid="save"]') as HTMLButtonElement).click()
    await flush()
    expect((root.querySelector('[data-testid="draft"]') as HTMLInputElement).value).toBe('11')
    expect(root.querySelector('[data-testid="dirty"]')?.textContent).toBe('false')
  })
})
