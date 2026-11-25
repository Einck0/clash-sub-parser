// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive, type App } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { api, ApiError } from '../../api/client'
import ChainsView from './ChainsView.vue'
import ChainEffectLedger from './ChainEffectLedger.vue'
import type { EffectNode, OptionsResult, ProxyChainBinding } from './types'

const binding: ProxyChainBinding = {
  id: 'binding-main', target_type: 'subscription', target_id: 'subscription-main',
  dialer_type: 'node_group', dialer_id: 'group-upstream', enabled: true,
  sort_order: 10, note: 'Original note', revision: 3,
}
const options: OptionsResult = {
  subscriptions: [{ id: 'subscription-main', name: 'Main egress' }],
  groups: [{ id: 'group-upstream', name: 'Global upstream' }],
  nodes: [],
}

// Only the API boundary is replaced; the real view, drawer and draft manager are mounted.
describe('Proxy chain component recovery', () => {
  let app: App | undefined
  let container: HTMLDivElement
  async function settle() {
    for (let i = 0; i < 5; i++) { await Promise.resolve(); await nextTick() }
  }
  function element<T extends HTMLElement = HTMLElement>(selector: string): T {
    const result = document.body.querySelector<T>(selector)
    expect(result, `Expected ${selector}`).toBeTruthy()
    return result!
  }
  async function input(selector: string, value: string) {
    const field = element<HTMLInputElement>(selector)
    field.value = value
    field.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
  }
  async function mountView() {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/chains', component: ChainsView }],
    })
    await router.push('/chains')
    await router.isReady()
    app = createApp(ChainsView)
    app.use(router)
    app.mount(container)
    await settle()
  }
  async function edit() {
    element<HTMLButtonElement>('[data-testid="chain-edit-btn-binding-main"]').click()
    await settle()
  }

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/proxy-chains') return { bindings: [{ ...binding }], version: 'config-verified' }
      if (path.endsWith('/options')) return options
      return { version: 'config-verified', nodes: [], effects: [] }
    })
    vi.spyOn(api, 'post').mockResolvedValue({ version: 'config-verified', nodes: [], effects: [] })
  })
  afterEach(() => {
    app?.unmount()
    app = undefined
    container.remove()
    vi.restoreAllMocks()
  })

  it('consumes one real Drawer close gesture once and preserves a declined dirty draft', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    await mountView()
    await edit()
    await input('[data-testid="chain-note"]', 'Keep this draft')
    element<HTMLButtonElement>('[data-testid="drawer-close"]').click()
    await settle()
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(element<HTMLInputElement>('[data-testid="chain-note"]').value).toBe('Keep this draft')
    confirm.mockReturnValue(true)
    element<HTMLButtonElement>('[data-testid="drawer-close"]').click()
    await settle()
    expect(confirm).toHaveBeenCalledTimes(2)
    expect(document.body.querySelector('[data-testid="chain-drawer"]')).toBeNull()
  })

  it('keeps unavailable lookups unverified in the table and drawer until a successful lookup proves a missing reference', async () => {
    const get = vi.mocked(api.get)
    get.mockImplementation(async (path: string) => {
      if (path === '/api/v1/proxy-chains') return { bindings: [{ ...binding }], version: 'config-verified' }
      if (path.endsWith('/options')) throw new Error('Options read failed')
      return { version: 'config-verified', nodes: [], effects: [] }
    })
    await mountView()
    const row = element('[data-testid="chain-row-binding-main"]')
    expect(row.textContent).toContain('待确认')
    expect(row.textContent).not.toContain('失效')
    await edit()
    expect(element('[data-testid="chain-drawer"]').textContent).not.toContain('已失效')
    expect(element<HTMLSelectElement>('[data-testid="chain-target-id"]').value).toBe(binding.target_id)
    expect(element<HTMLSelectElement>('[data-testid="chain-dialer-id"]').value).toBe(binding.dialer_id)
    get.mockResolvedValue({ nodes: [], groups: [], subscriptions: [] })
    element<HTMLButtonElement>('[data-testid="chain-options-unverified"] button').click()
    await settle()
    expect(element('[data-testid="chain-missing-notice"]').textContent).toContain('失效引用')
    expect(element<HTMLSelectElement>('[data-testid="chain-target-id"]').value).toBe(binding.target_id)
    expect(element<HTMLButtonElement>('[data-testid="chain-save-btn"]').disabled).toBe(false)
  })

  it.each(['reload', 'rebase'] as const)('preserves draft and revision and exposes %s failure inside the drawer', async (action) => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const put = vi.spyOn(api, 'put').mockRejectedValue(new ApiError(409, 'conflict', 'Server revision changed'))
    await mountView()
    await edit()
    await input('[data-testid="chain-note"]', 'Unsaved recovery note')
    element<HTMLButtonElement>('[data-testid="chain-save-btn"]').click()
    await settle()
    expect(element('[data-testid="chain-conflict-alert"]')).toBeTruthy()
    vi.mocked(api.get).mockRejectedValue(new Error('Latest read unavailable'))
    element<HTMLButtonElement>(`[data-testid="chain-conflict-${action}-btn"]`).click()
    await settle()
    expect(element<HTMLInputElement>('[data-testid="chain-note"]').value).toBe('Unsaved recovery note')
    expect(element('[data-testid="chain-recovery-error"]').textContent).toContain('Latest read unavailable')
    expect(element('[data-testid="chain-conflict-alert"]')).toBeTruthy()
    // A failed recovery must not silently adopt a new row revision.
    element<HTMLButtonElement>('[data-testid="chain-save-btn"]').click()
    await settle()
    expect(put).toHaveBeenLastCalledWith('/api/v1/proxy-chains/binding-main', expect.objectContaining({
      expected_revision: 3,
      binding: expect.objectContaining({ note: 'Unsaved recovery note' }),
    }))
  })

  it('does not discard a draft when only the options part of a reload fails', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    vi.spyOn(api, 'put').mockRejectedValue(new ApiError(409, 'conflict', 'Changed'))
    await mountView()
    await edit()
    await input('[data-testid="chain-note"]', 'Retain after partial read')
    element<HTMLButtonElement>('[data-testid="chain-save-btn"]').click()
    await settle()
    vi.mocked(api.get).mockImplementation(async (path: string) => {
      if (path.endsWith('/options')) throw new Error('Candidate read failed')
      return { bindings: [{ ...binding, revision: 7 }], version: 'config-new', nodes: [], effects: [] }
    })
    element<HTMLButtonElement>('[data-testid="chain-conflict-reload-btn"]').click()
    await settle()
    expect(element<HTMLInputElement>('[data-testid="chain-note"]').value).toBe('Retain after partial read')
    expect(element('[data-testid="chain-recovery-error"]').textContent).toContain('Candidate read failed')
  })

  it('blocks repeat saves and close gestures while saving', async () => {
    let rejectSave!: (error: Error) => void
    const put = vi.spyOn(api, 'put').mockImplementation(() => new Promise((_, reject) => { rejectSave = reject }))
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    await mountView()
    await edit()
    await input('[data-testid="chain-note"]', 'Pending save')
    const save = element<HTMLButtonElement>('[data-testid="chain-save-btn"]')
    save.click()
    save.click()
    element<HTMLButtonElement>('[data-testid="drawer-close"]').click()
    await settle()
    expect(put).toHaveBeenCalledTimes(1)
    expect(confirm).not.toHaveBeenCalled()
    expect(save.disabled).toBe(true)
    expect(element<HTMLInputElement>('[data-testid="chain-note"]').value).toBe('Pending save')
    rejectSave(new Error('Save rejected'))
    await settle()
    expect(element('[data-testid="chain-save-error"]').textContent).toContain('Save rejected')
  })

  it('searches bindings by business name or note and limits long candidate selects without losing selection', async () => {
    const subscriptions = Array.from({ length: 85 }, (_, i) => ({ id: `sub-${i}`, name: `Egress ${i}` }))
    vi.mocked(api.get).mockImplementation(async (path: string) => {
      if (path.endsWith('/options')) return { ...options, subscriptions }
      if (path === '/api/v1/proxy-chains') return { bindings: [{ ...binding, target_id: 'sub-84', note: 'Special route' }], version: 'config-verified' }
      return { version: 'config-verified', nodes: [], effects: [] }
    })
    await mountView()
    await input('[data-testid="chain-binding-search"]', 'special')
    expect(document.body.querySelector('[data-testid="chain-row-binding-main"]')).toBeTruthy()
    await input('[data-testid="chain-binding-search"]', 'no such binding')
    expect(document.body.querySelector('[data-testid="chain-row-binding-main"]')).toBeNull()
    await input('[data-testid="chain-binding-search"]', 'Egress 84')
    await edit()
    const select = element<HTMLSelectElement>('[data-testid="chain-target-id"]')
    expect(select.options.length).toBeLessThanOrEqual(51)
    expect(select.value).toBe('sub-84')
    await input('[data-testid="chain-target-search"]', 'Egress 72')
    expect(select.options.length).toBe(2)
    expect(select.value).toBe('sub-84')
    expect(select.textContent).toContain('Egress 72')
    await input('[data-testid="chain-dialer-search"]', 'Global upstream')
    expect(element<HTMLSelectElement>('[data-testid="chain-dialer-id"]').textContent).toContain('Global upstream')
  })

  it('shows unknown counts on first ledger failure and retries without presenting an empty result', async () => {
    const retry = vi.fn()
    app = createApp({ render: () => h(ChainEffectLedger, { nodes: [], error: 'Ledger offline', onRetry: retry }) })
    app.mount(container)
    await nextTick()
    expect(element('[data-testid="ledger-total-count"]').textContent).toContain('—')
    expect(element('[data-testid="ledger-chained-count"]').textContent).toContain('—')
    expect(element('[data-testid="ledger-error"]').textContent).toContain('Ledger offline')
    element<HTMLButtonElement>('[data-testid="ledger-retry"]').click()
    expect(retry).toHaveBeenCalledTimes(1)
    expect(document.body.querySelector('table')).toBeNull()
  })

  it('labels retained ledger data after failure, searches resolved dialer names and does not dump all nodes', async () => {
    const nodes: EffectNode[] = Array.from({ length: 80 }, (_, i) => ({
      logical_id: `node-${i}`, display_name: `Egress ${i}`, dialer_proxy: i === 79 ? 'group-upstream' : undefined,
      chain_source: i === 79 ? 'subscription' : undefined,
    }))
    const state = reactive({ nodes, options, loaded: true, error: null as string | null })
    app = createApp({ render: () => h(ChainEffectLedger, state) })
    app.mount(container)
    await nextTick()
    expect(container.querySelectorAll('[data-testid^="ledger-node-row-"]')).toHaveLength(50)
    state.error = 'Refresh failed'
    await nextTick()
    expect(element('[data-testid="ledger-previous-data"]').textContent).toContain('上次成功读取')
    expect(element('[data-testid="ledger-total-count"]').textContent).toContain('80')
    await input('[data-testid="ledger-search-input"]', 'Global upstream')
    expect(container.querySelectorAll('[data-testid^="ledger-node-row-"]')).toHaveLength(1)
    expect(element('[data-testid="ledger-node-row-node-79"]').textContent).toContain('Global upstream')
    await input('[data-testid="ledger-search-input"]', '订阅绑定')
    expect(container.querySelectorAll('[data-testid^="ledger-node-row-"]')).toHaveLength(1)
  })
})
