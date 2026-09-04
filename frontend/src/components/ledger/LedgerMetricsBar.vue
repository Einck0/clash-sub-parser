<template>
  <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 sm:gap-4 mb-6">
    <!-- Total Nodes Metric -->
    <MetricCard
      label="TOTAL NODES"
      :value="total"
      :active="activeFilter?.status === 'all' && (!activeFilter?.minSpeed) && activeFilter?.chain === 'all'"
      :clickable="true"
      status="info"
      title="点击重置全部筛选"
      @click="$emit('filter-metric', 'all')"
    >
      <template #value-extra v-if="filteredCount != null && filteredCount !== total">
        <span class="text-xs font-mono tabular-nums text-text-muted">
          (匹配 {{ filteredCount }})
        </span>
      </template>
      <template #subtext>
        <span class="truncate">
          {{ subCount ? `${subCount} 订阅 · ` : '' }}{{ protoCount ? `${protoCount} 协议` : '全部已登记' }}
        </span>
      </template>
    </MetricCard>

    <!-- Healthy Online Metric -->
    <MetricCard
      label="HEALTHY (ONLINE)"
      :value="healthy"
      :active="activeFilter?.status === 'ok'"
      :clickable="true"
      status="success"
      title="点击快速筛选在线正常节点"
      @click="$emit('filter-metric', 'healthy')"
    >
      <template #value-extra>
        <span class="text-xs font-mono tabular-nums text-text-muted">
          {{ testedCount ? `${Math.round((healthy / testedCount) * 100)}%` : '未测' }}
        </span>
      </template>
      <template #subtext>
        <span class="truncate">
          {{ testedCount ? `${testedCount} 已测` : '尚未质检' }}{{ avgLatency ? ` · 均延 ${avgLatency}ms` : '' }}
        </span>
      </template>
    </MetricCard>

    <!-- Fast Nodes Metric -->
    <MetricCard
      label="FAST (> 10 Mbps)"
      :value="fast"
      :active="(activeFilter?.minSpeed ?? 0) > 0"
      :clickable="true"
      status="info"
      title="点击快速筛选高速可用节点 (>=10Mbps)"
      @click="$emit('filter-metric', 'fast')"
    >
      <template #value-extra v-if="maxSpeed">
        <span class="text-xs font-mono tabular-nums text-text-muted">
          峰值 {{ maxSpeed }}M
        </span>
      </template>
      <template #subtext>
        <span class="truncate">
          {{ (activeFilter?.minSpeed ?? 0) > 0 ? '已生效高速门槛' : '点击筛选高速节点' }}
        </span>
      </template>
    </MetricCard>

    <!-- Proxy Chained Metric -->
    <MetricCard
      label="PROXY CHAINED"
      :value="chained"
      :active="activeFilter?.chain === 'chained'"
      :clickable="true"
      status="warning"
      title="点击快速筛选已配置跳板链路节点"
      @click="$emit('filter-metric', 'chained')"
    >
      <template #value-extra>
        <span class="text-xs font-mono tabular-nums text-text-muted">
          / {{ total - chained }} 直连
        </span>
      </template>
      <template #subtext>
        <span class="truncate">
          {{ activeFilter?.chain === 'chained' ? '仅看已挂跳板' : '点击筛选挂链节点' }}
        </span>
      </template>
    </MetricCard>
  </div>
</template>

<script setup lang="ts">
import MetricCard from '../ui/MetricCard.vue'

withDefaults(
  defineProps<{
    total?: number
    filteredCount?: number
    healthy?: number
    testedCount?: number
    avgLatency?: number | null
    fast?: number
    maxSpeed?: number | null
    chained?: number
    subCount?: number
    protoCount?: number
    activeFilter?: {
      status?: string
      minSpeed?: number
      chain?: string
    }
  }>(),
  {
    total: 0,
    healthy: 0,
    fast: 0,
    chained: 0,
    avgLatency: null,
    maxSpeed: null,
    subCount: 0,
    protoCount: 0,
  }
)

defineEmits<{
  (e: 'filter-metric', type: 'all' | 'healthy' | 'fast' | 'chained'): void
}>()
</script>
