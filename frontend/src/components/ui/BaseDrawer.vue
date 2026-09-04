<template>
  <teleport to="body">
    <transition name="drawer-fade">
      <div
        v-if="modelValue"
        :class="[
          'fixed inset-0 z-50 flex bg-black/60 backdrop-blur-xs transition-opacity',
          placement === 'left' ? 'justify-start' : 'justify-end'
        ]"
        @click.self="handleClose"
      >
        <div
          :class="[
            'relative flex h-full w-full flex-col bg-[#0F172A] text-[#F8FAFC] shadow-2xl transition-transform',
            placement === 'left'
              ? 'max-w-xs sm:max-w-sm border-r border-white/10 drawer-slide-left'
              : 'max-w-xl border-l border-white/10 drawer-slide-right'
          ]"
        >
          <div class="flex items-center justify-between border-b border-white/10 px-6 py-4">
            <h3 class="text-base font-medium tracking-wide text-white">{{ title }}</h3>
            <button
              type="button"
              class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white cursor-pointer transition-colors"
              aria-label="关闭抽屉"
              @click="handleClose"
            >
              ✕
            </button>
          </div>
          <div class="flex-1 overflow-y-auto p-6">
            <slot />
          </div>
          <div v-if="$slots.footer" class="border-t border-white/10 bg-slate-900/50 px-6 py-4">
            <slot name="footer" />
          </div>
        </div>
      </div>
    </transition>
  </teleport>
</template>

<script setup lang="ts">
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
  animation: slideRightIn 0.2s ease-out;
}
.drawer-slide-left {
  animation: slideLeftIn 0.2s ease-out;
}
@keyframes slideRightIn {
  from { transform: translateX(100%); }
  to { transform: translateX(0); }
}
@keyframes slideLeftIn {
  from { transform: translateX(-100%); }
  to { transform: translateX(0); }
}
</style>
