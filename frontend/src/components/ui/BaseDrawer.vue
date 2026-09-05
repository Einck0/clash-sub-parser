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
          'fixed inset-0 z-50 flex bg-black/80 backdrop-blur-[4px] transition-opacity',
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
            'relative flex w-full flex-col bg-surface-base text-text-main shadow-md highlight-top transition-transform focus:outline-hidden',
            placement === 'left'
              ? 'h-full max-w-[320px] border-r border-border-subtle drawer-slide-left'
              : 'h-auto max-h-[88vh] sm:h-full sm:max-w-[480px] rounded-t-lg sm:rounded-none border-t sm:border-t-0 sm:border-l border-border-subtle drawer-slide-right pb-safe'
          ]"
        >
          <!-- Mobile Bottom Sheet Handle Pill -->
          <div
            v-if="placement === 'right'"
            ref="handleZoneRef"
            class="sm:hidden mx-auto my-2.5 h-3 w-16 flex items-center justify-center cursor-grab touch-none select-none shrink-0 rounded-full"
            aria-hidden="true"
            @pointerdown="onHandlePointerDown"
          >
            <div class="h-1 w-12 rounded-full bg-white/20 shrink-0" />
          </div>

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
import { onUnmounted, ref, toRef } from 'vue'
import { X } from 'lucide-vue-next'
import { useModalA11y } from '../../composables/useModalA11y'
import type { DrawerPlacement } from './types'

/**
 * BaseDrawer / AppDrawer component providing sliding drawer on desktop
 * (320px left navigation edge drawer, 480px right secondary panel) and
 * adaptive Bottom Sheet on mobile (<640px) for secondary panels.
 * Complies with full A11y lifecycle: role="dialog", aria-modal="true",
 * focus trapping, initial focus, Escape closing, and scroll locking.
 * Supports handle-only drag dismissal: >=96px or >=0.5px/ms with form exclusion.
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
const handleZoneRef = ref<HTMLElement | null>(null)
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

// Drag to dismiss gesture: handle-only + 96px/0.5 px/ms + form exclusion
let startY = 0
let currentY = 0
let startTime = 0
let isDragging = false

function onHandlePointerDown(e: PointerEvent) {
  if (e.button !== 0) return
  const target = e.target as HTMLElement | null
  // Form exclusion: pointer interaction on inputs/textareas/selects/buttons/links retains native behavior
  if (target && target.closest('input, textarea, select, button, a')) return

  startY = e.clientY
  currentY = e.clientY
  startTime = Date.now()
  isDragging = true

  try {
    handleZoneRef.value?.setPointerCapture(e.pointerId)
  } catch {
    // Pointer capture fallback
  }

  window.addEventListener('pointermove', onPointerMove)
  window.addEventListener('pointerup', onPointerUp)
  window.addEventListener('pointercancel', onPointerCancel)
}

function onPointerMove(e: PointerEvent) {
  if (!isDragging || !drawerRef.value) return
  currentY = e.clientY
  const deltaY = Math.max(0, currentY - startY)
  drawerRef.value.style.transform = `translateY(${deltaY}px)`
}

function onPointerUp(e: PointerEvent) {
  if (!isDragging) return
  isDragging = false
  window.removeEventListener('pointermove', onPointerMove)
  window.removeEventListener('pointerup', onPointerUp)
  window.removeEventListener('pointercancel', onPointerCancel)

  try {
    handleZoneRef.value?.releasePointerCapture(e.pointerId)
  } catch {
    // Ignore release capture error
  }

  const deltaY = Math.max(0, currentY - startY)
  const elapsed = Math.max(1, Date.now() - startTime)
  const velocity = deltaY / elapsed

  if (drawerRef.value) {
    drawerRef.value.style.transform = ''
  }

  // Handle-only dismissal threshold: >=96px or >=0.5 px/ms
  if (deltaY >= 96 || velocity >= 0.5) {
    handleClose()
  }
}

function onPointerCancel() {
  if (!isDragging) return
  isDragging = false
  window.removeEventListener('pointermove', onPointerMove)
  window.removeEventListener('pointerup', onPointerUp)
  window.removeEventListener('pointercancel', onPointerCancel)
  if (drawerRef.value) {
    drawerRef.value.style.transform = ''
  }
}

onUnmounted(() => {
  window.removeEventListener('pointermove', onPointerMove)
  window.removeEventListener('pointerup', onPointerUp)
  window.removeEventListener('pointercancel', onPointerCancel)
})
</script>
