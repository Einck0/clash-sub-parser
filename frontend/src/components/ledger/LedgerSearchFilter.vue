<template>
  <div class="p-4 rounded-lg border border-border-subtle bg-surface-base space-y-3.5 mb-6">
    <!-- Top Row: Main Filters & Controls -->
    <div class="flex flex-wrap items-center gap-3">
      <!-- Search Box -->
      <div class="relative min-w-[240px] flex-1 max-w-md">
        <input
          :value="modelValue.keyword"
          type="text"
          placeholder="搜索节点名 / 出口 IP / 服务器 / 端口 / 协议 / 跳板 / 策略组…"
          class="w-full rounded-md border border-border-subtle bg-canvas px-3.5 py-2 pl-9 pr-8 text-xs text-text-main placeholder:text-text-sub focus:border-accent focus:outline-hidden font-mono"
          @input="updateField('keyword', ($event.target as HTMLInputElement).value)"
        />
        <Search class="absolute left-3 top-2.5 h-3.5 w-3.5 text-text-muted" aria-hidden="true" />
        <button
          v-if="modelValue.keyword"
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
        class="rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono cursor-pointer"
        title="按订阅来源筛选"
        @change="updateField('subscription', ($event.target as HTMLSelectElement).value)"
      >
        <option value="">全部订阅 ({{ totalCount }})</option>
        <option v-for="s in subscriptions" :key="s" :value="s">{{ s }}</option>
      </select>

      <!-- Protocol Dropdown -->
      <select
        :value="modelValue.protocol"
        class="rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono cursor-pointer"
        title="按节点协议筛选"
        @change="updateField('protocol', ($event.target as HTMLSelectElement).value)"
      >
        <option value="">全部协议</option>
        <option v-for="t in protocols" :key="t" :value="t">{{ t.toUpperCase() }}</option>
      </select>

      <!-- Health Status Filter -->
      <select
        :value="modelValue.status"
        class="rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono cursor-pointer"
        title="按健康状态筛选"
        @change="updateField('status', ($event.target as HTMLSelectElement).value)"
      >
        <option value="all">全部状态</option>
        <option value="ok">正常可用</option>
        <option value="fast">低延极速 (&lt;300ms)</option>
        <option value="medium">普通延迟 (300-800ms)</option>
        <option value="fail">离线失败</option>
        <option value="untested">尚未探测</option>
      </select>

      <!-- Dialer Chain Filter -->
      <select
        :value="modelValue.chain"
        class="rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono cursor-pointer"
        title="按链路跳板筛选"
        @change="updateField('chain', ($event.target as HTMLSelectElement).value)"
      >
        <option value="all">全部链路</option>
        <option value="chained">仅看已挂链</option>
        <option value="plain">仅看直连 (未挂链)</option>
      </select>

      <!-- Sort Options -->
      <select
        :value="modelValue.sortBy"
        class="rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono cursor-pointer"
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

    <!-- Media & AI Unlock Matrix Chips -->
    <div class="flex flex-wrap items-center gap-2 pt-2 border-t border-border-subtle">
      <span class="text-xs font-mono text-text-muted mr-1 flex items-center gap-1.5">
        <Film class="h-3.5 w-3.5 text-accent" aria-hidden="true" />
        <span>解锁过滤:</span>
      </span>
      <button
        v-for="platform in mediaPlatforms"
        :key="platform.key"
        class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-mono transition-colors cursor-pointer select-none"
        :class="[
          modelValue.mediaPlatforms.includes(platform.key)
            ? 'border-accent bg-accent-subtle text-accent font-semibold ring-1 ring-accent/30'
            : (mediaStats?.[platform.key] || 0) > 0
              ? 'border-border-subtle bg-surface-hover text-text-main hover:border-accent/40'
              : 'border-border-subtle/50 bg-canvas text-text-sub hover:border-border-subtle'
        ]"
        :title="`点击筛选支持 ${platform.name} 的节点`"
        @click="toggleMedia(platform.key)"
      >
        <span>{{ platform.name }}</span>
        <span
          class="rounded-full px-1.5 py-0.2 text-[10px] font-mono tabular-nums"
          :class="[
            (mediaStats?.[platform.key] || 0) > 0
              ? 'bg-status-success/15 text-status-success font-bold'
              : 'bg-surface-active text-text-sub'
          ]"
        >
          {{ mediaStats?.[platform.key] || 0 }}
        </span>
      </button>
    </div>

    <!-- Quick Country / Region Pills -->
    <div v-if="countries && countries.length" class="flex flex-wrap items-center gap-1.5 pt-2 border-t border-border-subtle">
      <span class="text-xs font-mono text-text-muted mr-1 flex items-center gap-1.5">
        <Globe class="h-3.5 w-3.5 text-accent" aria-hidden="true" />
        <span>快捷地区:</span>
      </span>
      <button
        class="rounded-full border px-2.5 py-0.5 text-xs font-mono transition-colors cursor-pointer"
        :class="[
          !modelValue.country
            ? 'border-accent bg-accent-subtle text-accent font-semibold ring-1 ring-accent/30'
            : 'border-border-subtle bg-surface-hover text-text-muted hover:text-text-main'
        ]"
        @click="updateField('country', '')"
      >
        全部 ({{ totalCount }})
      </button>
      <button
        v-for="c in countries"
        :key="c.code"
        class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-mono transition-colors cursor-pointer"
        :class="[
          modelValue.country === c.code
            ? 'border-accent bg-accent-subtle text-accent font-semibold ring-1 ring-accent/30'
            : 'border-border-subtle bg-surface-hover text-text-muted hover:border-accent/30 hover:text-text-main'
        ]"
        @click="updateField('country', modelValue.country === c.code ? '' : c.code)"
      >
        <span>{{ c.name }}</span>
        <span class="text-[10px] tabular-nums text-text-sub">({{ c.count }})</span>
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
            class="rounded-sm border-border-strong bg-canvas text-accent focus:ring-0"
            @change="$emit('update:includeSpeed', ($event.target as HTMLInputElement).checked)"
          />
          <Gauge class="h-3.5 w-3.5 text-accent" aria-hidden="true" />
          <span>包含测速</span>
        </label>

        <label class="inline-flex items-center gap-1.5 text-text-muted hover:text-text-main cursor-pointer select-none">
          <input
            type="checkbox"
            :checked="includeMedia"
            class="rounded-sm border-border-strong bg-canvas text-accent focus:ring-0"
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

      <!-- Right: Selection actions & View Switcher -->
      <div class="flex items-center gap-3">
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
        <div v-else class="flex items-center gap-2">
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

        <!-- View mode switcher -->
        <div class="flex items-center gap-1 border-l border-border-subtle pl-3">
          <button
            :class="[
              'flex h-7 w-7 items-center justify-center rounded-md border transition-colors cursor-pointer text-xs focus-ring',
              viewMode === 'table' ? 'border-accent bg-accent-subtle text-accent' : 'border-border-subtle bg-surface-hover text-text-muted hover:text-text-main'
            ]"
            title="表格模式"
            aria-label="表格模式"
            @click="$emit('change-view', 'table')"
          >
            <Table class="h-3.5 w-3.5" aria-hidden="true" />
          </button>
          <button
            :class="[
              'flex h-7 w-7 items-center justify-center rounded-md border transition-colors cursor-pointer text-xs focus-ring',
              viewMode === 'grid' ? 'border-accent bg-accent-subtle text-accent' : 'border-border-subtle bg-surface-hover text-text-muted hover:text-text-main'
            ]"
            title="卡片模式"
            aria-label="卡片模式"
            @click="$emit('change-view', 'grid')"
          >
            <LayoutGrid class="h-3.5 w-3.5" aria-hidden="true" />
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Film,
  Gauge,
  Globe,
  LayoutGrid,
  Search,
  Table,
  Target,
  Trash2,
  Tv,
  X,
  Zap,
} from 'lucide-vue-next'
import Button from '../ui/Button.vue'
import type { CountryOption, FilterState, MediaPlatformDef } from '../../views/nodeLedgerDomain'

const props = withDefaults(
  defineProps<{
    modelValue: FilterState
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
  (e: 'update:modelValue', val: FilterState): void
  (e: 'change-view', mode: 'table' | 'grid'): void
  (e: 'update:includeSpeed', val: boolean): void
  (e: 'update:includeMedia', val: boolean): void
  (e: 'probe-selected'): void
  (e: 'clear-selection'): void
  (e: 'probe-untested'): void
  (e: 'clear-probe-data'): void
  (e: 'reset-filters'): void
}>()

function updateField<K extends keyof FilterState>(key: K, value: FilterState[K]) {
  emit('update:modelValue', {
    ...props.modelValue,
    [key]: value,
  })
}

function toggleMedia(key: string) {
  const current = [...props.modelValue.mediaPlatforms]
  const idx = current.indexOf(key)
  if (idx >= 0) {
    current.splice(idx, 1)
  } else {
    current.push(key)
  }
  updateField('mediaPlatforms', current)
}
</script>
