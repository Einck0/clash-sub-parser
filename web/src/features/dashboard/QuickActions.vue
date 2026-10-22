<script setup lang="ts">
import {
  DocumentDuplicateIcon,
  ServerStackIcon,
  CpuChipIcon,
  ShieldCheckIcon,
  ArrowDownTrayIcon,
  Cog6ToothIcon,
  SparklesIcon,
} from '@heroicons/vue/24/outline'
import { t, uiText } from '../../locales'
import type { QuickActionItem } from './dashboardTypes'

const emit = defineEmits<{
  (e: 'navigate', routeName: string): void
}>()

const actions: QuickActionItem[] = [
  {
    id: 'subscriptions',
    title: t('dashboard.addSubscription'),
    description: uiText('管理上游订阅与节点拉取', 'Manage feeds & ingestion'),
    routeName: 'subscriptions',
    icon: DocumentDuplicateIcon,
  },
  {
    id: 'nodes',
    title: uiText('节点台账', 'Node Ledger'),
    description: uiText('查看可用节点与探针记录', 'Browse nodes & probe records'),
    routeName: 'nodes',
    icon: ServerStackIcon,
  },
  {
    id: 'policy',
    title: uiText('策略组编排', 'Policy Groups'),
    description: uiText('配置分流策略与故障转移', 'Configure policy trees & fallbacks'),
    routeName: 'policy',
    icon: CpuChipIcon,
  },
  {
    id: 'rules',
    title: uiText('分流规则库', 'Rule Desk'),
    description: uiText('分流分类、域名与IP规则', 'Manage routing rules & categories'),
    routeName: 'rules',
    icon: ShieldCheckIcon,
  },
  {
    id: 'publications',
    title: t('dashboard.exportConfig'),
    description: uiText('生成Clash/Sing-box配置', 'Generate client subscription links'),
    routeName: 'publications',
    icon: ArrowDownTrayIcon,
  },
  {
    id: 'settings',
    title: t('dashboard.systemSettings'),
    description: uiText('全局筛选、认证与偏好', 'Security, filters & preferences'),
    routeName: 'settings',
    icon: Cog6ToothIcon,
  },
]

function handleClick(routeName: string) {
  emit('navigate', routeName)
}
</script>

<template>
  <div class="card bg-base-200/60 border border-base-content/10 shadow-sm" data-testid="quick-actions-card">
    <div class="card-body p-4 sm:p-5 gap-3">
      <!-- Section Header -->
      <div class="flex items-center justify-between gap-2 min-w-0">
        <div class="flex items-center gap-2 min-w-0">
          <SparklesIcon class="w-4 h-4 text-primary shrink-0" />
          <h3 class="text-sm sm:text-base font-bold text-base-content truncate">
            {{ t('dashboard.quickActionsTitle') }}
          </h3>
        </div>
        <span class="text-xs text-base-content/50 hidden sm:inline truncate">
          {{ uiText('快捷直达模块，不触发后台探针', 'Direct navigation, no side-effects') }}
        </span>
      </div>

      <!-- Action Buttons Grid -->
      <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-2 sm:gap-2.5">
        <button
          v-for="action in actions"
          :key="action.id"
          type="button"
          :data-testid="`quick-action-${action.id}`"
          class="btn btn-outline btn-neutral btn-sm h-auto py-2.5 px-3 flex flex-col items-start text-left gap-1 hover:border-primary/50 hover:bg-base-300/40 touch-manipulation transition-all"
          @click="handleClick(action.routeName)"
        >
          <div class="flex items-center gap-1.5 w-full">
            <component :is="action.icon" class="w-4 h-4 text-primary shrink-0" />
            <span class="font-semibold text-xs text-base-content truncate">{{ action.title }}</span>
          </div>
          <span class="text-[11px] text-base-content/60 font-normal truncate w-full">
            {{ action.description }}
          </span>
        </button>
      </div>
    </div>
  </div>
</template>
