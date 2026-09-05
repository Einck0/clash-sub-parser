<template>
  <header class="sticky top-0 z-40 flex h-12 items-center justify-between border-b border-border-subtle bg-surface-base px-3 sm:px-6">
    <!-- Left: Brand / Title + Mobile Menu Toggle -->
    <div class="flex items-center gap-2 sm:gap-3 min-w-0">
      <button
        type="button"
        class="flex h-11 w-11 min-h-[44px] min-w-[44px] items-center justify-center rounded-md border border-border-subtle bg-surface-hover text-text-muted hover:bg-surface-active hover:text-text-main transition-colors md:hidden cursor-pointer shrink-0"
        aria-label="打开导航菜单"
        @click="$emit('toggle-sidebar')"
      >
        <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
        </svg>
      </button>

      <div class="flex h-7 w-7 items-center justify-center rounded-md bg-accent-subtle text-accent border border-accent/20 shrink-0">
        <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
        </svg>
      </div>
      <span class="font-mono text-xs font-semibold tracking-wider text-text-main truncate">CSP // WORKBENCH</span>
    </div>

    <!-- Center: Global Quick Stats -->
    <div class="hidden md:flex items-center gap-6 text-xs font-mono text-text-muted">
      <div class="flex items-center gap-2">
        <span class="h-2 w-2 rounded-full bg-status-success"></span>
        <span>GATEWAY: <span class="text-status-success font-medium">ONLINE</span></span>
      </div>
      <div class="flex items-center gap-2">
        <span>NODES: <span class="text-text-main font-medium tabular-nums">{{ displayTotalNodes }}</span></span>
      </div>
      <div class="flex items-center gap-2">
        <span>PROBED: <span class="text-status-info font-medium tabular-nums">{{ displayProbedCount }}</span></span>
      </div>
    </div>

    <!-- Right: Quick Actions & Theme Toggle -->
    <div class="flex items-center gap-2 sm:gap-2.5 shrink-0">
      <button
        type="button"
        class="flex h-11 w-11 min-h-[44px] min-w-[44px] sm:h-7 sm:w-7 sm:min-h-0 sm:min-w-0 items-center justify-center rounded-md border border-border-subtle bg-surface-hover text-text-muted hover:text-text-main transition-colors cursor-pointer shrink-0"
        :aria-label="theme === 'dark' ? '切换为浅色主题' : '切换为暗色主题'"
        :title="theme === 'dark' ? '切换为浅色主题' : '切换为暗色主题'"
        @click="toggle"
      >
        <svg v-if="theme === 'dark'" class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z" />
        </svg>
        <svg v-else class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
        </svg>
      </button>

      <button
        type="button"
        class="inline-flex min-h-[44px] sm:min-h-0 items-center justify-center gap-1.5 rounded-md border border-accent/30 bg-accent-subtle px-2.5 py-1 text-xs font-medium text-accent hover:bg-accent/20 transition-colors cursor-pointer shrink-0"
        @click="$emit('open-export')"
      >
        <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
        </svg>
        <span class="hidden sm:inline">Quick Export</span>
        <span class="sm:hidden">导出</span>
      </button>
    </div>
  </header>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useTheme } from '../../utils/theme'

const { theme, toggle } = useTheme()

interface NodeSummaryProp {
  status: 'idle' | 'loading' | 'ready' | 'error'
  total: number
  probed: number
}

const props = withDefaults(
  defineProps<{
    summary?: NodeSummaryProp
    totalNodes?: number
    probedCount?: number
  }>(),
  {
    summary: undefined,
    totalNodes: 0,
    probedCount: 0,
  }
)

const displayTotalNodes = computed(() => {
  if (props.summary) {
    if (props.summary.status === 'idle' || props.summary.status === 'loading') {
      return '--'
    }
    if (props.summary.status === 'error') {
      return '-'
    }
    return props.summary.total
  }
  return props.totalNodes
})

const displayProbedCount = computed(() => {
  if (props.summary) {
    if (props.summary.status === 'idle' || props.summary.status === 'loading') {
      return '--'
    }
    if (props.summary.status === 'error') {
      return '-'
    }
    return props.summary.probed
  }
  return props.probedCount
})

defineEmits<{
  (e: 'open-export'): void
  (e: 'toggle-sidebar'): void
}>()
</script>
