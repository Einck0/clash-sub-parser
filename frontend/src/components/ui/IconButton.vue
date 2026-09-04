<template>
  <button
    :type="type"
    :disabled="disabled || loading"
    :class="[
      'inline-flex items-center justify-center select-none cursor-pointer transition-colors duration-150 shrink-0',
      'active:scale-[0.98]',
      'focus:outline-hidden focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-1',
      'disabled:cursor-not-allowed disabled:opacity-60 disabled:active:scale-100',
      sizeClasses,
      variantClasses,
    ]"
    :aria-label="label"
    :title="label"
    :aria-busy="loading ? 'true' : undefined"
  >
    <Loader2 v-if="loading" class="h-4 w-4 animate-spin" aria-hidden="true" />
    <component :is="icon" v-else-if="icon" class="h-4 w-4" aria-hidden="true" />
    <slot v-else />
  </button>
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue'
import { Loader2 } from 'lucide-vue-next'
import type { ButtonVariant, ButtonSize } from './types'

const props = withDefaults(
  defineProps<{
    label: string
    icon?: Component
    variant?: ButtonVariant
    size?: ButtonSize
    disabled?: boolean
    loading?: boolean
    type?: 'button' | 'submit' | 'reset'
  }>(),
  {
    icon: undefined,
    variant: 'ghost',
    size: 'md',
    disabled: false,
    loading: false,
    type: 'button',
  }
)

const variantClasses = computed(() => {
  switch (props.variant) {
    case 'primary':
      return 'bg-accent text-white hover:bg-accent-hover active:bg-accent-hover border border-transparent shadow-xs'
    case 'danger':
      return 'bg-status-danger text-white hover:bg-red-600 active:bg-red-700 border border-transparent shadow-xs'
    case 'secondary':
      return 'bg-surface-hover text-text-main hover:bg-surface-active hover:text-white border border-border-subtle'
    case 'ghost':
    default:
      return 'bg-transparent text-text-muted hover:text-text-main hover:bg-surface-hover border border-transparent'
  }
})

const sizeClasses = computed(() => {
  switch (props.size) {
    case 'sm':
      return 'h-9 w-9 sm:h-8 sm:w-8 min-h-[44px] min-w-[44px] sm:min-h-[32px] sm:min-w-[32px] rounded-md'
    case 'lg':
      return 'h-11 w-11 min-h-[44px] min-w-[44px] rounded-lg'
    case 'md':
    default:
      return 'h-11 w-11 sm:h-9 sm:w-9 min-h-[44px] min-w-[44px] sm:min-h-[36px] sm:min-w-[36px] rounded-md'
  }
})
</script>
