<template>
  <TooltipPortal>
    <TooltipContent
      :side="side"
      :side-offset="sideOffset"
      :align="align"
      :class="cn(
        'z-50 overflow-hidden rounded-md border border-border-subtle bg-surface-panel px-3 py-1.5 text-xs text-text-main shadow-md transition-opacity duration-150',
        props.class
      )"
    >
      <slot />
    </TooltipContent>
  </TooltipPortal>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { TooltipPortal, TooltipContent } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    side?: 'top' | 'right' | 'bottom' | 'left'
    sideOffset?: number
    align?: 'start' | 'center' | 'end'
    class?: HTMLAttributes['class']
  }>(),
  {
    side: 'top',
    sideOffset: 4,
    align: 'center',
    class: undefined
  }
)
</script>
