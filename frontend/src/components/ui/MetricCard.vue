<template>
  <div
    :class="[
      'flex flex-col justify-between p-4 rounded-xl border transition-all duration-200 backdrop-blur-md',
      active
        ? 'border-blue-500/50 bg-blue-950/20 shadow-xs shadow-blue-500/10 ring-1 ring-blue-500/30'
        : 'border-white/10 bg-slate-900/60 hover:border-white/20 hover:bg-slate-900/80',
      clickable ? 'cursor-pointer select-none' : '',
      wide ? 'col-span-full md:col-span-2' : ''
    ]"
    :role="clickable ? 'button' : undefined"
    :tabindex="clickable ? 0 : undefined"
    @click="handleClick"
    @keydown.enter="handleClick"
  >
    <!-- Top label and indicator -->
    <div class="flex items-center justify-between gap-2">
      <span class="text-xs font-mono font-medium uppercase tracking-wider text-slate-400 truncate">
        {{ label }}
      </span>
      <div class="flex items-center gap-1.5">
        <span
          v-if="status"
          :class="[
            'h-2 w-2 rounded-full ring-2 ring-white/5 transition-transform duration-200',
            statusDotClass
          ]"
        />
        <slot name="extra" />
      </div>
    </div>

    <!-- Main Value and Typography -->
    <div class="flex items-baseline gap-2 mt-2">
      <span class="text-2xl font-mono font-bold text-white tracking-tight truncate">
        {{ value }}
      </span>
      <span v-if="unit" class="text-xs font-mono text-slate-400">
        {{ unit }}
      </span>
      <slot name="value-extra" />
    </div>

    <!-- Subtext or Long Description -->
    <div v-if="description" class="text-xs text-slate-400 mt-2 leading-relaxed">
      {{ description }}
    </div>
    <div v-else-if="subtext || $slots.subtext" class="text-xs font-mono text-slate-400 mt-1 truncate">
      <slot name="subtext">
        {{ subtext }}
      </slot>
    </div>
    <div v-else class="h-2"></div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

/**
 * MetricCard component props adhering to ui-ux-pro-max design tokens.
 */
export interface MetricCardProps {
  label: string
  value: string | number
  unit?: string
  subtext?: string
  description?: string
  status?: 'success' | 'warning' | 'danger' | 'info' | 'neutral'
  active?: boolean
  clickable?: boolean
  wide?: boolean
}

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
      return 'bg-emerald-400 shadow-xs shadow-emerald-500/50 animate-pulse'
    case 'warning':
      return 'bg-amber-400 shadow-xs shadow-amber-500/50'
    case 'danger':
      return 'bg-rose-400 shadow-xs shadow-rose-500/50'
    case 'info':
      return 'bg-blue-400 shadow-xs shadow-blue-500/50'
    case 'neutral':
    default:
      return 'bg-slate-500'
  }
})

function handleClick(e: MouseEvent | KeyboardEvent) {
  if (props.clickable) {
    emit('click', e)
  }
}
</script>
