<template>
  <BaseDrawer :model-value="open" :title="node ? node.name : '节点详情'" @close="$emit('close')">
    <div v-if="node" class="space-y-6 text-xs font-mono">
      <!-- Tabs header -->
      <div class="flex items-center gap-2 border-b border-white/10 pb-2">
        <button
          v-for="tab in tabs"
          :key="tab.id"
          class="min-h-[44px] rounded-lg px-3.5 py-2 text-xs font-medium transition-colors cursor-pointer flex items-center justify-center"
          :class="[
            activeTab === tab.id
              ? 'border border-blue-500/40 bg-blue-600/20 text-blue-400'
              : 'border border-transparent text-slate-400 hover:text-white'
          ]"
          @click="activeTab = tab.id"
        >
          {{ tab.name }}
        </button>
      </div>

      <!-- Tab 1: Diagnostics -->
      <div v-if="activeTab === 'diagnostics'" class="space-y-4">
        <!-- Node Basic Info -->
        <div class="rounded-xl border border-white/10 bg-slate-900/60 p-4 space-y-2">
          <div class="flex justify-between items-center pb-2 border-b border-white/5">
            <span class="text-slate-400">PROTOCOL</span>
            <StatusBadge type="info" :text="(node.type || 'unknown').toUpperCase()" />
          </div>
          <div class="flex justify-between items-center py-1">
            <span class="text-slate-400">SERVER</span>
            <span class="text-white select-all">{{ node.server }}</span>
          </div>
          <div class="flex justify-between items-center py-1">
            <span class="text-slate-400">PORT</span>
            <span class="text-white">{{ node.port }}</span>
          </div>
          <div v-if="node.subscription_name" class="flex justify-between items-center py-1">
            <span class="text-slate-400">SUBSCRIPTION</span>
            <span class="text-slate-300">📁 {{ node.subscription_name }}</span>
          </div>
        </div>

        <!-- Outbound & Probing Results -->
        <div class="rounded-xl border border-white/10 bg-slate-900/60 p-4 space-y-3">
          <div class="text-slate-300 font-semibold border-b border-white/5 pb-2 flex items-center justify-between">
            <span>PROBE & CAPABILITIES</span>
            <StatusBadge
              :type="probe?.status === 'ok' ? 'success' : (probe?.status === 'fail' ? 'danger' : 'neutral')"
              :text="probe?.status ? probe.status.toUpperCase() : 'UNTESTED'"
            />
          </div>
          <div class="flex justify-between items-center py-1">
            <span class="text-slate-400">LATENCY</span>
            <span :class="probe?.latency_ms ? 'text-emerald-400 font-bold' : 'text-slate-500'">
              {{ probe?.latency_ms ? `${probe.latency_ms} ms` : 'N/A' }}
            </span>
          </div>
          <div class="flex justify-between items-center py-1">
            <span class="text-slate-400">OUTBOUND IP</span>
            <span class="text-white font-mono select-all">{{ probe?.ip || probe?.outbound_ip || 'N/A' }}</span>
          </div>
          <div class="flex justify-between items-center py-1">
            <span class="text-slate-400">COUNTRY / REGION</span>
            <span class="text-white">{{ probe?.country || nodeCountry || 'N/A' }}</span>
          </div>
          <div class="flex justify-between items-center py-1">
            <span class="text-slate-400">ASN & ORG</span>
            <span class="text-slate-300 truncate max-w-[220px]" :title="probeOrgText">
              {{ probeOrgText }}
            </span>
          </div>
          <div class="flex justify-between items-center py-1">
            <span class="text-slate-400">SPEED (DOWN)</span>
            <span :class="probe?.speed_mbps ? 'text-cyan-400 font-bold' : 'text-slate-500'">
              {{ probe?.speed_mbps ? `${probe.speed_mbps} Mbps` : 'N/A' }}
            </span>
          </div>
          <div v-if="probe?.error" class="p-2.5 rounded-lg border border-rose-500/20 bg-rose-500/10 text-rose-400">
            {{ probe.error }}
          </div>
        </div>

        <!-- Media Unlocks List -->
        <div class="rounded-xl border border-white/10 bg-slate-900/60 p-4 space-y-2.5">
          <span class="text-slate-300 font-semibold block pb-1 border-b border-white/5">
            🎬 STREAMING & AI UNLOCKS
          </span>
          <div class="grid grid-cols-2 gap-2 pt-1">
            <div
              v-for="platform in mediaPlatformList"
              :key="platform.key"
              class="flex items-center justify-between p-2 rounded-lg border border-white/5 bg-slate-950/40"
            >
              <div class="flex items-center gap-1.5">
                <span>{{ platform.icon }}</span>
                <span class="text-slate-300">{{ platform.name }}</span>
              </div>
              <span
                class="rounded px-1.5 py-0.5 text-[10px] font-bold"
                :class="getMediaStatusBadgeClass(probe?.media?.[platform.key])"
              >
                {{ getMediaStatusLabel(probe?.media?.[platform.key]) }}
              </span>
            </div>
          </div>
        </div>
      </div>

      <!-- Tab 2: Dialer Chain Management -->
      <div v-else-if="activeTab === 'chain'" class="space-y-4">
        <!-- Current Effective Chain Card -->
        <div class="rounded-xl border border-white/10 bg-slate-900/60 p-4 space-y-3">
          <div class="flex justify-between items-center border-b border-white/5 pb-2">
            <span class="text-slate-300 font-semibold">当前生效跳板链路</span>
            <span
              v-if="node.dialer_proxy"
              class="rounded-full px-2 py-0.5 text-[10px] font-mono"
              :class="node.chain_source === 'node' ? 'bg-blue-500/20 text-blue-400 border border-blue-500/30' : 'bg-purple-500/20 text-purple-400 border border-purple-500/30'"
            >
              {{ sourceLabel(node.chain_source) }}
            </span>
            <span v-else class="text-slate-500 text-xs">直连 (未挂链)</span>
          </div>

          <div v-if="node.dialer_proxy" class="flex justify-between items-center py-1">
            <span class="text-slate-400">DIALER PROXY</span>
            <strong class="text-blue-400 select-all font-mono">🔗 {{ node.dialer_proxy }}</strong>
          </div>

          <!-- Notice if inherited -->
          <div
            v-if="node.dialer_proxy && node.chain_source !== 'node'"
            class="p-2.5 rounded-lg border border-purple-500/30 bg-purple-950/20 text-purple-300 text-[11px]"
          >
            ℹ️ 当前跳板继承自{{ sourceLabel(node.chain_source) }}。若在此处保存新跳板，将创建专属节点级绑定并优先覆盖继承。
          </div>

          <!-- Clear chain button (only if node-level) -->
          <div v-if="node.dialer_proxy && node.chain_source === 'node'" class="pt-2 border-t border-white/5">
            <button
              class="w-full py-2 rounded-lg border border-rose-500/30 bg-rose-600/20 text-rose-400 font-medium hover:bg-rose-600/30 transition-colors cursor-pointer text-center"
              :disabled="clearingChain"
              @click="$emit('clear-chain', node)"
            >
              {{ clearingChain ? '正在清链…' : '🗑️ 清除专属节点级跳板' }}
            </button>
          </div>
        </div>

        <!-- Configure New / Replace Chain Card -->
        <div class="rounded-xl border border-white/10 bg-slate-900/60 p-4 space-y-3">
          <span class="text-slate-300 font-semibold block pb-1 border-b border-white/5">
            设置 / 替换跳板
          </span>

          <div class="space-y-2">
            <label class="text-slate-400 block">跳板目标类型</label>
            <div class="flex gap-2">
              <button
                type="button"
                class="flex-1 min-h-[44px] py-1.5 rounded-lg border text-xs cursor-pointer transition-colors flex items-center justify-center"
                :class="chainForm.dialer_type === 'node' ? 'border-blue-500 bg-blue-600/20 text-white font-bold' : 'border-white/10 text-slate-400 hover:text-white'"
                @click="chainForm.dialer_type = 'node'"
              >
                节点 (Node)
              </button>
              <button
                type="button"
                class="flex-1 min-h-[44px] py-1.5 rounded-lg border text-xs cursor-pointer transition-colors flex items-center justify-center"
                :class="chainForm.dialer_type === 'node_group' ? 'border-blue-500 bg-blue-600/20 text-white font-bold' : 'border-white/10 text-slate-400 hover:text-white'"
                @click="chainForm.dialer_type = 'node_group'"
              >
                策略组 (Node Group)
              </button>
            </div>
          </div>

          <div class="space-y-2">
            <label class="text-slate-400 block">选择前置跳板代理</label>
            <input
              v-model.trim="dialerSearch"
              type="text"
              placeholder="搜索可用跳板名称..."
              class="w-full min-h-[44px] rounded-lg border border-white/10 bg-slate-950/60 px-3 py-2 text-xs text-white placeholder-slate-500 focus:border-blue-500 focus:outline-hidden font-mono"
            />
            <select
              v-model="chainForm.dialer_ref"
              class="w-full min-h-[44px] rounded-lg border border-white/10 bg-slate-950/60 px-3 py-2 text-xs text-slate-200 focus:border-blue-500 focus:outline-hidden font-mono"
            >
              <option value="">-- 请选择跳板目标 --</option>
              <template v-if="chainForm.dialer_type === 'node'">
                <option v-for="c in filteredNodeCandidates" :key="c.name" :value="c.name">
                  {{ c.name }} ({{ (c.type || '').toUpperCase() }})
                </option>
              </template>
              <template v-else>
                <option v-for="g in filteredGroupCandidates" :key="g.name" :value="g.name">
                  {{ g.name }} (策略组)
                </option>
              </template>
            </select>
          </div>

          <button
            class="w-full min-h-[44px] py-2.5 rounded-lg border border-blue-500/40 bg-blue-600/30 text-blue-300 font-medium hover:bg-blue-600/40 transition-colors cursor-pointer text-center mt-2 flex items-center justify-center"
            :disabled="!chainForm.dialer_ref || savingChain"
            @click="submitSaveChain"
          >
            {{ savingChain ? '保存中…' : '🔗 保存跳板配置' }}
          </button>
        </div>
      </div>

      <!-- Tab 3: Node Raw Parameters -->
      <div v-else-if="activeTab === 'params'" class="space-y-4">
        <div class="rounded-xl border border-white/10 bg-slate-900/60 p-4 space-y-2">
          <span class="text-slate-300 font-semibold block pb-2 border-b border-white/5">
            NODE PARAMETERS
          </span>
          <div
            v-for="(val, key) in cleanNodeParams"
            :key="key"
            class="flex justify-between items-center py-1 border-b border-white/5"
          >
            <span class="text-slate-400">{{ key }}</span>
            <span class="text-white font-mono select-all truncate max-w-[260px]" :title="String(val)">
              {{ typeof val === 'object' ? JSON.stringify(val) : String(val) }}
            </span>
          </div>
        </div>
      </div>

      <!-- Tab 4: JSON Previews -->
      <div v-else-if="activeTab === 'json'" class="space-y-4">
        <div class="rounded-xl border border-white/10 bg-slate-900/60 p-4 space-y-3">
          <div class="flex justify-between items-center border-b border-white/5 pb-2">
            <span class="text-slate-300 font-semibold">SING-BOX OUTBOUND JSON</span>
            <button
              class="rounded-md border border-white/10 bg-slate-800/60 px-2.5 py-1 text-slate-300 hover:text-white cursor-pointer"
              @click="copyText(singboxJsonPreview)"
            >
              📋 复制
            </button>
          </div>
          <pre class="rounded-lg bg-slate-950 p-3 text-[11px] text-sky-400 overflow-auto max-h-60 font-mono">{{ singboxJsonPreview }}</pre>
        </div>

        <div class="rounded-xl border border-white/10 bg-slate-900/60 p-4 space-y-3">
          <div class="flex justify-between items-center border-b border-white/5 pb-2">
            <span class="text-slate-300 font-semibold">CLASH PROXY JSON</span>
            <button
              class="rounded-md border border-white/10 bg-slate-800/60 px-2.5 py-1 text-slate-300 hover:text-white cursor-pointer"
              @click="copyText(clashJsonPreview)"
            >
              📋 复制
            </button>
          </div>
          <pre class="rounded-lg bg-slate-950 p-3 text-[11px] text-cyan-400 overflow-auto max-h-60 font-mono">{{ clashJsonPreview }}</pre>
        </div>
      </div>

      <!-- Footer Action: Probe Single Node -->
      <div class="pt-2 border-t border-white/10">
        <button
          class="w-full py-2.5 rounded-lg border border-blue-500/40 bg-blue-600/20 text-blue-400 font-medium hover:bg-blue-600/30 transition-colors cursor-pointer text-center inline-flex items-center justify-center gap-2"
          :disabled="probingSingle"
          @click="$emit('probe-single', node)"
        >
          <span v-if="probingSingle" class="inline-block h-3.5 w-3.5 animate-spin rounded-full border-2 border-solid border-current border-r-transparent"></span>
          {{ probingSingle ? '质检探测执行中…' : '⚡ 单节点测速质检' }}
        </button>
      </div>
    </div>
  </BaseDrawer>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import BaseDrawer from '../ui/BaseDrawer.vue'
import StatusBadge from '../ui/StatusBadge.vue'
import {
  COUNTRY_NAME_MAP,
  MEDIA_PLATFORMS,
  type LedgerNodeItem,
  type ProbeRecord,
  resolveNodeCountryCode,
} from '../../views/nodeLedgerDomain'

const props = withDefaults(
  defineProps<{
    open: boolean
    node: LedgerNodeItem | null
    probe?: ProbeRecord | null
    nodeCandidates?: LedgerNodeItem[]
    groupCandidates?: any[]
    probingSingle?: boolean
    savingChain?: boolean
    clearingChain?: boolean
  }>(),
  {
    probe: null,
    nodeCandidates: () => [],
    groupCandidates: () => [],
    probingSingle: false,
    savingChain: false,
    clearingChain: false,
  }
)

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'probe-single', node: LedgerNodeItem): void
  (e: 'save-chain', payload: { nodeName: string; dialerType: string; dialerRef: string }): void
  (e: 'clear-chain', node: LedgerNodeItem): void
}>()

const tabs = [
  { id: 'diagnostics', name: '📊 真实诊断' },
  { id: 'chain', name: '🔗 跳板设置' },
  { id: 'params', name: '⚙️ 原始参数' },
  { id: 'json', name: '📦 JSON预览' },
]

const activeTab = ref('diagnostics')
const mediaPlatformList = MEDIA_PLATFORMS

const chainForm = reactive({
  dialer_type: 'node',
  dialer_ref: '',
})
const dialerSearch = ref('')

watch(
  () => props.node,
  (newNode) => {
    if (newNode) {
      chainForm.dialer_type = 'node'
      chainForm.dialer_ref = newNode.dialer_proxy || ''
      dialerSearch.value = ''
      activeTab.value = 'diagnostics'
    }
  },
  { immediate: true }
)

const nodeCountry = computed(() => {
  if (!props.node) return ''
  const code = resolveNodeCountryCode(props.node, props.probe || undefined)
  return COUNTRY_NAME_MAP[code] || code
})

const probeOrgText = computed(() => {
  if (!props.probe) return 'N/A'
  const p = props.probe
  const parts = []
  if (p.asn) parts.push(`AS${p.asn}`)
  if (p.organization) parts.push(p.organization)
  return parts.join(' ') || 'N/A'
})

const filteredNodeCandidates = computed(() => {
  const q = dialerSearch.value.trim().toLowerCase()
  const list = (props.nodeCandidates || []).filter((n) => n.name !== props.node?.name)
  if (!q) return list
  return list.filter((n) => n.name.toLowerCase().includes(q))
})

const filteredGroupCandidates = computed(() => {
  const q = dialerSearch.value.trim().toLowerCase()
  const list = props.groupCandidates || []
  if (!q) return list
  return list.filter((g) => (g.name || '').toLowerCase().includes(q))
})

const cleanNodeParams = computed(() => {
  if (!props.node) return {}
  const res: Record<string, any> = { ...props.node }
  delete res.group_names
  delete res.subscription_name
  return res
})

const singboxJsonPreview = computed(() => {
  if (!props.node) return ''
  const n = props.node
  const proto = (n.type || '').toLowerCase()
  return JSON.stringify(
    {
      type: proto,
      tag: 'proxy-out',
      server: n.server,
      server_port: n.port,
      uuid: n.uuid,
      password: n.password ? '***' : undefined,
      tls: n.tls ? { enabled: true, server_name: n.sni || n.servername } : undefined,
    },
    null,
    2
  )
})

const clashJsonPreview = computed(() => {
  if (!props.node) return ''
  return JSON.stringify(props.node, null, 2)
})

function sourceLabel(src?: string | null): string {
  if (src === 'node') return '专属节点级绑定'
  if (src === 'node_group') return '策略组继承'
  if (src === 'subscription') return '订阅继承'
  return src || '未知'
}

function getMediaStatusLabel(m: any): string {
  if (!m) return '未测'
  if (m.status === 'ok' || m.unlocked === true) return '解锁'
  if (m.status === 'full') return '全解'
  if (m.status === 'originals') return '自制'
  if (m.status === 'blocked') return '阻断'
  if (m.status === 'fail') return '失败'
  return '未知'
}

function getMediaStatusBadgeClass(m: any): string {
  if (!m) return 'bg-white/5 text-slate-500'
  if (m.status === 'ok' || m.status === 'full' || m.unlocked === true) {
    return 'bg-emerald-500/20 text-emerald-400'
  }
  if (m.status === 'originals') {
    return 'bg-amber-500/20 text-amber-400'
  }
  return 'bg-rose-500/20 text-rose-400'
}

function submitSaveChain() {
  if (!props.node || !chainForm.dialer_ref) return
  emit('save-chain', {
    nodeName: props.node.name,
    dialerType: chainForm.dialer_type,
    dialerRef: chainForm.dialer_ref,
  })
}

function copyText(txt: string) {
  if (!txt) return
  navigator.clipboard.writeText(txt)
}
</script>
