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
        :class="[
          'fixed inset-0 z-50 flex bg-black/70 transition-opacity',
          placement === 'left' ? 'justify-start items-stretch' : 'items-end sm:items-stretch justify-end'
        ]"
        @click.self="handleClose"
      >
        <div
          ref="drawerRef"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="titleId"
          tabindex="-1"
          :class="[
            'relative flex w-full flex-col bg-surface-base text-text-main shadow-md transition-transform focus:outline-hidden',
            placement === 'left'
              ? 'h-full max-w-xs sm:max-w-sm border-r border-border-subtle drawer-slide-left'
              : 'h-auto max-h-[88vh] sm:h-full sm:max-w-xl rounded-t-lg sm:rounded-none border-t sm:border-t-0 sm:border-l border-border-subtle drawer-slide-right pb-safe'
          ]"
        >
          <!-- Mobile Bottom Sheet Handle Pill -->
          <div
            v-if="placement === 'right'"
            class="sm:hidden mx-auto my-2.5 h-1 w-12 rounded-full bg-white/20 shrink-0"
            aria-hidden="true"
          />

          <!-- Header -->
          <div class="flex items-center justify-between border-b border-border-subtle px-5 sm:px-6 py-3.5 sm:py-4 shrink-0">
            <h3 :id="titleId" class="text-base font-semibold tracking-wide text-text-main">
              {{ title }}
            </h3>
            <button
              ref="closeButtonRef"
              type="button"
              class="flex min-h-[44px] min-w-[44px] items-center justify-center rounded-lg text-text-muted hover:bg-surface-hover hover:text-text-main cursor-pointer transition-colors focus:outline-hidden focus-visible:ring-2 focus-visible:ring-accent"
              aria-label="关闭抽屉"
              @click="handleClose"
            >
              <X class="h-5 w-5" aria-hidden="true" />
            </button>
          </div>

          <!-- Body -->
          <div class="flex-1 overflow-y-auto p-5 sm:p-6">
            <slot />
          </div>

          <!-- Footer -->
          <div v-if="$slots.footer" class="border-t border-border-subtle bg-surface px-5 sm:px-6 py-3.5 sm:py-4 pb-safe shrink-0">
            <slot name="footer" />
          </div>
        </div>
      </div>
    </transition>
  </teleport>
</template>

<script setup lang="ts">
import { ref, toRef } from 'vue'
import { X } from 'lucide-vue-next'
import { useModalA11y } from '../../composables/useModalA11y'

/**
 * BaseDrawer component providing sliding drawer on desktop and
 * adaptive Bottom Sheet on mobile for secondary panels.
 * Complies with full A11y lifecycle: role="dialog", aria-modal="true",
 * focus trapping, initial focus, Escape closing, and scroll locking.
 */
const props = withDefaults(
  defineProps<{
    modelValue: boolean
    title: string
    placement?: 'left' | 'right'
  }>(),
  {
    placement: 'right'
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'close'): void
}>()

const drawerRef = ref<HTMLElement | null>(null)
const closeButtonRef = ref<HTMLElement | null>(null)
const titleId = `drawer-title-${Math.random().toString(36).slice(2, 9)}`

useModalA11y({
  isOpen: toRef(props, 'modelValue'),
  containerRef: drawerRef,
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
</script>
