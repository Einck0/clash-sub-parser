<template>
  <AlertDialogTitle
    v-bind="props"
    :class="cn('text-base font-semibold tracking-wide text-text-main', props.class)"
  >
    <slot />
  </AlertDialogTitle>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { AlertDialogTitle, type AlertDialogTitleProps } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props extends AlertDialogTitleProps {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>
