<script setup lang="ts">
import { watch, onMounted, onUnmounted } from 'vue'
import { XMarkIcon } from '@heroicons/vue/24/outline'

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

function close() {
  emit('update:modelValue', false)
  emit('close')
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.modelValue) {
    close()
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

watch(
  () => props.modelValue,
  (val) => {
    if (typeof document !== 'undefined' && document.body) {
      if (val) {
        document.body.style.overflow = 'hidden'
      } else {
        document.body.style.overflow = ''
      }
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
        @click="close"
      />

      <!-- Drawer Content Container: Bottom Sheet on Mobile, Right Panel on Desktop -->
      <aside
        class="relative z-10 flex flex-col w-full bg-base-100/95 text-base-content backdrop-blur-2xl shadow-2xl border-t md:border-t-0 md:border-l border-white/10 rounded-t-3xl md:rounded-l-3xl md:rounded-tr-none md:fixed md:inset-y-0 md:right-0 adaptive-surface-sheet md:max-h-full overflow-hidden transition-all duration-300"
        :class="widthClass"
        role="dialog"
        aria-modal="true"
        aria-labelledby="drawer-title"
      >
        <!-- Mobile touch grab handle -->
        <div class="md:hidden flex justify-center py-2.5 cursor-grab shrink-0 select-none">
          <span class="w-12 h-1.5 rounded-full bg-base-content/25" />
        </div>

        <!-- Drawer Header -->
        <div class="flex items-start justify-between gap-4 px-5 pt-3 md:pt-6 pb-4 border-b border-base-300/40 shrink-0">
          <div class="min-w-0 flex-1">
            <h3 id="drawer-title" class="text-lg md:text-xl font-bold tracking-tight break-words text-base-content">
              {{ title }}
            </h3>
            <p v-if="description" class="text-xs md:text-sm text-base-content/60 mt-1 break-words">
              {{ description }}
            </p>
          </div>
          <button
            type="button"
            class="btn btn-ghost btn-circle btn-sm shrink-0 touch-manipulation hover:bg-base-300/50"
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
        <div v-if="$slots.footer" class="p-4 md:p-5 pb-[max(1rem,env(safe-area-inset-bottom,0px))] md:pb-5 border-t border-base-300/40 bg-base-200/50 shrink-0">
          <slot name="footer" />
        </div>
      </aside>
    </div>
  </Teleport>
</template>
