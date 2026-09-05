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
      :active-filter="filters"
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
      :selected-count="selectedNodeNames.size"
      :untested-or-failed-count="untestedOrFailedCount"
      :probing="probing"
      @change-view="viewMode = $event"
      @probe-selected="probeSelectedNodes"
      @clear-selection="selectedNodeNames.clear()"
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
        <span v-if="selectedNodeNames.size > 0" class="text-accent">
          已跨视口选中 <span class="tabular-nums font-semibold">{{ selectedNodeNames.size }}</span> 个节点
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
          :selected-names="selectedNodeNames"
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
            :selected-keys="selectedNodeNames"
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
                    @click.stop="toggleSelectNode(item.name)"
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
                    v-if="item.dialer_proxy"
                    class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-mono border"
                    :class="item.chain_source === 'node' ? 'border-accent/30 bg-accent-subtle text-accent' : 'border-purple-500/30 bg-purple-500/10 text-purple-400'"
                  >
                    链: {{ item.dialer_proxy }}
                  </span>
                </div>

                <!-- Probe Metrics & Quick Actions -->
                <div class="flex items-center justify-end gap-3 font-mono text-xs shrink-0">
                  <span
                    v-if="getProbe(item.name)?.status === 'ok'"
                    class="text-status-success font-semibold tabular-nums"
                  >
                    {{ getProbe(item.name)?.latency_ms }}ms
                  </span>
                  <span
                    v-else-if="getProbe(item.name)?.status === 'fail'"
                    class="text-status-danger font-medium"
                  >
                    失败
                  </span>
                  <span
                    v-else-if="getProbe(item.name)?.status === 'timeout'"
                    class="text-status-warning font-medium"
                  >
                    超时
                  </span>
                  <span v-else class="text-text-sub">
                    未测
                  </span>

                  <span
                    v-if="getProbe(item.name)?.speed_mbps"
                    class="text-status-info hidden sm:inline font-semibold tabular-nums"
                  >
                    {{ getProbe(item.name)?.speed_mbps }}M
                  </span>

                  <IconButton
                    :icon="Zap"
                    label="单节点测速"
                    size="sm"
                    variant="ghost"
                    :loading="probingSingleNodeKey === item.name"
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
            :key="item.name"
            class="flex flex-col justify-between p-4 rounded-lg border bg-surface-base transition-colors cursor-pointer space-y-3"
            :class="[
              selectedNodeNames.has(item.name)
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
                  :checked="selectedNodeNames.has(item.name)"
                  class="rounded-sm border-border-strong bg-canvas text-accent focus:ring-0 cursor-pointer shrink-0"
                  @click.stop="toggleSelectNode(item.name)"
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
              <span v-if="getProbe(item.name)?.status === 'ok'" class="text-status-success font-semibold tabular-nums">
                {{ getProbe(item.name)?.latency_ms }}ms
              </span>
              <span v-else-if="getProbe(item.name)?.status === 'fail'" class="text-status-danger">失败</span>
              <span v-else-if="getProbe(item.name)?.status === 'timeout'" class="text-status-warning">超时</span>
              <span v-else class="text-text-sub">未测</span>

              <span v-if="getProbe(item.name)?.speed_mbps" class="text-status-info font-semibold tabular-nums">
                {{ getProbe(item.name)?.speed_mbps }} Mbps
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
                :disabled="probingSingleNodeKey === item.name"
                :loading="probingSingleNodeKey === item.name"
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
      :probing-single="selectedNode ? probingSingleNodeKey === selectedNode.name : false"
      :saving-chain="savingChain"
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
  getProxyChains,
  probeNode,
  probeNodesFull,
} from '../api'
import LedgerDrawer from '../components/ledger/LedgerDrawer.vue'
import LedgerMetricsBar from '../components/ledger/LedgerMetricsBar.vue'
import LedgerMobileList from '../components/ledger/LedgerMobileList.vue'
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
  filterAndSortNodes,
  getProbeForNode,
  isMediaFullUnlocked,
  MEDIA_PLATFORMS,
  mergeNodeLedgerProbePages,
  normalizeNodeLedgerMap,
  replaceNodeDialerProxy,
  resolveNodeCountryCode,
  type FilterState,
  type LedgerNodeItem,
  type ProbeRecord,
} from './nodeLedgerDomain'

const store = useAppStore()

// State
const loading = ref(false)
const error = ref('')
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

const selectedNodeNames = reactive<Set<string>>(new Set())

const includeSpeedtest = ref(false)
const includeMediaCheck = ref(true)

const probeProgress = reactive({ done: 0, total: 0, ok: 0, fail: 0 })
let probeAbortController: AbortController | null = null

const filters = ref<FilterState>({
  keyword: '',
  subscription: '',
  protocol: '',
  status: 'all',
  country: '',
  chain: 'all',
  minSpeed: 0,
  mediaPlatforms: [],
  sortBy: 'default',
})

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
  buildEffectiveBatchTargets(filteredRows.value, selectedNodeNames, rows.value)
)

const totalGridPages = computed(() => Math.max(1, Math.ceil(filteredRows.value.length / gridPageSize)))
const pagedGridRows = computed(() => {
  const start = (gridPage.value - 1) * gridPageSize
  return filteredRows.value.slice(start, start + gridPageSize)
})

const isAllFilteredSelected = computed(() => {
  if (!filteredRows.value.length) return false
  return filteredRows.value.every((r) => selectedNodeNames.has(r.name))
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

function getProbe(name: string): ProbeRecord | undefined {
  return probes.value[name]
}

function resolveCountryCode(node: LedgerNodeItem): string {
  const p = getProbeForNode(probes.value, node)
  const code = resolveNodeCountryCode(node, p)
  return code === 'OTHER' ? '--' : code
}

function toggleSelectNode(name: string) {
  if (selectedNodeNames.has(name)) {
    selectedNodeNames.delete(name)
  } else {
    selectedNodeNames.add(name)
  }
}

function toggleSelectAllFiltered() {
  if (isAllFilteredSelected.value) {
    for (const r of filteredRows.value) {
      selectedNodeNames.delete(r.name)
    }
  } else {
    for (const r of filteredRows.value) {
      selectedNodeNames.add(r.name)
    }
  }
}

function handleMetricFilter(type: 'all' | 'healthy' | 'fast' | 'chained') {
  filters.value = applyMetricShortcut(filters.value, type)
}

function resetFilters() {
  filters.value = {
    keyword: '',
    subscription: '',
    protocol: '',
    status: 'all',
    country: '',
    chain: 'all',
    minSpeed: 0,
    mediaPlatforms: [],
    sortBy: 'default',
  }
  selectedNodeNames.clear()
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
    if (selectedNode.value?.node_key === requestedKey) {
      if (res?.data) {
        drawerDetailProbe.value = res.data
        drawerDetailStatus.value = 'ready'
        probes.value = {
          ...probes.value,
          [requestedKey]: res.data,
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
    rows.value = Array.isArray(ledgerRes?.data) ? ledgerRes.data : []
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
  } catch (err: any) {
    error.value = getApiErrorMessage(err, '加载节点与跳板数据失败')
  } finally {
    loading.value = false
  }
}

// Single node probe
async function handleProbeSingle(node: LedgerNodeItem) {
  if (!node || probingSingleNodeKey.value) return
  probingSingleNodeKey.value = node.name
  error.value = ''
  try {
    const { data } = await probeNode({
      node,
      include_speed: includeSpeedtest.value,
      include_media: includeMediaCheck.value,
      use_cache: false,
    })
    if (data) {
      if (node.name) probes.value[node.name] = data
      if (data.node_key) probes.value[data.node_key] = data
      probes.value = { ...probes.value }
    }
    store.toast(`节点「${node.name}」探测完成`, 'success')
  } catch (err) {
    store.toast(getApiErrorMessage(err, '节点探测失败'), 'error')
  } finally {
    probingSingleNodeKey.value = null
  }
}

// Batch Probe
async function startProbeBatch(targetNodes: LedgerNodeItem[]) {
  if (!targetNodes || !targetNodes.length || probing.value) return
  probing.value = true
  error.value = ''
  probeProgress.done = 0
  probeProgress.total = targetNodes.length
  probeProgress.ok = 0
  probeProgress.fail = 0

  probeAbortController = new AbortController()

  const CHUNK_SIZE = 10
  const chunks: LedgerNodeItem[][] = []
  for (let i = 0; i < targetNodes.length; i += CHUNK_SIZE) {
    chunks.push(targetNodes.slice(i, i + CHUNK_SIZE))
  }

  try {
    for (const chunk of chunks) {
      if (probeAbortController.signal.aborted || !probing.value) break

      const res = await probeNodesFull({
        nodes: chunk,
        include_speed: includeSpeedtest.value,
        include_media: includeMediaCheck.value,
        use_cache: false,
      })

      const list = res?.data?.results || []
      for (const item of list) {
        if (item.status === 'ok') probeProgress.ok++
        else probeProgress.fail++
        probeProgress.done++

        if (item.name) probes.value[item.name] = item
        if (item.node_key) probes.value[item.node_key] = item
      }
      probes.value = { ...probes.value }
    }
    store.toast(`质检完成：${probeProgress.ok} 正常，${probeProgress.fail} 异常`, 'success')
  } catch (err: any) {
    error.value = getApiErrorMessage(err, '综合质检执行失败')
  } finally {
    probing.value = false
    probeAbortController = null
  }
}

function cancelProbeBatch() {
  if (probeAbortController) {
    probeAbortController.abort()
  }
  probing.value = false
  store.toast('已停止后续探测任务', 'info')
}

function probeSelectedNodes() {
  const targets = rows.value.filter((r) => selectedNodeNames.has(r.name))
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
