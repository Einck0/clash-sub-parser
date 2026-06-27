<template>
  <div class="skeleton" :class="[`skeleton-${variant}`, { 'skeleton-animated': animated }]">
    <div v-if="variant === 'card'" class="skeleton-card">
      <div class="skeleton-line skeleton-line-lg"></div>
      <div class="skeleton-line skeleton-line-md"></div>
      <div class="skeleton-line skeleton-line-sm"></div>
    </div>
    <div v-else-if="variant === 'table'" class="skeleton-table">
      <div v-for="i in rows" :key="i" class="skeleton-row">
        <div class="skeleton-line skeleton-line-sm"></div>
        <div class="skeleton-line skeleton-line-md"></div>
        <div class="skeleton-line skeleton-line-lg"></div>
      </div>
    </div>
    <div v-else class="skeleton-lines">
      <div v-for="i in rows" :key="i" class="skeleton-line" :style="{ width: widths[(i - 1) % widths.length] }"></div>
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

<style scoped>
.skeleton {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.skeleton-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 16px;
  border: 1px solid var(--border);
  border-radius: 10px;
}
.skeleton-table {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.skeleton-row {
  display: flex;
  gap: 12px;
  padding: 8px 0;
  border-bottom: 1px solid var(--border);
}
.skeleton-line {
  height: 14px;
  border-radius: 4px;
  background: var(--surface-2);
}
.skeleton-line-lg { width: 70%; }
.skeleton-line-md { width: 50%; }
.skeleton-line-sm { width: 30%; }
.skeleton-animated .skeleton-line {
  background: linear-gradient(90deg, var(--surface-2) 25%, var(--surface-3) 50%, var(--surface-2) 75%);
  background-size: 200% 100%;
  animation: skeleton-shimmer 1.5s infinite;
}
@keyframes skeleton-shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}
</style>
