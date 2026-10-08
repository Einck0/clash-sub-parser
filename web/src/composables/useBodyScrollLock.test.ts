// @vitest-environment jsdom
import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { ref, defineComponent, h, nextTick, createApp } from 'vue'
import {
  acquireBodyScrollLock,
  releaseBodyScrollLock,
  resetBodyScrollLockForTesting,
  useBodyScrollLock,
} from './useBodyScrollLock'

describe('useBodyScrollLock composable', () => {
  beforeEach(() => {
    resetBodyScrollLockForTesting()
    document.body.style.overflow = ''
  })

  afterEach(() => {
    resetBodyScrollLockForTesting()
    document.body.style.overflow = ''
  })

  it('locks and unlocks body scroll imperatively', () => {
    expect(document.body.style.overflow).toBe('')
    acquireBodyScrollLock()
    expect(document.body.style.overflow).toBe('hidden')
    releaseBodyScrollLock()
    expect(document.body.style.overflow).toBe('')
  })

  it('manages multiple locks with reference counting', () => {
    acquireBodyScrollLock()
    acquireBodyScrollLock()
    expect(document.body.style.overflow).toBe('hidden')

    releaseBodyScrollLock()
    // Still locked because second lock is active
    expect(document.body.style.overflow).toBe('hidden')

    releaseBodyScrollLock()
    // Now unlocked
    expect(document.body.style.overflow).toBe('')
  })

  it('restores previous custom overflow value on final release', () => {
    document.body.style.overflow = 'auto'
    acquireBodyScrollLock()
    expect(document.body.style.overflow).toBe('hidden')

    releaseBodyScrollLock()
    expect(document.body.style.overflow).toBe('auto')
  })

  it('binds reactively to a ref and cleans up on unmount without breaking other dialogs', async () => {
    const isLockedA = ref(false)
    const isLockedB = ref(false)

    const CompA = defineComponent({
      setup() {
        useBodyScrollLock(isLockedA)
        return () => h('div', 'CompA')
      },
    })

    const CompB = defineComponent({
      setup() {
        useBodyScrollLock(isLockedB)
        return () => h('div', 'CompB')
      },
    })

    const containerA = document.createElement('div')
    const containerB = document.createElement('div')
    document.body.appendChild(containerA)
    document.body.appendChild(containerB)

    const appA = createApp(CompA)
    const appB = createApp(CompB)
    appA.mount(containerA)
    appB.mount(containerB)

    expect(document.body.style.overflow).toBe('')

    // Open Dialog A
    isLockedA.value = true
    await nextTick()
    expect(document.body.style.overflow).toBe('hidden')

    // Open Dialog B
    isLockedB.value = true
    await nextTick()
    expect(document.body.style.overflow).toBe('hidden')

    // Close Dialog A (Dialog B still open)
    isLockedA.value = false
    await nextTick()
    expect(document.body.style.overflow).toBe('hidden')

    // Unmount CompB while open -> should clean up and restore body
    appB.unmount()
    await nextTick()
    expect(document.body.style.overflow).toBe('')

    appA.unmount()
    containerA.remove()
    containerB.remove()
  })
})
