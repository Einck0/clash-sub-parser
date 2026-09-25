<script setup lang="ts">
withDefaults(defineProps<{
  lines?: number
  rounded?: boolean
}>(), { lines: 1, rounded: true })
</script>

<template>
  <div class="space-y-3" aria-busy="true" aria-label="Loading">
    <div
      v-for="line in lines"
      :key="line"
      class="skeleton h-4 w-full"
      :class="{ 'rounded-lg': rounded, 'max-w-xs': line === lines && lines > 1 }"
    />
  </div>
</template>
