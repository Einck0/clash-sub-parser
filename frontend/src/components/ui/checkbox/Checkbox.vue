<template>
  <CheckboxRoot
    :model-value="modelValue"
    :disabled="disabled"
    :class="cn(
      'peer flex h-5 w-5 min-h-[20px] min-w-[20px] shrink-0 items-center justify-center rounded-md border border-border-strong bg-surface-base text-white transition-colors cursor-pointer focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:bg-accent data-[state=checked]:border-accent',
      props.class
    )"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <CheckboxIndicator class="flex items-center justify-center text-current">
      <Check class="h-3.5 w-3.5" aria-hidden="true" />
    </CheckboxIndicator>
  </CheckboxRoot>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { CheckboxRoot, CheckboxIndicator } from 'reka-ui'
import { Check } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    modelValue?: boolean | 'indeterminate'
    disabled?: boolean
    class?: HTMLAttributes['class']
  }>(),
  {
    modelValue: false,
    disabled: false,
    class: undefined
  }
)

defineEmits<{
  (e: 'update:modelValue', val: boolean | 'indeterminate'): void
}>()
</script>
