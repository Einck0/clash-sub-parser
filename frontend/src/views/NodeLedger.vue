<template>
  <section class="page nodes-page p-2 space-y-6">
    <!-- Top Header -->
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 pb-4 border-b border-border-subtle">
      <div>
        <p class="text-xs font-mono text-accent uppercase tracking-wider">Node Quality Control & Routing Ledger</p>
        <h2 class="text-xl font-bold text-text-main tracking-tight">节点管理与质检中心</h2>
        <p class="text-xs text-text-muted mt-1">
          查看所有订阅与手动节点，进行真实出站握手测速、流媒体与 AI 解锁全项质检，以及配置跳板代理链路。
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2.5 w-full sm:w-auto">
        <Button
          variant="primary"
          size="md"
          class="flex-1 sm:flex-initial"
          :disabled="probing || !rows.length"
          :loading="probing"
          :icon="Zap"
          @click="startProbeBatch(effectiveBatchTargets)"
        >
          {{ probing ? `质检中 (${probeProgress.done}/${probeProgress.total})…` : '综合质检' }}
        </Button>
        <Button
          v-if="probing"
          variant="danger"
          size="md"
          class="flex-1 sm:flex-initial"
          :icon="Square"
          @click="cancelProbeBatch"
        >
          停止探测
        </Button>
        <Button
          variant="secondary"
          size="md"
          class="flex-1 sm:flex-initial"
          :disabled="loading"
          :icon="RefreshCw"
          @click="reload"
        >
          刷新
        </Button>
      </div>
    </div>

    <!-- Alert / Error Message -->
    <div v-if="error" class="p-4 rounded-lg border border-status-danger/30 bg-status-danger/10 text-xs font-mono text-status-danger">
      {{ error }}
    </div>

    <!-- Periodic Probe Scheduler Banner -->
    <LedgerSchedulerBanner ref="schedulerBannerRef" />

    <!-- Live Probe Progress Banner -->
    <div v-if="probing" class="p-4 rounded-lg border border-accent/40 bg-surface-base space-y-2 shadow-xs">
      <div class="flex justify-between items-center text-xs font-mono">
        <span class="text-accent font-semibold">
          全协议深度质检进行中: <span class="tabular-nums">{{ probeProgress.done }}</span> / <span class="tabular-nums">{{ probeProgress.total }}</span> (<span class="tabular-nums">{{ probeProgressPercent }}%</span>)
        </span>
        <div class="flex items-center gap-3">
          <span class="text-status-success flex items-center gap-1">
            <span class="h-1.5 w-1.5 rounded-full bg-status-success"></span>
            正常: <span class="tabular-nums">{{ probeProgress.ok }}</span>
          </span>
          <span class="text-status-danger flex items-center gap-1">
            <span class="h-1.5 w-1.5 rounded-full bg-status-danger"></span>
            失败: <span class="tabular-nums">{{ probeProgress.fail }}</span>
          </span>
          <span v-if="includeSpeedtest" class="text-status-info flex items-center gap-1">
            <Gauge class="h-3 w-3" aria-hidden="true" />
            测速中
          </span>
        </div>
      </div>
      <div class="h-1.5 w-full overflow-hidden rounded-full bg-surface-active">
        <div
          class="h-full bg-accent duration-150"
          :style="{ width: `${probeProgressPercent}%` }"
        ></div>
      </div>
    </div>

    <!-- 4 Key Metrics Bar (Componentized) -->
    <LedgerMetricsBar
      :total="rows.length"
      :filtered-count="filteredRows.length"
      :healthy="healthyCount"
      :tested-count="testedCount"
      :avg-latency="avgLatency"
      :fast="highSpeedCount"
      :max-speed="maxSpeed"
      :chained="chainedCount"
      :sub-count="filterOptions.subscriptions.length"
      :proto-count="filterOptions.protocols.length"
      :active-filter="metricsActiveFilter"
      @filter-metric="handleMetricFilter"
    />

    <!-- Multi-dimensional Search & Filter Bar (Componentized) -->
    <LedgerSearchFilter
      v-model="filters"
      v-model:include-speed="includeSpeedtest"
      v-model:include-media="includeMediaCheck"
      :view-mode="viewMode"
      :subscriptions="filterOptions.subscriptions"
      :protocols="filterOptions.protocols"
      :countries="filterOptions.countries"
      :media-platforms="mediaPlatformList"
      :media-stats="mediaStats"
      :total-count="rows.length"
      :filtered-count="filteredRows.length"
      :selected-count="selectedNodeKeys.size"
      :untested-or-failed-count="untestedOrFailedCount"
      :probing="probing"
      @change-view="viewMode = $event"
      @probe-selected="probeSelectedNodes"
      @clear-selection="selectedNodeKeys.clear()"
      @probe-untested="probeUntestedOrFailed"
      @clear-probe-data="clearProbeData"
      @reset-filters="resetFilters"
    />

    <!-- Progressive Loading Indicator for Background Probe Pages -->
    <div
      v-if="probeLoadingMore"
      class="flex items-center gap-2 px-3 py-1.5 rounded-md bg-accent-subtle text-accent text-xs font-mono"
      role="status"
      aria-live="polite"
    >
      <RefreshCw class="h-3.5 w-3.5 animate-spin shrink-0" aria-hidden="true" />
      <span>正在按需分页同步后续节点质检数据…</span>
    </div>

    <!-- Selection indicator & quick select-all toolbar -->
    <div class="flex flex-wrap items-center justify-between text-xs font-mono text-text-muted px-1 gap-2">
      <div class="flex items-center gap-3">
        <label class="inline-flex items-center gap-2 min-h-[44px] cursor-pointer select-none py-1">
          <input
            type="checkbox"
            :checked="isAllFilteredSelected"
            class="rounded-sm border-border-strong bg-canvas text-accent focus:ring-0 cursor-pointer"
            @change="toggleSelectAllFiltered"
          />
          <span>全选当前筛选节点 (<span class="tabular-nums">{{ filteredRows.length }}</span>)</span>
        </label>
        <span v-if="selectedNodeKeys.size > 0" class="text-accent">
          已跨视口选中 <span class="tabular-nums font-semibold">{{ selectedNodeKeys.size }}</span> 个节点
        </span>
      </div>

      <div v-if="viewMode === 'grid'" class="hidden sm:flex items-center gap-2">
        <button
          class="px-2 py-1 rounded-md border border-border-subtle bg-surface-hover text-text-main disabled:opacity-40 cursor-pointer focus-ring"
          :disabled="gridPage <= 1"
          @click="gridPage--"
        >
          上一页
        </button>
        <span class="tabular-nums">第 {{ gridPage }} / {{ totalGridPages }} 页</span>
        <button
          class="px-2 py-1 rounded-md border border-border-subtle bg-surface-hover text-text-main disabled:opacity-40 cursor-pointer focus-ring"
          :disabled="gridPage >= totalGridPages"
          @click="gridPage++"
        >
          下一页
        </button>
      </div>
    </div>

    <!-- Empty State -->
    <div
      v-if="!filteredRows.length && !loading"
      class="p-12 rounded-lg border border-dashed border-border-subtle bg-surface-base text-center space-y-3"
    >
      <Search class="h-8 w-8 text-text-sub mx-auto" aria-hidden="true" />
      <p class="text-sm text-text-muted">没有找到符合当前筛选条件的节点</p>
      <Button
        variant="secondary"
        size="sm"
        @click="resetFilters"
      >
        清空所有筛选条件
      </Button>
    </div>

    <!-- Node Views (Responsive Split: Mobile Compact List for <640px, Virtual Table/Grid for >=640px) -->
    <template v-else>
      <!-- Mobile Compact List (< 640px) -->
      <div class="block sm:hidden" data-testid="node-ledger-mobile-container">
        <LedgerMobileList
          :items="filteredRows"
          :selected-keys="selectedNodeKeys"
          :probes="probes"
          :probing-single-key="probingSingleNodeKey"
          @toggle-select="toggleSelectNode"
          @inspect="inspectNode"
          @probe-single="handleProbeSingle"
        />
      </div>

      <!-- Desktop Views (>= 640px) -->
      <div class="hidden sm:block">
        <!-- Node Virtual Table View (High Performance Virtualized for 2000+ Nodes, 36px Density) -->
        <div v-if="viewMode === 'table'" class="space-y-1">
          <!-- Table Column Headers -->
          <div class="flex items-center justify-between px-4 py-2 border-b border-border-subtle bg-surface-hover/60 rounded-t-lg text-[11px] font-mono text-text-muted select-none uppercase tracking-wider">
            <div class="flex items-center gap-3 min-w-0 flex-1">
              <span class="w-4"></span>
              <span>节点名称 / 协议</span>
            </div>
            <div class="hidden md:flex items-center gap-2 flex-1 min-w-0">
              <span>服务器与端口</span>
            </div>
            <div class="hidden lg:flex items-center gap-2 flex-1 min-w-0">
              <span>跳板链路</span>
            </div>
            <div class="flex items-center justify-end gap-4 shrink-0 text-right">
              <span>状态 / 延迟</span>
              <span class="hidden sm:inline">测速</span>
              <span class="w-8">操作</span>
            </div>
          </div>

          <VirtualNodeTable
            :items="filteredRows"
            :estimate-size="36"
            :selected-keys="selectedNodeKeys"
            key-field="node_key"
            class="border-t-0 rounded-t-none"
          >
            <template #default="{ item, isSelected }">
              <div
                class="flex w-full h-[36px] items-center justify-between py-1 cursor-pointer select-none text-xs font-mono table-row-dense"
                @click="inspectNode(item)"
              >
                <!-- Checkbox & Country Code & Node Name & Protocol -->
                <div class="flex items-center gap-2 sm:gap-2.5 min-w-0 flex-1">
                  <input
                    type="checkbox"
                    :checked="isSelected"
                    class="rounded-sm border-border-strong bg-canvas text-accent focus:ring-0 cursor-pointer shrink-0"
                    @click.stop="toggleSelectNode(item.node_key || '')"
                  />
                  <span class="px-1.5 py-0.5 rounded text-[10px] font-mono font-medium bg-surface-active text-text-muted border border-border-subtle shrink-0 tabular-nums">
                    {{ resolveCountryCode(item) }}
                  </span>
                  <span
                    class="font-medium text-text-main hover:text-accent transition-colors truncate max-w-[140px] sm:max-w-xs"
                    :title="item.name"
                  >
                    {{ item.name }}
                  </span>
                  <StatusBadge type="info" :text="(item.type || 'RAW').toUpperCase()" class="shrink-0" />
                </div>

                <!-- Server & Port (Desktop) -->
                <div class="hidden md:flex items-center gap-2 text-xs font-mono text-text-muted flex-1 min-w-0">
                  <span class="truncate max-w-[180px] tabular-nums">{{ item.server }}</span>
                  <span class="text-text-sub tabular-nums">:{{ item.port }}</span>
                  <span v-if="item.subscription_name" class="text-[10px] text-text-sub truncate max-w-[100px]">
                    [{{ item.subscription_name }}]
                  </span>
                </div>

                <!-- Dialer Chain Badge (Large Desktop) -->
                <div class="hidden lg:flex items-center gap-2 flex-1 min-w-0">
                  <span
                    v-if="isDuplicateName(item)"
                    class="text-status-warning text-[10px] font-medium"
                    :title="'名称重复，暂不能安全编辑跳板'"
                  >
                    名称重复，暂不能安全编辑跳板
                  </span>
                  <span
                    v-else-if="item.dialer_proxy"
                    class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-mono border border-accent/30 bg-accent-subtle text-accent"
                  >
                    链: {{ item.dialer_proxy }}
                  </span>
                  <span
                    v-else
                    class="text-text-sub"
                  >
                    直连
                  </span>
                </div>

                <!-- Probe Metrics & Quick Actions -->
                <div class="flex items-center justify-end gap-3 font-mono text-xs shrink-0">
                  <span
                    :class="getProbePresentation(item, probes).status === 'ok' ? 'text-status-success font-semibold tabular-nums' : getProbePresentation(item, probes).status === 'fail' ? 'text-status-danger font-medium' : getProbePresentation(item, probes).status === 'timeout' ? 'text-status-warning font-medium' : 'text-text-sub'"
                  >
                    {{ getProbePresentation(item, probes).label }}
                  </span>

                  <span
                    v-if="getProbePresentation(item, probes).speedMbps"
                    class="text-status-info hidden sm:inline font-semibold tabular-nums"
                  >
                    {{ getProbePresentation(item, probes).speedMbps }}M
                  </span>

                  <IconButton
                    :icon="Zap"
                    label="单节点测速"
                    size="sm"
                    variant="ghost"
                    :loading="probingSingleNodeKey === item.node_key"
                    @click.stop="handleProbeSingle(item)"
                  />
                </div>
              </div>
            </template>
          </VirtualNodeTable>
        </div>

        <!-- Node Grid View (Bounded Paged Cards) -->
        <div v-else class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          <div
            v-for="item in pagedGridRows"
            :key="item.node_key"
            class="flex flex-col justify-between p-4 rounded-lg border bg-surface-base transition-colors cursor-pointer space-y-3"
            :class="[
              selectedNodeKeys.has(item.node_key || '')
                ? 'border-accent bg-accent-subtle ring-1 ring-accent/30'
                : 'border-border-subtle hover:border-border-strong hover:bg-surface-hover'
            ]"
            @click="inspectNode(item)"
          >
            <!-- Card Top -->
            <div class="flex items-start justify-between gap-2">
              <div class="flex items-center gap-2 min-w-0 flex-1">
                <input
                  type="checkbox"
                  :checked="selectedNodeKeys.has(item.node_key || '')"
                  class="rounded-sm border-border-strong bg-canvas text-accent focus:ring-0 cursor-pointer shrink-0"
                  @click.stop="toggleSelectNode(item.node_key || '')"
                />
                <span class="px-1.5 py-0.5 rounded text-[10px] font-mono font-medium bg-surface-active text-text-muted border border-border-subtle shrink-0 tabular-nums">
                  {{ resolveCountryCode(item) }}
                </span>
                <span class="text-sm font-semibold text-text-main truncate" :title="item.name">{{ item.name }}</span>
              </div>
              <StatusBadge type="info" :text="(item.type || 'RAW').toUpperCase()" />
            </div>

            <!-- Endpoint & Subscription -->
            <div class="text-xs font-mono text-text-muted truncate flex justify-between tabular-nums">
              <span>{{ item.server }}:{{ item.port }}</span>
              <span v-if="item.subscription_name" class="text-text-sub truncate max-w-[120px]">[{{ item.subscription_name }}]</span>
            </div>

            <!-- Dialer Chain if set -->
            <div v-if="item.dialer_proxy" class="text-xs font-mono text-accent flex items-center gap-1">
              <span>跳板: {{ item.dialer_proxy }}</span>
            </div>

            <!-- Probe Metrics Strip -->
            <div class="flex items-center justify-between pt-2 border-t border-border-subtle text-xs font-mono">
              <span :class="getProbePresentation(item, probes).status === 'ok' ? 'text-status-success font-semibold tabular-nums' : getProbePresentation(item, probes).status === 'fail' ? 'text-status-danger' : getProbePresentation(item, probes).status === 'timeout' ? 'text-status-warning' : 'text-text-sub'">
                {{ getProbePresentation(item, probes).label }}
              </span>

              <span v-if="getProbePresentation(item, probes).speedMbps" class="text-status-info font-semibold tabular-nums">
                {{ getProbePresentation(item, probes).speedMbps }} Mbps
              </span>
            </div>

            <!-- Footer Actions -->
            <div class="flex gap-2 pt-2 border-t border-border-subtle">
              <Button
                variant="secondary"
                size="sm"
                class="flex-1"
                @click.stop="inspectNode(item)"
              >
                详情
              </Button>
              <Button
                variant="primary"
                size="sm"
                class="flex-1"
                :disabled="probingSingleNodeKey === item.node_key"
                :loading="probingSingleNodeKey === item.node_key"
                :icon="Zap"
                @click.stop="handleProbeSingle(item)"
              >
                测速
              </Button>
            </div>
          </div>
        </div>
      </div>
    </template>

    <!-- Node Inspector & Dialer Chain Drawer (Componentized) -->
    <LedgerDrawer
      :open="drawerOpen"
      :node="selectedNode"
      :probe="selectedNode ? getProbeForNode(probes, selectedNode) : null"
      :detail-probe="drawerDetailProbe"
      :detail-status="drawerDetailStatus"
      :node-candidates="rows"
      :group-candidates="nodeGroups"
      :probing-single="selectedNode ? probingSingleNodeKey === selectedNode.node_key : false"
      :saving-chain="savingChain"
      :chain-edit-eligibility="selectedNode ? checkNodeChainActionEligibility(selectedNode, rows) : { canEdit: false }"
      :clearing-chain="clearingChain"
      @close="drawerOpen = false"
      @retry-detail="() => loadNodeDetail(selectedNode)"
      @probe-single="handleProbeSingle"
      @save-chain="handleSaveChain"
      @clear-chain="handleClearChain"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import {
  Gauge,
  RefreshCw,
  Search,
  Square,
  Zap,
} from 'lucide-vue-next'
import {
  clearProbeResults,
  createProxyChain,
  deleteProxyChain,
  getApiErrorMessage,
  getNodeGroups,
  getNodeLedger,
  getProbeResultDetail,
  getProbeResults,
  getProbeSettings,
  getProbeStatus,
  getProxyChains,
  probeNode,
} from '../api'
import LedgerDrawer from '../components/ledger/LedgerDrawer.vue'
import LedgerMetricsBar from '../components/ledger/LedgerMetricsBar.vue'
import LedgerMobileList from '../components/ledger/LedgerMobileList.vue'
import LedgerSchedulerBanner from '../components/ledger/LedgerSchedulerBanner.vue'
import LedgerSearchFilter from '../components/ledger/LedgerSearchFilter.vue'
import Button from '../components/ui/Button.vue'
import IconButton from '../components/ui/IconButton.vue'
import StatusBadge from '../components/ui/StatusBadge.vue'
import VirtualNodeTable from '../components/ui/VirtualNodeTable.vue'
import { useAppStore } from '../stores/app'
import {
  applyMetricShortcut,
  buildEffectiveBatchTargets,
  clearNodeDialerProxy,
  computeFilterOptions,
  createDefaultFacetFilterState,
  filterAndSortNodes,
  getProbeForNode,
  getProbePresentation,
  checkNodeChainActionEligibility,
  isMediaFullUnlocked,
  MEDIA_PLATFORMS,
  mergeNodeLedgerProbePages,
  normalizeImmediateProbeResponse,
  normalizeKeyedProbeResponse,
  normalizeNodeLedgerItems,
  normalizeNodeLedgerMap,
  replaceNodeDialerProxy,
  resolveNodeCountryCode,
  runSlidingWorkerPool,
  type FacetFilterState,
  type FilterState,
  type LedgerNodeItem,
  type ProbeRecord,
} from './nodeLedgerDomain'

const store = useAppStore()

// State
const loading = ref(false)
const error = ref('')
const schedulerBannerRef = ref<any>(null)
const probing = ref(false)
const probingSingleNodeKey = ref<string | null>(null)
const savingChain = ref(false)
const clearingChain = ref(false)

const rows = ref<LedgerNodeItem[]>([])
const bindings = ref<any[]>([])
const nodeGroups = ref<any[]>([])
const probes = ref<Record<string, ProbeRecord>>({})

const viewMode = ref<'table' | 'grid'>('table')
const gridPage = ref(1)
const gridPageSize = 48

const drawerOpen = ref(false)
const selectedNode = ref<LedgerNodeItem | null>(null)
const drawerDetailProbe = ref<ProbeRecord | null>(null)
const drawerDetailStatus = ref<'idle' | 'loading' | 'ready' | 'unavailable'>('idle')
let detailAbortController: AbortController | null = null
let pagedProbeAbortController: AbortController | null = null
const probeLoadingMore = ref(false)

const selectedNodeKeys = reactive<Set<string>>(new Set())

const includeSpeedtest = ref(false)
const includeMediaCheck = ref(true)

const probeProgress = reactive({ done: 0, total: 0, ok: 0, fail: 0 })
let probeAbortController: AbortController | null = null

const filters = ref<FacetFilterState>(createDefaultFacetFilterState())

const metricsActiveFilter = computed(() => ({
  status:
    filters.value.statuses.length === 0
      ? 'all'
      : filters.value.statuses.length === 1 && filters.value.statuses[0] === 'ok'
        ? 'ok'
        : filters.value.statuses.join(','),
  minSpeed: filters.value.minSpeed,
  chain: filters.value.chain,
}))

const mediaPlatformList = MEDIA_PLATFORMS

// Computed derived options & stats
const filterOptions = computed(() => computeFilterOptions(rows.value, probes.value))

const mediaStats = computed(() => {
  const stats: Record<string, number> = {}
  for (const p of mediaPlatformList) {
    stats[p.key] = 0
  }
  for (const row of rows.value) {
    const probe = getProbeForNode(probes.value, row)
    if (probe?.media) {
      for (const p of mediaPlatformList) {
        const item = probe.media[p.key]
        if (isMediaFullUnlocked(item)) {
          stats[p.key]++
        }
      }
    }
  }
  return stats
})

const filteredRows = computed(() => filterAndSortNodes(rows.value, probes.value, filters.value))

const effectiveBatchTargets = computed(() =>
  buildEffectiveBatchTargets(filteredRows.value, selectedNodeKeys, rows.value)
)

const totalGridPages = computed(() => Math.max(1, Math.ceil(filteredRows.value.length / gridPageSize)))
const pagedGridRows = computed(() => {
  const start = (gridPage.value - 1) * gridPageSize
  return filteredRows.value.slice(start, start + gridPageSize)
})

const isAllFilteredSelected = computed(() => {
  if (!filteredRows.value.length) return false
  return filteredRows.value.every((r) => selectedNodeKeys.has(r.node_key || ''))
})

const healthyCount = computed(() => {
  return rows.value.filter((r) => getProbeForNode(probes.value, r)?.status === 'ok').length
})

const testedCount = computed(() => {
  return rows.value.filter((r) => {
    const p = getProbeForNode(probes.value, r)
    return p && p.status && p.status !== 'untested'
  }).length
})

const highSpeedCount = computed(() => {
  return rows.value.filter((r) => (getProbeForNode(probes.value, r)?.speed_mbps || 0) >= 10).length
})

const chainedCount = computed(() => {
  return rows.value.filter((r) => r.dialer_proxy).length
})

const untestedOrFailedCount = computed(() => {
  return rows.value.filter((r) => {
    const p = getProbeForNode(probes.value, r)
    return !p || p.status !== 'ok'
  }).length
})

const avgLatency = computed(() => {
  const list = rows.value
    .map((r) => getProbeForNode(probes.value, r)?.latency_ms)
    .filter((ms): ms is number => typeof ms === 'number' && ms > 0)
  if (!list.length) return null
  return Math.round(list.reduce((a, b) => a + b, 0) / list.length)
})

const maxSpeed = computed(() => {
  const list = rows.value
    .map((r) => getProbeForNode(probes.value, r)?.speed_mbps)
    .filter((s): s is number => typeof s === 'number' && s > 0)
  if (!list.length) return null
  return Math.max(...list)
})

const probeProgressPercent = computed(() => {
  if (!probeProgress.total) return 0
  return Math.min(100, Math.round((probeProgress.done / probeProgress.total) * 100))
})

watch(
  () => filters.value,
  () => {
    gridPage.value = 1
  },
  { deep: true }
)

function isDuplicateName(node: LedgerNodeItem): boolean {
  return rows.value.filter(candidate => candidate.name === node.name).length > 1
}

function resolveCountryCode(node: LedgerNodeItem): string {
  const p = getProbeForNode(probes.value, node)
  const code = resolveNodeCountryCode(node, p)
  return code === 'OTHER' ? '--' : code
}

function toggleSelectNode(nodeKey: string) {
  if (!nodeKey) return
  if (selectedNodeKeys.has(nodeKey)) {
    selectedNodeKeys.delete(nodeKey)
  } else {
    selectedNodeKeys.add(nodeKey)
  }
}

function toggleSelectAllFiltered() {
  if (isAllFilteredSelected.value) {
    for (const r of filteredRows.value) {
      selectedNodeKeys.delete(r.node_key || '')
    }
  } else {
    for (const r of filteredRows.value) {
      selectedNodeKeys.add(r.node_key || '')
    }
  }
}

function handleMetricFilter(type: 'all' | 'healthy' | 'fast' | 'chained') {
  filters.value = applyMetricShortcut(filters.value, type)
}

function resetFilters() {
  filters.value = createDefaultFacetFilterState()
  selectedNodeKeys.clear()
}

async function loadNodeDetail(node: LedgerNodeItem | null) {
  if (detailAbortController) {
    detailAbortController.abort()
    detailAbortController = null
  }

  if (!node) {
    drawerDetailProbe.value = null
    drawerDetailStatus.value = 'idle'
    return
  }

  if (!node.node_key) {
    drawerDetailProbe.value = null
    drawerDetailStatus.value = 'unavailable'
    return
  }

  const requestedKey = node.node_key
  drawerDetailStatus.value = 'loading'
  drawerDetailProbe.value = null
  detailAbortController = new AbortController()

  try {
    const res = await getProbeResultDetail(requestedKey, { signal: detailAbortController.signal })
    const detail = normalizeKeyedProbeResponse(res?.data, requestedKey)
    if (selectedNode.value?.node_key === requestedKey) {
      if (detail) {
        drawerDetailProbe.value = detail
        drawerDetailStatus.value = 'ready'
        probes.value = {
          ...probes.value,
          [requestedKey]: detail,
        }
      } else {
        drawerDetailStatus.value = 'unavailable'
      }
    }
  } catch (err: any) {
    if (err?.name === 'AbortError' || detailAbortController?.signal?.aborted) {
      return
    }
    if (selectedNode.value?.node_key === requestedKey) {
      drawerDetailStatus.value = 'unavailable'
    }
  } finally {
    if (selectedNode.value?.node_key === requestedKey && drawerDetailStatus.value === 'loading') {
      drawerDetailStatus.value = 'unavailable'
    }
  }
}

function inspectNode(node: LedgerNodeItem) {
  selectedNode.value = node
  drawerOpen.value = true
  loadNodeDetail(node)
}

watch(drawerOpen, (isOpen) => {
  if (!isOpen) {
    if (detailAbortController) {
      detailAbortController.abort()
      detailAbortController = null
    }
    drawerDetailStatus.value = 'idle'
    drawerDetailProbe.value = null
  }
})

// Progressive background loader for subsequent probe summary pages
async function fetchRemainingProbePages(initialCursor: string | null) {
  if (!initialCursor) return
  if (pagedProbeAbortController) {
    pagedProbeAbortController.abort()
  }
  pagedProbeAbortController = new AbortController()
  probeLoadingMore.value = true
  let cursor: string | null = initialCursor

  try {
    while (cursor && !pagedProbeAbortController.signal.aborted) {
      const res = await getProbeResults({ cursor, limit: 100 })
      if (pagedProbeAbortController.signal.aborted) break
      const pageData = res?.data
      if (!pageData) break
      probes.value = mergeNodeLedgerProbePages(probes.value, pageData)
      store.setNodeSummary({
        status: 'ready',
        total: rows.value.length,
        probed: rows.value.filter((r) => getProbeForNode(probes.value, r)?.status === 'ok').length,
      })
      if (pageData.has_more && pageData.next_cursor) {
        cursor = pageData.next_cursor
      } else {
        cursor = null
      }
    }
  } catch {
    // Aborted or fetch error
  } finally {
    probeLoadingMore.value = false
  }
}

// Data Load
async function reload() {
  loading.value = true
  error.value = ''
  if (pagedProbeAbortController) {
    pagedProbeAbortController.abort()
    pagedProbeAbortController = null
  }
  try {
    const [ledgerRes, chainsRes, groupsRes, probeRes] = await Promise.all([
      getNodeLedger(),
      getProxyChains(),
      getNodeGroups(),
      getProbeResults({ limit: 100 }).catch(() => ({ data: {} })),
    ])
    rows.value = normalizeNodeLedgerItems(ledgerRes?.data)
    bindings.value = Array.isArray(chainsRes?.data) ? chainsRes.data : []
    nodeGroups.value = Array.isArray(groupsRes?.data) ? groupsRes.data : []

    const rawProbeData = probeRes?.data?.results || probeRes?.data
    probes.value = normalizeNodeLedgerMap(rawProbeData)

    store.setNodeSummary({
      status: 'ready',
      total: rows.value.length,
      probed: rows.value.filter((r) => getProbeForNode(probes.value, r)?.status === 'ok').length,
    })

    if (probeRes?.data?.has_more && probeRes?.data?.next_cursor) {
      fetchRemainingProbePages(probeRes.data.next_cursor)
    }
    schedulerBannerRef.value?.refresh()
  } catch (err: any) {
    error.value = getApiErrorMessage(err, '加载节点与跳板数据失败')
  } finally {
    loading.value = false
  }
}

// Single node probe
async function handleProbeSingle(node: LedgerNodeItem) {
  if (!node || probingSingleNodeKey.value) return
  probingSingleNodeKey.value = node.node_key || null
  error.value = ''
  try {
    const { data } = await probeNode({
      node,
      include_speed: includeSpeedtest.value,
      include_media: includeMediaCheck.value,
      use_cache: false,
    })
    const item = normalizeImmediateProbeResponse(data, node.node_key, rows.value)
    if (item) {
      probes.value = { ...probes.value, [item.node_key as string]: item }
      store.toast(`节点「${node.name}」探测完成`, 'success')
    } else {
      store.toast('节点探测返回数据无效，未更新台账', 'error')
    }
  } catch (err) {
    store.toast(getApiErrorMessage(err, '节点探测失败'), 'error')
  } finally {
    probingSingleNodeKey.value = null
  }
}

// Batch Probe
let cancelToastShown = false

async function startProbeBatch(targetNodes: LedgerNodeItem[]) {
  if (!targetNodes || !targetNodes.length || probing.value) return
  probing.value = true
  error.value = ''
  cancelToastShown = false
  probeProgress.done = 0
  probeProgress.total = targetNodes.length
  probeProgress.ok = 0
  probeProgress.fail = 0

  probeAbortController = new AbortController()

  // 1. Read single snapshot of probe_concurrency from settings
  let workerLimit = 10
  try {
    const settingsRes = await getProbeSettings()
    const concurrency = Number(settingsRes?.data?.probe_concurrency)
    if (concurrency && !isNaN(concurrency)) {
      workerLimit = Math.max(1, Math.min(20, concurrency))
    }
  } catch (_) {
    workerLimit = 10
  }

  try {
    const { stopped } = await runSlidingWorkerPool<LedgerNodeItem, any>(targetNodes, {
      concurrency: workerLimit,
      signal: probeAbortController.signal,
      workerFn: async (node, signal) => {
        const res = await probeNode(
          {
            node,
            include_speed: includeSpeedtest.value,
            include_media: includeMediaCheck.value,
            use_cache: false,
          },
          { signal }
        )
        return res?.data || {}
      },
      onItemDone: (res, node, prog) => {
        probeProgress.done = prog.done
        probeProgress.ok = prog.ok
        probeProgress.fail = prog.fail

        const item = normalizeKeyedProbeResponse(res, node.node_key)
        if (item) {
          probes.value = { ...probes.value, [item.node_key as string]: item }
        }
      },
    })

    if (stopped) {
      if (!cancelToastShown) {
        cancelToastShown = true
        store.toast('已停止后续探测', 'info')
      }
    } else {
      store.toast(`质检完成：${probeProgress.ok} 正常，${probeProgress.fail} 异常`, 'success')
    }
  } catch (err: any) {
    if (probeAbortController?.signal.aborted) {
      if (!cancelToastShown) {
        cancelToastShown = true
        store.toast('已停止后续探测', 'info')
      }
    } else {
      error.value = getApiErrorMessage(err, '综合质检执行失败')
    }
  } finally {
    probing.value = false
    probeAbortController = null
  }
}

function cancelProbeBatch() {
  if (probeAbortController && !probeAbortController.signal.aborted) {
    probeAbortController.abort()
  }
  probing.value = false
  if (!cancelToastShown) {
    cancelToastShown = true
    store.toast('已停止后续探测', 'info')
  }
}

function probeSelectedNodes() {
  const targets = rows.value.filter((r) => selectedNodeKeys.has(r.node_key || ''))
  startProbeBatch(targets)
}

function probeUntestedOrFailed() {
  const targets = rows.value.filter((r) => {
    const p = getProbeForNode(probes.value, r)
    return !p || p.status !== 'ok'
  })
  startProbeBatch(targets)
}

async function clearProbeData() {
  const ok = await store.confirm({
    title: '清空质检数据',
    message: '确定要清空所有已持久化的探测、测速与流媒体解锁记录吗？',
    confirmText: '清空',
    danger: true,
  })
  if (!ok) return

  try {
    await clearProbeResults()
    probes.value = {}
    store.toast('已清空全部质检记录', 'success')
  } catch (err) {
    store.toast(getApiErrorMessage(err, '清空失败'), 'error')
  }
}

// Chain management
async function handleSaveChain(payload: { nodeName: string; dialerType: string; dialerRef: string }) {
  const target = rows.value.find(row => row.name === payload.nodeName)
  const eligibility = checkNodeChainActionEligibility(target, rows.value)
  if (!eligibility.canEdit) {
    store.toast(eligibility.reason || '当前节点暂不可编辑跳板', 'error')
    return
  }
  savingChain.value = true
  error.value = ''
  try {
    await replaceNodeDialerProxy({
      nodeName: payload.nodeName,
      dialerType: payload.dialerType,
      dialerRef: payload.dialerRef,
      existingBindings: bindings.value,
      deleteBindingFn: deleteProxyChain,
      createBindingFn: (data) => createProxyChain({ ...data, enabled: true }),
      note: 'configured via node workbench',
    })
    store.toast('跳板链路绑定成功', 'success')
    await reload()
    if (selectedNode.value) {
      selectedNode.value = rows.value.find((r) => r.name === payload.nodeName) || null
    }
  } catch (err) {
    error.value = getApiErrorMessage(err, '保存跳板失败')
  } finally {
    savingChain.value = false
  }
}

async function handleClearChain(node: LedgerNodeItem) {
  const ok = await store.confirm({
    title: '清除节点跳板',
    message: `确定要清除节点「${node.name}」的专属跳板绑定吗？`,
    confirmText: '清除',
    danger: true,
  })
  if (!ok) return

  clearingChain.value = true
  error.value = ''
  try {
    await clearNodeDialerProxy(node.name, bindings.value, deleteProxyChain)
    store.toast('已清除节点跳板', 'success')
    await reload()
    if (selectedNode.value) {
      selectedNode.value = rows.value.find((r) => r.name === node.name) || null
    }
  } catch (err) {
    error.value = getApiErrorMessage(err, '清除跳板失败')
  } finally {
    clearingChain.value = false
  }
}

onMounted(() => {
  reload()
})

onUnmounted(() => {
  if (pagedProbeAbortController) {
    pagedProbeAbortController.abort()
  }
  if (detailAbortController) {
    detailAbortController.abort()
  }
  if (probeAbortController) {
    probeAbortController.abort()
  }
})
</script>
