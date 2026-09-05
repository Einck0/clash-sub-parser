<template>
  <div class="p-4 rounded-lg border border-border-subtle bg-surface-base space-y-3.5 mb-6">
    <!-- Top Row: Search Box, Subscription Select & Collapsible Facet Buttons -->
    <div class="flex flex-wrap items-center gap-2.5">
      <!-- Search Box -->
      <div class="relative min-w-[220px] flex-1 max-w-sm">
        <input
          :value="modelValue.keyword"
          type="text"
          placeholder="搜索节点 / IP / 端口 / 协议 / 跳板…"
          class="w-full rounded-md border border-border-subtle bg-canvas px-3.5 py-2 pl-9 pr-8 text-xs text-text-main placeholder:text-text-sub focus:border-accent focus:outline-hidden font-mono"
          @input="updateField('keyword', ($event.target as HTMLInputElement).value)"
        />
        <Search class="absolute left-3 top-2.5 h-3.5 w-3.5 text-text-muted" aria-hidden="true" />
        <button
          v-if="modelValue.keyword"
          type="button"
          class="absolute right-2.5 top-2 p-0.5 text-text-muted hover:text-text-main cursor-pointer"
          title="清空搜索"
          aria-label="清空搜索"
          @click="updateField('keyword', '')"
        >
          <X class="h-3.5 w-3.5" aria-hidden="true" />
        </button>
      </div>

      <!-- Subscription Dropdown -->
      <select
        :value="modelValue.subscription"
        class="min-h-[44px] sm:min-h-[36px] rounded-md border border-border-subtle bg-surface-base px-3 py-1.5 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono cursor-pointer"
        title="按订阅来源筛选"
        @change="updateField('subscription', ($event.target as HTMLSelectElement).value)"
      >
        <option value="">全部订阅 ({{ totalCount }})</option>
        <option v-for="s in subscriptions" :key="s" :value="s">{{ s }}</option>
      </select>

      <!-- Collapsible Facet 1: 快捷地区 (Countries) -->
      <LedgerFacetSelect
        title="快捷地区"
        :icon="Globe"
        :selected="modelValue.countries"
        :options="countryFacetOptions"
        :is-open="activeFacet === 'countries'"
        @toggle="toggleFacet('countries')"
        @close="closeFacet"
        @change="updateField('countries', $event)"
        @clear="updateField('countries', [])"
      />

      <!-- Collapsible Facet 2: 全部协议 (Protocols) -->
      <LedgerFacetSelect
        title="全部协议"
        :icon="Network"
        :selected="modelValue.protocols"
        :options="protocolFacetOptions"
        :is-open="activeFacet === 'protocols'"
        @toggle="toggleFacet('protocols')"
        @close="closeFacet"
        @change="updateField('protocols', $event)"
        @clear="updateField('protocols', [])"
      />

      <!-- Collapsible Facet 3: 健康状态 (Statuses) -->
      <LedgerFacetSelect
        title="健康状态"
        :icon="Activity"
        :selected="modelValue.statuses"
        :options="statusFacetOptions"
        :is-open="activeFacet === 'statuses'"
        @toggle="toggleFacet('statuses')"
        @close="closeFacet"
        @change="updateField('statuses', $event)"
        @clear="updateField('statuses', [])"
      />

      <!-- Collapsible Facet 4: 解锁过滤 (Media Platforms) -->
      <LedgerFacetSelect
        title="解锁过滤"
        :icon="Film"
        :selected="modelValue.mediaPlatforms"
        :options="mediaFacetOptions"
        :is-open="activeFacet === 'media'"
        @toggle="toggleFacet('media')"
        @close="closeFacet"
        @change="updateField('mediaPlatforms', $event)"
        @clear="updateField('mediaPlatforms', [])"
      />

      <!-- Collapsible Facet 5: 链路跳板 (Chains) -->
      <LedgerFacetSelect
        title="链路跳板"
        :icon="GitFork"
        :selected="modelValue.chain === 'all' ? [] : [modelValue.chain]"
        :options="chainFacetOptions"
        :is-open="activeFacet === 'chain'"
        @toggle="toggleFacet('chain')"
        @close="closeFacet"
        @change="handleChainFacetChange"
        @clear="updateField('chain', 'all')"
      />

      <!-- Sort Options -->
      <select
        :value="modelValue.sortBy"
        class="min-h-[44px] sm:min-h-[36px] rounded-md border border-border-subtle bg-surface-base px-3 py-1.5 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono cursor-pointer"
        title="排序规则"
        @change="updateField('sortBy', ($event.target as HTMLSelectElement).value)"
      >
        <option value="default">默认排列</option>
        <option value="latency_asc">延迟从低到高</option>
        <option value="latency_desc">延迟从高到低</option>
        <option value="speed_desc">测速从高到低</option>
        <option value="name_asc">节点名 (A-Z)</option>
        <option value="country">国家地区</option>
        <option value="checked_desc">最近质检时间</option>
      </select>
    </div>

    <!-- Bounded Active-Filter Summary Strip: Only renders when active filters exist -->
    <div
      v-if="hasActiveFilters"
      class="flex flex-wrap items-center gap-1.5 pt-2 border-t border-border-subtle text-xs font-mono min-w-0 max-w-full overflow-hidden"
      role="region"
      aria-label="已生效筛选条件"
    >
      <span class="text-text-muted text-[11px] shrink-0 mr-1 flex items-center gap-1">
        <Filter class="h-3 w-3 text-accent" aria-hidden="true" />
        <span>已选条件:</span>
      </span>

      <!-- Keyword chip -->
      <span
        v-if="modelValue.keyword"
        class="inline-flex items-center gap-1 rounded-full border border-border-subtle bg-surface-hover px-2.5 py-0.5 text-xs text-text-main max-w-[200px] truncate"
      >
        <span class="truncate">搜索: {{ modelValue.keyword }}</span>
        <button
          type="button"
          class="text-text-muted hover:text-text-main p-0.5 cursor-pointer"
          title="移除关键词"
          aria-label="移除关键词"
          @click="updateField('keyword', '')"
        >
          <X class="h-3 w-3" aria-hidden="true" />
        </button>
      </span>

      <!-- Subscription chip -->
      <span
        v-if="modelValue.subscription"
        class="inline-flex items-center gap-1 rounded-full border border-border-subtle bg-surface-hover px-2.5 py-0.5 text-xs text-text-main max-w-[200px] truncate"
      >
        <span class="truncate">订阅: {{ modelValue.subscription }}</span>
        <button
          type="button"
          class="text-text-muted hover:text-text-main p-0.5 cursor-pointer"
          title="移除订阅筛选"
          aria-label="移除订阅筛选"
          @click="updateField('subscription', '')"
        >
          <X class="h-3 w-3" aria-hidden="true" />
        </button>
      </span>

      <!-- Country chips -->
      <span
        v-for="c in modelValue.countries"
        :key="'c-' + c"
        class="inline-flex items-center gap-1 rounded-full border border-border-subtle bg-surface-hover px-2.5 py-0.5 text-xs text-text-main max-w-[200px] truncate"
      >
        <span class="truncate">地区: {{ getCountryLabel(c) }}</span>
        <button
          type="button"
          class="text-text-muted hover:text-text-main p-0.5 cursor-pointer"
          :title="`移除地区 ${getCountryLabel(c)}`"
          :aria-label="`移除地区 ${getCountryLabel(c)}`"
          @click="removeCountry(c)"
        >
          <X class="h-3 w-3" aria-hidden="true" />
        </button>
      </span>

      <!-- Protocol chips -->
      <span
        v-for="p in modelValue.protocols"
        :key="'p-' + p"
        class="inline-flex items-center gap-1 rounded-full border border-border-subtle bg-surface-hover px-2.5 py-0.5 text-xs text-text-main max-w-[200px] truncate"
      >
        <span class="truncate">协议: {{ p.toUpperCase() }}</span>
        <button
          type="button"
          class="text-text-muted hover:text-text-main p-0.5 cursor-pointer"
          :title="`移除协议 ${p}`"
          :aria-label="`移除协议 ${p}`"
          @click="removeProtocol(p)"
        >
          <X class="h-3 w-3" aria-hidden="true" />
        </button>
      </span>

      <!-- Status chips -->
      <span
        v-for="s in modelValue.statuses"
        :key="'s-' + s"
        class="inline-flex items-center gap-1 rounded-full border border-border-subtle bg-surface-hover px-2.5 py-0.5 text-xs text-text-main max-w-[200px] truncate"
      >
        <span class="truncate">状态: {{ getStatusLabel(s) }}</span>
        <button
          type="button"
          class="text-text-muted hover:text-text-main p-0.5 cursor-pointer"
          :title="`移除状态 ${getStatusLabel(s)}`"
          :aria-label="`移除状态 ${getStatusLabel(s)}`"
          @click="removeStatus(s)"
        >
          <X class="h-3 w-3" aria-hidden="true" />
        </button>
      </span>

      <!-- Chain chip -->
      <span
        v-if="modelValue.chain && modelValue.chain !== 'all'"
        class="inline-flex items-center gap-1 rounded-full border border-border-subtle bg-surface-hover px-2.5 py-0.5 text-xs text-text-main max-w-[200px] truncate"
      >
        <span class="truncate">链路: {{ modelValue.chain === 'chained' ? '已挂链' : '直连' }}</span>
        <button
          type="button"
          class="text-text-muted hover:text-text-main p-0.5 cursor-pointer"
          title="移除链路筛选"
          aria-label="移除链路筛选"
          @click="updateField('chain', 'all')"
        >
          <X class="h-3 w-3" aria-hidden="true" />
        </button>
      </span>

      <!-- Speed chip -->
      <span
        v-if="modelValue.minSpeed > 0"
        class="inline-flex items-center gap-1 rounded-full border border-border-subtle bg-surface-hover px-2.5 py-0.5 text-xs text-text-main max-w-[200px] truncate"
      >
        <span class="truncate">门槛: ≥{{ modelValue.minSpeed }}M</span>
        <button
          type="button"
          class="text-text-muted hover:text-text-main p-0.5 cursor-pointer"
          title="移除测速门槛"
          aria-label="移除测速门槛"
          @click="updateField('minSpeed', 0)"
        >
          <X class="h-3 w-3" aria-hidden="true" />
        </button>
      </span>

      <!-- Media chips -->
      <span
        v-for="m in modelValue.mediaPlatforms"
        :key="'m-' + m"
        class="inline-flex items-center gap-1 rounded-full border border-accent/40 bg-accent-subtle px-2.5 py-0.5 text-xs text-accent max-w-[200px] truncate"
      >
        <span class="truncate">解锁: {{ getMediaPlatformLabel(m) }}</span>
        <button
          type="button"
          class="text-accent hover:text-accent p-0.5 cursor-pointer"
          :title="`移除解锁 ${getMediaPlatformLabel(m)}`"
          :aria-label="`移除解锁 ${getMediaPlatformLabel(m)}`"
          @click="removeMediaPlatform(m)"
        >
          <X class="h-3 w-3" aria-hidden="true" />
        </button>
      </span>

      <!-- Reset all filters button -->
      <button
        type="button"
        class="inline-flex items-center gap-1 text-xs text-text-muted hover:text-status-danger cursor-pointer ml-auto px-2 py-1 transition-colors min-h-[32px] sm:min-h-[28px]"
        title="清空所有筛选条件"
        @click="resetAllFilters"
      >
        <RotateCcw class="h-3 w-3" aria-hidden="true" />
        <span>清空所有</span>
      </button>
    </div>

    <!-- Bottom Row: Speed Threshold, Probe Toggles & Actions -->
    <div class="flex flex-wrap items-center justify-between gap-4 pt-2 border-t border-border-subtle">
      <!-- Left: Probe options and speed gate -->
      <div class="flex flex-wrap items-center gap-4 text-xs font-mono">
        <label class="inline-flex items-center gap-1.5 text-text-muted hover:text-text-main cursor-pointer select-none">
          <input
            type="checkbox"
            :checked="includeSpeed"
            class="rounded-xs border-border-strong bg-canvas text-accent focus:ring-0"
            @change="$emit('update:includeSpeed', ($event.target as HTMLInputElement).checked)"
          />
          <Gauge class="h-3.5 w-3.5 text-accent" aria-hidden="true" />
          <span>包含测速</span>
        </label>

        <label class="inline-flex items-center gap-1.5 text-text-muted hover:text-text-main cursor-pointer select-none">
          <input
            type="checkbox"
            :checked="includeMedia"
            class="rounded-xs border-border-strong bg-canvas text-accent focus:ring-0"
            @change="$emit('update:includeMedia', ($event.target as HTMLInputElement).checked)"
          />
          <Tv class="h-3.5 w-3.5 text-accent" aria-hidden="true" />
          <span>包含流媒体/AI</span>
        </label>

        <div class="inline-flex items-center gap-1.5 text-text-muted">
          <span>门槛:</span>
          <input
            :value="modelValue.minSpeed || ''"
            type="number"
            min="0"
            step="5"
            placeholder="0"
            class="w-16 rounded-md border border-border-subtle bg-canvas px-2 py-1 text-xs text-text-main text-center focus:border-accent focus:outline-hidden font-mono tabular-nums"
            @input="updateField('minSpeed', Math.max(0, Number(($event.target as HTMLInputElement).value) || 0))"
          />
          <span>Mbps</span>
        </div>
      </div>

      <!-- Right: Action Bar & View Switcher -->
      <div class="flex flex-wrap items-center gap-3">
        <!-- Action Bar: Batch actions with programmatic label -->
        <div class="flex flex-wrap items-center gap-2" role="toolbar" aria-label="节点质检操作">
          <!-- If items are selected -->
          <div v-if="selectedCount && selectedCount > 0" class="flex items-center gap-2">
            <span class="text-xs font-mono text-accent">
              已选 <strong class="tabular-nums">{{ selectedCount }}</strong> 项
            </span>
            <Button
              variant="primary"
              size="sm"
              :disabled="probing"
              :icon="Zap"
              @click="$emit('probe-selected')"
            >
              质检选中
            </Button>
            <Button
              variant="secondary"
              size="sm"
              @click="$emit('clear-selection')"
            >
              取消选择
            </Button>
          </div>

          <!-- If no items selected: quick batch helpers -->
          <div v-else class="flex flex-wrap items-center gap-2">
            <Button
              variant="secondary"
              size="sm"
              :disabled="probing || !untestedOrFailedCount"
              :icon="Target"
              title="仅对尚未测试或上次失败的节点发起探测"
              @click="$emit('probe-untested')"
            >
              仅测未测/失败 ({{ untestedOrFailedCount }})
            </Button>
            <Button
              variant="danger"
              size="sm"
              :icon="Trash2"
              title="清空所有已持久化的节点测速与解锁记录"
              @click="$emit('clear-probe-data')"
            >
              清除质检
            </Button>
          </div>
        </div>

        <!-- View mode switcher: Accessible Segmented Control -->
        <div
          class="inline-flex items-center rounded-lg border border-border-subtle bg-surface-base p-0.5"
          role="radiogroup"
          aria-label="视图模式切换"
        >
          <button
            type="button"
            role="radio"
            :aria-checked="viewMode === 'table'"
            :class="[
              'inline-flex min-h-[44px] min-w-[44px] sm:min-h-[40px] sm:min-w-[40px] items-center justify-center rounded-md text-xs font-medium transition-colors cursor-pointer focus-ring',
              viewMode === 'table'
                ? 'bg-accent text-white font-semibold shadow-xs'
                : 'text-text-muted hover:text-text-main hover:bg-surface-hover'
            ]"
            title="表格模式"
            aria-label="表格模式"
            @click="$emit('change-view', 'table')"
            @keydown.left.prevent="$emit('change-view', 'grid')"
            @keydown.right.prevent="$emit('change-view', 'grid')"
          >
            <Table class="h-4 w-4" aria-hidden="true" />
            <span class="sr-only">表格模式</span>
          </button>
          <button
            type="button"
            role="radio"
            :aria-checked="viewMode === 'grid'"
            :class="[
              'inline-flex min-h-[44px] min-w-[44px] sm:min-h-[40px] sm:min-w-[40px] items-center justify-center rounded-md text-xs font-medium transition-colors cursor-pointer focus-ring',
              viewMode === 'grid'
                ? 'bg-accent text-white font-semibold shadow-xs'
                : 'text-text-muted hover:text-text-main hover:bg-surface-hover'
            ]"
            title="卡片模式"
            aria-label="卡片模式"
            @click="$emit('change-view', 'grid')"
            @keydown.left.prevent="$emit('change-view', 'table')"
            @keydown.right.prevent="$emit('change-view', 'table')"
          >
            <LayoutGrid class="h-4 w-4" aria-hidden="true" />
            <span class="sr-only">卡片模式</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  Activity,
  Film,
  Filter,
  Gauge,
  GitFork,
  Globe,
  LayoutGrid,
  Network,
  RotateCcw,
  Search,
  Table,
  Target,
  Trash2,
  Tv,
  X,
  Zap,
} from 'lucide-vue-next'
import Button from '../ui/Button.vue'
import LedgerFacetSelect, { type FacetOption } from './LedgerFacetSelect.vue'
import {
  CHAIN_FACET_OPTIONS,
  createDefaultFacetFilterState,
  STATUS_FACET_OPTIONS,
  type CountryOption,
  type FacetFilterState,
  type MediaPlatformDef,
} from '../../views/nodeLedgerDomain'

type FacetId = 'countries' | 'protocols' | 'statuses' | 'media' | 'chain' | null

const props = withDefaults(
  defineProps<{
    modelValue: FacetFilterState
    viewMode: 'table' | 'grid'
    subscriptions?: string[]
    protocols?: string[]
    countries?: CountryOption[]
    mediaPlatforms?: MediaPlatformDef[]
    mediaStats?: Record<string, number>
    totalCount?: number
    filteredCount?: number
    selectedCount?: number
    untestedOrFailedCount?: number
    probing?: boolean
    includeSpeed?: boolean
    includeMedia?: boolean
  }>(),
  {
    subscriptions: () => [],
    protocols: () => [],
    countries: () => [],
    mediaPlatforms: () => [],
    mediaStats: () => ({}),
    totalCount: 0,
    filteredCount: 0,
    selectedCount: 0,
    untestedOrFailedCount: 0,
    probing: false,
    includeSpeed: false,
    includeMedia: true,
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', val: FacetFilterState): void
  (e: 'change-view', mode: 'table' | 'grid'): void
  (e: 'update:includeSpeed', val: boolean): void
  (e: 'update:includeMedia', val: boolean): void
  (e: 'probe-selected'): void
  (e: 'clear-selection'): void
  (e: 'probe-untested'): void
  (e: 'clear-probe-data'): void
  (e: 'reset-filters'): void
}>()

const activeFacet = ref<FacetId>(null)

function toggleFacet(id: FacetId) {
  activeFacet.value = activeFacet.value === id ? null : id
}

function closeFacet() {
  activeFacet.value = null
}

function onDocumentClick(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (!target.closest?.('.facet-controller-container')) {
    activeFacet.value = null
  }
}

function onDocumentKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && activeFacet.value) {
    activeFacet.value = null
  }
}

onMounted(() => {
  document.addEventListener('click', onDocumentClick)
  document.addEventListener('keydown', onDocumentKeydown)
})

onUnmounted(() => {
  document.removeEventListener('click', onDocumentClick)
  document.removeEventListener('keydown', onDocumentKeydown)
})

// Options for facets
const countryFacetOptions = computed<FacetOption[]>(() => {
  return (props.countries || []).map((c) => ({
    key: c.code,
    name: c.name,
    count: c.count,
  }))
})

const protocolFacetOptions = computed<FacetOption[]>(() => {
  return (props.protocols || []).map((p) => ({
    key: p.toLowerCase(),
    name: p.toUpperCase(),
  }))
})

const statusFacetOptions = computed<FacetOption[]>(() => {
  return STATUS_FACET_OPTIONS.map((s) => ({
    key: s.key,
    name: s.name,
    short: s.short,
  }))
})

const mediaFacetOptions = computed<FacetOption[]>(() => {
  return (props.mediaPlatforms || []).map((m) => ({
    key: m.key,
    name: m.name,
    count: props.mediaStats?.[m.key] || 0,
    short: m.short,
  }))
})

const chainFacetOptions = computed<FacetOption[]>(() => {
  return CHAIN_FACET_OPTIONS.filter((c) => c.key !== 'all').map((c) => ({
    key: c.key,
    name: c.name,
  }))
})

function handleChainFacetChange(keys: string[]) {
  const val = keys.length > 0 ? (keys[keys.length - 1] as 'chained' | 'plain') : 'all'
  updateField('chain', val)
}

function updateField<K extends keyof FacetFilterState>(key: K, value: FacetFilterState[K]) {
  emit('update:modelValue', {
    ...props.modelValue,
    [key]: value,
  })
}

const hasActiveFilters = computed(() => {
  const v = props.modelValue
  return Boolean(
    v.keyword ||
      v.subscription ||
      v.countries.length > 0 ||
      v.protocols.length > 0 ||
      v.statuses.length > 0 ||
      (v.chain && v.chain !== 'all') ||
      v.minSpeed > 0 ||
      v.mediaPlatforms.length > 0
  )
})

function getCountryLabel(code: string): string {
  const c = props.countries?.find((item) => item.code === code)
  return c?.name || code
}

function getStatusLabel(st: string): string {
  const s = STATUS_FACET_OPTIONS.find((item) => item.key === st)
  return s?.name || st
}

function getMediaPlatformLabel(key: string): string {
  const m = props.mediaPlatforms?.find((item) => item.key === key)
  return m?.name || key
}

function removeCountry(code: string) {
  updateField(
    'countries',
    props.modelValue.countries.filter((c) => c !== code)
  )
}

function removeProtocol(proto: string) {
  updateField(
    'protocols',
    props.modelValue.protocols.filter((p) => p !== proto)
  )
}

function removeStatus(st: string) {
  updateField(
    'statuses',
    props.modelValue.statuses.filter((s) => s !== st)
  )
}

function removeMediaPlatform(key: string) {
  updateField(
    'mediaPlatforms',
    props.modelValue.mediaPlatforms.filter((m) => m !== key)
  )
}

function resetAllFilters() {
  emit('update:modelValue', createDefaultFacetFilterState())
  emit('reset-filters')
}
</script>
