// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'
import Drawer from './Drawer.vue'
import DrawerCard from './DrawerCard.vue'
import { resetBodyScrollLockForTesting } from '../composables/useBodyScrollLock'

describe('Drawer and DrawerCard Components', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null
  const hasNativeInert = 'inert' in HTMLElement.prototype

  beforeEach(() => {
    // jsdom 25 lacks inert reflection; Vue must remove the attribute when it is false.
    if (!hasNativeInert) {
      Object.defineProperty(HTMLElement.prototype, 'inert', {
        configurable: true,
        get(this: HTMLElement) { return this.hasAttribute('inert') },
        set(this: HTMLElement, value: boolean) { this.toggleAttribute('inert', Boolean(value)) },
      })
    }
    resetBodyScrollLockForTesting()
    document.body.style.overflow = ''
    container = document.createElement('div')
    document.body.appendChild(container)
  })

  afterEach(() => {
    if (app) {
      app.unmount()
      app = null
    }
    container.remove()
    document.body.innerHTML = ''
    resetBodyScrollLockForTesting()
    document.body.style.overflow = ''
    vi.restoreAllMocks()
    if (!hasNativeInert) Reflect.deleteProperty(HTMLElement.prototype, 'inert')
  })

  it('renders Drawer when open and provides scrollable content structure with safe-area support', async () => {
    const propsState = reactive({
      modelValue: true,
      title: 'Drawer Title Extremely Long Long Long Content Header',
      description: 'A very detailed and long description that wraps across multiple mobile viewport lines nicely',
    })

    const emitted: Record<string, any[]> = {
      'update:modelValue': [],
      close: [],
    }

    app = createApp({
      render() {
        return h(
          Drawer,
          {
            ...propsState,
            'onUpdate:modelValue': (val: boolean) => {
              propsState.modelValue = val
              emitted['update:modelValue'].push(val)
            },
            onClose: () => {
              emitted.close.push(true)
            },
          },
          {
            default: () => h('div', { class: 'long-content-probe' }, 'Scrollable body content'),
            footer: () => h('button', { type: 'button', id: 'footer-btn' }, 'Save Action'),
          }
        )
      },
    })

    app.mount(container)
    await nextTick()

    const aside = document.querySelector('aside[role="dialog"]') as HTMLElement
    expect(aside).toBeTruthy()
    expect(aside.classList.contains('overflow-hidden')).toBe(true)
    expect(aside.className).toContain('adaptive-surface-sheet')

    const title = document.getElementById(aside.getAttribute('aria-labelledby')!) as HTMLElement
    expect(title).toBeTruthy()
    expect(title.classList.contains('break-words')).toBe(true)

    const scrollBody = aside.querySelector('.flex-1.overflow-y-auto') as HTMLElement
    expect(scrollBody).toBeTruthy()
    expect(scrollBody.classList.contains('min-h-0')).toBe(true)
    expect(scrollBody.querySelector('.long-content-probe')).toBeTruthy()

    const closeBtn = document.querySelector('button[data-testid="drawer-close"]') as HTMLButtonElement
    expect(closeBtn).toBeTruthy()
    closeBtn.click()
    await nextTick()
    expect(emitted['update:modelValue']).toContain(false)
    expect(emitted.close.length).toBe(1)
  })

  it('DrawerCard passes through slots, header and footer correctly', async () => {
    const propsState = reactive({
      modelValue: true,
      title: 'Drawer Card Config',
    })

    app = createApp({
      render() {
        return h(
          DrawerCard,
          {
            ...propsState,
          },
          {
            default: () => h('div', { id: 'card-slot' }, 'Card Body Slot'),
            footer: () => h('div', { id: 'card-footer' }, 'Card Footer Slot'),
          }
        )
      },
    })

    app.mount(container)
    await nextTick()

    expect(document.querySelector('#card-slot')).toBeTruthy()
    expect(document.querySelector('#card-footer')).toBeTruthy()
    const panel = document.querySelector('aside[role="dialog"]')!
    expect(document.getElementById(panel.getAttribute('aria-labelledby')!)?.textContent?.trim()).toBe('Drawer Card Config')
  })

  it('closes on Escape key press when open', async () => {
    const propsState = reactive({
      modelValue: true,
      title: 'Escape test',
    })

    const emitted: Record<string, any[]> = {
      'update:modelValue': [],
      close: [],
    }

    app = createApp({
      render() {
        return h(Drawer, {
          ...propsState,
          'onUpdate:modelValue': (val: boolean) => {
            propsState.modelValue = val
            emitted['update:modelValue'].push(val)
          },
          onClose: () => {
            emitted.close.push(true)
          },
        })
      },
    })

    app.mount(container)
    await nextTick()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()

    expect(emitted['update:modelValue']).toContain(false)
    expect(emitted.close.length).toBe(1)
  })

  it('closes and emits events when clicking the backdrop overlay', async () => {
    const propsState = reactive({
      modelValue: true,
      title: 'Backdrop test',
    })

    const emitted: Record<string, any[]> = {
      'update:modelValue': [],
      close: [],
    }

    app = createApp({
      render() {
        return h(Drawer, {
          ...propsState,
          'onUpdate:modelValue': (val: boolean) => {
            propsState.modelValue = val
            emitted['update:modelValue'].push(val)
          },
          onClose: () => {
            emitted.close.push(true)
          },
        })
      },
    })

    app.mount(container)
    await nextTick()

    const backdrop = document.querySelector('[data-testid="drawer-backdrop"]') as HTMLElement
    expect(backdrop).toBeTruthy()

    backdrop.click()
    await nextTick()

    expect(emitted['update:modelValue']).toContain(false)
    expect(emitted.close.length).toBe(1)
  })

  it('acquires body scroll lock when open and releases when closed or unmounted', async () => {
    const propsState = reactive({
      modelValue: false,
      title: 'Scroll lock test',
    })

    app = createApp({
      render() {
        return h(Drawer, {
          ...propsState,
        })
      },
    })

    app.mount(container)
    await nextTick()
    expect(document.body.style.overflow).toBe('')

    // Open drawer
    propsState.modelValue = true
    await nextTick()
    expect(document.body.style.overflow).toBe('hidden')

    // Close drawer
    propsState.modelValue = false
    await nextTick()
    expect(document.body.style.overflow).toBe('')

    // Re-open and unmount
    propsState.modelValue = true
    await nextTick()
    expect(document.body.style.overflow).toBe('hidden')

    app.unmount()
    app = null
    await nextTick()
    expect(document.body.style.overflow).toBe('')
  })

  it('focuses on open, loops Tab within visible controls, and leaves native dialogs in control', async () => {
    const trigger = document.createElement('button')
    document.body.appendChild(trigger)
    trigger.focus()
    const propsState = reactive({ modelValue: false, title: 'Keyboard drawer' })
    app = createApp({
      render: () => h(Drawer, {
        ...propsState,
        'onUpdate:modelValue': (value: boolean) => { propsState.modelValue = value },
      }, {
        default: () => [
          h('button', { disabled: true }, 'Disabled'),
          h('button', { hidden: true }, 'Hidden'),
        ],
        footer: () => h('button', { id: 'keyboard-last' }, 'Save'),
      }),
    })
    app.mount(container)
    propsState.modelValue = true
    await nextTick()
    await nextTick()

    const panel = document.querySelector<HTMLElement>('[data-drawer-panel]')!
    const close = panel.querySelector<HTMLButtonElement>('button[data-testid="drawer-close"]')!
    const last = panel.querySelector<HTMLButtonElement>('#keyboard-last')!
    expect(panel.closest('[inert]')).toBeNull()
    expect(Array.from(panel.querySelectorAll('button')).filter((button) => !button.disabled && !button.hidden))
      .toEqual([close, last])
    // jsdom has no layout; give every button geometry so disabled/hidden filtering is exercised too.
    for (const element of panel.querySelectorAll('button')) {
      vi.spyOn(element, 'getClientRects').mockReturnValue([{} as DOMRect] as unknown as DOMRectList)
    }
    expect(document.activeElement).toBe(close)
    const tab = (shiftKey = false) => {
      const event = new KeyboardEvent('keydown', { key: 'Tab', shiftKey, bubbles: true, cancelable: true })
      document.activeElement!.dispatchEvent(event)
      return event
    }
    expect(tab(true).defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(last)
    expect(tab().defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(close)

    const dialog = document.createElement('dialog')
    dialog.setAttribute('open', '')
    document.body.appendChild(dialog)
    expect(tab(true).defaultPrevented).toBe(false)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(propsState.modelValue).toBe(true)
    dialog.remove()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()
    await nextTick()
    expect(propsState.modelValue).toBe(false)
    expect(document.activeElement).toBe(trigger)
  })

  it('uses unique labels and only the last opened drawer owns focus and Escape', async () => {
    const state = reactive({ first: false, second: true })
    app = createApp({
      render: () => [
        h(Drawer, { modelValue: state.first, title: 'First', description: 'First description',
          'onUpdate:modelValue': (value: boolean) => { state.first = value } }),
        h(Drawer, { modelValue: state.second, title: 'Second', description: 'Second description',
          'onUpdate:modelValue': (value: boolean) => { state.second = value } }),
      ],
    })
    app.mount(container)
    await nextTick()
    await nextTick()
    const second = document.querySelector<HTMLElement>('[data-drawer-panel]')!
    const secondClose = second.querySelector<HTMLButtonElement>('button')!
    expect(document.activeElement).toBe(secondClose)
    // First is mounted earlier, but opening it now must put it above Second.
    state.first = true
    await nextTick()
    await nextTick()
    const panels = Array.from(document.querySelectorAll<HTMLElement>('[data-drawer-panel]'))
    const first = panels.find((panel) => document.getElementById(panel.getAttribute('aria-labelledby')!)?.textContent?.trim() === 'First')!
    for (const attribute of ['aria-labelledby', 'aria-describedby']) {
      const ids = panels.map((panel) => panel.getAttribute(attribute)!)
      expect(new Set(ids).size).toBe(2)
      expect(ids.every((id) => document.getElementById(id))).toBe(true)
    }
    expect(document.activeElement).toBe(first.querySelector('button'))
    expect(second.hasAttribute('inert')).toBe(true)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }))
    await nextTick()
    await nextTick()
    expect(state.first).toBe(false)
    expect(state.second).toBe(true)
    expect(document.activeElement).toBe(secondClose)
  })

  it('restores focus to previous active element upon close', async () => {
    const triggerBtn = document.createElement('button')
    triggerBtn.id = 'trigger-btn'
    document.body.appendChild(triggerBtn)
    triggerBtn.focus()
    expect(document.activeElement).toBe(triggerBtn)

    const propsState = reactive({
      modelValue: true,
      title: 'Focus restore test',
    })

    app = createApp({
      render() {
        return h(Drawer, {
          ...propsState,
          'onUpdate:modelValue': (val: boolean) => {
            propsState.modelValue = val
          },
        })
      },
    })

    app.mount(container)
    await nextTick()

    expect(document.activeElement).toBe(document.querySelector('button[data-testid="drawer-close"]'))

    // Close the drawer
    propsState.modelValue = false
    await nextTick()
    await nextTick()

    expect(document.activeElement).toBe(triggerBtn)
    triggerBtn.remove()
  })
})
