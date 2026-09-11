<template>
  <input
    :value="modelValue"
    :type="type"
    :disabled="disabled"
    :class="cn(
      'flex min-h-[44px] sm:min-h-[36px] w-full rounded-md border border-border-subtle bg-surface-base px-3 py-2 text-xs sm:text-sm text-text-main placeholder:text-text-muted transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium focus:outline-hidden focus-visible:ring-2 focus-visible:ring-accent focus-visible:border-transparent disabled:cursor-not-allowed disabled:opacity-50',
      props.class
    )"
    @input="onInput"
  />
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    modelValue?: string | number
    type?: string
    disabled?: boolean
    class?: HTMLAttributes['class']
  }>(),
  {
    modelValue: '',
    type: 'text',
    disabled: false,
    class: undefined
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', val: string | number): void
}>()

function onInput(event: Event) {
  emit('update:modelValue', (event.target as HTMLInputElement).value)
}
</script>
