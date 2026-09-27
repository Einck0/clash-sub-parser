<script setup lang="ts">
import { computed } from 'vue'
import type { ToastTone } from './toast'

const props = withDefaults(defineProps<{
  label: string
  tone?: ToastTone
  outline?: boolean
  pulse?: boolean
}>(), { tone: 'info', outline: false, pulse: false })

const isPulsing = computed(
  () => props.pulse || props.label.includes('检测中') || props.label.includes('队列')
)
</script>

<template>
  <span
    class="badge gap-1.5 whitespace-nowrap h-auto min-h-[1.5rem] py-0.5 px-2.5 text-xs leading-normal"
    :class="[
      `badge-${tone}`,
      {
        'badge-outline': outline,
        'animate-pulse font-semibold ring-1 ring-current/30': isPulsing,
      },
    ]"
  >
    <span class="relative flex h-1.5 w-1.5 shrink-0" aria-hidden="true">
      <span
        v-if="isPulsing"
        class="animate-ping absolute inline-flex h-full w-full rounded-full bg-current opacity-75"
      />
      <span class="relative inline-flex rounded-full h-1.5 w-1.5 bg-current" />
    </span>
    {{ label }}
  </span>
</template>
