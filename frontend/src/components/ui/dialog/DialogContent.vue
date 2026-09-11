<template>
  <DialogPortal>
    <DialogOverlay />
    <DialogContent
      v-bind="forwarded"
      :class="cn(
        'fixed left-[50%] top-[50%] z-50 flex w-[calc(100%-32px)] max-w-lg translate-x-[-50%] translate-y-[-50%] flex-col rounded-lg border border-border-subtle bg-surface-base p-6 text-text-main shadow-md highlight-top duration-200 focus:outline-hidden',
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
import type { HTMLAttributes } from 'vue'
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
import DialogOverlay from './DialogOverlay.vue'

interface Props extends DialogContentProps {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
const emits = defineEmits<DialogContentEmits>()

const forwarded = useForwardPropsEmits(props, emits)
</script>
