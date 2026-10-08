import { onUnmounted, watch, type Ref } from 'vue'

let activeLocks = 0
let originalOverflow: string | null = null

export function acquireBodyScrollLock(): void {
  if (typeof document === 'undefined') return
  if (activeLocks === 0) {
    originalOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
  }
  activeLocks++
}

export function releaseBodyScrollLock(): void {
  if (typeof document === 'undefined') return
  if (activeLocks > 0) {
    activeLocks--
  }
  if (activeLocks === 0 && originalOverflow !== null) {
    document.body.style.overflow = originalOverflow
    originalOverflow = null
  }
}

export function resetBodyScrollLockForTesting(): void {
  activeLocks = 0
  originalOverflow = null
}

export function useBodyScrollLock(isLocked?: Ref<boolean> | (() => boolean)): {
  lock: () => void
  unlock: () => void
} {
  let isCurrentlyLockedByThis = false

  const lock = () => {
    if (!isCurrentlyLockedByThis) {
      isCurrentlyLockedByThis = true
      acquireBodyScrollLock()
    }
  }

  const unlock = () => {
    if (isCurrentlyLockedByThis) {
      isCurrentlyLockedByThis = false
      releaseBodyScrollLock()
    }
  }

  if (isLocked) {
    watch(
      isLocked,
      (val) => {
        if (val) {
          lock()
        } else {
          unlock()
        }
      },
      { immediate: true }
    )
  }

  onUnmounted(() => {
    unlock()
  })

  return { lock, unlock }
}
