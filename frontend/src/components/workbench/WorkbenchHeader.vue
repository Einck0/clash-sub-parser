<template>
  <header class="sticky top-0 z-40 flex h-14 items-center justify-between border-b border-white/10 bg-[#090D16]/80 px-6 backdrop-blur-md">
    <!-- Left: Brand / Title -->
    <div class="flex items-center gap-3">
      <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-blue-600/20 text-blue-400 border border-blue-500/30">
        <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
        </svg>
      </div>
      <span class="font-mono text-sm font-semibold tracking-wider text-white">CSP // WORKBENCH</span>
    </div>

    <!-- Center: Global Quick Stats -->
    <div class="hidden md:flex items-center gap-6 text-xs font-mono text-slate-400">
      <div class="flex items-center gap-2">
        <span class="h-2 w-2 rounded-full bg-emerald-400"></span>
        <span>GATEWAY: <span class="text-emerald-400">ONLINE</span></span>
      </div>
      <div class="flex items-center gap-2">
        <span>NODES: <span class="text-white">{{ totalNodes }}</span></span>
      </div>
      <div class="flex items-center gap-2">
        <span>PROBED: <span class="text-cyan-400">{{ probedCount }}</span></span>
      </div>
    </div>

    <!-- Right: Quick Actions -->
    <div class="flex items-center gap-3">
      <button
        class="inline-flex items-center gap-1.5 rounded-lg border border-blue-500/30 bg-blue-600/10 px-3 py-1.5 text-xs font-medium text-blue-400 hover:bg-blue-600/20 transition-colors cursor-pointer"
        @click="$emit('open-export')"
      >
        <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
        </svg>
        Quick Export
      </button>
    </div>
  </header>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    totalNodes?: number
    probedCount?: number
  }>(),
  {
    totalNodes: 0,
    probedCount: 0,
  }
)

defineEmits<{
  (e: 'open-export'): void
}>()
</script>
