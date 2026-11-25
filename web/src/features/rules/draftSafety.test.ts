// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import RuleEditorModal from './RuleEditorModal.vue'
import RuleCategoryModal from './RuleCategoryModal.vue'
import RulesView from './RulesView.vue'
import { useRules } from './useRules'
import { api } from '../../api/client'
import { setLocale, t } from '../../locales'
import type { PolicyRuleView } from './rulesTypes'

describe('Rule draft persistence boundary', () => {
  let app: ReturnType<typeof createApp> | null = null
  let container: HTMLDivElement
  beforeEach(() => {
    setLocale('zh-CN')
    container = document.createElement('div')
    document.body.appendChild(container)
    HTMLDialogElement.prototype.showModal = function () { this.open = true }
    HTMLDialogElement.prototype.close = function () { this.open = false }
  })
  afterEach(() => {
    app?.unmount()
    app = null
    container.remove()
    vi.restoreAllMocks()
  })
  const categories = [{ id: 'cat-one', name: 'Custom', sort_order: 10 }]

  it('keeps the editor dirty after submitting without a successful persistence result', async () => {
    const save = vi.fn()
    const close = vi.fn()
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    app = createApp({ render: () => h(RuleEditorModal, {
      modelValue: true, rule: null, categories, groups: [], saving: false,
      error: 'Server rejected this rule', onSave: save, 'onUpdate:modelValue': close,
    }) })
    app.mount(container)
    await nextTick()
    const name = container.querySelector('#rule-name-input') as HTMLInputElement
    name.value = 'My draft'
    name.dispatchEvent(new Event('input'))
    await nextTick()
    container.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
    await nextTick()
    expect(save).toHaveBeenCalledOnce()
    expect(container.querySelector('[data-testid="rule-save-error"]')?.textContent).toContain('Server rejected')
    const cancel = Array.from(container.querySelectorAll('button')).find(button => button.textContent?.trim() === '取消')!
    cancel.click()
    expect(confirm).toHaveBeenCalledOnce()
    expect(close).not.toHaveBeenCalled()
    expect(name.value).toBe('My draft')
  })

  it('does not clear a category name before the parent acknowledges success', async () => {
    const create = vi.fn()
    app = createApp({ render: () => h(RuleCategoryModal, { modelValue: true, categories, saving: false, onCreateCategory: create }) })
    app.mount(container)
    await nextTick()
    const input = container.querySelector('#new-category-name') as HTMLInputElement
    input.value = 'Unsaved category'
    input.dispatchEvent(new Event('input'))
    await nextTick()
    const save = Array.from(container.querySelectorAll('button')).find(button => button.textContent?.trim() === '保存')!
    save.click()
    await nextTick()
    expect(input.value).toBe('Unsaved category')
    const unload = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(unload)
    expect(unload.defaultPrevented).toBe(true)
  })

  it('disables rule inputs and close while the accepted save is pending', async () => {
    const pending = ref(false)
    const close = vi.fn()
    app = createApp({ render: () => h(RuleEditorModal, {
      modelValue: true, rule: null, categories, groups: [], saving: pending.value,
      'onUpdate:modelValue': close,
    }) })
    app.mount(container)
    await nextTick()
    const name = container.querySelector('#rule-name-input') as HTMLInputElement
    name.value = 'Submitted draft'
    name.dispatchEvent(new Event('input'))
    pending.value = true
    await nextTick()
    expect(name.matches(':disabled')).toBe(true)
    const cancel = Array.from(container.querySelectorAll('button')).find(button => button.textContent?.trim() === '取消')!
    cancel.click()
    expect(close).not.toHaveBeenCalled()
    pending.value = false
    await nextTick()
    expect(name.matches(':disabled')).toBe(false)
    expect(name.value).toBe('Submitted draft')
  })

  it('freezes category inputs during save and shows persistent failure inside the modal', async () => {
    app = createApp({ render: () => h(RuleCategoryModal, {
      modelValue: true, categories, saving: true, error: 'Category write rejected',
    }) })
    app.mount(container)
    await nextTick()
    expect(container.querySelector('#new-category-name')?.matches(':disabled')).toBe(true)
    expect(container.querySelector('#new-category-sort-order')?.matches(':disabled')).toBe(true)
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('Category write rejected')
  })

  it.each(['rule', 'category'] as const)('does not repeat an acknowledged %s create when refreshing fails', async kind => {
    const state = useRules()
    const created = kind === 'rule'
      ? { id: 'created-rule', expression: 'DOMAIN,example.test', enabled: true, position: 0, revision_id: 'revision-one' }
      : { id: 'created-category', name: 'Created category', sort_order: 10 }
    const post = vi.spyOn(api, 'post').mockResolvedValue(created)
    const get = vi.spyOn(api, 'get').mockRejectedValue(new Error('Read-back unavailable'))
    const create = () => kind === 'rule'
      ? state.createRule({ expression: 'DOMAIN,example.test', builtin_target: 'DIRECT' })
      : state.createCategory({ name: 'Created category', sort_order: 10 })

    expect(await create()).toEqual(created)
    expect(state.refreshRequired.value).toBe(true)
    expect(state.error.value).toContain('修改已保存')
    expect(state.error.value).toContain('Read-back unavailable')
    await expect(create()).rejects.toThrow('请先重新读取')
    expect(post).toHaveBeenCalledTimes(1)

    await expect(state.loadData()).rejects.toThrow('Read-back unavailable')
    expect(state.refreshRequired.value).toBe(true)
    expect(state.error.value).toContain('修改已保存')
    get.mockImplementation(async (path: string) => {
      if (path.endsWith('/categories')) return kind === 'category' ? [created] : []
      if (path.endsWith('/rules')) return { policy_rules: kind === 'rule' ? [created] : [], revision_id: 'revision-one', total: kind === 'rule' ? 1 : 0 }
      return { items: [], total: 0 }
    })
    await state.loadData()
    expect(state.refreshRequired.value).toBe(false)
    expect(state.error.value).toBeNull()
    expect(post).toHaveBeenCalledTimes(1)
    expect(kind === 'rule' ? state.rules.value : state.categories.value).toEqual([created])
  })

  it('closes the acknowledged rule create in the real parent and offers only a read retry', async () => {
    let failRulesRead = false
    const created: PolicyRuleView = { id: 'created-rule', expression: 'DOMAIN-SUFFIX,example.test', enabled: true, position: 0, revision_id: 'revision-one' }
    const get = vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path.endsWith('/categories')) return categories
      if (path.endsWith('/rules')) {
        if (failRulesRead) throw new Error('Rules read-back unavailable')
        return { policy_rules: [], revision_id: 'revision-one', total: 0 }
      }
      return { items: [], total: 0 }
    })
    const post = vi.spyOn(api, 'post').mockImplementation(async () => {
      failRulesRead = true
      return created
    })
    const settle = async () => {
      for (let i = 0; i < 12; i++) await Promise.resolve()
      await nextTick()
    }
    app = createApp(RulesView)
    app.mount(container)
    await settle()
    const newRule = Array.from(container.querySelectorAll('button')).find(button => button.textContent?.trim() === t('rules.newRule'))!
    newRule.click()
    await settle()
    const input = container.querySelector('#rule-expression-input') as HTMLInputElement
    input.value = 'example.test'
    input.dispatchEvent(new Event('input'))
    input.closest('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
    await settle()
    expect(post).toHaveBeenCalledTimes(1)
    expect(container.querySelector('dialog[open]')).toBeNull()
    expect(container.textContent).toContain('修改已保存')
    expect(newRule.disabled).toBe(true)
    newRule.click()
    expect(post).toHaveBeenCalledTimes(1)

    failRulesRead = false
    const reads = get.mock.calls.length
    const retry = Array.from(container.querySelectorAll('button')).find(button => button.textContent?.trim() === t('common.retry'))!
    retry.click()
    await settle()
    expect(get.mock.calls.length).toBeGreaterThan(reads)
    expect(post).toHaveBeenCalledTimes(1)
    expect(newRule.disabled).toBe(false)
  })

  it('retains an editable create after an actual write failure', async () => {
    const state = useRules()
    const post = vi.spyOn(api, 'post').mockRejectedValue(new Error('Write rejected'))
    const get = vi.spyOn(api, 'get')
    await expect(state.createCategory({ name: 'Unsaved category', sort_order: 10 })).rejects.toThrow('Write rejected')
    expect(state.refreshRequired.value).toBe(false)
    expect(state.saving.value).toBe(false)
    expect(post).toHaveBeenCalledOnce()
    expect(get).not.toHaveBeenCalled()
  })

  it('ignores a repeated toggle while the same rule is pending and retains state on failure', async () => {
    const state = useRules()
    const rule: PolicyRuleView = { id: 'rule-one', expression: 'DOMAIN,example.com', enabled: true, position: 0, revision_id: 'revision-one' }
    state.rules.value = [rule]
    let reject!: (reason: Error) => void
    const put = vi.spyOn(api, 'put').mockImplementationOnce(() => new Promise((_resolve, fail) => { reject = fail }))
    const pending = state.toggleRuleEnabled(rule).catch(error => error)
    await state.toggleRuleEnabled(rule)
    expect(put).toHaveBeenCalledOnce()
    expect(state.rules.value[0].enabled).toBe(true)
    reject(new Error('Save rejected'))
    expect((await pending).message).toBe('Save rejected')
    expect(state.rules.value[0].enabled).toBe(true)
    expect(state.busyRuleIds.value.size).toBe(0)
  })
})
