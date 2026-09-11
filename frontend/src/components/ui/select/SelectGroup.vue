<template>
  <SelectGroup v-bind="props" :class="cn('p-1 w-full', props.class)">
    <slot />
  </SelectGroup>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { SelectGroup, type SelectGroupProps } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props extends SelectGroupProps {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>
