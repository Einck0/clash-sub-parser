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
          <Button
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
          </Button>
        </router-link>
      </nav>
    </div>
  </aside>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import { ClipboardList, FileOutput, Globe, History, Link2, RefreshCw, Settings, Shield, Users } from 'lucide-vue-next'
import { Button } from '../ui/button'

interface NavItem {
  path: string
  alias?: string
  label: string
  icon: Component
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
        icon: ClipboardList
      },
      {
        path: '/subscriptions',
        label: 'Subscriptions',
        icon: RefreshCw
      },
      {
        path: '/node-groups',
        alias: '/groups',
        label: 'Policy Groups',
        icon: Users
      },
      {
        path: '/rules',
        label: 'Rules',
        icon: Shield
      },
      {
        path: '/proxy-chains',
        alias: '/chains',
        label: 'Proxy Chains',
        icon: Link2
      },
      {
        path: '/dns',
        label: 'DNS Settings',
        icon: Globe
      }
    ]
  },
  {
    title: 'Distribution',
    items: [
      {
        path: '/generate',
        label: 'Generate & Export',
        icon: FileOutput
      }
    ]
  },
  {
    title: 'System',
    items: [
      {
        path: '/settings',
        label: 'Settings',
        icon: Settings
      },
      {
        path: '/history',
        label: 'Config History',
        icon: History
      }
    ]
  }
]
</script>
