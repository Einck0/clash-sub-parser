<template>
  <AppModal
    :model-value="open"
    size="md"
    title="快速订阅与导出"
    @update:model-value="(val) => !val && close()"
    @close="close"
  >
    <div class="flex flex-col gap-5 pb-safe">
      <div class="flex flex-col gap-2">
        <label class="text-xs font-medium text-text-muted">导出模式</label>
        <div class="grid grid-cols-2 gap-2 rounded-lg bg-surface-base p-1 border border-border-subtle">
          <Button type="button" :variant="exportMode === 'merged' ? 'primary' : 'ghost'" size="sm" @click="exportMode = 'merged'">合并配置 <span class="text-[10px] opacity-80">(全部节点)</span></Button>
          <Button type="button" :variant="exportMode === 'subscription' ? 'primary' : 'ghost'" size="sm" @click="exportMode = 'subscription'">单订阅独立导出</Button>
        </div>
      </div>

      <!-- Single Subscription Dropdown (when mode is 'subscription') -->
      <div v-if="exportMode === 'subscription'" class="flex flex-col gap-1.5 rounded-lg bg-surface-base p-3 border border-border-subtle">
        <label class="text-xs font-medium text-text-muted">选择订阅源</label>
        <Select
          :model-value="selectedSubscriptionId ?? ''"
          class="w-full rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden"
          @update:model-value="(value) => { selectedSubscriptionId = value === '' ? null : Number(value) }"
        >
          <option v-if="!subscriptionsList.length" :value="null">暂无可用的有效订阅</option>
          <option
            v-for="sub in subscriptionsList"
            :key="sub.id"
            :value="sub.id"
          >
            {{ sub.name }} (ID: {{ sub.id }})
          </option>
        </Select>
      </div>

      <!-- Target Selection Tabs (5 Targets) -->
      <div class="flex flex-col gap-2">
        <div class="flex items-center justify-between">
          <label class="text-xs font-medium text-text-muted">导出目标核心</label>
          <span class="text-[10px] font-mono text-text-muted">{{ currentTargetDef.desc }}</span>
        </div>
        <div class="grid grid-cols-5 gap-1.5 rounded-lg bg-surface-base p-1 border border-border-subtle">
          <Button
            v-for="t in TARGET_DEFS"
            :key="t.key"
            type="button"
            :class="[
              'flex flex-col items-center justify-center py-2 px-1 rounded-md text-xs font-medium transition-colors cursor-pointer',
              selectedTarget === t.key
                ? 'bg-accent/15 text-accent border border-accent/30 shadow-xs'
                : 'text-text-muted hover:text-text-main hover:bg-surface-hover border border-transparent'
            ]"
            @click="selectedTarget = t.key"
          >
            <span>{{ t.name }}</span>
            <span class="text-[9px] font-mono opacity-70">{{ t.format }}</span>
          </Button>
        </div>
      </div>

      <!-- URL Input & One-click Copy -->
      <div class="flex flex-col gap-2">
        <label class="flex items-center justify-between text-xs font-medium text-text-muted">
          <span>完整订阅链接</span>
          <span class="font-mono text-[10px] text-text-muted">{{ currentTargetDef.badge }}</span>
        </label>
        <div class="flex gap-2">
          <Input
            :value="currentExportUrl"
            readonly
            class="flex-1 rounded-md border border-border-subtle bg-surface-hover px-3 py-2 font-mono text-xs text-text-main select-all focus:border-accent focus:outline-hidden"
            @focus="($event.target as HTMLInputElement).select()"
          />
          <Button
            type="button"
            :class="[
              'inline-flex items-center justify-center gap-1.5 rounded-md px-4 py-2 text-xs font-medium transition-colors cursor-pointer shrink-0',
              copied
                ? 'bg-success text-white'
                : 'bg-accent text-white hover:bg-accent-hover active:bg-accent-active'
            ]"
            @click="copyUrl"
          >
            <Check v-if="copied" class="h-3.5 w-3.5" aria-hidden="true" />
            <Copy v-else class="h-3.5 w-3.5" aria-hidden="true" />
            <span>{{ copied ? '已复制！' : '复制链接' }}</span>
          </Button>
        </div>
      </div>

      <!-- QR Code Section -->
      <div class="flex flex-col items-center justify-center rounded-lg bg-surface-base p-4 border border-border-subtle gap-2">
        <QrCode :url="currentQrPayload" :size="150" />
        <p class="text-xs text-text-muted text-center">
          客户端扫码直接导入 <span class="text-accent font-medium">{{ currentTargetDef.name }}</span>
        </p>
      </div>

      <!-- Action Grid: Scheme Wakeup, Download Raw -->
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5 pt-1">
        <a
          :href="currentSchemeUrl"
          class="flex min-h-[44px] items-center justify-center gap-2 rounded-lg bg-accent px-4 py-2.5 text-xs font-medium text-white shadow-xs hover:bg-accent-hover transition-colors cursor-pointer text-center"
        >
          <ExternalLink class="h-4 w-4" aria-hidden="true" />
          <span>{{ clientWakeupLabel }}</span>
        </a>

        <a
          :href="currentExportUrl"
          target="_blank"
          rel="noreferrer"
          class="flex min-h-[44px] items-center justify-center gap-1.5 rounded-lg border border-border-subtle bg-surface-base px-4 py-2.5 text-xs font-medium text-text-main hover:bg-surface-hover transition-colors cursor-pointer text-center"
        >
          <Download class="h-4 w-4" aria-hidden="true" />
          <span>下载 / 查看 {{ currentTargetDef.format }}</span>
        </a>
      </div>
    </div>
  </AppModal>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  Check,
  Copy,
  ExternalLink,
  Download,
} from 'lucide-vue-next'
import { Button, Input, Select } from './ui'
import AppModal from './ui/AppModal.vue'
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
      return '一键导入 Clash'
    case 'mihomo':
      return '一键导入 Mihomo'
    case 'stash':
      return '一键导入 Stash'
    case 'shadowrocket':
      return '唤醒 Shadowrocket'
    case 'sing-box':
      return '一键导入 Sing-box'
    default:
      return '唤醒客户端导入'
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
