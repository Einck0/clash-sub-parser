<template>
  <button
    :type="type"
    :disabled="disabled || loading"
    :class="[
      'inline-flex items-center justify-center font-medium select-none cursor-pointer transition-colors duration-150',
      'active:scale-[0.98]',
      'focus:outline-hidden focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-1',
      'disabled:cursor-not-allowed disabled:opacity-60 disabled:active:scale-100',
      sizeClasses,
      variantClasses,
    ]"
    :aria-label="ariaLabel"
    :aria-busy="loading ? 'true' : undefined"
  >
    <Loader2 v-if="loading" class="h-4 w-4 animate-spin shrink-0" aria-hidden="true" />
    <component :is="icon" v-else-if="icon" class="h-4 w-4 shrink-0" aria-hidden="true" />
    <slot name="prefix" />
    <span v-if="$slots.default" class="truncate">
      <slot />
    </span>
    <slot name="suffix" />
  </button>
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue'
import { Loader2 } from 'lucide-vue-next'
import type { ButtonVariant, ButtonSize } from './types'

const props = withDefaults(
  defineProps<{
    variant?: ButtonVariant
    size?: ButtonSize
    disabled?: boolean
    loading?: boolean
    type?: 'button' | 'submit' | 'reset'
    icon?: Component
    ariaLabel?: string
  }>(),
  {
    variant: 'secondary',
    size: 'md',
    disabled: false,
    loading: false,
    type: 'button',
    icon: undefined,
    ariaLabel: undefined,
  }
)

const variantClasses = computed(() => {
  switch (props.variant) {
    case 'primary':
      return 'bg-accent text-white hover:bg-accent-hover active:bg-accent-hover border border-transparent shadow-xs'
    case 'danger':
      return 'bg-status-danger text-white hover:opacity-90 active:opacity-80 border border-transparent shadow-xs'
    case 'ghost':
      return 'bg-transparent text-text-muted hover:text-text-main hover:bg-surface-hover border border-transparent'
    case 'secondary':
    default:
      return 'bg-surface-hover text-text-main hover:bg-surface-active border border-border-subtle'
  }
})

const sizeClasses = computed(() => {
  switch (props.size) {
    case 'sm':
      return 'px-2.5 py-1 text-xs min-h-[44px] min-w-[44px] sm:min-h-[40px] sm:min-w-[40px] gap-1.5 rounded-md'
    case 'lg':
      return 'px-4.5 py-2.5 text-sm sm:text-base min-h-[44px] min-w-[44px] sm:min-h-[44px] gap-2.5 rounded-lg'
    case 'md':
    default:
      return 'px-3.5 py-2 text-xs sm:text-sm min-h-[44px] min-w-[44px] sm:min-h-[40px] sm:min-w-[40px] gap-2 rounded-md'
  }
})
</script>
