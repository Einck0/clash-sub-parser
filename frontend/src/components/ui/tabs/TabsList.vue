<template>
  <TabsList
    v-bind="props"
    :class="cn(
      'inline-flex h-10 items-center justify-center rounded-md bg-surface-hover p-1 text-text-muted border border-border-subtle',
      props.class
    )"
  >
    <slot />
  </TabsList>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { TabsList, type TabsListProps } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props extends TabsListProps {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>
