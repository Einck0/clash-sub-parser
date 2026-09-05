<template>
  <teleport to="body">
    <transition
      enter-active-class="transition-opacity duration-200 ease-out"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition-opacity duration-150 ease-in"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div
        v-if="modelValue"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 transition-opacity"
        @click.self="handleClose"
      >
        <div
          ref="modalRef"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="titleId"
          tabindex="-1"
          :class="[
            'relative flex w-[calc(100%-32px)] max-h-[calc(100dvh-64px)] flex-col rounded-lg border border-border-subtle bg-surface-base text-text-main shadow-md focus:outline-hidden',
            sizeClass
          ]"
        >
          <!-- Header -->
          <div class="flex items-center justify-between border-b border-border-subtle px-5 sm:px-6 py-3.5 sm:py-4 shrink-0">
            <h3 :id="titleId" class="text-base font-semibold tracking-wide text-text-main">
              {{ title }}
            </h3>
            <button
              ref="closeButtonRef"
              type="button"
              class="flex min-h-[44px] min-w-[44px] items-center justify-center rounded-lg text-text-muted hover:bg-surface-hover hover:text-text-main cursor-pointer transition-colors focus:outline-hidden focus-visible:ring-2 focus-visible:ring-accent"
              aria-label="关闭对话框"
              @click="handleClose"
            >
              <X class="h-5 w-5" aria-hidden="true" />
            </button>
          </div>

          <!-- Body with independent scroll -->
          <div class="flex-1 overflow-y-auto p-5 sm:p-6">
            <slot />
          </div>

          <!-- Optional Fixed Footer -->
          <div v-if="$slots.footer" class="border-t border-border-subtle bg-surface px-5 sm:px-6 py-3.5 sm:py-4 shrink-0">
            <slot name="footer" />
          </div>
        </div>
      </div>
    </transition>
  </teleport>
</template>

<script setup lang="ts">
import { computed, ref, toRef } from 'vue'
import { X } from 'lucide-vue-next'
import { useModalA11y } from '../../composables/useModalA11y'

export type ModalSize = 'sm' | 'md' | 'lg'

const props = withDefaults(
  defineProps<{
    modelValue: boolean
    title: string
    size?: 'sm' | 'md' | 'lg'
  }>(),
  {
    size: 'md'
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'close'): void
}>()

const modalRef = ref<HTMLElement | null>(null)
const closeButtonRef = ref<HTMLElement | null>(null)
const titleId = `app-modal-title-${Math.random().toString(36).slice(2, 9)}`

useModalA11y({
  isOpen: toRef(props, 'modelValue'),
  containerRef: modalRef,
  initialFocusRef: closeButtonRef,
  onClose: handleClose,
  lockScroll: true,
  trapFocus: true,
  closeOnEscape: true,
})

function handleClose() {
  emit('update:modelValue', false)
  emit('close')
}

const sizeClass = computed(() => {
  switch (props.size) {
    case 'sm':
      return 'max-w-[448px]'
    case 'lg':
      return 'max-w-[960px]'
    case 'md':
    default:
      return 'max-w-[640px]'
  }
})
</script>
