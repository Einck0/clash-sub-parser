// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../../api/client'
import { createApp, h, nextTick } from 'vue'
import StructuredFilterEditor from './StructuredFilterEditor.vue'
import { setLocale, t } from '../../locales'
import type { FilterCondition } from './policyTypes'

const flush = async () => {
  await nextTick()
  await Promise.resolve()
  await nextTick()
}

describe('StructuredFilterEditor real-locale i18n label verification', () => {
  let root: HTMLDivElement
  let app: ReturnType<typeof createApp> | undefined

  const mountEditor = (props: Record<string, any> = {}) => {
    app = createApp({
      render: () => h(StructuredFilterEditor, { ...props }),
    })
    app.mount(root)
  }

  beforeEach(() => {
    setLocale('zh-CN')
    root = document.createElement('div')
    document.body.append(root)
  })

  afterEach(() => {
    app?.unmount()
    app = undefined
    root.remove()
    setLocale('zh-CN')
    vi.restoreAllMocks()
  })

  it('loads subscription names for asynchronously supplied conditions and persists only the selected ID', async () => {
    const { ref } = await import('vue')
    const rows = ref<FilterCondition[]>([])
    const first = '019520c3-8268-7000-8000-000000000001'
    const second = '019520c3-8268-7000-8000-000000000002'
    const get = vi.spyOn(api, 'get').mockResolvedValue({ items: [{ id: first, name: '香港订阅' }, { id: second, name: '日本订阅' }], total: 2 })
    const update = vi.fn()
    app = createApp({ render: () => h(StructuredFilterEditor, { modelValue: rows.value, 'onUpdate:modelValue': update }) })
    app.mount(root)
    await flush()
    rows.value = [{ field: 'source_subscription_ids', op: 'contains', value: first }]
    await flush(); await flush()
    expect(get).toHaveBeenCalledWith('/api/v1/subscriptions', { params: { page: 1, page_size: 100 } })
    const select = root.querySelector<HTMLSelectElement>('[data-testid="condition-value"]')!
    expect(select.tagName).toBe('SELECT')
    expect(select.textContent).toContain('香港订阅')
    expect(select.textContent).not.toContain(first)
    select.value = second
    select.dispatchEvent(new Event('change', { bubbles: true }))
    await flush()
    expect(update).toHaveBeenLastCalledWith([{ field: 'source_subscription_ids', op: 'contains', value: second }])
  })

  it('retains unavailable saved subscription values without exposing IDs and offers a load retry', async () => {
    const get = vi.spyOn(api, 'get').mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ items: [], total: 0 })
    mountEditor({ modelValue: [{ field: 'source_subscription_ids', op: 'not_contains', value: 'sub_private' }] })
    await flush(); await flush()
    expect(root.textContent).not.toContain('sub_private')
    expect(root.textContent).toContain('已保存的订阅')
    root.querySelector<HTMLButtonElement>('[data-testid="subscription-filter-error"] button')!.click()
    await flush(); await flush()
    expect(get).toHaveBeenCalledTimes(2)
    expect(root.querySelector('[data-testid="subscription-filter-error"]')).toBeNull()
  })

  it('renders Chinese localized labels for health_status, exit_region, and platform_verdict without raw keys', async () => {
    setLocale('zh-CN')
    const conditions: FilterCondition[] = [
      { field: 'health_status', op: 'equals', value: 'healthy' },
      { field: 'exit_region', op: 'equals', value: 'HK' },
      { field: 'platform_verdict', op: 'equals', value: 'available', probe_kind: 'ai', platform: 'openai' },
    ]
    mountEditor({ modelValue: conditions })
    await flush()

    const fieldSelects = root.querySelectorAll<HTMLSelectElement>('[data-testid="condition-field"]')
    expect(fieldSelects).toHaveLength(3)

    // Verify first row options
    const firstOptions = Array.from(fieldSelects[0]!.options)
    const optionMap = Object.fromEntries(firstOptions.map((o) => [o.value, o.textContent?.trim()]))

    expect(optionMap['health_status']).toBe('健康状态')
    expect(optionMap['exit_region']).toBe('实际出口地区')
    expect(optionMap['platform_verdict']).toBe('平台判定')
    expect(optionMap['display_name']).toBe('显示名称')
    expect(optionMap['protocol']).toBe('协议类型')
    expect(optionMap['source_subscription_ids']).toBe('来源订阅')
    expect(optionMap['probe_latency_ms']).toBe('探针延迟 (ms)')

    // Crucial check: No raw settings. keys leaked into DOM text or attributes
    expect(root.innerHTML).not.toContain('settings.filterField_')
    expect(root.innerHTML).not.toContain('settings.filter')
  })

  it('renders English localized labels for health_status, exit_region, and platform_verdict in en-US', async () => {
    setLocale('en-US')
    const conditions: FilterCondition[] = [
      { field: 'health_status', op: 'equals', value: 'healthy' },
      { field: 'exit_region', op: 'equals', value: 'HK' },
      { field: 'platform_verdict', op: 'equals', value: 'available', probe_kind: 'ai', platform: 'openai' },
    ]
    mountEditor({ modelValue: conditions })
    await flush()

    const fieldSelects = root.querySelectorAll<HTMLSelectElement>('[data-testid="condition-field"]')
    expect(fieldSelects).toHaveLength(3)

    const firstOptions = Array.from(fieldSelects[0]!.options)
    const optionMap = Object.fromEntries(firstOptions.map((o) => [o.value, o.textContent?.trim()]))

    expect(optionMap['health_status']).toBe('Health status')
    expect(optionMap['exit_region']).toBe('Measured exit region')
    expect(optionMap['platform_verdict']).toBe('Platform verdict')
    expect(optionMap['display_name']).toBe('Display name')
    expect(optionMap['protocol']).toBe('Protocol')
    expect(optionMap['source_subscription_ids']).toBe('Source subscription')
    expect(optionMap['probe_latency_ms']).toBe('Probe latency (ms)')

    // Crucial check: No raw settings. keys leaked into DOM text or attributes
    expect(root.innerHTML).not.toContain('settings.filterField_')
    expect(root.innerHTML).not.toContain('settings.filter')
  })

  it('edits, emits and clears risk conditions with string score values for persistence', async () => {
    let emitted: FilterCondition[] = []
    mountEditor({
      modelValue: [{ field: 'ip_risk_band', op: 'equals', value: 'unknown' }],
      'onUpdate:modelValue': (value: FilterCondition[]) => { emitted = value },
    })
    await flush()
    const field = root.querySelector<HTMLSelectElement>('[data-testid="condition-field"]')!
    expect(Array.from(field.options).map(o => o.value)).toContain('ip_risk_score')
    expect(root.querySelector<HTMLSelectElement>('[data-testid="condition-value"]')!.value).toBe('unknown')
    field.value = 'ip_risk_score'
    field.dispatchEvent(new Event('change', { bubbles: true }))
    await flush()
    const score = root.querySelector<HTMLInputElement>('[data-testid="condition-value"]')!
    score.value = '0'
    score.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()
    expect(emitted).toEqual([{ field: 'ip_risk_score', op: 'lte', value: '0' }])
    root.querySelector<HTMLButtonElement>('[data-testid="filter-clear"]')!.click()
    await flush()
    expect(emitted).toEqual([])
  })

  it('offers antigravity AI platform and abnormal/skipped verdicts in the editor', async () => {
    const conditions: FilterCondition[] = [
      { field: 'platform_verdict', op: 'equals', value: 'abnormal', probe_kind: 'ai', platform: 'antigravity' },
      { field: 'platform_verdict', op: 'equals', value: 'skipped', probe_kind: 'ai', platform: 'antigravity' },
    ]
    mountEditor({ modelValue: conditions })
    await flush()

    const rows = root.querySelectorAll<HTMLElement>('[data-testid="filter-condition"]')
    expect(rows).toHaveLength(2)

    for (const [index, verdict] of ['abnormal', 'skipped'].entries()) {
      const row = rows[index]!
      const selects = Array.from(row.querySelectorAll<HTMLSelectElement>('select'))
      const platformSelect = selects.find((select) => Array.from(select.options).some((option) => option.value === 'antigravity'))
      const verdictSelect = row.querySelector<HTMLSelectElement>('[data-testid="condition-value"]')

      expect(platformSelect).toBeDefined()
      expect(Array.from(platformSelect!.options).map((option) => option.value)).toContain('antigravity')
      expect(platformSelect!.value).toBe('antigravity')
      expect(Array.from(verdictSelect!.options).map((option) => option.value)).toContain(verdict)
      expect(verdictSelect!.value).toBe(verdict)
    }
  })

  it('does not render probe freshness input or obsolete freshness text under probe_verdict and probe_latency_ms', async () => {
    setLocale('zh-CN')
    const conditions: FilterCondition[] = [
      { field: 'probe_verdict', op: 'equals', value: 'available', probe_kind: 'baseline', freshness_seconds: 3600 },
      { field: 'probe_latency_ms', op: 'lte', value: '200', probe_kind: 'baseline', freshness_seconds: 7200 },
    ]
    mountEditor({ modelValue: conditions })
    await flush()

    // No freshness number inputs rendered in DOM
    const numberInputs = root.querySelectorAll('input[type="number"]')
    expect(numberInputs).toHaveLength(0)

    // Verify DOM contains neither old freshness labels nor 86400/24h/freshness texts
    expect(root.innerHTML).not.toContain('最大有效期')
    expect(root.innerHTML).not.toContain('Maximum freshness')
    expect(root.innerHTML).not.toContain('86400')
    expect(root.innerHTML).not.toContain('24 小时')
    expect(root.innerHTML).not.toContain('24 hours')
    expect(root.innerHTML).not.toContain('filterFreshness')
    expect(root.innerHTML).not.toContain('filterDefaultFreshness')
    expect(root.innerHTML).not.toContain('settings.')
  })

  it('validates filterExplanation semantics in both locales', () => {
    setLocale('zh-CN')
    const zhExp = t('settings.filterExplanation')
    expect(zhExp).toContain('当前连接版本')
    expect(zhExp).toContain('最新有效检测')
    expect(zhExp).toContain('未知地区或平台判定对等于与不等于（neq）均不匹配')
    expect(zhExp).toContain('时间不自然失效')
    expect(zhExp).toContain('当前预览与实时导出即时更新')
    expect(zhExp).toContain('WARP 仅享软规则豁免')
    expect(zhExp).toContain('硬准入仍始终生效')
    expect(zhExp).toContain('queued/probing 是任务瞬态，不是可保存的判定')
    expect(zhExp).not.toContain('24 小时')
    expect(zhExp).not.toContain('86400')
    expect(zhExp).not.toContain('未核对节点当前凭据版本')

    setLocale('en-US')
    const enExp = t('settings.filterExplanation')
    expect(enExp).toContain('current connection revision')
    expect(enExp).toContain('latest valid check')
    expect(enExp).toContain('matches neither equals nor not_equals (neq)')
    expect(enExp).toContain('do not naturally expire over time')
    expect(enExp).toContain('WARP receives soft filter exemption only')
    expect(enExp).toContain('queued/probing are transient task states, not persisted verdicts')
    expect(enExp).not.toContain('24 hours')
    expect(enExp).not.toContain('86400')
  })
})
