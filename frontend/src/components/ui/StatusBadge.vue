<template>
  <span
    :class="[
      'inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-mono font-medium tracking-tight border',
      variantClass
    ]"
  >
    <span :class="['h-1.5 w-1.5 rounded-full', dotClass]"></span>
    <slot>{{ text }}</slot>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    type?: 'success' | 'warning' | 'danger' | 'info' | 'neutral'
    text?: string
  }>(),
  {
    type: 'neutral',
    text: '',
  }
)

const variantClass = computed(() => {
  switch (props.type) {
    case 'success':
      return 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20'
    case 'warning':
      return 'bg-amber-500/10 text-amber-400 border-amber-500/20'
    case 'danger':
      return 'bg-rose-500/10 text-rose-400 border-rose-500/20'
    case 'info':
      return 'bg-cyan-500/10 text-cyan-400 border-cyan-500/20'
    default:
      return 'bg-slate-800/60 text-slate-300 border-slate-700/50'
  }
})

const dotClass = computed(() => {
  switch (props.type) {
    case 'success':
      return 'bg-emerald-400 animate-pulse'
    case 'warning':
      return 'bg-amber-400'
    case 'danger':
      return 'bg-rose-400'
    case 'info':
      return 'bg-cyan-400'
    default:
      return 'bg-slate-400'
  }
})
</script>
