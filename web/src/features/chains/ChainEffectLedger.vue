<script setup lang="ts">
import { ref, computed } from 'vue'
import {
  MagnifyingGlassIcon,
  ServerStackIcon,
  LinkIcon,
  GlobeAltIcon,
} from '@heroicons/vue/24/outline'
import { uiText } from '../../locales/redesign'
import type { EffectNode } from './types'

const props = withDefaults(
  defineProps<{
    nodes: EffectNode[]
    loading?: boolean
    error?: string | null
  }>(),
  {
    loading: false,
    error: null,
  }
)

const searchQuery = ref('')
const statusFilter = ref<'all' | 'chained' | 'direct'>('all')

const chainedCount = computed(() => {
  return props.nodes.filter((n) => Boolean(n.dialer_proxy)).length
})

const directCount = computed(() => {
  return props.nodes.filter((n) => !n.dialer_proxy).length
})

const filteredNodes = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return props.nodes.filter((node) => {
    // Status filter
    if (statusFilter.value === 'chained' && !node.dialer_proxy) return false
    if (statusFilter.value === 'direct' && node.dialer_proxy) return false

    // Search query
    if (q) {
      const matchName = node.display_name?.toLowerCase().includes(q)
      const matchId = node.logical_id?.toLowerCase().includes(q)
      const matchDialer = node.dialer_proxy?.toLowerCase().includes(q)
      if (!matchName && !matchId && !matchDialer) return false
    }

    return true
  })
})

function getSourceBadgeLabel(source?: string): string {
  switch (source) {
    case 'node':
      return uiText('单节点绑定', 'Node')
    case 'node_group':
      return uiText('策略组绑定', 'Group')
    case 'subscription':
      return uiText('订阅绑定', 'Subscription')
    default:
      return source || uiText('未绑定', 'None')
  }
}
</script>

<template>
  <div
    class="bg-base-100 rounded-3xl border border-base-300/60 shadow-sm p-4 sm:p-6 space-y-4"
    data-testid="chain-effect-ledger"
  >
    <!-- Header & Summary Metrics -->
    <div class="flex flex-wrap items-center justify-between gap-4">
      <div class="space-y-1">
        <h3 class="text-base sm:text-lg font-bold text-base-content flex items-center gap-2">
          <ServerStackIcon class="w-5 h-5 text-primary shrink-0" />
          <span>{{ uiText('生效节点跳板台账', 'Resolved Node Dialer Ledger') }}</span>
        </h3>
        <p class="text-xs text-base-content/60">
          {{
            uiText(
              '当前解析策略快照中各节点的最终前置跳板代理与来源归属',
              'Final upstream dialer proxy assignment and binding provenance for each egress node'
            )
          }}
        </p>
      </div>

      <!-- Quick Metrics Chips -->
      <div class="flex flex-wrap items-center gap-2 text-xs">
        <div class="badge badge-neutral gap-1.5 py-3 px-3" data-testid="ledger-total-count">
          <span class="text-base-content/60">{{ uiText('总节点数', 'Total') }}:</span>
          <span class="font-bold">{{ nodes.length }}</span>
        </div>
        <div class="badge badge-primary badge-outline gap-1.5 py-3 px-3" data-testid="ledger-chained-count">
          <LinkIcon class="w-3.5 h-3.5 text-primary" />
          <span class="text-base-content/60">{{ uiText('已配置跳板', 'Chained') }}:</span>
          <span class="font-bold text-primary">{{ chainedCount }}</span>
        </div>
        <div class="badge badge-ghost gap-1.5 py-3 px-3">
          <GlobeAltIcon class="w-3.5 h-3.5 text-base-content/50" />
          <span class="text-base-content/60">{{ uiText('直连/无跳板', 'Direct') }}:</span>
          <span class="font-bold">{{ directCount }}</span>
        </div>
      </div>
    </div>

    <!-- Controls: Search & Status Filter -->
    <div class="flex flex-wrap items-center justify-between gap-3 pt-2">
      <div class="relative flex-1 min-w-[200px] max-w-sm">
        <MagnifyingGlassIcon class="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-base-content/40" />
        <input
          v-model="searchQuery"
          type="search"
          :placeholder="uiText('搜索节点名称、ID 或跳板...', 'Search node name, ID, dialer...')"
          class="input input-bordered input-sm w-full pl-9"
          data-testid="ledger-search-input"
        />
      </div>

      <div class="join" data-testid="ledger-filter-status">
        <button
          type="button"
          class="btn btn-xs join-item"
          :class="statusFilter === 'all' ? 'btn-active btn-primary' : 'btn-ghost'"
          @click="statusFilter = 'all'"
        >
          {{ uiText('全部', 'All') }}
        </button>
        <button
          type="button"
          class="btn btn-xs join-item"
          :class="statusFilter === 'chained' ? 'btn-active btn-primary' : 'btn-ghost'"
          @click="statusFilter = 'chained'"
        >
          {{ uiText('仅跳板', 'Chained Only') }}
        </button>
        <button
          type="button"
          class="btn btn-xs join-item"
          :class="statusFilter === 'direct' ? 'btn-active btn-primary' : 'btn-ghost'"
          @click="statusFilter = 'direct'"
        >
          {{ uiText('仅直连', 'Direct Only') }}
        </button>
      </div>
    </div>

    <!-- Loading State -->
    <div v-if="loading" class="p-8 flex flex-col items-center justify-center space-y-2">
      <span class="loading loading-spinner loading-md text-primary" />
      <span class="text-xs text-base-content/60">{{ uiText('加载节点台账中...', 'Loading node ledger...') }}</span>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="alert alert-error shadow-sm rounded-xl p-3 text-xs">
      <span>{{ error }}</span>
    </div>

    <!-- Table -->
    <div v-else-if="filteredNodes.length > 0" class="overflow-x-auto rounded-2xl border border-base-200">
      <table class="table table-sm table-zebra w-full text-xs">
        <thead class="bg-base-200/50 text-base-content/70">
          <tr>
            <th>{{ uiText('节点名称 / ID', 'Node Name / ID') }}</th>
            <th>{{ uiText('最终前置跳板代理 (dialer-proxy)', 'Assigned Dialer Proxy') }}</th>
            <th>{{ uiText('生效来源', 'Winning Source') }}</th>
            <th>{{ uiText('生效绑定 ID', 'Binding ID') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="node in filteredNodes"
            :key="node.logical_id"
            :data-testid="`ledger-node-row-${node.logical_id}`"
          >
            <td>
              <div class="font-semibold text-base-content">{{ node.display_name }}</div>
              <div class="text-[10px] text-base-content/50 font-mono">{{ node.logical_id }}</div>
            </td>

            <td>
              <span
                v-if="node.dialer_proxy"
                class="badge badge-primary badge-sm font-semibold gap-1"
              >
                <LinkIcon class="w-3 h-3" />
                <span>{{ node.dialer_proxy }}</span>
              </span>
              <span v-else class="badge badge-ghost badge-sm text-base-content/50">
                {{ uiText('直连 (Direct)', 'Direct') }}
              </span>
            </td>

            <td>
              <span
                v-if="node.chain_source"
                class="badge badge-sm"
                :class="{
                  'badge-secondary badge-outline': node.chain_source === 'node',
                  'badge-accent badge-outline': node.chain_source === 'node_group',
                  'badge-info badge-outline': node.chain_source === 'subscription',
                }"
              >
                {{ getSourceBadgeLabel(node.chain_source) }}
              </span>
              <span v-else class="text-base-content/40">—</span>
            </td>

            <td>
              <span v-if="node.chain_binding_id" class="font-mono text-[11px] text-base-content/70">
                {{ node.chain_binding_id }}
              </span>
              <span v-else class="text-base-content/40">—</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Empty State -->
    <div
      v-else
      class="p-8 text-center rounded-2xl border border-dashed border-base-300 text-xs text-base-content/60"
    >
      {{ uiText('未找到匹配的生效节点记录', 'No matching nodes found in ledger') }}
    </div>
  </div>
</template>
