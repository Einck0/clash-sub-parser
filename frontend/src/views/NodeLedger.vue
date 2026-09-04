<template>
  <section class="page nodes-page p-2 space-y-6">
    <!-- Top Header -->
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 pb-4 border-b border-white/10">
      <div>
        <p class="text-xs font-mono text-blue-400 uppercase tracking-wider">Node Quality Control & Routing Ledger</p>
        <h2 class="text-xl font-bold text-white tracking-tight">节点管理与质检中心</h2>
        <p class="text-xs text-slate-400 mt-1">
          查看所有订阅与手动节点，进行真实出站握手测速、流媒体与 AI 解锁全项质检，以及配置跳板代理链路。
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2.5 w-full sm:w-auto">
        <button
          class="inline-flex min-h-[44px] items-center justify-center gap-1.5 rounded-lg border border-blue-500/40 bg-blue-600/20 px-4 py-2 text-xs font-medium text-blue-400 hover:bg-blue-600/30 transition-colors cursor-pointer flex-1 sm:flex-initial"
          :disabled="probing || !rows.length"
          @click="startProbeBatch(effectiveBatchTargets)"
        >
          <span v-if="probing" class="inline-block h-3 w-3 animate-spin rounded-full border-2 border-solid border-current border-r-transparent"></span>
          {{ probing ? `质检中 (${probeProgress.done}/${probeProgress.total})…` : '⚡ 综合质检' }}
        </button>
        <button
          v-if="probing"
          class="inline-flex min-h-[44px] items-center justify-center gap-1.5 rounded-lg border border-rose-500/40 bg-rose-600/20 px-3.5 py-2 text-xs font-medium text-rose-400 hover:bg-rose-600/30 transition-colors cursor-pointer flex-1 sm:flex-initial"
          @click="cancelProbeBatch"
        >
          停止探测
        </button>
        <button
          class="inline-flex min-h-[44px] items-center justify-center gap-1.5 rounded-lg border border-white/10 bg-slate-800/40 px-3.5 py-2 text-xs font-medium text-slate-300 hover:bg-slate-800 transition-colors cursor-pointer flex-1 sm:flex-initial"
          :disabled="loading"
          @click="reload"
        >
          🔄 刷新
        </button>
      </div>
    </div>

    <!-- Alert / Error Message -->
    <div v-if="error" class="p-4 rounded-xl border border-rose-500/30 bg-rose-500/10 text-xs font-mono text-rose-400">
      {{ error }}
    </div>

    <!-- Live Probe Progress Banner -->
    <div v-if="probing" class="p-4 rounded-xl border border-blue-500/30 bg-slate-900/60 backdrop-blur-md space-y-2">
      <div class="flex justify-between items-center text-xs font-mono">
        <span class="text-blue-400 font-semibold">
          全协议深度质检进行中: {{ probeProgress.done }} / {{ probeProgress.total }} ({{ probeProgressPercent }}%)
        </span>
        <div class="flex gap-3">
          <span class="text-emerald-400">🟢 正常: {{ probeProgress.ok }}</span>
          <span class="text-rose-400">🔴 失败: {{ probeProgress.fail }}</span>
          <span v-if="includeSpeedtest" class="text-cyan-400">🚀 测速中</span>
        </div>
      </div>
      <div class="h-1.5 w-full overflow-hidden rounded-full bg-slate-800">
        <div
          class="h-full bg-blue-500 transition-all duration-300"
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

    <!-- Selection indicator & quick select-all toolbar -->
    <div class="flex items-center justify-between text-xs font-mono text-slate-400 px-1">
      <div class="flex items-center gap-3">
        <label class="inline-flex items-center gap-1.5 cursor-pointer select-none">
          <input
            type="checkbox"
            :checked="isAllFilteredSelected"
            class="rounded border-white/20 bg-slate-900 text-blue-500 focus:ring-0"
            @change="toggleSelectAllFiltered"
          />
          <span>全选当前筛选节点 ({{ filteredRows.length }})</span>
        </label>
        <span v-if="selectedNodeNames.size > 0" class="text-blue-400">
          已跨视口选中 {{ selectedNodeNames.size }} 个节点
        </span>
      </div>

      <div v-if="viewMode === 'grid'" class="flex items-center gap-2">
        <button
          class="px-2 py-1 rounded border border-white/10 bg-slate-800/40 text-slate-300 disabled:opacity-40"
          :disabled="gridPage <= 1"
          @click="gridPage--"
        >
          上一页
        </button>
        <span>第 {{ gridPage }} / {{ totalGridPages }} 页</span>
        <button
          class="px-2 py-1 rounded border border-white/10 bg-slate-800/40 text-slate-300 disabled:opacity-40"
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
      class="p-12 rounded-xl border border-dashed border-white/10 bg-slate-900/30 text-center space-y-3"
    >
      <span class="text-3xl">🔍</span>
      <p class="text-sm text-slate-300">没有找到符合当前筛选条件的节点</p>
      <button
        class="inline-flex items-center gap-1.5 rounded-lg border border-blue-500/40 bg-blue-600/20 px-3 py-1.5 text-xs text-blue-400 hover:bg-blue-600/30 cursor-pointer"
        @click="resetFilters"
      >
        清空所有筛选条件
      </button>
    </div>

    <!-- Node Virtual Table View (High Performance Virtualized for 2000+ Nodes) -->
    <div v-else-if="viewMode === 'table'" class="space-y-4">
      <VirtualNodeTable
        :items="filteredRows"
        :estimate-size="52"
        :selected-keys="selectedNodeNames"
      >
        <template #default="{ item, isSelected }">
          <div
            class="flex w-full items-center justify-between py-1.5 cursor-pointer select-none"
            @click="inspectNode(item)"
          >
            <!-- Checkbox & Node Name & Protocol -->
            <div class="flex items-center gap-2 sm:gap-3 min-w-0 flex-1">
              <input
                type="checkbox"
                :checked="isSelected"
                class="rounded border-white/20 bg-slate-900 text-blue-500 focus:ring-0 cursor-pointer shrink-0"
                @click.stop="toggleSelectNode(item.name)"
              />
              <span class="text-sm shrink-0">{{ getNodeFlagEmoji(item) }}</span>
              <span
                class="text-xs font-mono font-bold text-white hover:text-blue-400 transition-colors truncate max-w-[140px] sm:max-w-xs"
                :title="item.name"
              >
                {{ item.name }}
              </span>
              <StatusBadge type="info" :text="(item.type || 'RAW').toUpperCase()" class="shrink-0" />
            </div>

            <!-- Server & Port -->
            <div class="hidden md:flex items-center gap-2 text-xs font-mono text-slate-400 flex-1">
              <span class="truncate max-w-[180px]">{{ item.server }}</span>
              <span class="text-slate-500">:{{ item.port }}</span>
              <span v-if="item.subscription_name" class="text-[10px] text-slate-500 truncate max-w-[100px]">
                [{{ item.subscription_name }}]
              </span>
            </div>

            <!-- Dialer Chain Badge -->
            <div class="hidden lg:flex items-center gap-2 flex-1">
              <span
                v-if="item.dialer_proxy"
                class="inline-flex items-center gap-1 rounded px-2 py-0.5 text-[10px] font-mono border"
                :class="item.chain_source === 'node' ? 'border-blue-500/30 bg-blue-500/10 text-blue-400' : 'border-purple-500/30 bg-purple-500/10 text-purple-400'"
              >
                🔗 {{ item.dialer_proxy }}
              </span>
            </div>

            <!-- Probe Metrics & Quick Actions -->
            <div class="flex items-center gap-3 font-mono text-xs">
              <span
                v-if="getProbe(item.name)?.status === 'ok'"
                class="text-emerald-400 font-semibold"
              >
                ⚡ {{ getProbe(item.name)?.latency_ms }}ms
              </span>
              <span
                v-else-if="getProbe(item.name)?.status === 'fail'"
                class="text-rose-400 font-semibold"
              >
                🔴 失败
              </span>
              <span
                v-else-if="getProbe(item.name)?.status === 'timeout'"
                class="text-amber-400 font-semibold"
              >
                ⏱️ 超时
              </span>
              <span v-else class="text-slate-600">⚪ 未测</span>

              <span
                v-if="getProbe(item.name)?.speed_mbps"
                class="text-cyan-400 hidden sm:inline font-semibold"
              >
                🚀 {{ getProbe(item.name)?.speed_mbps }}M
              </span>

              <button
                class="min-h-[36px] min-w-[36px] sm:min-h-[32px] sm:min-w-[32px] rounded border border-white/10 bg-slate-800/40 px-2 py-1 text-[11px] text-slate-300 hover:bg-slate-800 hover:text-white cursor-pointer flex items-center justify-center transition-colors"
                title="单节点测速"
                :disabled="probingSingleNodeKey === item.name"
                @click.stop="handleProbeSingle(item)"
              >
                {{ probingSingleNodeKey === item.name ? '…' : '⚡' }}
              </button>
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
        class="flex flex-col justify-between p-4 rounded-xl border bg-slate-900/50 transition-all cursor-pointer backdrop-blur-md space-y-3"
        :class="[
          selectedNodeNames.has(item.name)
            ? 'border-blue-500/60 bg-blue-950/20'
            : 'border-white/10 hover:border-blue-500/40'
        ]"
        @click="inspectNode(item)"
      >
        <!-- Card Top -->
        <div class="flex items-start justify-between gap-2">
          <div class="flex items-center gap-2 min-w-0 flex-1">
            <input
              type="checkbox"
              :checked="selectedNodeNames.has(item.name)"
              class="rounded border-white/20 bg-slate-900 text-blue-500 focus:ring-0 cursor-pointer"
              @click.stop="toggleSelectNode(item.name)"
            />
            <span class="text-sm">{{ getNodeFlagEmoji(item) }}</span>
            <span class="text-sm font-semibold text-white truncate" :title="item.name">{{ item.name }}</span>
          </div>
          <StatusBadge type="info" :text="(item.type || 'RAW').toUpperCase()" />
        </div>

        <!-- Endpoint & Subscription -->
        <div class="text-xs font-mono text-slate-400 truncate flex justify-between">
          <span>{{ item.server }}:{{ item.port }}</span>
          <span v-if="item.subscription_name" class="text-slate-500">📁 {{ item.subscription_name }}</span>
        </div>

        <!-- Dialer Chain if set -->
        <div v-if="item.dialer_proxy" class="text-xs font-mono text-blue-400 flex items-center gap-1">
          <span>🔗 跳板: {{ item.dialer_proxy }}</span>
        </div>

        <!-- Probe Metrics Strip -->
        <div class="flex items-center justify-between pt-2 border-t border-white/5 text-xs font-mono">
          <span v-if="getProbe(item.name)?.status === 'ok'" class="text-emerald-400 font-semibold">
            ⚡ {{ getProbe(item.name)?.latency_ms }}ms
          </span>
          <span v-else-if="getProbe(item.name)?.status === 'fail'" class="text-rose-400">🔴 失败</span>
          <span v-else-if="getProbe(item.name)?.status === 'timeout'" class="text-amber-400">⏱️ 超时</span>
          <span v-else class="text-slate-600">⚪ 未测</span>

          <span v-if="getProbe(item.name)?.speed_mbps" class="text-cyan-400 font-semibold">
            🚀 {{ getProbe(item.name)?.speed_mbps }} Mbps
          </span>
        </div>

        <!-- Footer Actions -->
        <div class="flex gap-2 pt-2 border-t border-white/5">
          <button
            class="flex-1 py-1.5 rounded-lg border border-white/10 bg-slate-800/40 text-xs font-mono text-slate-300 hover:bg-slate-800 hover:text-white cursor-pointer text-center"
            @click.stop="inspectNode(item)"
          >
            🔍 详情
          </button>
          <button
            class="flex-1 py-1.5 rounded-lg border border-blue-500/30 bg-blue-600/20 text-xs font-mono text-blue-400 hover:bg-blue-600/30 cursor-pointer text-center"
            :disabled="probingSingleNodeKey === item.name"
            @click.stop="handleProbeSingle(item)"
          >
            {{ probingSingleNodeKey === item.name ? '探测中…' : '⚡ 测速' }}
          </button>
        </div>
      </div>
    </div>

    <!-- Node Inspector & Dialer Chain Drawer (Componentized) -->
    <LedgerDrawer
      :open="drawerOpen"
      :node="selectedNode"
      :probe="selectedNode ? getProbe(selectedNode.name) : null"
      :node-candidates="rows"
      :group-candidates="nodeGroups"
      :probing-single="selectedNode ? probingSingleNodeKey === selectedNode.name : false"
      :saving-chain="savingChain"
      :clearing-chain="clearingChain"
      @close="drawerOpen = false"
      @probe-single="handleProbeSingle"
      @save-chain="handleSaveChain"
      @clear-chain="handleClearChain"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  clearProbeResults,
  createProxyChain,
  deleteProxyChain,
  getApiErrorMessage,
  getNodeGroups,
  getNodeLedger,
  getProbeResults,
  getProxyChains,
  probeNode,
  probeNodesFull,
} from '../api'
import LedgerDrawer from '../components/ledger/LedgerDrawer.vue'
import LedgerMetricsBar from '../components/ledger/LedgerMetricsBar.vue'
import LedgerSearchFilter from '../components/ledger/LedgerSearchFilter.vue'
import StatusBadge from '../components/ui/StatusBadge.vue'
import VirtualNodeTable from '../components/ui/VirtualNodeTable.vue'
import { useAppStore } from '../stores/app'
import {
  applyMetricShortcut,
  buildEffectiveBatchTargets,
  clearNodeDialerProxy,
  computeFilterOptions,
  COUNTRY_FLAG_MAP,
  filterAndSortNodes,
  getProbeForNode,
  MEDIA_PLATFORMS,
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
        if (
          item?.status === 'ok' ||
          item?.status === 'full' ||
          item?.status === 'originals' ||
          item?.unlocked === true
        ) {
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

function getNodeFlagEmoji(node: LedgerNodeItem): string {
  const p = getProbeForNode(probes.value, node)
  const code = resolveNodeCountryCode(node, p)
  return COUNTRY_FLAG_MAP[code] || '🌐'
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

function inspectNode(node: LedgerNodeItem) {
  selectedNode.value = node
  drawerOpen.value = true
}

// Data Load
async function reload() {
  loading.value = true
  error.value = ''
  try {
    const [ledgerRes, chainsRes, groupsRes, probeRes] = await Promise.all([
      getNodeLedger(),
      getProxyChains(),
      getNodeGroups(),
      getProbeResults().catch(() => ({ data: {} })),
    ])
    rows.value = ledgerRes?.data || []
    bindings.value = chainsRes?.data || []
    nodeGroups.value = groupsRes?.data || []

    const rawProbeData = probeRes?.data?.results || probeRes?.data
    probes.value = normalizeNodeLedgerMap(rawProbeData)
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
        concurrency: 5,
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
</script>
