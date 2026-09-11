<template>
  <SelectTrigger
    v-bind="props"
    :class="cn(
      'flex min-h-[44px] sm:min-h-[36px] w-full items-center justify-between rounded-md border border-border-subtle bg-surface-base px-3 py-2 text-xs sm:text-sm text-text-main shadow-xs transition-colors placeholder:text-text-muted focus:outline-hidden focus-visible:ring-2 focus-visible:ring-accent disabled:cursor-not-allowed disabled:opacity-50 [&>span]:line-clamp-1',
      props.class
    )"
  >
    <slot />
    <SelectIcon as-child>
      <ChevronDown class="h-4 w-4 shrink-0 text-text-muted transition-transform" aria-hidden="true" />
    </SelectIcon>
  </SelectTrigger>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { SelectTrigger, SelectIcon, type SelectTriggerProps } from 'reka-ui'
import { ChevronDown } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

interface Props extends SelectTriggerProps {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>
