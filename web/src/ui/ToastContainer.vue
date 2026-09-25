<script setup lang="ts">
import { computed } from 'vue'
import {
  CheckCircleIcon,
  ExclamationCircleIcon,
  ExclamationTriangleIcon,
  InformationCircleIcon,
  XMarkIcon,
} from '@heroicons/vue/24/outline'
import { toastStore, type ToastTone } from './toast'

const items = computed(() => toastStore.items.value)

const toneBorder: Record<ToastTone, string> = {
  success: 'border-success/40 text-success',
  error: 'border-error/40 text-error',
  warning: 'border-warning/40 text-warning',
  info: 'border-info/40 text-info',
}
</script>

<template>
  <div
    v-if="items.length > 0"
    class="fixed bottom-20 md:bottom-6 right-4 sm:right-6 z-[9999] flex flex-col gap-2.5 max-w-sm w-full pointer-events-none"
    aria-live="polite"
    aria-atomic="false"
  >
    <TransitionGroup
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="transform translate-y-2 opacity-0 scale-95"
      enter-to-class="transform translate-y-0 opacity-100 scale-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="transform opacity-100 scale-100"
      leave-to-class="transform opacity-0 scale-95"
    >
      <div
        v-for="toast in items"
        :key="toast.id"
        class="pointer-events-auto flex items-start gap-3 p-3.5 sm:p-4 rounded-2xl bg-base-300/90 text-base-content backdrop-blur-2xl shadow-2xl border transition-all"
        :class="toneBorder[toast.tone]"
        role="status"
      >
        <!-- Status accent icon -->
        <div class="shrink-0 mt-0.5">
          <CheckCircleIcon v-if="toast.tone === 'success'" class="w-5 h-5 text-success" />
          <ExclamationCircleIcon v-else-if="toast.tone === 'error'" class="w-5 h-5 text-error" />
          <ExclamationTriangleIcon v-else-if="toast.tone === 'warning'" class="w-5 h-5 text-warning" />
          <InformationCircleIcon v-else class="w-5 h-5 text-info" />
        </div>

        <!-- Content -->
        <div class="min-w-0 flex-1">
          <p v-if="toast.title" class="font-bold text-xs uppercase tracking-wider text-base-content">
            {{ toast.title }}
          </p>
          <p class="text-xs sm:text-sm text-base-content/90 font-medium break-words leading-relaxed">
            {{ toast.message }}
          </p>
        </div>

        <!-- Dismiss button -->
        <button
          type="button"
          class="btn btn-ghost btn-circle btn-xs text-base-content/50 hover:text-base-content shrink-0"
          aria-label="Dismiss notification"
          @click="toastStore.remove(toast.id)"
        >
          <XMarkIcon class="w-4 h-4" />
        </button>
      </div>
    </TransitionGroup>
  </div>
</template>
