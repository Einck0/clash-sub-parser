<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch, type VNodeRef } from 'vue'
import {
  ArrowPathIcon,
  CheckCircleIcon,
  ExclamationTriangleIcon,
  EyeIcon,
  ShieldCheckIcon,
} from '@heroicons/vue/24/outline'
import { useWindowVirtualizer } from '@tanstack/vue-virtual'
import { useElementSize, useResizeObserver } from '@vueuse/core'
import DrawerCard from '../../ui/DrawerCard.vue'
import EmptyState from '../../ui/EmptyState.vue'
import ErrorStateCard from '../../ui/ErrorStateCard.vue'
import StatusBadge from '../../ui/StatusBadge.vue'
import {
  SUPPORTED_NODE_PROTOCOLS,
  nodeCapabilityLabel,
  nodeHealthBadge,
  nodeRiskBadge,
  protocolSupportedTargets,
  renderNodePreview,
  type NormalizedNode,
} from './nodeView'
import { useNodes } from './useNodes'
import { deriveColumns } from '../../composables/useResponsiveColumns'
import { t } from '../../locales'

const GAP = 16
const MIN_CARD_WIDTH = 288
const ESTIMATED_ROW_HEIGHT = 172

const {
  items,
  loading,
  loadingMore,
  savingConnection,
  error,
  total,
  hasMore,
  protocolFilter,
  searchQuery,
  selectedNode,
  load,
  loadMore,
  fetchNodeDetail,
  updateNodeConnection,
} = useNodes()

const drawerOpen = ref(false)
const previewTarget = ref<'mihomo' | 'singbox'>('mihomo')
const connectionError = ref('')
const connectionSaved = ref(false)

// Editable plaintext connection draft fields inside the Node Detail Drawer
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

async function openNodeDetail(node: NormalizedNode) {
  selectedNode.value = node
  syncDraftFromNode(node)
  drawerOpen.value = true
  const detailed = await fetchNodeDetail(node.logicalId)
  if (detailed) {
    syncDraftFromNode(detailed)
  }
}

async function handleApplyConnectionUpdate() {
  if (!selectedNode.value) return
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

  const res = await updateNodeConnection(selectedNode.value.logicalId, {
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
  })

  if (!res.ok) {
    connectionError.value = res.error || '连接参数无效'
    return
  }
  if (res.node) {
    syncDraftFromNode(res.node)
  }
  connectionSaved.value = true
}

const previewNodeSnippet = computed(() => {
  if (!selectedNode.value) return ''
  return renderNodePreview(selectedNode.value, previewTarget.value)
})

function selectProtocolFilter(proto: string) {
  protocolFilter.value = proto
  load()
}

const containerRef = ref<HTMLElement | null>(null)
const { width } = useElementSize(containerRef)

const columns = computed(() => deriveColumns(width.value, MIN_CARD_WIDTH, GAP))
const rowCount = computed(() => Math.ceil(items.value.length / columns.value))

function getRowItems(rowIndex: number) {
  const start = rowIndex * columns.value
  return items.value.slice(start, start + columns.value)
}

const scrollMargin = ref(0)

function updateScrollMargin() {
  if (containerRef.value && typeof window !== 'undefined') {
    const rect = containerRef.value.getBoundingClientRect()
    scrollMargin.value = Math.max(0, rect.top + window.scrollY)
  }
}

useResizeObserver(containerRef, updateScrollMargin)

const rowVirtualizer = useWindowVirtualizer(
  computed(() => ({
    count: rowCount.value,
    estimateSize: () => ESTIMATED_ROW_HEIGHT + GAP,
    overscan: 4,
    scrollMargin: scrollMargin.value,
    scrollToFn: (offset, options) => {
      if (
        typeof window !== 'undefined' &&
        typeof window.scrollTo === 'function' &&
        !navigator.userAgent.includes('jsdom')
      ) {
        window.scrollTo({ top: offset, behavior: options?.behavior })
      }
    },
  })),
)

const measureRow: VNodeRef = (element) => {
  if (element instanceof HTMLElement) {
    rowVirtualizer.value.measureElement(element)
  }
}

watch(columns, () => {
  nextTick(() => {
    rowVirtualizer.value.measure()
  })
})

watch(width, () => {
  nextTick(() => {
    rowVirtualizer.value.measure()
  })
})

function onScroll() {
  if (typeof window === 'undefined') return
  const scrollPosition = window.innerHeight + window.scrollY
  const threshold = document.documentElement.scrollHeight - 420
  if (scrollPosition >= threshold) {
    loadMore()
  }
}

onMounted(() => {
  load()
  if (typeof window !== 'undefined') {
    window.addEventListener('scroll', onScroll, { passive: true })
    nextTick(updateScrollMargin)
  }
})

onUnmounted(() => {
  if (typeof window !== 'undefined') {
    window.removeEventListener('scroll', onScroll)
  }
})
</script>

<template>
  <section class="space-y-5" aria-labelledby="nodes-title">
    <!-- Header -->
    <div class="flex flex-wrap items-end justify-between gap-3">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-primary">节点资产台账</p>
        <h2 id="nodes-title" class="mt-1 text-2xl font-bold">{{ t('nodes.title') }}</h2>
        <p class="mt-1 text-sm opacity-70">{{ t('nodes.subtitle') }}</p>
      </div>
      <div class="flex items-center gap-2">
        <span class="badge badge-ghost text-xs">{{ total.toLocaleString() }} {{ t('nodes.totalNodes') }}</span>
        <button class="btn btn-ghost btn-sm btn-square touch-manipulation" type="button" :title="t('common.refresh')" @click="load">
          <ArrowPathIcon class="h-4 w-4" :class="{ 'animate-spin': loading }" />
        </button>
      </div>
    </div>

    <!-- Protocol Filter Pills & Search Bar -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
      <div
        data-testid="node-protocol-filter"
        class="flex flex-wrap items-center gap-1.5 text-xs"
        role="group"
        aria-label="协议筛选"
      >
        <button
          type="button"
          class="btn btn-xs rounded-lg font-mono"
          :class="protocolFilter === 'all' ? 'btn-primary' : 'btn-ghost bg-base-200/70'"
          @click="selectProtocolFilter('all')"
        >
          {{ t('nodes.protocolAll') }}
        </button>
        <button
          v-for="proto in SUPPORTED_NODE_PROTOCOLS"
          :key="proto"
          type="button"
          class="btn btn-xs rounded-lg font-mono uppercase"
          :class="protocolFilter === proto ? 'btn-primary' : 'btn-ghost bg-base-200/70'"
          @click="selectProtocolFilter(proto)"
        >
          {{ proto }}
        </button>
      </div>

      <div class="flex items-center gap-2">
        <input
          v-model="searchQuery"
          data-testid="node-search-input"
          type="search"
          :placeholder="t('nodes.searchPlaceholder')"
          class="input input-bordered input-xs sm:input-sm w-full sm:w-60 font-mono text-xs"
          @keydown.enter="load"
        />
      </div>
    </div>

    <!-- Error Alert / Degraded State Card -->
    <ErrorStateCard
      v-if="error"
      :error="error"
      :retrying="loading"
      @retry="load"
    />

    <!-- Skeleton Loading with identical responsive min-card width -->
    <div
      v-if="loading && items.length === 0"
      class="grid gap-4 [grid-template-columns:repeat(auto-fit,minmax(min(100%,288px),1fr))]"
    >
      <div v-for="index in 6" :key="index" class="skeleton h-32 rounded-box" />
    </div>

    <!-- Virtualized Responsive Node Grid Normal page flow with width-derived columns -->
    <div v-else ref="containerRef" class="w-full min-w-0">
      <div
        v-if="items.length > 0"
        class="relative w-full"
        :style="{ height: `${rowVirtualizer.getTotalSize()}px` }"
      >
        <div
          v-for="virtualRow in rowVirtualizer.getVirtualItems()"
          :key="String(virtualRow.key)"
          :ref="measureRow"
          :data-index="virtualRow.index"
          class="absolute left-0 top-0 w-full"
          :style="{
            transform: `translateY(${virtualRow.start - scrollMargin}px)`,
            paddingBottom: `${GAP}px`,
          }"
        >
          <div
            class="grid min-w-0 max-w-full"
            :style="{
              gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
              gap: `${GAP}px`,
            }"
          >
            <article
              v-for="node in getRowItems(virtualRow.index)"
              :key="node.logicalId"
              data-testid="node-card"
              class="card border border-base-300 bg-base-200 shadow-sm transition hover:border-primary/40 hover:shadow-md min-w-0 max-w-full overflow-hidden cursor-pointer"
              @click="openNodeDetail(node)"
            >
              <div class="card-body gap-3 p-4 sm:p-5 min-w-0 max-w-full overflow-hidden">
                <div class="flex flex-wrap items-start justify-between gap-2 sm:gap-3 min-w-0">
                  <div class="flex min-w-0 items-center gap-2.5 sm:gap-3 flex-1">
                    <div class="rounded-xl bg-primary/10 p-2 text-primary shrink-0">
                      <CheckCircleIcon v-if="node.active" class="h-5 w-5" />
                      <ExclamationTriangleIcon v-else class="h-5 w-5" />
                    </div>
                    <div class="min-w-0 flex-1">
                      <h3 class="truncate font-semibold text-sm sm:text-base">{{ node.displayName }}</h3>
                      <p class="mt-0.5 truncate font-mono text-xs opacity-60">{{ node.logicalId }}</p>
                    </div>
                  </div>
                  <div class="flex items-center gap-1.5 sm:gap-2 shrink-0">
                    <span class="badge badge-outline font-mono text-xs uppercase">{{ node.protocol }}</span>
                    <StatusBadge
                      :label="node.active ? t('common.enabled') : t('common.disabled')"
                      :tone="node.active ? 'success' : 'warning'"
                    />
                  </div>
                </div>

                <!-- Protocol Connection Summary & Target Boundary Badges -->
                <div class="flex flex-wrap items-center gap-1.5 text-[11px] font-mono opacity-80">
                  <span class="badge badge-xs badge-ghost">
                    兼容目标: {{ protocolSupportedTargets(node.protocol).join('/') }}
                  </span>
                  <span v-if="node.connection.server && node.connection.port" class="badge badge-xs badge-ghost">
                    {{ node.connection.server }}:{{ node.connection.port }}
                  </span>
                  <template v-if="node.protocol.toLowerCase() === 'wireguard'">
                    <span v-if="node.connection.localAddress.length" class="badge badge-xs badge-info badge-outline">
                      内网 IP: {{ node.connection.localAddress.join(', ') }}
                    </span>
                    <span v-if="node.connection.mtu" class="badge badge-xs badge-ghost">
                      MTU: {{ node.connection.mtu }}
                    </span>
                  </template>
                  <template v-else-if="node.protocol.toLowerCase() === 'tuic'">
                    <span v-if="node.connection.congestionControl" class="badge badge-xs badge-info badge-outline">
                      拥塞控制: {{ node.connection.congestionControl }}
                    </span>
                    <span v-if="node.connection.udpRelayMode" class="badge badge-xs badge-ghost">
                      UDP 模式: {{ node.connection.udpRelayMode }}
                    </span>
                  </template>
                </div>

                <div class="flex flex-wrap items-center justify-between gap-2 border-t border-base-300 pt-3 text-xs min-w-0">
                  <div class="flex flex-wrap items-center gap-1.5 sm:gap-2 min-w-0">
                    <span class="mr-1 text-xs opacity-60 shrink-0">{{ t('nodes.capabilities') }}</span>
                    <StatusBadge
                      :label="`健康: ${nodeHealthBadge(node).label}`"
                      :tone="nodeHealthBadge(node).tone"
                    />
                    <StatusBadge
                      :label="`风险: ${nodeRiskBadge(node).label}`"
                      :tone="nodeRiskBadge(node).tone"
                    />
                    <StatusBadge
                      :label="`${t('nodes.streaming')}: ${nodeCapabilityLabel(node, 'streaming').label}`"
                      :tone="nodeCapabilityLabel(node, 'streaming').tone"
                    />
                    <StatusBadge
                      :label="`${t('nodes.ai')}: ${nodeCapabilityLabel(node, 'ai').label}`"
                      :tone="nodeCapabilityLabel(node, 'ai').tone"
                    />
                    <span
                      v-if="node.probeStale"
                      class="badge badge-warning badge-sm gap-1 font-mono text-[11px]"
                      title="探测观测数据已过期（超出保鲜窗口）"
                    >
                      探测已过期
                    </span>
                    <span
                      v-if="node.probeMissing"
                      class="badge badge-ghost badge-sm gap-1 font-mono text-[11px] opacity-75"
                      title="尚无探针观测记录"
                    >
                      未探测
                    </span>
                  </div>

                  <button
                    type="button"
                    data-testid="node-inspect-btn"
                    class="btn btn-ghost btn-xs font-mono shrink-0"
                    @click.stop="openNodeDetail(node)"
                  >
                    {{ t('nodes.inspectNode') }}
                  </button>
                </div>
              </div>
            </article>
          </div>
        </div>
      </div>

      <!-- Terminal content reserves inherited dynamic dock inset for clean separation -->
      <div
        v-if="items.length > 0"
        class="py-4 text-center"
        :style="{ paddingBottom: 'var(--content-dock-inset, 32px)' }"
      >
        <div v-if="loadingMore" class="flex justify-center py-2">
          <span class="loading loading-spinner loading-sm text-primary" />
        </div>
        <p v-if="!hasMore" class="text-xs opacity-60">{{ t('nodes.allLoaded') }}</p>
      </div>

      <!-- Empty State -->
      <EmptyState
        v-if="!items.length && !loading"
        :icon="EyeIcon"
        :title="t('nodes.emptyTitle')"
        :description="t('nodes.emptyDesc')"
      />
    </div>

    <!-- Node Detail, Edit & Preview Drawer -->
    <DrawerCard
      v-model="drawerOpen"
      :title="selectedNode ? `${selectedNode.displayName} (${selectedNode.protocol.toUpperCase()})` : t('nodes.inspectNode')"
      :description="t('nodes.connectionProfile')"
    >
      <div v-if="selectedNode" data-testid="node-detail-drawer" class="space-y-4 text-xs">
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
            <span class="font-mono text-[11px] opacity-75">逻辑 ID: {{ selectedNode.logicalId }}</span>
          </p>
          <p class="opacity-80 leading-relaxed">
            直接编辑明文连接参数将原地更新当前节点；后续上游订阅源触发自动同步 (Reconcile) 时将从订阅源刷新节点配置。
          </p>
          <p
            v-if="selectedNode.sources && selectedNode.sources.length > 0"
            data-testid="node-provenance-sources"
            class="font-mono text-[11px] opacity-80"
          >
            来源订阅: {{ selectedNode.sources.map((s) => s.subscription_id).join(', ') }}
          </p>
        </div>

        <!-- Compiler Target Compatibility for this Node Protocol -->
        <div
          data-testid="node-target-compatibility"
          class="p-3 rounded-xl bg-base-200 border border-base-300 space-y-1.5"
        >
          <div class="flex items-center gap-1.5 font-semibold">
            <ShieldCheckIcon class="w-4 h-4 text-primary" />
            <span>{{ t('nodes.targetCompatibility') }} ({{ selectedNode.protocol.toUpperCase() }})</span>
          </div>
          <div class="flex flex-wrap items-center gap-1.5 font-mono">
            <span
              v-for="target in protocolSupportedTargets(selectedNode.protocol)"
              :key="target"
              class="badge badge-xs badge-success badge-outline uppercase"
            >
              {{ target }}: 已支持
            </span>
            <span
              v-if="selectedNode.protocol.toLowerCase() === 'vless'"
              class="badge badge-xs badge-warning badge-outline"
            >
              surge / qx: 不支持该协议
            </span>
            <span
              v-else-if="['wireguard', 'tuic', 'hysteria2'].includes(selectedNode.protocol.toLowerCase())"
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
            <label class="form-control sm:col-span-1">
              <span class="label-text text-xs font-semibold">节点显示名称</span>
              <input
                v-model="draftDisplayName"
                data-testid="node-display-name-input"
                class="input input-bordered input-xs font-mono mt-1"
              />
            </label>
            <label class="form-control sm:col-span-1">
              <span class="label-text text-xs font-semibold">服务器地址 / IP</span>
              <input
                v-model="draftServer"
                data-testid="node-server-input"
                class="input input-bordered input-xs font-mono mt-1"
              />
            </label>
            <label class="form-control sm:col-span-1">
              <span class="label-text text-xs font-semibold">端口</span>
              <input
                v-model.number="draftPort"
                data-testid="node-port-input"
                type="number"
                min="1"
                max="65535"
                class="input input-bordered input-xs font-mono mt-1"
              />
            </label>
          </div>

          <!-- WireGuard Specific Fields -->
          <div v-if="selectedNode.protocol.toLowerCase() === 'wireguard'" class="space-y-2.5 pt-2 border-t border-base-300">
            <div class="font-bold text-primary uppercase tracking-wider text-[11px]">
              WireGuard 端点与对端配置
            </div>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
              <label class="form-control">
                <span class="label-text text-xs font-semibold">内网地址 (CIDR)</span>
                <input
                  v-model="draftWgLocalAddress"
                  data-testid="wg-local-address-input"
                  placeholder="10.0.0.2/32, fd00::2/128"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">对端公钥 (Peer Public Key)</span>
                <input
                  v-model="draftWgPublicKey"
                  data-testid="wg-public-key-input"
                  placeholder="Base64 对端公钥"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">客户端私钥 (Private Key)</span>
                <input
                  v-model="draftWgPrivateKey"
                  data-testid="wg-private-key-input"
                  placeholder="Base64 客户端私钥"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">预共享密钥 (可选)</span>
                <input
                  v-model="draftWgPreSharedKey"
                  data-testid="wg-psk-input"
                  placeholder="Base64 预共享密钥"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">最大传输单元 (MTU)</span>
                <input
                  v-model.number="draftWgMtu"
                  data-testid="wg-mtu-input"
                  type="number"
                  min="576"
                  max="9000"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">DNS 服务器</span>
                <input
                  v-model="draftWgDns"
                  data-testid="wg-dns-input"
                  placeholder="1.1.1.1, 8.8.8.8"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control sm:col-span-2">
                <span class="label-text text-xs font-semibold">保留字节 (可选，3 个 uint8)</span>
                <input
                  v-model="draftWgReserved"
                  data-testid="wg-reserved-input"
                  placeholder="0, 0, 0"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
            </div>
          </div>

          <!-- TUIC Specific Fields -->
          <div v-else-if="selectedNode.protocol.toLowerCase() === 'tuic'" class="space-y-2.5 pt-2 border-t border-base-300">
            <div class="font-bold text-primary uppercase tracking-wider text-[11px]">
              TUIC v5 连接与 QUIC 传输参数
            </div>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
              <label class="form-control">
                <span class="label-text text-xs font-semibold">UUID</span>
                <input
                  v-model="draftUuid"
                  data-testid="tuic-uuid-input"
                  placeholder="00000000-0000-4000-8000-000000000001"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">认证密码</span>
                <input
                  v-model="draftPassword"
                  data-testid="tuic-password-input"
                  placeholder="TUIC 认证密码"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">拥塞控制算法</span>
                <select
                  v-model="draftCongestionControl"
                  data-testid="tuic-cc-select"
                  class="select select-bordered select-xs font-mono mt-1"
                >
                  <option value="">（默认 / 未设置）</option>
                  <option value="bbr">bbr</option>
                  <option value="cubic">cubic</option>
                  <option value="new_reno">new_reno</option>
                </select>
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">UDP 转发模式</span>
                <select
                  v-model="draftUdpRelayMode"
                  data-testid="tuic-udp-mode-select"
                  class="select select-bordered select-xs font-mono mt-1"
                >
                  <option value="">（默认 / 未设置）</option>
                  <option value="native">native</option>
                  <option value="quic">quic</option>
                </select>
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">ALPN 协议协商</span>
                <input
                  v-model="draftAlpn"
                  data-testid="tuic-alpn-input"
                  placeholder="h3"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">SNI 域名</span>
                <input
                  v-model="draftSni"
                  data-testid="tuic-sni-input"
                  placeholder="tuic.example.com"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="flex items-center gap-2 cursor-pointer sm:col-span-2 pt-1">
                <input
                  v-model="draftDisableSni"
                  data-testid="tuic-disable-sni-checkbox"
                  type="checkbox"
                  class="checkbox checkbox-primary checkbox-xs"
                />
                <span class="label-text text-xs font-mono">禁用 SNI (disable_sni)</span>
              </label>
            </div>
          </div>

          <!-- Other Protocols (SS, VMess, VLESS, Trojan, Hysteria2) Plaintext Credential Fields -->
          <div v-else class="pt-2 border-t border-base-300 space-y-2.5">
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
              <label
                v-if="['vmess', 'vless'].includes(selectedNode.protocol.toLowerCase())"
                class="form-control"
              >
                <span class="label-text text-xs font-semibold">UUID</span>
                <input
                  v-model="draftUuid"
                  data-testid="node-uuid-input"
                  placeholder="UUID"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label
                v-if="['ss', 'shadowsocks', 'vmess'].includes(selectedNode.protocol.toLowerCase())"
                class="form-control"
              >
                <span class="label-text text-xs font-semibold">加密方式 (Cipher)</span>
                <input
                  v-model="draftMethod"
                  data-testid="node-method-input"
                  placeholder="aes-256-gcm, chacha20-ietf-poly1305..."
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control sm:col-span-2">
                <span class="label-text text-xs font-semibold">连接密码 / 密钥</span>
                <input
                  v-model="draftPassword"
                  data-testid="node-password-input"
                  placeholder="输入协议连接密码或密钥"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">SNI 域名</span>
                <input
                  v-model="draftSni"
                  data-testid="node-sni-input"
                  placeholder="sni.example.com"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">ALPN 协议协商</span>
                <input
                  v-model="draftAlpn"
                  data-testid="node-alpn-input"
                  placeholder="h2, http/1.1"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="flex items-center gap-2 cursor-pointer sm:col-span-2 pt-1">
                <input
                  v-model="draftDisableSni"
                  data-testid="node-disable-sni-checkbox"
                  type="checkbox"
                  class="checkbox checkbox-primary checkbox-xs"
                />
                <span class="label-text text-xs font-mono">禁用 SNI (disable_sni)</span>
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
          <button type="button" class="btn btn-ghost btn-sm" @click="drawerOpen = false">
            {{ t('common.close') }}
          </button>
        </div>
      </template>
    </DrawerCard>
  </section>
</template>
