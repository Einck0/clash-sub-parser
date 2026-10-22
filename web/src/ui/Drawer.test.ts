// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'
import Drawer from './Drawer.vue'
import DrawerCard from './DrawerCard.vue'
import { resetBodyScrollLockForTesting } from '../composables/useBodyScrollLock'

describe('Drawer and DrawerCard Components', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  beforeEach(() => {
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

    const title = document.querySelector('#drawer-title') as HTMLElement
    expect(title).toBeTruthy()
    expect(title.classList.contains('break-words')).toBe(true)

    const scrollBody = aside.querySelector('.flex-1.overflow-y-auto') as HTMLElement
    expect(scrollBody).toBeTruthy()
    expect(scrollBody.classList.contains('min-h-0')).toBe(true)
    expect(scrollBody.querySelector('.long-content-probe')).toBeTruthy()

    const closeBtn = document.querySelector('button[aria-label="Close drawer"]') as HTMLButtonElement
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
    expect(document.querySelector('#drawer-title')?.textContent?.trim()).toBe('Drawer Card Config')
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

    // Close the drawer
    propsState.modelValue = false
    await nextTick()

    expect(document.activeElement).toBe(triggerBtn)
    triggerBtn.remove()
  })
})
