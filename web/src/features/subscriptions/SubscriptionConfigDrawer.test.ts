// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import SubscriptionConfigDrawer from './SubscriptionConfigDrawer.vue'
import type { SubscriptionDraft, SubscriptionRecord } from './useSubscriptions'
import { i18n } from '../../locales'

const flush = async () => {
  await nextTick()
  await new Promise((r) => setTimeout(r, 10))
  await nextTick()
}

describe('SubscriptionConfigDrawer DaisyUI 5 & Modern Contract Tests', () => {
  let root: HTMLDivElement
  let app: ReturnType<typeof createApp>
  let lastSavedDraft: SubscriptionDraft | null = null
  let updateModelEvents: boolean[] = []

  const sampleSub: SubscriptionRecord = {
    id: 'sub-test-drawer',
    name: 'HK Direct Premium',
    source_url_secret_ref: 'https://hk.node-provider.com/feed.yaml',
    enabled: true,
    prefix: '[HK-VIP] ',
    refresh_policy: {
      interval_seconds: 7200,
      user_agent_policy: 'default',
      timeout_seconds: 25,
      max_response_bytes: 5242880,
    },
    config: {
      cron_schedule: '0 */4 * * *',
      rename_rules: [{ pattern: 'HK-(.*)', replace: 'HongKong-$1' }],
      filter_rules: [{ type: 'include', pattern: 'HK' }],
      target_groups: ['HK-GRP', 'FALLBACK'],
      probe_policy: {
        mode: 'inherit',
      },
    },
    revision: 'rev-001',
    created_at: '2026-10-20T10:00:00Z',
    updated_at: '2026-10-20T10:00:00Z',
  }

  function mountDrawer(props: {
    modelValue?: boolean
    subscription?: SubscriptionRecord
    saving?: boolean
  } = {}) {
    lastSavedDraft = null
    updateModelEvents = []
    root = document.createElement('div')
    document.body.append(root)

    app = createApp({
      render: () =>
        h(SubscriptionConfigDrawer, {
          modelValue: props.modelValue ?? true,
          subscription: props.subscription,
          saving: props.saving,
          'onUpdate:modelValue': (val: boolean) => {
            updateModelEvents.push(val)
          },
          onSave: (draft: SubscriptionDraft) => {
            lastSavedDraft = draft
          },
        }),
    })
    app.use(i18n)
    app.mount(root)
  }

  afterEach(() => {
    if (app) {
      app.unmount()
    }
    root?.remove()
    vi.restoreAllMocks()
  })

  it('strictly adheres to DaisyUI 5 and contains zero legacy DaisyUI 4 classes', async () => {
    mountDrawer({ subscription: sampleSub })
    await flush()

    // 1. Verify no deprecated form-control / label-text / bordered classes anywhere in the drawer
    const drawerHtml = document.body.innerHTML
    expect(drawerHtml).not.toContain('form-control')
    expect(drawerHtml).not.toContain('label-text')
    expect(drawerHtml).not.toContain('input-bordered')
    expect(drawerHtml).not.toContain('textarea-bordered')
    expect(drawerHtml).not.toContain('select-bordered')
    expect(drawerHtml).not.toContain('tabs-boxed')

    // 2. Verify modern DaisyUI 5 classes are present
    const tabsBox = document.body.querySelector('.tabs.tabs-box')
    expect(tabsBox).not.toBeNull()

    const nameInput = document.body.querySelector<HTMLInputElement>('[data-testid="subscription-name-input"]')
    expect(nameInput).not.toBeNull()
    expect(nameInput?.className).toContain('input')
    expect(nameInput?.className).toContain('input-sm')
    expect(nameInput?.id).toBe('sub-name-input')

    // 3. Verify semantic label for attribute matches input id
    const nameLabel = document.body.querySelector('label[for="sub-name-input"]')
    expect(nameLabel).not.toBeNull()
  })

  it('preserves clean drawer close without triggering unsaved changes prompt', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm')
    mountDrawer({ subscription: sampleSub })
    await flush()

    const cancelBtn = Array.from(document.body.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('取消') || b.textContent?.includes('Cancel')
    ) as HTMLButtonElement | null
    expect(cancelBtn).not.toBeNull()
    cancelBtn?.click()
    await flush()

    expect(confirmSpy).not.toHaveBeenCalled()
    expect(updateModelEvents).toEqual([false])
  })

  it('unsaved guard blocks closing when user cancels prompt, and closes when confirmed', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
    mountDrawer({ subscription: sampleSub })
    await flush()

    // Make drawer dirty
    const nameInput = document.body.querySelector<HTMLInputElement>('[data-testid="subscription-name-input"]')!
    nameInput.value = 'Modified HK Sub Name'
    nameInput.dispatchEvent(new Event('input'))
    await flush()

    const cancelBtn = Array.from(document.body.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('取消') || b.textContent?.includes('Cancel')
    ) as HTMLButtonElement | null

    // Attempt 1: Cancel discard prompt
    cancelBtn?.click()
    await flush()

    expect(confirmSpy).toHaveBeenCalledTimes(1)
    expect(updateModelEvents).toEqual([]) // Must NOT emit update:modelValue(false)

    // Attempt 2: Confirm discard prompt
    confirmSpy.mockReturnValue(true)
    cancelBtn?.click()
    await flush()

    expect(confirmSpy).toHaveBeenCalledTimes(2)
    expect(updateModelEvents).toEqual([false])
  })

  it('validates required fields, switches to basic tab on error, and saves with high fidelity', async () => {
    mountDrawer()
    await flush()

    // Switch to advanced tab
    const advancedTabBtn = Array.from(document.body.querySelectorAll('.tab')).find((b) =>
      b.textContent?.includes('高级') || b.textContent?.includes('Advanced')
    ) as HTMLButtonElement | null
    advancedTabBtn?.click()
    await flush()

    // Click save with empty fields
    const saveBtn = document.body.querySelector<HTMLButtonElement>('[data-testid="subscription-drawer-save-btn"]')!
    saveBtn.click()
    await flush()

    // Must be blocked and switched back to basic tab
    expect(lastSavedDraft).toBeNull()
    const basicTab = Array.from(document.body.querySelectorAll('.tab'))[0]
    expect(basicTab.className).toContain('tab-active')

    // Error messages visible
    const nameError = document.body.querySelector('[data-testid="subscription-name-error"]')
    expect(nameError?.textContent).toContain('订阅源名称不能为空')

    // Fill valid basic inputs
    const nameInput = document.body.querySelector<HTMLInputElement>('[data-testid="subscription-name-input"]')!
    const urlInput = document.body.querySelector<HTMLInputElement>('[data-testid="subscription-url-input"]')!
    nameInput.value = 'My Custom Provider'
    nameInput.dispatchEvent(new Event('input'))
    urlInput.value = 'https://provider.example.com/clash.yaml'
    urlInput.dispatchEvent(new Event('input'))
    await flush()

    // Save should now succeed
    saveBtn.click()
    await flush()

    expect(lastSavedDraft).not.toBeNull()
    expect(lastSavedDraft?.name).toBe('My Custom Provider')
    expect(lastSavedDraft?.source_url_secret_ref).toBe('https://provider.example.com/clash.yaml')
    expect(lastSavedDraft?.enabled).toBe(true)
  })

  it('manages rename rules and filter rules dynamically with data preservation', async () => {
    mountDrawer({ subscription: sampleSub })
    await flush()

    // Switch to advanced tab
    const advancedTabBtn = Array.from(document.body.querySelectorAll('.tab')).find((b) =>
      b.textContent?.includes('高级') || b.textContent?.includes('Advanced')
    ) as HTMLButtonElement | null
    advancedTabBtn?.click()
    await flush()

    // Verify initial rules from sampleSub
    const renameInputs = Array.from(document.body.querySelectorAll<HTMLInputElement>('input[placeholder="匹配正则"]'))
    expect(renameInputs.length).toBeGreaterThanOrEqual(1)
    expect(renameInputs[0].value).toBe('HK-(.*)')

    // Add a new rename rule
    const addRenameBtn = Array.from(document.body.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('添加重命名规则') || b.textContent?.includes('Add Rename Rule')
    ) as HTMLButtonElement | null
    expect(addRenameBtn).not.toBeNull()
    addRenameBtn?.click()
    await flush()

    const updatedRenameInputs = Array.from(document.body.querySelectorAll<HTMLInputElement>('input[placeholder="匹配正则"]'))
    expect(updatedRenameInputs.length).toBe(renameInputs.length + 1)

    // Save and verify draft contains both rules
    const saveBtn = document.body.querySelector<HTMLButtonElement>('[data-testid="subscription-drawer-save-btn"]')!
    saveBtn.click()
    await flush()

    expect(lastSavedDraft?.config.rename_rules?.length).toBe(2)
  })
})
