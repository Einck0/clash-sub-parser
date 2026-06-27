<template>
  <Teleport to="body">
    <div class="toast-container" aria-live="polite" aria-atomic="false">
      <TransitionGroup name="toast">
        <div
          v-for="t in toasts"
          :key="t.id"
          class="toast-item"
          :class="`toast-${t.type}`"
          role="status"
          @click="dismissToast(t.id)"
        >
          <span class="toast-icon">{{ icons[t.type] || 'i' }}</span>
          <span class="toast-text">{{ t.message }}</span>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<script setup>
import { useAppStore } from '../stores/app'
import { storeToRefs } from 'pinia'

const store = useAppStore()
const { toasts } = storeToRefs(store)
const { dismissToast } = store

const icons = {
  success: '✓',
  error: '✕',
  warning: '⚠',
  info: 'ℹ',
}
</script>

<style scoped>
.toast-container {
  position: fixed;
  top: 16px;
  right: 16px;
  z-index: 9999;
  display: flex;
  flex-direction: column;
  gap: 8px;
  pointer-events: none;
  max-width: 400px;
}

.toast-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 16px;
  border-radius: 12px;
  border: 1px solid;
  font-size: 14px;
  line-height: 1.4;
  cursor: pointer;
  pointer-events: auto;
  backdrop-filter: blur(12px);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.12);
}

.toast-icon {
  flex-shrink: 0;
  width: 22px;
  height: 22px;
  display: grid;
  place-items: center;
  border-radius: 50%;
  font-size: 12px;
  font-weight: 900;
}

.toast-success {
  border-color: rgba(15, 138, 95, 0.3);
  background: rgba(236, 253, 245, 0.95);
  color: #065f46;
}
.toast-success .toast-icon {
  background: rgba(15, 138, 95, 0.15);
  color: #0f8a5f;
}

.toast-error {
  border-color: rgba(220, 38, 38, 0.3);
  background: rgba(254, 242, 242, 0.95);
  color: #991b1b;
}
.toast-error .toast-icon {
  background: rgba(220, 38, 38, 0.15);
  color: #dc2626;
}

.toast-warning {
  border-color: rgba(180, 83, 9, 0.3);
  background: rgba(255, 251, 235, 0.95);
  color: #92400e;
}
.toast-warning .toast-icon {
  background: rgba(180, 83, 9, 0.15);
  color: #b45309;
}

.toast-info {
  border-color: rgba(37, 99, 235, 0.3);
  background: rgba(239, 246, 255, 0.95);
  color: #1e40af;
}
.toast-info .toast-icon {
  background: rgba(37, 99, 235, 0.15);
  color: #2563eb;
}

/* Transitions */
.toast-enter-active {
  transition: all 0.25s ease-out;
}
.toast-leave-active {
  transition: all 0.2s ease-in;
}
.toast-enter-from {
  opacity: 0;
  transform: translateX(40px) scale(0.95);
}
.toast-leave-to {
  opacity: 0;
  transform: translateX(40px) scale(0.95);
}
.toast-move {
  transition: transform 0.2s ease;
}
</style>
