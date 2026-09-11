<template>
  <AlertDialogAction
    v-bind="props"
    :class="cn(
      'inline-flex min-h-[44px] sm:min-h-[36px] items-center justify-center rounded-md px-4 py-2 text-xs sm:text-sm font-medium text-white transition-colors cursor-pointer active:scale-[0.98] focus:outline-hidden focus-visible:ring-2 focus-visible:ring-accent shadow-xs',
      variant === 'danger'
        ? 'bg-status-danger hover:bg-status-danger/90 active:bg-status-danger/80'
        : 'bg-accent hover:bg-accent-hover active:bg-accent-hover',
      props.class
    )"
  >
    <slot />
  </AlertDialogAction>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { AlertDialogAction, type AlertDialogActionProps } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props extends AlertDialogActionProps {
  variant?: 'primary' | 'danger'
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'primary',
})
</script>
