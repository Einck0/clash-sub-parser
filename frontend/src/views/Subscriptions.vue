<template>
  <section class="page subscriptions-page p-2 space-y-6">
    <!-- Top Header -->
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 pb-4 border-b border-border-subtle">
      <div>
        <p class="text-xs font-mono text-accent uppercase tracking-wider">Subscription Control Plane</p>
        <h2 class="text-xl font-bold text-text-main tracking-tight">订阅管理</h2>
        <p class="text-xs text-text-muted mt-1">
          拉取订阅时保留上游流量与到期响应头，支持多源合并、初筛正则、精修筛选与重命名。
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2.5 w-full sm:w-auto">
        <Button
          variant="primary"
          size="md"
          class="flex-1 sm:flex-initial min-h-[44px]"
          :icon="Plus"
          @click="openCreate"
        >
          添加订阅
        </Button>
        <Button
          data-testid="add-manual-node"
          variant="secondary"
          size="md"
          class="flex-1 sm:flex-initial min-h-[44px]"
          :icon="Plus"
          @click="openManualNode"
        >
          添加自定义节点
        </Button>
        <Button
          variant="secondary"
          size="md"
          class="flex-1 sm:flex-initial min-h-[44px]"
          :disabled="loading"
          :loading="loading"
          :icon="RefreshCw"
          @click="load"
        >
          {{ loading ? '刷新中…' : '刷新' }}
        </Button>
      </div>
    </div>

    <!-- Alert / Error Message -->
    <UiState v-if="error" type="error" title="订阅加载失败" :description="error" compact>
      <template #actions>
        <Button variant="primary" size="sm" class="min-h-[44px]" @click="load">
          重试
        </Button>
      </template>
    </UiState>

    <UiState
      v-if="loading && !subscriptions.length"
      type="loading"
      title="正在加载订阅"
      description="正在读取订阅列表和节点缓存，请稍等。"
    />

    <template v-else>
      <!-- Industrial MetricCards Grid (Responsive: 1 col on mobile, 2 cols on sm, 3 cols on md+) -->
      <div v-if="subscriptions.length" class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-3 sm:gap-4">
        <MetricCard
          label="TOTAL SUBSCRIPTIONS"
          :value="subscriptions.length"
          :subtext="`${enabledSubs} 个已启用`"
          status="info"
        />
        <MetricCard
          label="TOTAL NODES"
          :value="totalNodes"
          subtext="已同步节点总数"
          status="success"
        />
        <MetricCard
          label="PRIMARY SUBSCRIPTION"
          :value="primarySub ? primarySub.name : '未设置'"
          :subtext="primarySub ? `${(primarySub.raw_nodes || []).length} 节点` : '点击列表卡片设置'"
          :status="primarySub ? 'warning' : 'neutral'"
        />
      </div>

      <!-- Action Toolbar with Search & Status Filter -->
      <PageToolbar
        v-model="search"
        placeholder="搜索订阅名 / URL / 节点数…"
        :count-text="`${filteredSubscriptions.length} / ${subscriptions.length}`"
      >
        <template #filters>
          <select
            v-model="enabledFilter"
            aria-label="订阅状态筛选"
            class="min-h-[44px] rounded-lg border border-border-subtle bg-surface-base px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono cursor-pointer transition-colors"
          >
            <option value="">全部状态</option>
            <option value="enabled">仅启用</option>
            <option value="disabled">仅禁用</option>
          </select>
        </template>
      </PageToolbar>

      <!-- Empty States -->
      <div v-if="!subscriptions.length && !loading">
        <UiState type="empty" title="暂无订阅" description="添加第一个订阅后，就能拉取节点、查看流量和配置筛选规则。">
          <template #actions>
            <Button variant="primary" size="md" class="min-h-[44px]" :icon="Plus" @click="openCreate">
              添加订阅
            </Button>
          </template>
        </UiState>
      </div>

      <div v-else-if="subscriptions.length && !filteredSubscriptions.length">
        <UiState
          type="empty"
          title="没有匹配的订阅"
          description="换个关键词或清空筛选。"
        >
          <template #actions>
            <Button variant="secondary" size="md" class="min-h-[44px]" @click="clearFilter">
              清空筛选
            </Button>
          </template>
        </UiState>
      </div>

      <div v-else class="space-y-4">
        <!-- Desktop High-Density Structured Table (36px compact rows) -->
        <div class="hidden lg:block overflow-hidden rounded-lg border border-border-subtle bg-surface-base shadow-xs">
          <table class="w-full text-left border-collapse font-mono text-xs">
            <thead>
              <tr class="border-b border-border-subtle bg-surface-hover/50 text-[11px] uppercase tracking-wider text-text-muted select-none">
                <th scope="col" class="py-2.5 px-4 font-semibold w-24">状态</th>
                <th scope="col" class="py-2.5 px-4 font-semibold min-w-[220px]">订阅名称与来源</th>
                <th scope="col" class="py-2.5 px-4 font-semibold w-24 text-right">节点数</th>
                <th scope="col" class="py-2.5 px-4 font-semibold min-w-[200px]">流量配额 (工业语义)</th>
                <th scope="col" class="py-2.5 px-4 font-semibold min-w-[170px]">周期与拉取</th>
                <th scope="col" class="py-2.5 px-4 font-semibold text-right w-64">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border-subtle">
              <tr
                v-for="sub in filteredSubscriptions"
                :key="sub.id"
                class="hover:bg-surface-hover/60 transition-colors h-[48px]"
                :class="[
                  !sub.enabled ? 'opacity-65 bg-surface-base/50' : (sub.is_primary ? 'bg-accent/5' : '')
                ]"
              >
                <!-- Status Badge -->
                <td class="py-2 px-4 whitespace-nowrap">
                  <div class="flex flex-col gap-1 items-start">
                    <StatusBadge
                      v-if="sub.is_primary"
                      type="warning"
                      text="主订阅"
                    />
                    <StatusBadge
                      v-else-if="sub.enabled"
                      type="success"
                      text="已启用"
                    />
                    <StatusBadge
                      v-else
                      type="danger"
                      text="已禁用"
                    />
                    <span v-if="sub.last_fetch_error" class="flex items-center gap-1 text-[10px] text-status-danger truncate max-w-[90px]" :title="sub.last_fetch_error">
                      <span class="h-1.5 w-1.5 rounded-full bg-status-danger shrink-0"></span>
                      失败({{ sub.fetch_failed_count || 1 }})
                    </span>
                    <span v-else class="flex items-center gap-1 text-[10px] text-status-success">
                      <span class="h-1.5 w-1.5 rounded-full bg-status-success shrink-0"></span>
                      正常
                    </span>
                  </div>
                </td>

                <!-- Subscription Name & Source -->
                <td class="py-2 px-4">
                  <div class="space-y-1 min-w-0">
                    <div class="flex items-center gap-2">
                      <strong class="text-text-main font-semibold tracking-tight truncate max-w-[260px]" :title="sub.name">
                        {{ sub.name }}
                      </strong>
                      <a
                        v-if="sub.profile_web_page_url"
                        :href="sub.profile_web_page_url"
                        target="_blank"
                        rel="noreferrer"
                        class="text-accent hover:underline inline-flex items-center gap-0.5 text-[10px]"
                        title="打开订阅官网"
                      >
                        <ExternalLink class="h-3 w-3" aria-hidden="true" />
                        <span>官网</span>
                      </a>
                    </div>
                    <div class="text-[11px] text-text-muted truncate max-w-[340px] select-all" :title="sub.url">
                      {{ short(sub.url, 56) }}
                    </div>
                    <!-- Selection Filter Badges -->
                    <div class="flex items-center gap-1.5 text-[10px] text-text-sub flex-wrap">
                      <span class="px-1.5 py-0.5 rounded bg-surface-active border border-border-subtle">
                        候选: <span class="tabular-nums">{{ (sub.source_nodes || []).length || (sub.raw_nodes || []).length }}</span>
                      </span>
                      <span v-if="(sub.filter_regex || []).length" class="px-1.5 py-0.5 rounded bg-surface-active border border-border-subtle">
                        正则: <span class="tabular-nums">{{ sub.filter_regex.length }}</span>
                      </span>
                      <span v-if="(sub.include_node_names || []).length" class="px-1.5 py-0.5 rounded bg-surface-active border border-border-subtle">
                        包含: <span class="tabular-nums">{{ sub.include_node_names.length }}</span>
                      </span>
                      <span v-if="(sub.exclude_node_names || []).length" class="px-1.5 py-0.5 rounded bg-surface-active border border-border-subtle">
                        排除: <span class="tabular-nums">{{ sub.exclude_node_names.length }}</span>
                      </span>
                      <span v-if="Object.keys(sub.node_renames || {}).length" class="px-1.5 py-0.5 rounded bg-surface-active border border-border-subtle">
                        改名: <span class="tabular-nums">{{ Object.keys(sub.node_renames).length }}</span>
                      </span>
                    </div>
                  </div>
                </td>

                <!-- Node Count -->
                <td class="py-2 px-4 text-right whitespace-nowrap">
                  <span class="font-bold tabular-nums text-text-main text-sm">
                    {{ (sub.raw_nodes || []).length }}
                  </span>
                  <span class="text-text-muted ml-0.5 text-[11px]">节点</span>
                </td>

                <!-- Traffic Progress (Solid Semantic Bar, No Gradients) -->
                <td class="py-2 px-4">
                  <div v-if="parseUserinfo(sub.subscription_userinfo)" class="space-y-1.5">
                    <div class="flex justify-between items-center text-[11px]">
                      <span class="text-text-main tabular-nums font-medium">{{ trafficSummary(sub) }}</span>
                      <span class="text-accent font-semibold tabular-nums">{{ trafficPercent(sub) }}%</span>
                    </div>
                    <!-- Industrial Semantic Bar -->
                    <div class="h-1.5 w-full rounded-full bg-surface-active overflow-hidden">
                      <div
                        class="h-full duration-150 rounded-full"
                        :class="trafficBarColor(sub)"
                        :style="{ width: `${trafficPercent(sub)}%` }"
                      />
                    </div>
                    <div class="flex justify-between items-center text-[10px] text-text-muted">
                      <span>已用: <span class="tabular-nums">{{ formatBytes(usedTraffic(sub)) }}</span></span>
                      <span class="text-status-success font-medium">剩余: <span class="tabular-nums">{{ remainingTraffic(sub) }}</span></span>
                    </div>
                  </div>
                  <div v-else class="text-[11px] text-text-sub italic">
                    {{ isManualSubscription(sub) ? '本地手动节点' : '无上游流量头' }}
                  </div>
                </td>

                <!-- Interval & Last Fetched -->
                <td class="py-2 px-4 text-[11px] text-text-muted whitespace-nowrap">
                  <div>过期: <strong class="text-text-main font-medium">{{ expireText(sub) }}</strong></div>
                  <div>周期: <span class="text-text-main tabular-nums">{{ sub.update_interval ? `${sub.update_interval} 分钟` : '手动' }}</span></div>
                  <div>上次: <span class="tabular-nums">{{ sub.last_fetched_at ? formatLocalTime(sub.last_fetched_at) : '-' }}</span></div>
                </td>

                <!-- Action Buttons -->
                <td class="py-2 px-4 text-right whitespace-nowrap">
                  <div class="inline-flex items-center gap-1.5">
                    <Button
                      variant="secondary"
                      size="sm"
                      :icon="Edit2"
                      title="编辑订阅"
                      data-testid="subscription-edit"
                      @click="openEdit(sub)"
                    >
                      编辑
                    </Button>
                    <Button
                      variant="secondary"
                      size="sm"
                      :icon="RefreshCw"
                      :disabled="loadingFetchId === sub.id"
                      :loading="loadingFetchId === sub.id"
                      :title="isManualSubscription(sub) ? '刷新节点' : '拉取订阅'"
                      @click="doFetch(sub.id)"
                    >
                      {{ isManualSubscription(sub) ? '刷新' : '拉取' }}
                    </Button>
                    <Button
                      variant="secondary"
                      size="sm"
                      :icon="ListFilter"
                      title="查看节点与重命名"
                      @click="showNodes(sub)"
                    >
                      节点
                    </Button>
                    <Button
                      v-if="!sub.is_primary"
                      variant="secondary"
                      size="sm"
                      :icon="Crown"
                      :disabled="loadingPrimaryId === sub.id"
                      title="设为主订阅"
                      @click="setPrimary(sub)"
                    >
                      主订阅
                    </Button>
                    <Button
                      variant="secondary"
                      size="sm"
                      :icon="Power"
                      :disabled="loadingToggleId === sub.id"
                      :title="sub.enabled ? '禁用订阅' : '启用订阅'"
                      @click="toggleEnabled(sub)"
                    >
                      {{ sub.enabled ? '禁用' : '启用' }}
                    </Button>
                    <Button
                      variant="danger"
                      size="sm"
                      :icon="Trash2"
                      title="删除订阅"
                      @click="remove(sub)"
                    >
                      删除
                    </Button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Mobile & Tablet Responsive List Cards (data-testid="subscription-card") -->
        <div class="lg:hidden space-y-3">
          <article
            v-for="sub in filteredSubscriptions"
            :key="sub.id"
            data-testid="subscription-card"
            class="rounded-lg border border-border-subtle bg-surface-base p-4 space-y-3 shadow-xs"
            :class="[
              !sub.enabled ? 'opacity-70 bg-surface-base/60' : (sub.is_primary ? 'border-accent/40 bg-accent/5 ring-1 ring-accent/20' : '')
            ]"
          >
            <!-- Card Head -->
            <div class="flex items-start justify-between gap-2 pb-2 border-b border-border-subtle">
              <div class="min-w-0 space-y-1">
                <div class="flex items-center gap-1.5 flex-wrap">
                  <h3 class="text-sm font-semibold text-text-main truncate max-w-[220px]" :title="sub.name">
                    {{ sub.name }}
                  </h3>
                  <StatusBadge v-if="sub.is_primary" type="warning" text="主订阅" />
                  <StatusBadge v-if="!sub.enabled" type="danger" text="已禁用" />
                </div>
                <div class="text-[11px] font-mono text-text-muted truncate select-all" :title="sub.url">
                  {{ short(sub.url, 48) }}
                </div>
              </div>
              <span class="inline-flex items-center px-2 py-0.5 rounded-md text-xs font-mono font-medium bg-accent/10 text-accent border border-accent/20 whitespace-nowrap tabular-nums">
                {{ (sub.raw_nodes || []).length }} 节点
              </span>
            </div>

            <!-- Traffic Section -->
            <div v-if="parseUserinfo(sub.subscription_userinfo)" class="rounded-md border border-border-subtle bg-surface p-3 space-y-2">
              <div class="flex justify-between items-center text-xs font-mono">
                <span class="text-text-main font-medium tabular-nums">{{ trafficSummary(sub) }}</span>
                <span class="text-accent font-semibold tabular-nums">{{ trafficPercent(sub) }}%</span>
              </div>
              <div class="h-1.5 w-full rounded-full bg-surface-active overflow-hidden">
                <div
                  class="h-full duration-150 rounded-full"
                  :class="trafficBarColor(sub)"
                  :style="{ width: `${trafficPercent(sub)}%` }"
                />
              </div>
              <div class="flex justify-between items-center text-[10px] font-mono text-text-muted">
                <span>上传: <span class="tabular-nums">{{ formatBytes(parseUserinfo(sub.subscription_userinfo).upload) }}</span></span>
                <span>下载: <span class="tabular-nums">{{ formatBytes(parseUserinfo(sub.subscription_userinfo).download) }}</span></span>
                <span class="text-status-success font-medium">剩余: <span class="tabular-nums">{{ remainingTraffic(sub) }}</span></span>
              </div>
            </div>
            <div v-else class="text-xs font-mono text-text-sub bg-surface-active/40 rounded-md border border-border-subtle p-2.5">
              {{ isManualSubscription(sub) ? '本地手动节点，编辑后保存即生效' : '暂无上游流量信息' }}
            </div>

            <!-- Meta details grid -->
            <div class="grid grid-cols-2 gap-2 text-xs font-mono text-text-muted py-1 border-t border-b border-border-subtle">
              <div>
                <span class="block text-[10px] text-text-sub uppercase">过期时间</span>
                <span class="text-text-main truncate block">{{ expireText(sub) }}</span>
              </div>
              <div>
                <span class="block text-[10px] text-text-sub uppercase">更新周期</span>
                <span class="text-text-main tabular-nums block">{{ sub.update_interval ? `${sub.update_interval} 分钟` : '-' }}</span>
              </div>
            </div>

            <!-- Action buttons (min-h-[44px] touch target for mobile) -->
            <div class="grid grid-cols-2 sm:grid-cols-3 gap-2 pt-1">
              <Button
                variant="secondary"
                size="md"
                class="min-h-[44px] justify-center"
                data-testid="subscription-edit"
                :icon="Edit2"
                @click="openEdit(sub)"
              >
                编辑
              </Button>
              <Button
                variant="secondary"
                size="md"
                class="min-h-[44px] justify-center"
                :icon="RefreshCw"
                :disabled="loadingFetchId === sub.id"
                :loading="loadingFetchId === sub.id"
                @click="doFetch(sub.id)"
              >
                {{ loadingFetchId === sub.id ? '拉取中…' : (isManualSubscription(sub) ? '刷新节点' : '拉取') }}
              </Button>
              <Button
                variant="secondary"
                size="md"
                class="min-h-[44px] justify-center"
                :icon="ListFilter"
                @click="showNodes(sub)"
              >
                节点
              </Button>
              <Button
                v-if="!sub.is_primary"
                variant="secondary"
                size="md"
                class="min-h-[44px] justify-center"
                :icon="Crown"
                :disabled="loadingPrimaryId === sub.id"
                @click="setPrimary(sub)"
              >
                设为主订阅
              </Button>
              <Button
                variant="secondary"
                size="md"
                class="min-h-[44px] justify-center"
                :icon="Power"
                :disabled="loadingToggleId === sub.id"
                @click="toggleEnabled(sub)"
              >
                {{ sub.enabled ? '禁用' : '启用' }}
              </Button>
              <Button
                variant="danger"
                size="md"
                class="min-h-[44px] justify-center"
                :icon="Trash2"
                @click="remove(sub)"
              >
                删除
              </Button>
            </div>
          </article>
        </div>
      </div>
    </template>

    <!-- Node Preview BaseDrawer (Adaptive Bottom Sheet on Mobile with pb-safe) -->
    <BaseDrawer
      :model-value="viewingSub !== null"
      :title="`${nodePreviewTitle || '节点预览'} (${viewingNodes.length})`"
      @close="closeNodePreview"
    >
      <div class="space-y-4">
        <p class="text-xs text-text-muted">
          支持搜索、TCP 端口探活与出口能力质检；可直接修改前缀后的最终节点名。
        </p>
        <NodePreviewList
          :nodes="viewingBaseNodes"
          :collapsed-limit="60"
          :auto-geo="true"
          :editable="true"
          :renames="viewingSub ? (viewingSub.node_renames || {}) : {}"
          :saving="renamingSaving"
          @save-renames="saveNodeRenames"
        />
      </div>
    </BaseDrawer>

    <!-- Subscription Form BaseDrawer (Adaptive Bottom Sheet on Mobile with pb-safe) -->
    <BaseDrawer
      :model-value="showForm"
      :title="editing?.id ? '编辑订阅' : '添加订阅'"
      @close="showForm = false"
    >
      <SubscriptionForm
        :subscription="editing"
        @save="save"
        @cancel="showForm = false"
        @fetched="onFormFetched"
      />
    </BaseDrawer>

    <!-- Manual Node Editor BaseDrawer (Adaptive Bottom Sheet on Mobile with pb-safe) -->
    <BaseDrawer
      :model-value="showManualNode"
      title="添加自定义节点"
      @close="showManualNode = false"
    >
      <ManualNodeEditor
        :key="manualEditorKey"
        :saving="manualSaving"
        :error="manualFormError"
        @save="saveManualNode"
        @cancel="showManualNode = false"
      />
    </BaseDrawer>
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import {
  Plus,
  RefreshCw,
  Edit2,
  Trash2,
  ListFilter,
  Crown,
  Power,
  ExternalLink
} from 'lucide-vue-next'
import { useAppStore } from '../stores/app'
import { useUrlState } from '../utils/urlState'
import { formatBytes, short, formatLocalTime } from '../utils/format'
import ManualNodeEditor from '../components/ManualNodeEditor.vue'
import NodePreviewList from '../components/NodePreviewList.vue'
import PageToolbar from '../components/PageToolbar.vue'
import SubscriptionForm from '../components/SubscriptionForm.vue'
import UiState from '../components/UiState.vue'
import { Button, StatusBadge, MetricCard, BaseDrawer } from '../components/ui'
import {
  createManualNodeSubscription,
  createSubscription,
  deleteSubscription,
  fetchSubscription,
  getApiErrorMessage,
  getSubscriptionNodes,
  getSubscriptions,
  updateSubscription,
} from '../api'

const store = useAppStore()

const subscriptions = ref([])
const viewingNodes = ref([])
const viewingSub = ref(null)
const nodePreviewTitle = ref('')
const showForm = ref(false)
const editing = ref(null)
const showManualNode = ref(false)
const manualEditorKey = ref(0)
const manualFormError = ref('')
const manualSaving = ref(false)
const loadingFetchId = ref(null)
const loadingPrimaryId = ref(null)
const loadingToggleId = ref(null)
const renamingSaving = ref(false)
const loading = ref(false)
const error = ref('')
const search = useUrlState('q', '')
const enabledFilter = useUrlState('status', '')

const filteredSubscriptions = computed(() => {
  const q = String(search.value || '').trim().toLowerCase()
  return (subscriptions.value || []).filter((sub) => {
    if (enabledFilter.value === 'enabled' && !sub.enabled) return false
    if (enabledFilter.value === 'disabled' && sub.enabled) return false
    if (!q) return true
    const hay = [sub.name, sub.url, String((sub.raw_nodes || []).length)]
      .join(' ')
      .toLowerCase()
    return hay.includes(q)
  })
})

const totalNodes = computed(() => {
  return (subscriptions.value || []).reduce((acc, s) => acc + (s.raw_nodes?.length || 0), 0)
})

const enabledSubs = computed(() => {
  return (subscriptions.value || []).filter((s) => s.enabled).length
})

const primarySub = computed(() => {
  return (subscriptions.value || []).find((s) => s.is_primary)
})

function clearFilter() {
  search.value = ''
  enabledFilter.value = ''
}

// Build post-prefix base names for rename editor. Keys in node_renames are
// always these base names, never already-renamed display names.
const viewingBaseNodes = computed(() => {
  if (!viewingSub.value) return viewingNodes.value
  const sub = viewingSub.value
  const prefix = resolvePrefix(sub)
  const source = (sub.source_nodes || []).length
    ? sub.source_nodes
    : (sub.manual_nodes || []).length
      ? sub.manual_nodes
      : null
  if (source && source.length) {
    return source.map((node) => {
      const original = String(node?.name || '').trim()
      const baseName = prefix ? `${prefix}-${original}` : original
      return { ...node, name: baseName }
    })
  }
  // Fallback: invert existing renames so editor shows base keys when possible.
  const reverse = {}
  for (const [base, finalName] of Object.entries(sub.node_renames || {})) {
    if (base && finalName) reverse[String(finalName)] = String(base)
  }
  return (viewingNodes.value || []).map((node) => {
    const current = String(node?.name || '').trim()
    const baseName = reverse[current] || current
    return { ...node, name: baseName }
  })
})

function isManualSubscription(sub) {
  return sub?.url === 'manual://nodes'
}

function resolvePrefix(sub) {
  const custom = String(sub?.node_prefix || '').trim()
  if (custom) return custom
  if (sub?.is_primary) return ''
  return String(sub?.name || '').trim()
}

onMounted(load)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const data = await getSubscriptions()
    subscriptions.value = (Array.isArray(data) ? data : data?.data) || []
  } catch (err) {
    error.value = getApiErrorMessage(err, '加载订阅列表失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  showForm.value = true
}

function openEdit(sub) {
  editing.value = sub
  showForm.value = true
}

function openManualNode() {
  manualEditorKey.value += 1
  manualFormError.value = ''
  showManualNode.value = true
}

async function save(payload) {
  const isCreate = !editing.value?.id
  const actionText = isCreate ? '创建订阅' : '保存订阅'
  try {
    let saved
    if (isCreate) {
      saved = await createSubscription(payload)
    } else {
      saved = await updateSubscription(editing.value.id, payload)
    }
    showForm.value = false
    editing.value = null
    await load()
    store.toast(`${actionText}成功`, 'success')
    return saved
  } catch (err) {
    store.error(getApiErrorMessage(err, `${actionText}失败`))
    throw err
  }
}

async function saveManualNode(payload) {
  manualSaving.value = true
  manualFormError.value = ''
  try {
    const created = await createManualNodeSubscription({
      name: payload.name,
      manual_nodes: payload.manual_nodes,
      enabled: payload.enabled,
    })
    showManualNode.value = false
    await load()
    const createdName = (created && typeof created === 'object' && 'name' in created) ? created.name : payload.name
    store.toast(`手动节点订阅「${createdName}」已添加`, 'success')
  } catch (err) {
    manualFormError.value = getApiErrorMessage(err, '保存手动节点失败')
  } finally {
    manualSaving.value = false
  }
}

async function onFormFetched(saved) {
  await load()
  if (saved?.id) {
    const fresh = subscriptions.value.find((s) => s.id === saved.id)
    if (fresh) editing.value = fresh
  }
}

async function doFetch(id) {
  loadingFetchId.value = id
  try {
    await fetchSubscription(id)
    await load()
    store.toast('拉取完成', 'success')
  } catch (err) {
    store.error(getApiErrorMessage(err, '拉取订阅失败'))
  } finally {
    loadingFetchId.value = null
  }
}

async function setPrimary(sub) {
  if (sub.is_primary) return
  loadingPrimaryId.value = sub.id
  try {
    await updateSubscription(sub.id, { is_primary: true })
    await load()
    store.toast(`已将「${sub.name}」设为主订阅`, 'success')
  } catch (err) {
    store.error(getApiErrorMessage(err, '设为主订阅失败'))
  } finally {
    loadingPrimaryId.value = null
  }
}

async function toggleEnabled(sub) {
  loadingToggleId.value = sub.id
  try {
    await updateSubscription(sub.id, { enabled: !sub.enabled })
    await load()
    store.toast(sub.enabled ? '已禁用' : '已启用', 'success')
  } catch (err) {
    store.error(getApiErrorMessage(err, '切换状态失败'))
  } finally {
    loadingToggleId.value = null
  }
}

async function remove(sub) {
  const ok = await store.confirm({
    title: '删除订阅',
    message: `确定要删除订阅「${sub.name}」吗？关联的节点缓存也将被清除。`,
    confirmText: '删除',
    danger: true,
  })
  if (!ok) return
  try {
    await deleteSubscription(sub.id)
    await load()
    store.toast('已删除', 'success')
  } catch (err) {
    store.error(getApiErrorMessage(err, '删除订阅失败'))
  }
}

async function showNodes(sub) {
  viewingSub.value = sub
  nodePreviewTitle.value = sub.name
  try {
    const data = await getSubscriptionNodes(sub.id)
    viewingNodes.value = (Array.isArray(data) ? data : data?.data) || []
  } catch (err) {
    store.error(getApiErrorMessage(err, '获取节点列表失败'))
  }
}

function closeNodePreview() {
  viewingSub.value = null
  viewingNodes.value = []
  nodePreviewTitle.value = ''
}

async function saveNodeRenames(renames) {
  if (!viewingSub.value) return
  renamingSaving.value = true
  try {
    await updateSubscription(viewingSub.value.id, { node_renames: renames })
    viewingSub.value = { ...viewingSub.value, node_renames: renames }
    store.toast('重命名规则已保存', 'success')
    const match = subscriptions.value.find((s) => s.id === viewingSub.value.id)
    if (match) match.node_renames = renames
  } catch (err) {
    store.error(getApiErrorMessage(err, '保存节点名称失败'))
  } finally {
    renamingSaving.value = false
  }
}

function parseUserinfo(value) {
  if (!value) return null
  const cacheKey = String(value)
  if (_userinfoCache.has(cacheKey)) return _userinfoCache.get(cacheKey)
  const result = {}
  for (const part of String(value).split(';')) {
    const [key, raw] = part.split('=').map((item) => item && item.trim())
    if (!key || raw === undefined) continue
    const number = Number(raw)
    result[key] = Number.isFinite(number) ? number : raw
  }
  const parsed = Object.keys(result).length ? result : null
  _userinfoCache.set(cacheKey, parsed)
  return parsed
}
const _userinfoCache = new Map()

function usedTraffic(sub) {
  const info = parseUserinfo(sub.subscription_userinfo)
  return Number(info?.upload || 0) + Number(info?.download || 0)
}

function trafficPercent(sub) {
  const info = parseUserinfo(sub.subscription_userinfo)
  const total = Number(info?.total || 0)
  if (!total) return 0
  return Math.min(100, Math.round((usedTraffic(sub) / total) * 1000) / 10)
}

function trafficSummary(sub) {
  const info = parseUserinfo(sub.subscription_userinfo)
  if (!info) return '-'
  return `${formatBytes(usedTraffic(sub))} / ${formatBytes(info.total)}`
}

function remainingTraffic(sub) {
  const info = parseUserinfo(sub.subscription_userinfo)
  if (!info?.total) return '-'
  return formatBytes(Math.max(0, Number(info.total) - usedTraffic(sub)))
}

function trafficBarColor(sub) {
  const percent = trafficPercent(sub)
  if (percent >= 90) return 'bg-status-danger'
  if (percent >= 75) return 'bg-status-warning'
  return 'bg-accent'
}

function expireText(sub) {
  const info = parseUserinfo(sub.subscription_userinfo)
  const expire = Number(info?.expire || 0)
  if (!expire) return '-'
  const date = new Date(expire * 1000)
  const days = Math.ceil((date.getTime() - Date.now()) / 86400000)
  const suffix = days >= 0 ? `剩 ${days} 天` : `已过期 ${Math.abs(days)} 天`
  return `${date.toLocaleDateString()}（${suffix}）`
}
</script>
