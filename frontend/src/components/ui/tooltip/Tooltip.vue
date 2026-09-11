<template>
  <TooltipRoot
    :open="open"
    :default-open="defaultOpen"
    :delay-duration="delayDuration"
    :disable-hoverable-content="disableHoverableContent"
    @update:open="$emit('update:open', $event)"
  >
    <slot />
  </TooltipRoot>
</template>

<script setup lang="ts">
import { TooltipRoot, type TooltipRootEmits, type TooltipRootProps } from 'reka-ui'

const props = defineProps<TooltipRootProps>()
defineEmits<TooltipRootEmits>()
</script>
