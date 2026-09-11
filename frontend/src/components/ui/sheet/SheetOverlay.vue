<template>
  <DialogOverlay
    v-bind="props"
    :class="cn(
      'fixed inset-0 z-50 bg-black/80 transition-opacity duration-200',
      props.class
    )"
  />
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { DialogOverlay, type DialogOverlayProps } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props extends DialogOverlayProps {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>
