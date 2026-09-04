<template>
  <div class="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-6">
    <div
      class="flex flex-col p-4 rounded-xl border border-white/10 bg-slate-900/60 backdrop-blur-md cursor-pointer hover:border-blue-500/30 transition-colors"
      @click="$emit('filter-metric', 'all')"
    >
      <span class="text-xs font-mono text-slate-400">TOTAL NODES</span>
      <span class="text-2xl font-mono font-bold text-white mt-1">{{ total }}</span>
    </div>

    <div
      class="flex flex-col p-4 rounded-xl border border-white/10 bg-slate-900/60 backdrop-blur-md cursor-pointer hover:border-emerald-500/30 transition-colors"
      @click="$emit('filter-metric', 'healthy')"
    >
      <span class="text-xs font-mono text-slate-400">HEALTHY (ONLINE)</span>
      <span class="text-2xl font-mono font-bold text-emerald-400 mt-1">{{ healthy }}</span>
    </div>

    <div
      class="flex flex-col p-4 rounded-xl border border-white/10 bg-slate-900/60 backdrop-blur-md cursor-pointer hover:border-cyan-500/30 transition-colors"
      @click="$emit('filter-metric', 'fast')"
    >
      <span class="text-xs font-mono text-slate-400">FAST (> 20 Mbps)</span>
      <span class="text-2xl font-mono font-bold text-cyan-400 mt-1">{{ fast }}</span>
    </div>

    <div
      class="flex flex-col p-4 rounded-xl border border-white/10 bg-slate-900/60 backdrop-blur-md cursor-pointer hover:border-purple-500/30 transition-colors"
      @click="$emit('filter-metric', 'chained')"
    >
      <span class="text-xs font-mono text-slate-400">PROXY CHAINED</span>
      <span class="text-2xl font-mono font-bold text-purple-400 mt-1">{{ chained }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    total?: number
    healthy?: number
    fast?: number
    chained?: number
  }>(),
  {
    total: 0,
    healthy: 0,
    fast: 0,
    chained: 0,
  }
)

defineEmits<{
  (e: 'filter-metric', type: 'all' | 'healthy' | 'fast' | 'chained'): void
}>()
</script>
