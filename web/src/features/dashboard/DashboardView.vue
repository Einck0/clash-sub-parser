<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  DocumentDuplicateIcon,
  ServerStackIcon,
  BoltIcon,
  CpuChipIcon,
  ArrowDownTrayIcon,
  Cog6ToothIcon,
  CheckCircleIcon,
  ExclamationTriangleIcon,
  ArrowPathIcon,
  PlusIcon,
  SparklesIcon,
} from '@heroicons/vue/24/outline'
import { api } from '../../api/client'
import { t } from '../../locales'

const router = useRouter()

const stats = ref({
  subscriptions: 0,
  nodes: 0,
  probes: 0,
  policies: 0,
})

const loadingStats = ref(true)
const healthStatus = ref<'checking' | 'healthy' | 'unhealthy'>('checking')
const healthLatency = ref<number | null>(null)
const refreshingHealth = ref(false)

async function checkHealth() {
  refreshingHealth.value = true
  const start = Date.now()
  try {
    await api.get('/healthz')
    healthStatus.value = 'healthy'
    healthLatency.value = Date.now() - start
  } catch {
    healthStatus.value = 'unhealthy'
    healthLatency.value = null
  } finally {
    refreshingHealth.value = false
  }
}

async function loadStats() {
  loadingStats.value = true
  try {
    const [subsRes, nodesRes, probesRes, policyRes] = await Promise.allSettled([
      api.get<{ total: number }>('/api/v1/subscriptions', { params: { page: 1, page_size: 1 } }),
      api.get<{ total: number }>('/api/v1/nodes', { params: { page: 1, page_size: 1 } }),
      api.get<{ total: number }>('/api/v1/probes/runs', { params: { page: 1, page_size: 1 } }),
      api.get<{ total: number }>('/api/v1/policies/groups', { params: { page: 1, page_size: 1 } }),
    ])

    if (subsRes.status === 'fulfilled') stats.value.subscriptions = subsRes.value?.total || 0
    if (nodesRes.status === 'fulfilled') stats.value.nodes = nodesRes.value?.total || 0
    if (probesRes.status === 'fulfilled') stats.value.probes = probesRes.value?.total || 0
    if (policyRes.status === 'fulfilled') stats.value.policies = policyRes.value?.total || 0
  } catch {
    // Graceful fallback
  } finally {
    loadingStats.value = false
  }
}

function navigateTo(routeName: string) {
  router.push({ name: routeName })
}

onMounted(() => {
  checkHealth()
  loadStats()
})
</script>

<template>
  <div class="space-y-6">
    <!-- Header Banner -->
    <div class="flex flex-wrap items-center justify-between gap-4">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.2em] text-primary">
          {{ t('dashboard.tag') }}
        </p>
        <h2 class="mt-1 text-2xl sm:text-3xl font-bold tracking-tight text-base-content">
          {{ t('dashboard.title') }}
        </h2>
        <p class="mt-1 text-xs sm:text-sm text-base-content/70">
          {{ t('dashboard.subtitle') }}
        </p>
      </div>

      <div class="flex items-center gap-2">
        <button
          type="button"
          class="btn btn-ghost btn-sm gap-1.5 touch-manipulation hover:bg-base-300/60"
          :disabled="refreshingHealth"
          @click="checkHealth"
        >
          <ArrowPathIcon class="w-4 h-4" :class="{ 'animate-spin': refreshingHealth }" />
          <span class="text-xs">{{ t('dashboard.refreshHealth') }}</span>
        </button>
      </div>
    </div>

    <!-- Health Telemetry Glass Card -->
    <div
      class="p-4 sm:p-5 rounded-2xl bg-base-200/70 backdrop-blur-xl border border-white/10 shadow-lg flex flex-wrap items-center justify-between gap-4 min-w-0"
    >
      <div class="flex items-center gap-3 min-w-0">
        <div
          class="w-3.5 h-3.5 rounded-full ring-4 shrink-0 transition-colors"
          :class="{
            'bg-success ring-success/20 animate-pulse': healthStatus === 'healthy',
            'bg-error ring-error/20': healthStatus === 'unhealthy',
            'bg-warning ring-warning/20 animate-bounce': healthStatus === 'checking',
          }"
        />
        <div class="min-w-0">
          <h3 class="text-sm font-semibold text-base-content truncate">{{ t('dashboard.healthTitle') }}</h3>
          <p class="text-xs text-base-content/60 truncate">
            {{
              healthStatus === 'healthy'
                ? `${t('dashboard.healthy')} (${healthLatency !== null ? `${healthLatency}ms` : '<10ms'})`
                : healthStatus === 'unhealthy'
                ? t('dashboard.unhealthy')
                : t('dashboard.checking')
            }}
          </p>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-2 text-xs font-mono text-base-content/60">
        <span
          data-testid="telemetry-badge-engine"
          class="badge badge-neutral font-mono h-auto min-h-[1.75rem] py-1 px-2.5 text-xs leading-normal whitespace-normal inline-flex items-center"
        >Go 1.22+ Control Plane</span>
        <span
          data-testid="telemetry-badge-storage"
          class="badge badge-primary badge-outline font-mono h-auto min-h-[1.75rem] py-1 px-2.5 text-xs leading-normal whitespace-normal inline-flex items-center"
        >Chi v5 & SQLite WAL</span>
      </div>
    </div>

    <!-- Stats Grid -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-3 sm:gap-4">
      <div
        class="card bg-base-200/60 backdrop-blur-xl border border-white/10 shadow-sm hover:border-primary/40 transition-all cursor-pointer group"
        @click="navigateTo('subscriptions')"
      >
        <div class="card-body p-4 sm:p-5 gap-2">
          <div class="flex items-center justify-between">
            <span class="text-xs font-medium text-base-content/70">{{ t('dashboard.subscriptionsCount') }}</span>
            <DocumentDuplicateIcon class="w-5 h-5 text-primary group-hover:scale-110 transition-transform" />
          </div>
          <div class="text-2xl sm:text-3xl font-bold font-mono tracking-tight text-base-content">
            {{ loadingStats ? '--' : stats.subscriptions }}
          </div>
        </div>
      </div>

      <div
        class="card bg-base-200/60 backdrop-blur-xl border border-white/10 shadow-sm hover:border-primary/40 transition-all cursor-pointer group"
        @click="navigateTo('nodes')"
      >
        <div class="card-body p-4 sm:p-5 gap-2">
          <div class="flex items-center justify-between">
            <span class="text-xs font-medium text-base-content/70">{{ t('dashboard.nodesCount') }}</span>
            <ServerStackIcon class="w-5 h-5 text-secondary group-hover:scale-110 transition-transform" />
          </div>
          <div class="text-2xl sm:text-3xl font-bold font-mono tracking-tight text-base-content">
            {{ loadingStats ? '--' : stats.nodes }}
          </div>
        </div>
      </div>

      <div
        class="card bg-base-200/60 backdrop-blur-xl border border-white/10 shadow-sm hover:border-primary/40 transition-all cursor-pointer group"
        @click="navigateTo('probes')"
      >
        <div class="card-body p-4 sm:p-5 gap-2">
          <div class="flex items-center justify-between">
            <span class="text-xs font-medium text-base-content/70">{{ t('dashboard.probeRunsCount') }}</span>
            <BoltIcon class="w-5 h-5 text-warning group-hover:scale-110 transition-transform" />
          </div>
          <div class="text-2xl sm:text-3xl font-bold font-mono tracking-tight text-base-content">
            {{ loadingStats ? '--' : stats.probes }}
          </div>
        </div>
      </div>

      <div
        class="card bg-base-200/60 backdrop-blur-xl border border-white/10 shadow-sm hover:border-primary/40 transition-all cursor-pointer group"
        @click="navigateTo('policy')"
      >
        <div class="card-body p-4 sm:p-5 gap-2">
          <div class="flex items-center justify-between">
            <span class="text-xs font-medium text-base-content/70">{{ t('dashboard.policiesCount') }}</span>
            <CpuChipIcon class="w-5 h-5 text-info group-hover:scale-110 transition-transform" />
          </div>
          <div class="text-2xl sm:text-3xl font-bold font-mono tracking-tight text-base-content">
            {{ loadingStats ? '--' : stats.policies }}
          </div>
        </div>
      </div>
    </div>

    <!-- Quick Action Channels -->
    <div class="card bg-base-200/60 backdrop-blur-xl border border-white/10 shadow-sm">
      <div class="card-body p-5 sm:p-6 space-y-4">
        <div class="flex items-center gap-2">
          <SparklesIcon class="w-5 h-5 text-primary" />
          <h3 class="text-base font-bold text-base-content">{{ t('dashboard.quickActionsTitle') }}</h3>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
          <button
            type="button"
            class="btn btn-outline btn-primary btn-sm sm:btn-md justify-start gap-2.5 font-semibold text-xs touch-manipulation hover:shadow-md"
            @click="navigateTo('subscriptions')"
          >
            <PlusIcon class="w-4 h-4 shrink-0" />
            <span>{{ t('dashboard.addSubscription') }}</span>
          </button>

          <button
            type="button"
            class="btn btn-outline btn-secondary btn-sm sm:btn-md justify-start gap-2.5 font-semibold text-xs touch-manipulation hover:shadow-md"
            @click="navigateTo('probes')"
          >
            <BoltIcon class="w-4 h-4 shrink-0" />
            <span>{{ t('dashboard.runProbe') }}</span>
          </button>

          <button
            type="button"
            class="btn btn-outline btn-accent btn-sm sm:btn-md justify-start gap-2.5 font-semibold text-xs touch-manipulation hover:shadow-md"
            @click="navigateTo('publications')"
          >
            <ArrowDownTrayIcon class="w-4 h-4 shrink-0" />
            <span>{{ t('dashboard.exportConfig') }}</span>
          </button>

          <button
            type="button"
            class="btn btn-outline btn-neutral btn-sm sm:btn-md justify-start gap-2.5 font-semibold text-xs touch-manipulation hover:shadow-md"
            @click="navigateTo('settings')"
          >
            <Cog6ToothIcon class="w-4 h-4 shrink-0" />
            <span>{{ t('dashboard.systemSettings') }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
