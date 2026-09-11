<template>
  <DialogPortal>
    <SheetOverlay />
    <DialogContent
      v-bind="forwarded"
      :class="cn(
        'fixed z-50 flex flex-col bg-surface-base text-text-main shadow-md highlight-top duration-200 focus:outline-hidden',
        sideClasses,
        props.class
      )"
    >
      <slot />
      <DialogClose
        class="absolute right-4 top-4 flex min-h-[44px] min-w-[44px] items-center justify-center rounded-lg text-text-muted transition-colors hover:bg-surface-hover hover:text-text-main focus:outline-hidden focus-visible:ring-2 focus-visible:ring-accent disabled:pointer-events-none cursor-pointer"
      >
        <X class="h-4 w-4" aria-hidden="true" />
        <span class="sr-only">关闭</span>
      </DialogClose>
    </DialogContent>
  </DialogPortal>
</template>

<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import {
  DialogPortal,
  DialogContent,
  DialogClose,
  type DialogContentEmits,
  type DialogContentProps,
  useForwardPropsEmits,
} from 'reka-ui'
import { X } from 'lucide-vue-next'
import { cn } from '@/lib/utils'
import SheetOverlay from './SheetOverlay.vue'

interface Props extends DialogContentProps {
  side?: 'top' | 'bottom' | 'left' | 'right'
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  side: 'right',
})

const emits = defineEmits<DialogContentEmits>()
const forwarded = useForwardPropsEmits(props, emits)

const sideClasses = computed(() => {
  switch (props.side) {
    case 'left':
      return 'inset-y-0 left-0 h-full w-full max-w-[320px] border-r border-border-subtle'
    case 'top':
      return 'inset-x-0 top-0 border-b border-border-subtle'
    case 'bottom':
      return 'inset-x-0 bottom-0 max-h-[88vh] rounded-t-lg border-t border-border-subtle pb-safe'
    case 'right':
    default:
      return 'inset-y-0 right-0 h-full w-full sm:max-w-[480px] border-l border-border-subtle pb-safe'
  }
})
</script>
