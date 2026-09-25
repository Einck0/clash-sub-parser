import { computed, type Ref } from 'vue'

export function deriveColumns(width: number, minCardWidth: number, gap: number): number {
  if (width <= 0) return 1
  return Math.max(1, Math.floor((width + gap) / (minCardWidth + gap)))
}

export function useResponsiveColumns(
  width: Ref<number>,
  options: { minCardWidth?: number; gap?: number } = {},
) {
  const minCardWidth = options.minCardWidth ?? 288
  const gap = options.gap ?? 16

  const columns = computed(() => deriveColumns(width.value, minCardWidth, gap))

  return { columns, minCardWidth, gap }
}
