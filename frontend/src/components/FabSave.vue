<template>
  <Transition name="fab">
    <button
      v-if="visible"
      class="fab-save"
      :class="{ saving: saving }"
      @click="$emit('save')"
      :disabled="saving"
      title="保存全部 (Ctrl+S)"
    >
      <span v-if="saving" class="fab-spinner"></span>
      <span v-else class="fab-icon">💾</span>
      <span class="fab-text">{{ saving ? '保存中...' : '保存全部' }}</span>
    </button>
  </Transition>
</template>

<script setup>
defineProps({
  visible: { type: Boolean, default: false },
  saving: { type: Boolean, default: false },
})
defineEmits(['save'])
</script>

<style scoped>
.fab-save {
  position: fixed;
  bottom: 24px;
  right: 24px;
  z-index: 1000;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 20px;
  border: none;
  border-radius: 28px;
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
  color: #fff;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  box-shadow: 0 4px 16px rgba(99, 102, 241, 0.4);
  transition: all 0.2s ease;
}
.fab-save:hover:not(:disabled) {
  transform: translateY(-2px);
  box-shadow: 0 6px 20px rgba(99, 102, 241, 0.5);
}
.fab-save:active:not(:disabled) {
  transform: translateY(0);
}
.fab-save.saving {
  opacity: 0.8;
  cursor: wait;
}
.fab-save:disabled {
  cursor: not-allowed;
}
.fab-icon {
  font-size: 18px;
  line-height: 1;
}
.fab-spinner {
  width: 16px;
  height: 16px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 0.6s linear infinite;
}
@keyframes spin {
  to { transform: rotate(360deg); }
}
.fab-enter-active,
.fab-leave-active {
  transition: all 0.3s ease;
}
.fab-enter-from,
.fab-leave-to {
  opacity: 0;
  transform: translateY(20px) scale(0.9);
}
@media (max-width: 640px) {
  .fab-save {
    bottom: 16px;
    right: 16px;
    padding: 10px 16px;
  }
  .fab-text {
    display: none;
  }
}
</style>
