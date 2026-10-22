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
    await new Promise((resolve) => setTimeout(resolve, 50))
    await nextTick()
  }

  it('renders Rules Desk header, stats counters, and category list', async () => {
    await mountRulesView()

    expect(container.textContent).toContain('局域网直连')
    expect(container.textContent).toContain('人工智能')
    expect(container.textContent).toContain('兜底终结')

    // Check stats counters (3 categories and 3 rules)
    expect(container.textContent).toContain('3')
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
    expect(container.textContent).not.toContain('10.0.0.0/8')
  })

  it('validates rules topology when clicking validate button', async () => {
    await mountRulesView()

    // Validate rules button is inside toolbar or more dropdown
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

  it('applies whitespace-nowrap and natural height classes to rule type badges to prevent text wrapping', async () => {
    await mountRulesView()

    const badges = container.querySelectorAll('.badge')
    const typeBadges = Array.from(badges).filter((b) =>
      ['IP-CIDR', 'DOMAIN-SUFFIX', 'MATCH'].some((t) => b.textContent?.trim().includes(t))
    )
    expect(typeBadges.length).toBeGreaterThan(0)
    for (const badge of typeBadges) {
      expect(badge.className).toContain('whitespace-nowrap')
      expect(badge.className).toContain('h-auto')
    }
  })

  it('switches categories and updates visible rules using category navigation', async () => {
    await mountRulesView()

    const catAiNavBtn = Array.from(container.querySelectorAll('button')).find((el) =>
      el.textContent?.includes('人工智能')
    )
    expect(catAiNavBtn).toBeDefined()
    catAiNavBtn?.click()
    await nextTick()

    expect(container.textContent).toContain('openai.com')
    // LAN category rule should not be rendered in single category view
    expect(container.textContent).not.toContain('10.0.0.0/8')
  })

  it('performs cross-category search and displays category attribution badge', async () => {
    await mountRulesView()

    const searchInput = container.querySelector('input[type="text"]') as HTMLInputElement
    expect(searchInput).not.toBeNull()

    searchInput.value = 'openai'
    searchInput.dispatchEvent(new Event('input'))
    await nextTick()

    expect(container.textContent).toContain('openai.com')
    // Displays category attribution in search results
    expect(container.textContent).toContain('人工智能')

    // Clear search button restores full view
    const clearBtn = container.querySelector('button[aria-label="Clear search"]') as HTMLButtonElement
    expect(clearBtn).not.toBeNull()
    clearBtn.click()
    await nextTick()

    expect(container.textContent).toContain('10.0.0.0/8')
  })

  it('disables move up on terminal MATCH rule and displays terminal tail notice without duplicate virtual rules', async () => {
    await mountRulesView()

    // Find table row with MATCH expression
    const matchRow = Array.from(container.querySelectorAll('tr')).find(
      (el) => el.textContent?.includes('MATCH') && el.textContent?.includes('Final Match')
    )
    expect(matchRow).toBeDefined()

    // Move up button on terminal MATCH must be disabled
    const moveUpBtn = matchRow?.querySelector('button[title*="终结规则不可前移"], button[title*="Terminal rule cannot move up"]') as HTMLButtonElement
    expect(moveUpBtn).not.toBeNull()
    expect(moveUpBtn.disabled).toBe(true)

    // Tail notice is rendered in category tail
    expect(container.textContent).toContain('兜底终结规则')
  })

  it('guards unsaved changes in rule editor modal before discarding', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
    await mountRulesView()

    // Open new rule modal
    const newRuleBtn = Array.from(container.querySelectorAll('button')).find((btn) =>
      btn.textContent?.includes('新建规则') || btn.textContent?.includes('New Rule')
    )
    newRuleBtn?.click()
    await nextTick()

    // Find input inside modal and modify it
    const inputs = container.querySelectorAll('input[type="text"]')
    const exprInput = Array.from(inputs).find((inp) =>
      (inp as HTMLInputElement).placeholder?.includes('google.com')
    ) as HTMLInputElement
    expect(exprInput).toBeDefined()

    exprInput.value = 'custom-rule.example.com'
    exprInput.dispatchEvent(new Event('input'))
    await nextTick()

    // Click cancel button
    const cancelBtn = Array.from(container.querySelectorAll('button')).find(
      (btn) => btn.textContent?.trim() === '取消' || btn.textContent?.trim() === 'Cancel'
    )
    cancelBtn?.click()
    await nextTick()

    expect(confirmSpy).toHaveBeenCalled()
    confirmSpy.mockRestore()
  })

  it('applies modern DaisyUI 5 form controls with associated labels and without legacy bordered or label-text classes', async () => {
    await mountRulesView()

    // 1. RuleToolbar search input & RuleCategoryNav select
    const searchInput = container.querySelector('#rules-search-input') as HTMLInputElement
    expect(searchInput).not.toBeNull()
    const searchLabel = container.querySelector('label[for="rules-search-input"]')
    expect(searchLabel).not.toBeNull()

    const mobileSelect = container.querySelector('#rules-mobile-category-select') as HTMLSelectElement
    expect(mobileSelect).not.toBeNull()
    const mobileSelectLabel = container.querySelector('label[for="rules-mobile-category-select"]')
    expect(mobileSelectLabel).not.toBeNull()

    // 2. Open new rule modal
    const newRuleBtn = Array.from(container.querySelectorAll('button')).find((btn) =>
      btn.textContent?.includes('新建规则') || btn.textContent?.includes('New Rule')
    )
    newRuleBtn?.click()
    await nextTick()

    // Verify label-input associations in RuleEditorModal
    expect(container.querySelector('label[for="rule-category-select"]')).not.toBeNull()
    expect(container.querySelector('#rule-category-select')).not.toBeNull()
    expect(container.querySelector('label[for="rule-name-input"]')).not.toBeNull()
    expect(container.querySelector('#rule-name-input')).not.toBeNull()
    expect(container.querySelector('label[for="rule-type-select"]')).not.toBeNull()
    expect(container.querySelector('#rule-type-select')).not.toBeNull()
    expect(container.querySelector('label[for="rule-expression-input"]')).not.toBeNull()
    expect(container.querySelector('#rule-expression-input')).not.toBeNull()
    expect(container.querySelector('label[for="rule-no-resolve-field"]')).not.toBeNull()
    expect(container.querySelector('#rule-no-resolve-field')).not.toBeNull()

    // Check absence of legacy classes across all rendered DOM
    expect(container.querySelectorAll('.label-text').length).toBe(0)
    expect(container.querySelectorAll('.input-bordered').length).toBe(0)
    expect(container.querySelectorAll('.select-bordered').length).toBe(0)
    expect(container.querySelectorAll('.form-control').length).toBe(0)
  })

  it('renders category modal with DaisyUI 5 layout and associated labels', async () => {
    await mountRulesView()

    // Open manage categories modal
    const manageCatBtn = Array.from(container.querySelectorAll('button')).find((btn) =>
      btn.textContent?.includes('管理分类') || btn.textContent?.includes('Manage Categories')
    )
    manageCatBtn?.click()
    await nextTick()

    // Verify new category form label-input associations
    expect(container.querySelector('label[for="new-category-name"]')).not.toBeNull()
    expect(container.querySelector('#new-category-name')).not.toBeNull()
    expect(container.querySelector('label[for="new-category-sort-order"]')).not.toBeNull()
    expect(container.querySelector('#new-category-sort-order')).not.toBeNull()

    // Verify absence of legacy classes in category modal
    expect(container.querySelectorAll('.label-text').length).toBe(0)
    expect(container.querySelectorAll('.input-bordered').length).toBe(0)
    expect(container.querySelectorAll('.select-bordered').length).toBe(0)
    expect(container.querySelectorAll('.form-control').length).toBe(0)
  })
})
