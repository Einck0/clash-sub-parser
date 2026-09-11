<template>
  <SelectSeparator v-bind="props" :class="cn('-mx-1 my-1 h-px bg-border-subtle', props.class)" />
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { SelectSeparator, type SelectSeparatorProps } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props extends SelectSeparatorProps {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>
