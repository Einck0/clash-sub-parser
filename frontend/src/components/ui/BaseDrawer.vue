<template>
  <teleport to="body">
    <transition name="drawer-fade">
      <div
        v-if="modelValue"
        :class="[
          'fixed inset-0 z-50 flex bg-black/70 backdrop-blur-xs transition-opacity',
          placement === 'left' ? 'justify-start items-stretch' : 'items-end sm:items-stretch justify-end'
        ]"
        @click.self="handleClose"
      >
        <div
          :class="[
            'relative flex w-full flex-col bg-[#0F172A] text-[#F8FAFC] shadow-2xl transition-transform',
            placement === 'left'
              ? 'h-full max-w-xs sm:max-w-sm border-r border-white/10 drawer-slide-left'
              : 'h-auto max-h-[88vh] sm:h-full sm:max-w-xl rounded-t-2xl sm:rounded-none border-t sm:border-t-0 sm:border-l border-white/10 drawer-slide-right pb-safe'
          ]"
        >
          <!-- Mobile Bottom Sheet Handle Pill -->
          <div
            v-if="placement === 'right'"
            class="sm:hidden mx-auto my-2.5 h-1 w-12 rounded-full bg-white/20"
            aria-hidden="true"
          />

          <!-- Header -->
          <div class="flex items-center justify-between border-b border-white/10 px-5 sm:px-6 py-3.5 sm:py-4">
            <h3 class="text-base font-semibold tracking-wide text-white">{{ title }}</h3>
            <button
              type="button"
              class="flex min-h-[44px] min-w-[44px] items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white cursor-pointer transition-colors"
              aria-label="关闭抽屉"
              @click="handleClose"
            >
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>

          <!-- Body -->
          <div class="flex-1 overflow-y-auto p-5 sm:p-6">
            <slot />
          </div>

          <!-- Footer -->
          <div v-if="$slots.footer" class="border-t border-white/10 bg-slate-900/50 px-5 sm:px-6 py-3.5 sm:py-4 pb-safe">
            <slot name="footer" />
          </div>
        </div>
      </div>
    </transition>
  </teleport>
</template>

<script setup lang="ts">
/**
 * BaseDrawer component providing sliding drawer on desktop and
 * adaptive Bottom Sheet on mobile for secondary panels.
 */
withDefaults(
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

function handleClose() {
  emit('update:modelValue', false)
  emit('close')
}
</script>

<style scoped>
.drawer-fade-enter-active,
.drawer-fade-leave-active {
  transition: opacity 0.2s ease;
}
.drawer-fade-enter-from,
.drawer-fade-leave-to {
  opacity: 0;
}
.drawer-slide-right {
  animation: slideRightIn 0.25s cubic-bezier(0.16, 1, 0.3, 1);
}
.drawer-slide-left {
  animation: slideLeftIn 0.25s cubic-bezier(0.16, 1, 0.3, 1);
}

@keyframes slideRightIn {
  from {
    transform: translateX(100%);
  }
  to {
    transform: translateX(0);
  }
}

@keyframes slideLeftIn {
  from {
    transform: translateX(-100%);
  }
  to {
    transform: translateX(0);
  }
}

@media (max-width: 639px) {
  .drawer-slide-right {
    animation: slideUpIn 0.25s cubic-bezier(0.16, 1, 0.3, 1);
  }

  @keyframes slideUpIn {
    from {
      transform: translateY(100%);
    }
    to {
      transform: translateY(0);
    }
  }
}
</style>
