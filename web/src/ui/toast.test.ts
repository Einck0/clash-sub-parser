import { afterEach, describe, expect, it, vi } from 'vitest'
import { createToastStore } from './toast'

describe('toast store', () => {
  afterEach(() => vi.useRealTimers())

  it('adds and removes notifications without mutating the current list', () => {
    const store = createToastStore({ id: () => 'toast-1' })
    const id = store.push({ message: 'Saved', tone: 'success' })

    expect(store.items.value).toHaveLength(1)
    expect(store.items.value[0]).toMatchObject({ id, message: 'Saved', tone: 'success' })

    store.remove(id)
    expect(store.items.value).toEqual([])
  })

  it('redacts internal identifiers at the user-visible toast boundary', () => {
    const store = createToastStore({ id: () => 'toast-safe' })
    store.push({ message: 'node_private failed in run_private 8ef0ce60-c175-4265-8fa1-1a1bc5658bf0 sha256:abcdef0123456789abcdef0123456789' })

    expect(store.items.value[0]?.message).toContain('节点（标识已省略）')
    expect(store.items.value[0]?.message).not.toContain('node_private')
    expect(store.items.value[0]?.message).not.toContain('run_private')
    expect(store.items.value[0]?.message).not.toContain('8ef0ce60-c175-4265-8fa1-1a1bc5658bf0')
    expect(store.items.value[0]?.message).not.toContain('sha256:abcdef0123456789abcdef0123456789')
  })

  it('removes notifications after their configured duration', () => {
    vi.useFakeTimers()
    const store = createToastStore({ id: () => 'toast-1' })
    store.push({ message: 'Retrying', duration: 1200 })

    vi.advanceTimersByTime(1199)
    expect(store.items.value).toHaveLength(1)
    vi.advanceTimersByTime(1)
    expect(store.items.value).toEqual([])
  })
})
