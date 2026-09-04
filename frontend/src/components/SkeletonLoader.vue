<template>
  <div class="flex flex-col gap-2.5" :class="{ 'animate-pulse': animated }">
    <div v-if="variant === 'card'" class="flex flex-col gap-2 p-4 border border-border-subtle rounded-lg bg-surface-base">
      <div class="h-3.5 rounded bg-surface-active w-[70%]" />
      <div class="h-3.5 rounded bg-surface-active w-[50%]" />
      <div class="h-3.5 rounded bg-surface-active w-[30%]" />
    </div>
    <div v-else-if="variant === 'table'" class="flex flex-col gap-1.5">
      <div v-for="i in rows" :key="i" class="flex gap-3 py-2 border-b border-border-subtle">
        <div class="h-3.5 rounded bg-surface-active w-[30%]" />
        <div class="h-3.5 rounded bg-surface-active w-[50%]" />
        <div class="h-3.5 rounded bg-surface-active w-[70%]" />
      </div>
    </div>
    <div v-else class="flex flex-col gap-2">
      <div
        v-for="i in rows"
        :key="i"
        class="h-3.5 rounded bg-surface-active"
        :style="{ width: widths[(i - 1) % widths.length] }"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  variant?: 'card' | 'table' | 'lines'
  rows?: number
  animated?: boolean
}>(), {
  variant: 'lines',
  rows: 3,
  animated: true,
})

const widths = ['100%', '80%', '60%', '90%', '70%']
</script>
