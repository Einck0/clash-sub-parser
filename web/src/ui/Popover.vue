<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { onClickOutside } from '@vueuse/core'

const props = withDefaults(
  defineProps<{
    placement?: 'bottom-end' | 'bottom-start' | 'top-end' | 'top-start'
    panelClass?: string
  }>(),
  {
    placement: 'bottom-end',
    panelClass: 'w-48',
  }
)

const isOpen = ref(false)
const triggerRef = ref<HTMLElement | null>(null)
const panelRef = ref<HTMLElement | null>(null)

function toggle() {
  isOpen.value = !isOpen.value
}

function open() {
  isOpen.value = true
}

function close() {
  isOpen.value = false
}

onClickOutside(panelRef, (event) => {
  if (triggerRef.value && triggerRef.value.contains(event.target as Node)) {
    return
  }
  close()
})

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && isOpen.value) {
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

defineExpose({
  isOpen,
  open,
  close,
  toggle,
})
</script>

<template>
  <div class="relative inline-block text-left">
    <!-- Trigger slot -->
    <div ref="triggerRef" class="inline-flex" @click="toggle">
      <slot name="trigger" :open="isOpen" />
    </div>

    <!-- Dropdown Panel with dark glass aesthetics -->
    <Transition
      enter-active-class="transition duration-150 ease-out"
      enter-from-class="transform scale-95 opacity-0"
      enter-to-class="transform scale-100 opacity-100"
      leave-active-class="transition duration-100 ease-in"
      leave-from-class="transform scale-100 opacity-100"
      leave-to-class="transform scale-95 opacity-0"
    >
      <div
        v-if="isOpen"
        ref="panelRef"
        class="absolute z-40 mt-1.5 rounded-2xl bg-base-200/95 backdrop-blur-2xl border border-white/10 shadow-2xl p-1.5 outline-none"
        :class="[
          placement === 'bottom-end' ? 'right-0 origin-top-right' :
          placement === 'bottom-start' ? 'left-0 origin-top-left' :
          placement === 'top-end' ? 'right-0 bottom-full mb-1.5 origin-bottom-right' :
          'left-0 bottom-full mb-1.5 origin-bottom-left',
          panelClass,
        ]"
        role="menu"
        aria-orientation="vertical"
        tabindex="-1"
      >
        <slot :close="close" />
      </div>
    </Transition>
  </div>
</template>
