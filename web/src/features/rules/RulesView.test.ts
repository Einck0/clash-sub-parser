// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import RulesView from './RulesView.vue'
import { api } from '../../api/client'
import type { RuleCategory, PolicyRuleView } from './rulesTypes'

describe('RulesView Component', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  const mockCategories: RuleCategory[] = [
    { id: 'cat-lan', name: '局域网直连', sort_order: 10 },
    { id: 'cat-ai', name: '人工智能', sort_order: 20 },
    { id: 'cat-match', name: '兜底终结', sort_order: 1000 },
  ]

  const mockRules: PolicyRuleView[] = [
    {
      id: 'r-1',
      revision_id: 'rev-test',
      category_id: 'cat-lan',
      category: '局域网直连',
      name: 'Local Private IPs',
      builtin_target: 'DIRECT',
      expression: 'IP-CIDR,10.0.0.0/8,no-resolve',
      position: 1,
      enabled: true,
    },
    {
      id: 'r-2',
      revision_id: 'rev-test',
      category_id: 'cat-ai',
      category: '人工智能',
      name: 'OpenAI Domain',
      target_group_id: 'grp-ai',
      expression: 'DOMAIN-SUFFIX,openai.com',
      position: 2,
      enabled: true,
    },
    {
      id: 'r-3',
      revision_id: 'rev-test',
      category_id: 'cat-match',
      category: '兜底终结',
      name: 'Final Match',
      builtin_target: 'DIRECT',
      expression: 'MATCH',
      position: 3,
      enabled: true,
    },
  ]

  const mockGroups = [{ id: 'grp-ai', name: 'AI Services', group_type: 'select' }]

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
      if (path === '/api/v1/policies/categories') {
        return [...mockCategories]
      }
      if (path === '/api/v1/policies/rules') {
        return {
          revision_id: 'rev-test',
          policy_rules: [...mockRules],
          total: mockRules.length,
        }
      }
      if (path === '/api/v1/policies/groups') {
        return { items: [...mockGroups] }
      }
      if (path === '/api/v1/policies/validate') {
        return { valid: true, revision_id: 'rev-test', issues: [] }
      }
      return []
    })

    vi.spyOn(api, 'post').mockImplementation(async (_path: string, body?: any) => {
      return body || {}
    })
    vi.spyOn(api, 'put').mockImplementation(async (_path: string, body?: any) => {
      return body || {}
    })
    vi.spyOn(api, 'delete').mockImplementation(async () => {
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

  async function mountRulesView() {
    app = createApp({
      render() {
        return h(RulesView)
      },
    })
    app.mount(container)
    await nextTick()
    // Wait for async loadData
    await new Promise((resolve) => setTimeout(resolve, 50))
    await nextTick()
  }

  it('renders Rules Desk header, stats counters, and category list', async () => {
    await mountRulesView()

    expect(container.textContent).toContain('局域网直连')
    expect(container.textContent).toContain('人工智能')
    expect(container.textContent).toContain('兜底终结')

    // Check stats
    expect(container.textContent).toContain('3') // 3 categories and 3 rules
  })

  it('displays DIRECT builtin badge and Policy Group badge', async () => {
    await mountRulesView()

    // DIRECT badge
    expect(container.textContent).toContain('DIRECT')
    // Policy Group name
    expect(container.textContent).toContain('AI Services')
    // Expressions
    expect(container.textContent).toContain('10.0.0.0/8')
    expect(container.textContent).toContain('openai.com')
    expect(container.textContent).toContain('MATCH')
  })

  it('opens new rule modal upon clicking new rule button', async () => {
    await mountRulesView()

    const newRuleBtn = Array.from(container.querySelectorAll('button')).find((btn) =>
      btn.textContent?.includes('新建规则') || btn.textContent?.includes('New Rule')
    )
    expect(newRuleBtn).toBeDefined()
    newRuleBtn?.click()
    await nextTick()

    // Dialog should be displayed with form inputs
    const dialogs = container.querySelectorAll('dialog')
    expect(dialogs.length).toBeGreaterThan(0)
  })

  it('filters rules dynamically when typing in search input', async () => {
    await mountRulesView()

    const searchInput = container.querySelector('input[type="text"]') as HTMLInputElement
    expect(searchInput).not.toBeNull()

    searchInput.value = 'openai'
    searchInput.dispatchEvent(new Event('input'))
    await nextTick()

    expect(container.textContent).toContain('openai.com')
    // LAN rule should not be present in filtered list
    expect(container.textContent).not.toContain('10.0.0.0/8')
  })

  it('validates rules topology when clicking validate button', async () => {
    await mountRulesView()

    const validateBtn = Array.from(container.querySelectorAll('button')).find((btn) =>
      btn.textContent?.includes('校验规则拓扑') || btn.textContent?.includes('Validate Rules')
    )
    expect(validateBtn).toBeDefined()
    validateBtn?.click()
    await nextTick()
    await new Promise((resolve) => setTimeout(resolve, 50))
    await nextTick()

    expect(api.get).toHaveBeenCalledWith('/api/v1/policies/validate')
  })

  it('applies mobile safe bottom padding and responsive non-truncated category card headers', async () => {
    await mountRulesView()

    // Verify container padding includes pb-24 md:pb-8
    const rootEl = container.firstElementChild as HTMLElement
    expect(rootEl).not.toBeNull()
    expect(rootEl.className).toContain('pb-24')
    expect(rootEl.className).toContain('md:pb-8')

    // Verify category header flex-wrap layout and title without truncate class
    const categoryHeaderLeft = container.querySelector('.card .cursor-pointer > div') as HTMLElement
    expect(categoryHeaderLeft).not.toBeNull()
    expect(categoryHeaderLeft.className).toContain('flex')
    expect(categoryHeaderLeft.className).toContain('flex-wrap')
    expect(categoryHeaderLeft.className).toContain('gap-2')

    const categoryTitle = categoryHeaderLeft.querySelector('span.font-bold') as HTMLElement
    expect(categoryTitle).not.toBeNull()
    expect(categoryTitle.className).not.toContain('truncate')
  })
})
