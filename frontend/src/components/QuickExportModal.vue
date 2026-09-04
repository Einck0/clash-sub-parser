<template>
  <Teleport to="body">
    <Transition name="modal-fade">
      <div
        v-if="open"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4"
        @click.self="close"
        @keydown.esc="close"
      >
        <div
          class="relative flex w-full max-w-lg flex-col rounded-2xl border border-white/10 bg-[#0F172A] text-[#F8FAFC] shadow-2xl overflow-hidden"
          role="dialog"
          aria-modal="true"
          aria-labelledby="export-modal-title"
        >
          <!-- Header -->
          <div class="flex items-center justify-between border-b border-white/10 px-6 py-4">
            <div>
              <div class="text-[10px] font-mono tracking-wider text-blue-400 uppercase">Quick Export</div>
              <h3 id="export-modal-title" class="text-lg font-semibold tracking-tight text-white">快速订阅与导出</h3>
            </div>
            <button
              type="button"
              class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white cursor-pointer transition-colors"
              aria-label="关闭导出弹窗"
              @click="close"
            >
              ✕
            </button>
          </div>

          <!-- Body -->
          <div class="flex flex-col gap-5 p-6 overflow-y-auto max-h-[85vh]">
            <!-- Mode Switcher: Merged vs Single Subscription -->
            <div class="flex flex-col gap-2">
              <label class="text-xs font-medium text-slate-400">导出模式</label>
              <div class="grid grid-cols-2 gap-2 rounded-xl bg-slate-900/60 p-1 border border-white/5">
                <button
                  type="button"
                  :class="[
                    'flex items-center justify-center gap-2 rounded-lg py-2 text-xs font-medium transition-all cursor-pointer',
                    exportMode === 'merged'
                      ? 'bg-blue-600 text-white shadow-xs'
                      : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/40'
                  ]"
                  @click="exportMode = 'merged'"
                >
                  <span>🌐 合并配置</span>
                  <span class="text-[10px] opacity-80">(全部节点)</span>
                </button>
                <button
                  type="button"
                  :class="[
                    'flex items-center justify-center gap-2 rounded-lg py-2 text-xs font-medium transition-all cursor-pointer',
                    exportMode === 'subscription'
                      ? 'bg-blue-600 text-white shadow-xs'
                      : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/40'
                  ]"
                  @click="exportMode = 'subscription'"
                >
                  <span>📋 单订阅独立导出</span>
                </button>
              </div>
            </div>

            <!-- Single Subscription Dropdown (when mode is 'subscription') -->
            <div v-if="exportMode === 'subscription'" class="flex flex-col gap-1.5 rounded-xl bg-slate-900/40 p-3 border border-white/5">
              <label class="text-xs font-medium text-slate-300">选择订阅源</label>
              <select
                v-model="selectedSubscriptionId"
                class="w-full rounded-lg border border-white/10 bg-[#1E293B] px-3 py-2 text-xs text-white focus:border-blue-500 focus:outline-hidden"
              >
                <option v-if="!subscriptionsList.length" :value="null">暂无可用的有效订阅</option>
                <option
                  v-for="sub in subscriptionsList"
                  :key="sub.id"
                  :value="sub.id"
                >
                  {{ sub.name }} (ID: {{ sub.id }})
                </option>
              </select>
            </div>

            <!-- Target Selection Tabs (5 Targets) -->
            <div class="flex flex-col gap-2">
              <div class="flex items-center justify-between">
                <label class="text-xs font-medium text-slate-400">导出目标核心</label>
                <span class="text-[10px] font-mono text-slate-500">{{ currentTargetDef.desc }}</span>
              </div>
              <div class="grid grid-cols-5 gap-1.5 rounded-xl bg-slate-900/60 p-1 border border-white/5">
                <button
                  v-for="t in TARGET_DEFS"
                  :key="t.key"
                  type="button"
                  :class="[
                    'flex flex-col items-center justify-center py-2 px-1 rounded-lg text-xs font-medium transition-all cursor-pointer',
                    selectedTarget === t.key
                      ? 'bg-blue-600/20 text-blue-400 border border-blue-500/30 shadow-xs'
                      : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/40 border border-transparent'
                  ]"
                  @click="selectedTarget = t.key"
                >
                  <span>{{ t.name }}</span>
                  <span class="text-[9px] font-mono opacity-70">{{ t.format }}</span>
                </button>
              </div>
            </div>

            <!-- URL Input & One-click Copy -->
            <div class="flex flex-col gap-2">
              <label class="flex items-center justify-between text-xs font-medium text-slate-400">
                <span>完整订阅链接</span>
                <span class="font-mono text-[10px] text-slate-500">{{ currentTargetDef.badge }}</span>
              </label>
              <div class="flex gap-2">
                <input
                  :value="currentExportUrl"
                  readonly
                  class="flex-1 rounded-lg border border-white/10 bg-[#1E293B] px-3 py-2 font-mono text-xs text-slate-200 select-all focus:border-blue-500 focus:outline-hidden"
                  @focus="($event.target as HTMLInputElement).select()"
                />
                <button
                  type="button"
                  :class="[
                    'inline-flex items-center justify-center rounded-lg px-4 py-2 text-xs font-medium transition-all cursor-pointer shrink-0',
                    copied
                      ? 'bg-emerald-600 text-white'
                      : 'bg-blue-600 text-white hover:bg-blue-500 active:scale-95'
                  ]"
                  @click="copyUrl"
                >
                  {{ copied ? '已复制！' : '复制链接' }}
                </button>
              </div>
            </div>

            <!-- QR Code Section -->
            <div class="flex flex-col items-center justify-center rounded-xl bg-slate-900/40 p-4 border border-white/5 gap-2">
              <QrCode :url="currentQrPayload" :size="150" />
              <p class="text-xs text-slate-400 text-center">
                客户端扫码直接导入 <span class="text-blue-400 font-medium">{{ currentTargetDef.name }}</span>
              </p>
            </div>

            <!-- Action Grid: Scheme Wakeup, Download Raw -->
            <div class="grid grid-cols-2 gap-2.5 pt-1">
              <a
                :href="currentSchemeUrl"
                class="flex items-center justify-center gap-2 rounded-xl bg-gradient-to-r from-blue-600 to-indigo-600 px-4 py-2.5 text-xs font-semibold text-white shadow-md hover:from-blue-500 hover:to-indigo-500 transition-all cursor-pointer text-center"
              >
                <span>{{ clientWakeupLabel }}</span>
              </a>

              <a
                :href="currentExportUrl"
                target="_blank"
                rel="noreferrer"
                class="flex items-center justify-center gap-1.5 rounded-xl border border-white/10 bg-[#1E293B] px-4 py-2.5 text-xs font-medium text-slate-200 hover:bg-slate-700/60 hover:text-white transition-all cursor-pointer text-center"
              >
                <span>⬇️ 下载 / 查看 {{ currentTargetDef.format }}</span>
              </a>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { getQuickExport, getSubscriptions } from '../api'
import { getAuthToken, withAuthToken } from '../auth'
import { useAppStore } from '../stores/app'
import QrCode from './QrCode.vue'

interface TargetDef {
  key: 'clash' | 'mihomo' | 'stash' | 'shadowrocket' | 'sing-box'
  name: string
  format: string
  badge: string
  ext: string
  desc: string
}

interface SubItem {
  id: number
  name: string
  url?: string
  enabled?: boolean
}

const TARGET_DEFS: TargetDef[] = [
  { key: 'clash', name: 'Clash', format: 'YAML', badge: 'Clash 官方', ext: 'yaml', desc: 'Clash Verge / ClashX' },
  { key: 'mihomo', name: 'Mihomo', format: 'YAML', badge: 'Meta 内核', ext: 'yaml', desc: 'Mihomo Party / Flclash' },
  { key: 'stash', name: 'Stash', format: 'YAML', badge: 'Stash iOS', ext: 'yaml', desc: 'Stash iOS / macOS' },
  { key: 'shadowrocket', name: 'Shadowrocket', format: 'TXT', badge: '小火箭', ext: 'txt', desc: '小火箭扫码与节点列表' },
  { key: 'sing-box', name: 'Sing-box', format: 'JSON', badge: 'sing-box 1.8+', ext: 'json', desc: 'sing-box 远程 Profile' },
]

const props = withDefaults(
  defineProps<{
    open: boolean
    needsToken?: boolean
    initialTarget?: string
    initialSubscriptionId?: number | null
  }>(),
  {
    open: false,
    needsToken: false,
    initialTarget: 'clash',
    initialSubscriptionId: null,
  }
)

const emit = defineEmits<{
  (e: 'close'): void
}>()

const store = useAppStore()
const exportMode = ref<'merged' | 'subscription'>('merged')
const selectedTarget = ref<TargetDef['key']>('clash')
const selectedSubscriptionId = ref<number | null>(null)
const copied = ref(false)
const remoteTargets = ref<Record<string, { url: string; scheme_url: string; qrcode_payload: string }> | null>(null)
const subscriptionsList = ref<SubItem[]>([])

const currentTargetDef = computed(() => {
  return TARGET_DEFS.find(t => t.key === selectedTarget.value) || TARGET_DEFS[0]
})

const selectedSubName = computed(() => {
  if (exportMode.value !== 'subscription' || !selectedSubscriptionId.value) {
    return 'ClashSubParser'
  }
  const match = subscriptionsList.value.find((s: SubItem) => s.id === selectedSubscriptionId.value)
  return match?.name || `Sub-${selectedSubscriptionId.value}`
})

// Safe base64 helper for Shadowrocket and browser safe encoding
function safeBtoa(str: string): string {
  try {
    return btoa(encodeURIComponent(str).replace(/%([0-9A-F]{2})/g, (_, p1) => String.fromCharCode(parseInt(p1, 16))))
  } catch {
    return btoa(str)
  }
}

// Compute client-side fallback export URL
const fallbackExportUrl = computed(() => {
  const origin = typeof window !== 'undefined' && window.location?.origin ? window.location.origin : ''
  const tKey = selectedTarget.value

  if (exportMode.value === 'subscription') {
    const subId = selectedSubscriptionId.value || (subscriptionsList.value[0]?.id ?? 1)
    const base = `${origin}/api/generate/subscription/${subId}?target=${tKey}`
    return withAuthToken(base, props.needsToken || Boolean(getAuthToken()))
  } else {
    const base = `${origin}/api/generate/${tKey}`
    return withAuthToken(base, props.needsToken || Boolean(getAuthToken()))
  }
})

// Current export URL: prefer backend-returned URL if available, else computed
const currentExportUrl = computed(() => {
  if (remoteTargets.value && remoteTargets.value[selectedTarget.value]?.url) {
    return remoteTargets.value[selectedTarget.value].url
  }
  return fallbackExportUrl.value
})

// Client Wakeup Scheme URL
const currentSchemeUrl = computed(() => {
  if (remoteTargets.value && remoteTargets.value[selectedTarget.value]?.scheme_url) {
    return remoteTargets.value[selectedTarget.value].scheme_url
  }

  const url = currentExportUrl.value
  const name = selectedSubName.value
  const tKey = selectedTarget.value

  if (tKey === 'clash' || tKey === 'mihomo') {
    return `clash://install-config?url=${encodeURIComponent(url)}&name=${encodeURIComponent(name)}`
  }
  if (tKey === 'stash') {
    return `stash://install-config?url=${encodeURIComponent(url)}&name=${encodeURIComponent(name)}`
  }
  if (tKey === 'shadowrocket') {
    return `sub://${safeBtoa(url)}`
  }
  if (tKey === 'sing-box') {
    return `sing-box://import-remote-profile?url=${encodeURIComponent(url)}#${encodeURIComponent(name)}`
  }
  return url
})

// QR Code Payload
const currentQrPayload = computed(() => {
  if (remoteTargets.value && remoteTargets.value[selectedTarget.value]?.qrcode_payload) {
    return remoteTargets.value[selectedTarget.value].qrcode_payload
  }
  if (selectedTarget.value === 'shadowrocket') {
    return currentSchemeUrl.value
  }
  return currentExportUrl.value
})

// Client Wakeup Button Text
const clientWakeupLabel = computed(() => {
  switch (selectedTarget.value) {
    case 'clash':
      return '🚀 一键导入 Clash'
    case 'mihomo':
      return '🚀 一键导入 Mihomo'
    case 'stash':
      return '💎 一键导入 Stash'
    case 'shadowrocket':
      return '🚀 唤醒 Shadowrocket'
    case 'sing-box':
      return '📦 一键导入 Sing-box'
    default:
      return '🚀 唤醒客户端导入'
  }
})

// Fetch QuickExport payload from backend
async function fetchExportData() {
  try {
    const params: { subscription_id?: number; target?: string; token?: string } = {}
    if (exportMode.value === 'subscription' && selectedSubscriptionId.value) {
      params.subscription_id = selectedSubscriptionId.value
    }
    const token = getAuthToken()
    if (token) {
      params.token = token
    }
    const { data } = await getQuickExport(params)
    if (data && data.targets) {
      remoteTargets.value = data.targets
    }
  } catch {
    // Graceful fallback to client-side generated URLs
  }
}

async function loadSubscriptions() {
  try {
    const { data } = await getSubscriptions()
    if (Array.isArray(data)) {
      subscriptionsList.value = data
      if (data.length > 0 && !selectedSubscriptionId.value) {
        selectedSubscriptionId.value = data[0].id
      }
    }
  } catch {
    // Keep empty
  }
}

watch(
  () => props.open,
  (val) => {
    if (val) {
      copied.value = false
      if (props.initialTarget && TARGET_DEFS.some(t => t.key === props.initialTarget)) {
        selectedTarget.value = props.initialTarget as TargetDef['key']
      }
      if (props.initialSubscriptionId) {
        exportMode.value = 'subscription'
        selectedSubscriptionId.value = props.initialSubscriptionId
      }
      loadSubscriptions()
      fetchExportData()
    }
  }
)

watch([exportMode, selectedSubscriptionId], () => {
  if (props.open) {
    fetchExportData()
  }
})

onMounted(() => {
  if (props.open) {
    loadSubscriptions()
    fetchExportData()
  }
})

function close() {
  emit('close')
}

async function copyUrl() {
  try {
    await navigator.clipboard.writeText(currentExportUrl.value)
    copied.value = true
    store.success('订阅链接已复制到剪贴板')
    setTimeout(() => {
      copied.value = false
    }, 2000)
  } catch {
    store.error('复制失败，请手动选中并复制')
  }
}
</script>

<style scoped>
.modal-fade-enter-active,
.modal-fade-leave-active {
  transition: opacity 0.2s ease;
}
.modal-fade-enter-from,
.modal-fade-leave-to {
  opacity: 0;
}
</style>
