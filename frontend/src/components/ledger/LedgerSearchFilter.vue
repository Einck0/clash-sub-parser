<template>
  <div class="p-4 rounded-xl border border-white/10 bg-slate-900/50 backdrop-blur-md space-y-4 mb-6">
    <!-- Top Row: Main Filters & Controls -->
    <div class="flex flex-wrap items-center gap-3">
      <!-- Search Box -->
      <div class="relative min-w-[240px] flex-1 max-w-md">
        <input
          :value="modelValue.keyword"
          type="text"
          placeholder="搜索节点名 / 出口 IP / 服务器 / 端口 / 协议 / 跳板 / 策略组…"
          class="w-full rounded-lg border border-white/10 bg-slate-950/60 px-3.5 py-2 pl-9 pr-8 text-xs text-white placeholder-slate-500 focus:border-blue-500 focus:outline-hidden font-mono"
          @input="updateField('keyword', ($event.target as HTMLInputElement).value)"
        />
        <svg class="absolute left-3 top-2.5 h-3.5 w-3.5 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
        <button
          v-if="modelValue.keyword"
          class="absolute right-2.5 top-2 text-xs text-slate-500 hover:text-white cursor-pointer"
          title="清空搜索"
          @click="updateField('keyword', '')"
        >
          ✕
        </button>
      </div>

      <!-- Subscription Dropdown -->
      <select
        :value="modelValue.subscription"
        class="rounded-lg border border-white/10 bg-slate-950/60 px-3 py-2 text-xs text-slate-300 focus:border-blue-500 focus:outline-hidden font-mono"
        title="按订阅来源筛选"
        @change="updateField('subscription', ($event.target as HTMLSelectElement).value)"
      >
        <option value="">📁 全部订阅 ({{ totalCount }})</option>
        <option v-for="s in subscriptions" :key="s" :value="s">{{ s }}</option>
      </select>

      <!-- Protocol Dropdown -->
      <select
        :value="modelValue.protocol"
        class="rounded-lg border border-white/10 bg-slate-950/60 px-3 py-2 text-xs text-slate-300 focus:border-blue-500 focus:outline-hidden font-mono"
        title="按节点协议筛选"
        @change="updateField('protocol', ($event.target as HTMLSelectElement).value)"
      >
        <option value="">⚡ 全部协议</option>
        <option v-for="t in protocols" :key="t" :value="t">{{ t.toUpperCase() }}</option>
      </select>

      <!-- Health Status Filter -->
      <select
        :value="modelValue.status"
        class="rounded-lg border border-white/10 bg-slate-950/60 px-3 py-2 text-xs text-slate-300 focus:border-blue-500 focus:outline-hidden font-mono"
        title="按健康状态筛选"
        @change="updateField('status', ($event.target as HTMLSelectElement).value)"
      >
        <option value="all">🚦 全部状态</option>
        <option value="ok">🟢 正常可用</option>
        <option value="fast">⚡ 低延极速 (&lt;300ms)</option>
        <option value="medium">🟡 普通延迟 (300-800ms)</option>
        <option value="fail">🔴 离线失败</option>
        <option value="untested">⚪ 尚未探测</option>
      </select>

      <!-- Dialer Chain Filter -->
      <select
        :value="modelValue.chain"
        class="rounded-lg border border-white/10 bg-slate-950/60 px-3 py-2 text-xs text-slate-300 focus:border-blue-500 focus:outline-hidden font-mono"
        title="按链路跳板筛选"
        @change="updateField('chain', ($event.target as HTMLSelectElement).value)"
      >
        <option value="all">🔗 全部链路</option>
        <option value="chained">仅看已挂链</option>
        <option value="plain">仅看直连 (未挂链)</option>
      </select>

      <!-- Sort Options -->
      <select
        :value="modelValue.sortBy"
        class="rounded-lg border border-white/10 bg-slate-950/60 px-3 py-2 text-xs text-slate-300 focus:border-blue-500 focus:outline-hidden font-mono"
        title="排序规则"
        @change="updateField('sortBy', ($event.target as HTMLSelectElement).value)"
      >
        <option value="default">默认排列</option>
        <option value="latency_asc">⚡ 延迟从低到高</option>
        <option value="latency_desc">⚡ 延迟从高到低</option>
        <option value="speed_desc">🚀 测速从高到低</option>
        <option value="name_asc">🏷️ 节点名 (A-Z)</option>
        <option value="country">🌍 国家地区</option>
        <option value="checked_desc">⏱️ 最近质检时间</option>
      </select>
    </div>

    <!-- Media & AI Unlock Matrix Chips -->
    <div class="flex flex-wrap items-center gap-2 pt-2 border-t border-white/5">
      <span class="text-xs font-mono text-slate-400 mr-1 flex items-center gap-1">
        <span>🎬</span>
        <span>解锁过滤:</span>
      </span>
      <button
        v-for="platform in mediaPlatforms"
        :key="platform.key"
        class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-mono transition-all cursor-pointer"
        :class="[
          modelValue.mediaPlatforms.includes(platform.key)
            ? 'border-blue-500 bg-blue-600/30 text-white font-semibold'
            : (mediaStats?.[platform.key] || 0) > 0
              ? 'border-white/10 bg-slate-800/40 text-slate-300 hover:border-blue-500/40'
              : 'border-white/5 bg-slate-900/30 text-slate-500 hover:border-white/20'
        ]"
        :title="`点击筛选支持 ${platform.name} 的节点`"
        @click="toggleMedia(platform.key)"
      >
        <span>{{ platform.icon }}</span>
        <span>{{ platform.name }}</span>
        <span
          class="rounded-full px-1.5 py-0.2 text-[10px]"
          :class="[
            (mediaStats?.[platform.key] || 0) > 0
              ? 'bg-emerald-500/20 text-emerald-400 font-bold'
              : 'bg-white/5 text-slate-500'
          ]"
        >
          {{ mediaStats?.[platform.key] || 0 }}
        </span>
      </button>
    </div>

    <!-- Quick Country / Region Pills -->
    <div v-if="countries && countries.length" class="flex flex-wrap items-center gap-1.5 pt-2 border-t border-white/5">
      <span class="text-xs font-mono text-slate-400 mr-1 flex items-center gap-1">
        <span>🌍</span>
        <span>快捷地区:</span>
      </span>
      <button
        class="rounded-full border px-2.5 py-0.8 text-xs font-mono transition-all cursor-pointer"
        :class="[
          !modelValue.country
            ? 'border-blue-500 bg-blue-600/30 text-white font-semibold'
            : 'border-white/10 bg-slate-800/40 text-slate-400 hover:text-white'
        ]"
        @click="updateField('country', '')"
      >
        全部 ({{ totalCount }})
      </button>
      <button
        v-for="c in countries"
        :key="c.code"
        class="inline-flex items-center gap-1 rounded-full border px-2 py-0.8 text-xs font-mono transition-all cursor-pointer"
        :class="[
          modelValue.country === c.code
            ? 'border-blue-500 bg-blue-600/30 text-white font-semibold'
            : 'border-white/10 bg-slate-800/40 text-slate-300 hover:border-blue-500/30 hover:text-white'
        ]"
        @click="updateField('country', modelValue.country === c.code ? '' : c.code)"
      >
        <span>{{ c.flag }}</span>
        <span>{{ c.name }}</span>
        <span class="text-[10px] text-slate-400">({{ c.count }})</span>
      </button>
    </div>

    <!-- Bottom Row: Speed Threshold, Probe Toggles & Actions -->
    <div class="flex flex-wrap items-center justify-between gap-4 pt-2 border-t border-white/5">
      <!-- Left: Probe options and speed gate -->
      <div class="flex flex-wrap items-center gap-4 text-xs font-mono">
        <label class="inline-flex items-center gap-1.5 text-slate-300 cursor-pointer select-none">
          <input
            type="checkbox"
            :checked="includeSpeed"
            class="rounded border-white/20 bg-slate-900 text-blue-500 focus:ring-0"
            @change="$emit('update:includeSpeed', ($event.target as HTMLInputElement).checked)"
          />
          <span>🚀 包含测速</span>
        </label>

        <label class="inline-flex items-center gap-1.5 text-slate-300 cursor-pointer select-none">
          <input
            type="checkbox"
            :checked="includeMedia"
            class="rounded border-white/20 bg-slate-900 text-blue-500 focus:ring-0"
            @change="$emit('update:includeMedia', ($event.target as HTMLInputElement).checked)"
          />
          <span>🎬 包含流媒体/AI</span>
        </label>

        <div class="inline-flex items-center gap-1.5 text-slate-400">
          <span>门槛:</span>
          <input
            :value="modelValue.minSpeed || ''"
            type="number"
            min="0"
            step="5"
            placeholder="0"
            class="w-16 rounded border border-white/10 bg-slate-950/60 px-2 py-1 text-xs text-white text-center focus:border-blue-500 focus:outline-hidden font-mono"
            @input="updateField('minSpeed', Math.max(0, Number(($event.target as HTMLInputElement).value) || 0))"
          />
          <span>Mbps</span>
        </div>
      </div>

      <!-- Right: Selection actions & View Switcher -->
      <div class="flex items-center gap-3">
        <!-- If items are selected -->
        <div v-if="selectedCount && selectedCount > 0" class="flex items-center gap-2">
          <span class="text-xs font-mono text-blue-400">
            已选 <strong>{{ selectedCount }}</strong> 项
          </span>
          <button
            class="inline-flex items-center gap-1 rounded-lg border border-blue-500/40 bg-blue-600/20 px-3 py-1.5 text-xs font-mono text-blue-400 hover:bg-blue-600/30 transition-colors cursor-pointer"
            :disabled="probing"
            @click="$emit('probe-selected')"
          >
            ⚡ 质检选中
          </button>
          <button
            class="rounded-lg border border-white/10 bg-slate-800/40 px-2.5 py-1.5 text-xs font-mono text-slate-400 hover:text-white transition-colors cursor-pointer"
            @click="$emit('clear-selection')"
          >
            取消选择
          </button>
        </div>

        <!-- If no items selected: quick batch helpers -->
        <div v-else class="flex items-center gap-2">
          <button
            class="rounded-lg border border-white/10 bg-slate-800/40 px-3 py-1.5 text-xs font-mono text-slate-300 hover:bg-slate-800 hover:text-white transition-colors cursor-pointer"
            :disabled="probing || !untestedOrFailedCount"
            title="仅对尚未测试或上次失败的节点发起探测"
            @click="$emit('probe-untested')"
          >
            🎯 仅测未测/失败 ({{ untestedOrFailedCount }})
          </button>
          <button
            class="rounded-lg border border-rose-500/30 bg-rose-500/10 px-2.5 py-1.5 text-xs font-mono text-rose-400 hover:bg-rose-500/20 transition-colors cursor-pointer"
            title="清空所有已持久化的节点测速与解锁记录"
            @click="$emit('clear-probe-data')"
          >
            🗑️ 清除质检
          </button>
        </div>

        <!-- View mode switcher -->
        <div class="flex items-center gap-1 border-l border-white/10 pl-3">
          <button
            :class="[
              'flex h-7 w-7 items-center justify-center rounded-lg border transition-colors cursor-pointer text-xs',
              viewMode === 'table' ? 'border-blue-500/50 bg-blue-600/20 text-blue-400' : 'border-white/10 bg-slate-800/40 text-slate-400 hover:text-white'
            ]"
            title="表格模式"
            @click="$emit('change-view', 'table')"
          >
            📋
          </button>
          <button
            :class="[
              'flex h-7 w-7 items-center justify-center rounded-lg border transition-colors cursor-pointer text-xs',
              viewMode === 'grid' ? 'border-blue-500/50 bg-blue-600/20 text-blue-400' : 'border-white/10 bg-slate-800/40 text-slate-400 hover:text-white'
            ]"
            title="卡片模式"
            @click="$emit('change-view', 'grid')"
          >
            🎴
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
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
