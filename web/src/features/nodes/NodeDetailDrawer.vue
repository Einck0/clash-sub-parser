<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  ArrowPathIcon,
  BoltIcon,
  ExclamationTriangleIcon,
} from '@heroicons/vue/24/outline'
import DrawerCard from '../../ui/DrawerCard.vue'
import ConfirmModal from '../../ui/ConfirmModal.vue'
import StatusBadge from '../../ui/StatusBadge.vue'
import NodeSpeedResult from './NodeSpeedResult.vue'
import NodeTargetCompatibility from './NodeTargetCompatibility.vue'
import { displayName, displayText } from '../../utils/displayNames'
import NodeSourceHistoryPanel from './NodeSourceHistoryPanel.vue'
import type { NodeSourceHistoryData } from './sourceHistoryTypes'
import { formatRelativeTime } from '../probes/probeTypes'
import {
  formatNodeLatency,
  getNodePlatformBadges,
  getNodeSpeedBadge,
  nodeCapabilityLabel,
  nodeHealthBadge,
  nodeHealthDiagnostic,
  nodeLatencyTone,
  nodeRiskBadge,
  renderNodePreview,
  type NormalizedNode,
} from './nodeView'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'
import { useUnsavedChanges } from '../../composables/useUnsavedChanges'

const props = defineProps<{
  modelValue: boolean
  node: NormalizedNode | null
  sourceHistory: NodeSourceHistoryData | null
  loadingSourceHistory: boolean
  sourceHistoryError: string
  probeState: 'probing' | 'queued' | 'idle'
  probePending?: boolean
  probeRunState?: 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled' | 'expired'
  probeFeedback: string
  savingConnection: boolean
  loadingDetail?: boolean
  detailStale?: boolean
  detailErrorMessage?: string
  speedBusy?: boolean
  speedDisabled?: boolean
  speedFailed?: boolean
  speedFeedback?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'probe-node', node: NormalizedNode): void
  (e: 'speed-node', node: NormalizedNode): void
  (e: 'retry-source-history'): void
  (e: 'retry-detail', logicalId: string): void
  (
    e: 'save-connection',
    payload: {
      draft: any
      onSuccess: (node: NormalizedNode) => void
      onError: (err: string) => void
    }
  ): void
}>()

const speedBadge = computed(() => props.node && !props.speedBusy && !props.speedFailed ? getNodeSpeedBadge(props.node) : null)
const previewTarget = ref<'mihomo' | 'singbox'>('mihomo')
const connectionError = ref('')
const connectionSaved = ref(false)
const draftBaseline = ref('')
const draftEntityId = ref('')
const optionalConnectionBaseline = ref<{ sni: string; alpn: string[] }>({ sni: '', alpn: [] })
let draftSession = 0
let needsDraftInitialization = true
const discardAction = ref<'close' | 'reload' | null>(null)
const draftSnapshot = computed(() => JSON.stringify([
  draftDisplayName.value, draftServer.value, draftPort.value, draftWgLocalAddress.value,
  draftWgPublicKey.value, draftWgPrivateKey.value, draftWgPreSharedKey.value, draftWgMtu.value,
  draftWgDns.value, draftWgReserved.value, draftUuid.value, draftPassword.value, draftMethod.value,
  draftCongestionControl.value, draftUdpRelayMode.value, draftAlpn.value, draftSni.value, draftDisableSni.value,
]))
const dirty = computed(() => draftBaseline.value !== draftSnapshot.value)
const guardedDirty = computed(() =>
  props.modelValue && !!props.node && draftEntityId.value === props.node.logicalId && dirty.value
)
const { confirmDiscard: confirmNavigationDiscard } = useUnsavedChanges(guardedDirty)
function confirmNavigation() {
  return !props.savingConnection && confirmNavigationDiscard()
}

// Plaintext Connection Draft Fields
const draftDisplayName = ref('')
const draftServer = ref('')
const draftPort = ref<number>(0)
// WireGuard fields
const draftWgLocalAddress = ref('')
const draftWgPublicKey = ref('')
const draftWgPrivateKey = ref('')
const draftWgPreSharedKey = ref('')
const draftWgMtu = ref<number | undefined>(undefined)
const draftWgDns = ref('')
const draftWgReserved = ref('')
// TUIC / Generic protocol fields
const draftUuid = ref('')
const draftPassword = ref('')
const draftMethod = ref('')
const draftCongestionControl = ref('')
const draftUdpRelayMode = ref('')
const draftAlpn = ref('')
const draftSni = ref('')
const draftDisableSni = ref(false)

function syncDraftFromNode(node: NormalizedNode) {
  draftSession++
  connectionError.value = ''
  connectionSaved.value = false
  draftDisplayName.value = node.displayName
  draftServer.value = node.connection.server
  draftPort.value = node.connection.port
  draftWgLocalAddress.value = node.connection.localAddress.join(', ')
  draftWgPublicKey.value = node.connection.publicKey
  draftWgPrivateKey.value = node.connection.privateKey
  draftWgPreSharedKey.value = node.connection.preSharedKey
  draftWgMtu.value = node.connection.mtu
  draftWgDns.value = node.connection.dns.join(', ')
  draftWgReserved.value = node.connection.reserved.join(', ')
  draftUuid.value = node.connection.uuid
  draftPassword.value = node.connection.password
  draftMethod.value = node.connection.method || ''
  draftCongestionControl.value = node.connection.congestionControl
  draftUdpRelayMode.value = node.connection.udpRelayMode
  draftAlpn.value = node.connection.alpn.join(', ')
  draftSni.value = node.connection.sni
  optionalConnectionBaseline.value = { sni: node.connection.sni, alpn: [...node.connection.alpn] }
  draftDisableSni.value = node.connection.disableSni
  draftEntityId.value = node.logicalId
  draftBaseline.value = draftSnapshot.value
}

watch(
  () => [props.node?.logicalId, props.modelValue, props.loadingDetail] as const,
  ([logicalId, open, loading], previous) => {
    if (logicalId !== draftEntityId.value || (open && !previous?.[1])) needsDraftInitialization = true
    if (props.node && !loading && needsDraftInitialization) {
      syncDraftFromNode(props.node)
      needsDraftInitialization = false
      discardAction.value = null
    }
  },
  { immediate: true }
)

const previewNodeSnippet = computed(() => {
  if (!props.node) return ''
  return renderNodePreview(props.node, previewTarget.value)
})

function handleApplyConnectionUpdate() {
  if (!props.node || props.savingConnection) return
  const savingEntityId = props.node.logicalId
  const savingSession = draftSession
  const submittedSnapshot = draftSnapshot.value
  connectionError.value = ''
  connectionSaved.value = false

  const localAddress = draftWgLocalAddress.value
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
  const dns = draftWgDns.value
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
  const reserved = draftWgReserved.value
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
    .map((s) => Number(s))
    .filter((n) => !Number.isNaN(n))
  const alpn = draftAlpn.value
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)

  const candidateDraft = {
    displayName: draftDisplayName.value,
    server: draftServer.value,
    port: Number(draftPort.value),
    localAddress,
    publicKey: draftWgPublicKey.value,
    privateKey: draftWgPrivateKey.value,
    preSharedKey: draftWgPreSharedKey.value,
    mtu: draftWgMtu.value ? Number(draftWgMtu.value) : undefined,
    dns,
    reserved,
    uuid: draftUuid.value,
    password: draftPassword.value,
    method: draftMethod.value,
    congestionControl: draftCongestionControl.value,
    udpRelayMode: draftUdpRelayMode.value,
    alpn: JSON.stringify(alpn) === JSON.stringify(optionalConnectionBaseline.value.alpn) ? undefined : alpn,
    sni: draftSni.value === optionalConnectionBaseline.value.sni ? undefined : draftSni.value,
    disableSni: draftDisableSni.value,
  }

  emit('save-connection', {
    draft: candidateDraft,
    onSuccess: (updatedNode) => {
      if (props.node?.logicalId !== savingEntityId || savingSession !== draftSession) return
      if (draftSnapshot.value === submittedSnapshot) syncDraftFromNode(updatedNode)
      connectionSaved.value = true
    },
    onError: (err) => {
      if (props.node?.logicalId === savingEntityId && savingSession === draftSession) connectionError.value = err
    },
  })
}

function requestClose() {
  if (props.savingConnection) return
  if (dirty.value) discardAction.value = 'close'
  else emit('update:modelValue', false)
}

function confirmDiscard() {
  const action = discardAction.value
  discardAction.value = null
  if (props.node) syncDraftFromNode(props.node)
  if (action === 'close') emit('update:modelValue', false)
}

defineExpose({ requestClose, confirmNavigation })
</script>

<template>
  <DrawerCard
    :model-value="modelValue"
    :title="node ? `${node.displayName} (${node.protocol.toUpperCase()})` : t('nodes.inspectNode')"
    :description="t('nodes.connectionProfile')"
    @update:model-value="!$event && requestClose()"
  >
    <div v-if="node" data-testid="node-detail-drawer" class="space-y-4 text-xs min-w-0 [overflow-wrap:anywhere]">
      <!-- Stale / Detail Error Alert Banner with In-Place Retry -->
      <div
        v-if="detailStale"
        data-testid="node-detail-stale-banner"
        class="p-2.5 rounded-xl bg-warning/10 border border-warning/30 flex items-center justify-between gap-2 text-warning-content"
      >
        <div class="flex items-center gap-2 min-w-0">
          <ExclamationTriangleIcon class="w-4 h-4 text-warning shrink-0" />
          <span class="break-words [overflow-wrap:anywhere]">
            {{ displayText(detailErrorMessage || uiText('最新节点详情未能成功刷新，当前展示列表缓存数据', 'Latest node details could not refresh; cached details are shown.')) }}
          </span>
        </div>
        <button
          type="button"
          data-testid="node-detail-retry-btn"
          class="btn btn-warning btn-xs shrink-0 gap-1"
          :disabled="loadingDetail"
          @click="emit('retry-detail', node.logicalId)"
        >
          <ArrowPathIcon class="w-3.5 h-3.5" :class="{ 'animate-spin': loadingDetail }" />
          <span>{{ loadingDetail ? uiText('刷新中…', 'Refreshing…') : uiText('重试', 'Retry') }}</span>
        </button>
      </div>

      <!-- Real-Time Probe Telemetry & Single-Node Quick Probe Bar -->
      <div
        data-testid="node-drawer-probe-panel"
        class="p-3 rounded-xl bg-base-200 border border-base-300 space-y-2.5"
      >
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="flex items-center gap-1.5 font-semibold">
            <BoltIcon class="w-4 h-4 text-primary shrink-0" />
            <span>{{ uiText('节点检测与带宽状态', 'Probe and bandwidth status') }}</span>
            <span v-if="node.lastProbedAt" class="text-[11px] font-normal opacity-65">
              · {{ uiText('最后检测', 'Last probe') }}: {{ formatRelativeTime(node.lastProbedAt) }}
            </span>
          </div>
          <button
            type="button"
            data-testid="node-probe-btn"
            class="btn btn-primary btn-xs gap-1"
            :disabled="probePending || probeState !== 'idle' || !node.active"
            @click="emit('probe-node', node)"
          >
            <BoltIcon
              class="w-3.5 h-3.5"
              :class="{ 'animate-spin': probeState === 'probing' }"
            />
            <span>{{ probePending ? uiText('提交中…', 'Submitting…') : probeState === 'probing' ? uiText('检测中…', 'Probing…') : probeState === 'queued' ? uiText('排队中', 'Queued') : uiText('普通检测', 'Probe node') }}</span>
          </button>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <NodeSpeedResult :node="node" :busy="speedBusy" :failed="speedFailed" />
          <button type="button" data-testid="node-drawer-speed-btn" class="btn btn-outline btn-primary btn-xs" :disabled="speedDisabled || !node.active || detailStale" @click="emit('speed-node', node)">
            {{ speedBusy ? uiText('测速中…', 'Measuring…') : uiText('手动带宽测速', 'Measure bandwidth') }}
          </button>
          <p class="w-full text-base-content/65">{{ uiText('普通检测更新连通、延迟与能力，不测带宽。带宽测速需单独确认并消耗流量。', 'Probes update connectivity, latency and capabilities, not bandwidth. Bandwidth measurement needs separate confirmation and uses traffic.') }}</p>
          <p v-if="speedFeedback" class="w-full break-words [overflow-wrap:anywhere]" role="status">{{ displayText(speedFeedback) }}</p>
          <StatusBadge
            v-if="probeState !== 'idle'"
            data-testid="node-drawer-probe-state-badge"
            :label="probeState === 'probing' ? uiText('⚡ 检测中', '⚡ Probing') : uiText('⏳ 队列等待', '⏳ Queued')"
            :tone="probeState === 'probing' ? 'info' : 'warning'"
            :pulse="true"
          />
          <StatusBadge
            data-testid="node-drawer-health-badge"
            :label="`${uiText('健康', 'Health')}: ${nodeHealthBadge(node).label}`"
            :tone="nodeHealthBadge(node).tone"
          />
          <StatusBadge
            data-testid="node-drawer-latency-badge"
            :label="`${uiText('响应延迟', 'Latency')}: ${formatNodeLatency(node)}`"
            :tone="nodeLatencyTone(node)"
          />
          <StatusBadge v-if="speedBadge" :label="speedBadge.label" :tone="speedBadge.tone" />
          <template v-if="getNodePlatformBadges(node, 'streaming').length > 0">
            <StatusBadge
              v-for="badge in getNodePlatformBadges(node, 'streaming')"
              :key="`drawer-streaming-${badge.platform}`"
              :label="badge.label"
              :tone="badge.tone"
              :title="badge.tooltip"
            />
          </template>
          <StatusBadge
            v-else
            :label="`${uiText('流媒体', 'Streaming')}: ${nodeCapabilityLabel(node, 'streaming').label}`"
            :tone="nodeCapabilityLabel(node, 'streaming').tone"
          />
          <template v-if="getNodePlatformBadges(node, 'ai').length > 0">
            <StatusBadge
              v-for="badge in getNodePlatformBadges(node, 'ai')"
              :key="`drawer-ai-${badge.platform}`"
              :label="badge.label"
              :tone="badge.tone"
              :title="badge.tooltip"
            />
          </template>
          <StatusBadge
            v-else
            :label="`${uiText('AI 解锁', 'AI access')}: ${nodeCapabilityLabel(node, 'ai').label}`"
            :tone="nodeCapabilityLabel(node, 'ai').tone"
          />
          <StatusBadge
            data-testid="node-drawer-risk-badge"
            :label="`${uiText('IP 风险', 'IP risk')}: ${nodeRiskBadge(node).label}`"
            :tone="nodeRiskBadge(node).tone"
          />
        </div>

        <div
          v-if="probeState === 'idle' && nodeHealthDiagnostic(node)"
          data-testid="node-drawer-diagnostic-notice"
          class="rounded-lg px-2.5 py-2 text-[11px] leading-relaxed border"
          :class="
            nodeHealthDiagnostic(node)?.isBlockedByPolicyOrConfig
              ? 'bg-warning/10 border-warning/30 text-warning'
              : 'bg-base-300/50 border-base-300 text-base-content/80'
          "
        >
          <span class="font-semibold">{{ nodeHealthDiagnostic(node)?.shortLabel }}：</span>
          <span class="break-words [overflow-wrap:anywhere]">{{ displayText(nodeHealthDiagnostic(node)?.detail) }}</span>
        </div>

        <p v-if="probeFeedback" data-testid="node-probe-feedback" class="text-[11px] text-primary font-medium">
          {{ displayText(probeFeedback) }}
        </p>
      </div>

      <!-- Platform Capabilities Breakdown (subcheck matrix) -->
      <div
        v-if="
          getNodePlatformBadges(node, 'streaming').length > 0 ||
          getNodePlatformBadges(node, 'ai').length > 0 ||
          speedBadge
        "
        class="p-3 rounded-xl bg-base-200 border border-base-300 space-y-2"
      >
        <div class="flex items-center justify-between">
          <span class="font-semibold text-xs text-primary">{{ t('nodes.platformMatrix') }}</span>
          <span v-if="speedBadge" class="font-mono text-[11px] opacity-75">
            {{ speedBadge.label }}
          </span>
        </div>
        <div class="grid grid-cols-2 gap-2 text-[11px]">
          <div
            v-for="badge in [
              ...getNodePlatformBadges(node, 'streaming'),
              ...getNodePlatformBadges(node, 'ai'),
            ]"
            :key="`matrix-${badge.platform}`"
            class="flex items-center justify-between p-2 rounded-lg bg-base-100 border border-base-300"
          >
            <div class="min-w-0 pr-1">
              <span class="font-medium block truncate">{{ badge.platform.toUpperCase() }}</span>
              <span v-if="badge.subTier" class="text-[10px] opacity-60 block truncate">{{ badge.subTier }}</span>
            </div>
            <StatusBadge
              :label="badge.label"
              :tone="badge.tone"
              :title="badge.tooltip"
            />
          </div>
        </div>
      </div>

      <details class="rounded-xl border border-base-300 p-3 space-y-3" data-testid="node-source-details">
        <summary class="cursor-pointer font-semibold">{{ uiText('来源与历史追溯', 'Sources and history') }}</summary>
      <!-- Subscription Provenance & Reconcile Notice -->
      <div
        data-testid="node-reconcile-overwrite-notice"
        class="p-3 rounded-xl bg-base-200 border border-base-300 space-y-1"
      >
        <p class="font-semibold flex items-center justify-between gap-1.5">
          <span class="flex items-center gap-1.5">
            <ArrowPathIcon class="w-4 h-4 shrink-0 text-primary" />
            <span>{{ uiText('订阅溯源与自动同步 (Reconcile)', 'Subscription provenance and automatic synchronization (Reconcile)') }}</span>
          </span>

        </p>
        <p class="opacity-80 leading-relaxed">
          {{ uiText('直接编辑明文连接参数将原地更新当前节点；后续上游订阅源触发自动同步 (Reconcile) 时将从订阅源刷新节点配置。', 'Editing plaintext connection parameters updates this node in place. A later automatic subscription synchronization (Reconcile) refreshes the node configuration from its upstream source.') }}
        </p>
        <p
          v-if="node.sources && node.sources.length > 0"
          data-testid="node-provenance-sources"
          class="font-mono text-[11px] opacity-80"
        >
          {{ uiText('来源订阅', 'Source subscriptions') }}: {{ sourceHistory?.current_sources?.map((source) => displayName(source.name, uiText('未知订阅', 'Unknown subscription'), source.subscription_id)).join(uiText('、', ', ')) || uiText('来源信息暂不可用，请查看下方来源历史', 'Source information is unavailable. See the source history below.') }}
        </p>
      </div>

      <!-- Dedicated Source Attribution & Provenance Panel -->
      <NodeSourceHistoryPanel
        :logical-id="node.logicalId"
        :source-history="sourceHistory"
        :loading="loadingSourceHistory"
        :error="sourceHistoryError"
        @retry="emit('retry-source-history')"
      />
      </details>

      <!-- Compiler Target Compatibility for this Node Protocol -->
      <NodeTargetCompatibility :node="node" class="p-3 rounded-xl bg-base-200 border border-base-300" />

      <!-- Connection Parameters Form (Direct Plaintext Edit & Inspect) -->
      <form
        data-testid="node-connection-form"
        class="p-3.5 rounded-xl bg-base-200/70 border border-base-300 space-y-3"
        @submit.prevent="handleApplyConnectionUpdate"
      >
        <div class="flex flex-wrap items-center justify-between gap-2">
          <h3 class="font-semibold">{{ uiText('连接编辑工作区', 'Connection editor') }}</h3>
          <div class="flex flex-wrap items-center gap-2">
            <span v-if="dirty" data-testid="node-connection-dirty" class="badge badge-warning badge-outline badge-sm">{{ uiText('未保存', 'Unsaved') }}</span>
            <button type="button" data-testid="node-reload-draft" class="btn btn-ghost btn-xs" :disabled="savingConnection" @click="dirty ? (discardAction = 'reload') : node && syncDraftFromNode(node)">{{ uiText('重载最新详情', 'Reload latest details') }}</button>
          </div>
        </div>
        <p class="text-base-content/65">{{ uiText('后台刷新只更新检测结果，不覆盖此处草稿。保存失败后可继续修改。', 'Background refreshes update probe results without overwriting this draft. Failed saves keep your edits.') }}</p>
        <fieldset v-if="node.logicalId === draftEntityId" :disabled="savingConnection || loadingDetail" class="space-y-3 min-w-0">
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-2.5">
          <div class="flex flex-col gap-1 sm:col-span-1">
            <span class="text-xs font-semibold">{{ uiText('节点显示名称', 'Node display name') }}</span>
            <input
              v-model="draftDisplayName"
              data-testid="node-display-name-input"
              class="input input-xs font-mono mt-1"
            />
          </div>
          <div class="flex flex-col gap-1 sm:col-span-1">
            <span class="text-xs font-semibold">{{ uiText('服务器地址 / IP', 'Server address / IP') }}</span>
            <input
              v-model="draftServer"
              data-testid="node-server-input"
              class="input input-xs font-mono mt-1"
            />
          </div>
          <div class="flex flex-col gap-1 sm:col-span-1">
            <span class="text-xs font-semibold">{{ uiText('端口', 'Port') }}</span>
            <input
              v-model.number="draftPort"
              data-testid="node-port-input"
              type="number"
              min="1"
              max="65535"
              class="input input-xs font-mono mt-1"
            />
          </div>
        </div>

        <!-- WireGuard Specific Fields -->
        <div v-if="node.protocol.toLowerCase() === 'wireguard'" class="space-y-2.5 pt-2 border-t border-base-300">
          <div class="font-bold text-primary uppercase tracking-wider text-[11px]">
            {{ uiText('WireGuard 端点与对端配置', 'WireGuard endpoint and peer configuration') }}
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">{{ uiText('内网地址 (CIDR)', 'Local address (CIDR)') }}</span>
              <input
                v-model="draftWgLocalAddress"
                data-testid="wg-local-address-input"
                placeholder="10.0.0.2/32, fd00::2/128"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">{{ uiText('对端公钥 (Peer Public Key)', 'Peer public key') }}</span>
              <input
                v-model="draftWgPublicKey"
                data-testid="wg-public-key-input"
                :placeholder="uiText('Base64 对端公钥', 'Base64 peer public key')"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">{{ uiText('客户端私钥 (Private Key)', 'Client private key') }}</span>
              <input
                v-model="draftWgPrivateKey"
                data-testid="wg-private-key-input"
                :placeholder="uiText('Base64 客户端私钥', 'Base64 client private key')"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">{{ uiText('预共享密钥 (可选)', 'Pre-shared key (optional)') }}</span>
              <input
                v-model="draftWgPreSharedKey"
                data-testid="wg-psk-input"
                :placeholder="uiText('Base64 预共享密钥', 'Base64 pre-shared key')"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">{{ uiText('最大传输单元 (MTU)', 'Maximum transmission unit (MTU)') }}</span>
              <input
                v-model.number="draftWgMtu"
                data-testid="wg-mtu-input"
                type="number"
                min="576"
                max="9000"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">{{ uiText('DNS 服务器', 'DNS servers') }}</span>
              <input
                v-model="draftWgDns"
                data-testid="wg-dns-input"
                placeholder="1.1.1.1, 8.8.8.8"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1 sm:col-span-2">
              <span class="text-xs font-semibold">{{ uiText('保留字节 (可选，3 个 uint8)', 'Reserved bytes (optional, 3 uint8 values)') }}</span>
              <input
                v-model="draftWgReserved"
                data-testid="wg-reserved-input"
                placeholder="0, 0, 0"
                class="input input-xs font-mono mt-1"
              />
            </div>
          </div>
        </div>

        <!-- TUIC Specific Fields -->
        <div v-else-if="node.protocol.toLowerCase() === 'tuic'" class="space-y-2.5 pt-2 border-t border-base-300">
          <div class="font-bold text-primary uppercase tracking-wider text-[11px]">
            {{ uiText('TUIC v5 连接与 QUIC 传输参数', 'TUIC v5 connection and QUIC transport parameters') }}
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">UUID</span>
              <input
                v-model="draftUuid"
                data-testid="tuic-uuid-input"
                placeholder="00000000-0000-4000-8000-000000000001"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">{{ uiText('认证密码', 'Authentication password') }}</span>
              <input
                v-model="draftPassword"
                data-testid="tuic-password-input"
                :placeholder="uiText('TUIC 认证密码', 'TUIC authentication password')"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">{{ uiText('拥塞控制算法', 'Congestion control algorithm') }}</span>
              <select
                v-model="draftCongestionControl"
                data-testid="tuic-cc-select"
                class="select select-xs font-mono mt-1"
              >
                <option value="">{{ uiText('（默认 / 未设置）', '(Default / not set)') }}</option>
                <option value="bbr">bbr</option>
                <option value="cubic">cubic</option>
                <option value="new_reno">new_reno</option>
              </select>
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">{{ uiText('UDP 转发模式', 'UDP relay mode') }}</span>
              <select
                v-model="draftUdpRelayMode"
                data-testid="tuic-udp-mode-select"
                class="select select-xs font-mono mt-1"
              >
                <option value="">{{ uiText('（默认 / 未设置）', '(Default / not set)') }}</option>
                <option value="native">native</option>
                <option value="quic">quic</option>
              </select>
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">{{ uiText('自定义 ALPN（可选），留空恢复默认', 'Custom ALPN (optional); leave blank to restore defaults') }}</span>
              <input
                v-model="draftAlpn"
                data-testid="tuic-alpn-input"
                placeholder="h3"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">{{ uiText('自定义 SNI（可选），留空恢复默认', 'Custom SNI (optional); leave blank to restore defaults') }}</span>
              <input
                v-model="draftSni"
                data-testid="tuic-sni-input"
                placeholder="tuic.example.com"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <label class="flex items-center gap-2 cursor-pointer sm:col-span-2 pt-1">
              <input
                v-model="draftDisableSni"
                data-testid="tuic-disable-sni-checkbox"
                type="checkbox"
                class="checkbox checkbox-primary checkbox-xs"
              />
              <span class="text-xs font-mono">{{ uiText('禁用 SNI (disable_sni)', 'Disable SNI (disable_sni)') }}</span>
            </label>
          </div>
        </div>

        <!-- Other Protocols (SS, VMess, VLESS, Trojan, Hysteria2) Plaintext Credential Fields -->
        <div v-else class="pt-2 border-t border-base-300 space-y-2.5">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
            <div
              v-if="['vmess', 'vless'].includes(node.protocol.toLowerCase())"
              class="flex flex-col gap-1"
            >
              <span class="text-xs font-semibold">UUID</span>
              <input
                v-model="draftUuid"
                data-testid="node-uuid-input"
                placeholder="UUID"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div
              v-if="['ss', 'shadowsocks', 'vmess'].includes(node.protocol.toLowerCase())"
              class="flex flex-col gap-1"
            >
              <span class="text-xs font-semibold">{{ uiText('加密方式 (Cipher)', 'Cipher') }}</span>
              <input
                v-model="draftMethod"
                data-testid="node-method-input"
                placeholder="aes-256-gcm, chacha20-ietf-poly1305..."
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1 sm:col-span-2">
              <span class="text-xs font-semibold">{{ uiText('连接密码 / 密钥', 'Connection password / key') }}</span>
              <input
                v-model="draftPassword"
                data-testid="node-password-input"
                :placeholder="uiText('输入协议连接密码或密钥', 'Enter the protocol password or key')"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">{{ uiText('自定义 SNI（可选），留空恢复默认', 'Custom SNI (optional); leave blank to restore defaults') }}</span>
              <input
                v-model="draftSni"
                data-testid="node-sni-input"
                placeholder="sni.example.com"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">{{ uiText('自定义 ALPN（可选），留空恢复默认', 'Custom ALPN (optional); leave blank to restore defaults') }}</span>
              <input
                v-model="draftAlpn"
                data-testid="node-alpn-input"
                placeholder="h2, http/1.1"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <label class="flex items-center gap-2 cursor-pointer sm:col-span-2 pt-1">
              <input
                v-model="draftDisableSni"
                data-testid="node-disable-sni-checkbox"
                type="checkbox"
                class="checkbox checkbox-primary checkbox-xs"
              />
              <span class="text-xs font-mono">{{ uiText('禁用 SNI (disable_sni)', 'Disable SNI (disable_sni)') }}</span>
            </label>
          </div>
        </div>

        <div class="flex items-center justify-between pt-2">
          <span
            v-if="connectionError"
            data-testid="node-connection-error"
            class="text-error font-medium"
          >
            {{ displayText(connectionError) }}
          </span>
          <span
            v-else-if="connectionSaved"
            data-testid="node-connection-saved"
            class="text-success font-medium"
          >
            {{ uiText('连接参数已保存', 'Connection parameters saved') }}
          </span>
          <span v-else />
          <button
            type="submit"
            data-testid="node-save-connection-btn"
            class="btn btn-primary btn-xs"
            :disabled="savingConnection"
          >
            {{ t('common.save') }}
          </button>
        </div>
        </fieldset>
        <p v-else role="status">{{ uiText('正在加载连接参数…', 'Loading connection parameters…') }}</p>
      </form>

      <details class="rounded-xl border border-base-300 p-3 space-y-2" data-testid="node-preview-details">
        <summary class="font-semibold cursor-pointer">{{ uiText('已保存连接的导出预览', 'Export preview of the saved connection') }}</summary>
        <p data-testid="node-preview-scope-note" class="text-[11px] opacity-65">{{ uiText('本地片段仅展示已保存连接，不是完整编译输出；实际导出会应用客户端默认值。清空自定义 SNI 恢复默认，不会禁用 SNI 或证书校验。', 'This local snippet shows the saved connection, not the full compiled output; exports apply client defaults. Clearing custom SNI restores defaults without disabling SNI or certificate verification.') }}</p>
      <!-- Node Config Snippet Preview (Mihomo YAML / sing-box JSON) -->
      <div class="space-y-2">
        <div class="flex items-center justify-between">
          <span class="font-bold text-xs">{{ uiText('节点导出格式预览', 'Node export format preview') }}</span>
          <div class="flex items-center gap-1">
            <button
              type="button"
              data-testid="node-preview-target-mihomo"
              class="btn btn-xs font-mono"
              :class="previewTarget === 'mihomo' ? 'btn-primary' : 'btn-ghost'"
              @click="previewTarget = 'mihomo'"
            >
              Mihomo (YAML)
            </button>
            <button
              type="button"
              data-testid="node-preview-target-singbox"
              class="btn btn-xs font-mono"
              :class="previewTarget === 'singbox' ? 'btn-primary' : 'btn-ghost'"
              @click="previewTarget = 'singbox'"
            >
              sing-box (JSON)
            </button>
          </div>
        </div>
        <pre
          data-testid="node-config-preview"
          class="p-3 rounded-xl bg-base-300/50 border border-base-300 font-mono text-xs overflow-x-auto leading-relaxed"
        >{{ previewNodeSnippet }}</pre>
      </div>
      </details>
    </div>

    <template #footer>
      <div class="flex justify-end w-full">
        <button
          type="button"
          data-testid="node-drawer-close-btn"
          class="btn btn-ghost btn-sm"
          @click="requestClose"
        >
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </DrawerCard>
  <ConfirmModal
    :model-value="Boolean(discardAction)"
    :title="uiText('放弃未保存的连接修改？', 'Discard unsaved connection changes?')"
    :message="discardAction === 'reload' ? uiText('将使用当前最新详情替换草稿，未保存的修改将丢失。', 'Replace this draft with the latest details. Unsaved changes will be lost.') : uiText('关闭将丢弃当前连接草稿。保存失败的内容也不会保留。', 'Closing discards the connection draft, including edits from a failed save.')"
    :confirm-text="discardAction === 'reload' ? uiText('重载', 'Reload') : uiText('放弃并关闭', 'Discard and close')"
    tone="warning"
    data-testid="node-discard-draft-modal"
    @confirm="confirmDiscard"
    @update:model-value="!$event && (discardAction = null)"
  />
</template>
