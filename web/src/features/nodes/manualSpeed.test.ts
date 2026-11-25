// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { api } from '../../api/client'
import { useNodes } from './useNodes'
import { normalizeNode, getNodeSpeedBadge, type NodeRecord } from './nodeView'
import NodeSpeedResult from './NodeSpeedResult.vue'
import NodeCompactRow from './NodeCompactRow.vue'

const record = (speed: Record<string, unknown> = {}): NodeRecord => ({
  logical_id: 'node_private', display_name: '香港专线', protocol: 'ss', active: true,
  capabilities: { speed: { verdict: 'available', throughput: 2048, observed_at: '2026-11-15T12:00:00Z', ...speed } },
})
beforeEach(() => { vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-11-15T12:00:00Z')) })
afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers() })

describe('explicit single-node manual speed', () => {
  it('uses speed-only explicit intent and a fresh key, refreshes persisted node state without changing an unrelated drawer', async () => {
    const hook = useNodes()
    hook.items.value = [normalizeNode(record({ observed_at: '2026-11-14T12:00:00Z' }))]
    hook.selectedNode.value = normalizeNode({ ...record(), logical_id: 'node_other', display_name: '另一节点' })
    const post = vi.spyOn(api, 'post').mockResolvedValue({ run_id: 'run_private', state: 'succeeded' })
    vi.spyOn(api, 'get').mockResolvedValue(record())
    expect(await hook.measureNodeSpeed('node_private')).toEqual({ ok: true })
    expect(post).toHaveBeenCalledWith('/api/v1/probes/runs', {
      manual_speed: true, node_logical_ids: ['node_private'], kinds: ['speed'],
    }, { headers: { 'Idempotency-Key': expect.any(String) } })
    expect(hook.items.value[0]!.capabilityDetails.speed?.throughput).toBe(2048)
    expect(hook.selectedNode.value?.logicalId).toBe('node_other')
    expect(hook.speedingNodeId.value).toBeNull()
    await hook.measureNodeSpeed('node_private')
    expect(post.mock.calls[0]![2]).not.toEqual(post.mock.calls[1]![2])
  })

  it.each([
    ['failed', record()],
    ['cancelled', record()],
    ['expired', record()],
    ['succeeded', record({ verdict: 'error' })],
    ['succeeded', record({ stale: true })],
    ['succeeded', record({ observed_at: '2026-11-14T12:00:00Z' })],
  ])('does not call an old/stale/failed observation successful (%s)', async (state, result) => {
    const hook = useNodes()
    hook.items.value = [normalizeNode(record({ observed_at: '2026-11-14T12:00:00Z' }))]
    vi.spyOn(api, 'post').mockResolvedValue({ run_id: 'run_private', state })
    vi.spyOn(api, 'get').mockResolvedValue(result)
    expect((await hook.measureNodeSpeed('node_private')).ok).toBe(false)
    expect(hook.speedingNodeId.value).toBeNull()
  })

  it('polls a queued run and blocks duplicate clicks while a request is active', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-15T12:00:00Z'))
    const hook = useNodes()
    const post = vi.spyOn(api, 'post').mockResolvedValue({ run_id: 'run_private', state: 'queued' })
    const get = vi.spyOn(api, 'get').mockImplementation(async (path) => path.includes('/probes/runs/') ? { state: 'succeeded' } : record())
    const pending = hook.measureNodeSpeed('node_private')
    expect(hook.speedingNodeId.value).toBe('node_private')
    expect((await hook.measureNodeSpeed('node_private')).ok).toBe(false)
    await vi.advanceTimersByTimeAsync(1000)
    expect((await pending).ok).toBe(true)
    expect(post).toHaveBeenCalledTimes(1)
    expect(get).toHaveBeenCalledWith('/api/v1/probes/runs/run_private')
  })

  it('does not reject a first server observation solely because the browser clock is ahead', async () => {
    const hook = useNodes()
    hook.items.value = [normalizeNode({ logical_id: 'node_private', display_name: 'Fixture', protocol: 'ss', active: true })]
    vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-11-15T12:05:00Z'))
    vi.spyOn(api, 'post').mockResolvedValue({ run_id: 'run_private', state: 'succeeded' })
    vi.spyOn(api, 'get').mockResolvedValue(record())
    expect((await hook.measureNodeSpeed('node_private')).ok).toBe(true)
  })

  it('rejects blank targets and reports request failures without changing old observations into success', async () => {
    const hook = useNodes()
    const post = vi.spyOn(api, 'post').mockRejectedValue(new Error('request failed'))
    expect((await hook.measureNodeSpeed('')).ok).toBe(false)
    expect(post).not.toHaveBeenCalled()
    expect((await hook.measureNodeSpeed('node_private')).ok).toBe(false)
    expect(hook.speedingNodeId.value).toBeNull()
  })
})

describe('persisted speed presentation', () => {
  it.each([
    ['failed', '上次检测失败'],
    ['cancelled', '上次检测已取消'],
    ['expired', '上次检测已超时'],
  ] as const)('keeps the ordinary probe %s outcome visible on the idle row and permits retry', async (probeRunState, label) => {
    const host = document.createElement('div')
    const probe = vi.fn()
    const app = createApp({ render: () => h(NodeCompactRow, {
      node: normalizeNode(record()), selected: false, probeState: 'idle', probeRunState, onProbeNode: probe,
    }) })
    try {
      app.mount(host)
      await nextTick()
      expect(host.querySelector('[data-testid="node-probe-terminal-state-badge"]')?.textContent).toContain(label)
      const button = host.querySelector<HTMLButtonElement>('[data-testid="node-card-probe-btn"]')!
      expect(button.disabled).toBe(false)
      button.click()
      expect(probe).toHaveBeenCalledOnce()
    } finally {
      app.unmount()
    }
  })

  it('shows saved speed and timestamp, but never a current successful number for stale/error/busy/latest-failed states', async () => {
    for (const [speed, props, expected] of [
      [{}, {}, '2.0 MB/s'],
      [{ stale: true }, {}, '已失效'],
      [{ verdict: 'error' }, {}, '不可用'],
      [{}, { busy: true }, '正在测速'],
      [{}, { failed: true }, '本次测速未取得'],
    ] as const) {
      const host = document.createElement('div')
      const app = createApp({ render: () => h(NodeSpeedResult, { node: normalizeNode(record(speed)), ...props }) })
      app.mount(host); await nextTick()
      expect(host.textContent).toContain(expected)
      if (Object.keys(speed).length || Object.keys(props).length) expect(host.textContent).not.toContain('MB/s')
      if (!('busy' in props)) expect(host.querySelector('time')?.getAttribute('datetime')).toBe('2026-11-15T12:00:00Z')
      app.unmount()
    }
    expect(getNodeSpeedBadge(record({ stale: true }))).toBeNull()
    expect(getNodeSpeedBadge(record({ verdict: 'error' }))).toBeNull()
  })

  it('keeps platform expansion distinct, keyboard accessible, and manual speed a separate emitted action', async () => {
    const host = document.createElement('div')
    const speed = vi.fn(), inspect = vi.fn()
    const app = createApp({ render: () => h(NodeCompactRow, {
      node: normalizeNode(record()), selected: false, probeState: 'idle',
      onSpeedNode: speed, onOpenDetail: inspect,
    }) })
    app.mount(host); await nextTick()
    expect(host.textContent).not.toContain('node_private')
    const toggle = host.querySelector<HTMLButtonElement>('[data-testid="node-platform-details-toggle"]')!
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    toggle.click(); await nextTick()
    expect(toggle.getAttribute('aria-expanded')).toBe('true')
    expect(host.querySelector('[data-testid="node-platform-details-expanded"]')?.textContent).toContain('平台能力详情')
    host.querySelector<HTMLButtonElement>('[data-testid="node-card-speed-btn"]')!.click()
    expect(speed).toHaveBeenCalledOnce()
    expect(inspect).not.toHaveBeenCalled()
    toggle.click(); await nextTick()
    expect(host.querySelector('[data-testid="node-platform-details-expanded"]')).toBeNull()
    app.unmount()
  })
})
