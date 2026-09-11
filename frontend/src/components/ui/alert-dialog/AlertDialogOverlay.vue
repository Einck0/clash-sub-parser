<template>
  <AlertDialogOverlay
    v-bind="props"
    :class="cn(
      'fixed inset-0 z-50 bg-black/80 transition-opacity duration-200',
      props.class
    )"
  />
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { AlertDialogOverlay, type AlertDialogOverlayProps } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props extends AlertDialogOverlayProps {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>
