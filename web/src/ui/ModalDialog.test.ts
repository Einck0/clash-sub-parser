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

  it('assigns independent labels to multiple dialog instances and emits one close request', async () => {
    const state = reactive({ first: true, second: true })
    const updates = vi.fn((value: boolean) => { state.first = value })
    app = createApp({ render: () => [
      h(ModalDialog, { modelValue: state.first, title: 'First', description: 'First details', 'onUpdate:modelValue': updates }),
      h(ModalDialog, { modelValue: state.second, title: 'Second', description: 'Second details' }),
    ] })
    app.mount(container)
    await nextTick()
    const dialogs = [...container.querySelectorAll('dialog')]
    expect(new Set(dialogs.map((dialog) => dialog.getAttribute('aria-labelledby'))).size).toBe(2)
    expect(new Set(dialogs.map((dialog) => dialog.getAttribute('aria-describedby'))).size).toBe(2)
    for (const [index, dialog] of dialogs.entries()) {
      expect(document.getElementById(dialog.getAttribute('aria-labelledby')!)?.textContent).toBe(index === 0 ? 'First' : 'Second')
    }
    dialogs[0]!.querySelector<HTMLButtonElement>('[data-testid="modal-dialog-close"]')!.click()
    await nextTick()
    await nextTick()
    expect(updates).toHaveBeenCalledOnce()
    expect(dialogs[0]!.open).toBe(false)
    expect(dialogs[1]!.open).toBe(true)
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
    const scrollContainer = bodyProbe?.parentElement?.parentElement as HTMLElement
    expect(scrollContainer.classList.contains('overflow-y-auto')).toBe(true)
    expect(scrollContainer.classList.contains('min-h-0')).toBe(true)

    const footerBtn = container.querySelector('#modal-footer-btn')
    expect(footerBtn).toBeTruthy()

    const closeBtn = container.querySelector('button[data-testid="modal-dialog-close"]') as HTMLButtonElement
    expect(closeBtn).toBeTruthy()
    closeBtn.click()
    await nextTick()
    expect(emitted['update:modelValue']).toContain(false)
  })
})
