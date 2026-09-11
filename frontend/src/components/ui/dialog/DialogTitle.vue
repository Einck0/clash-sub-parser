<template>
  <DialogTitle
    v-bind="props"
    :class="cn('text-base font-semibold tracking-wide text-text-main', props.class)"
  >
    <slot />
  </DialogTitle>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { DialogTitle, type DialogTitleProps } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props extends DialogTitleProps {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>
