<script setup lang="ts">
import { computed } from 'vue'
import {
  ExclamationTriangleIcon,
  ExclamationCircleIcon,
  InformationCircleIcon,
} from '@heroicons/vue/24/outline'

interface Props {
  modelValue?: boolean
  title?: string
  message?: string
  description?: string
  confirmText?: string
  cancelText?: string
  tone?: 'danger' | 'warning' | 'primary'
  loading?: boolean
  closable?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  modelValue: false,
  title: 'Confirm Operation',
  message: '',
  description: '',
  confirmText: 'Confirm',
  cancelText: 'Cancel',
  tone: 'danger',
  loading: false,
  closable: true,
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  confirm: []
  cancel: []
}>()

const displayMessage = computed(() => props.message || props.description || '')

function handleConfirm() {
  if (props.loading) return
  emit('confirm')
}

function handleCancel() {
  if (props.loading || !props.closable) return
  emit('update:modelValue', false)
  emit('cancel')
}

function handleBackdrop(event: MouseEvent) {
  if (props.closable && !props.loading && event.target === event.currentTarget) {
    handleCancel()
  }
}
</script>

<template>
  <div
    class="modal modal-middle z-50 bg-base-900/60 backdrop-blur-sm transition-opacity duration-200"
    :class="{ 'modal-open': modelValue }"
    role="dialog"
    aria-modal="true"
    aria-labelledby="confirm-modal-title"
    aria-describedby="confirm-modal-description"
    tabindex="-1"
    @click="handleBackdrop"
    @keydown.esc="handleCancel"
  >
    <div
      class="modal-box relative max-w-md w-[calc(100%-2rem)] adaptive-surface-dialog flex flex-col bg-base-100 text-base-content shadow-2xl border border-base-300 p-5 md:p-6 overflow-hidden"
    >
      <!-- Close button -->
      <button
        v-if="closable"
        type="button"
        class="btn btn-ghost btn-sm btn-circle absolute right-4 top-4 text-base-content/60 hover:text-base-content"
        data-testid="confirm-modal-close"
        aria-label="Close dialog"
        :disabled="loading"
        @click="handleCancel"
      >
        ✕
      </button>

      <!-- Tone icon + Title & description -->
      <div class="flex items-start gap-3 shrink-0">
        <div
          class="p-2.5 rounded-xl border shrink-0"
          :class="{
            'bg-error/10 text-error border-error/20': tone === 'danger',
            'bg-warning/10 text-warning border-warning/20': tone === 'warning',
            'bg-primary/10 text-primary border-primary/20': tone === 'primary',
          }"
          data-testid="confirm-modal-tone-icon"
        >
          <ExclamationTriangleIcon v-if="tone === 'danger'" class="w-6 h-6" />
          <ExclamationCircleIcon v-else-if="tone === 'warning'" class="w-6 h-6" />
          <InformationCircleIcon v-else class="w-6 h-6" />
        </div>

        <div class="pr-6 min-w-0 flex-1">
          <h2
            id="confirm-modal-title"
            class="text-base sm:text-lg font-bold tracking-tight text-base-content break-words"
            data-testid="confirm-modal-title"
          >
            {{ title }}
          </h2>
          <p
            v-if="displayMessage"
            id="confirm-modal-description"
            class="text-xs sm:text-sm text-base-content/70 mt-1 leading-relaxed break-words"
            data-testid="confirm-modal-message"
          >
            {{ displayMessage }}
          </p>
        </div>
      </div>

      <!-- Optional slot content -->
      <div v-if="$slots.default" class="py-3 flex-1 min-h-0 overflow-y-auto overscroll-contain">
        <slot />
      </div>

      <!-- Action buttons -->
      <div class="modal-action mt-6 flex justify-end items-center gap-2 shrink-0">
        <button
          v-if="closable"
          type="button"
          class="btn btn-ghost btn-sm text-xs font-normal"
          data-testid="confirm-modal-cancel"
          :disabled="loading"
          @click="handleCancel"
        >
          {{ cancelText }}
        </button>
        <button
          type="button"
          class="btn btn-sm px-4 text-xs font-semibold shadow-sm"
          :class="{
            'btn-error text-white': tone === 'danger',
            'btn-warning text-warning-content': tone === 'warning',
            'btn-primary text-primary-content': tone === 'primary',
            loading: loading,
          }"
          data-testid="confirm-modal-confirm"
          :disabled="loading"
          @click="handleConfirm"
        >
          {{ confirmText }}
        </button>
      </div>
    </div>
  </div>
</template>
