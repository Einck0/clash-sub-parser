<template>
  <SelectLabel v-bind="props" :class="cn('py-1.5 pl-8 pr-2 text-xs font-semibold text-text-muted', props.class)">
    <slot />
  </SelectLabel>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { SelectLabel, type SelectLabelProps } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props extends SelectLabelProps {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>
