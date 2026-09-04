<template>
  <teleport to="body">
    <transition name="drawer-fade">
      <div
        v-if="modelValue"
        class="fixed inset-0 z-50 flex justify-end bg-black/60 backdrop-blur-xs transition-opacity"
        @click.self="handleClose"
      >
        <div
          class="relative flex h-full w-full max-w-xl flex-col border-l border-white/10 bg-[#0F172A] text-[#F8FAFC] shadow-2xl transition-transform"
        >
          <div class="flex items-center justify-between border-b border-white/10 px-6 py-4">
            <h3 class="text-base font-medium tracking-wide text-white">{{ title }}</h3>
            <button
              class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white"
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
defineProps<{
  modelValue: boolean
  title: string
}>()

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
</style>
