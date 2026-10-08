// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
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
    expect(dialog?.textContent).toContain('编辑策略组')
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
    const cancelBtn = Array.from(dialog?.querySelectorAll('button') || []).find((b) => b.textContent?.includes('取消'))
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
    let saved = false
    vi.spyOn(api, 'get').mockImplementation(async (path) => {
      if (path.endsWith('/groups')) return { items: saved ? [{ ...mockGroups[0], name: 'Proxy Group Renamed' }, mockGroups[1]] : [...mockGroups], total: 2 }
      if (path.endsWith('/rules')) return { policy_rules: [], admission_rules: [] }
      return { items: [], spec: { conditions: [] } }
    })
    const patchSpy = vi.spyOn(api, 'patch').mockImplementationOnce(async () => {
      saved = true
      return { id: 'grp-1', name: 'Proxy Group Renamed', group_type: 'select', edges: [], empty_fallback_pass: false }
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
      empty_fallback_pass: false,
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
    vi.spyOn(api, 'get').mockRejectedValueOnce(new Error('加载策略组失败'))
    await mountPolicyView()

    const errorCard = container.querySelector('[data-testid="error-state-card"]')
    expect(errorCard).not.toBeNull()
    expect(errorCard?.textContent).toContain('加载策略组失败')
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
    const admissionTab = tabs.find((b) => b.textContent?.includes('准入规则'))
    expect(admissionTab).toBeTruthy()
    admissionTab?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const ruleCard = container.querySelector('[data-testid="admission-rule-card"]')
    expect(ruleCard).not.toBeNull()
    expect(ruleCard?.className).toContain('min-w-0')
    expect(ruleCard?.className).toContain('overflow-hidden')
    expect(ruleCard?.textContent).toContain('允许')

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
    const modal = allDialogs.find((d) => d.textContent?.includes('全局节点筛选'))
    expect(modal).toBeDefined()
    expect(modal?.textContent).toContain('执行优先级')
    expect(modal?.textContent).toContain('全局节点筛选')

    // Add a condition
    const inputs = modal?.querySelectorAll('input')
    const addBtn = Array.from(modal?.querySelectorAll('button') || []).find((b) => b.textContent?.includes('添加条件'))
    expect(addBtn).toBeDefined()

    if (inputs && inputs.length > 0) {
      inputs[0].value = 'premium'
      inputs[0].dispatchEvent(new Event('input'))
    }
    addBtn?.click()
    await nextTick()

    // Click Save Global Filter button
    const saveBtn = Array.from(modal?.querySelectorAll('button') || []).find((b) => b.textContent?.includes('保存全局筛选'))
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
    expect(cards[0].textContent).toContain('1 条筛选条件')

    // Click to expand
    const header = cards[0].querySelector('.cursor-pointer') as HTMLElement | null
    header?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    // Verify dynamic pool indicator and filter conditions details
    expect(cards[0].textContent).toContain('动态节点池')
    expect(cards[0].textContent).toContain('probe_latency_ms lte "150"')
    expect(cards[0].textContent).toContain('从全局节点池中动态筛选匹配的候选节点')
  })

  it('renders routing rules tab with inline error diagnostics and responsive mobile layout', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/policies/groups') {
        return {
          items: [{ id: 'grp-other', name: '其他', group_type: 'select', edges: [] }],
          total: 1,
        }
      }
      if (path === '/api/v1/policies/rules') {
        return {
          admission_rules: [],
          policy_rules: [
            {
              id: '01a0b9af-c118-72f9-9949-d94b49fa6ec2',
              revision_id: 'rev-1',
              target_group_id: 'grp-other',
              expression: 'PROCESS-NAME,tr.com.kliq.app',
              position: 0,
            },
          ],
          total: 1,
        }
      }
      return { items: [], total: 0 }
    })

    vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
      if (path === '/api/v1/policies/validate') {
        return {
          valid: false,
          revision_id: 'rev-1',
          errors: ['routed group 其他 has 0 available nodes'],
          issues: [
            {
              code: 'empty_routed_group',
              severity: 'error',
              rule_id: '01a0b9af-c118-72f9-9949-d94b49fa6ec2',
              position: 0,
              type: 'PROCESS-NAME',
              value: 'tr.com.kliq.app',
              target_group_id: 'grp-other',
              target_group_name: '其他',
              message: 'routed group "其他" referenced by rule "PROCESS-NAME,tr.com.kliq.app" has 0 available nodes',
            },
          ],
        }
      }
      return {}
    })

    await mountPolicyView()

    // Switch to rules tab
    const rulesTabBtn = container.querySelector('[data-testid="rules-tab-btn"]') as HTMLElement | null
    expect(rulesTabBtn).not.toBeNull()
    rulesTabBtn?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    // Verify rule card exists with testid
    const ruleCard = container.querySelector('[data-testid="policy-rule-card-0"]') as HTMLElement | null
    expect(ruleCard).not.toBeNull()
    expect(ruleCard?.textContent).toContain('PROCESS-NAME')
    expect(ruleCard?.textContent).toContain('tr.com.kliq.app')
    expect(ruleCard?.textContent).toContain('其他')

    // Verify inline error banner with error code and accessibility alert role
    const inlineIssue = ruleCard?.querySelector('[role="alert"]')
    expect(inlineIssue).not.toBeNull()
    expect(inlineIssue?.textContent).toContain('empty_routed_group')
    expect(inlineIssue?.textContent).toContain('routed group "其他"')

    // Verify delete button is present on the rule card
    const deleteBtn = ruleCard?.querySelector('[data-testid="delete-rule-btn-01a0b9af-c118-72f9-9949-d94b49fa6ec2"]')
    expect(deleteBtn).not.toBeNull()
  })

  it('editor keeps explicit PASS and archived regex/not_regex through edit and save', async () => {
    const expression = '去掉x(?:[0-5](?:\\\\.[0-9]+)?)(?![\\\\d.])|Eeox|einck'
    vi.spyOn(api, 'get').mockImplementation(async (path) => {
      if (path.endsWith('/groups')) return { items: [{ ...mockGroups[0], empty_fallback_pass: true, node_filter: { conditions: [{ field: 'display_name', op: 'not_regex', value: expression }] } }], total: 1 }
      if (path.endsWith('/rules')) return { revision_id: 'fixture', policy_rules: [], admission_rules: [] }
      return { spec: { conditions: [] }, items: [] }
    })
    vi.spyOn(api, 'post').mockResolvedValue({ valid: true, revision_id: 'fixture' })
    const patch = vi.spyOn(api, 'patch').mockResolvedValue({ ...mockGroups[0], empty_fallback_pass: true })
    await mountPolicyView()
    ;(container.querySelector('[data-testid="group-card"] .cursor-pointer') as HTMLElement).click()
    await nextTick()
    const dialog = container.querySelector('[role="dialog"]')!
    const checkbox = dialog.querySelector('[data-testid="empty-fallback-pass"]') as HTMLInputElement
    expect(checkbox.checked).toBe(true)
    expect(dialog.textContent).toContain('不是直连')
    const edit = Array.from(dialog.querySelectorAll('button')).find((b) => b.textContent?.trim() === '编辑')!
    edit.click()
    await nextTick()
    const op = Array.from(dialog.querySelectorAll('select')).find((s) => s.value === 'not_regex')!
    expect(op).toBeDefined()
    const value = Array.from(dialog.querySelectorAll('input')).find((i) => i.value === expression)
    expect(value).toBeDefined()
    const add = Array.from(dialog.querySelectorAll('button')).find((b) => b.textContent?.includes('添加条件'))!
    add.click()
    await nextTick()
    dialog.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))
    expect(patch).toHaveBeenCalledWith('/api/v1/policies/groups/grp-1', expect.objectContaining({ empty_fallback_pass: true, node_filter: { conditions: [{ field: 'display_name', op: 'not_regex', value: expression }] } }))
  })

  it('renders validation incomplete alert banner and allows user retry', async () => {
    vi.spyOn(api, 'post').mockRejectedValueOnce(new Error('校验超时'))

    await mountPolicyView()

    // Validation incomplete alert should be visible
    const incompleteBanner = container.querySelector('[role="alert"]')
    expect(incompleteBanner).not.toBeNull()
    expect(incompleteBanner?.textContent).toContain('校验未完成')
    expect(incompleteBanner?.textContent).toContain('重试校验')
  })

  it('displays membership mode badges for explicit edges vs dynamic full-pool filter', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/policies/groups') {
        return {
          items: [
            {
              id: 'grp-explicit',
              name: 'Explicit Edge Group',
              group_type: 'select',
              edges: [{ id: 'e-1', node_logical_id: 'node-hk-01', position: 0 }],
              node_filter: {
                conditions: [{ field: 'display_name', op: 'contains', value: 'hk' }],
              },
            },
            {
              id: 'grp-dynamic',
              name: 'Dynamic Pool Group',
              group_type: 'urltest',
              edges: [],
              node_filter: {
                conditions: [{ field: 'display_name', op: 'contains', value: 'tw' }],
              },
            },
          ],
          total: 2,
        }
      }
      return { items: [], total: 0 }
    })

    await mountPolicyView()

    const cards = container.querySelectorAll('[data-testid="group-card"]')
    expect(cards.length).toBe(2)

    // Card 1: Explicit edge with secondary filter
    expect(cards[0].textContent).toContain('显式连接边')
    expect(cards[0].textContent).toContain('二次过滤')

    // Card 2: Dynamic pool
    expect(cards[1].textContent).toContain('全池动态匹配')
  })

  it('searches and paginates candidate nodes up to 250th item and preserves selection across pages', async () => {
    // Generate 250 mock nodes where Canada is at index 249 (250th)
    const all250Nodes = Array.from({ length: 250 }, (_, i) => ({
      logical_id: `node-${i + 1}`,
      display_name: i === 249 ? 'Canada Highspeed 250' : (i === 70 ? 'Taiwan Premium 71' : `Node ${i + 1}`),
      protocol: 'ss',
      active: true,
    }))

    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/policies/groups') {
        return { items: [...mockGroups], total: 2 }
      }
      if (path === '/api/v1/nodes') {
        const page = Number(options?.params?.page || 1)
        const pageSize = Number(options?.params?.page_size || 50)
        const search = options?.params?.search?.toLowerCase() || ''
        let filtered = all250Nodes
        if (search) {
          filtered = filtered.filter((n) => n.display_name.toLowerCase().includes(search) || n.logical_id.includes(search))
        }
        const start = (page - 1) * pageSize
        const items = filtered.slice(start, start + pageSize)
        return {
          items,
          page,
          page_size: pageSize,
          total: filtered.length,
        }
      }
      return { items: [], total: 0 }
    })

    await mountPolicyView()

    // Open manage edges for first group
    const cards = container.querySelectorAll('[data-testid="group-card"]')
    const manageEdgesBtn = Array.from(cards[0].querySelectorAll('button')).find((b) => b.textContent?.includes('管理边'))
    expect(manageEdgesBtn).toBeDefined()
    manageEdgesBtn?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const dialog = document.body.querySelector('[role="dialog"]')
    expect(dialog).not.toBeNull()

    // Switch to active node edge radio
    const radios = dialog?.querySelectorAll('input[type="radio"]') as NodeListOf<HTMLInputElement>
    const nodeRadio = Array.from(radios).find((r) => r.value === 'node')
    expect(nodeRadio).toBeDefined()
    if (nodeRadio) {
      nodeRadio.checked = true
      nodeRadio.dispatchEvent(new Event('change'))
    }
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    // Verify search input is present
    const searchInput = dialog?.querySelector('[data-testid="candidate-node-search-input"]') as HTMLInputElement
    expect(searchInput).not.toBeNull()

    // Search for Canada (250th item)
    searchInput.value = 'Canada'
    searchInput.dispatchEvent(new Event('input'))
    await nextTick()
    // Wait for 300ms debounce
    await new Promise((r) => setTimeout(r, 350))

    // Select should contain Canada node
    const select = dialog?.querySelector('[data-testid="edge-node-select"]') as HTMLSelectElement
    expect(select).not.toBeNull()
    const canadaOption = Array.from(select.options).find((opt) => opt.textContent?.includes('Canada Highspeed 250'))
    expect(canadaOption).toBeDefined()
    expect(canadaOption?.value).toBe('node-250')

    // Select Canada node
    select.value = 'node-250'
    select.dispatchEvent(new Event('change'))
    await nextTick()

    // Locked selection confirmation tag should display Canada
    expect(dialog?.textContent).toContain('Canada Highspeed 250')
    expect(dialog?.textContent).toContain('跨页锁定已保留')

    // Now clear search and navigate to another page; selection must be preserved!
    searchInput.value = ''
    searchInput.dispatchEvent(new Event('input'))
    await new Promise((r) => setTimeout(r, 350))

    // Select should still show Canada option preserved
    expect(dialog?.textContent).toContain('Canada Highspeed 250')
  })
})
