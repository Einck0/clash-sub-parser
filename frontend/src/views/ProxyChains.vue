<template>
  <section class="page chain-page p-2 space-y-6 min-w-0 max-w-full overflow-x-hidden">
    <!-- Top Header -->
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 pb-4 border-b border-border-subtle">
      <div>
        <p class="text-xs font-mono text-accent uppercase tracking-wider">Proxy Chains</p>
        <h2 class="text-xl font-bold text-text-main tracking-tight">链式代理</h2>
        <p class="text-xs text-text-muted mt-1">
          后置挂跳板。优先级：节点 &gt; 策略组 &gt; 订阅。单节点快捷操作请到「节点」台账。
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2.5 w-full sm:w-auto">
        <Button
          variant="secondary"
          size="md"
          class="flex-1 sm:flex-initial min-h-[44px]"
          :disabled="loading"
          :icon="RefreshCw"
          @click="reload"
        >
          刷新
        </Button>
        <Button
          variant="primary"
          size="md"
          class="flex-1 sm:flex-initial min-h-[44px]"
          :icon="Plus"
          @click="showComposer = !showComposer"
        >
          {{ showComposer ? '收起新建' : '新建绑定' }}
        </Button>
      </div>
    </div>

    <!-- Alert / Error Message -->
    <UiState v-if="error" type="error" title="链式绑定操作失败" :description="error" compact>
      <template #actions>
        <Button variant="primary" size="sm" class="min-h-[44px]" @click="reload">
          重试
        </Button>
      </template>
    </UiState>

    <!-- Industrial MetricCards Grid -->
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-3 sm:gap-4 font-mono">
      <MetricCard
        label="TOTAL BINDINGS"
        :value="bindings.length"
        subtext="已配置链式总数"
        status="neutral"
      />
      <MetricCard
        label="ENABLED"
        :value="enabledCount"
        subtext="生效中链路"
        status="success"
      />
      <MetricCard
        label="SUBSCRIPTION LEVEL"
        :value="subscriptionBindCount"
        subtext="订阅整包跳板"
        status="info"
      />
      <MetricCard
        label="GROUP LEVEL"
        :value="groupBindCount"
        subtext="策略组跳板"
        status="info"
      />
      <MetricCard
        label="NODE LEVEL"
        :value="nodeBindCount"
        subtext="单节点跳板"
        status="info"
      />
    </div>

    <!-- Composer Section (Inline Accordion/Panel) -->
    <div
      v-if="showComposer"
      class="rounded-lg border border-border-subtle bg-surface-base p-4 sm:p-5 space-y-4 shadow-xs"
    >
      <div class="flex items-center justify-between border-b border-border-subtle pb-3">
        <div>
          <h3 class="text-sm font-semibold text-text-main">新建链式绑定</h3>
          <p class="text-xs text-text-muted mt-0.5">先选「给谁挂」，再选「经谁出去」。保存前可预览会挂多少、跳过多少。</p>
        </div>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 lg:gap-6">
        <!-- Step 1: Target (Exit) -->
        <div class="rounded-lg border border-border-subtle bg-surface p-4 space-y-3">
          <div class="flex items-center justify-between">
            <span class="text-xs font-mono font-semibold text-accent uppercase tracking-wider">1. 目标（出口）</span>
            <span class="text-[11px] text-text-sub font-mono">流量最终由其流向目标网站</span>
          </div>

          <!-- Target Type Segmented Control -->
          <div class="grid grid-cols-3 gap-1 rounded-lg bg-surface-base p-1 border border-border-subtle">
            <button
              v-for="opt in targetTypeOptions"
              :key="opt.value"
              type="button"
              class="min-h-[44px] rounded-md text-xs font-medium transition-colors cursor-pointer"
              :class="form.target_type === opt.value ? 'bg-accent text-white font-semibold' : 'text-text-muted hover:text-text-main'"
              @click="setTargetType(opt.value)"
            >
              {{ opt.label }}
            </button>
          </div>

          <!-- Target Select Controls -->
          <div v-if="form.target_type === 'node'" class="space-y-2">
            <label class="block text-xs font-medium text-text-muted">选择目标节点</label>
            <input
              v-model="nodeSearch"
              placeholder="过滤节点名…"
              class="w-full min-h-[44px] rounded-md border border-border-subtle bg-surface-base px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono"
            />
            <select
              v-model="form.target_name"
              aria-label="选择最终节点"
              class="w-full min-h-[44px] rounded-md border border-border-subtle bg-surface-base px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono"
            >
              <option value="">选择最终节点</option>
              <option v-for="n in filteredTargetNodes" :key="n.name" :value="n.name">
                {{ n.name }}{{ n.subscription_name ? ` · ${n.subscription_name}` : '' }}
              </option>
            </select>
          </div>

          <div v-else-if="form.target_type === 'node_group'" class="space-y-2">
            <label class="block text-xs font-medium text-text-muted">选择目标策略组</label>
            <input
              v-model="groupSearch"
              placeholder="过滤策略组…"
              class="w-full min-h-[44px] rounded-md border border-border-subtle bg-surface-base px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono"
            />
            <select
              v-model.number="form.target_id"
              aria-label="选择目标策略组"
              class="w-full min-h-[44px] rounded-md border border-border-subtle bg-surface-base px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono"
            >
              <option :value="null">选择策略组</option>
              <option v-for="g in filteredGroups" :key="g.id" :value="g.id">{{ g.name }}</option>
            </select>
          </div>

          <div v-else class="space-y-2">
            <label class="block text-xs font-medium text-text-muted">选择目标订阅</label>
            <select
              v-model.number="form.target_id"
              aria-label="选择目标订阅"
              class="w-full min-h-[44px] rounded-md border border-border-subtle bg-surface-base px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono"
            >
              <option :value="null">选择订阅</option>
              <option v-for="s in subscriptions" :key="s.id" :value="s.id">{{ s.name }}</option>
            </select>
          </div>
        </div>

        <!-- Step 2: Dialer (Entry / Jump) -->
        <div class="rounded-lg border border-border-subtle bg-surface p-4 space-y-3">
          <div class="flex items-center justify-between">
            <span class="text-xs font-mono font-semibold text-accent uppercase tracking-wider">2. 跳板（入口）</span>
            <span class="text-[11px] text-text-sub font-mono">前置代理 / 流量先经此跳板发出</span>
          </div>

          <!-- Dialer Type Segmented Control -->
          <div class="grid grid-cols-2 gap-1 rounded-lg bg-surface-base p-1 border border-border-subtle">
            <button
              type="button"
              class="min-h-[44px] rounded-md text-xs font-medium transition-colors cursor-pointer"
              :class="form.dialer_type === 'node_group' ? 'bg-accent text-white font-semibold' : 'text-text-muted hover:text-text-main'"
              @click="form.dialer_type = 'node_group'; form.dialer_ref = ''"
            >
              策略组
            </button>
            <button
              type="button"
              class="min-h-[44px] rounded-md text-xs font-medium transition-colors cursor-pointer"
              :class="form.dialer_type === 'node' ? 'bg-accent text-white font-semibold' : 'text-text-muted hover:text-text-main'"
              @click="form.dialer_type = 'node'; form.dialer_ref = ''"
            >
              节点
            </button>
          </div>

          <div v-if="form.dialer_type === 'node'" class="space-y-2">
            <label class="block text-xs font-medium text-text-muted">选择跳板节点</label>
            <input
              v-model="dialerNodeSearch"
              placeholder="过滤跳板节点…"
              class="w-full min-h-[44px] rounded-md border border-border-subtle bg-surface-base px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono"
            />
            <select
              v-model="form.dialer_ref"
              aria-label="选择跳板节点"
              class="w-full min-h-[44px] rounded-md border border-border-subtle bg-surface-base px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono"
            >
              <option value="">选择节点</option>
              <option v-for="n in filteredDialerNodes" :key="'d-' + n.name" :value="n.name">
                {{ n.name }}
              </option>
            </select>
          </div>

          <div v-else class="space-y-2">
            <label class="block text-xs font-medium text-text-muted">选择跳板策略组</label>
            <input
              v-model="dialerGroupSearch"
              placeholder="过滤跳板组…"
              class="w-full min-h-[44px] rounded-md border border-border-subtle bg-surface-base px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono"
            />
            <select
              v-model="form.dialer_ref"
              aria-label="选择跳板策略组"
              class="w-full min-h-[44px] rounded-md border border-border-subtle bg-surface-base px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono"
            >
              <option value="">选择策略组</option>
              <option v-for="g in filteredDialerGroups" :key="'dg-' + g.id" :value="g.name">
                {{ g.name }}
              </option>
            </select>
          </div>

          <div class="space-y-2">
            <label class="block text-xs font-medium text-text-muted">备注说明（可选）</label>
            <input
              v-model="form.note"
              placeholder="如：HK落地配日本前置…"
              class="w-full min-h-[44px] rounded-md border border-border-subtle bg-surface-base px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono"
            />
          </div>
        </div>
      </div>

      <!-- Actions bar -->
      <div class="flex items-center justify-end gap-2.5 pt-2 border-t border-border-subtle">
        <Button
          variant="secondary"
          size="md"
          class="min-h-[44px]"
          :icon="Eye"
          :disabled="!canCreate || previewing"
          @click="runPreview"
        >
          {{ previewing ? '预览中…' : '预览生效' }}
        </Button>
        <Button
          variant="primary"
          size="md"
          class="min-h-[44px]"
          :disabled="saving || !canCreate"
          @click="createBinding"
        >
          {{ saving ? '保存中…' : '保存绑定' }}
        </Button>
      </div>
    </div>

    <!-- Search Toolbar -->
    <PageToolbar
      v-model="listSearch"
      placeholder="搜索目标 / 跳板 / 备注…"
      :count-text="`${filteredBindings.length} / ${bindings.length} 条绑定`"
    />

    <!-- Empty State -->
    <div v-if="!filteredBindings.length">
      <UiState
        type="empty"
        :title="bindings.length ? '没有匹配的链式绑定' : '暂无链式代理绑定'"
        :description="bindings.length ? '换个搜索关键词试试。' : '链式绑定允许后置挂跳板，实现入口与出口节点分流。'"
      >
        <template #actions>
          <Button
            v-if="!bindings.length"
            variant="primary"
            size="md"
            class="min-h-[44px]"
            :icon="Plus"
            @click="showComposer = true"
          >
            新建绑定
          </Button>
          <Button
            v-else
            variant="secondary"
            size="md"
            class="min-h-[44px]"
            @click="listSearch = ''"
          >
            清空搜索
          </Button>
        </template>
      </UiState>
    </div>

    <div v-else class="space-y-4">
      <!-- High-Density Structured Table (Desktop) -->
      <div class="hidden md:block overflow-hidden rounded-lg border border-border-subtle bg-surface-base shadow-xs">
        <table class="w-full text-left border-collapse font-mono text-xs">
          <thead>
            <tr class="border-b border-border-subtle bg-surface-hover/50 text-[11px] uppercase tracking-wider text-text-muted select-none">
              <th scope="col" class="py-2.5 px-4 font-semibold min-w-[200px]">目标（出口）</th>
              <th scope="col" class="py-2.5 px-3 font-semibold text-center w-24">跳板链路</th>
              <th scope="col" class="py-2.5 px-4 font-semibold min-w-[200px]">跳板（入口）</th>
              <th scope="col" class="py-2.5 px-4 font-semibold">备注</th>
              <th scope="col" class="py-2.5 px-4 font-semibold w-24 text-center">状态</th>
              <th scope="col" class="py-2.5 px-4 font-semibold text-right w-44">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border-subtle">
            <tr
              v-for="item in filteredBindings"
              :key="item.id"
              class="hover:bg-surface-hover/60 transition-colors h-[48px]"
              :class="!item.enabled ? 'opacity-60 bg-surface/40' : ''"
            >
              <!-- Target -->
              <td class="py-2 px-4">
                <div class="flex items-center gap-2">
                  <span class="px-1.5 py-0.5 rounded bg-surface-active text-text-sub border border-border-subtle text-[10px] uppercase font-semibold">
                    {{ targetKind(item) }}
                  </span>
                  <strong class="text-text-main font-semibold tracking-tight truncate max-w-[240px]" :title="targetName(item)">
                    {{ targetName(item) }}
                  </strong>
                </div>
              </td>

              <!-- Flow Arrow / Connector -->
              <td class="py-2 px-3 text-center whitespace-nowrap">
                <span class="inline-flex items-center gap-1 text-[11px] text-accent font-semibold px-2 py-0.5 rounded bg-accent/10 border border-accent/20">
                  via
                  <ArrowRight class="h-3 w-3" aria-hidden="true" />
                </span>
              </td>

              <!-- Dialer -->
              <td class="py-2 px-4">
                <div class="flex items-center gap-2">
                  <span class="px-1.5 py-0.5 rounded bg-accent/10 text-accent border border-accent/20 text-[10px] font-semibold">
                    {{ dialerKind(item) }}
                  </span>
                  <strong class="text-text-main font-semibold tracking-tight truncate max-w-[240px]" :title="item.dialer_ref">
                    {{ item.dialer_ref }}
                  </strong>
                </div>
              </td>

              <!-- Note -->
              <td class="py-2 px-4 text-text-muted truncate max-w-[200px]" :title="item.note || '-'">
                {{ item.note || '-' }}
              </td>

              <!-- Status Badge -->
              <td class="py-2 px-4 text-center whitespace-nowrap">
                <StatusBadge
                  :type="item.enabled ? 'success' : 'neutral'"
                  :text="item.enabled ? '已启用' : '已禁用'"
                />
              </td>

              <!-- Actions -->
              <td class="py-2 px-4 text-right whitespace-nowrap">
                <div class="inline-flex items-center gap-1.5">
                  <Button
                    variant="secondary"
                    size="sm"
                    :disabled="workingId === item.id"
                    @click="toggleEnabled(item)"
                  >
                    {{ workingId === item.id ? '…' : (item.enabled ? '禁用' : '启用') }}
                  </Button>
                  <Button
                    variant="danger"
                    size="sm"
                    :icon="Trash2"
                    :disabled="workingId === item.id"
                    @click="removeBinding(item)"
                  >
                    删除
                  </Button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Mobile Responsive List Cards -->
      <div class="md:hidden space-y-3">
        <article
          v-for="item in filteredBindings"
          :key="item.id"
          class="rounded-lg border border-border-subtle bg-surface-base p-4 space-y-3 shadow-xs font-mono"
          :class="!item.enabled ? 'opacity-70 bg-surface/30' : ''"
        >
          <!-- Flow representation -->
          <div class="space-y-2">
            <div class="flex items-center justify-between gap-2">
              <div class="flex items-center gap-1.5 min-w-0">
                <span class="px-1.5 py-0.5 rounded bg-surface-active text-text-sub border border-border-subtle text-[10px]">
                  {{ targetKind(item) }}
                </span>
                <span class="text-xs font-semibold text-text-main truncate max-w-[180px]">
                  {{ targetName(item) }}
                </span>
              </div>
              <StatusBadge
                :type="item.enabled ? 'success' : 'neutral'"
                :text="item.enabled ? '启用' : '禁用'"
              />
            </div>

            <div class="flex items-center gap-2 pl-2 text-xs text-text-muted border-l-2 border-accent/40 my-1">
              <span class="text-[10px] text-accent font-semibold">via 跳板</span>
              <span class="text-xs font-medium text-text-main truncate">
                {{ item.dialer_ref }}
              </span>
              <span class="text-[10px] text-text-sub">({{ dialerKind(item) }})</span>
            </div>

            <div v-if="item.note" class="text-xs text-text-muted italic pt-1">
              备注：{{ item.note }}
            </div>
          </div>

          <!-- Actions -->
          <div class="flex items-center justify-end gap-2 pt-2 border-t border-border-subtle">
            <Button
              variant="secondary"
              size="md"
              class="flex-1 min-h-[44px] justify-center"
              :disabled="workingId === item.id"
              @click="toggleEnabled(item)"
            >
              {{ workingId === item.id ? '处理中…' : (item.enabled ? '禁用' : '启用') }}
            </Button>
            <Button
              variant="danger"
              size="md"
              class="flex-1 min-h-[44px] justify-center"
              :icon="Trash2"
              :disabled="workingId === item.id"
              @click="removeBinding(item)"
            >
              删除
            </Button>
          </div>
        </article>
      </div>
    </div>

    <!-- Preview BaseDrawer (Adaptive Bottom Sheet on Mobile with pb-safe) -->
    <BaseDrawer
      :model-value="showPreviewModal && !!preview"
      title="链式代理生效预览"
      @close="closePreviewModal"
    >
      <div v-if="preview" class="space-y-4 text-xs font-mono">
        <!-- Preview Stats -->
        <div class="grid grid-cols-3 gap-2 p-3 rounded-lg bg-surface border border-border-subtle text-center">
          <div>
            <div class="text-[11px] text-text-sub uppercase">目标匹配</div>
            <div class="text-base font-bold text-text-main tabular-nums mt-0.5">{{ preview.target_count }}</div>
          </div>
          <div>
            <div class="text-[11px] text-status-success uppercase font-semibold">挂链生效</div>
            <div class="text-base font-bold text-status-success tabular-nums mt-0.5">{{ preview.chain_count }}</div>
          </div>
          <div>
            <div class="text-[11px] text-text-muted uppercase">跳过忽略</div>
            <div class="text-base font-bold text-text-muted tabular-nums mt-0.5">{{ preview.skip_count }}</div>
          </div>
        </div>

        <!-- Samples: Chain -->
        <div v-if="preview.chain_samples?.length" class="space-y-2">
          <strong class="text-text-main flex items-center gap-1.5">
            <span class="inline-block w-2 h-2 rounded-full bg-status-success"></span>
            挂链样例节点
          </strong>
          <div class="flex flex-wrap gap-1.5 p-2.5 rounded-lg bg-surface border border-border-subtle">
            <span
              v-for="sample in preview.chain_samples.slice(0, 16)"
              :key="sample"
              class="px-1.5 py-0.5 rounded bg-surface-active text-text-main border border-border-subtle text-[11px]"
            >
              {{ sample }}
            </span>
          </div>
        </div>

        <!-- Samples: Skipped -->
        <div v-if="preview.skip_samples?.length" class="space-y-2">
          <strong class="text-text-muted flex items-center gap-1.5">
            <span class="inline-block w-2 h-2 rounded-full bg-text-sub"></span>
            跳过样例节点
          </strong>
          <div class="flex flex-wrap gap-1.5 p-2.5 rounded-lg bg-surface border border-border-subtle">
            <span
              v-for="sample in preview.skip_samples.slice(0, 16)"
              :key="sample"
              class="px-1.5 py-0.5 rounded bg-surface-active text-text-sub border border-border-subtle text-[11px]"
            >
              {{ sample }}
            </span>
          </div>
        </div>

        <div v-if="!preview.chain_samples?.length && !preview.skip_samples?.length" class="text-text-sub italic py-4 text-center">
          没有可展示的样例节点。
        </div>
      </div>
    </BaseDrawer>
  </section>
</template>

<script setup>
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { Plus, RefreshCw, Trash2, ArrowRight, Eye } from 'lucide-vue-next'
import {
  createProxyChain,
  deleteProxyChain,
  getApiErrorMessage,
  getFinalNodes,
  getNodeGroups,
  getProxyChains,
  getSubscriptions,
  previewProxyChain,
  updateProxyChain,
} from '../api'
import PageToolbar from '../components/PageToolbar.vue'
import UiState from '../components/UiState.vue'
import { Button, StatusBadge, MetricCard, BaseDrawer } from '../components/ui'
import { useAppStore } from '../stores/app'
import { useUrlState } from '../utils/urlState'

const store = useAppStore()

const loading = ref(false)
const saving = ref(false)
const previewing = ref(false)
const error = ref('')
const showComposer = ref(false)
const bindings = ref([])
const finalNodes = ref([])
const nodeGroups = ref([])
const subscriptions = ref([])
const preview = ref(null)
const showPreviewModal = ref(false)
const listSearch = useUrlState('q', '')
const workingId = ref(null)

const nodeSearch = ref('')
const groupSearch = ref('')
const dialerNodeSearch = ref('')
const dialerGroupSearch = ref('')

const targetTypeOptions = [
  { value: 'subscription', label: '订阅' },
  { value: 'node_group', label: '策略组' },
  { value: 'node', label: '节点' },
]

const form = reactive({
  target_type: 'subscription',
  target_id: null,
  target_name: '',
  dialer_type: 'node_group',
  dialer_ref: '',
  note: '',
  enabled: true,
})

const canCreate = computed(() => {
  if (!form.dialer_ref) return false
  if (form.target_type === 'node') return !!form.target_name
  return form.target_id != null
})

const enabledCount = computed(() => bindings.value.filter((b) => b.enabled).length)
const subscriptionBindCount = computed(
  () => bindings.value.filter((b) => b.target_type === 'subscription').length,
)
const groupBindCount = computed(
  () => bindings.value.filter((b) => b.target_type === 'node_group').length,
)
const nodeBindCount = computed(() => bindings.value.filter((b) => b.target_type === 'node').length)

const filteredTargetNodes = computed(() => filterNodes(finalNodes.value, nodeSearch.value))
const filteredDialerNodes = computed(() => filterNodes(finalNodes.value, dialerNodeSearch.value))
const filteredGroups = computed(() => filterGroups(nodeGroups.value, groupSearch.value))
const filteredDialerGroups = computed(() => filterGroups(nodeGroups.value, dialerGroupSearch.value))

const filteredBindings = computed(() => {
  const q = String(listSearch.value || '').trim().toLowerCase()
  if (!q) return bindings.value
  return bindings.value.filter((item) => {
    const hay = [
      item.target_type,
      item.target_name,
      item.target_id,
      item.dialer_type,
      item.dialer_ref,
      item.note,
    ]
      .filter((x) => x != null && x !== '')
      .join(' ')
      .toLowerCase()
    return hay.includes(q)
  })
})

onMounted(async () => {
  await reload()
  if (!bindings.value.length) showComposer.value = true
})

function onKeydown(e) {
  if (e.key === 'Escape' && showPreviewModal.value) {
    closePreviewModal()
  }
}
window.addEventListener('keydown', onKeydown)
onUnmounted(() => window.removeEventListener('keydown', onKeydown))

watch(
  () => [form.target_type, form.target_id, form.target_name, form.dialer_type, form.dialer_ref],
  () => {
    preview.value = null
  },
)

async function reload() {
  loading.value = true
  error.value = ''
  try {
    const [b, n, g, s] = await Promise.all([
      getProxyChains(),
      getFinalNodes(),
      getNodeGroups(),
      getSubscriptions(),
    ])
    bindings.value = b.data || []
    finalNodes.value = n.data || []
    nodeGroups.value = g.data || []
    subscriptions.value = s.data || []
  } catch (err) {
    error.value = getApiErrorMessage(err, '加载失败')
  } finally {
    loading.value = false
  }
}

function filterNodes(list, q) {
  const query = String(q || '').trim().toLowerCase()
  if (!query) return list
  return (list || []).filter((n) => String(n.name || '').toLowerCase().includes(query))
}

function filterGroups(list, q) {
  const query = String(q || '').trim().toLowerCase()
  if (!query) return list
  return (list || []).filter((g) => String(g.name || '').toLowerCase().includes(query))
}

function setTargetType(value) {
  form.target_type = value
  form.target_id = null
  form.target_name = ''
  preview.value = null
}

function targetKind(item) {
  if (item.target_type === 'node') return '节点'
  if (item.target_type === 'node_group') return '策略组'
  return '订阅'
}

function targetName(item) {
  return item.target_name || String(item.target_id || '?')
}

function dialerKind(item) {
  return item.dialer_type === 'node_group' ? '策略组跳板' : '节点跳板'
}

function buildPayload() {
  const payload = {
    target_type: form.target_type,
    dialer_type: form.dialer_type,
    dialer_ref: form.dialer_ref,
    enabled: true,
    note: form.note || null,
  }
  if (form.target_type === 'node') {
    payload.target_name = form.target_name
  } else {
    payload.target_id = form.target_id
  }
  return payload
}

async function runPreview() {
  if (!canCreate.value || previewing.value) return
  previewing.value = true
  error.value = ''
  try {
    const { data } = await previewProxyChain(buildPayload())
    preview.value = data
    showPreviewModal.value = true
  } catch (err) {
    error.value = getApiErrorMessage(err, '预览失败')
    preview.value = null
    showPreviewModal.value = false
  } finally {
    previewing.value = false
  }
}

function closePreviewModal() {
  showPreviewModal.value = false
}

async function createBinding() {
  if (!canCreate.value || saving.value) return
  saving.value = true
  error.value = ''
  try {
    await createProxyChain(buildPayload())
    form.target_name = ''
    form.target_id = null
    form.dialer_ref = ''
    form.note = ''
    preview.value = null
    showPreviewModal.value = false
    showComposer.value = false
    await reload()
    store.toast('链式绑定已创建', 'success')
  } catch (err) {
    error.value = getApiErrorMessage(err, '创建失败')
  } finally {
    saving.value = false
  }
}

async function toggleEnabled(item) {
  if (workingId.value) return
  workingId.value = item.id
  try {
    await updateProxyChain(item.id, { enabled: !item.enabled })
    store.toast(item.enabled ? '已禁用绑定' : '已启用绑定', 'success')
    await reload()
  } catch (err) {
    error.value = getApiErrorMessage(err, '更新失败')
  } finally {
    workingId.value = null
  }
}

async function removeBinding(item) {
  const ok = await store.confirm({
    title: '删除链式绑定',
    message: `确定删除「${targetName(item)} → ${item.dialer_ref}」的绑定吗？`,
    confirmText: '删除',
    danger: true,
  })
  if (!ok || workingId.value) return
  workingId.value = item.id
  try {
    await deleteProxyChain(item.id)
    store.toast('链式绑定已删除', 'success')
    await reload()
  } catch (err) {
    error.value = getApiErrorMessage(err, '删除失败')
  } finally {
    workingId.value = null
  }
}
</script>
