<template>
  <Transition name="snackbar">
    <div v-if="visible" class="snackbar" :class="type">
      <span class="snackbar-text">{{ message }}</span>
      <button v-if="undoable" class="snackbar-undo" @click="undo">撤销</button>
      <button class="snackbar-close" @click="close">×</button>
    </div>
  </Transition>
</template>

<script setup>
import { ref, onUnmounted } from 'vue'

const props = defineProps({
  message: { type: String, default: '' },
  type: { type: String, default: 'info' },
  duration: { type: Number, default: 5000 },
  undoable: { type: Boolean, default: false },
})

const emit = defineEmits(['undo', 'close'])

const visible = ref(false)
let timer = null

function show() {
  visible.value = true
  if (props.duration > 0) {
    timer = setTimeout(close, props.duration)
  }
}

function close() {
  visible.value = false
  clearTimeout(timer)
  emit('close')
}

function undo() {
  visible.value = false
  clearTimeout(timer)
  emit('undo')
}

onUnmounted(() => clearTimeout(timer))

defineExpose({ show, close })
</script>

<style scoped>
.snackbar {
  position: fixed;
  bottom: 80px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 2000;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 16px;
  border-radius: 8px;
  background: var(--bg-0, #1e293b);
  color: var(--text-0, #f8fafc);
  border: 1px solid var(--border, #334155);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.25);
  font-size: 13px;
  max-width: 500px;
  white-space: nowrap;
}
.snackbar.info { border-left: 3px solid #6366f1; }
.snackbar.success { border-left: 3px solid #22c55e; }
.snackbar.warning { border-left: 3px solid #f59e0b; }
.snackbar.error { border-left: 3px solid #ef4444; }
.snackbar-text { flex: 1; }
.snackbar-undo {
  padding: 4px 10px;
  border: 1px solid #6366f1;
  border-radius: 4px;
  background: transparent;
  color: #6366f1;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}
.snackbar-undo:hover { background: rgba(99, 102, 241, 0.1); }
.snackbar-close {
  padding: 2px 6px;
  border: none;
  background: transparent;
  color: var(--muted, #94a3b8);
  font-size: 16px;
  cursor: pointer;
}
.snackbar-enter-active,
.snackbar-leave-active {
  transition: all 0.3s ease;
}
.snackbar-enter-from {
  opacity: 0;
  transform: translateX(-50%) translateY(20px);
}
.snackbar-leave-to {
  opacity: 0;
  transform: translateX(-50%) translateY(-10px);
}
@media (max-width: 640px) {
  .snackbar {
    left: 16px;
    right: 16px;
    transform: none;
    max-width: none;
  }
  .snackbar-enter-from {
    transform: translateY(20px);
  }
  .snackbar-leave-to {
    transform: translateY(-10px);
  }
}
</style>
