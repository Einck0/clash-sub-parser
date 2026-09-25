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
