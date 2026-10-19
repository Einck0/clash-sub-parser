// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import ProbeScheduleSettings from './ProbeScheduleSettings.vue'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'
import type { ProbeSchedule } from '../probes/probeTypes'

const flush = async () => {
  await nextTick()
  await new Promise((r) => setTimeout(r, 10))
  await nextTick()
}

describe('ProbeScheduleSettings (US-R2 Global Probe Settings)', () => {
  let root: HTMLDivElement
  let app: ReturnType<typeof createApp>
  const find = <T extends HTMLElement>(id: string) => root.querySelector(`[data-testid="${id}"]`) as T
  const click = (id: string) => find<HTMLButtonElement>(id).click()

  const initialSchedule: ProbeSchedule = {
    enabled: false,
    interval_seconds: 3600,
    kinds: ['baseline'],
    next_due_at: null,
    generation: 1,
    updated_at: '2026-10-19T10:00:00Z',
  }

  beforeEach(() => {
    vi.restoreAllMocks()
    toastStore.clear()
    root = document.createElement('div')
    document.body.append(root)

    vi.spyOn(api, 'get').mockResolvedValue(initialSchedule)
    vi.spyOn(api, 'put').mockImplementation(async (path: string, body?: any) => {
      if (path === '/api/v1/probes/schedule') {
        return {
          enabled: body.enabled,
          interval_seconds: body.interval_seconds,
          kinds: body.kinds,
          next_due_at: null,
          generation: 2,
          updated_at: '2026-10-19T10:05:00Z',
        }
      }
      return {}
    })

    app = createApp({ render: () => h(ProbeScheduleSettings) })
    app.mount(root)
  })

  afterEach(() => {
    app.unmount()
    root.remove()
    vi.restoreAllMocks()
  })

  it('loads schedule from GET /api/v1/probes/schedule and displays values on mount', async () => {
    await flush()
    expect(api.get).toHaveBeenCalledWith('/api/v1/probes/schedule')
    const toggle = find<HTMLInputElement>('probe-schedule-toggle')
    expect(toggle.checked).toBe(false)
    const intervalInput = find<HTMLInputElement>('probe-schedule-interval-input')
    expect(intervalInput.value).toBe('3600')
    const baselineCheckbox = find<HTMLInputElement>('probe-kind-checkbox-baseline')
    expect(baselineCheckbox.checked).toBe(true)
  })

  it('updates schedule and reads back server response into reactive state (保存回读)', async () => {
    await flush()

    // 1. Toggle switch to enabled
    const toggle = find<HTMLInputElement>('probe-schedule-toggle')
    toggle.checked = true
    toggle.dispatchEvent(new Event('change'))

    // 2. Edit interval to 7200 seconds
    const intervalInput = find<HTMLInputElement>('probe-schedule-interval-input')
    intervalInput.value = '7200'
    intervalInput.dispatchEvent(new Event('input'))

    // 3. Select AI kind
    const aiCheckbox = find<HTMLInputElement>('probe-kind-checkbox-ai')
    aiCheckbox.click()
    await flush()

    // 4. Click save
    click('save-probe-schedule-btn')
    await flush()

    expect(api.put).toHaveBeenCalledWith('/api/v1/probes/schedule', {
      enabled: true,
      interval_seconds: 7200,
      kinds: expect.arrayContaining(['baseline', 'ai']),
    })

    // Read-back verification
    expect(find<HTMLInputElement>('probe-schedule-toggle').checked).toBe(true)
    expect(find<HTMLInputElement>('probe-schedule-interval-input').value).toBe('7200')
    expect(toastStore.items.value.some((t) => t.tone === 'success')).toBe(true)
  })

  it('rejects interval_seconds out of bounds (60..604800) and displays visible error without API request', async () => {
    await flush()

    const intervalInput = find<HTMLInputElement>('probe-schedule-interval-input')
    intervalInput.value = '30' // below 60
    intervalInput.dispatchEvent(new Event('input'))

    click('save-probe-schedule-btn')
    await flush()

    expect(api.put).not.toHaveBeenCalled()
    const errorEl = find('probe-schedule-error')
    expect(errorEl).not.toBeNull()
    expect(errorEl.textContent).toContain('60 到 604800 秒')
  })

  it('prevents deselecting all kinds and enforces at least one probe kind', async () => {
    await flush()

    // Try unchecking baseline (the only checked kind)
    const baselineCheckbox = find<HTMLInputElement>('probe-kind-checkbox-baseline')
    baselineCheckbox.click()
    await flush()

    // Should still have baseline checked and show warning
    const errorEl = find('probe-schedule-error')
    expect(errorEl).not.toBeNull()
    expect(errorEl.textContent).toContain('必须至少保留一个检测项目')
  })

  it('surfaces visible server error when API put rejects with 422 or 500', async () => {
    await flush()

    vi.spyOn(api, 'put').mockRejectedValueOnce(new Error('Schedule update conflict on server'))

    click('save-probe-schedule-btn')
    await flush()

    const errorEl = find('probe-schedule-error')
    expect(errorEl).not.toBeNull()
    expect(errorEl.textContent).toContain('Schedule update conflict on server')
  })
})
