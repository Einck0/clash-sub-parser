<template>
  <aside class="flex w-56 flex-col border-r border-white/10 bg-[#090D16]/90 p-4 text-slate-300">
    <div class="mb-4 px-2 text-[10px] font-mono tracking-wider text-slate-500 uppercase">Control Center</div>
    <nav class="space-y-1">
      <router-link
        v-for="item in navItems"
        :key="item.path"
        :to="item.path"
        v-slot="{ isActive, navigate }"
        custom
      >
        <button
          :class="[
            'flex w-full items-center gap-3 rounded-lg px-3 py-2 text-xs font-medium transition-all cursor-pointer',
            isActive
              ? 'bg-blue-600/15 text-blue-400 border border-blue-500/20 shadow-xs'
              : 'text-slate-400 hover:bg-slate-800/50 hover:text-slate-200 border border-transparent'
          ]"
          @click="navigate"
        >
          <component :is="item.icon" class="h-4 w-4" />
          <span>{{ item.label }}</span>
        </button>
      </router-link>
    </nav>
  </aside>
</template>

<script setup lang="ts">
import { h } from 'vue'

const navItems = [
  {
    path: '/nodes',
    label: 'Node Ledger',
    icon: () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', class: 'h-4 w-4' }, [
      h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2', d: 'M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10' })
    ])
  },
  {
    path: '/subscriptions',
    label: 'Subscriptions',
    icon: () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', class: 'h-4 w-4' }, [
      h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2', d: 'M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15' })
    ])
  },
  {
    path: '/groups',
    label: 'Policy Groups',
    icon: () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', class: 'h-4 w-4' }, [
      h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2', d: 'M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z' })
    ])
  },
  {
    path: '/rules',
    label: 'Rules',
    icon: () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', class: 'h-4 w-4' }, [
      h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2', d: 'M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-3 7h3m-3 4h3m-6-4h.01M9 16h.01' })
    ])
  },
  {
    path: '/chains',
    label: 'Proxy Chains',
    icon: () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', class: 'h-4 w-4' }, [
      h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2', d: 'M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1' })
    ])
  },
  {
    path: '/generate',
    label: 'Generate & Export',
    icon: () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', class: 'h-4 w-4' }, [
      h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2', d: 'M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z' })
    ])
  },
  {
    path: '/settings',
    label: 'Settings',
    icon: () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', class: 'h-4 w-4' }, [
      h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2', d: 'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z' }),
      h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2', d: 'M15 12a3 3 0 11-6 0 3 3 0 016 0z' })
    ])
  }
]
</script>
