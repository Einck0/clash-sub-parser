<script setup lang="ts">
import type { Component } from 'vue'

export interface EmptyStateProps {
  icon?: Component
  title: string
  description?: string
  actionLabel?: string
}

export interface EmptyStateEmits {
  (e: 'action'): void
}

withDefaults(defineProps<EmptyStateProps>(), {
  icon: undefined,
  description: '',
  actionLabel: '',
})

const emit = defineEmits<EmptyStateEmits>()
</script>

<template>
  <div
    class="csp-empty-state p-6 sm:p-10 text-center flex flex-col items-center justify-center transition-colors min-w-0"
    data-testid="empty-state"
  >
    <!-- Icon Slot / Prop -->
    <div
      v-if="icon || $slots.icon"
      class="w-12 h-12 rounded-2xl bg-base-300/40 flex items-center justify-center mb-3 text-primary/70"
    >
      <slot name="icon">
        <component :is="icon" class="w-6 h-6" />
      </slot>
    </div>

    <!-- Title -->
    <h3 class="font-semibold text-base sm:text-lg text-base-content tracking-tight">
      <slot name="title">{{ title }}</slot>
    </h3>

    <!-- Description -->
    <p
      v-if="description || $slots.description"
      class="mt-2 text-sm text-base-content/75 max-w-lg mx-auto leading-relaxed"
    >
      <slot name="description">{{ description }}</slot>
    </p>

    <!-- Action Button Slot / Prop -->
    <div v-if="actionLabel || $slots.action" class="mt-4">
      <slot name="action">
        <button
          type="button"
          class="btn btn-primary btn-sm gap-2 touch-manipulation shadow-sm"
          @click="emit('action')"
        >
          <slot name="action-icon" />
          <span>{{ actionLabel }}</span>
        </button>
      </slot>
    </div>

    <slot />
  </div>
</template>
