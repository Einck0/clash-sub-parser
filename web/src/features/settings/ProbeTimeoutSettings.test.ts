// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import ProbeTimeoutSettings from './ProbeTimeoutSettings.vue'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'

const endpoint = '/api/v1/settings/probe-timeout'
const flush = async () => {
  await nextTick()
  await Promise.resolve()
  await nextTick()
}

describe('ProbeTimeoutSettings', () => {
  let root: HTMLDivElement
  let app: ReturnType<typeof createApp>
  const find = <T extends HTMLElement>(id: string) => root.querySelector(`[data-testid="${id}"]`) as T
  const click = (id: string) => find<HTMLButtonElement>(id).click()
  const edit = (id: string, value: string) => {
    const node = find<HTMLInputElement>(id)
    node.value = value
    node.dispatchEvent(new Event('input', { bubbles: true }))
  }

  beforeEach(() => {
    vi.restoreAllMocks()
    toastStore.clear()
    root = document.createElement('div')
    document.body.append(root)
    vi.spyOn(api, 'get').mockResolvedValue({ probe_timeout_seconds: 5 })
    vi.spyOn(api, 'put').mockResolvedValue({ probe_timeout_seconds: 10 })
    app = createApp({ render: () => h(ProbeTimeoutSettings) })
    app.mount(root)
  })

  afterEach(() => {
    app.unmount()
    root.remove()
    vi.restoreAllMocks()
  })

  it('loads probe timeout on mount and displays in input', async () => {
    await flush()
    expect(api.get).toHaveBeenCalledWith(endpoint)
    expect(find<HTMLInputElement>('probe-timeout-input').value).toBe('5')
  })

  it('updates probe timeout and submits to API', async () => {
    await flush()
    edit('probe-timeout-input', '10')
    click('save-probe-timeout-btn')
    await flush()
    expect(api.put).toHaveBeenCalledWith(endpoint, { probe_timeout_seconds: 10 })
  })

  it('rejects values out of range (1..30) without sending API request', async () => {
    await flush()
    edit('probe-timeout-input', '0')
    click('save-probe-timeout-btn')
    await flush()
    expect(api.put).not.toHaveBeenCalled()
    expect(root.textContent).toContain('1 到 30 秒')
  })
})
