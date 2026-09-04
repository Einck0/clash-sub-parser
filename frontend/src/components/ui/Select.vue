<template>
  <div class="relative w-full inline-block">
    <select
      :id="id"
      :value="modelValue"
      :disabled="disabled"
      :class="[
        'w-full appearance-none rounded-md border bg-surface px-3 py-2 pr-9 text-xs sm:text-sm text-text-main transition-colors min-h-[44px] sm:min-h-[36px] cursor-pointer',
        'focus:outline-hidden focus:ring-1',
        mono ? 'font-mono tabular-nums' : '',
        error
          ? 'border-status-danger focus:border-status-danger focus:ring-status-danger'
          : 'border-border-subtle focus:border-accent focus:ring-accent',
        disabled ? 'cursor-not-allowed opacity-60 bg-surface-base' : '',
      ]"
      :aria-invalid="error ? 'true' : undefined"
      @change="$emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
    >
      <option v-if="placeholder" value="" disabled selected>
        {{ placeholder }}
      </option>
      <slot>
        <option
          v-for="opt in options"
          :key="String(opt.value)"
          :value="opt.value"
          :disabled="opt.disabled"
        >
          {{ opt.label }}
        </option>
      </slot>
    </select>
    <ChevronDown
      class="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-text-muted"
      aria-hidden="true"
    />
  </div>
</template>

<script setup lang="ts">
import { ChevronDown } from 'lucide-vue-next'
import type { SelectOption } from './types'

withDefaults(
  defineProps<{
    modelValue: string | number
    options?: SelectOption[]
    placeholder?: string
    disabled?: boolean
    error?: string
    mono?: boolean
    id?: string
  }>(),
  {
    options: () => [],
    placeholder: undefined,
    disabled: false,
    error: undefined,
    mono: false,
    id: undefined,
  }
)

defineEmits<{
  (e: 'update:modelValue', value: string | number): void
  (e: 'change', value: string | number): void
}>()
</script>
