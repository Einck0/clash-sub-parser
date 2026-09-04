<template>
  <Teleport to="body">
    <div
      class="fixed top-4 right-4 z-50 flex flex-col gap-2 max-w-sm w-full pointer-events-none px-3 sm:px-0"
      aria-live="polite"
      aria-atomic="false"
    >
      <TransitionGroup
        enter-active-class="transition-opacity duration-200 ease-out"
        enter-from-class="opacity-0 translate-y-2"
        enter-to-class="opacity-100 translate-y-0"
        leave-active-class="transition-opacity duration-150 ease-in"
        leave-from-class="opacity-100"
        leave-to-class="opacity-0 translate-y-1"
      >
        <div
          v-for="t in toasts"
          :key="t.id"
          role="status"
          tabindex="0"
          :class="[
            'pointer-events-auto flex items-start justify-between gap-3 rounded-lg border bg-surface-base p-3.5 shadow-md transition-colors cursor-pointer select-none',
            'focus:outline-hidden focus-visible:ring-2 focus-visible:ring-accent',
            getToastClass(t.type),
          ]"
          @click="dismissToast(t.id)"
          @keydown.enter="dismissToast(t.id)"
          @keydown.space.prevent="dismissToast(t.id)"
        >
          <!-- Left Status Icon -->
          <div class="flex items-center gap-2.5 min-w-0 flex-1">
            <component :is="getIcon(t.type)" class="h-4 w-4 shrink-0 mt-0.5" aria-hidden="true" />
            <span class="text-xs sm:text-sm text-text-main leading-snug break-words">
              {{ t.message }}
            </span>
          </div>

          <!-- Dismiss Button -->
          <button
            type="button"
            class="flex h-5 w-5 shrink-0 items-center justify-center rounded text-text-muted hover:text-text-main transition-colors cursor-pointer"
            aria-label="关闭通知"
            @click.stop="dismissToast(t.id)"
          >
            <X class="h-3.5 w-3.5" aria-hidden="true" />
          </button>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { CheckCircle2, AlertCircle, AlertTriangle, Info, X } from 'lucide-vue-next'
import { useAppStore } from '../stores/app'

const store = useAppStore()
const { toasts } = storeToRefs(store)
const { dismissToast } = store

function getIcon(type?: string) {
  switch (type) {
    case 'success':
      return CheckCircle2
    case 'error':
      return AlertCircle
    case 'warning':
      return AlertTriangle
    case 'info':
    default:
      return Info
  }
}

function getToastClass(type?: string) {
  switch (type) {
    case 'success':
      return 'border-status-success/30 text-status-success'
    case 'error':
      return 'border-status-danger/30 text-status-danger'
    case 'warning':
      return 'border-status-warning/30 text-status-warning'
    case 'info':
    default:
      return 'border-status-info/30 text-status-info'
  }
}
</script>
