// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import GlobalNodeFilterSettings from './GlobalNodeFilterSettings.vue'
import { api, ApiError } from '../../api/client'
import { setLocale } from '../../locales'
import PolicyView from '../policy/PolicyView.vue'

const endpoint = '/api/v1/policies/global-node-filter'
const baseline = { spec: { conditions: [{ field: 'probe_verdict', op: 'equals', value: 'available', probe_kind: 'baseline', freshness_seconds: 3600 }] }, updated_at: '2026-01-01T00:00:00Z' }
const flush = async () => { await nextTick(); await Promise.resolve(); await nextTick() }

describe('global export filter settings', () => {
  let root: HTMLDivElement
  let app: ReturnType<typeof createApp> | undefined
  const find = (id: string) => root.querySelector(`[data-testid="${id}"]`) as HTMLElement | null
  const button = (id: string) => find(id) as HTMLButtonElement
  const conditions = () => [...root.querySelectorAll('[data-testid="filter-condition"]')]
  const input = (element: HTMLInputElement | HTMLSelectElement, value: string) => {
    element.value = value
    element.dispatchEvent(new Event(element instanceof HTMLSelectElement ? 'change' : 'input', { bubbles: true }))
  }
  const mount = (view = GlobalNodeFilterSettings) => {
    app = createApp({ render: () => h(view) })
    app.mount(root)
  }

  beforeEach(() => {
    vi.restoreAllMocks()
    HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', '') }
    HTMLDialogElement.prototype.close = function () { this.removeAttribute('open') }
    setLocale('zh-CN')
    root = document.createElement('div')
    document.body.append(root)
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => path === endpoint ? structuredClone(baseline) : {})
    vi.spyOn(api, 'put').mockImplementation(async (_path, payload: any) => ({ ...payload, updated_at: 'now' }))
  })
  afterEach(() => { app?.unmount(); app = undefined; root.remove(); setLocale('zh-CN'); vi.restoreAllMocks() })

  it('loads persisted probe kind/verdict/freshness and keeps the same GET endpoint on remount', async () => {
    mount()
    await flush()
    expect(api.get).toHaveBeenCalledWith(endpoint)
    expect(conditions()).toHaveLength(1)
    expect([...conditions()[0]!.querySelectorAll('select')].map((s) => s.value)).toEqual(['probe_verdict', 'equals', 'baseline', 'available'])
    expect((conditions()[0]!.querySelector('input[type=number]') as HTMLInputElement).value).toBe('3600')
    expect(root.textContent).toContain('24 小时')
    expect(root.textContent).toContain('queued/probing')
    expect(root.textContent).toContain('重新发布')
    app!.unmount(); app = undefined
    mount()
    await flush()
    expect(api.get).toHaveBeenCalledTimes(2)
    expect(conditions()).toHaveLength(1)
  })

  it('edits each supported field and writes unchanged persisted payload plus new AND conditions', async () => {
    mount(); await flush()
    const first = conditions()[0]!
    input(first.querySelector('select[aria-label="运算符"]')!, 'not_equals')
    button('filter-add').click(); await flush()
    expect(conditions()).toHaveLength(2)
    input(conditions()[1]!.querySelector('select[aria-label="字段"]')!, 'protocol'); await flush()
    input(conditions()[1]!.querySelector('select[aria-label="值"]')!, 'vless'); await flush()
    button('filter-save').click(); await flush()
    expect(api.put).toHaveBeenCalledWith(endpoint, { spec: { conditions: [
      { ...baseline.spec.conditions[0], op: 'not_equals' },
      { field: 'protocol', op: 'equals', value: 'vless' },
    ] } })
    expect(root.textContent).not.toContain('未保存的修改')
    button('filter-clear').click(); await flush()
    button('filter-save').click(); await flush()
    expect(api.put).toHaveBeenLastCalledWith(endpoint, { spec: { conditions: [] } })
    expect(find('filter-empty')).not.toBeNull()
  })

  it('supports all domain fields and en-US labels, restores server values after refresh', async () => {
    setLocale('en-US'); mount(); await flush()
    const field = conditions()[0]!.querySelector('select[aria-label="Field"]') as HTMLSelectElement
    expect([...field.options].map((o) => o.value)).toEqual(['display_name', 'protocol', 'source_subscription_ids', 'probe_verdict', 'probe_latency_ms'])
    expect([...conditions()[0]!.querySelectorAll<HTMLOptionElement>('select[aria-label="Probe kind"] option')].map((o) => o.value)).toEqual(['baseline', 'geo', 'streaming', 'ai', 'speed', 'ip_risk'])
    expect([...conditions()[0]!.querySelectorAll<HTMLOptionElement>('select[aria-label="Value"] option')].map((o) => o.value)).toEqual(['available', 'restricted', 'unknown', 'error', 'stale'])
    expect(root.textContent).toContain('24 hours')
    input(field, 'display_name'); await flush()
    input(conditions()[0]!.querySelector('input[aria-label="Value"]')!, 'My node'); await flush()
    expect(root.textContent).toContain('Unsaved changes')
    button('filter-refresh').click(); await flush()
    expect((conditions()[0]!.querySelector('select[aria-label="Field"]') as HTMLSelectElement).value).toBe('probe_verdict')
    expect(root.textContent).not.toContain('Unsaved changes')
  })

  it('rejects invalid freshness, kind, verdict and missing values without PUT', async () => {
    mount(); await flush()
    const fresh = conditions()[0]!.querySelector('input[type=number]') as HTMLInputElement
    input(fresh, '0'); button('filter-save').click(); await flush()
    expect(find('filter-validation-error')?.textContent).toMatch(/有效期/)
    input(fresh, '3600')
    input(conditions()[0]!.querySelector('select[aria-label="探针类型"]')!, 'invalid'); button('filter-save').click(); await flush()
    expect(find('filter-validation-error')?.textContent).toMatch(/探针类型/)
    input(conditions()[0]!.querySelector('select[aria-label="探针类型"]')!, 'baseline')
    input(conditions()[0]!.querySelector('select[aria-label="值"]')!, 'queued'); button('filter-save').click(); await flush()
    expect(find('filter-validation-error')?.textContent).toMatch(/判定/)
    input(conditions()[0]!.querySelector('select[aria-label="字段"]')!, 'probe_latency_ms'); await flush()
    button('filter-save').click(); await flush()
    expect(find('filter-validation-error')?.textContent).toMatch(/延迟/)
    expect(api.put).not.toHaveBeenCalled()
  })

  it('supports latency, source, name and protocol input without silently changing saved conditions', async () => {
    mount(); await flush()
    const field = () => conditions()[0]!.querySelector('select[aria-label="字段"]') as HTMLSelectElement
    const value = () => conditions()[0]!.querySelector('input[aria-label="值"]') as HTMLInputElement
    input(field(), 'probe_latency_ms'); await flush()
    input(value(), '125'); await flush()
    button('filter-save').click(); await flush()
    expect(api.put).toHaveBeenLastCalledWith(endpoint, { spec: { conditions: [
      { field: 'probe_latency_ms', op: 'lte', value: '125', probe_kind: 'baseline', freshness_seconds: 3600 },
    ] } })
    input(field(), 'source_subscription_ids'); await flush()
    input(value(), 'not-a-uuid'); await flush()
    button('filter-save').click(); await flush()
    expect(find('filter-validation-error')?.textContent).toContain('UUIDv7')
    input(value(), '019520c3-8268-7000-8000-000000000001'); await flush()
    button('filter-save').click(); await flush()
    expect(api.put).toHaveBeenLastCalledWith(endpoint, { spec: { conditions: [
      { field: 'source_subscription_ids', op: 'contains', value: '019520c3-8268-7000-8000-000000000001' },
    ] } })
    input(field(), 'display_name'); await flush()
    input(value(), '节点A'); await flush()
    button('filter-save').click(); await flush()
    expect(api.put).toHaveBeenLastCalledWith(endpoint, { spec: { conditions: [
      { field: 'display_name', op: 'contains', value: '节点A' },
    ] } })
    input(value(), '中'.repeat(100)); await flush()
    button('filter-save').click(); await flush()
    expect(find('filter-validation-error')?.textContent).toContain('255 字节')
  })

  it('treats only a missing record as empty; read failures do not unlock an empty save', async () => {
    vi.mocked(api.get).mockImplementation(async (path: string) => {
      if (path === endpoint) throw new Error('database unavailable')
      return {}
    })
    mount(); await flush()
    expect(find('filter-request-error')?.textContent).toContain('database unavailable')
    expect(find('filter-empty')).toBeNull()
    expect(button('filter-save')).toBeNull()
    expect(api.put).not.toHaveBeenCalled()
    app!.unmount(); app = undefined
    vi.mocked(api.get).mockImplementation(async (path: string) => {
      if (path === endpoint) throw new ApiError(404, 'not_found', 'missing')
      return {}
    })
    mount(); await flush()
    expect(find('filter-empty')).not.toBeNull()
  })

  it('retains edited conditions and shows the server error on failed PUT', async () => {
    vi.mocked(api.put).mockRejectedValue(new Error('write rejected'))
    mount(); await flush()
    button('filter-add').click(); await flush()
    button('filter-save').click(); await flush()
    expect(conditions()).toHaveLength(2)
    expect(root.textContent).toContain('未保存的修改')
    expect(find('filter-request-error')?.textContent).toContain('write rejected')
  })

  it('keeps the existing Policy page entry and corrects its credential-version claim', async () => {
    // PolicyView has other independently loaded resources; avoid exercising those unrelated endpoints.
    vi.spyOn(api, 'post').mockResolvedValue({})
    mount(PolicyView); await flush()
    const entry = root.querySelector('[data-testid="global-filter-btn"]') as HTMLButtonElement
    expect(entry).not.toBeNull()
    entry.click(); await flush()
    expect(root.textContent).toContain('queued/probing')
    expect(root.textContent).toContain('重新发布')
    expect(root.textContent).not.toContain('当前凭据版本匹配')
  })
})
