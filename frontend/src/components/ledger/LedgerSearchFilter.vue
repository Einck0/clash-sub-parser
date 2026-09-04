<template>
  <div class="flex flex-wrap items-center justify-between gap-4 p-4 rounded-xl border border-white/10 bg-slate-900/40 backdrop-blur-md mb-6">
    <!-- Left: Keyword Search & Quick Dropdowns -->
    <div class="flex flex-wrap items-center gap-3 flex-1">
      <div class="relative min-w-[240px] flex-1 max-w-md">
        <input
          :value="modelValue.keyword"
          type="text"
          placeholder="搜索节点名称 / IP / 端口..."
          class="w-full rounded-lg border border-white/10 bg-slate-950/60 px-3.5 py-2 pl-9 text-xs text-white placeholder-slate-500 focus:border-blue-500 focus:outline-hidden font-mono"
          @input="$emit('update:modelValue', { ...modelValue, keyword: ($event.target as HTMLInputElement).value })"
        />
        <svg class="absolute left-3 top-2.5 h-3.5 w-3.5 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
      </div>

      <!-- Protocol Filter -->
      <select
        :value="modelValue.protocol"
        class="rounded-lg border border-white/10 bg-slate-950/60 px-3 py-2 text-xs text-slate-300 focus:border-blue-500 focus:outline-hidden font-mono"
        @change="$emit('update:modelValue', { ...modelValue, protocol: ($event.target as HTMLSelectElement).value })"
      >
        <option value="">全部协议</option>
        <option value="ss">Shadowsocks</option>
        <option value="vmess">VMess</option>
        <option value="vless">VLESS</option>
        <option value="trojan">Trojan</option>
        <option value="hysteria2">Hysteria2</option>
        <option value="tuic">TUIC</option>
        <option value="wireguard">WireGuard</option>
      </select>

      <!-- Status Filter -->
      <select
        :value="modelValue.status"
        class="rounded-lg border border-white/10 bg-slate-950/60 px-3 py-2 text-xs text-slate-300 focus:border-blue-500 focus:outline-hidden font-mono"
        @change="$emit('update:modelValue', { ...modelValue, status: ($event.target as HTMLSelectElement).value })"
      >
        <option value="">全部状态</option>
        <option value="online">可用 (Online)</option>
        <option value="offline">不可用 (Offline)</option>
        <option value="untested">未探测 (Untested)</option>
      </select>
    </div>

    <!-- Right: View Switcher (Table vs Grid) -->
    <div class="flex items-center gap-2 border-l border-white/10 pl-4">
      <button
        :class="[
          'flex h-8 w-8 items-center justify-center rounded-lg border transition-colors cursor-pointer',
          viewMode === 'table' ? 'border-blue-500/50 bg-blue-600/20 text-blue-400' : 'border-white/10 bg-slate-800/40 text-slate-400 hover:text-white'
        ]"
        title="表格视图"
        @click="$emit('change-view', 'table')"
      >
        <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16" />
        </svg>
      </button>

      <button
        :class="[
          'flex h-8 w-8 items-center justify-center rounded-lg border transition-colors cursor-pointer',
          viewMode === 'grid' ? 'border-blue-500/50 bg-blue-600/20 text-blue-400' : 'border-white/10 bg-slate-800/40 text-slate-400 hover:text-white'
        ]"
        title="卡片视图"
        @click="$emit('change-view', 'grid')"
      >
        <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z" />
        </svg>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
export interface FilterState {
  keyword: string
  protocol: string
  status: string
}

defineProps<{
  modelValue: FilterState
  viewMode: 'table' | 'grid'
}>()

defineEmits<{
  (e: 'update:modelValue', val: FilterState): void
  (e: 'change-view', mode: 'table' | 'grid'): void
}>()
</script>
