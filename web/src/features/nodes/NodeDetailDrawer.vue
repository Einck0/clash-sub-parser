<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  ArrowPathIcon,
  BoltIcon,
  ShieldCheckIcon,
} from '@heroicons/vue/24/outline'
import DrawerCard from '../../ui/DrawerCard.vue'
import StatusBadge from '../../ui/StatusBadge.vue'
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
  protocolSupportedTargets,
  renderNodePreview,
  type NormalizedNode,
} from './nodeView'
import { t } from '../../locales'

const props = defineProps<{
  modelValue: boolean
  node: NormalizedNode | null
  sourceHistory: NodeSourceHistoryData | null
  loadingSourceHistory: boolean
  sourceHistoryError: string
  probeState: 'probing' | 'queued' | 'idle'
  probeFeedback: string
  savingConnection: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'probe-node', node: NormalizedNode): void
  (e: 'retry-source-history'): void
  (
    e: 'save-connection',
    payload: {
      draft: any
      onSuccess: (node: NormalizedNode) => void
      onError: (err: string) => void
    }
  ): void
}>()

const previewTarget = ref<'mihomo' | 'singbox'>('mihomo')
const connectionError = ref('')
const connectionSaved = ref(false)

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
  draftDisableSni.value = node.connection.disableSni
}

watch(
  () => props.node,
  (newNode) => {
    if (newNode) {
      syncDraftFromNode(newNode)
    }
  },
  { immediate: true }
)

const previewNodeSnippet = computed(() => {
  if (!props.node) return ''
  return renderNodePreview(props.node, previewTarget.value)
})

function handleApplyConnectionUpdate() {
  if (!props.node) return
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
    alpn,
    sni: draftSni.value,
    disableSni: draftDisableSni.value,
  }

  emit('save-connection', {
    draft: candidateDraft,
    onSuccess: (updatedNode) => {
      syncDraftFromNode(updatedNode)
      connectionSaved.value = true
    },
    onError: (err) => {
      connectionError.value = err
    },
  })
}

function handleClose() {
  emit('update:modelValue', false)
}
</script>

<template>
  <DrawerCard
    :model-value="modelValue"
    :title="node ? `${node.displayName} (${node.protocol.toUpperCase()})` : t('nodes.inspectNode')"
    :description="t('nodes.connectionProfile')"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div v-if="node" data-testid="node-detail-drawer" class="space-y-4 text-xs">
      <!-- Real-Time Probe Telemetry & Single-Node Quick Probe Bar -->
      <div
        data-testid="node-drawer-probe-panel"
        class="p-3 rounded-xl bg-base-200 border border-base-300 space-y-2.5"
      >
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="flex items-center gap-1.5 font-semibold">
            <BoltIcon class="w-4 h-4 text-primary shrink-0" />
            <span>实时测速与解锁状态</span>
            <span v-if="node.lastProbedAt" class="text-[11px] font-normal opacity-65">
              · 最后测速: {{ formatRelativeTime(node.lastProbedAt) }}
            </span>
          </div>
          <button
            type="button"
            data-testid="node-probe-btn"
            class="btn btn-primary btn-xs gap-1"
            :disabled="probeState === 'probing'"
            @click="emit('probe-node', node)"
          >
            <BoltIcon
              class="w-3.5 h-3.5"
              :class="{ 'animate-spin': probeState === 'probing' }"
            />
            <span>{{ probeState === 'probing' ? '检测中...' : '立即重测 (插队)' }}</span>
          </button>
        </div>

        <div class="flex flex-wrap items-center gap-1.5">
          <StatusBadge
            v-if="probeState !== 'idle'"
            data-testid="node-drawer-probe-state-badge"
            :label="probeState === 'probing' ? '⚡ 检测中' : '⏳ 队列等待'"
            :tone="probeState === 'probing' ? 'info' : 'warning'"
            :pulse="true"
          />
          <StatusBadge
            data-testid="node-drawer-health-badge"
            :label="`健康: ${nodeHealthBadge(node).label}`"
            :tone="nodeHealthBadge(node).tone"
          />
          <StatusBadge
            data-testid="node-drawer-latency-badge"
            :label="`响应延迟: ${formatNodeLatency(node)}`"
            :tone="nodeLatencyTone(node)"
          />
          <StatusBadge
            v-if="getNodeSpeedBadge(node)"
            :label="getNodeSpeedBadge(node)!.label"
            :tone="getNodeSpeedBadge(node)!.tone"
          />
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
            :label="`流媒体: ${nodeCapabilityLabel(node, 'streaming').label}`"
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
            :label="`AI 解锁: ${nodeCapabilityLabel(node, 'ai').label}`"
            :tone="nodeCapabilityLabel(node, 'ai').tone"
          />
          <StatusBadge
            data-testid="node-drawer-risk-badge"
            :label="`IP 风险: ${nodeRiskBadge(node).label}`"
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
          <span>{{ nodeHealthDiagnostic(node)?.detail }}</span>
        </div>

        <p v-if="probeFeedback" data-testid="node-probe-feedback" class="text-[11px] text-primary font-medium">
          {{ probeFeedback }}
        </p>
      </div>

      <!-- Platform Capabilities Breakdown (subcheck matrix) -->
      <div
        v-if="
          getNodePlatformBadges(node, 'streaming').length > 0 ||
          getNodePlatformBadges(node, 'ai').length > 0 ||
          getNodeSpeedBadge(node)
        "
        class="p-3 rounded-xl bg-base-200 border border-base-300 space-y-2"
      >
        <div class="flex items-center justify-between">
          <span class="font-semibold text-xs text-primary">{{ t('nodes.platformMatrix') }}</span>
          <span v-if="getNodeSpeedBadge(node)" class="font-mono text-[11px] opacity-75">
            {{ getNodeSpeedBadge(node)!.label }}
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

      <!-- Subscription Provenance & Reconcile Notice -->
      <div
        data-testid="node-reconcile-overwrite-notice"
        class="p-3 rounded-xl bg-base-200 border border-base-300 space-y-1"
      >
        <p class="font-semibold flex items-center justify-between gap-1.5">
          <span class="flex items-center gap-1.5">
            <ArrowPathIcon class="w-4 h-4 shrink-0 text-primary" />
            <span>订阅溯源与自动同步 (Reconcile)</span>
          </span>
          <span class="font-mono text-[11px] opacity-75">逻辑 ID: {{ node.logicalId }}</span>
        </p>
        <p class="opacity-80 leading-relaxed">
          直接编辑明文连接参数将原地更新当前节点；后续上游订阅源触发自动同步 (Reconcile) 时将从订阅源刷新节点配置。
        </p>
        <p
          v-if="node.sources && node.sources.length > 0"
          data-testid="node-provenance-sources"
          class="font-mono text-[11px] opacity-80"
        >
          来源订阅: {{ node.sources.map((s) => s.subscription_id).join(', ') }}
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

      <!-- Compiler Target Compatibility for this Node Protocol -->
      <div
        data-testid="node-target-compatibility"
        class="p-3 rounded-xl bg-base-200 border border-base-300 space-y-1.5"
      >
        <div class="flex items-center gap-1.5 font-semibold">
          <ShieldCheckIcon class="w-4 h-4 text-primary" />
          <span>{{ t('nodes.targetCompatibility') }} ({{ node.protocol.toUpperCase() }})</span>
        </div>
        <div class="flex flex-wrap items-center gap-1.5 font-mono">
          <span
            v-for="target in protocolSupportedTargets(node.protocol)"
            :key="target"
            class="badge badge-xs badge-success badge-outline uppercase"
          >
            {{ target }}: 已支持
          </span>
          <span
            v-if="node.protocol.toLowerCase() === 'vless'"
            class="badge badge-xs badge-warning badge-outline"
          >
            surge / qx: 不支持该协议
          </span>
          <span
            v-else-if="['wireguard', 'tuic', 'hysteria2'].includes(node.protocol.toLowerCase())"
            class="badge badge-xs badge-warning badge-outline"
          >
            qx: 不支持该协议
          </span>
        </div>
      </div>

      <!-- Connection Parameters Form (Direct Plaintext Edit & Inspect) -->
      <form
        data-testid="node-connection-form"
        class="p-3.5 rounded-xl bg-base-200/70 border border-base-300 space-y-3"
        @submit.prevent="handleApplyConnectionUpdate"
      >
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-2.5">
          <div class="flex flex-col gap-1 sm:col-span-1">
            <span class="text-xs font-semibold">节点显示名称</span>
            <input
              v-model="draftDisplayName"
              data-testid="node-display-name-input"
              class="input input-xs font-mono mt-1"
            />
          </div>
          <div class="flex flex-col gap-1 sm:col-span-1">
            <span class="text-xs font-semibold">服务器地址 / IP</span>
            <input
              v-model="draftServer"
              data-testid="node-server-input"
              class="input input-xs font-mono mt-1"
            />
          </div>
          <div class="flex flex-col gap-1 sm:col-span-1">
            <span class="text-xs font-semibold">端口</span>
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
            WireGuard 端点与对端配置
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">内网地址 (CIDR)</span>
              <input
                v-model="draftWgLocalAddress"
                data-testid="wg-local-address-input"
                placeholder="10.0.0.2/32, fd00::2/128"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">对端公钥 (Peer Public Key)</span>
              <input
                v-model="draftWgPublicKey"
                data-testid="wg-public-key-input"
                placeholder="Base64 对端公钥"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">客户端私钥 (Private Key)</span>
              <input
                v-model="draftWgPrivateKey"
                data-testid="wg-private-key-input"
                placeholder="Base64 客户端私钥"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">预共享密钥 (可选)</span>
              <input
                v-model="draftWgPreSharedKey"
                data-testid="wg-psk-input"
                placeholder="Base64 预共享密钥"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">最大传输单元 (MTU)</span>
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
              <span class="text-xs font-semibold">DNS 服务器</span>
              <input
                v-model="draftWgDns"
                data-testid="wg-dns-input"
                placeholder="1.1.1.1, 8.8.8.8"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1 sm:col-span-2">
              <span class="text-xs font-semibold">保留字节 (可选，3 个 uint8)</span>
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
            TUIC v5 连接与 QUIC 传输参数
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
              <span class="text-xs font-semibold">认证密码</span>
              <input
                v-model="draftPassword"
                data-testid="tuic-password-input"
                placeholder="TUIC 认证密码"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">拥塞控制算法</span>
              <select
                v-model="draftCongestionControl"
                data-testid="tuic-cc-select"
                class="select select-xs font-mono mt-1"
              >
                <option value="">（默认 / 未设置）</option>
                <option value="bbr">bbr</option>
                <option value="cubic">cubic</option>
                <option value="new_reno">new_reno</option>
              </select>
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">UDP 转发模式</span>
              <select
                v-model="draftUdpRelayMode"
                data-testid="tuic-udp-mode-select"
                class="select select-xs font-mono mt-1"
              >
                <option value="">（默认 / 未设置）</option>
                <option value="native">native</option>
                <option value="quic">quic</option>
              </select>
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">ALPN 协议协商</span>
              <input
                v-model="draftAlpn"
                data-testid="tuic-alpn-input"
                placeholder="h3"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">SNI 域名</span>
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
              <span class="text-xs font-mono">禁用 SNI (disable_sni)</span>
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
              <span class="text-xs font-semibold">加密方式 (Cipher)</span>
              <input
                v-model="draftMethod"
                data-testid="node-method-input"
                placeholder="aes-256-gcm, chacha20-ietf-poly1305..."
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1 sm:col-span-2">
              <span class="text-xs font-semibold">连接密码 / 密钥</span>
              <input
                v-model="draftPassword"
                data-testid="node-password-input"
                placeholder="输入协议连接密码或密钥"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">SNI 域名</span>
              <input
                v-model="draftSni"
                data-testid="node-sni-input"
                placeholder="sni.example.com"
                class="input input-xs font-mono mt-1"
              />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs font-semibold">ALPN 协议协商</span>
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
              <span class="text-xs font-mono">禁用 SNI (disable_sni)</span>
            </label>
          </div>
        </div>

        <div class="flex items-center justify-between pt-2">
          <span
            v-if="connectionError"
            data-testid="node-connection-error"
            class="text-error font-medium"
          >
            {{ connectionError }}
          </span>
          <span
            v-else-if="connectionSaved"
            data-testid="node-connection-saved"
            class="text-success font-medium"
          >
            连接参数已保存
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
      </form>

      <!-- Node Config Snippet Preview (Mihomo YAML / sing-box JSON) -->
      <div class="space-y-2">
        <div class="flex items-center justify-between">
          <span class="font-bold text-xs">节点导出格式预览</span>
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
    </div>

    <template #footer>
      <div class="flex justify-end w-full">
        <button
          type="button"
          data-testid="node-drawer-close-btn"
          class="btn btn-ghost btn-sm"
          @click="handleClose"
        >
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </DrawerCard>
</template>
