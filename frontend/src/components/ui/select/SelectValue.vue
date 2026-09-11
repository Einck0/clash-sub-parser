<template>
  <SelectValue v-bind="props">
    <slot />
  </SelectValue>
</template>

<script setup lang="ts">
import { SelectValue, type SelectValueProps } from 'reka-ui'

const props = defineProps<SelectValueProps>()
</script>
