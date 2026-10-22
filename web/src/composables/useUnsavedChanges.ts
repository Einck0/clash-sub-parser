import { getCurrentInstance, inject, onUnmounted, type Ref, type ComputedRef } from 'vue'
import { onBeforeRouteLeave, matchedRouteKey } from 'vue-router'

export interface UseUnsavedChangesReturn {
  /**
   * Prompts user to confirm discarding unsaved changes if dirty.
   * Returns `true` if changes can be discarded (either not dirty or user confirmed),
   * or `false` if discard was cancelled (draft should be preserved).
   */
  confirmDiscard: () => boolean
}

/**
 * Guard against accidental navigation or tab closure when unsaved changes exist.
 * - Safely registers `beforeunload` with automatic disposal on unmount.
 * - Safely hooks router navigation guard using single return-value convention (no mixed next()).
 * - Guards against calling onBeforeRouteLeave outside RouterView to prevent warnings in isolated unit tests.
 * - Exposes `confirmDiscard()` for drawer/modal close checks.
 */
export function useUnsavedChanges(
  dirty: Ref<boolean> | ComputedRef<boolean>,
  message?: () => string
): UseUnsavedChangesReturn {
  const getPromptMessage = (): string => {
    if (typeof message === 'function') {
      try {
        return message()
      } catch {
        // fallback
      }
    }
    return 'You have unsaved changes. Are you sure you want to discard them?'
  }

  const confirmDiscard = (): boolean => {
    if (!dirty.value) {
      return true
    }

    if (typeof window !== 'undefined' && typeof window.confirm === 'function') {
      return window.confirm(getPromptMessage())
    }

    return true
  }

  // Handle browser window / tab unload
  const handleBeforeUnload = (event: BeforeUnloadEvent) => {
    if (dirty.value) {
      event.preventDefault()
      event.returnValue = ''
      return ''
    }
  }

  if (typeof window !== 'undefined') {
    window.addEventListener('beforeunload', handleBeforeUnload)
  }

  onUnmounted(() => {
    if (typeof window !== 'undefined') {
      window.removeEventListener('beforeunload', handleBeforeUnload)
    }
  })

  // Hook router navigation guard only when mounted inside an active RouterView
  // to avoid "[Vue Router warn]: onBeforeRouteLeave must be called in a component mounted by a RouterView"
  const instance = getCurrentInstance()
  if (instance) {
    const routeRecordRef = inject(matchedRouteKey, null)
    if (routeRecordRef && (routeRecordRef as any).value) {
      try {
        onBeforeRouteLeave(() => {
          if (!dirty.value) {
            return true
          }
          return confirmDiscard()
        })
      } catch {
        // Graceful fallback
      }
    }
  }

  return {
    confirmDiscard,
  }
}
