<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition-opacity duration-200 ease-out"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition-opacity duration-150 ease-in"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div
        v-if="confirmState.open"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 transition-opacity"
        @click.self="cancel"
      >
        <div
          ref="dialogRef"
          role="alertdialog"
          aria-modal="true"
          :aria-labelledby="titleId"
          :aria-describedby="descId"
          tabindex="-1"
          class="relative w-full max-w-md rounded-lg border border-border-strong bg-surface-base p-5 sm:p-6 shadow-md focus:outline-hidden"
        >
          <!-- Header with Warning/Danger Icon -->
          <div class="flex items-start gap-3">
            <div
              :class="[
                'flex h-10 w-10 shrink-0 items-center justify-center rounded-md border',
                confirmState.danger
                  ? 'bg-status-danger/10 border-status-danger/20 text-status-danger'
                  : 'bg-status-warning/10 border-status-warning/20 text-status-warning'
              ]"
            >
              <AlertTriangle class="h-5 w-5" aria-hidden="true" />
            </div>
            <div class="flex-1 min-w-0">
              <h3 :id="titleId" class="text-base font-semibold tracking-wide text-text-main">
                {{ confirmState.title || '确认操作' }}
              </h3>
              <p :id="descId" class="mt-2 text-sm text-text-muted leading-relaxed whitespace-pre-line">
                {{ confirmState.message }}
              </p>
            </div>
          </div>

          <!-- Footer Actions -->
          <div class="mt-6 flex items-center justify-end gap-3">
            <button
              type="button"
              class="inline-flex min-h-[44px] sm:min-h-[36px] items-center justify-center rounded-md border border-border-subtle bg-surface-hover px-4 py-2 text-xs sm:text-sm font-medium text-text-main hover:bg-surface-active hover:text-white transition-colors cursor-pointer active:scale-[0.98] focus:outline-hidden focus-visible:ring-2 focus-visible:ring-accent"
              @click="cancel"
            >
              {{ confirmState.cancelText || '取消' }}
            </button>
            <button
              ref="confirmBtnRef"
              type="button"
              :class="[
                'inline-flex min-h-[44px] sm:min-h-[36px] items-center justify-center rounded-md px-4 py-2 text-xs sm:text-sm font-medium text-white transition-colors cursor-pointer active:scale-[0.98] focus:outline-hidden focus-visible:ring-2 focus-visible:ring-accent shadow-xs',
                confirmState.danger
                  ? 'bg-status-danger hover:bg-red-600 active:bg-red-700'
                  : 'bg-accent hover:bg-accent-hover active:bg-accent-hover'
              ]"
              @click="ok"
            >
              {{ confirmState.confirmText || '确定' }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { storeToRefs } from 'pinia'
import { AlertTriangle } from 'lucide-vue-next'
import { useAppStore } from '../stores/app'
import { useModalA11y } from '../composables/useModalA11y'

const store = useAppStore()
const { confirmState } = storeToRefs(store)
const { resolveConfirm } = store

const dialogRef = ref<HTMLElement | null>(null)
const confirmBtnRef = ref<HTMLElement | null>(null)
const titleId = `confirm-title-${Math.random().toString(36).slice(2, 9)}`
const descId = `confirm-desc-${Math.random().toString(36).slice(2, 9)}`

const isOpen = computed(() => Boolean(confirmState.value.open))

useModalA11y({
  isOpen,
  containerRef: dialogRef,
  initialFocusRef: confirmBtnRef,
  onClose: cancel,
  lockScroll: true,
  trapFocus: true,
  closeOnEscape: true,
})

function ok() {
  resolveConfirm(true)
}

function cancel() {
  resolveConfirm(false)
}
</script>
