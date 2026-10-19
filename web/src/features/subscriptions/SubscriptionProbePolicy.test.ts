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

describe('SubscriptionProbePolicy in SubscriptionConfigDrawer (US-R3)', () => {
  let root: HTMLDivElement
  let app: ReturnType<typeof createApp>
  let lastSavedDraft: SubscriptionDraft | null = null

  const find = <T extends HTMLElement>(id: string) => document.body.querySelector(`[data-testid="${id}"]`) as T
  const click = (id: string) => find<HTMLButtonElement>(id).click()

  const existingSub: SubscriptionRecord = {
    id: 'sub-test-1',
    name: 'Existing Subscription 1',
    source_url_secret_ref: 'https://example.com/sub1.yaml',
    enabled: true,
    prefix: '[HK]',
    refresh_policy: {
      interval_seconds: 86400,
      user_agent_policy: 'default',
      timeout_seconds: 30,
      max_response_bytes: 10485760,
    },
    config: {
      cron_schedule: '0 0 * * *',
      rename_rules: [{ pattern: 'HK-(.*)', replace: 'HongKong-$1' }],
      filter_rules: [{ type: 'include', pattern: 'HK|JP' }],
      target_groups: ['AUTO', 'FALLBACK'],
      probe_policy: {
        mode: 'inherit',
      },
    },
    revision: 'rev-001',
    created_at: '2026-10-19T10:00:00Z',
    updated_at: '2026-10-19T10:00:00Z',
  }

  function mountDrawer(subscription?: SubscriptionRecord) {
    lastSavedDraft = null
    root = document.createElement('div')
    document.body.append(root)

    app = createApp({
      render: () =>
        h(SubscriptionConfigDrawer, {
          modelValue: true,
          subscription,
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
    root.remove()
    vi.restoreAllMocks()
  })

  it('initializes with inherit mode by default and does not submit interval_seconds for inherit', async () => {
    mountDrawer(existingSub)
    await flush()

    // Switch to advanced tab
    const advancedTabBtn = Array.from(document.body.querySelectorAll('.tab')).find((b) =>
      b.textContent?.includes('高级') || b.textContent?.includes('Advanced')
    ) as HTMLButtonElement | null
    advancedTabBtn?.click()
    await flush()

    const inheritRadio = find<HTMLInputElement>('probe-policy-mode-inherit')
    expect(inheritRadio.checked).toBe(true)

    // Custom interval input block must NOT be visible
    expect(find('probe-policy-custom-interval-block')).toBeNull()

    // Save drawer
    click('subscription-drawer-save-btn')
    await flush()

    expect(lastSavedDraft).not.toBeNull()
    expect(lastSavedDraft?.config.probe_policy).toEqual({
      mode: 'inherit',
    })
    expect(lastSavedDraft?.config.probe_policy?.interval_seconds).toBeUndefined()
    // Preserves rename rules, filter rules, target groups
    expect(lastSavedDraft?.config.rename_rules).toHaveLength(1)
    expect(lastSavedDraft?.config.filter_rules).toHaveLength(1)
    expect(lastSavedDraft?.config.target_groups).toContain('AUTO')
  })

  it('switches to disabled mode and submits probe_policy mode=disabled without interval', async () => {
    mountDrawer(existingSub)
    await flush()

    const advancedTabBtn = Array.from(document.body.querySelectorAll('.tab')).find((b) =>
      b.textContent?.includes('高级') || b.textContent?.includes('Advanced')
    ) as HTMLButtonElement | null
    advancedTabBtn?.click()
    await flush()

    const disabledRadio = find<HTMLInputElement>('probe-policy-mode-disabled')
    disabledRadio.click()
    await flush()
    expect(disabledRadio.checked).toBe(true)

    click('subscription-drawer-save-btn')
    await flush()

    expect(lastSavedDraft).not.toBeNull()
    expect(lastSavedDraft?.config.probe_policy).toEqual({
      mode: 'disabled',
    })
    expect(lastSavedDraft?.config.probe_policy?.interval_seconds).toBeUndefined()
  })

  it('switches to custom mode, validates interval range (60..604800), and submits custom interval', async () => {
    mountDrawer(existingSub)
    await flush()

    const advancedTabBtn = Array.from(document.body.querySelectorAll('.tab')).find((b) =>
      b.textContent?.includes('高级') || b.textContent?.includes('Advanced')
    ) as HTMLButtonElement | null
    advancedTabBtn?.click()
    await flush()

    // 1. Select custom mode
    const customRadio = find<HTMLInputElement>('probe-policy-mode-custom')
    customRadio.click()
    await flush()

    // Custom interval input should now appear
    const intervalInput = find<HTMLInputElement>('probe-policy-interval-input')
    expect(intervalInput).not.toBeNull()

    // 2. Test invalid interval (< 60)
    intervalInput.value = '45'
    intervalInput.dispatchEvent(new Event('input'))

    click('subscription-drawer-save-btn')
    await flush()

    // Submit should be rejected
    expect(lastSavedDraft).toBeNull()
    const errorAlert = find('subscription-drawer-error')
    expect(errorAlert.textContent).toContain('60 到 604800 秒')

    // 3. Test valid interval (300 seconds)
    intervalInput.value = '300'
    intervalInput.dispatchEvent(new Event('input'))
    click('subscription-drawer-save-btn')
    await flush()

    expect(lastSavedDraft).not.toBeNull()
    expect(lastSavedDraft?.config.probe_policy).toEqual({
      mode: 'custom',
      interval_seconds: 300,
    })
  })

  it('guarantees legacy auto_test control is completely removed from UI', async () => {
    mountDrawer(existingSub)
    await flush()

    const advancedTabBtn = Array.from(root.querySelectorAll('.tab')).find((b) =>
      b.textContent?.includes('高级') || b.textContent?.includes('Advanced')
    ) as HTMLButtonElement | null
    advancedTabBtn?.click()
    await flush()

    // Check that auto_test text/toggle is nowhere in the drawer
    expect(root.textContent).not.toContain('拉取后自动触发全域探针')
    expect(root.textContent).not.toContain('Trigger Probes Automatically After Ingest')
    expect(root.querySelector('input[type="checkbox"][name="auto_test"]')).toBeNull()
  })
})
