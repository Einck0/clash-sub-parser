<template>
  <TabsTrigger
    v-bind="props"
    :class="cn(
      'inline-flex min-h-[36px] min-w-[44px] items-center justify-center whitespace-nowrap rounded-sm px-3 py-1.5 text-xs sm:text-sm font-medium transition-colors cursor-pointer focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent disabled:pointer-events-none disabled:opacity-50 data-[state=active]:bg-surface-base data-[state=active]:text-text-main data-[state=active]:shadow-xs',
      props.class
    )"
  >
    <slot />
  </TabsTrigger>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { TabsTrigger, type TabsTriggerProps } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props extends TabsTriggerProps {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>
