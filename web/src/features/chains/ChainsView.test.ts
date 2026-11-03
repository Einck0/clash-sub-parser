// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createRouter, createWebHashHistory } from 'vue-router'
import { api, ApiError } from '../../api/client'
import ChainsView from './ChainsView.vue'
import type { ProxyChainConfig, OptionsResult, EffectsResult } from './types'

describe('ChainsView component', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  const router = createRouter({
    history: createWebHashHistory(),
    routes: [
      { path: '/settings', component: { render: () => h('div', 'Settings') } },
      { path: '/dns', component: { render: () => h('div', 'DNS') } },
      { path: '/chains', component: ChainsView },
    ],
  })

  const mockConfig: ProxyChainConfig = {
    bindings: [
      {
        id: 'chain-binding-1',
        target_type: 'subscription',
        target_id: 'sub-main',
        dialer_type: 'node_group',
        dialer_id: 'group-upstream',
        enabled: true,
        sort_order: 10,
        note: 'Main Sub to Upstream Group',
        revision: 1,
      },
      {
        id: 'chain-binding-2',
        target_type: 'node',
        target_id: 'node-isolated-99', // missing reference on purpose
        dialer_type: 'node',
        dialer_id: 'node-hk-1',
        enabled: false,
        sort_order: 5,
        note: 'Disabled invalid binding',
        revision: 2,
      },
    ],
    version: 'config-ver-sha256-111',
  }

  const mockOptions: OptionsResult = {
    subscriptions: [{ id: 'sub-main', name: 'Main Egress Subscription' }],
    groups: [{ id: 'group-upstream', name: 'Global Upstream Group' }],
    nodes: [{ id: 'node-hk-1', name: 'Hong Kong Node 1' }],
  }

  const mockEffects: EffectsResult = {
    version: 'config-ver-sha256-111',
    effects: [
      {
        binding_id: 'chain-binding-1',
        target_count: 3,
        chain_count: 2,
        skip_count: 1,
        overridden_count: 0,
        chain_samples: ['Node JP 1', 'Node JP 2'],
        skip_samples: ['Node HK 1 (overlap)'],
      },
    ],
    nodes: [
      {
        logical_id: 'node-jp-1',
        display_name: 'Node JP 1',
        dialer_proxy: 'group-upstream',
        chain_binding_id: 'chain-binding-1',
        chain_source: 'subscription',
      },
      {
        logical_id: 'node-direct-1',
        display_name: 'Direct Fallback Node',
      },
    ],
  }

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/proxy-chains') return { ...mockConfig }
      if (path === '/api/v1/proxy-chains/options') return { ...mockOptions }
      if (path === '/api/v1/proxy-chains/effects') return { ...mockEffects }
      return {}
    })

    vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
      if (path === '/api/v1/proxy-chains/preview') return { ...mockEffects }
      if (path === '/api/v1/proxy-chains') return { binding: mockConfig.bindings[0], version: 'config-ver-sha256-111' }
      return {}
    })
  })

  afterEach(() => {
    if (app) {
      app.unmount()
      app = null
    }
    container.remove()
    vi.restoreAllMocks()
  })

  async function mountView() {
    router.push('/chains')
    await router.isReady()

    app = createApp(ChainsView)
    app.use(router)
    app.mount(container)

    await nextTick()
    await Promise.resolve()
    await nextTick()
  }

  it('renders top SettingsSections navigation with active=chains', async () => {
    await mountView()

    const nav = container.querySelector('[data-testid="settings-sections-nav"]')
    expect(nav).toBeTruthy()

    const chainsLink = container.querySelector('[data-testid="settings-section-chains"]')
    expect(chainsLink).toBeTruthy()
    expect(chainsLink?.getAttribute('aria-current')).toBe('page')
  })

  it('renders PageToolbar and bindings table with proper priority indicators and missing reference tags', async () => {
    await mountView()

    const toolbar = container.querySelector('[data-testid="chains-toolbar"]')
    expect(toolbar).toBeTruthy()

    const table = container.querySelector('[data-testid="chains-table"]')
    expect(table).toBeTruthy()

    // 2 bindings rendered
    const row1 = container.querySelector('[data-testid="chain-row-chain-binding-1"]')
    expect(row1).toBeTruthy()
    expect(row1?.textContent).toContain('Main Egress Subscription')
    expect(row1?.textContent).toContain('Global Upstream Group')
    expect(row1?.textContent).toContain('P3') // subscription priority

    const row2 = container.querySelector('[data-testid="chain-row-chain-binding-2"]')
    expect(row2).toBeTruthy()
    expect(row2?.textContent).toContain('node-isolated-99')
    // Missing reference indicator must be shown for node-isolated-99
    expect(row2?.textContent).toContain('引用失效')
    expect(row2?.textContent).toContain('P1') // node priority
  })

  it('renders lower half ChainEffectLedger with per-node dialer assignment and filters', async () => {
    await mountView()

    const ledger = container.querySelector('[data-testid="chain-effect-ledger"]')
    expect(ledger).toBeTruthy()

    expect(ledger?.textContent).toContain('生效节点跳板台账')
    // Total count: 2, Chained count: 1
    const totalCount = container.querySelector('[data-testid="ledger-total-count"]')
    expect(totalCount?.textContent).toContain('2')
    const chainedCount = container.querySelector('[data-testid="ledger-chained-count"]')
    expect(chainedCount?.textContent).toContain('1')

    // Node JP 1 is chained
    const nodeRow = container.querySelector('[data-testid="ledger-node-row-node-jp-1"]')
    expect(nodeRow).toBeTruthy()
    expect(nodeRow?.textContent).toContain('group-upstream')
    expect(nodeRow?.textContent).toContain('订阅绑定')
  })

  it('opens drawer on "新建绑定" button click', async () => {
    await mountView()

    const newBtn = container.querySelector('[data-testid="new-chain-btn"]') as HTMLButtonElement
    expect(newBtn).toBeTruthy()

    newBtn.click()
    await nextTick()

    const drawer = document.body.querySelector('[data-testid="chain-drawer"]')
    expect(drawer).toBeTruthy()
  })

  it('opens drawer on edit button click and populates draft', async () => {
    await mountView()

    const editBtn = container.querySelector('[data-testid="chain-edit-btn-chain-binding-1"]') as HTMLButtonElement
    expect(editBtn).toBeTruthy()

    editBtn.click()
    await nextTick()

    const drawer = document.body.querySelector('[data-testid="chain-drawer"]')
    expect(drawer).toBeTruthy()
  })

  it('opens delete confirmation modal on delete action and handles 422 errors safely', async () => {
    vi.spyOn(api, 'delete').mockRejectedValue(
      new ApiError(422, 'chain_cycle', 'Cannot delete: exposes cyclic proxy dependency')
    )

    await mountView()

    const delBtn = container.querySelector('[data-testid="chain-delete-btn-chain-binding-1"]') as HTMLButtonElement
    expect(delBtn).toBeTruthy()

    delBtn.click()
    await nextTick()

    const modal = document.body.querySelector('[data-testid="chain-delete-modal"]')
    expect(modal).toBeTruthy()

    // Click confirm in modal
    const confirmBtn = modal?.querySelector('[data-testid="confirm-modal-confirm"]') as HTMLButtonElement
    expect(confirmBtn).toBeTruthy()

    confirmBtn.click()
    await nextTick()
    await Promise.resolve()
    await nextTick()

    // 422 error must be visible in the modal
    const errAlert = modal?.querySelector('[data-testid="chain-delete-error"]')
    expect(errAlert).toBeTruthy()
    expect(errAlert?.textContent).toContain('exposes cyclic proxy dependency')
  })
})
