import { defineStore } from 'pinia'
import { ref, reactive } from 'vue'

export const useAppStore = defineStore('app', () => {
  // --- Toast system ---
  const toasts = ref([])
  let toastId = 0
  const timerMap = new Map()

  function toast(message, type = 'info', duration = 3500) {
    const id = ++toastId
    toasts.value.push({ id, message, type })
    if (duration > 0) {
      const timer = setTimeout(() => { dismissToast(id); timerMap.delete(id) }, duration)
      timerMap.set(id, timer)
    }
    return id
  }

  function dismissToast(id) {
    toasts.value = toasts.value.filter((t) => t.id !== id)
    if (timerMap.has(id)) {
      clearTimeout(timerMap.get(id))
      timerMap.delete(id)
    }
  }

  // Convenience shortcuts
  const success = (msg, ms) => toast(msg, 'success', ms)
  const error = (msg, ms) => toast(msg, 'error', ms || 5000)
  const warning = (msg, ms) => toast(msg, 'warning', ms)
  const info = (msg, ms) => toast(msg, 'info', ms)

  // --- Confirm dialog ---
  const confirmState = reactive({
    open: false,
    title: '',
    message: '',
    confirmText: '确定',
    cancelText: '取消',
    danger: false,
    _resolve: null,
  })

  function confirm({ title = '确认操作', message, confirmText = '确定', cancelText = '取消', danger = false } = {}) {
    return new Promise((resolve) => {
      // If a confirm is already pending, resolve it as false first
      if (confirmState.open && confirmState._resolve) {
        confirmState._resolve(false)
      }
      confirmState.open = true
      confirmState.title = title
      confirmState.message = message
      confirmState.confirmText = confirmText
      confirmState.cancelText = cancelText
      confirmState.danger = danger
      confirmState._resolve = resolve
    })
  }

  function resolveConfirm(result) {
    confirmState.open = false
    confirmState._resolve?.(result)
    confirmState._resolve = null
  }

  function cancelPendingConfirm() {
    if (confirmState.open && confirmState._resolve) {
      confirmState._resolve(false)
      confirmState._resolve = null
      confirmState.open = false
    }
  }

  return {
    // Toast
    toasts,
    toast,
    dismissToast,
    success,
    error,
    warning,
    info,
    // Confirm
    confirmState,
    confirm,
    resolveConfirm,
    cancelPendingConfirm,
  }
})
