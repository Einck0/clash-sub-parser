<template>
  <TooltipProvider>
    <header class="sticky top-0 z-40 flex h-12 items-center justify-between border-b border-border-subtle bg-surface-base px-3 sm:px-6">
      <!-- Left: Brand / Title + Mobile Menu Toggle -->
      <div class="flex items-center gap-2 sm:gap-3 min-w-0">
        <Tooltip>
          <TooltipTrigger as-child>
            <Button
              variant="outline"
              size="icon"
              class="flex h-11 w-11 min-h-[44px] min-w-[44px] items-center justify-center rounded-md md:hidden shrink-0"
              aria-label="打开导航菜单"
              @click="$emit('toggle-sidebar')"
            >
              <Menu class="h-4 w-4" aria-hidden="true" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>打开导航菜单</TooltipContent>
        </Tooltip>

        <div class="flex h-7 w-7 items-center justify-center rounded-md bg-accent-subtle text-accent border border-accent/20 shrink-0">
          <Zap class="h-3.5 w-3.5" aria-hidden="true" />
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
        <DropdownMenu>
          <DropdownMenuTrigger as-child>
            <Button
              variant="outline"
              size="icon"
              class="flex h-11 w-11 min-h-[44px] min-w-[44px] sm:h-7 sm:w-7 sm:min-h-0 sm:min-w-0 shrink-0"
              :aria-label="theme === 'dark' ? '当前为暗色主题，打开主题菜单' : '当前为浅色主题，打开主题菜单'"
              :title="theme === 'dark' ? '当前为暗色主题，打开主题菜单' : '当前为浅色主题，打开主题菜单'"
            >
              <Sun v-if="theme === 'dark'" class="h-3.5 w-3.5" aria-hidden="true" />
              <Moon v-else class="h-3.5 w-3.5" aria-hidden="true" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuLabel>主题</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem :data-selected="theme === 'dark'" @select="setTheme('dark')">暗色主题</DropdownMenuItem>
            <DropdownMenuItem :data-selected="theme === 'light'" @select="setTheme('light')">浅色主题</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        <Button
          variant="outline"
          class="inline-flex min-h-[44px] sm:min-h-0 items-center justify-center gap-1.5 border-accent/30 bg-accent-subtle px-2.5 py-1 text-xs text-accent hover:bg-accent/20 shrink-0"
          @click="$emit('open-export')"
        >
          <Share2 class="h-3.5 w-3.5" aria-hidden="true" />
          <span class="hidden sm:inline">Quick Export</span>
          <span class="sm:hidden">导出</span>
        </Button>
      </div>
    </header>
  </TooltipProvider>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Menu, Moon, Share2, Sun, Zap } from 'lucide-vue-next'
import { Button } from '../ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '../ui/dropdown-menu'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '../ui/tooltip'
import { useTheme } from '../../utils/theme'

const { theme, toggle, setTheme } = useTheme()

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
