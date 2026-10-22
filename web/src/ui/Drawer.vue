<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { XMarkIcon } from '@heroicons/vue/24/outline'
import { useBodyScrollLock } from '../composables/useBodyScrollLock'

const props = withDefaults(
  defineProps<{
    modelValue: boolean
    title?: string
    description?: string
    widthClass?: string
  }>(),
  {
    title: '',
    description: '',
    widthClass: 'md:max-w-2xl',
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'close'): void
}>()

let previousActiveElement: HTMLElement | null = null
const closeBtnRef = ref<HTMLButtonElement | null>(null)

function restoreFocus() {
  if (previousActiveElement && typeof previousActiveElement.focus === 'function') {
    previousActiveElement.focus()
    previousActiveElement = null
  }
}

function close() {
  emit('update:modelValue', false)
  emit('close')
  restoreFocus()
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.modelValue) {
    if (typeof document !== 'undefined' && !document.querySelector('dialog[open]')) {
      e.stopPropagation()
      close()
    }
  }
}

onMounted(() => {
  if (typeof window !== 'undefined') {
    window.addEventListener('keydown', handleKeydown)
  }
})

onUnmounted(() => {
  if (typeof window !== 'undefined') {
    window.removeEventListener('keydown', handleKeydown)
  }
})

useBodyScrollLock(() => props.modelValue)

watch(
  () => props.modelValue,
  (val) => {
    if (val) {
      if (typeof document !== 'undefined' && document.activeElement instanceof HTMLElement) {
        previousActiveElement = document.activeElement
      }
    } else {
      restoreFocus()
    }
  }
)
</script>

<template>
  <Teleport to="body">
    <div v-if="modelValue" class="fixed inset-0 z-50 flex flex-col justify-end md:justify-center">
      <!-- 40% soft backdrop blur overlay -->
      <div
        class="fixed inset-0 bg-black/40 backdrop-blur-sm transition-opacity"
        aria-hidden="true"
        data-testid="drawer-backdrop"
        @click="close"
      />

      <!-- Drawer Content Container: Bottom Sheet on Mobile, Right Panel on Desktop -->
      <aside
        class="relative z-10 flex flex-col w-full bg-base-100/95 text-base-content backdrop-blur-2xl shadow-2xl border-t md:border-t-0 md:border-l border-white/10 rounded-t-3xl md:rounded-l-3xl md:rounded-tr-none md:fixed md:inset-y-0 md:right-0 adaptive-surface-sheet md:max-h-full overflow-hidden transition-all duration-300"
        :class="widthClass"
        role="dialog"
        aria-modal="true"
        aria-labelledby="drawer-title"
        :aria-describedby="description ? 'drawer-desc' : undefined"
      >
        <!-- Mobile touch grab handle -->
        <div class="md:hidden flex justify-center py-2.5 cursor-grab shrink-0 select-none">
          <span class="w-12 h-1.5 rounded-full bg-base-content/25" />
        </div>

        <!-- Drawer Header -->
        <div class="flex items-start justify-between gap-4 px-5 pt-3 md:pt-6 pb-4 border-b border-base-300/40 shrink min-h-0 overflow-y-auto">
          <div class="min-w-0 flex-1">
            <h3 id="drawer-title" class="text-lg md:text-xl font-bold tracking-tight break-words text-base-content">
              {{ title }}
            </h3>
            <p v-if="description" id="drawer-desc" class="text-xs md:text-sm text-base-content/60 mt-1 break-words">
              {{ description }}
            </p>
          </div>
          <button
            ref="closeBtnRef"
            type="button"
            class="btn btn-ghost btn-circle btn-sm shrink-0 sticky top-0 touch-manipulation hover:bg-base-300/50"
            aria-label="Close drawer"
            @click="close"
          >
            <XMarkIcon class="w-5 h-5" />
          </button>
        </div>

        <!-- Drawer Scrollable Body -->
        <div class="flex-1 min-h-0 overflow-y-auto p-5 md:p-6 space-y-6 overscroll-contain">
          <slot />
        </div>

        <!-- Drawer Optional Footer -->
        <div v-if="$slots.footer" class="p-4 md:p-5 pb-[max(1rem,env(safe-area-inset-bottom,0px))] md:pb-5 border-t border-base-300/40 bg-base-200/50 shrink-0 flex flex-wrap gap-2">
          <slot name="footer" />
        </div>
      </aside>
    </div>
  </Teleport>
</template>
