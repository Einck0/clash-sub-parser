<template>
  <DropdownMenuPortal>
    <DropdownMenuContent
      v-bind="props"
      :class="cn(
        'z-50 min-w-32 overflow-hidden rounded-md border border-border-subtle bg-surface-panel p-1 text-text-main shadow-md transition-opacity duration-150',
        props.class
      )"
    >
      <slot />
    </DropdownMenuContent>
  </DropdownMenuPortal>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { DropdownMenuPortal, DropdownMenuContent, type DropdownMenuContentProps } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props extends DropdownMenuContentProps {
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  sideOffset: 4,
})
</script>
