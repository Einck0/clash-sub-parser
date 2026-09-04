<template>
  <div class="flex flex-col gap-1.5 w-full">
    <div v-if="label || $slots.label" class="flex items-center justify-between">
      <label
        v-if="label || $slots.label"
        :for="inputId"
        class="text-xs font-mono font-medium tracking-wide text-text-muted flex items-center gap-1 select-none"
      >
        <slot name="label">{{ label }}</slot>
        <span v-if="required" class="text-status-danger" aria-hidden="true">*</span>
      </label>
      <slot name="extra" />
    </div>

    <div class="relative w-full">
      <slot :id="inputId" :has-error="Boolean(error)" :is-mono="mono">
        <input
          :id="inputId"
          :type="type"
          :value="modelValue"
          :placeholder="placeholder"
          :disabled="disabled"
          :required="required"
          :class="[
            'w-full rounded-md border bg-surface px-3 py-2 text-xs sm:text-sm text-text-main placeholder-text-muted transition-colors min-h-[44px] sm:min-h-[36px]',
            'focus:outline-hidden focus:ring-1',
            mono ? 'font-mono tabular-nums' : '',
            error
              ? 'border-status-danger focus:border-status-danger focus:ring-status-danger'
              : 'border-border-subtle focus:border-accent focus:ring-accent',
            disabled ? 'cursor-not-allowed opacity-60 bg-surface-base' : '',
          ]"
          :aria-invalid="error ? 'true' : undefined"
          :aria-describedby="error ? `${inputId}-error` : hint ? `${inputId}-hint` : undefined"
          @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
          @change="$emit('change', ($event.target as HTMLInputElement).value)"
        />
      </slot>
    </div>

    <p
      v-if="error"
      :id="`${inputId}-error`"
      role="alert"
      class="flex items-center gap-1 text-xs text-status-danger font-mono mt-0.5"
    >
      <AlertCircle class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
      <span>{{ error }}</span>
    </p>

    <p
      v-else-if="hint || $slots.hint"
      :id="`${inputId}-hint`"
      class="text-xs text-text-sub font-mono mt-0.5"
    >
      <slot name="hint">{{ hint }}</slot>
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { AlertCircle } from 'lucide-vue-next'

const props = withDefaults(
  defineProps<{
    label?: string
    id?: string
    error?: string
    hint?: string
    required?: boolean
    mono?: boolean
    modelValue?: string | number
    type?: string
    placeholder?: string
    disabled?: boolean
  }>(),
  {
    label: undefined,
    id: undefined,
    error: undefined,
    hint: undefined,
    required: false,
    mono: false,
    modelValue: '',
    type: 'text',
    placeholder: '',
    disabled: false,
  }
)

defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'change', value: string): void
}>()

const generatedId = `field-${Math.random().toString(36).slice(2, 9)}`
const inputId = computed(() => props.id || generatedId)
</script>
