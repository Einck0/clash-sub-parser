<template>
  <AlertDialogCancel
    v-bind="props"
    :class="cn(
      'inline-flex min-h-[44px] sm:min-h-[36px] items-center justify-center rounded-md border border-border-subtle bg-surface-hover px-4 py-2 text-xs sm:text-sm font-medium text-text-main hover:bg-surface-active transition-colors cursor-pointer active:scale-[0.98] focus:outline-hidden focus-visible:ring-2 focus-visible:ring-accent',
      props.class
    )"
  >
    <slot />
  </AlertDialogCancel>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { AlertDialogCancel, type AlertDialogCancelProps } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props extends AlertDialogCancelProps {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>
