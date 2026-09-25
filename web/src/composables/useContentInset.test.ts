// @vitest-environment jsdom
import { describe, expect, it, beforeEach } from 'vitest'
import { useContentInset, dockRef } from './useContentInset'

describe('useContentInset dynamic dock inset composable', () => {
  beforeEach(() => {
    dockRef.value = null
  })

  it('provides desktop regular 32px bottom spacing when viewport is desktop size', () => {
    Object.defineProperty(window, 'innerWidth', { writable: true, configurable: true, value: 1024 })
    Object.defineProperty(window, 'innerHeight', { writable: true, configurable: true, value: 768 })
    window.dispatchEvent(new Event('resize'))

    const { isMobile, paddingBottomPx, style } = useContentInset(52)

    expect(isMobile.value).toBe(false)
    expect(paddingBottomPx.value).toBe(32)
    expect(style.value.paddingBottom).toBe('32px')
    expect(style.value['--content-dock-inset']).toBe('32px')
  })

  it('provides mobile clearance exceeding 40px when viewport is mobile size', () => {
    Object.defineProperty(window, 'innerWidth', { writable: true, configurable: true, value: 390 })
    Object.defineProperty(window, 'innerHeight', { writable: true, configurable: true, value: 844 })
    window.dispatchEvent(new Event('resize'))

    const { isMobile, paddingBottomPx, style } = useContentInset(52)

    expect(isMobile.value).toBe(true)
    expect(paddingBottomPx.value).toBeGreaterThanOrEqual(92)
    expect(style.value.paddingBottom).toBe(`${paddingBottomPx.value}px`)
    expect(style.value['--content-dock-inset']).toBe(`${paddingBottomPx.value}px`)
  })
})
