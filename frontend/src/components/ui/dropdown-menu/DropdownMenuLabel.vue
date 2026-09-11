<template>
  <DropdownMenuLabel v-bind="props" :class="cn('px-2 py-1.5 text-xs font-semibold text-text-muted', props.class)">
    <slot />
  </DropdownMenuLabel>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { DropdownMenuLabel, type DropdownMenuLabelProps } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props extends DropdownMenuLabelProps {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>
