<script setup lang="ts">
import { computed } from 'vue'
import {
  ArrowPathIcon,
  BoltIcon,
  ClockIcon,
} from '@heroicons/vue/24/outline'
import PageToolbar from '../../ui/PageToolbar.vue'
import { SUPPORTED_NODE_PROTOCOLS } from './nodeView'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'

const props = defineProps<{
  total: number
  itemsCount: number
  filteredCount: number
  initialLoaded: boolean
  loading: boolean
  protocolFilter: string
  healthFilter: string
  searchQuery: string
  healthCounts: {
    all: number
    probing: number
    healthy: number
    unhealthy: number
    untested: number
  }
  isAllVisibleSelected: boolean
  selectedCount: number
  visibleSelectedCount: number
}>()

const emit = defineEmits<{
  (e: 'update:protocolFilter', proto: string): void
  (e: 'update:healthFilter', health: string): void
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
          aria-label="协议筛选"
        >
          <button
            type="button"
            class="btn btn-xs rounded-lg font-mono"
            :class="protocolFilter === 'all' ? 'btn-primary' : 'btn-ghost bg-base-200/70'"
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
            aria-label="按节点状态筛选"
            class="select select-xs sm:select-sm text-xs"
            @change="$emit('update:healthFilter', ($event.target as HTMLSelectElement).value)"
          >
            <option value="all">全部 ({{ healthCounts.all }})</option>
            <option value="probing">检测中 ({{ healthCounts.probing }})</option>
            <option value="healthy">正常 ({{ healthCounts.healthy }})</option>
            <option value="unhealthy">异常 ({{ healthCounts.unhealthy }})</option>
            <option value="untested">未测试 ({{ healthCounts.untested }})</option>
          </select>

          <!-- Search Input -->
          <input
            :value="searchQuery"
            data-testid="node-search-input"
            type="search"
            :placeholder="t('nodes.searchPlaceholder')"
            class="input input-xs sm:input-sm w-full sm:w-60 font-mono text-xs"
            @input="$emit('update:searchQuery', ($event.target as HTMLInputElement).value)"
            @keydown.enter="$emit('search-submit')"
          />
        </div>
      </div>

      <!-- Health Status Quick Filter Capsules -->
      <div
        data-testid="node-health-pills"
        class="flex flex-wrap items-center gap-1.5 text-xs"
        role="group"
        aria-label="节点状态筛选"
      >
        <span class="opacity-60 mr-1 font-medium">{{ uiText('节点状态：', 'Health:') }}</span>
        <button
          type="button"
          data-testid="node-health-pill-all"
          class="btn btn-xs rounded-full"
          :class="healthFilter === 'all' ? 'btn-primary' : 'btn-ghost bg-base-200/70'"
          @click="$emit('update:healthFilter', 'all')"
        >
          全部 ({{ healthCounts.all }})
        </button>
        <button
          type="button"
          data-testid="node-health-pill-probing"
          class="btn btn-xs rounded-full gap-1"
          :class="healthFilter === 'probing' ? 'btn-info' : 'btn-ghost bg-base-200/70'"
          @click="$emit('update:healthFilter', 'probing')"
        >
          <span
            class="w-1.5 h-1.5 rounded-full bg-current"
            :class="{ 'animate-ping': healthCounts.probing > 0 }"
          />
          <span>检测中 ({{ healthCounts.probing }})</span>
        </button>
        <button
          type="button"
          data-testid="node-health-pill-healthy"
          class="btn btn-xs rounded-full"
          :class="healthFilter === 'healthy' ? 'btn-success' : 'btn-ghost bg-base-200/70'"
          @click="$emit('update:healthFilter', 'healthy')"
        >
          正常 ({{ healthCounts.healthy }})
        </button>
        <button
          type="button"
          data-testid="node-health-pill-unhealthy"
          class="btn btn-xs rounded-full"
          :class="healthFilter === 'unhealthy' ? 'btn-error' : 'btn-ghost bg-base-200/70'"
          @click="$emit('update:healthFilter', 'unhealthy')"
        >
          异常 ({{ healthCounts.unhealthy }})
        </button>
        <button
          type="button"
          data-testid="node-health-pill-untested"
          class="btn btn-xs rounded-full"
          :class="healthFilter === 'untested' ? 'btn-neutral' : 'btn-ghost bg-base-200/70'"
          @click="$emit('update:healthFilter', 'untested')"
        >
          未测试 ({{ healthCounts.untested }})
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
