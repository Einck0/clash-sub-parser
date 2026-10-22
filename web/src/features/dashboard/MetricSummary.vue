<script setup lang="ts">
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

defineProps<{
  metrics: DashboardMetrics
}>()

const emit = defineEmits<{
  (e: 'navigate', routeName: string): void
}>()

function handleClick(routeName: string) {
  emit('navigate', routeName)
}
</script>

<template>
  <div class="grid grid-cols-2 lg:grid-cols-5 gap-3 sm:gap-4" data-testid="metric-summary-grid">
    <!-- Subscriptions Card -->
    <div
      class="card bg-base-200/60 border border-base-content/10 shadow-sm hover:border-primary/40 transition-all cursor-pointer group"
      data-testid="metric-card-subscriptions"
      @click="handleClick('subscriptions')"
    >
      <div class="card-body p-4 sm:p-5 gap-2">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-base-content/70">{{ t('dashboard.subscriptionsCount') }}</span>
          <DocumentDuplicateIcon class="w-5 h-5 text-primary group-hover:scale-110 transition-transform" />
        </div>

        <div class="flex items-baseline gap-2 min-h-[2.25rem]">
          <span
            v-if="metrics.subscriptions.status === 'loading'"
            class="loading loading-spinner loading-sm text-primary"
            data-testid="metric-loading-subscriptions"
          />
          <div
            v-else-if="metrics.subscriptions.status === 'error' && !metrics.subscriptions.isStale"
            class="flex items-center gap-1 text-error text-xs font-medium"
            data-testid="metric-error-subscriptions"
          >
            <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
            <span>{{ uiText('数据不可用', 'Unavailable') }}</span>
          </div>
          <div v-else class="flex items-baseline gap-2">
            <span
              class="text-2xl sm:text-3xl font-bold font-mono tracking-tight text-base-content"
              data-testid="metric-value-subscriptions"
            >
              {{ metrics.subscriptions.value }}
            </span>
            <span
              v-if="metrics.subscriptions.isStale"
              class="badge badge-warning badge-xs"
              data-testid="metric-stale-subscriptions"
              :title="metrics.subscriptions.error || uiText('刷新失败，显示先前值', 'Refresh failed')"
            >
              {{ uiText('缓存', 'Stale') }}
            </span>
          </div>
        </div>

        <div class="text-[11px] text-base-content/50 truncate">
          {{ uiText('已配置上游源', 'Configured upstreams') }}
        </div>
      </div>
    </div>

    <!-- Nodes Card -->
    <div
      class="card bg-base-200/60 border border-base-content/10 shadow-sm hover:border-primary/40 transition-all cursor-pointer group"
      data-testid="metric-card-nodes"
      @click="handleClick('nodes')"
    >
      <div class="card-body p-4 sm:p-5 gap-2">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-base-content/70">{{ t('dashboard.nodesCount') }}</span>
          <ServerStackIcon class="w-5 h-5 text-secondary group-hover:scale-110 transition-transform" />
        </div>

        <div class="flex items-baseline justify-between gap-2 min-h-[2.25rem]">
          <span
            v-if="metrics.nodes.status === 'loading'"
            class="loading loading-spinner loading-sm text-secondary"
            data-testid="metric-loading-nodes"
          />
          <div
            v-else-if="metrics.nodes.status === 'error' && !metrics.nodes.isStale"
            class="flex items-center gap-1 text-error text-xs font-medium"
            data-testid="metric-error-nodes"
          >
            <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
            <span>{{ uiText('数据不可用', 'Unavailable') }}</span>
          </div>
          <div v-else class="flex items-baseline gap-2">
            <span
              class="text-2xl sm:text-3xl font-bold font-mono tracking-tight text-base-content"
              data-testid="metric-value-nodes"
            >
              {{ metrics.nodes.value }}
            </span>
            <span
              v-if="metrics.nodes.isStale"
              class="badge badge-warning badge-xs"
              data-testid="metric-stale-nodes"
              :title="metrics.nodes.error || uiText('刷新失败，显示先前值', 'Refresh failed')"
            >
              {{ uiText('缓存', 'Stale') }}
            </span>
          </div>

          <!-- Active subset badge (NOT probe health) -->
          <div
            v-if="metrics.nodes.status !== 'loading'"
            class="text-xs font-mono text-base-content/65 whitespace-nowrap"
            :title="uiText('当前启用订阅贡献的节点中处于活跃状态的子集，非探针健康检测结果', 'Active subset from enabled subscriptions, not probe health result')"
            data-testid="metric-active-nodes"
          >
            <span v-if="metrics.activeNodes.status === 'loading'">{{ uiText('活跃: ...', 'Active: ...') }}</span>
            <span v-else-if="metrics.activeNodes.status === 'error'" class="text-warning/80">{{ uiText('活跃: 待查', 'Active: N/A') }}</span>
            <span v-else>{{ uiText('活跃', 'Active') }} {{ metrics.activeNodes.value }}</span>
          </div>
        </div>

        <div class="flex items-center justify-between gap-1 pt-0.5">
          <span class="badge badge-xs badge-primary badge-outline font-mono">
            {{ t('dashboard.nodesScopeEnabled') }}
          </span>
          <span class="text-[10px] text-base-content/50 truncate" :title="uiText('活跃数仅指订阅状态，非实时探针健康', 'Active count reflects subscription status, not live probe health')">
            {{ uiText('非探针健康', 'Not probe health') }}
          </span>
        </div>
      </div>
    </div>

    <!-- Policy Groups Card -->
    <div
      class="card bg-base-200/60 border border-base-content/10 shadow-sm hover:border-primary/40 transition-all cursor-pointer group"
      data-testid="metric-card-policies"
      @click="handleClick('policy')"
    >
      <div class="card-body p-4 sm:p-5 gap-2">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-base-content/70">{{ t('dashboard.policiesCount') }}</span>
          <CpuChipIcon class="w-5 h-5 text-info group-hover:scale-110 transition-transform" />
        </div>

        <div class="flex items-baseline gap-2 min-h-[2.25rem]">
          <span
            v-if="metrics.policies.status === 'loading'"
            class="loading loading-spinner loading-sm text-info"
            data-testid="metric-loading-policies"
          />
          <div
            v-else-if="metrics.policies.status === 'error' && !metrics.policies.isStale"
            class="flex items-center gap-1 text-error text-xs font-medium"
            data-testid="metric-error-policies"
          >
            <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
            <span>{{ uiText('数据不可用', 'Unavailable') }}</span>
          </div>
          <div v-else class="flex items-baseline gap-2">
            <span
              class="text-2xl sm:text-3xl font-bold font-mono tracking-tight text-base-content"
              data-testid="metric-value-policies"
            >
              {{ metrics.policies.value }}
            </span>
            <span
              v-if="metrics.policies.isStale"
              class="badge badge-warning badge-xs"
              data-testid="metric-stale-policies"
              :title="metrics.policies.error || uiText('刷新失败，显示先前值', 'Refresh failed')"
            >
              {{ uiText('缓存', 'Stale') }}
            </span>
          </div>
        </div>

        <div class="text-[11px] text-base-content/50 truncate">
          {{ uiText('分流策略拓扑组', 'Policy routing groups') }}
        </div>
      </div>
    </div>

    <!-- Rules Card -->
    <div
      class="card bg-base-200/60 border border-base-content/10 shadow-sm hover:border-primary/40 transition-all cursor-pointer group"
      data-testid="metric-card-rules"
      @click="handleClick('rules')"
    >
      <div class="card-body p-4 sm:p-5 gap-2">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-base-content/70">{{ uiText('分流规则', 'Routing Rules') }}</span>
          <ShieldCheckIcon class="w-5 h-5 text-accent group-hover:scale-110 transition-transform" />
        </div>

        <div class="flex items-baseline gap-2 min-h-[2.25rem]">
          <span
            v-if="metrics.rules.status === 'loading'"
            class="loading loading-spinner loading-sm text-accent"
            data-testid="metric-loading-rules"
          />
          <div
            v-else-if="metrics.rules.status === 'error' && !metrics.rules.isStale"
            class="flex items-center gap-1 text-error text-xs font-medium"
            data-testid="metric-error-rules"
          >
            <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
            <span>{{ uiText('数据不可用', 'Unavailable') }}</span>
          </div>
          <div v-else class="flex items-baseline gap-2">
            <span
              class="text-2xl sm:text-3xl font-bold font-mono tracking-tight text-base-content"
              data-testid="metric-value-rules"
            >
              {{ metrics.rules.value }}
            </span>
            <span
              v-if="metrics.rules.isStale"
              class="badge badge-warning badge-xs"
              data-testid="metric-stale-rules"
              :title="metrics.rules.error || uiText('刷新失败，显示先前值', 'Refresh failed')"
            >
              {{ uiText('缓存', 'Stale') }}
            </span>
          </div>
        </div>

        <div class="text-[11px] text-base-content/50 truncate">
          {{ uiText('域名与IP分流规则', 'Domain & IP rules') }}
        </div>
      </div>
    </div>

    <!-- Probe Batches Card -->
    <div
      class="card bg-base-200/60 border border-base-content/10 shadow-sm hover:border-primary/40 transition-all cursor-pointer group col-span-2 sm:col-span-1"
      data-testid="metric-card-probes"
      @click="handleClick('nodes')"
    >
      <div class="card-body p-4 sm:p-5 gap-2">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-base-content/70">{{ t('dashboard.probeRunsCount') }}</span>
          <BoltIcon class="w-5 h-5 text-warning group-hover:scale-110 transition-transform" />
        </div>

        <div class="flex items-baseline gap-2 min-h-[2.25rem]">
          <span
            v-if="metrics.probes.status === 'loading'"
            class="loading loading-spinner loading-sm text-warning"
            data-testid="metric-loading-probes"
          />
          <div
            v-else-if="metrics.probes.status === 'error' && !metrics.probes.isStale"
            class="flex items-center gap-1 text-error text-xs font-medium"
            data-testid="metric-error-probes"
          >
            <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
            <span>{{ uiText('数据不可用', 'Unavailable') }}</span>
          </div>
          <div v-else class="flex items-baseline gap-2">
            <span
              class="text-2xl sm:text-3xl font-bold font-mono tracking-tight text-base-content"
              data-testid="metric-value-probes"
            >
              {{ metrics.probes.value }}
            </span>
            <span
              v-if="metrics.probes.isStale"
              class="badge badge-warning badge-xs"
              data-testid="metric-stale-probes"
              :title="metrics.probes.error || uiText('刷新失败，显示先前值', 'Refresh failed')"
            >
              {{ uiText('缓存', 'Stale') }}
            </span>
          </div>
        </div>

        <div class="text-[11px] text-base-content/50 truncate" :title="uiText('探针历史批次，非当前节点实时健康状态', 'Historical probe runs, not live node health')">
          {{ uiText('历史批次 (非节点健康)', 'History (not health)') }}
        </div>
      </div>
    </div>
  </div>
</template>
