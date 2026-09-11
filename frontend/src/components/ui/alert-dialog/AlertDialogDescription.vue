<template>
  <AlertDialogDescription
    v-bind="props"
    :class="cn('text-xs sm:text-sm text-text-muted leading-relaxed whitespace-pre-line', props.class)"
  >
    <slot />
  </AlertDialogDescription>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { AlertDialogDescription, type AlertDialogDescriptionProps } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props extends AlertDialogDescriptionProps {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>
