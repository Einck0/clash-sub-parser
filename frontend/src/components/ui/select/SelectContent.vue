<template>
  <SelectPortal>
    <SelectContent
      v-bind="props"
      :class="cn(
        'relative z-50 max-h-96 min-w-32 overflow-hidden rounded-md border border-border-subtle bg-surface-panel text-text-main shadow-md transition-opacity duration-150',
        props.class
      )"
      :position="position"
    >
      <SelectViewport class="p-1">
        <slot />
      </SelectViewport>
    </SelectContent>
  </SelectPortal>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { SelectPortal, SelectContent, SelectViewport, type SelectContentProps } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props extends SelectContentProps {
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  position: 'popper',
  sideOffset: 4,
})
</script>
