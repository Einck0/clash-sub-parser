// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, ref, h, nextTick } from 'vue'
import { createRouter, createMemoryHistory, RouterView } from 'vue-router'
import { useUnsavedChanges } from './useUnsavedChanges'

describe('useUnsavedChanges composable', () => {
  let originalConfirm: typeof window.confirm

  beforeEach(() => {
    originalConfirm = window.confirm
  })

  afterEach(() => {
    window.confirm = originalConfirm
    vi.restoreAllMocks()
  })

  it('allows discard immediately when not dirty without prompting', () => {
    const confirmSpy = vi.fn().mockReturnValue(true)
    window.confirm = confirmSpy

    let discardResult: boolean | undefined
    const app = createApp({
      setup() {
        const dirty = ref(false)
        const { confirmDiscard } = useUnsavedChanges(dirty)
        discardResult = confirmDiscard()
        return () => h('div')
      },
    })

    const el = document.createElement('div')
    app.mount(el)
    app.unmount()

    expect(discardResult).toBe(true)
    expect(confirmSpy).not.toHaveBeenCalled()
  })

  it('prompts window.confirm when dirty; returns true if user confirms and false if user cancels', () => {
    const confirmSpy = vi.fn()
    window.confirm = confirmSpy

    let guardRef: ReturnType<typeof useUnsavedChanges> | undefined
    const dirty = ref(true)

    const app = createApp({
      setup() {
        guardRef = useUnsavedChanges(dirty, () => 'Discard test message?')
        return () => h('div')
      },
    })

    const el = document.createElement('div')
    app.mount(el)

    // Case 1: User cancels -> keeps draft (returns false)
    confirmSpy.mockReturnValueOnce(false)
    const resultCancel = guardRef!.confirmDiscard()
    expect(resultCancel).toBe(false)
    expect(confirmSpy).toHaveBeenCalledWith('Discard test message?')

    // Case 2: User confirms -> allows discard (returns true)
    confirmSpy.mockReturnValueOnce(true)
    const resultConfirm = guardRef!.confirmDiscard()
    expect(resultConfirm).toBe(true)

    app.unmount()
  })

  it('can be mounted in an isolated component without Vue Router without throwing or emitting warnings', () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const dirty = ref(false)

    expect(() => {
      const app = createApp({
        setup() {
          useUnsavedChanges(dirty)
          return () => h('div')
        },
      })
      const el = document.createElement('div')
      app.mount(el)
      app.unmount()
    }).not.toThrow()

    // Ensure no Vue Router warnings about missing RouterView were emitted
    const routerWarnings = warnSpy.mock.calls.filter((call) =>
      String(call[0]).includes('onBeforeRouteLeave')
    )
    expect(routerWarnings).toHaveLength(0)
  })

  it('hooks onBeforeRouteLeave in an active RouterView to cancel or allow navigation based on return value', async () => {
    const confirmSpy = vi.fn()
    window.confirm = confirmSpy

    const isDirty = ref(true)

    const ComponentWithGuard = {
      setup() {
        useUnsavedChanges(isDirty, () => 'Leave page?')
        return () => h('div', { id: 'guarded-content' }, 'Guarded')
      },
    }

    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', component: ComponentWithGuard },
        { path: '/other', component: { render: () => h('div', 'Other') } },
      ],
    })

    const app = createApp({
      render: () => h(RouterView),
    })
    app.use(router)

    const el = document.createElement('div')
    document.body.appendChild(el)
    app.mount(el)
    await router.isReady()

    // 1. Attempt navigation while dirty and user cancels
    confirmSpy.mockReturnValueOnce(false)
    await router.push('/other').catch(() => {})
    await nextTick()
    expect(router.currentRoute.value.path).toBe('/')

    // 2. Attempt navigation while dirty and user confirms
    confirmSpy.mockReturnValueOnce(true)
    await router.push('/other')
    await nextTick()
    expect(router.currentRoute.value.path).toBe('/other')

    app.unmount()
    el.remove()
  })

  it('cleans up beforeunload listener on unmount', () => {
    const addEventSpy = vi.spyOn(window, 'addEventListener')
    const removeEventSpy = vi.spyOn(window, 'removeEventListener')

    const dirty = ref(true)
    const app = createApp({
      setup() {
        useUnsavedChanges(dirty)
        return () => h('div')
      },
    })

    const el = document.createElement('div')
    app.mount(el)
    expect(addEventSpy).toHaveBeenCalledWith('beforeunload', expect.any(Function))

    app.unmount()
    expect(removeEventSpy).toHaveBeenCalledWith('beforeunload', expect.any(Function))
  })
})
