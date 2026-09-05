<template>
  <aside
    :class="[
      'flex flex-col bg-surface-base text-text-muted',
      mobile ? 'w-full p-3' : 'w-56 border-r border-border-subtle p-3 shrink-0'
    ]"
  >
    <div v-for="section in navSections" :key="section.title" class="mb-4 last:mb-0">
      <div class="mb-1.5 px-2.5 text-[10px] font-mono font-semibold tracking-wider text-text-sub uppercase">
        {{ section.title }}
      </div>
      <nav class="space-y-0.5">
        <router-link
          v-for="item in section.items"
          :key="item.path"
          :to="item.path"
          v-slot="{ isActive, navigate }"
          custom
        >
          <button
            type="button"
            :class="[
              'flex w-full items-center gap-2.5 rounded-md px-2.5 py-2 text-xs font-medium transition-colors cursor-pointer text-left',
              mobile ? 'min-h-[44px]' : '',
              isActive
                ? 'bg-accent-subtle text-accent border border-accent/25'
                : 'text-text-muted hover:bg-surface-hover hover:text-text-main border border-transparent'
            ]"
            @click="handleClick(navigate)"
          >
            <component :is="item.icon" class="h-4 w-4 shrink-0" />
            <span class="truncate">{{ item.label }}</span>
          </button>
        </router-link>
      </nav>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { h, type VNode } from 'vue'

interface NavItem {
  path: string
  alias?: string
  label: string
  icon: () => VNode
}

interface NavSection {
  title: string
  items: NavItem[]
}

withDefaults(
  defineProps<{
    mobile?: boolean
  }>(),
  {
    mobile: false
  }
)

const emit = defineEmits<{
  (e: 'navigate'): void
}>()

function handleClick(navigate: () => void) {
  navigate()
  emit('navigate')
}

const navSections: NavSection[] = [
  {
    title: 'Control Center',
    items: [
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
        path: '/node-groups',
        alias: '/groups',
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
        path: '/proxy-chains',
        alias: '/chains',
        label: 'Proxy Chains',
        icon: () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', class: 'h-4 w-4' }, [
          h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2', d: 'M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1' })
        ])
      },
      {
        path: '/dns',
        label: 'DNS Settings',
        icon: () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', class: 'h-4 w-4' }, [
          h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2', d: 'M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9' })
        ])
      }
    ]
  },
  {
    title: 'Distribution',
    items: [
      {
        path: '/generate',
        label: 'Generate & Export',
        icon: () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', class: 'h-4 w-4' }, [
          h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2', d: 'M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z' })
        ])
      }
    ]
  },
  {
    title: 'System',
    items: [
      {
        path: '/settings',
        label: 'Settings',
        icon: () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', class: 'h-4 w-4' }, [
          h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2', d: 'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z' }),
          h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2', d: 'M15 12a3 3 0 11-6 0 3 3 0 016 0z' })
        ])
      },
      {
        path: '/history',
        label: 'Config History',
        icon: () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', class: 'h-4 w-4' }, [
          h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2', d: 'M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z' })
        ])
      }
    ]
  }
]
</script>
