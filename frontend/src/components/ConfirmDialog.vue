<template>
  <Teleport to="body">
    <Transition name="confirm-fade">
      <div v-if="confirmState.open" class="confirm-backdrop" @click.self="cancel">
        <div class="confirm-dialog" role="alertdialog" aria-modal="true" :aria-label="confirmState.title">
          <div class="confirm-header">
            <h3>{{ confirmState.title }}</h3>
          </div>
          <div class="confirm-body">
            <p>{{ confirmState.message }}</p>
          </div>
          <div class="confirm-footer">
            <button @click="cancel">{{ confirmState.cancelText }}</button>
            <button
              :class="{ danger: confirmState.danger, primary: !confirmState.danger }"
              @click="ok"
              ref="confirmBtn"
            >
              {{ confirmState.confirmText }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup>
import { ref, watch, nextTick } from 'vue'
import { useAppStore } from '../stores/app'
import { storeToRefs } from 'pinia'

const store = useAppStore()
const { confirmState } = storeToRefs(store)
const { resolveConfirm } = store
const confirmBtn = ref(null)

watch(() => confirmState.value.open, (open) => {
  if (open) nextTick(() => confirmBtn.value?.focus())
})

function ok() { resolveConfirm(true) }
function cancel() { resolveConfirm(false) }
</script>

<style scoped>
.confirm-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.35);
  backdrop-filter: blur(4px);
  display: grid;
  place-items: center;
  z-index: 10000;
}

.confirm-dialog {
  width: min(440px, 92vw);
  border: 1px solid rgba(219, 229, 239, 0.9);
  border-radius: 20px;
  background: var(--surface, linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(248, 250, 252, 0.95)));
  box-shadow: 0 24px 60px rgba(15, 23, 42, 0.18);
  padding: 24px;
}

.confirm-header h3 {
  margin: 0 0 8px;
  font-size: 18px;
  letter-spacing: -0.02em;
}

.confirm-body p {
  margin: 0;
  color: #64748b;
  font-size: 14px;
  line-height: 1.6;
  white-space: pre-line;
}

.confirm-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 20px;
}

.confirm-footer button {
  min-width: 80px;
  min-height: 38px;
}

/* Transition */
.confirm-fade-enter-active {
  transition: opacity 0.2s ease;
}
.confirm-fade-leave-active {
  transition: opacity 0.15s ease;
}
.confirm-fade-enter-from,
.confirm-fade-leave-to {
  opacity: 0;
}
.confirm-fade-enter-active .confirm-dialog {
  transition: transform 0.25s ease-out;
}
.confirm-fade-enter-from .confirm-dialog {
  transform: scale(0.95) translateY(8px);
}
</style>
