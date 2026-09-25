import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import { deriveColumns, useResponsiveColumns } from './useResponsiveColumns'

describe('useResponsiveColumns and deriveColumns', () => {
  const GAP = 16
  const MIN_CARD_WIDTH = 288

  it('derives 1 column on narrow mobile widths', () => {
    expect(deriveColumns(320, MIN_CARD_WIDTH, GAP)).toBe(1)
    expect(deriveColumns(375, MIN_CARD_WIDTH, GAP)).toBe(1)
    expect(deriveColumns(390, MIN_CARD_WIDTH, GAP)).toBe(1)
  })

  it('derives 2 or more columns naturally on wider container widths', () => {
    expect(deriveColumns(600, MIN_CARD_WIDTH, GAP)).toBe(2)
    expect(deriveColumns(920, MIN_CARD_WIDTH, GAP)).toBe(3)
    expect(deriveColumns(1240, MIN_CARD_WIDTH, GAP)).toBe(4)
  })

  it('safely falls back to 1 column for zero or negative widths', () => {
    expect(deriveColumns(0, MIN_CARD_WIDTH, GAP)).toBe(1)
    expect(deriveColumns(-100, MIN_CARD_WIDTH, GAP)).toBe(1)
  })

  it('adapts reactively with useResponsiveColumns composable', () => {
    const widthRef = ref(375)
    const { columns } = useResponsiveColumns(widthRef, { minCardWidth: 288, gap: 16 })

    expect(columns.value).toBe(1)

    widthRef.value = 650
    expect(columns.value).toBe(2)

    widthRef.value = 1000
    expect(columns.value).toBe(3)
  })
})
