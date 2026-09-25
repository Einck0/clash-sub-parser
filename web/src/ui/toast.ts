import { ref, type Ref } from 'vue'

export type ToastTone = 'info' | 'success' | 'warning' | 'error'

export interface ToastInput {
  message: string
  title?: string
  tone?: ToastTone
  duration?: number
}

export interface Toast extends ToastInput {
  id: string
  tone: ToastTone
  duration: number
}

export interface ToastStoreOptions {
  setTimeout?: typeof globalThis.setTimeout
  id?: () => string
}

export interface ToastStore {
  items: Ref<Toast[]>
  push(input: ToastInput): string
  remove(id: string): void
  clear(): void
}

export function createToastStore(options: ToastStoreOptions = {}): ToastStore {
  const items = ref<Toast[]>([])
  const schedule = options.setTimeout ?? globalThis.setTimeout
  const makeId = options.id ?? (() => `toast-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`)

  const remove = (id: string) => {
    items.value = items.value.filter((item) => item.id !== id)
  }

  const push = (input: ToastInput) => {
    const toast: Toast = {
      ...input,
      id: makeId(),
      tone: input.tone ?? 'info',
      duration: input.duration ?? 5000,
    }
    items.value = [...items.value, toast]
    if (toast.duration > 0) {
      schedule(() => remove(toast.id), toast.duration)
    }
    return toast.id
  }

  return { items, push, remove, clear: () => { items.value = [] } }
}

export const toastStore = createToastStore()
