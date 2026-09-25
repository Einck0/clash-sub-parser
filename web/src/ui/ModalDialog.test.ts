// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'
import ModalDialog from './ModalDialog.vue'

describe('ModalDialog Component', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  beforeEach(() => {
    // jsdom HTMLDialogElement showModal/close mock if needed
    if (!HTMLDialogElement.prototype.showModal) {
      HTMLDialogElement.prototype.showModal = function (this: HTMLDialogElement) {
        this.setAttribute('open', '')
        Object.defineProperty(this, 'open', { value: true, writable: true, configurable: true })
      }
    }
    if (!HTMLDialogElement.prototype.close) {
      HTMLDialogElement.prototype.close = function (this: HTMLDialogElement) {
        this.removeAttribute('open')
        Object.defineProperty(this, 'open', { value: false, writable: true, configurable: true })
        this.dispatchEvent(new Event('close'))
      }
    }

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
    vi.restoreAllMocks()
  })

  it('renders title, description and scrollable content flex column structure', async () => {
    const propsState = reactive({
      modelValue: true,
      title: 'Modal Title Long Enough For Testing Wrapping',
      description: 'Modal description for responsive testing',
    })

    const emitted: Record<string, any[]> = {
      'update:modelValue': [],
    }

    app = createApp({
      render() {
        return h(
          ModalDialog,
          {
            ...propsState,
            'onUpdate:modelValue': (val: boolean) => {
              propsState.modelValue = val
              emitted['update:modelValue'].push(val)
            },
          },
          {
            default: () => h('div', { class: 'modal-body-probe' }, 'Modal Body Content'),
            footer: () => h('button', { id: 'modal-footer-btn' }, 'Submit'),
          }
        )
      },
    })

    app.mount(container)
    await nextTick()

    const box = container.querySelector('.modal-box') as HTMLElement
    expect(box).toBeTruthy()
    expect(box.classList.contains('flex-col')).toBe(true)
    expect(box.classList.contains('overflow-hidden')).toBe(true)
    expect(box.className).toContain('adaptive-surface-dialog')

    const bodyProbe = container.querySelector('.modal-body-probe')
    expect(bodyProbe).toBeTruthy()
    const scrollContainer = bodyProbe?.parentElement as HTMLElement
    expect(scrollContainer.classList.contains('overflow-y-auto')).toBe(true)
    expect(scrollContainer.classList.contains('min-h-0')).toBe(true)

    const footerBtn = container.querySelector('#modal-footer-btn')
    expect(footerBtn).toBeTruthy()

    const closeBtn = container.querySelector('button[aria-label="Close dialog"]') as HTMLButtonElement
    expect(closeBtn).toBeTruthy()
    closeBtn.click()
    await nextTick()
    expect(emitted['update:modelValue']).toContain(false)
  })
})
