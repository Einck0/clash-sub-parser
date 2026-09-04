<template>
  <div class="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
    <!-- Total Nodes Metric -->
    <div
      class="flex flex-col p-4 rounded-xl border transition-all cursor-pointer bg-slate-900/60 backdrop-blur-md"
      :class="[
        activeFilter?.status === 'all' && (!activeFilter?.minSpeed) && activeFilter?.chain === 'all'
          ? 'border-blue-500/40 shadow-xs'
          : 'border-white/10 hover:border-blue-500/30'
      ]"
      role="button"
      tabindex="0"
      title="点击重置全部筛选"
      @click="$emit('filter-metric', 'all')"
      @keydown.enter="$emit('filter-metric', 'all')"
    >
      <div class="flex justify-between items-center">
        <span class="text-xs font-mono text-slate-400">TOTAL NODES</span>
        <span class="text-sm">🌐</span>
      </div>
      <div class="flex items-baseline gap-2 mt-1">
        <span class="text-2xl font-mono font-bold text-white">{{ total }}</span>
        <span v-if="filteredCount != null && filteredCount !== total" class="text-xs font-mono text-slate-400">
          (匹配 {{ filteredCount }})
        </span>
      </div>
      <span class="text-xs font-mono text-slate-500 mt-1">
        {{ subCount ? `${subCount} 订阅 · ` : '' }}{{ protoCount ? `${protoCount} 协议` : '全部已登记' }}
      </span>
    </div>

    <!-- Healthy Online Metric -->
    <div
      class="flex flex-col p-4 rounded-xl border transition-all cursor-pointer bg-slate-900/60 backdrop-blur-md"
      :class="[
        activeFilter?.status === 'ok'
          ? 'border-emerald-500/50 bg-emerald-950/20 shadow-xs'
          : 'border-white/10 hover:border-emerald-500/30'
      ]"
      role="button"
      tabindex="0"
      title="点击快速筛选在线正常节点"
      @click="$emit('filter-metric', 'healthy')"
      @keydown.enter="$emit('filter-metric', 'healthy')"
    >
      <div class="flex justify-between items-center">
        <span class="text-xs font-mono text-slate-400">HEALTHY (ONLINE)</span>
        <span class="text-sm">🟢</span>
      </div>
      <div class="flex items-baseline gap-2 mt-1">
        <span class="text-2xl font-mono font-bold text-emerald-400">{{ healthy }}</span>
        <span class="text-xs font-mono text-slate-400">
          {{ testedCount ? `${Math.round((healthy / testedCount) * 100)}%` : '未测' }}
        </span>
      </div>
      <span class="text-xs font-mono text-slate-500 mt-1">
        {{ testedCount ? `${testedCount} 已测` : '尚未质检' }}
        {{ avgLatency ? ` · 均延 ${avgLatency}ms` : '' }}
      </span>
    </div>

    <!-- Fast Nodes Metric -->
    <div
      class="flex flex-col p-4 rounded-xl border transition-all cursor-pointer bg-slate-900/60 backdrop-blur-md"
      :class="[
        (activeFilter?.minSpeed ?? 0) > 0
          ? 'border-cyan-500/50 bg-cyan-950/20 shadow-xs'
          : 'border-white/10 hover:border-cyan-500/30'
      ]"
      role="button"
      tabindex="0"
      title="点击快速筛选高速可用节点 (>=10Mbps)"
      @click="$emit('filter-metric', 'fast')"
      @keydown.enter="$emit('filter-metric', 'fast')"
    >
      <div class="flex justify-between items-center">
        <span class="text-xs font-mono text-slate-400">FAST (> 10 Mbps)</span>
        <span class="text-sm">🚀</span>
      </div>
      <div class="flex items-baseline gap-2 mt-1">
        <span class="text-2xl font-mono font-bold text-cyan-400">{{ fast }}</span>
        <span v-if="maxSpeed" class="text-xs font-mono text-slate-400">
          峰值 {{ maxSpeed }}M
        </span>
      </div>
      <span class="text-xs font-mono text-slate-500 mt-1">
        {{ (activeFilter?.minSpeed ?? 0) > 0 ? '已生效高速门槛' : '点击筛选高速节点' }}
      </span>
    </div>

    <!-- Proxy Chained Metric -->
    <div
      class="flex flex-col p-4 rounded-xl border transition-all cursor-pointer bg-slate-900/60 backdrop-blur-md"
      :class="[
        activeFilter?.chain === 'chained'
          ? 'border-purple-500/50 bg-purple-950/20 shadow-xs'
          : 'border-white/10 hover:border-purple-500/30'
      ]"
      role="button"
      tabindex="0"
      title="点击快速筛选已配置跳板链路节点"
      @click="$emit('filter-metric', 'chained')"
      @keydown.enter="$emit('filter-metric', 'chained')"
    >
      <div class="flex justify-between items-center">
        <span class="text-xs font-mono text-slate-400">PROXY CHAINED</span>
        <span class="text-sm">🔗</span>
      </div>
      <div class="flex items-baseline gap-2 mt-1">
        <span class="text-2xl font-mono font-bold text-purple-400">{{ chained }}</span>
        <span class="text-xs font-mono text-slate-400">
          / {{ total - chained }} 直连
        </span>
      </div>
      <span class="text-xs font-mono text-slate-500 mt-1">
        {{ activeFilter?.chain === 'chained' ? '仅看已挂跳板' : '点击筛选挂链节点' }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
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
