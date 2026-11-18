<script setup lang="ts">
import { computed } from 'vue'
import {
  ArrowPathIcon,
  BoltIcon,
  ClockIcon,
} from '@heroicons/vue/24/outline'
import PageToolbar from '../../ui/PageToolbar.vue'
import { SUPPORTED_NODE_PROTOCOLS } from './nodeView'
import { IP_RISK_BANDS, IP_RISK_BAND_LABELS } from '../policy/policyTypes'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'
import { toastStore } from '../../ui/toast'
import { parseSpeedMinimum } from './useNodes'

const props = defineProps<{
  total: number
  itemsCount: number
  filteredCount: number
  initialLoaded: boolean
  loading: boolean
  protocolFilter: string
  healthFilter: string
  riskBandFilter?: string
  riskScoreMax?: string
  speedMin?: string
  searchQuery: string
  healthCounts: {
    all: number
    probing: number
    healthy: number
    unhealthy: number
    unknown: number
    untested?: number
  }
  isAllVisibleSelected: boolean
  selectedCount: number
  visibleSelectedCount: number
}>()

const emit = defineEmits<{
  (e: 'update:protocolFilter', proto: string): void
  (e: 'update:healthFilter', health: string): void
  (e: 'update:riskBandFilter', band: string): void
  (e: 'update:riskScoreMax', score: string): void
  (e: 'update:speedMin', speed: string): void
  (e: 'reset-filters'): void
  (e: 'update:searchQuery', query: string): void
  (e: 'refresh'): void
  (e: 'search-submit'): void
  (e: 'toggle-select-all-visible'): void
  (e: 'clear-selection'): void
  (e: 'clear-invisible-selections'): void
  (e: 'probe-selected'): void
  (e: 'probe-all'): void
  (e: 'open-history'): void
}>()

function changeSpeedMinimum(event: Event) {
  const input = event.target as HTMLInputElement
  const speed = parseSpeedMinimum(input.value)
  if (input.validity.badInput || (input.value !== '' && speed === '')) {
    input.value = props.speedMin ?? ''
    toastStore.push({ tone: 'error', message: uiText('请输入大于或等于 0 的有限测速值。', 'Enter a finite speed of zero or more.') })
    return
  }
  input.value = speed
  emit('update:speedMin', speed)
}

const totalBadgeText = computed(() => {
  if (!props.initialLoaded && props.loading) {
    return '--'
  }
  if (props.healthFilter === 'all') {
    if (props.itemsCount < props.total) {
      return `${props.itemsCount} / ${props.total.toLocaleString()} ${t('nodes.totalNodes')}`
    }
    return `${props.total.toLocaleString()} ${t('nodes.totalNodes')}`
  }
  return `${props.filteredCount} / ${props.total.toLocaleString()} ${t('nodes.totalNodes')}`
})
</script>

<template>
  <div class="space-y-4">
    <!-- Top Level Page Toolbar (Replaces redundant big banner) -->
    <PageToolbar>
      <template #default>
        <div class="flex items-center gap-2">
          <h2 class="sr-only">{{ uiText('节点管理与测速', 'Node Management & Probing') }}</h2>
          <span class="badge badge-primary badge-outline text-xs" data-testid="nodes-scope-badge">
            {{ t('nodes.scopeEnabledBadge') }}
          </span>
          <span class="badge badge-ghost text-xs font-mono" data-testid="nodes-total-badge">
            {{ totalBadgeText }}
          </span>
        </div>
      </template>

      <template #actions>
        <button
          class="btn btn-ghost btn-sm btn-square touch-manipulation"
          type="button"
          :title="t('common.refresh')"
          :aria-label="t('common.refresh')"
          @click="$emit('refresh')"
        >
          <ArrowPathIcon class="h-4 w-4" :class="{ 'animate-spin': loading }" />
        </button>
      </template>
    </PageToolbar>

    <!-- Protocol Filter, Health Status Filter & Search Bar -->
    <div class="flex flex-col gap-3">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <!-- Protocol Pills -->
        <div
          data-testid="node-protocol-filter"
          class="flex flex-wrap items-center gap-1.5 text-xs"
          role="group"
          :aria-label="uiText('协议筛选', 'Filter by protocol')"
        >
          <button
            type="button"
            class="btn btn-xs rounded-lg font-mono"
            :class="protocolFilter === 'all' ? 'btn-primary' : 'btn-ghost bg-base-200/70'"
            :aria-pressed="protocolFilter === 'all'"
            @click="$emit('update:protocolFilter', 'all')"
          >
            {{ t('nodes.protocolAll') }}
          </button>
          <button
            v-for="proto in SUPPORTED_NODE_PROTOCOLS"
            :key="proto"
            type="button"
            class="btn btn-xs rounded-lg font-mono uppercase"
            :class="protocolFilter === proto ? 'btn-primary' : 'btn-ghost bg-base-200/70'"
            :aria-pressed="protocolFilter === proto"
            @click="$emit('update:protocolFilter', proto)"
          >
            {{ proto.toUpperCase() }}
          </button>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <!-- Health Select Dropdown -->
          <select
            :value="healthFilter"
            data-testid="node-health-filter"
            :aria-label="uiText('按节点状态筛选', 'Filter by node health')"
            class="select select-xs sm:select-sm text-xs"
            @change="$emit('update:healthFilter', ($event.target as HTMLSelectElement).value)"
          >
            <option value="all">全部</option>
            <option value="probing">检测中</option>
            <option value="healthy">正常</option>
            <option value="unhealthy">异常</option>
            <option value="unknown">未知</option>
          </select>

          <!-- Search Input -->
          <label class="w-full sm:w-60">
            <span class="sr-only">{{ uiText('搜索节点', 'Search nodes') }}</span>
            <input
              :value="searchQuery"
              data-testid="node-search-input"
              type="search"
              :placeholder="t('nodes.searchPlaceholder')"
              class="input input-xs sm:input-sm w-full font-mono text-xs"
              @input="$emit('update:searchQuery', ($event.target as HTMLInputElement).value)"
              @keydown.enter="$emit('search-submit')"
            />
          </label>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-2 text-xs" data-testid="node-risk-filters">
        <label class="flex items-center gap-2">
          <span>IP 风险</span>
          <select :value="riskBandFilter || 'all'" data-testid="node-risk-band-filter" aria-label="IP 风险等级" class="select select-xs sm:select-sm" @change="$emit('update:riskBandFilter', ($event.target as HTMLSelectElement).value)">
            <option value="all">全部风险等级</option>
            <option v-for="band in IP_RISK_BANDS" :key="band" :value="band">{{ IP_RISK_BAND_LABELS[band] }}</option>
          </select>
        </label>
        <label class="flex items-center gap-2">
          <span>分数上限</span>
          <input :value="riskScoreMax" type="number" min="0" max="100" step="1" placeholder="不限" class="input input-xs sm:input-sm w-24" data-testid="node-risk-score-max" aria-label="风险分数上限" @change="$emit('update:riskScoreMax', ($event.target as HTMLInputElement).value)" />
        </label>
        <label class="flex items-center gap-2">
          <span>{{ uiText('最低测速 (KiB/s)', 'Minimum speed (KiB/s)') }}</span>
          <input
            :value="speedMin" type="number" min="0" step="any" :placeholder="uiText('不限', 'Any')"
            class="input input-xs sm:input-sm w-28" data-testid="node-speed-min" aria-describedby="node-speed-filter-help"
            @change="changeSpeedMinimum"
          />
        </label>
        <span id="node-speed-filter-help" class="text-base-content/60">{{ uiText('留空不限。设置下限只筛选当前连接已保存的最新测速；失败、失效或未测速的节点不匹配，不会发起测速。', 'Leave blank for any speed. A minimum uses only the latest saved speed for this connection; failed, stale or untested nodes do not match. No test is started.') }}</span>
        <button type="button" class="btn btn-ghost btn-xs" data-testid="node-reset-filters" @click="$emit('reset-filters')">重置筛选</button>
        <span class="text-base-content/60">未知表示当前无有效风险分数；分数上限不匹配未知。</span>
      </div>

      <!-- Health Status Quick Filter Capsules -->
      <div
        data-testid="node-health-pills"
        class="flex flex-wrap items-center gap-1.5 text-xs"
        role="group"
        :aria-label="uiText('节点状态筛选', 'Filter by node health')"
      >
        <span class="opacity-60 mr-1 font-medium">{{ uiText('节点状态：', 'Health:') }}</span>
        <button
          type="button"
          data-testid="node-health-pill-all"
          class="btn btn-xs rounded-full"
          :class="healthFilter === 'all' ? 'btn-primary' : 'btn-ghost bg-base-200/70'"
          :aria-pressed="healthFilter === 'all'"
          @click="$emit('update:healthFilter', 'all')"
        >
          全部
        </button>
        <button
          type="button"
          data-testid="node-health-pill-probing"
          class="btn btn-xs rounded-full gap-1"
          :class="healthFilter === 'probing' ? 'btn-info' : 'btn-ghost bg-base-200/70'"
          :aria-pressed="healthFilter === 'probing'"
          @click="$emit('update:healthFilter', 'probing')"
        >
          <span
            class="w-1.5 h-1.5 rounded-full bg-current"
            :class="{ 'animate-ping': healthCounts.probing > 0 }"
          />
          <span>检测中</span>
        </button>
        <button
          type="button"
          data-testid="node-health-pill-healthy"
          class="btn btn-xs rounded-full"
          :class="healthFilter === 'healthy' ? 'btn-success' : 'btn-ghost bg-base-200/70'"
          :aria-pressed="healthFilter === 'healthy'"
          @click="$emit('update:healthFilter', 'healthy')"
        >
          正常
        </button>
        <button
          type="button"
          data-testid="node-health-pill-unhealthy"
          class="btn btn-xs rounded-full"
          :class="healthFilter === 'unhealthy' ? 'btn-error' : 'btn-ghost bg-base-200/70'"
          :aria-pressed="healthFilter === 'unhealthy'"
          @click="$emit('update:healthFilter', 'unhealthy')"
        >
          异常
        </button>
        <button
          type="button"
          data-testid="node-health-pill-unknown"
          class="btn btn-xs rounded-full"
          :class="healthFilter === 'unknown' || healthFilter === 'untested' ? 'btn-neutral' : 'btn-ghost bg-base-200/70'"
          :aria-pressed="healthFilter === 'unknown' || healthFilter === 'untested'"
          @click="$emit('update:healthFilter', 'unknown')"
        >
          未知
        </button>
      </div>
    </div>

    <!-- Probe Actions & Selection Control Toolbar -->
    <div
      data-testid="node-selection-toolbar"
      class="flex flex-wrap items-center justify-between gap-3 p-3 rounded-2xl bg-base-200/80 backdrop-blur-xl border border-base-300 shadow-sm"
    >
      <div class="flex flex-wrap items-center gap-2.5">
        <label class="flex items-center gap-1.5 cursor-pointer text-xs font-semibold select-none">
          <input
            type="checkbox"
            data-testid="select-all-visible-checkbox"
            class="checkbox checkbox-primary checkbox-xs sm:checkbox-sm"
            :checked="isAllVisibleSelected"
            @change="$emit('toggle-select-all-visible')"
          />
          <span class="hidden sm:inline">{{ t('nodes.selectAllVisible') }}</span>
        </label>

        <!-- Selected Count Badge with Visible Count breakdown -->
        <span
          data-testid="selected-count-badge"
          class="badge badge-sm font-mono text-xs"
          :class="selectedCount > 0 ? 'badge-primary' : 'badge-ghost'"
        >
          {{ t('nodes.selectedCount', { count: selectedCount }) }}
          <span v-if="selectedCount > 0 && selectedCount !== visibleSelectedCount" class="opacity-80 font-normal ml-1">
            ({{ uiText('当前可见', 'Visible') }}: {{ visibleSelectedCount }})
          </span>
        </span>

        <!-- Clear Selection Button -->
        <button
          v-if="selectedCount > 0"
          type="button"
          data-testid="clear-selection-btn"
          class="btn btn-ghost btn-xs text-base-content/60 hover:text-base-content"
          @click="$emit('clear-selection')"
        >
          {{ t('nodes.clearSelection') }}
        </button>

        <!-- Clear Invisible Selections Button -->
        <button
          v-if="selectedCount > visibleSelectedCount"
          type="button"
          data-testid="clear-invisible-selection-btn"
          class="btn btn-ghost btn-xs text-warning hover:text-warning"
          :title="uiText('清除不在当前页面或被筛选隐藏的选中节点', 'Clear selections outside current view')"
          @click="$emit('clear-invisible-selections')"
        >
          {{ uiText('清除不可见项', 'Clear Invisible') }} ({{ selectedCount - visibleSelectedCount }})
        </button>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <!-- Probe Selected Button (Disabled when selectedCount === 0) -->
        <button
          type="button"
          data-testid="probe-selected-btn"
          class="btn btn-primary btn-xs sm:btn-sm gap-1.5 shadow-sm"
          :disabled="selectedCount === 0"
          :title="selectedCount === 0 ? t('nodes.noNodeSelected') : t('nodes.probeSelected')"
          @click="$emit('probe-selected')"
        >
          <BoltIcon class="w-3.5 h-3.5 sm:w-4 sm:h-4" />
          <span>{{ t('nodes.probeSelected') }}</span>
          <span v-if="selectedCount > 0" class="badge badge-xs badge-neutral font-mono">
            {{ selectedCount }}
          </span>
        </button>

        <!-- Probe All (Full fleet) Button with Explicit Confirm -->
        <button
          type="button"
          data-testid="probe-all-btn"
          class="btn btn-outline btn-warning btn-xs sm:btn-sm gap-1.5"
          @click="$emit('probe-all')"
        >
          <BoltIcon class="w-3.5 h-3.5 sm:w-4 sm:h-4" />
          <span>{{ t('nodes.probeAll') }}</span>
        </button>

        <!-- Probe Runs & Batch History Drawer Button -->
        <button
          type="button"
          data-testid="node-probe-history-btn"
          class="btn btn-ghost btn-xs sm:btn-sm gap-1.5 font-normal text-base-content/80 hover:bg-base-300/60"
          @click="$emit('open-history')"
        >
          <ClockIcon class="w-3.5 h-3.5 sm:w-4 sm:h-4" />
          <span>{{ t('nodes.probeHistory') }}</span>
        </button>
      </div>
    </div>
  </div>
</template>
