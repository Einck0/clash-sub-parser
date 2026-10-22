<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  ArrowPathIcon,
} from '@heroicons/vue/24/outline'
import PageToolbar from '../../ui/PageToolbar.vue'
import { t, uiText } from '../../locales'
import { useDashboard } from './useDashboard'
import QuickActions from './QuickActions.vue'
import MetricSummary from './MetricSummary.vue'

const router = useRouter()

const {
  metrics,
  healthStatus,
  healthLatency,
  refreshingHealth,
  isRefreshingStats,
  checkHealth,
  loadStats,
  refreshAll,
} = useDashboard()

function navigateTo(routeName: string): void {
  if (router) {
    router.push({ name: routeName })
  } else if (typeof window !== 'undefined') {
    window.location.hash = `#${routeName}`
  }
}

onMounted(() => {
  // Pure GET telemetry queries only, no mutations or external probes
  checkHealth()
  loadStats()
})
</script>

<template>
  <div class="space-y-4 sm:space-y-5">
    <!-- Page Toolbar: compact header, non-redundant title -->
    <PageToolbar
      :title="t('dashboard.title')"
      :description="t('dashboard.subtitle')"
    >
      <template #actions>
        <button
          type="button"
          class="btn btn-ghost btn-sm gap-1.5 touch-manipulation hover:bg-base-300/60"
          :disabled="isRefreshingStats || refreshingHealth"
          data-testid="refresh-all-btn"
          @click="refreshAll"
        >
          <ArrowPathIcon
            class="w-4 h-4 shrink-0"
            :class="{ 'animate-spin': isRefreshingStats || refreshingHealth }"
          />
          <span class="text-xs">{{ uiText('刷新数据', 'Refresh Data') }}</span>
        </button>
      </template>
    </PageToolbar>

    <!-- Compact Health & Status Telemetry Card -->
    <div
      class="p-3.5 sm:p-4 rounded-xl bg-base-200/60 border border-base-content/10 shadow-sm flex flex-wrap items-center justify-between gap-3 min-w-0"
      data-testid="health-telemetry-card"
    >
      <!-- Left: Service Health -->
      <div class="flex items-center gap-3 min-w-0">
        <div
          class="w-3 h-3 rounded-full shrink-0 transition-colors ring-4"
          :class="{
            'bg-success ring-success/20': healthStatus === 'healthy',
            'bg-error ring-error/20': healthStatus === 'unhealthy',
            'bg-warning ring-warning/20 animate-pulse': healthStatus === 'checking',
          }"
          data-testid="health-status-indicator"
        />
        <div class="min-w-0">
          <div class="flex items-center gap-2">
            <h3 class="text-xs sm:text-sm font-semibold text-base-content truncate">
              {{ t('dashboard.healthTitle') }}
            </h3>
            <!-- Real Latency in ms: no <10ms fallback! -->
            <span
              v-if="healthStatus === 'healthy' && healthLatency !== null"
              class="text-xs font-mono text-success font-medium"
              data-testid="health-latency"
            >
              {{ healthLatency }}ms
            </span>
          </div>
          <p class="text-xs text-base-content/60 truncate" data-testid="health-status-desc">
            {{
              healthStatus === 'healthy'
                ? uiText('核心后端服务在线响应正常', 'Core backend service reachable')
                : healthStatus === 'unhealthy'
                ? uiText('核心服务检测未通过或不可达', 'Health check failed or unreachable')
                : uiText('正在检测服务连通性...', 'Checking service connectivity...')
            }}
          </p>
        </div>
      </div>

      <!-- Right: Topology Status & Health Refresh -->
      <div class="flex flex-wrap items-center gap-2.5 text-xs">
        <!-- Topology Status: Explicitly unvalidated (service health != topology valid) -->
        <div
          class="flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-base-300/50 border border-base-content/10"
          data-testid="topology-status"
          :title="uiText('策略拓扑完整性须在策略组/规则模块完成校验，概览不虚构就绪状态', 'Topology integrity validated in policy desk; not falsely marked ready in overview')"
        >
          <span class="text-base-content/60 font-medium">{{ uiText('拓扑校验:', 'Topology:') }}</span>
          <span class="badge badge-neutral badge-xs font-mono" data-testid="topology-status-badge">
            {{ uiText('未校验', 'Unvalidated') }}
          </span>
        </div>

        <button
          type="button"
          class="btn btn-ghost btn-xs gap-1 touch-manipulation hover:bg-base-300/60"
          :disabled="refreshingHealth"
          data-testid="refresh-health-btn"
          @click="checkHealth"
        >
          <ArrowPathIcon class="w-3.5 h-3.5 shrink-0" :class="{ 'animate-spin': refreshingHealth }" />
          <span class="text-xs">{{ t('dashboard.refreshHealth') }}</span>
        </button>
      </div>
    </div>

    <!-- Quick Actions: prioritized next actions directly below status -->
    <QuickActions @navigate="navigateTo" />

    <!-- Metrics Summary Grid -->
    <MetricSummary :metrics="metrics" @navigate="navigateTo" />
  </div>
</template>
