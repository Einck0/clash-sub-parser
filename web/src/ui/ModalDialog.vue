<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, useId, watch } from 'vue'
import { uiText } from '../locales'

const props = withDefaults(
  defineProps<{
    modelValue: boolean
    title?: string
    description?: string
    closeOnBackdrop?: boolean
  }>(),
  { closeOnBackdrop: true }
)

const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
const dialog = ref<HTMLDialogElement>()
const id = useId()
const titleId = `modal-dialog-title-${id}`
const descriptionId = `modal-dialog-desc-${id}`
let previousActiveElement: HTMLElement | null = null

function restoreFocus() {
  if (previousActiveElement?.isConnected) {
    previousActiveElement.focus()
    previousActiveElement = null
  }
}

function close() {
  if (props.modelValue) emit('update:modelValue', false)
}

function onNativeClose() {
  // A parent-controlled close already updated modelValue; do not request it twice.
  if (!dialog.value?.open) close()
}

function onBackdrop(event: MouseEvent) {
  if (props.closeOnBackdrop && event.target === event.currentTarget) {
    close()
  }
}

function onCancel(event: Event) {
  event.preventDefault()
  close()
}

async function syncDialog(open: boolean) {
  await nextTick()
  if (!dialog.value || open !== props.modelValue) return
  if (open) {
    if (typeof document !== 'undefined' && document.activeElement instanceof HTMLElement) {
      previousActiveElement = document.activeElement
    }
    if (!dialog.value.open) dialog.value.showModal()
  } else {
    if (dialog.value.open) dialog.value.close()
    restoreFocus()
  }
}

watch(() => props.modelValue, syncDialog)
onMounted(() => syncDialog(props.modelValue))
onUnmounted(restoreFocus)
</script>

<template>
  <dialog
    ref="dialog"
    class="modal"
    aria-modal="true"
    :aria-labelledby="title ? titleId : undefined"
    :aria-describedby="description ? descriptionId : undefined"
    @cancel="onCancel"
    @close="onNativeClose"
    @click="onBackdrop"
  >
    <div class="csp-overlay modal-box relative w-[calc(100%-2rem)] max-w-lg adaptive-surface-dialog flex flex-col p-0 overflow-hidden">
      <button
        type="button"
        class="btn btn-ghost btn-sm btn-circle absolute right-4 top-4 z-10"
        data-testid="modal-dialog-close"
        :aria-label="uiText('关闭对话框', 'Close dialog')"
        @click="close"
      >
        ✕
      </button>
      <div class="flex-1 min-h-0 overflow-y-auto overscroll-contain">
        <div class="flex items-start justify-between gap-4 p-5 md:p-6 pr-16 pb-3 border-b border-base-300/40">
          <div class="min-w-0 flex-1">
            <h2 :id="titleId" class="text-lg font-semibold break-words">{{ title }}</h2>
            <p v-if="description" :id="descriptionId" class="mt-1 text-sm opacity-70 break-words">{{ description }}</p>
          </div>
        </div>
        <div class="p-5 md:p-6 min-w-0"><slot /></div>
      </div>
      <div v-if="$slots.footer" class="modal-action flex flex-wrap shrink-0 m-0 p-4 md:p-5 border-t border-base-300/40 bg-base-200/50">
        <slot name="footer" />
      </div>
    </div>
  </dialog>
</template>
