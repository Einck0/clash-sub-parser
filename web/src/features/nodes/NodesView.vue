<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch, type VNodeRef } from 'vue'
import {
  ArrowPathIcon,
  CheckCircleIcon,
  ExclamationTriangleIcon,
  EyeIcon,
  LockClosedIcon,
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
  protocolSupportedTargets,
  renderSafeNodePreview,
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

// Editable non-secret connection draft fields inside the Node Detail Drawer
const draftDisplayName = ref('')
const draftServer = ref('')
const draftPort = ref<number>(0)
// WireGuard non-secret fields
const draftWgLocalAddress = ref('')
const draftWgPublicKey = ref('')
const draftWgMtu = ref<number | undefined>(undefined)
const draftWgDns = ref('')
const draftWgReserved = ref('')
// TUIC non-secret fields
const draftTuicUuid = ref('')
const draftTuicCongestionControl = ref('')
const draftTuicUdpRelayMode = ref('')
const draftTuicAlpn = ref('')
const draftTuicSni = ref('')
const draftTuicDisableSni = ref(false)
// Write-only secret rotation inputs (never pre-populated, empty = preserve existing)
const draftPrivateKeyInput = ref('')
const draftPreSharedKeyInput = ref('')
const draftPasswordInput = ref('')

function syncDraftFromNode(node: NormalizedNode) {
  connectionError.value = ''
  connectionSaved.value = false
  draftDisplayName.value = node.displayName
  draftServer.value = node.connection.server
  draftPort.value = node.connection.port
  draftWgLocalAddress.value = node.connection.localAddress.join(', ')
  draftWgPublicKey.value = node.connection.publicKey
  draftWgMtu.value = node.connection.mtu
  draftWgDns.value = node.connection.dns.join(', ')
  draftWgReserved.value = node.connection.reserved.join(', ')
  draftTuicUuid.value = node.connection.uuid
  draftTuicCongestionControl.value = node.connection.congestionControl
  draftTuicUdpRelayMode.value = node.connection.udpRelayMode
  draftTuicAlpn.value = node.connection.alpn.join(', ')
  draftTuicSni.value = node.connection.sni
  draftTuicDisableSni.value = node.connection.disableSni
  // Secret rotation inputs are always cleared on sync – write-only, never pre-populated
  draftPrivateKeyInput.value = ''
  draftPreSharedKeyInput.value = ''
  draftPasswordInput.value = ''
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
  const alpn = draftTuicAlpn.value
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)

  const res = await updateNodeConnection(selectedNode.value.logicalId, {
    displayName: draftDisplayName.value,
    server: draftServer.value,
    port: Number(draftPort.value),
    localAddress,
    publicKey: draftWgPublicKey.value,
    mtu: draftWgMtu.value ? Number(draftWgMtu.value) : undefined,
    dns,
    reserved,
    uuid: draftTuicUuid.value,
    congestionControl: draftTuicCongestionControl.value,
    udpRelayMode: draftTuicUdpRelayMode.value,
    alpn,
    sni: draftTuicSni.value,
    disableSni: draftTuicDisableSni.value,
    privateKeyInput: draftPrivateKeyInput.value,
    preSharedKeyInput: draftPreSharedKeyInput.value,
    passwordInput: draftPasswordInput.value,
  })

  if (!res.ok) {
    // Failure: preserve uncommitted draft fields — do NOT reset inputs
    connectionError.value = res.error || 'Invalid connection parameters'
    return
  }
  if (res.node) {
    syncDraftFromNode(res.node)
  } else {
    draftPrivateKeyInput.value = ''
    draftPreSharedKeyInput.value = ''
    draftPasswordInput.value = ''
  }
  connectionSaved.value = true
}

const previewNodeSnippet = computed(() => {
  if (!selectedNode.value) return ''
  return renderSafeNodePreview(selectedNode.value, previewTarget.value)
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
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-primary">{{ t('nodes.tag') }}</p>
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
        aria-label="Protocol filter"
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
                    Targets: {{ protocolSupportedTargets(node.protocol).join('/') }}
                  </span>
                  <template v-if="node.protocol.toLowerCase() === 'wireguard'">
                    <span class="badge badge-xs badge-info badge-outline">
                      IP: {{ node.connection.localAddress.join(', ') }}
                    </span>
                    <span v-if="node.connection.mtu" class="badge badge-xs badge-ghost">
                      MTU: {{ node.connection.mtu }}
                    </span>
                    <span class="badge badge-xs badge-ghost">
                      PrivKey: {{ node.connection.privateKeyMasked }}
                    </span>
                  </template>
                  <template v-else-if="node.protocol.toLowerCase() === 'tuic'">
                    <span v-if="node.connection.congestionControl" class="badge badge-xs badge-info badge-outline">
                      CC: {{ node.connection.congestionControl }}
                    </span>
                    <span v-if="node.connection.udpRelayMode" class="badge badge-xs badge-ghost">
                      UDP: {{ node.connection.udpRelayMode }}
                    </span>
                    <span class="badge badge-xs badge-ghost">
                      Pass: {{ node.connection.passwordMasked }}
                    </span>
                  </template>
                </div>

                <div class="flex flex-wrap items-center justify-between gap-2 border-t border-base-300 pt-3 text-xs min-w-0">
                  <div class="flex flex-wrap items-center gap-1.5 sm:gap-2 min-w-0">
                    <span class="mr-1 text-xs opacity-60 shrink-0">Capabilities</span>
                    <StatusBadge
                      :label="`Streaming: ${nodeCapabilityLabel(node, 'streaming').label}`"
                      :tone="nodeCapabilityLabel(node, 'streaming').tone"
                    />
                    <StatusBadge
                      :label="`AI: ${nodeCapabilityLabel(node, 'ai').label}`"
                      :tone="nodeCapabilityLabel(node, 'ai').tone"
                    />
                    <span
                      v-if="node.probeStale"
                      class="badge badge-warning badge-sm gap-1 font-mono text-[11px]"
                      title="Probe observations are stale (> freshness window)"
                    >
                      Stale Probe
                    </span>
                    <span
                      v-if="node.probeMissing"
                      class="badge badge-ghost badge-sm gap-1 font-mono text-[11px] opacity-75"
                      title="No probe observations recorded yet"
                    >
                      No Probe
                    </span>
                    <span
                      v-if="node.credentialMismatch"
                      class="badge badge-error badge-sm gap-1 font-mono text-[11px]"
                      title="Node credential version does not match observation version"
                    >
                      Version Mismatch
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
        <p v-if="!hasMore" class="text-xs opacity-60">All nodes loaded</p>
      </div>

      <!-- Empty State -->
      <EmptyState
        v-if="!items.length && !loading"
        :icon="EyeIcon"
        :title="t('nodes.emptyTitle')"
        :description="t('nodes.emptyDesc')"
      />
    </div>

    <!-- Node Detail, Edit & Safe Preview Drawer -->
    <DrawerCard
      v-model="drawerOpen"
      :title="selectedNode ? `${selectedNode.displayName} (${selectedNode.protocol.toUpperCase()})` : t('nodes.inspectNode')"
      :description="t('nodes.connectionProfile')"
    >
      <div v-if="selectedNode" data-testid="node-detail-drawer" class="space-y-4 text-xs">
        <!-- Security / Vault Redaction Banner -->
        <div class="p-3 rounded-xl bg-info/10 border border-info/30 text-info flex items-start gap-2.5">
          <LockClosedIcon class="w-4 h-4 shrink-0 mt-0.5" />
          <div class="space-y-0.5">
            <p class="font-semibold">{{ t('nodes.secretProtected') }}</p>
            <p class="opacity-80 font-mono text-[11px]">
              Logical ID: {{ selectedNode.logicalId }} · Credential Version: v{{ selectedNode.credentialVersion ?? 1 }}
            </p>
          </div>
        </div>

        <!-- Subscription Provenance & Reconcile Overwrite Notice -->
        <div
          data-testid="node-reconcile-overwrite-notice"
          class="p-3 rounded-xl bg-warning/10 border border-warning/30 text-warning-content space-y-1"
        >
          <p class="font-semibold flex items-center gap-1.5">
            <ArrowPathIcon class="w-4 h-4 shrink-0" />
            <span>Subscription Reconcile &amp; Identity Semantics</span>
          </p>
          <p class="opacity-85 leading-relaxed">
            Local credential/parameter edits apply to this logical ID with CAS version bumping. Subsequent upstream subscription Reconcile will overwrite local changes if the source payload for this logical ID updates. Identity-defining endpoint/transport changes (server, port, SNI, ALPN, disable_sni) are forbidden in-place; update the subscription source to reconcile as a new logical ID.
          </p>
          <p
            v-if="selectedNode.sources && selectedNode.sources.length > 0"
            data-testid="node-provenance-sources"
            class="font-mono text-[11px] opacity-80"
          >
            Sources: {{ selectedNode.sources.map((s) => s.subscription_id).join(', ') }}
          </p>
        </div>

        <!-- Unavailable Credentials Alert -->
        <div
          v-if="!selectedNode.connection.available"
          data-testid="node-connection-unavailable"
          class="p-3 rounded-xl bg-error/10 border border-error/30 text-error flex items-start gap-2.5"
        >
          <ExclamationTriangleIcon class="w-4 h-4 shrink-0 mt-0.5" />
          <div class="space-y-0.5">
            <p class="font-semibold">
              Connection details unavailable ({{ selectedNode.connection.unavailableReason || 'credential_unavailable' }})
            </p>
            <p class="opacity-80">
              Verified node credentials are missing or failed identity authentication. No fabricated parameters are displayed and editing is disabled.
            </p>
          </div>
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
              {{ target }}: Supported
            </span>
            <template
              v-if="['wireguard', 'tuic', 'vless', 'hysteria2'].includes(selectedNode.protocol.toLowerCase())"
            >
              <span class="badge badge-xs badge-warning badge-outline">
                surge / qx: Rejected (Unsupported Protocol)
              </span>
            </template>
          </div>
        </div>

        <!-- Connection Parameters Form (Edit & Inspect Non-Secret Fields) -->
        <form
          data-testid="node-connection-form"
          class="p-3.5 rounded-xl bg-base-200/70 border border-base-300 space-y-3"
          @submit.prevent="handleApplyConnectionUpdate"
        >
          <div class="grid grid-cols-1 sm:grid-cols-3 gap-2.5">
            <label class="form-control sm:col-span-1">
              <span class="label-text text-xs font-semibold">Display Name</span>
              <input
                v-model="draftDisplayName"
                data-testid="node-display-name-input"
                class="input input-bordered input-xs font-mono mt-1"
              />
            </label>
            <label class="form-control sm:col-span-1">
              <span class="label-text text-xs font-semibold">Server Host / IP</span>
              <input
                v-model="draftServer"
                data-testid="node-server-input"
                class="input input-bordered input-xs font-mono mt-1"
              />
            </label>
            <label class="form-control sm:col-span-1">
              <span class="label-text text-xs font-semibold">Port</span>
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
              WireGuard Endpoint & Peer Configuration
            </div>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
              <label class="form-control">
                <span class="label-text text-xs font-semibold">Local Address (CIDR)</span>
                <input
                  v-model="draftWgLocalAddress"
                  data-testid="wg-local-address-input"
                  placeholder="10.0.0.2/32, fd00::2/128"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">Peer Public Key</span>
                <input
                  v-model="draftWgPublicKey"
                  data-testid="wg-public-key-input"
                  placeholder="Base64 peer public key"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">MTU</span>
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
                <span class="label-text text-xs font-semibold">DNS Servers</span>
                <input
                  v-model="draftWgDns"
                  data-testid="wg-dns-input"
                  placeholder="1.1.1.1, 8.8.8.8"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control sm:col-span-2">
                <span class="label-text text-xs font-semibold">Reserved Bytes (Optional, 3 uint8)</span>
                <input
                  v-model="draftWgReserved"
                  data-testid="wg-reserved-input"
                  placeholder="0, 0, 0"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
            </div>

            <!-- Masked WireGuard Secrets & Write-Only Rotation Inputs (Never Exposed in DOM) -->
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 pt-1 font-mono">
              <div
                data-testid="wg-private-key-masked"
                class="p-2 rounded-lg bg-base-100 border border-base-300 flex items-center justify-between"
              >
                <span class="opacity-70">private_key:</span>
                <span class="badge badge-xs badge-neutral">
                  {{ selectedNode.connection.hasPrivateKey ? `${selectedNode.connection.privateKeyMasked} (Vault AEAD)` : 'unavailable' }}
                </span>
              </div>
              <div
                data-testid="wg-psk-masked"
                class="p-2 rounded-lg bg-base-100 border border-base-300 flex items-center justify-between"
              >
                <span class="opacity-70">pre_shared_key:</span>
                <span class="badge badge-xs badge-neutral">
                  {{ selectedNode.connection.hasPreSharedKey ? `${selectedNode.connection.preSharedKeyMasked} (Vault AEAD)` : 'unavailable' }}
                </span>
              </div>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">Rotate Private Key (Write-Only)</span>
                <input
                  v-model="draftPrivateKeyInput"
                  data-testid="wg-private-key-input"
                  type="password"
                  autocomplete="new-password"
                  placeholder="Leave empty to preserve current private_key"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">Rotate Pre-Shared Key (Write-Only)</span>
                <input
                  v-model="draftPreSharedKeyInput"
                  data-testid="wg-psk-input"
                  type="password"
                  autocomplete="new-password"
                  placeholder="Leave empty to preserve current pre_shared_key"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
            </div>
          </div>

          <!-- TUIC Specific Fields -->
          <div v-else-if="selectedNode.protocol.toLowerCase() === 'tuic'" class="space-y-2.5 pt-2 border-t border-base-300">
            <div class="font-bold text-primary uppercase tracking-wider text-[11px]">
              TUIC v5 Connection & QUIC Transport Parameters
            </div>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
              <label class="form-control sm:col-span-2">
                <span class="label-text text-xs font-semibold">UUID</span>
                <input
                  v-model="draftTuicUuid"
                  data-testid="tuic-uuid-input"
                  placeholder="00000000-0000-4000-8000-000000000001"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">Congestion Control</span>
                <select
                  v-model="draftTuicCongestionControl"
                  data-testid="tuic-cc-select"
                  class="select select-bordered select-xs font-mono mt-1"
                >
                  <option value="">(default / unset)</option>
                  <option value="bbr">bbr</option>
                  <option value="cubic">cubic</option>
                  <option value="new_reno">new_reno</option>
                </select>
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">UDP Relay Mode</span>
                <select
                  v-model="draftTuicUdpRelayMode"
                  data-testid="tuic-udp-mode-select"
                  class="select select-bordered select-xs font-mono mt-1"
                >
                  <option value="">(default / unset)</option>
                  <option value="native">native</option>
                  <option value="quic">quic</option>
                </select>
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">ALPN</span>
                <input
                  v-model="draftTuicAlpn"
                  data-testid="tuic-alpn-input"
                  placeholder="h3"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">SNI</span>
                <input
                  v-model="draftTuicSni"
                  data-testid="tuic-sni-input"
                  placeholder="tuic.example.com"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="flex items-center gap-2 cursor-pointer sm:col-span-2 pt-1">
                <input
                  v-model="draftTuicDisableSni"
                  data-testid="tuic-disable-sni-checkbox"
                  type="checkbox"
                  class="checkbox checkbox-primary checkbox-xs"
                />
                <span class="label-text text-xs font-mono">disable_sni</span>
              </label>
            </div>

            <!-- Masked TUIC Secret & Write-Only Rotation Input (Never Exposed in DOM) -->
            <div class="space-y-2 font-mono">
              <div
                data-testid="tuic-password-masked"
                class="p-2 rounded-lg bg-base-100 border border-base-300 flex items-center justify-between"
              >
                <span class="opacity-70">password:</span>
                <span class="badge badge-xs badge-neutral">
                  {{ selectedNode.connection.hasPassword ? `${selectedNode.connection.passwordMasked} (Vault AEAD)` : 'unavailable' }}
                </span>
              </div>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">Rotate Password (Write-Only)</span>
                <input
                  v-model="draftPasswordInput"
                  data-testid="tuic-password-input"
                  type="password"
                  autocomplete="new-password"
                  placeholder="Leave empty to preserve current password"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
            </div>
          </div>

          <!-- Other Protocols Protected Credential Summary -->
          <div v-else class="pt-2 border-t border-base-300 space-y-2 font-mono">
            <div class="p-2 rounded-lg bg-base-100 border border-base-300 flex items-center justify-between">
              <span class="opacity-70">credential secret:</span>
              <span class="badge badge-xs badge-neutral">
                {{ selectedNode.connection.hasPassword ? `${selectedNode.connection.passwordMasked} (Vault AEAD)` : 'unavailable' }}
              </span>
            </div>
            <label class="form-control">
              <span class="label-text text-xs font-semibold">Rotate Credential Secret (Write-Only)</span>
              <input
                v-model="draftPasswordInput"
                data-testid="node-password-input"
                type="password"
                autocomplete="new-password"
                placeholder="Leave empty to preserve current secret"
                class="input input-bordered input-xs font-mono mt-1"
              />
            </label>
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
              Connection parameters saved (v{{ selectedNode.credentialVersion ?? 1 }})
            </span>
            <span v-else />
            <button
              type="submit"
              data-testid="node-save-connection-btn"
              class="btn btn-primary btn-xs"
              :disabled="!selectedNode.connection.available || savingConnection"
            >
              Save &amp; Rotate
            </button>
          </div>
        </form>

        <!-- Redacted Node Config Snippet Preview (Mihomo YAML / sing-box JSON) -->
        <div class="space-y-2">
          <div class="flex items-center justify-between">
            <span class="font-bold text-xs">Redacted Node Target Preview</span>
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
