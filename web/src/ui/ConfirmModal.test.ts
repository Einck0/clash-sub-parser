// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'
import ConfirmModal from './ConfirmModal.vue'
import ModalDialog from './ModalDialog.vue'

describe('ConfirmModal Component', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  beforeEach(() => {
    HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', '') }
    HTMLDialogElement.prototype.close = function () { this.removeAttribute('open'); this.dispatchEvent(new Event('close')) }
    container = document.createElement('div')
    document.body.appendChild(container)
  })

  afterEach(() => {
    if (app) {
      app.unmount()
      app = null
    }
    container.remove()
    vi.restoreAllMocks()
  })

  function mountModal(props: Record<string, any> = {}) {
    const propsState = reactive({
      modelValue: true,
      title: 'Confirm Operation',
      message: 'Are you sure you want to proceed with this destructive action?',
      confirmText: 'Confirm',
      cancelText: 'Cancel',
      tone: 'danger' as 'danger' | 'warning' | 'primary',
      loading: false,
      ...props,
    })

    const emitted: Record<string, any[]> = {
      'update:modelValue': [],
      confirm: [],
      cancel: [],
    }

    app = createApp({
      render() {
        return h(ConfirmModal, {
          ...propsState,
          'onUpdate:modelValue': (val: boolean) => {
            propsState.modelValue = val
            emitted['update:modelValue'].push(val)
          },
          onConfirm: () => {
            emitted.confirm.push(true)
          },
          onCancel: () => {
            emitted.cancel.push(true)
          },
        })
      },
    })

    app.mount(container)

    return {
      propsState,
      emitted,
      getModal: () => container.querySelector('.modal') as HTMLDialogElement,
      getTitle: () => container.querySelector('[data-testid="confirm-modal-title"]'),
      getMessage: () => container.querySelector('[data-testid="confirm-modal-message"]'),
      getConfirmBtn: () => container.querySelector('button[data-testid="confirm-modal-confirm"]') as HTMLButtonElement,
      getCancelBtn: () => container.querySelector('button[data-testid="confirm-modal-cancel"]') as HTMLButtonElement,
      getCloseBtn: () => container.querySelector('button[data-testid="confirm-modal-close"]') as HTMLButtonElement,
      getToneIconContainer: () => container.querySelector('[data-testid="confirm-modal-tone-icon"]'),
    }
  }

  it('renders modal open when modelValue is true and closes when false', async () => {
    const { propsState, getModal } = mountModal({ modelValue: true })
    expect(getModal()).toBeTruthy()
    await nextTick()
    expect(getModal().open).toBe(true)

    propsState.modelValue = false
    await nextTick()
    await nextTick()
    expect(getModal().open).toBe(false)
  })

  it('displays correct title, message, and button labels', () => {
    const { getTitle, getMessage, getConfirmBtn, getCancelBtn } = mountModal({
      title: 'Delete Subscription',
      message: 'Are you sure you want to remove this source feed?',
      confirmText: 'Delete Feed',
      cancelText: 'Keep Feed',
    })

    expect(getTitle()?.textContent?.trim()).toBe('Delete Subscription')
    expect(getMessage()?.textContent?.trim()).toBe('Are you sure you want to remove this source feed?')
    expect(getConfirmBtn()?.textContent?.trim()).toBe('Delete Feed')
    expect(getCancelBtn()?.textContent?.trim()).toBe('Keep Feed')
  })

  it('defaults to danger tone with error button styling and icon', () => {
    const { getConfirmBtn, getToneIconContainer } = mountModal({ tone: 'danger' })
    expect(getConfirmBtn().classList.contains('btn-error')).toBe(true)
    expect(getToneIconContainer()?.classList.contains('text-error')).toBe(true)
  })

  it('supports warning and primary tone visual variants', async () => {
    const { propsState, getConfirmBtn, getToneIconContainer } = mountModal({ tone: 'warning' })
    expect(getConfirmBtn().classList.contains('btn-warning')).toBe(true)
    expect(getToneIconContainer()?.classList.contains('text-warning')).toBe(true)

    propsState.tone = 'primary'
    await nextTick()
    expect(getConfirmBtn().classList.contains('btn-primary')).toBe(true)
    expect(getToneIconContainer()?.classList.contains('text-primary')).toBe(true)
  })

  it('emits confirm when confirm button is clicked', async () => {
    const { getConfirmBtn, emitted } = mountModal()
    getConfirmBtn().click()
    await nextTick()

    expect(emitted.confirm).toHaveLength(1)
  })

  it('emits cancel and updates modelValue when cancel or close button is clicked', async () => {
    const { getCancelBtn, getCloseBtn, emitted } = mountModal()

    getCancelBtn().click()
    await nextTick()
    expect(emitted.cancel).toHaveLength(1)
    expect(emitted['update:modelValue']).toContain(false)

    getCloseBtn().click()
    await nextTick()
    expect(emitted.cancel).toHaveLength(2)
  })

  it('handles backdrop clicks and escape key to cancel', async () => {
    const { getModal, emitted } = mountModal({ closable: true })

    getModal().click()
    await nextTick()
    expect(emitted.cancel).toHaveLength(1)

    getModal().dispatchEvent(new Event('cancel', { cancelable: true }))
    await nextTick()
    expect(emitted.cancel).toHaveLength(2)
  })

  it('opens the confirmation as a native modal after an already open parent dialog', async () => {
    const showModal = vi.spyOn(HTMLDialogElement.prototype, 'showModal')
    const state = reactive({ parent: true, confirm: false })
    app = createApp({
      render: () => [
        h(ModalDialog, { modelValue: state.parent, 'onUpdate:modelValue': (value: boolean) => { state.parent = value } }, { default: () => h('button', { id: 'revoke', onClick: () => { state.confirm = true } }, 'Revoke') }),
        h(ConfirmModal, { modelValue: state.confirm, 'onUpdate:modelValue': (value: boolean) => { state.confirm = value } }),
      ],
    })
    app.mount(container)
    await nextTick()
    await nextTick()
    expect(showModal).toHaveBeenCalledTimes(1)
    ;(container.querySelector('#revoke') as HTMLButtonElement).click()
    await nextTick()
    await nextTick()
    expect(showModal).toHaveBeenCalledTimes(2)
    const dialogs = container.querySelectorAll('dialog')
    expect(dialogs[0].open).toBe(true)
    expect(dialogs[1].open).toBe(true)
    ;(dialogs[1].querySelector('[data-testid="confirm-modal-cancel"]') as HTMLButtonElement).click()
    await nextTick()
    await nextTick()
    expect(dialogs[0].open).toBe(true)
    expect(dialogs[1].open).toBe(false)
  })

  it('keeps message and slot in one scroll region with actions outside', async () => {
    const { getModal } = mountModal({ message: 'Long message '.repeat(100) })
    await nextTick()
    const box = getModal().querySelector('.modal-box') as HTMLElement
    const scroll = box.querySelector('.overflow-y-auto') as HTMLElement
    expect(scroll.textContent).toContain('Long message')
    expect(scroll.contains(getModal().querySelector('[data-testid="confirm-modal-confirm"]'))).toBe(false)
    expect(box.className).toContain('adaptive-surface-dialog')
  })

  it('disables actions and applies loading spinner when loading is true', async () => {
    const { propsState, getConfirmBtn, getCancelBtn, emitted } = mountModal({ loading: true })

    expect(getConfirmBtn().disabled).toBe(true)
    expect(getConfirmBtn().classList.contains('loading')).toBe(true)
    expect(getCancelBtn().disabled).toBe(true)

    getConfirmBtn().click()
    await nextTick()
    expect(emitted.confirm).toHaveLength(0)

    propsState.loading = false
    await nextTick()
    expect(getConfirmBtn().disabled).toBe(false)
  })

  it('handles long content in message and slot with scrollable layout classes', async () => {
    const longMessage = 'A'.repeat(500)
    const { getMessage, getModal } = mountModal({
      message: longMessage,
    })

    const box = getModal().querySelector('.modal-box') as HTMLElement
    expect(box).toBeTruthy()
    expect(box.classList.contains('flex-col')).toBe(true)
    expect(box.classList.contains('overflow-hidden')).toBe(true)
    expect(getMessage()?.classList.contains('break-words')).toBe(true)
  })
})
