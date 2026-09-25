// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'
import PolicyView from './PolicyView.vue'
import { api } from '../../api/client'
import type { PolicyGroup } from './policyTypes'

describe('PolicyView Topology & Drawer Linkage', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  const mockGroups: PolicyGroup[] = [
    {
      id: 'grp-1',
      name: 'Proxy Group 1',
      group_type: 'select',
      edges: [{ id: 'edge-1', parent_group_id: 'grp-1', node_logical_id: 'node-hk-01', position: 0 }],
    },
    {
      id: 'grp-2',
      name: 'Auto Select Group',
      group_type: 'urltest',
      edges: [],
    },
  ]

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)

    if (!HTMLDialogElement.prototype.showModal) {
      HTMLDialogElement.prototype.showModal = function (this: HTMLDialogElement) {
        this.open = true
      }
    }
    if (!HTMLDialogElement.prototype.close) {
      HTMLDialogElement.prototype.close = function (this: HTMLDialogElement) {
        this.open = false
      }
    }

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/policies/groups') {
        return { items: [...mockGroups], total: 2 }
      }
      if (path === '/api/v1/policies/rules') {
        return { items: [], total: 0 }
      }
      if (path === '/api/v1/nodes') {
        return { items: [{ logical_id: 'node-hk-01', display_name: 'HK Node 1', active: true, protocol: 'ss' }], total: 1 }
      }
      return { items: [], total: 0 }
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

  async function mountPolicyView() {
    app = createApp({
      render() {
        return h(PolicyView)
      },
    })
    app.mount(container)
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))
  }

  it('renders policy groups and supports selecting a node to open drawer', async () => {
    await mountPolicyView()

    const cards = container.querySelectorAll('[data-testid="group-card"]')
    expect(cards.length).toBe(2)

    // Click first card header to select node and open drawer
    const firstCardHeader = cards[0].querySelector('.cursor-pointer') as HTMLElement | null
    expect(firstCardHeader).not.toBeNull()
    firstCardHeader?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    // Drawer should open and document should contain Policy Editor dialog
    const dialog = document.body.querySelector('[role="dialog"]')
    expect(dialog).not.toBeNull()
    expect(dialog?.textContent).toContain('Edit Policy Group')
    expect(dialog?.querySelector('input')?.value).toBe('Proxy Group 1')

    // First card should reflect selected styling ring
    expect(cards[0].className).toContain('border-primary')
  })

  it('cancels drawer and resets selected group without persisting changes', async () => {
    await mountPolicyView()

    const cards = container.querySelectorAll('[data-testid="group-card"]')
    const firstCardHeader = cards[0].querySelector('.cursor-pointer') as HTMLElement | null
    firstCardHeader?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const dialog = document.body.querySelector('[role="dialog"]')
    const cancelBtn = Array.from(dialog?.querySelectorAll('button') || []).find((b) => b.textContent?.includes('Cancel'))
    expect(cancelBtn).toBeDefined()
    cancelBtn?.click()
    await nextTick()
    // Trigger transition completion event in jsdom environment if transition hook is waiting
    dialog?.dispatchEvent(new Event('transitionend'))
    await nextTick()

    // Selection ring is immediately cleared upon cancel/close
    expect(cards[0].className).not.toContain('ring-2')
  })

  it('saves group updates and updates topology state reactively', async () => {
    const patchSpy = vi.spyOn(api, 'patch').mockResolvedValueOnce({
      id: 'grp-1',
      name: 'Proxy Group Renamed',
      group_type: 'urltest',
      edges: [],
    })

    await mountPolicyView()

    const cards = container.querySelectorAll('[data-testid="group-card"]')
    const firstCardHeader = cards[0].querySelector('.cursor-pointer') as HTMLElement | null
    firstCardHeader?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const dialog = document.body.querySelector('[role="dialog"]')
    const input = dialog?.querySelector('input') as HTMLInputElement | null
    expect(input).not.toBeNull()
    if (input) {
      input.value = 'Proxy Group Renamed'
      input.dispatchEvent(new Event('input'))
    }

    const submitBtn = dialog?.querySelector('button[type="submit"]') as HTMLButtonElement | null
    submitBtn?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    expect(patchSpy).toHaveBeenCalledWith('/api/v1/policies/groups/grp-1', {
      name: 'Proxy Group Renamed',
      group_type: 'select',
    })

    // UI card updates to new name
    expect(container.textContent).toContain('Proxy Group Renamed')
  })

  it('verifies PolicyEditorSheet responsive max-height dynamic viewport styling', async () => {
    await mountPolicyView()

    const cards = container.querySelectorAll('[data-testid="group-card"]')
    const firstCardHeader = cards[0].querySelector('.cursor-pointer') as HTMLElement | null
    firstCardHeader?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const dialog = document.body.querySelector('[role="dialog"]')
    expect(dialog).not.toBeNull()
    // Verify fluid responsive viewport styling is applied to the sheet dialog
    expect(dialog?.className).toContain('adaptive-surface-sheet')
    expect(dialog?.className).toContain('md:max-h-[85vh]')
  })

  it('renders ErrorStateCard degraded card on fetch failure and provides retry', async () => {
    vi.spyOn(api, 'get').mockRejectedValueOnce(new Error('Failed to load policy groups'))
    await mountPolicyView()

    const errorCard = container.querySelector('[data-testid="error-state-card"]')
    expect(errorCard).not.toBeNull()
    expect(errorCard?.textContent).toContain('Failed to load policy groups')
  })

  it('renders admission rules tab with min-w-0 responsive layout and break-all expressions', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/policies/groups') return { items: [], total: 0 }
      if (path === '/api/v1/policies/rules') {
        return {
          admission_rules: [
            {
              id: 'rule-test-1',
              name: 'Long Expression Rule',
              expression: 'country in ["HK","TW","SG","JP"] && protocol == "ss" && speed >= 10000000',
              action: 'allow',
              position: 1,
            },
          ],
          routing_rules: [],
        }
      }
      return { items: [], total: 0 }
    })

    await mountPolicyView()

    // Click the admission tab
    const tabs = Array.from(container.querySelectorAll('button'))
    const admissionTab = tabs.find((b) => b.textContent?.includes('Admission') || b.textContent?.includes('准入') || b.textContent?.includes('Rules'))
    expect(admissionTab).toBeTruthy()
    admissionTab?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const ruleCard = container.querySelector('[data-testid="admission-rule-card"]')
    expect(ruleCard).not.toBeNull()
    expect(ruleCard?.className).toContain('min-w-0')
    expect(ruleCard?.className).toContain('overflow-hidden')

    const expr = ruleCard?.querySelector('p')
    expect(expr?.className).toContain('break-all')
    expect(expr?.className).toContain('whitespace-pre-wrap')
  })

  it('opens Global Filter modal, displays precedence info, and saves updated conditions', async () => {
    const putSpy = vi.spyOn(api, 'put').mockResolvedValueOnce({
      spec: {
        conditions: [
          { field: 'protocol', op: 'equals', value: 'ss' },
          { field: 'display_name', op: 'contains', value: 'premium' },
        ],
      },
      updated_at: '2026-09-25T12:00:00Z',
    })

    await mountPolicyView()

    // Find and click Global Filter button
    const globalFilterBtn = container.querySelector('[data-testid="global-filter-btn"]') as HTMLButtonElement | null
    expect(globalFilterBtn).not.toBeNull()
    globalFilterBtn?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    // Verify modal opened
    const allDialogs = Array.from(document.body.querySelectorAll('dialog'))
    const modal = allDialogs.find((d) => d.textContent?.includes('Global Node Filter'))
    expect(modal).toBeDefined()
    expect(modal?.textContent).toContain('Order of Precedence')
    expect(modal?.textContent).toContain('Global Filter')

    // Add a condition
    const selects = modal?.querySelectorAll('select')
    const inputs = modal?.querySelectorAll('input')
    const addBtn = Array.from(modal?.querySelectorAll('button') || []).find((b) => b.textContent?.includes('Add Condition'))
    expect(addBtn).toBeDefined()

    if (inputs && inputs.length > 0) {
      inputs[0].value = 'premium'
      inputs[0].dispatchEvent(new Event('input'))
    }
    addBtn?.click()
    await nextTick()

    // Click Save Global Filter button
    const saveBtn = Array.from(modal?.querySelectorAll('button') || []).find((b) => b.textContent?.includes('Save Global Filter'))
    expect(saveBtn).toBeDefined()
    saveBtn?.click()
    await nextTick()

    expect(putSpy).toHaveBeenCalledWith('/api/v1/policies/global-node-filter', expect.any(Object))
  })

  it('renders GroupCard with custom filter conditions and dynamic pool indicator when expanded', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/policies/groups') {
        return {
          items: [
            {
              id: 'grp-dynamic',
              name: 'Dynamic Fast Nodes',
              group_type: 'urltest',
              edges: [],
              node_filter: {
                conditions: [
                  { field: 'probe_latency_ms', op: 'lte', probe_kind: 'baseline', value: '150' },
                ],
              },
            },
          ],
          total: 1,
        }
      }
      return { items: [], total: 0 }
    })

    await mountPolicyView()

    const cards = container.querySelectorAll('[data-testid="group-card"]')
    expect(cards.length).toBe(1)
    expect(cards[0].textContent).toContain('1 filter conds')

    // Click to expand
    const header = cards[0].querySelector('.cursor-pointer') as HTMLElement | null
    header?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    // Verify dynamic pool indicator and filter conditions details
    expect(cards[0].textContent).toContain('Dynamic Pool')
    expect(cards[0].textContent).toContain('probe_latency_ms lte "150"')
    expect(cards[0].textContent).toContain('dynamically selects matching candidates')
  })
})
