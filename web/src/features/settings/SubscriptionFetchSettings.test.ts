// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import SubscriptionFetchSettings from './SubscriptionFetchSettings.vue'
import { api } from '../../api/client'
import { setLocale } from '../../locales'

const endpoint = '/api/v1/settings/subscription-fetch'
const baseline = { mode: 'proxy', effective_mode: 'proxy', effective_source: 'settings', configured_proxy_endpoint: 'http://proxy.example:7890', effective_proxy_endpoint: 'http://proxy.example:7890', credentials_present: true, effective_credentials_present: true }
const flush = async () => { await nextTick(); await Promise.resolve(); await nextTick() }
describe('subscription-fetch settings', () => {
  let root: HTMLDivElement
  let app: ReturnType<typeof createApp>
  const find = <T extends HTMLElement>(id: string) => root.querySelector(`[data-testid="${id}"]`) as T
  const click = (id: string) => find<HTMLButtonElement>(id).click()
  const edit = (id: string, value: string) => {
    const node = find<HTMLInputElement | HTMLSelectElement>(id)
    node.value = value
    node.dispatchEvent(new Event(node instanceof HTMLSelectElement ? 'change' : 'input', { bubbles: true }))
  }
  beforeEach(() => {
    vi.restoreAllMocks(); setLocale('zh-CN')
    root = document.createElement('div'); document.body.append(root)
    vi.spyOn(api, 'get').mockResolvedValue({ ...baseline })
    vi.spyOn(api, 'put').mockResolvedValue({ ...baseline })
    app = createApp({ render: () => h(SubscriptionFetchSettings) }); app.mount(root)
  })
  afterEach(() => { app.unmount(); root.remove(); vi.restoreAllMocks(); setLocale('zh-CN') })

  it('loads safe endpoints, leaves password blank, and retains credentials without echoing a masked URL', async () => {
    await flush()
    expect(api.get).toHaveBeenCalledWith(endpoint)
    expect(find<HTMLInputElement>('fetch-proxy-url').type).toBe('password')
    expect(find<HTMLInputElement>('fetch-proxy-url').value).toBe('')
    expect(root.textContent).toContain('已配置凭据')
    expect(find('fetch-effective').textContent).toBe(baseline.effective_proxy_endpoint)
    click('fetch-save'); await flush()
    expect(api.put).toHaveBeenCalledWith(endpoint, { mode: 'proxy' })
  })
  it('replaces credentials explicitly and clears input only after successful save', async () => {
    await flush()
    const replacement = 'https://fixture:replacement@proxy.example:8443'
    edit('fetch-proxy-url', replacement); click('fetch-save'); await flush()
    expect(api.put).toHaveBeenCalledWith(endpoint, { mode: 'proxy', proxy_url: replacement })
    expect(find<HTMLInputElement>('fetch-proxy-url').value).toBe('')
    expect(root.textContent).not.toContain('replacement@')
  })
  it('retains the draft and selected mode after a rejected save, then restores inheritance', async () => {
    await flush()
    vi.mocked(api.put).mockRejectedValueOnce(new Error('Rejected endpoint'))
    edit('fetch-mode', 'direct'); edit('fetch-proxy-url', 'not-a-url'); click('fetch-save'); await flush()
    expect(find('fetch-settings-error').textContent).toBe('Rejected endpoint')
    expect(find<HTMLInputElement>('fetch-proxy-url').value).toBe('not-a-url')
    expect(find<HTMLSelectElement>('fetch-mode').value).toBe('direct')
    vi.mocked(api.put).mockResolvedValueOnce({ ...baseline, mode: 'environment', effective_source: 'environment' })
    click('fetch-reset'); await flush()
    expect(api.put).toHaveBeenLastCalledWith(endpoint, { mode: 'environment' })
    expect(find<HTMLSelectElement>('fetch-mode').value).toBe('environment')
    expect(find<HTMLInputElement>('fetch-proxy-url').value).toBe('')
  })
  it('supports explicit clearing without also sending stale replacement input', async () => {
    await flush(); edit('fetch-mode', 'direct'); edit('fetch-proxy-url', 'http://stale:80')
    find<HTMLInputElement>('fetch-clear-proxy').click(); await nextTick()
    expect(find<HTMLInputElement>('fetch-proxy-url').disabled).toBe(true)
    click('fetch-save'); await flush()
    expect(api.put).toHaveBeenCalledWith(endpoint, { mode: 'direct', clear_proxy_url: true })
  })
  it('shows effective direct/inherited state and translated labels', async () => {
    setLocale('en-US')
    vi.mocked(api.put).mockResolvedValueOnce({ ...baseline, mode: 'environment', effective_mode: 'direct', effective_source: 'environment', effective_proxy_endpoint: '' })
    await flush(); click('fetch-reset'); await flush()
    expect(root.textContent).toContain('Inherit startup environment / CLI')
    expect(find('fetch-effective').textContent).toBe('Direct (ignore environment proxy)')
    expect(root.textContent).toContain('without restart')
  })
})
