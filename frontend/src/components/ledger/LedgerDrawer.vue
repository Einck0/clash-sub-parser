<template>
  <BaseDrawer :model-value="open" :title="node ? node.name : '节点详情'" @close="$emit('close')">
    <div v-if="node" class="space-y-6 text-xs font-mono">
      <!-- Tabs header -->
      <div
        class="flex items-center gap-2 border-b border-border-subtle pb-2"
        role="tablist"
        aria-label="详情面板标签"
      >
        <button
          v-for="(tab, index) in tabs"
          :key="tab.id"
          :id="`tab-${tab.id}`"
          type="button"
          role="tab"
          :aria-selected="activeTab === tab.id"
          :aria-controls="`panel-${tab.id}`"
          :tabindex="activeTab === tab.id ? 0 : -1"
          class="min-h-[44px] sm:min-h-[40px] rounded-md px-3.5 py-2 text-xs font-medium transition-colors cursor-pointer flex items-center justify-center gap-1.5 focus-ring"
          :class="[
            activeTab === tab.id
              ? 'border border-accent/40 bg-accent-subtle text-accent font-semibold'
              : 'border border-transparent text-text-muted hover:text-text-main hover:bg-surface-hover'
          ]"
          @click="activeTab = tab.id"
          @keydown.arrow-right.prevent="onTabKeydown(index, 1)"
          @keydown.arrow-left.prevent="onTabKeydown(index, -1)"
        >
          <component :is="tab.icon" class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
          <span>{{ tab.name }}</span>
        </button>
      </div>

      <!-- Tab 1: Diagnostics -->
      <div v-if="activeTab === 'diagnostics'" id="panel-diagnostics" role="tabpanel" aria-labelledby="tab-diagnostics" class="space-y-4">
        <!-- Node Basic Info -->
        <div class="rounded-lg border border-border-subtle bg-surface-base p-4 space-y-2">
          <div class="flex justify-between items-center pb-2 border-b border-border-subtle">
            <span class="text-text-muted uppercase">PROTOCOL</span>
            <StatusBadge type="info" :text="(node.type || 'unknown').toUpperCase()" />
          </div>
          <div class="flex justify-between items-center py-1">
            <span class="text-text-muted uppercase">SERVER</span>
            <span class="text-text-main select-all tabular-nums">{{ node.server }}</span>
          </div>
          <div class="flex justify-between items-center py-1">
            <span class="text-text-muted uppercase">PORT</span>
            <span class="text-text-main tabular-nums">{{ node.port }}</span>
          </div>
          <div v-if="node.subscription_name" class="flex justify-between items-center py-1">
            <span class="text-text-muted uppercase">SUBSCRIPTION</span>
            <span class="text-text-muted truncate max-w-[200px]">{{ node.subscription_name }}</span>
          </div>
        </div>

        <!-- Outbound & Probing Results -->
        <div class="rounded-lg border border-border-subtle bg-surface-base p-4 space-y-3">
          <div class="text-text-main font-semibold border-b border-border-subtle pb-2 flex items-center justify-between">
            <span class="uppercase">PROBE & CAPABILITIES</span>
            <StatusBadge
              :type="effectiveProbe?.status === 'ok' ? 'success' : (effectiveProbe?.status === 'fail' ? 'danger' : 'neutral')"
              :text="effectiveProbe?.status ? effectiveProbe.status.toUpperCase() : 'UNTESTED'"
            />
          </div>
          <div class="flex justify-between items-center py-1">
            <span class="text-text-muted uppercase">LATENCY</span>
            <span :class="effectiveProbe?.latency_ms ? 'text-status-success font-bold tabular-nums' : 'text-text-sub'">
              {{ effectiveProbe?.latency_ms ? `${effectiveProbe.latency_ms} ms` : 'N/A' }}
            </span>
          </div>
          <div class="flex justify-between items-center py-1">
            <span class="text-text-muted uppercase">OUTBOUND IP</span>
            <span class="text-text-main font-mono tabular-nums select-all">{{ effectiveProbe?.ip || effectiveProbe?.outbound_ip || 'N/A' }}</span>
          </div>
          <div class="flex justify-between items-center py-1">
            <span class="text-text-muted uppercase">COUNTRY / REGION</span>
            <span class="text-text-main">{{ effectiveProbe?.country || nodeCountry || 'N/A' }}</span>
          </div>
          <div class="flex justify-between items-center py-1">
            <span class="text-text-muted uppercase">ASN & ORG</span>
            <span class="text-text-muted truncate max-w-[220px]" :title="probeOrgText">
              {{ probeOrgText }}
            </span>
          </div>
          <div class="flex justify-between items-center py-1">
            <span class="text-text-muted uppercase">SPEED (DOWN)</span>
            <span :class="effectiveProbe?.speed_mbps ? 'text-status-info font-bold tabular-nums' : 'text-text-sub'">
              {{ effectiveProbe?.speed_mbps ? `${effectiveProbe.speed_mbps} Mbps` : 'N/A' }}
            </span>
          </div>
          <div v-if="effectiveProbe?.error" class="p-2.5 rounded-md border border-status-danger/30 bg-status-danger/10 text-status-danger">
            {{ effectiveProbe.error }}
          </div>
        </div>

        <!-- Media Unlocks Evidence Diagnostics List -->
        <div class="rounded-lg border border-border-subtle bg-surface-base p-4 space-y-3">
          <div class="flex items-center justify-between border-b border-border-subtle pb-2">
            <span class="text-text-main font-semibold uppercase tracking-wider">
              STREAMING & AI UNLOCKS · PROBE EVIDENCE (流媒体与 AI 证据诊断)
            </span>
            <div class="flex items-center gap-2">
              <span v-if="detailStatus === 'loading'" class="text-[10px] text-accent flex items-center gap-1 font-mono">
                <Loader2 class="h-3 w-3 animate-spin" />
                正在拉取完整证据链…
              </span>
              <span class="text-[10px] text-text-sub font-mono tabular-nums">
                {{ mediaPlatformList.length }} 平台
              </span>
            </div>
          </div>

          <!-- Loading Skeleton State -->
          <div v-if="detailStatus === 'loading'" class="space-y-2 py-2" aria-busy="true">
            <div
              v-for="idx in 3"
              :key="idx"
              class="h-16 rounded-md border border-border-subtle/50 bg-surface-hover/50 animate-pulse"
            />
          </div>

          <!-- Unavailable State with Retry -->
          <div
            v-else-if="detailStatus === 'unavailable'"
            class="rounded-md border border-border-subtle bg-surface-hover/60 p-3 text-xs space-y-2"
          >
            <div class="flex flex-wrap items-center justify-between gap-2">
              <span class="text-text-muted">当前节点未返回详细证据诊断链或详情请求不可用</span>
              <Button
                variant="ghost"
                size="sm"
                :icon="RotateCcw"
                @click="$emit('retry-detail')"
              >
                重试获取详情
              </Button>
            </div>
          </div>

          <!-- Dense Bordered Diagnostic Rows -->
          <div v-else class="space-y-2 pt-1">
            <div
              v-for="platform in mediaPlatformList"
              :key="platform.key"
              class="rounded-md border border-border-subtle bg-canvas p-2.5 space-y-2 text-xs font-mono"
            >
              <!-- Row 1: Platform name + Status/Verdict Badge + Region -->
              <div class="flex items-center justify-between gap-2">
                <div class="flex items-center gap-2 min-w-0">
                  <span class="text-text-sub text-xs">·</span>
                  <strong class="text-text-main font-semibold truncate">{{ platform.name }}</strong>
                  <span
                    v-if="getPlatformPres(platform).region"
                    class="px-1.5 py-0.2 rounded text-[10px] font-bold bg-surface-active text-text-main border border-border-subtle tabular-nums shrink-0"
                  >
                    {{ getPlatformPres(platform).region }}
                  </span>
                </div>
                <div class="flex items-center gap-1.5 shrink-0">
                  <span
                    class="rounded px-2 py-0.5 text-[10px] font-bold font-mono border inline-flex items-center gap-1"
                    :class="getPlatformPres(platform).badgeClass"
                  >
                    {{ getPlatformPres(platform).label }}
                  </span>
                </div>
              </div>

              <!-- Row 2: Diagnostic Attributes (Verdict, Confidence, Version, Timestamp) -->
              <div class="grid grid-cols-2 gap-x-3 gap-y-2 text-[11px] text-text-muted border-t border-border-subtle/60 pt-2">
                <div class="min-w-0">
                  <span class="text-text-sub text-[10px] uppercase block">VERDICT</span>
                  <span class="text-text-main font-medium block truncate">{{ getPlatformPres(platform).verdict || (getPlatformPres(platform).isFullUnlocked ? 'full' : (getPlatformPres(platform).isPartial ? 'originals_only' : 'unknown')) }}</span>
                </div>
                <div class="min-w-0">
                  <span class="text-text-sub text-[10px] uppercase block">CONFIDENCE</span>
                  <span class="text-text-main font-medium block truncate">{{ getPlatformPres(platform).confidence || (probe?.media?.[platform.key] ? 'verified' : 'unavailable') }}</span>
                </div>
                <div class="min-w-0">
                  <span class="text-text-sub text-[10px] uppercase block">VERSION</span>
                  <span class="text-text-main font-medium block truncate" :title="getPlatformPres(platform).evidenceVersion || 'catalogue-2026-09-05'">
                    {{ getPlatformPres(platform).evidenceVersion || 'catalogue-2026-09-05' }}
                  </span>
                </div>
                <div class="min-w-0">
                  <span class="text-text-sub text-[10px] uppercase block">TIMESTAMP</span>
                  <span class="text-text-main font-medium tabular-nums block truncate">
                    {{ formatTimestamp(getPlatformPres(platform).checkedAt || probe?.checked_at) }}
                  </span>
                </div>
              </div>

              <!-- Row 3: Sanitized Signals Summary (if present) -->
              <div
                v-if="getPlatformPres(platform).sanitizedSignalSummary"
                class="text-[10px] text-text-sub bg-surface-hover/60 px-2 py-1 rounded border border-border-subtle/40 truncate font-mono"
                :title="getPlatformPres(platform).sanitizedSignalSummary"
              >
                <span class="text-accent font-semibold">SIGNALS:</span> {{ getPlatformPres(platform).sanitizedSignalSummary }}
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Tab 2: Dialer Chain Management -->
      <div v-else-if="activeTab === 'chain'" id="panel-chain" role="tabpanel" aria-labelledby="tab-chain" class="space-y-4">
        <!-- Current Effective Chain Card -->
        <div class="rounded-lg border border-border-subtle bg-surface-base p-4 space-y-3">
          <div class="flex justify-between items-center border-b border-border-subtle pb-2">
            <span class="text-text-main font-semibold">当前生效跳板链路</span>
            <span
              v-if="node.dialer_proxy"
              class="rounded-full px-2 py-0.5 text-[10px] font-mono border"
              :class="node.chain_source === 'node' ? 'bg-accent-subtle text-accent border-accent/30' : 'bg-purple-500/20 text-purple-400 border-purple-500/30'"
            >
              {{ sourceLabel(node.chain_source) }}
            </span>
            <span v-else class="text-text-sub text-xs">直连 (未挂链)</span>
          </div>

          <div v-if="node.dialer_proxy" class="flex justify-between items-center py-1">
            <span class="text-text-muted">DIALER PROXY</span>
            <strong class="text-accent select-all font-mono">{{ node.dialer_proxy }}</strong>
          </div>

          <!-- Notice if inherited -->
          <div
            v-if="node.dialer_proxy && node.chain_source !== 'node'"
            class="p-2.5 rounded-md border border-purple-500/30 bg-purple-950/20 text-purple-300 text-[11px]"
          >
            当前跳板继承自{{ sourceLabel(node.chain_source) }}。若在此处保存新跳板，将创建专属节点级绑定并优先覆盖继承。
          </div>

          <!-- Clear chain button (only if node-level) -->
          <div v-if="node.dialer_proxy && node.chain_source === 'node'" class="pt-2 border-t border-border-subtle">
            <Button
              variant="danger"
              size="md"
              class="w-full"
              :disabled="clearingChain"
              :loading="clearingChain"
              :icon="Trash2"
              @click="$emit('clear-chain', node)"
            >
              {{ clearingChain ? '正在清链…' : '清除专属节点级跳板' }}
            </Button>
          </div>
        </div>

        <!-- Configure New / Replace Chain Card -->
        <div class="rounded-lg border border-border-subtle bg-surface-base p-4 space-y-3">
          <span class="text-text-main font-semibold block pb-1 border-b border-border-subtle">
            设置 / 替换跳板
          </span>

          <div class="space-y-2">
            <label class="text-text-muted block">跳板目标类型</label>
            <div class="flex gap-2">
              <button
                type="button"
                class="flex-1 min-h-[44px] py-1.5 rounded-md border text-xs cursor-pointer transition-colors flex items-center justify-center focus-ring"
                :class="chainForm.dialer_type === 'node' ? 'border-accent bg-accent-subtle text-accent font-bold ring-1 ring-accent/30' : 'border-border-subtle text-text-muted hover:text-text-main hover:bg-surface-hover'"
                @click="chainForm.dialer_type = 'node'"
              >
                节点 (Node)
              </button>
              <button
                type="button"
                class="flex-1 min-h-[44px] py-1.5 rounded-md border text-xs cursor-pointer transition-colors flex items-center justify-center focus-ring"
                :class="chainForm.dialer_type === 'node_group' ? 'border-accent bg-accent-subtle text-accent font-bold ring-1 ring-accent/30' : 'border-border-subtle text-text-muted hover:text-text-main hover:bg-surface-hover'"
                @click="chainForm.dialer_type = 'node_group'"
              >
                策略组 (Node Group)
              </button>
            </div>
          </div>

          <div class="space-y-2">
            <label class="text-text-muted block">选择前置跳板代理</label>
            <input
              v-model.trim="dialerSearch"
              type="text"
              placeholder="搜索可用跳板名称..."
              class="w-full min-h-[44px] rounded-md border border-border-subtle bg-canvas px-3 py-2 text-xs text-text-main placeholder:text-text-sub focus:border-accent focus:outline-hidden font-mono"
            />
            <select
              v-model="chainForm.dialer_ref"
              class="w-full min-h-[44px] rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono cursor-pointer"
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

          <Button
            variant="primary"
            size="md"
            class="w-full mt-2"
            :disabled="!chainForm.dialer_ref || savingChain"
            :loading="savingChain"
            :icon="LinkIcon"
            @click="submitSaveChain"
          >
            {{ savingChain ? '保存中…' : '保存跳板配置' }}
          </Button>
        </div>
      </div>

      <!-- Tab 3: Node Raw Parameters -->
      <div v-else-if="activeTab === 'params'" id="panel-params" role="tabpanel" aria-labelledby="tab-params" class="space-y-4">
        <div class="rounded-lg border border-border-subtle bg-surface-base p-4 space-y-2">
          <span class="text-text-main font-semibold block pb-2 border-b border-border-subtle uppercase">
            NODE PARAMETERS
          </span>
          <div
            v-for="(val, key) in cleanNodeParams"
            :key="key"
            class="flex justify-between items-center py-1 border-b border-border-subtle"
          >
            <span class="text-text-muted">{{ key }}</span>
            <span class="text-text-main font-mono tabular-nums select-all truncate max-w-[260px]" :title="String(val)">
              {{ typeof val === 'object' ? JSON.stringify(val) : String(val) }}
            </span>
          </div>
        </div>
      </div>

      <!-- Tab 4: JSON Previews -->
      <div v-else-if="activeTab === 'json'" id="panel-json" role="tabpanel" aria-labelledby="tab-json" class="space-y-4">
        <div class="rounded-lg border border-border-subtle bg-surface-base p-4 space-y-3">
          <div class="flex justify-between items-center border-b border-border-subtle pb-2">
            <span class="text-text-main font-semibold uppercase">SING-BOX OUTBOUND JSON</span>
            <Button
              variant="secondary"
              size="sm"
              :icon="Copy"
              @click="copyText(singboxJsonPreview)"
            >
              复制
            </Button>
          </div>
          <pre class="rounded-md bg-canvas p-3 text-[11px] text-accent overflow-auto max-h-60 font-mono tabular-nums border border-border-subtle">{{ singboxJsonPreview }}</pre>
        </div>

        <div class="rounded-lg border border-border-subtle bg-surface-base p-4 space-y-3">
          <div class="flex justify-between items-center border-b border-border-subtle pb-2">
            <span class="text-text-main font-semibold uppercase">CLASH PROXY JSON</span>
            <Button
              variant="secondary"
              size="sm"
              :icon="Copy"
              @click="copyText(clashJsonPreview)"
            >
              复制
            </Button>
          </div>
          <pre class="rounded-md bg-canvas p-3 text-[11px] text-text-muted overflow-auto max-h-60 font-mono tabular-nums border border-border-subtle">{{ clashJsonPreview }}</pre>
        </div>
      </div>

      <!-- Footer Action: Probe Single Node -->
      <div class="pt-2 border-t border-border-subtle">
        <Button
          variant="primary"
          size="lg"
          class="w-full"
          :disabled="probingSingle"
          :loading="probingSingle"
          :icon="Zap"
          @click="$emit('probe-single', node)"
        >
          {{ probingSingle ? '质检探测执行中…' : '单节点测速质检' }}
        </Button>
      </div>
    </div>
  </BaseDrawer>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import {
  Activity,
  Copy,
  FileCode,
  Link as LinkIcon,
  Loader2,
  RotateCcw,
  Sliders,
  Trash2,
  Zap,
} from 'lucide-vue-next'
import BaseDrawer from '../ui/BaseDrawer.vue'
import Button from '../ui/Button.vue'
import StatusBadge from '../ui/StatusBadge.vue'
import {
  COUNTRY_NAME_MAP,
  MEDIA_PLATFORMS,
  getMediaSemanticPresentation,
  type LedgerNodeItem,
  type MediaSemanticPresentation,
  type ProbeRecord,
  resolveNodeCountryCode,
} from '../../views/nodeLedgerDomain'

const props = withDefaults(
  defineProps<{
    open: boolean
    node: LedgerNodeItem | null
    probe?: ProbeRecord | null
    detailProbe?: ProbeRecord | null
    detailStatus?: 'idle' | 'loading' | 'ready' | 'unavailable'
    nodeCandidates?: LedgerNodeItem[]
    groupCandidates?: any[]
    probingSingle?: boolean
    savingChain?: boolean
    clearingChain?: boolean
  }>(),
  {
    probe: null,
    detailProbe: null,
    detailStatus: 'idle',
    nodeCandidates: () => [],
    groupCandidates: () => [],
    probingSingle: false,
    savingChain: false,
    clearingChain: false,
  }
)

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'retry-detail'): void
  (e: 'probe-single', node: LedgerNodeItem): void
  (e: 'save-chain', payload: { nodeName: string; dialerType: string; dialerRef: string }): void
  (e: 'clear-chain', node: LedgerNodeItem): void
}>()

const effectiveProbe = computed(() => props.detailProbe || props.probe)

const tabs = [
  { id: 'diagnostics', name: '真实诊断', icon: Activity },
  { id: 'chain', name: '跳板设置', icon: LinkIcon },
  { id: 'params', name: '原始参数', icon: Sliders },
  { id: 'json', name: 'JSON预览', icon: FileCode },
]

const activeTab = ref('diagnostics')
const mediaPlatformList = MEDIA_PLATFORMS

function onTabKeydown(currentIndex: number, direction: number) {
  const nextIndex = (currentIndex + direction + tabs.length) % tabs.length
  activeTab.value = tabs[nextIndex].id
  const el = document.getElementById(`tab-${tabs[nextIndex].id}`)
  el?.focus()
}

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
  const code = resolveNodeCountryCode(props.node, effectiveProbe.value || undefined)
  return COUNTRY_NAME_MAP[code] || code
})

const probeOrgText = computed(() => {
  if (!effectiveProbe.value) return 'N/A'
  const p = effectiveProbe.value
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

function getPlatformPres(platform: { key: string; name: string; short: string }): MediaSemanticPresentation {
  const item = effectiveProbe.value?.media?.[platform.key]
  return getMediaSemanticPresentation(item, platform)
}

function formatTimestamp(ts?: number): string {
  if (!ts) return 'N/A'
  try {
    const d = new Date(ts > 1e11 ? ts : ts * 1000)
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  } catch (_) {
    return String(ts)
  }
}

function getMediaStatusLabel(m: any): string {
  const pres = getMediaSemanticPresentation(m)
  return pres.label
}

function getMediaStatusBadgeClass(m: any): string {
  const pres = getMediaSemanticPresentation(m)
  return pres.badgeClass
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
