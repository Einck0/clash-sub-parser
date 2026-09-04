<template>
  <section class="space-y-6">
    <!-- Top Header -->
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 pb-4 border-b border-white/10">
      <div>
        <p class="text-xs font-mono text-blue-400 uppercase tracking-wider">Subscription Management</p>
        <h2 class="text-xl font-bold text-white tracking-tight">订阅管理</h2>
        <p class="text-xs text-slate-400 mt-1">
          拉取订阅时会保留上游流量与到期响应头，支持多源合并、初筛正则与重命名。
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2.5 w-full sm:w-auto">
        <button
          class="inline-flex items-center justify-center min-h-[44px] gap-1.5 rounded-lg border border-blue-500/40 bg-blue-600/20 px-4 py-2 text-xs font-medium text-blue-400 hover:bg-blue-600/30 transition-colors cursor-pointer flex-1 sm:flex-initial"
          @click="openCreate"
        >
          添加订阅
        </button>
        <button
          data-testid="add-manual-node"
          class="inline-flex items-center justify-center min-h-[44px] gap-1.5 rounded-lg border border-white/10 bg-slate-800/40 px-3.5 py-2 text-xs font-medium text-slate-300 hover:bg-slate-800 transition-colors cursor-pointer flex-1 sm:flex-initial"
          @click="openManualNode"
        >
          添加自定义节点
        </button>
        <button
          class="inline-flex items-center justify-center min-h-[44px] gap-1.5 rounded-lg border border-white/10 bg-slate-800/40 px-3.5 py-2 text-xs font-medium text-slate-300 hover:bg-slate-800 transition-colors cursor-pointer flex-1 sm:flex-initial"
          :disabled="loading"
          @click="load"
        >
          {{ loading ? '刷新中...' : '刷新' }}
        </button>
      </div>
    </div>

    <!-- Alert / Error Message -->
    <UiState v-if="error" type="error" title="订阅加载失败" :description="error" compact>
      <template #actions>
        <button
          class="min-h-[44px] px-4 py-2 rounded-lg bg-blue-600 text-white text-xs font-medium cursor-pointer"
          @click="load"
        >
          重试
        </button>
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
            class="min-h-[44px] rounded-lg border border-white/10 bg-slate-950/60 px-3 py-2 text-xs text-slate-300 focus:border-blue-500 focus:outline-hidden font-mono cursor-pointer"
          >
            <option value="">全部状态</option>
            <option value="enabled">仅启用</option>
            <option value="disabled">仅禁用</option>
          </select>
        </template>
      </PageToolbar>

      <!-- Subscriptions Grid (Responsive: 1 col on mobile, 2 cols on lg) -->
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <article
          v-for="sub in filteredSubscriptions"
          :key="sub.id"
          class="flex flex-col justify-between p-4 sm:p-5 rounded-xl border transition-all backdrop-blur-md space-y-4"
          :class="[
            !sub.enabled
              ? 'border-white/5 bg-slate-900/30 opacity-75'
              : sub.is_primary
                ? 'border-amber-500/40 bg-slate-900/70 shadow-xs shadow-amber-500/10 ring-1 ring-amber-500/20'
                : 'border-white/10 bg-slate-900/60 hover:border-white/20'
          ]"
          data-testid="subscription-card"
        >
          <!-- Card Header -->
          <div class="flex items-start justify-between gap-3 pb-3 border-b border-white/5">
            <div class="min-w-0 flex-1 space-y-1">
              <div class="flex items-center gap-2 flex-wrap">
                <h3 class="text-base font-semibold text-white tracking-tight truncate" :title="sub.name">
                  {{ sub.name }}
                </h3>
                <span
                  v-if="sub.is_primary"
                  class="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-mono font-medium border border-amber-500/40 bg-amber-500/10 text-amber-300"
                >
                  主订阅
                </span>
                <span
                  v-if="!sub.enabled"
                  class="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-mono font-medium border border-rose-500/40 bg-rose-500/10 text-rose-300"
                >
                  已禁用
                </span>
              </div>
              <div class="text-xs font-mono text-slate-400 truncate" :title="sub.url">
                {{ short(sub.url, 72) }}
              </div>
            </div>
            <span class="inline-flex items-center px-2.5 py-1 rounded-lg text-xs font-mono font-medium bg-blue-500/10 text-blue-400 border border-blue-500/20 whitespace-nowrap">
              {{ (sub.raw_nodes || []).length }} 节点
            </span>
          </div>

          <!-- Traffic Details (if userinfo exists) -->
          <div v-if="parseUserinfo(sub.subscription_userinfo)" class="rounded-xl border border-white/5 bg-slate-950/50 p-3.5 space-y-2.5">
            <div class="flex items-center justify-between text-xs font-mono">
              <span class="text-white font-medium">{{ trafficSummary(sub) }}</span>
              <span class="text-blue-400 font-semibold">{{ trafficPercent(sub) }}%</span>
            </div>
            <div class="h-1.5 w-full rounded-full bg-slate-800 overflow-hidden">
              <div
                class="h-full bg-gradient-to-r from-blue-500 to-cyan-400 rounded-full transition-all duration-300"
                :style="{ width: `${trafficPercent(sub)}%` }"
              />
            </div>
            <div class="flex flex-wrap items-center justify-between gap-2 text-[11px] font-mono text-slate-400 pt-0.5">
              <span>上传: {{ formatBytes(parseUserinfo(sub.subscription_userinfo).upload) }}</span>
              <span>下载: {{ formatBytes(parseUserinfo(sub.subscription_userinfo).download) }}</span>
              <span class="text-emerald-400 font-medium">剩余: {{ remainingTraffic(sub) }}</span>
            </div>
          </div>
          <div v-else class="text-xs font-mono text-slate-500 bg-slate-950/30 rounded-xl border border-white/5 p-3">
            {{ isManualSubscription(sub) ? '本地手动节点，编辑后保存即生效' : '暂无流量信息，拉取成功后如果上游返回 header 会显示在这里' }}
          </div>

          <!-- Sub Selection Filter Badges -->
          <div class="flex flex-wrap gap-1.5 text-xs font-mono">
            <span class="px-2 py-0.5 rounded bg-slate-800/60 border border-white/5 text-slate-300">
              候选: {{ (sub.source_nodes || []).length || (sub.raw_nodes || []).length }}
            </span>
            <span class="px-2 py-0.5 rounded bg-slate-800/60 border border-white/5 text-slate-300">
              正则: {{ (sub.filter_regex || []).length || '全选' }}
            </span>
            <span class="px-2 py-0.5 rounded bg-slate-800/60 border border-white/5 text-slate-300">
              包含: {{ (sub.include_node_names || []).length }}
            </span>
            <span class="px-2 py-0.5 rounded bg-slate-800/60 border border-white/5 text-slate-300">
              排除: {{ (sub.exclude_node_names || []).length }}
            </span>
            <span class="px-2 py-0.5 rounded bg-slate-800/60 border border-white/5 text-slate-300">
              重命名: {{ Object.keys(sub.node_renames || {}).length }}
            </span>
          </div>

          <!-- Sub Info Grid -->
          <div class="grid grid-cols-2 sm:grid-cols-3 gap-2.5 py-2.5 border-t border-b border-white/5 text-xs font-mono">
            <div>
              <span class="block text-[10px] text-slate-500 uppercase">过期时间</span>
              <strong class="text-slate-300 font-medium truncate block">{{ expireText(sub) }}</strong>
            </div>
            <div>
              <span class="block text-[10px] text-slate-500 uppercase">更新周期</span>
              <strong class="text-slate-300 font-medium truncate block">{{ sub.update_interval ? `${sub.update_interval} 分钟` : '-' }}</strong>
            </div>
            <div class="col-span-2 sm:col-span-1">
              <span class="block text-[10px] text-slate-500 uppercase">{{ isManualSubscription(sub) ? '本地更新' : '上次拉取' }}</span>
              <strong class="text-slate-300 font-medium truncate block">{{ sub.last_fetched_at ? formatLocalTime(sub.last_fetched_at) : '-' }}</strong>
            </div>
          </div>

          <!-- Fetch Status Line -->
          <div class="flex items-center justify-between text-xs font-mono">
            <span v-if="sub.last_fetch_error" class="text-rose-400 truncate" :title="sub.last_fetch_error">
              失败 {{ sub.fetch_failed_count || 1 }} 次: {{ short(sub.last_fetch_error, 50) }}
            </span>
            <span v-else class="text-emerald-400 flex items-center gap-1.5">
              <span class="h-1.5 w-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
              {{ isManualSubscription(sub) ? '本地节点正常' : '拉取正常' }}
            </span>
            <a
              v-if="sub.profile_web_page_url"
              :href="sub.profile_web_page_url"
              target="_blank"
              rel="noreferrer"
              class="text-blue-400 hover:text-blue-300 hover:underline transition-colors"
            >
              订阅主页 ↗
            </a>
          </div>

          <!-- Action Buttons (Responsive: min 44px touch height) -->
          <div class="grid grid-cols-2 sm:grid-cols-3 md:flex md:flex-wrap items-center gap-2 pt-2 border-t border-white/5">
            <button
              class="min-h-[44px] px-3 py-2 rounded-lg border border-blue-500/40 bg-blue-600/20 text-xs font-medium text-blue-400 hover:bg-blue-600/30 transition-colors cursor-pointer flex-1 justify-center"
              data-testid="subscription-edit"
              @click="openEdit(sub)"
            >
              编辑
            </button>
            <button
              class="min-h-[44px] px-3 py-2 rounded-lg border border-white/10 bg-slate-800/40 text-xs font-medium text-slate-300 hover:bg-slate-800 transition-colors cursor-pointer flex-1 justify-center"
              :disabled="loadingFetchId === sub.id"
              @click="doFetch(sub.id)"
            >
              {{ loadingFetchId === sub.id ? (isManualSubscription(sub) ? '刷新中...' : '拉取中...') : (isManualSubscription(sub) ? '刷新节点' : '拉取') }}
            </button>
            <button
              class="min-h-[44px] px-3 py-2 rounded-lg border border-white/10 bg-slate-800/40 text-xs font-medium text-slate-300 hover:bg-slate-800 transition-colors cursor-pointer flex-1 justify-center"
              @click="showNodes(sub)"
            >
              节点
            </button>
            <button
              class="min-h-[44px] px-3 py-2 rounded-lg border text-xs font-medium transition-colors cursor-pointer flex-1 justify-center"
              :class="[
                sub.is_primary
                  ? 'border-amber-500/30 bg-amber-500/10 text-amber-300 cursor-default'
                  : 'border-white/10 bg-slate-800/40 text-slate-300 hover:bg-slate-800'
              ]"
              :disabled="sub.is_primary || loadingPrimaryId === sub.id"
              @click="setPrimary(sub)"
            >
              {{ sub.is_primary ? '当前主订阅' : (loadingPrimaryId === sub.id ? '设置中...' : '设为主订阅') }}
            </button>
            <button
              class="min-h-[44px] px-3 py-2 rounded-lg border text-xs font-medium transition-colors cursor-pointer flex-1 justify-center"
              :class="[
                sub.enabled
                  ? 'border-white/10 bg-slate-800/40 text-slate-300 hover:bg-slate-800'
                  : 'border-emerald-500/40 bg-emerald-600/20 text-emerald-400 hover:bg-emerald-600/30'
              ]"
              :disabled="loadingToggleId === sub.id"
              @click="toggleEnabled(sub)"
            >
              {{ loadingToggleId === sub.id ? '...' : (sub.enabled ? '禁用' : '启用') }}
            </button>
            <button
              class="min-h-[44px] px-3 py-2 rounded-lg border border-rose-500/40 bg-rose-600/20 text-xs font-medium text-rose-400 hover:bg-rose-600/30 transition-colors cursor-pointer flex-1 justify-center"
              @click="remove(sub)"
            >
              删除
            </button>
          </div>
        </article>

        <!-- Empty States -->
        <div v-if="!subscriptions.length && !loading" class="col-span-full">
          <UiState type="empty" title="暂无订阅" description="添加第一个订阅后，就能拉取节点、查看流量和配置筛选规则。">
            <template #actions>
              <button
                class="min-h-[44px] px-4 py-2 rounded-lg bg-blue-600 text-white text-xs font-medium cursor-pointer"
                @click="openCreate"
              >
                添加订阅
              </button>
            </template>
          </UiState>
        </div>
        <div v-else-if="subscriptions.length && !filteredSubscriptions.length" class="col-span-full">
          <UiState
            type="empty"
            title="没有匹配的订阅"
            description="换个关键词或清空筛选。"
          >
            <template #actions>
              <button
                class="min-h-[44px] px-4 py-2 rounded-lg border border-white/10 bg-slate-800 text-slate-300 text-xs font-medium cursor-pointer"
                @click="search = ''; enabledFilter = ''"
              >
                清空筛选
              </button>
            </template>
          </UiState>
        </div>
      </div>
    </template>

    <!-- Node Preview Modal (Adaptive Bottom Sheet on Mobile) -->
    <div
      v-if="viewingSub"
      class="fixed inset-0 z-50 flex items-end sm:items-center justify-center bg-black/70 backdrop-blur-sm p-0 sm:p-4"
      @click.self="closeNodePreview"
    >
      <div
        class="relative flex w-full max-w-4xl flex-col rounded-t-2xl sm:rounded-2xl border border-white/10 bg-[#0F172A] text-[#F8FAFC] shadow-2xl overflow-hidden max-h-[90vh] sm:max-h-[85vh] pb-safe"
        role="dialog"
        aria-modal="true"
        :aria-label="nodePreviewTitle || '节点预览'"
      >
        <!-- Mobile drag indicator -->
        <div class="sm:hidden mx-auto my-2.5 h-1 w-12 rounded-full bg-white/20" aria-hidden="true" />

        <div class="flex items-center justify-between border-b border-white/10 px-6 py-4">
          <div>
            <p class="text-[10px] font-mono tracking-wider text-blue-400 uppercase">Node Preview</p>
            <h3 class="text-base font-semibold text-white tracking-tight">
              {{ nodePreviewTitle || '节点预览' }}（{{ viewingNodes.length }}）
            </h3>
            <p class="text-xs text-slate-400 mt-0.5">
              支持搜索、TCP 端口探活；可点「改名」直接修改前缀后的最终节点名。
            </p>
          </div>
          <button
            class="flex min-h-[44px] min-w-[44px] items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white cursor-pointer transition-colors"
            @click="closeNodePreview"
          >
            ✕
          </button>
        </div>
        <div class="p-4 sm:p-6 overflow-y-auto flex-1">
          <NodePreviewList
            :nodes="viewingBaseNodes"
            :collapsed-limit="60"
            :auto-geo="true"
            :editable="true"
            :renames="viewingSub.node_renames || {}"
            :saving="renamingSaving"
            @save-renames="saveNodeRenames"
          />
        </div>
      </div>
    </div>

    <!-- Subscription Form Modal (Adaptive Bottom Sheet on Mobile) -->
    <div
      v-if="showForm"
      class="fixed inset-0 z-50 flex items-end sm:items-center justify-center bg-black/70 backdrop-blur-sm p-0 sm:p-4"
      @click.self="showForm = false"
    >
      <div
        class="relative flex w-full max-w-2xl flex-col rounded-t-2xl sm:rounded-2xl border border-white/10 bg-[#0F172A] text-[#F8FAFC] shadow-2xl overflow-hidden max-h-[90vh] sm:max-h-[85vh] pb-safe p-4 sm:p-6 overflow-y-auto"
        role="dialog"
        aria-modal="true"
      >
        <div class="sm:hidden mx-auto mb-3 h-1 w-12 rounded-full bg-white/20" aria-hidden="true" />
        <SubscriptionForm
          :subscription="editing"
          @save="save"
          @cancel="showForm = false"
          @fetched="onFormFetched"
        />
      </div>
    </div>

    <!-- Manual Node Editor Modal (Adaptive Bottom Sheet on Mobile) -->
    <div
      v-if="showManualNode"
      class="fixed inset-0 z-50 flex items-end sm:items-center justify-center bg-black/70 backdrop-blur-sm p-0 sm:p-4"
      @click.self="showManualNode = false"
    >
      <div
        class="relative flex w-full max-w-2xl flex-col rounded-t-2xl sm:rounded-2xl border border-white/10 bg-[#0F172A] text-[#F8FAFC] shadow-2xl overflow-hidden max-h-[90vh] sm:max-h-[85vh] pb-safe p-4 sm:p-6 overflow-y-auto"
        role="dialog"
        aria-modal="true"
      >
        <div class="sm:hidden mx-auto mb-3 h-1 w-12 rounded-full bg-white/20" aria-hidden="true" />
        <ManualNodeEditor
          :key="manualEditorKey"
          :saving="manualSaving"
          :error="manualFormError"
          @save="saveManualNode"
          @cancel="showManualNode = false"
        />
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useAppStore } from '../stores/app'
import { useUrlState } from '../utils/urlState'
import { formatBytes, short, formatLocalTime } from '../utils/format'
import ManualNodeEditor from '../components/ManualNodeEditor.vue'
import NodePreviewList from '../components/NodePreviewList.vue'
import PageToolbar from '../components/PageToolbar.vue'
import SubscriptionForm from '../components/SubscriptionForm.vue'
import UiState from '../components/UiState.vue'
import MetricCard from '../components/ui/MetricCard.vue'
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

async function copyText(text, msg = '已复制') {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    store.showToast(msg, 'success')
  } catch {
    store.showToast('复制失败', 'error')
  }
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

// Esc to close modals
function onKeydown(e) {
  if (e.key === 'Escape') {
    if (showForm.value) showForm.value = false
    else if (showManualNode.value) showManualNode.value = false
    else if (viewingNodes.value.length) closeNodePreview()
  }
}
window.addEventListener('keydown', onKeydown)
onUnmounted(() => window.removeEventListener('keydown', onKeydown))

async function load() {
  loading.value = true
  error.value = ''
  try {
    const data = await getSubscriptions()
    subscriptions.value = data || []
    store.setSubscriptions(subscriptions.value)
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
    store.toast(`手动节点订阅「${created?.name || payload.name}」已添加`, 'success')
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
    viewingNodes.value = data || []
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
  const cacheKey = value
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
