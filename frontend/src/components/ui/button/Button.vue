<template>
  <Primitive
    :as="as"
    :as-child="asChild"
    :type="as === 'button' ? type : undefined"
    :disabled="disabled || loading"
    :class="cn(buttonVariants({ variant, size }), props.class)"
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
  </Primitive>
</template>

<script setup lang="ts">
import { type Component, type HTMLAttributes } from 'vue'
import { Primitive, type PrimitiveProps } from 'reka-ui'
import { Loader2 } from 'lucide-vue-next'
import { cn } from '@/lib/utils'
import { buttonVariants, type ButtonVariants } from './index'

interface Props extends PrimitiveProps {
  variant?: ButtonVariants['variant']
  size?: ButtonVariants['size']
  as?: string
  asChild?: boolean
  disabled?: boolean
  loading?: boolean
  type?: 'button' | 'submit' | 'reset'
  icon?: Component
  ariaLabel?: string
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  as: 'button',
  asChild: false,
  variant: 'secondary',
  size: 'md',
  disabled: false,
  loading: false,
  type: 'button',
  icon: undefined,
  ariaLabel: undefined,
  class: undefined,
})
</script>
