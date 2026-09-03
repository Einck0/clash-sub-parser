<template>
  <Teleport to="body">
    <Transition name="modal-fade">
      <div v-if="open" class="modal-backdrop" @click.self="close">
        <div class="modal export-modal" role="dialog" aria-modal="true" aria-labelledby="export-modal-title">
          <div class="modal-head">
            <div>
              <p class="eyebrow">Quick Export</p>
              <h3 id="export-modal-title">快速订阅与导出</h3>
            </div>
            <button class="close-btn" @click="close" aria-label="关闭">✕</button>
          </div>

          <div class="modal-body">
            <div class="export-content">
              <div class="url-card">
                <span class="url-label">完整订阅链接</span>
                <div class="url-box">
                  <input :value="currentUrl" readonly class="url-input mono" @focus="$event.target.select()" />
                  <button class="primary copy-btn" @click="copyUrl">
                    {{ copied ? '已复制！' : '复制链接' }}
                  </button>
                </div>
              </div>

              <div class="qr-container">
                <QrCode :url="currentUrl" :size="160" />
                <p class="qr-hint">客户端扫码或直接导入配置</p>
              </div>

              <div class="action-grid" v-if="activeTab === 'yaml'">
                <a :href="clashSchemeUrl" class="scheme-btn">
                  🚀 导入 Clash / Mihomo
                </a>
                <a :href="stashSchemeUrl" class="scheme-btn stash-btn">
                  💎 导入 Stash
                </a>
                <a :href="shadowrocketSchemeUrl" class="scheme-btn sr-btn">
                  🚀 导入 Shadowrocket
                </a>
                <a :href="currentUrl" target="_blank" class="download-link-btn" rel="noreferrer">
                  ⬇️ 查看原始 YAML
                </a>
              </div>
              <div class="action-grid" v-else>
                <a :href="currentUrl" target="_blank" class="download-link-btn" rel="noreferrer">
                  ⬇️ 直接打开 Script.js
                </a>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import QrCode from './QrCode.vue'
import { withAuthToken } from '../auth'
import { useAppStore } from '../stores/app'

const props = defineProps({
  open: { type: Boolean, default: false },
  needsToken: { type: Boolean, default: false },
})

const emit = defineEmits(['close'])
const store = useAppStore()

const activeTab = ref('yaml')
const copied = ref(false)

const currentPath = computed(() => '/yaml')

const currentUrl = computed(() => {
  const rel = withAuthToken(currentPath.value, props.needsToken)
  return `${window.location.origin}${rel}`
})

const clashSchemeUrl = computed(() => {
  return `clash://install-config?url=${encodeURIComponent(currentUrl.value)}&name=ClashSubParser`
})

const stashSchemeUrl = computed(() => {
  return `stash://install-config?url=${encodeURIComponent(currentUrl.value)}&name=ClashSubParser`
})

const shadowrocketSchemeUrl = computed(() => {
  try {
    return `sub://${btoa(currentUrl.value)}`
  } catch {
    return currentUrl.value
  }
})

watch(() => props.open, (val) => {
  if (val) copied.value = false
})

function close() {
  emit('close')
}

async function copyUrl() {
  try {
    await navigator.clipboard.writeText(currentUrl.value)
    copied.value = true
    store.showToast('订阅链接已复制到剪贴板', 'success')
    setTimeout(() => { copied.value = false }, 2000)
  } catch {
    store.showToast('复制失败，请手动选择复制', 'error')
  }
}
</script>

<style scoped>
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.6);
  backdrop-filter: blur(8px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 999;
  padding: 16px;
}

.export-modal {
  width: 100%;
  max-width: 480px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 20px;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.3);
  overflow: hidden;
  animation: scaleUp 0.18s ease-out;
}

@keyframes scaleUp {
  from { opacity: 0; transform: scale(0.95); }
  to { opacity: 1; transform: scale(1); }
}

.modal-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  padding: 20px 24px 14px;
  border-bottom: 1px solid var(--border);
}

.modal-head h3 {
  margin: 4px 0 0;
  font-size: 18px;
  font-weight: 700;
}

.close-btn {
  background: transparent;
  border: none;
  font-size: 18px;
  color: var(--ink-soft);
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 8px;
}

.close-btn:hover {
  background: var(--surface-2);
  color: var(--ink);
}

.modal-body {
  padding: 20px 24px 24px;
}

.export-tabs {
  display: flex;
  gap: 8px;
  background: var(--surface-2);
  padding: 4px;
  border-radius: 12px;
  margin-bottom: 18px;
}

.export-tab-btn {
  flex: 1;
  border: none;
  background: transparent;
  padding: 8px;
  font-size: 14px;
  font-weight: 600;
  color: var(--ink-soft);
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.export-tab-btn.active {
  background: var(--surface);
  color: var(--brand);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}

.url-card {
  margin-bottom: 18px;
}

.url-label {
  display: block;
  font-size: 12px;
  font-weight: 600;
  color: var(--ink-soft);
  margin-bottom: 6px;
}

.url-box {
  display: flex;
  gap: 8px;
}

.url-input {
  flex: 1;
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 8px 12px;
  font-size: 13px;
  color: var(--ink);
}

.copy-btn {
  white-space: nowrap;
  padding: 8px 16px;
  font-size: 13px;
  font-weight: 600;
  border-radius: 10px;
}

.qr-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 16px 0 12px;
}

.qr-hint {
  font-size: 12px;
  color: var(--ink-soft);
  margin: 10px 0 0;
}

.action-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  margin-top: 14px;
}

.scheme-btn,
.download-link-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 10px 14px;
  font-size: 13px;
  font-weight: 600;
  text-decoration: none;
  border-radius: 10px;
  transition: all 0.15s ease;
}

.scheme-btn {
  background: var(--brand);
  color: #fff;
}

.stash-btn {
  background: #7c3aed;
  color: #fff;
}

.sr-btn {
  background: #ea580c;
  color: #fff;
}

.scheme-btn:hover {
  opacity: 0.92;
  transform: translateY(-1px);
}

.download-link-btn {
  background: var(--surface-2);
  color: var(--ink);
  border: 1px solid var(--border);
}

.download-link-btn:hover {
  background: var(--surface);
  border-color: var(--brand);
}
</style>
