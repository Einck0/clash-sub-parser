<template>
  <AlertDialogPortal>
    <AlertDialogOverlay />
    <AlertDialogContent
      v-bind="forwarded"
      :class="cn(
        'fixed left-[50%] top-[50%] z-50 flex w-[calc(100%-32px)] max-w-md translate-x-[-50%] translate-y-[-50%] flex-col rounded-lg border border-border-strong bg-surface-base p-6 text-text-main shadow-md highlight-top duration-200 focus:outline-hidden',
        props.class
      )"
    >
      <slot />
    </AlertDialogContent>
  </AlertDialogPortal>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import {
  AlertDialogPortal,
  AlertDialogContent,
  type AlertDialogContentEmits,
  type AlertDialogContentProps,
  useForwardPropsEmits,
} from 'reka-ui'
import { cn } from '@/lib/utils'
import AlertDialogOverlay from './AlertDialogOverlay.vue'

interface Props extends AlertDialogContentProps {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
const emits = defineEmits<AlertDialogContentEmits>()

const forwarded = useForwardPropsEmits(props, emits)
</script>
