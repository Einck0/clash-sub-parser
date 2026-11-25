<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { MagnifyingGlassIcon, ServerStackIcon, LinkIcon, GlobeAltIcon, ArrowPathIcon } from '@heroicons/vue/24/outline'
import { uiText } from '../../locales/redesign'
import type { EffectNode, OptionsResult } from './types'
import { displayName, displayText } from '../../utils/displayNames'

const props = withDefaults(defineProps<{
  nodes: EffectNode[]
  options?: OptionsResult
  loading?: boolean
  loaded?: boolean
  stale?: boolean
  error?: string | null
}>(), {
  options: () => ({ nodes: [], groups: [], subscriptions: [] }),
  loading: false,
  loaded: false,
  stale: false,
  error: null,
})
const emit = defineEmits<{ (e: 'retry'): void }>()
const searchQuery = ref('')
const statusFilter = ref<'all' | 'chained' | 'direct'>('all')
const visibleLimit = ref(50)
const showingPrevious = computed(() => props.loaded && (props.loading || !!props.error || props.stale))
const chainedCount = computed(() => props.nodes.filter((node) => Boolean(node.dialer_proxy)).length)
const directCount = computed(() => props.nodes.length - chainedCount.value)
const optionNames = computed(() => new Map([...props.options.nodes, ...props.options.groups].map((item) => [item.id, item])))

function dialerName(id?: string): string {
  if (!id) return ''
  const option = optionNames.value.get(id)
  // Effects may contain either an option ID or an already-resolved business name.
  if (!option && displayText(id) !== id) return uiText('已配置跳板', 'Dialer configured')
  return displayName(option?.name || id, uiText('已配置跳板', 'Dialer configured'), option ? id : undefined)
}
function sourceLabel(source?: string): string {
  switch (source) {
    case 'node': return uiText('单节点绑定', 'Node')
    case 'node_group': return uiText('策略组绑定', 'Group')
    case 'subscription': return uiText('订阅绑定', 'Subscription')
    default: return displayText(source) || uiText('未绑定', 'None')
  }
}
const filteredNodes = computed(() => {
  const query = searchQuery.value.trim().toLocaleLowerCase()
  return props.nodes.filter((node) => {
    if (statusFilter.value === 'chained' && !node.dialer_proxy) return false
    if (statusFilter.value === 'direct' && node.dialer_proxy) return false
    return !query || [node.display_name, node.logical_id, node.dialer_proxy, dialerName(node.dialer_proxy), sourceLabel(node.chain_source)]
      .some((value) => value?.toLocaleLowerCase().includes(query))
  })
})
const visibleNodes = computed(() => filteredNodes.value.slice(0, visibleLimit.value))
watch([searchQuery, statusFilter], () => { visibleLimit.value = 50 })
</script>

<template>
  <section class="bg-base-100 rounded-3xl border border-base-300/60 shadow-sm p-4 sm:p-6 space-y-4 min-w-0" data-testid="chain-effect-ledger" :aria-busy="loading">
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div class="space-y-1 min-w-0">
        <h3 class="text-base sm:text-lg font-bold text-base-content flex items-center gap-2">
          <ServerStackIcon class="w-5 h-5 text-primary shrink-0" />
          <span>{{ uiText('生效节点跳板台账', 'Resolved Node Dialer Ledger') }}</span>
        </h3>
        <p class="text-xs text-base-content/60 break-words">{{ uiText('当前解析策略快照中各节点的最终前置跳板代理与来源归属', 'Final upstream dialer assignment and provenance for each egress node') }}</p>
      </div>
      <div class="flex flex-wrap items-center gap-2 text-xs">
        <div class="badge badge-neutral gap-1.5 py-3 px-3" data-testid="ledger-total-count">
          <span>{{ uiText('总节点数', 'Total') }}:</span><strong>{{ loaded ? nodes.length : '—' }}</strong>
        </div>
        <div class="badge badge-primary badge-outline gap-1.5 py-3 px-3" data-testid="ledger-chained-count">
          <LinkIcon class="w-3.5 h-3.5 shrink-0" /><span>{{ uiText('已配置跳板', 'Chained') }}:</span><strong>{{ loaded ? chainedCount : '—' }}</strong>
        </div>
        <div class="badge badge-ghost gap-1.5 py-3 px-3" data-testid="ledger-direct-count">
          <GlobeAltIcon class="w-3.5 h-3.5 shrink-0" /><span>{{ uiText('直连/无跳板', 'Direct') }}:</span><strong>{{ loaded ? directCount : '—' }}</strong>
        </div>
      </div>
    </div>

    <div v-if="error" role="alert" class="alert alert-error rounded-xl p-3 text-xs flex flex-wrap items-start gap-3" data-testid="ledger-error">
      <span class="min-w-0 flex-1 break-words">{{ uiText('台账读取失败：', 'Unable to read ledger: ') }}{{ displayText(error) }}</span>
      <button type="button" class="btn btn-sm btn-outline shrink-0" :disabled="loading" data-testid="ledger-retry" @click="emit('retry')">{{ uiText('重试', 'Retry') }}</button>
    </div>
    <div v-if="showingPrevious" role="status" class="rounded-xl border border-warning/30 bg-warning/10 p-3 text-xs break-words" data-testid="ledger-previous-data">
      {{ uiText('以下计数和节点来自上次成功读取，不代表当前配置已重新确认。', 'Counts and nodes below are from the last successful read, not a confirmation of the current configuration.') }}
      <button v-if="stale && !error" type="button" class="btn btn-xs btn-ghost mt-1" :disabled="loading" @click="emit('retry')">{{ uiText('刷新台账', 'Refresh ledger') }}</button>
    </div>
    <div v-if="loading" role="status" class="flex items-center gap-2 py-3 text-xs text-base-content/60">
      <ArrowPathIcon class="w-4 h-4 animate-spin shrink-0" />{{ uiText('加载节点台账中...', 'Loading node ledger...') }}
    </div>
    <p v-if="!loaded && !loading" class="text-sm text-base-content/60" data-testid="ledger-unverified">{{ uiText('尚未取得可确认的台账，计数未知。', 'No verified ledger is available; counts are unknown.') }}</p>

    <template v-if="loaded">
      <div class="flex flex-wrap items-center justify-between gap-3 pt-2">
        <label class="relative flex-1 min-w-0 basis-60 max-w-sm">
          <MagnifyingGlassIcon class="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-base-content/40" />
          <input v-model="searchQuery" type="search" :aria-label="uiText('搜索生效台账', 'Search resolved ledger')" :placeholder="uiText('搜索节点名称、跳板或来源', 'Search node, dialer or source')" class="input input-bordered input-sm w-full pl-9" data-testid="ledger-search-input" />
        </label>
        <div class="join" data-testid="ledger-filter-status">
          <button type="button" class="btn btn-sm join-item" :class="statusFilter === 'all' ? 'btn-primary' : 'btn-ghost'" :aria-pressed="statusFilter === 'all'" @click="statusFilter = 'all'">{{ uiText('全部', 'All') }}</button>
          <button type="button" class="btn btn-sm join-item" :class="statusFilter === 'chained' ? 'btn-primary' : 'btn-ghost'" :aria-pressed="statusFilter === 'chained'" @click="statusFilter = 'chained'">{{ uiText('仅跳板', 'Chained') }}</button>
          <button type="button" class="btn btn-sm join-item" :class="statusFilter === 'direct' ? 'btn-primary' : 'btn-ghost'" :aria-pressed="statusFilter === 'direct'" @click="statusFilter = 'direct'">{{ uiText('仅直连', 'Direct') }}</button>
        </div>
        <span class="text-xs text-base-content/60" role="status">{{ uiText('匹配', 'Matches') }} {{ filteredNodes.length }} / {{ nodes.length }}</span>
      </div>

      <div v-if="filteredNodes.length" class="overflow-x-auto rounded-2xl border border-base-200">
        <table class="table table-sm table-zebra w-full text-xs max-md:block">
          <thead class="hidden md:table-header-group bg-base-200/50 text-base-content/70">
            <tr><th>{{ uiText('节点名称', 'Node Name') }}</th><th>{{ uiText('最终前置跳板代理', 'Assigned Dialer Proxy') }}</th><th>{{ uiText('生效来源', 'Winning Source') }}</th><th>{{ uiText('绑定状态', 'Binding Status') }}</th></tr>
          </thead>
          <tbody class="block md:table-row-group">
            <tr v-for="node in visibleNodes" :key="node.logical_id" class="block md:table-row max-md:p-2 max-md:border-b max-md:border-base-200" :data-testid="`ledger-node-row-${node.logical_id}`">
              <td class="block md:table-cell whitespace-normal break-words md:max-w-xs"><div class="font-semibold text-base-content">{{ displayName(node.display_name, uiText('未命名节点', 'Unnamed node'), node.logical_id) }}</div></td>
              <td class="block md:table-cell whitespace-normal break-words md:max-w-xs">
                <span class="md:hidden text-base-content/60 mr-2">{{ uiText('跳板', 'Dialer') }}</span>
                <span v-if="node.dialer_proxy" class="inline-flex items-start gap-1 text-primary font-semibold"><LinkIcon class="w-3.5 h-3.5 shrink-0 mt-0.5" /><span class="min-w-0 break-words">{{ dialerName(node.dialer_proxy) }}</span></span>
                <span v-else class="text-base-content/60">{{ uiText('直连 (Direct)', 'Direct') }}</span>
              </td>
              <td class="block md:table-cell"><span class="md:hidden text-base-content/60 mr-2">{{ uiText('来源', 'Source') }}</span><span>{{ sourceLabel(node.chain_source) }}</span></td>
              <td class="block md:table-cell text-base-content/60">{{ node.chain_binding_id ? uiText('已配置', 'Configured') : '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-else class="p-6 text-center rounded-2xl border border-dashed border-base-300 text-xs text-base-content/60">{{ nodes.length ? uiText('未找到匹配的生效节点记录', 'No matching nodes found in ledger') : uiText('当前确认的台账没有节点', 'The verified ledger contains no nodes') }}</p>
      <button v-if="filteredNodes.length > visibleNodes.length" type="button" class="btn btn-sm btn-ghost" data-testid="ledger-show-more" @click="visibleLimit += 50">{{ uiText('显示更多节点', 'Show more nodes') }} ({{ visibleNodes.length }} / {{ filteredNodes.length }})</button>
    </template>
  </section>
</template>
