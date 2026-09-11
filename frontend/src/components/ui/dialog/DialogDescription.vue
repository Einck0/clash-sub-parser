<template>
  <DialogDescription
    v-bind="props"
    :class="cn('text-xs sm:text-sm text-text-muted leading-relaxed', props.class)"
  >
    <slot />
  </DialogDescription>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { DialogDescription, type DialogDescriptionProps } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props extends DialogDescriptionProps {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>
