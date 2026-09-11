<template>
  <div :class="cn('flex flex-col space-y-1.5 p-6 border-b border-border-subtle shrink-0', props.class)">
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
