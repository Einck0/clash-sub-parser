<script setup lang="ts">
import type { ToastTone } from './toast'

withDefaults(defineProps<{
  label: string
  tone?: ToastTone
  outline?: boolean
}>(), { tone: 'info', outline: false })
</script>

<template>
  <span class="badge gap-1.5 whitespace-nowrap h-auto min-h-[1.5rem] py-0.5 px-2.5 text-xs leading-normal" :class="[`badge-${tone}`, { 'badge-outline': outline }]">
    <span class="h-1.5 w-1.5 rounded-full bg-current shrink-0" aria-hidden="true" />
    {{ label }}
  </span>
</template>
