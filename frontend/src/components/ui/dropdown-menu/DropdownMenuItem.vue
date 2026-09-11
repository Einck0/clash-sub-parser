<template>
  <DropdownMenuItem
    v-bind="props"
    :class="cn(
      'relative flex min-h-[36px] sm:min-h-[32px] cursor-pointer select-none items-center rounded-sm px-2 py-1.5 text-xs sm:text-sm text-text-main outline-hidden transition-colors data-[highlighted]:bg-surface-hover data-[disabled]:pointer-events-none data-[disabled]:opacity-50',
      props.class
    )"
  >
    <slot />
  </DropdownMenuItem>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { DropdownMenuItem, type DropdownMenuItemProps } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props extends DropdownMenuItemProps {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>
