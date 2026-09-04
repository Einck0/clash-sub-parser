<template>
  <div
    :class="[
      'flex flex-col justify-between p-3.5 sm:p-4 rounded-lg border transition-colors duration-150',
      active
        ? 'border-accent/50 bg-accent-subtle ring-1 ring-accent/30'
        : 'border-border-subtle bg-surface-base hover:border-border-strong hover:bg-surface-hover',
      clickable ? 'cursor-pointer select-none focus:outline-hidden focus-visible:ring-2 focus-visible:ring-accent' : '',
      wide ? 'col-span-full md:col-span-2' : ''
    ]"
    :role="clickable ? 'button' : undefined"
    :tabindex="clickable ? 0 : undefined"
    @click="handleClick"
    @keydown.enter="handleClick"
    @keydown.space.prevent="handleClick"
  >
    <!-- Top label and indicator -->
    <div class="flex items-center justify-between gap-2">
      <span class="text-xs font-mono font-medium uppercase tracking-wider text-text-muted truncate">
        {{ label }}
      </span>
      <div class="flex items-center gap-1.5">
        <span
          v-if="status"
          :class="[
            'h-2 w-2 rounded-full transition-transform duration-150',
            statusDotClass
          ]"
        />
        <slot name="extra" />
      </div>
    </div>

    <!-- Main Value and Typography -->
    <div class="flex items-baseline gap-2 mt-2">
      <span class="text-2xl font-mono tabular-nums font-bold text-text-main tracking-tight truncate">
        {{ value }}
      </span>
      <span v-if="unit" class="text-xs font-mono tabular-nums text-text-muted">
        {{ unit }}
      </span>
      <slot name="value-extra" />
    </div>

    <!-- Subtext or Long Description -->
    <div v-if="description" class="text-xs text-text-muted mt-2 leading-relaxed">
      {{ description }}
    </div>
    <div v-else-if="subtext || $slots.subtext" class="text-xs font-mono text-text-muted mt-1 truncate">
      <slot name="subtext">
        {{ subtext }}
      </slot>
    </div>
    <div v-else class="h-2"></div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { MetricCardProps } from './types'

const props = withDefaults(defineProps<MetricCardProps>(), {
  unit: '',
  subtext: '',
  description: '',
  status: undefined,
  active: false,
  clickable: false,
  wide: false,
})

const emit = defineEmits<{
  (e: 'click', event: MouseEvent | KeyboardEvent): void
}>()

const statusDotClass = computed(() => {
  switch (props.status) {
    case 'success':
      return 'bg-status-success ring-1 ring-status-success/30 animate-pulse'
    case 'warning':
      return 'bg-status-warning ring-1 ring-status-warning/30'
    case 'danger':
      return 'bg-status-danger ring-1 ring-status-danger/30'
    case 'info':
      return 'bg-status-info ring-1 ring-status-info/30'
    case 'neutral':
    default:
      return 'bg-text-sub'
  }
})

function handleClick(e: MouseEvent | KeyboardEvent) {
  if (props.clickable) {
    emit('click', e)
  }
}
</script>
