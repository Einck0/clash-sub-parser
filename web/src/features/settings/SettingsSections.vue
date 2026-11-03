<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { Cog6ToothIcon, GlobeAltIcon, LinkIcon } from '@heroicons/vue/24/outline'
import { uiText } from '../../locales/redesign'

export type SettingsSectionId = 'csp' | 'dns' | 'chains'

defineProps<{
  active: SettingsSectionId
}>()

const sections = [
  {
    id: 'csp' as const,
    to: '/settings',
    labelZh: 'CSP 设置',
    labelEn: 'CSP Settings',
    icon: Cog6ToothIcon,
    testId: 'settings-section-csp',
  },
  {
    id: 'dns' as const,
    to: '/dns',
    labelZh: 'DNS 设置',
    labelEn: 'DNS Settings',
    icon: GlobeAltIcon,
    testId: 'settings-section-dns',
  },
  {
    id: 'chains' as const,
    to: '/chains',
    labelZh: '链式代理',
    labelEn: 'Proxy Chains',
    icon: LinkIcon,
    testId: 'settings-section-chains',
  },
]
</script>

<template>
  <nav
    class="flex flex-wrap items-center gap-1.5 p-1 rounded-xl bg-base-200/60 border border-base-300/50 w-full"
    aria-label="Settings Sections"
    data-testid="settings-sections-nav"
  >
    <RouterLink
      v-for="sec in sections"
      :key="sec.id"
      :to="sec.to"
      class="btn btn-sm gap-2 transition-all flex-1 sm:flex-initial"
      :class="active === sec.id ? 'btn-primary shadow-sm font-semibold' : 'btn-ghost text-base-content/70 hover:text-base-content'"
      :aria-current="active === sec.id ? 'page' : undefined"
      :aria-label="uiText(sec.labelZh, sec.labelEn)"
      :data-testid="sec.testId"
    >
      <component :is="sec.icon" class="w-4 h-4 shrink-0" aria-hidden="true" />
      <span>{{ uiText(sec.labelZh, sec.labelEn) }}</span>
    </RouterLink>
  </nav>
</template>
