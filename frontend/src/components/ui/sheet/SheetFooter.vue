<template>
  <div :class="cn('flex items-center justify-end gap-3 p-6 border-t border-border-subtle shrink-0 pb-safe', props.class)">
    <slot />
  </div>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>
