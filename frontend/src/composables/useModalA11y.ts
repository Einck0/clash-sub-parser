import { ref, onMounted, onUnmounted, watch, nextTick, type Ref } from 'vue'

export interface UseModalA11yOptions {
  isOpen: Ref<boolean>
  containerRef: Ref<HTMLElement | null>
  initialFocusRef?: Ref<HTMLElement | null>
  onClose?: () => void
  lockScroll?: boolean
  trapFocus?: boolean
  closeOnEscape?: boolean
}

/**
 * Reusable modal accessibility composable providing:
 * - Focus capture and initial focus
 * - Focus trapping (Tab / Shift+Tab cycling)
 * - Escape key closing
 * - Background scroll lock
 * - Previous focus restoration on close
 */
export function useModalA11y(options: UseModalA11yOptions) {
  const {
    isOpen,
    containerRef,
    initialFocusRef,
    onClose,
    lockScroll = true,
    trapFocus = true,
    closeOnEscape = true,
  } = options

  let previouslyFocusedElement: HTMLElement | null = null
  let previousBodyOverflow = ''

  const focusableSelector =
    'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

  function getFocusableElements(): HTMLElement[] {
    if (!containerRef.value) return []
    return Array.from(containerRef.value.querySelectorAll<HTMLElement>(focusableSelector)).filter(
      (el) => el.offsetParent !== null || el === document.activeElement
    )
  }

  function handleKeyDown(e: KeyboardEvent) {
    if (!isOpen.value) return

    if (e.key === 'Escape' && closeOnEscape) {
      e.stopPropagation()
      e.preventDefault()
      onClose?.()
      return
    }

    if (e.key === 'Tab' && trapFocus && containerRef.value) {
      const focusables = getFocusableElements()
      if (focusables.length === 0) {
        e.preventDefault()
        return
      }

      const first = focusables[0]
      const last = focusables[focusables.length - 1]
      const active = document.activeElement

      if (e.shiftKey) {
        if (active === first || !containerRef.value.contains(active)) {
          e.preventDefault()
          last.focus()
        }
      } else {
        if (active === last || !containerRef.value.contains(active)) {
          e.preventDefault()
          first.focus()
        }
      }
    }
  }

  function applyOpenState() {
    previouslyFocusedElement = document.activeElement as HTMLElement | null

    if (lockScroll && typeof document !== 'undefined') {
      previousBodyOverflow = document.body.style.overflow
      document.body.style.overflow = 'hidden'
    }

    nextTick(() => {
      if (initialFocusRef?.value) {
        initialFocusRef.value.focus()
      } else {
        const focusables = getFocusableElements()
        if (focusables.length > 0) {
          focusables[0].focus()
        } else if (containerRef.value) {
          containerRef.value.focus()
        }
      }
    })
  }

  function cleanupOpenState() {
    if (lockScroll && typeof document !== 'undefined') {
      document.body.style.overflow = previousBodyOverflow
    }

    if (previouslyFocusedElement && typeof previouslyFocusedElement.focus === 'function') {
      previouslyFocusedElement.focus()
      previouslyFocusedElement = null
    }
  }

  watch(isOpen, (open) => {
    if (open) {
      applyOpenState()
    } else {
      cleanupOpenState()
    }
  })

  onMounted(() => {
    if (typeof window !== 'undefined') {
      window.addEventListener('keydown', handleKeyDown)
    }
    if (isOpen.value) {
      applyOpenState()
    }
  })

  onUnmounted(() => {
    if (typeof window !== 'undefined') {
      window.removeEventListener('keydown', handleKeyDown)
    }
    if (isOpen.value) {
      cleanupOpenState()
    }
  })

  return {
    getFocusableElements,
    handleKeyDown,
  }
}
