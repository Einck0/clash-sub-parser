<script setup lang="ts">
import { computed } from 'vue'
import {
  DocumentDuplicateIcon,
  ServerStackIcon,
  BoltIcon,
  CpuChipIcon,
  ShieldCheckIcon,
  ExclamationTriangleIcon,
} from '@heroicons/vue/24/outline'
import { t, uiText } from '../../locales'
import type { DashboardMetrics } from './dashboardTypes'

const props = defineProps<{ metrics: DashboardMetrics }>()
const emit = defineEmits<{ (e: 'navigate', routeName: string): void }>()
const cards = computed(() => [
  { key: 'subscriptions' as const, route: 'subscriptions', icon: DocumentDuplicateIcon,
    label: uiText('订阅源', 'Subscriptions'), description: uiText('已配置上游源', 'Configured upstreams') },
  { key: 'nodes' as const, route: 'nodes', icon: ServerStackIcon,
    label: uiText('节点总数', 'Nodes'), description: uiText('非探针健康', 'Not probe health') },
  { key: 'policies' as const, route: 'policy', icon: CpuChipIcon,
    label: uiText('策略组', 'Policy groups'), description: uiText('分流策略拓扑组', 'Policy routing groups') },
  { key: 'rules' as const, route: 'rules', icon: ShieldCheckIcon,
    label: uiText('分流规则', 'Routing Rules'), description: uiText('域名与IP分流规则', 'Domain & IP rules') },
  { key: 'probes' as const, route: 'nodes', icon: BoltIcon,
    label: uiText('探针批次', 'Probe runs'), description: uiText('历史批次 (非节点健康)', 'History (not health)') },
].map(card => ({ ...card, metric: props.metrics[card.key] })))
</script>

<template>
  <div class="grid grid-cols-2 md:grid-cols-3 xl:grid-cols-5 gap-3 sm:gap-4" data-testid="metric-summary-grid">
    <button
      v-for="card in cards"
      :key="card.key"
      type="button"
      class="min-w-0 rounded-xl bg-base-200/60 border border-base-content/10 shadow-sm p-4 sm:p-5 flex flex-col items-stretch text-left gap-2 hover:border-primary/50 hover:bg-base-200 focus-visible:outline-2 focus-visible:outline-primary focus-visible:outline-offset-2 transition-colors touch-manipulation group"
      :data-testid="`metric-card-${card.key}`"
      :aria-busy="card.metric.status === 'loading'"
      @click="emit('navigate', card.route)"
    >
      <span class="flex items-start justify-between gap-2">
        <span class="text-xs font-medium text-base-content/70 break-words">{{ card.label }}</span>
        <component :is="card.icon" class="w-5 h-5 shrink-0 text-primary" aria-hidden="true" />
      </span>
      <span class="flex flex-wrap items-baseline gap-2 min-h-9" aria-live="polite">
        <span v-if="card.metric.status === 'loading' || card.metric.status === 'idle'"
          class="loading loading-spinner loading-sm text-primary" :data-testid="`metric-loading-${card.key}`">
          <span class="sr-only">{{ uiText('正在加载', 'Loading') }}</span>
        </span>
        <span v-else-if="card.metric.status === 'error' && !card.metric.isStale"
          class="flex items-center gap-1 text-error text-xs font-medium" :data-testid="`metric-error-${card.key}`">
          <ExclamationTriangleIcon class="w-4 h-4 shrink-0" aria-hidden="true" />
          {{ uiText('数据不可用', 'Unavailable') }}
        </span>
        <template v-else>
          <span class="text-2xl sm:text-3xl font-bold font-mono tracking-tight text-base-content" :data-testid="`metric-value-${card.key}`">
            {{ card.metric.value }}
          </span>
          <span v-if="card.metric.isStale" class="badge badge-warning badge-xs" :data-testid="`metric-stale-${card.key}`">
            {{ uiText('缓存', 'Stale') }}
          </span>
        </template>
      </span>
      <span v-if="card.metric.error" class="text-[11px] text-error whitespace-pre-wrap break-words [overflow-wrap:anywhere]" :data-testid="`metric-error-detail-${card.key}`">
        {{ card.metric.isStale ? uiText('刷新失败，保留先前值：', 'Refresh failed; showing the previous count: ') : '' }}{{ card.metric.error }}
      </span>

      <span v-if="card.key === 'nodes'" class="flex flex-col items-start gap-1 text-xs text-base-content/65" data-testid="metric-active-nodes" aria-live="polite">
        <span v-if="metrics.activeNodes.status === 'loading' || metrics.activeNodes.status === 'idle'" data-testid="metric-loading-activeNodes">
          {{ uiText('活跃子集：加载中…', 'Active subset: loading…') }}
        </span>
        <span v-else-if="metrics.activeNodes.value !== null" class="flex flex-wrap items-center gap-1">
          {{ uiText('活跃子集', 'Active subset') }} <span data-testid="metric-value-activeNodes">{{ metrics.activeNodes.value }}</span>
          <span v-if="metrics.activeNodes.isStale" class="badge badge-warning badge-xs" data-testid="metric-stale-activeNodes">{{ uiText('缓存', 'Stale') }}</span>
        </span>
        <span v-else class="text-warning" data-testid="metric-error-activeNodes">{{ uiText('活跃子集：数据不可用', 'Active subset: unavailable') }}</span>
        <span v-if="metrics.activeNodes.error" class="text-warning whitespace-pre-wrap break-words [overflow-wrap:anywhere]" data-testid="metric-error-detail-activeNodes">{{ metrics.activeNodes.error }}</span>
        <span class="text-[10px] text-primary break-words">{{ t('dashboard.nodesScopeEnabled') }}</span>
      </span>
      <span class="mt-auto text-[11px] text-base-content/60 break-words">{{ card.description }}</span>
    </button>
  </div>
</template>
