import { computed, ref } from 'vue'
import { useElementBounding, useWindowSize } from '@vueuse/core'

/**
 * Global reference to the mobile floating Dock element.
 * Bound to the actual mobile nav element in App.vue.
 */
export const dockRef = ref<HTMLElement | null>(null)
export const dockBounds = useElementBounding(dockRef)
const { width: windowWidth, height: windowHeight } = useWindowSize()

/**
 * Mathematical clearance composable.
 * Computes dynamic padding-bottom and CSS custom property for routed content container
 * so that the lowest interactive elements are guaranteed to clear the mobile floating dock.
 *
 * Clearance formula based on geometry:
 * Distance from viewport bottom to Dock top + minimum 48px (default 52px)
 * after last page element (including safe area).
 *
 * On desktop (viewport >= 768px): standard 32px (pb-8).
 */
export function useContentInset(extraOffset = 52) {
  const isMobile = computed(() => {
    if (typeof window === 'undefined') return false
    const w = windowWidth.value || window.innerWidth
    return w < 768
  })

  const paddingBottomPx = computed(() => {
    if (!isMobile.value) {
      return 32
    }

    const vh = (typeof window !== 'undefined' && window.innerHeight) ? window.innerHeight : (windowHeight.value || 844)
    const boundsHeight = dockBounds.height.value
    const boundsTop = dockBounds.top.value

    if (dockRef.value && boundsHeight > 0 && boundsTop > 0) {
      // Geometry: distance from viewport bottom to dock top
      const distanceToDockTop = Math.max(boundsHeight, vh - boundsTop)
      return Math.ceil(distanceToDockTop + extraOffset)
    }

    // Fallback in headless / jsdom before initial layout pass:
    // estimated 64px dock + 16px bottom float + extraOffset
    return Math.ceil(80 + extraOffset)
  })

  const style = computed(() => ({
    paddingBottom: `${paddingBottomPx.value}px`,
    '--content-dock-inset': `${paddingBottomPx.value}px`,
  }))

  return {
    dockRef,
    dockBounds,
    isMobile,
    paddingBottomPx,
    style,
  }
}
