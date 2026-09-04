<template>
  <Transition
    enter-active-class="transition-opacity transition-transform duration-200 ease-out"
    enter-from-class="opacity-0 translate-y-4 scale-95"
    enter-to-class="opacity-100 translate-y-0 scale-100"
    leave-active-class="transition-opacity transition-transform duration-150 ease-in"
    leave-from-class="opacity-100 translate-y-0 scale-100"
    leave-to-class="opacity-0 translate-y-4 scale-95"
  >
    <button
      v-if="visible"
      type="button"
      class="fixed bottom-6 right-6 z-50 flex items-center gap-2 rounded-lg border border-accent/40 bg-accent px-4 py-2.5 sm:px-5 sm:py-3 text-sm font-medium text-white shadow-md hover:bg-accent-hover active:bg-accent-active disabled:opacity-60 disabled:cursor-not-allowed transition-colors cursor-pointer"
      :disabled="saving"
      title="保存全部 (Ctrl+S)"
      @click="$emit('save')"
    >
      <Loader2 v-if="saving" class="h-4 w-4 animate-spin shrink-0" aria-hidden="true" />
      <Save v-else class="h-4 w-4 shrink-0" aria-hidden="true" />
      <span class="hidden sm:inline font-mono">{{ saving ? '保存中…' : '保存全部' }}</span>
    </button>
  </Transition>
</template>

<script setup lang="ts">
import { Save, Loader2 } from 'lucide-vue-next'

withDefaults(defineProps<{
  visible?: boolean
  saving?: boolean
}>(), {
  visible: false,
  saving: false,
})

defineEmits<{
  (e: 'save'): void
}>()
</script>
