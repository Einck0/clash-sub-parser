<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from 'vue'

const props = withDefaults(defineProps<{
  modelValue: boolean
  title?: string
  description?: string
  closeOnBackdrop?: boolean
}>(), { closeOnBackdrop: true })
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
const dialog = ref<HTMLDialogElement>()

function close() {
  emit('update:modelValue', false)
}
function onBackdrop(event: MouseEvent) {
  if (props.closeOnBackdrop && event.target === event.currentTarget) close()
}
async function syncDialog(open: boolean) {
  await nextTick()
  if (!dialog.value) return
  if (open && !dialog.value.open) dialog.value.showModal()
  if (!open && dialog.value.open) dialog.value.close()
}
watch(() => props.modelValue, syncDialog)
onMounted(() => syncDialog(props.modelValue))
</script>

<template>
  <dialog
    ref="dialog"
    class="modal"
    aria-modal="true"
    @close="close"
    @click="onBackdrop"
  >
    <div class="modal-box relative w-[calc(100%-2rem)] max-w-lg adaptive-surface-dialog flex flex-col p-0 overflow-hidden">
      <button type="button" class="btn btn-ghost btn-sm btn-circle absolute right-4 top-4 z-10" aria-label="Close dialog" @click="close">×</button>
      <div class="flex-1 min-h-0 overflow-y-auto overscroll-contain">
      <div class="flex items-start justify-between gap-4 p-5 md:p-6 pr-16 pb-3 border-b border-base-300/40">
        <div class="min-w-0 flex-1">
          <h2 class="text-lg font-semibold break-words">{{ title }}</h2>
          <p v-if="description" class="mt-1 text-sm opacity-70 break-words">{{ description }}</p>
        </div>
      </div>
      <div class="p-5 md:p-6 min-w-0"><slot /></div>
      </div>
      <div v-if="$slots.footer" class="modal-action shrink-0 m-0 p-4 md:p-5 border-t border-base-300/40 bg-base-200/50"><slot name="footer" /></div>
    </div>
  </dialog>
</template>
