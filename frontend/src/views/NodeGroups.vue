<template>
  <section class="page groups-page p-2 space-y-6 min-w-0 max-w-full overflow-x-hidden">
    <!-- Top Header -->
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 pb-4 border-b border-border-subtle">
      <div>
        <p class="text-xs font-mono text-accent uppercase tracking-wider">Proxy Policy Groups</p>
        <h2 class="text-xl font-bold text-text-main tracking-tight">策略组</h2>
        <p class="text-xs text-text-muted mt-1">
          编辑与排序在列表完成。拖拽手柄或用上移/下移调整顺序，解析预览点击「预览」查看。
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2.5 w-full sm:w-auto">
        <Button
          variant="secondary"
          size="md"
          class="flex-1 sm:flex-initial min-h-[44px] justify-center"
          :disabled="loading || !!working"
          :loading="working === 'validate'"
          :icon="CheckCircle2"
          @click="validateRefs"
        >
          {{ working === 'validate' ? '校验中…' : '校验引用' }}
        </Button>
        <Button
          variant="secondary"
          size="md"
          class="flex-1 sm:flex-initial min-h-[44px] justify-center"
          :disabled="loading || !!working"
          :loading="working === 'preview'"
          :icon="RefreshCw"
          @click="loadPreview"
        >
          {{ working === 'preview' ? '刷新中…' : '刷新解析' }}
        </Button>
        <Button
          variant="primary"
          size="md"
          class="flex-1 sm:flex-initial min-h-[44px] justify-center"
          :icon="Plus"
          @click="openCreate"
        >
          添加策略组
        </Button>
      </div>
    </div>

    <!-- Alert / Error Message -->
    <UiState v-if="error" type="error" title="策略组操作失败" :description="error" compact>
      <template #actions>
        <Button variant="primary" size="sm" class="min-h-[44px]" @click="load">
          重新加载
        </Button>
      </template>
    </UiState>

    <UiState
      v-if="loading && !groups.length"
      type="loading"
      title="正在加载策略组"
      description="同步组配置与解析预览。"
    />

    <template v-else>
      <!-- Industrial MetricCards Grid (Responsive: 1 col mobile, 2 cols sm, 3 cols md+) -->
      <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-3 sm:gap-4">
        <MetricCard
          label="POLICY GROUPS"
          :value="groups.length"
          subtext="已配置策略组数量"
          status="info"
        />
        <MetricCard
          label="RESOLVED NODES"
          :value="totalResolved"
          subtext="全组解析叶子节点总数"
          status="success"
        />
        <MetricCard
          label="ROUTING MINDSET"
          value="引用与展开"
          description="组引用 = 输出策略组名；组节点 = 展开叶子节点。排序请拖拽排序手柄或上移/下移；点「预览」抽屉查节点。"
          status="neutral"
        />
      </div>

      <!-- Action Toolbar with Search & Filter -->
      <PageToolbar
        v-model="search"
        placeholder="搜索组名 / 类型 / 条目 / 节点…"
        :count-text="`${filteredGroups.length} / ${groups.length} 组`"
      >
        <template #filters>
          <select
            v-model="typeFilter"
            aria-label="策略组类型筛选"
            class="min-h-[44px] rounded-lg border border-border-subtle bg-surface-base px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono cursor-pointer transition-colors"
          >
            <option value="">全部类型</option>
            <option v-for="t in typeOptions" :key="t" :value="t">{{ t }}</option>
          </select>
          <label class="min-h-[44px] inline-flex items-center gap-2 px-3 py-2 rounded-lg border border-border-subtle bg-surface-base text-xs text-text-main cursor-pointer select-none">
            <input
              v-model="onlyEmpty"
              type="checkbox"
              class="rounded border-border-subtle bg-surface text-accent focus:ring-0"
            />
            <span>仅空组</span>
          </label>
        </template>
      </PageToolbar>

      <!-- Empty States -->
      <div v-if="!filteredGroups.length">
        <UiState
          type="empty"
          :title="groups.length ? '没有匹配的策略组' : '暂无策略组'"
          :description="groups.length ? '换个关键词或筛选条件试试。' : '创建后点「预览」抽屉查解析节点。'"
        >
          <template #actions>
            <Button
              v-if="!groups.length"
              variant="primary"
              size="md"
              class="min-h-[44px]"
              :icon="Plus"
              @click="openCreate"
            >
              添加策略组
            </Button>
            <Button
              v-else
              variant="secondary"
              size="md"
              class="min-h-[44px]"
              @click="clearFilters"
            >
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
                <th scope="col" class="py-2.5 px-3 font-semibold w-16 text-center">排序</th>
                <th scope="col" class="py-2.5 px-4 font-semibold min-w-[200px]">策略组名称与类型</th>
                <th scope="col" class="py-2.5 px-4 font-semibold min-w-[260px]">包含条目与规则</th>
                <th scope="col" class="py-2.5 px-4 font-semibold w-36">条目统计</th>
                <th scope="col" class="py-2.5 px-4 font-semibold w-28 text-center">解析状态</th>
                <th scope="col" class="py-2.5 px-4 font-semibold text-right w-64">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border-subtle">
              <tr
                v-for="group in filteredGroups"
                :key="group.id"
                class="hover:bg-surface-hover/60 transition-colors h-[48px]"
                :class="[
                  (previewById(group.id)?.resolved_count || 0) === 0 ? 'bg-status-warning/5' : '',
                  draggingGroupId === group.id ? 'opacity-50 ring-2 ring-accent/40' : ''
                ]"
                @dragover.prevent
                @drop="onGroupDrop(group)"
              >
                <!-- Drag handle & Index -->
                <td class="py-2 px-3 text-center whitespace-nowrap">
                  <div class="flex items-center justify-center gap-1">
                    <button
                      type="button"
                      class="flex min-h-[32px] min-w-[32px] items-center justify-center rounded-md border border-border-subtle bg-surface text-text-muted hover:text-text-main transition-colors select-none"
                      :class="canReorderGroups ? 'cursor-grab active:cursor-grabbing' : 'opacity-40 cursor-not-allowed'"
                      :title="canReorderGroups ? '拖拽排序' : '清空筛选后再拖拽排序'"
                      data-drag-handle
                      :disabled="!canReorderGroups"
                      :draggable="canReorderGroups"
                      @dragstart="onGroupDragStart($event, group)"
                      @dragend="draggingGroupId = null"
                      @click.stop
                      @mousedown.stop
                    >
                      <GripVertical class="h-3.5 w-3.5" aria-hidden="true" />
                    </button>
                    <span class="text-[11px] text-text-sub tabular-nums">#{{ originalIndex(group.id) + 1 }}</span>
                  </div>
                </td>

                <!-- Group Name & Type Badge -->
                <td class="py-2 px-4">
                  <div class="space-y-1 min-w-0">
                    <div class="flex items-center gap-2">
                      <strong class="text-text-main font-semibold tracking-tight truncate max-w-[220px]" :title="group.name">
                        {{ group.name }}
                      </strong>
                    </div>
                    <div class="flex items-center gap-1.5 flex-wrap text-[10px]">
                      <span class="px-1.5 py-0.5 rounded bg-accent/10 text-accent border border-accent/20 uppercase font-semibold">
                        {{ group.group_type }}
                      </span>
                      <span class="px-1.5 py-0.5 rounded bg-surface-active text-text-sub border border-border-subtle">
                        {{ group.kind || 'manual' }}
                      </span>
                      <span v-if="group.add_fallback" class="px-1.5 py-0.5 rounded bg-status-info/10 text-status-info border border-status-info/20">
                        空组 PASS
                      </span>
                    </div>
                  </div>
                </td>

                <!-- Included Entries Tags -->
                <td class="py-2 px-4">
                  <div
                    v-if="(previewById(group.id)?.include_entries || group.include_entries || []).length"
                    class="flex flex-wrap gap-1 items-center max-w-[340px]"
                  >
                    <span
                      v-for="(entry, eidx) in (previewById(group.id)?.include_entries || group.include_entries || []).slice(0, 5)"
                      :key="`${entry.type}-${entry.value}-${eidx}`"
                      class="px-1.5 py-0.5 rounded bg-surface-active text-text-muted border border-border-subtle text-[10px] truncate max-w-[140px]"
                      :title="formatEntryFull(entry)"
                    >
                      {{ formatEntry(entry) }}
                    </span>
                    <span
                      v-if="(previewById(group.id)?.include_entries || group.include_entries || []).length > 5"
                      class="px-1.5 py-0.5 rounded bg-accent/10 border border-accent/20 text-accent text-[10px] font-medium"
                    >
                      +{{ (previewById(group.id)?.include_entries || group.include_entries || []).length - 5 }}
                    </span>
                  </div>
                  <div v-else class="text-[11px] text-text-sub italic">
                    无条目
                  </div>
                </td>

                <!-- Entry Counts -->
                <td class="py-2 px-4 text-[11px] text-text-muted whitespace-nowrap">
                  <div>静态: <strong class="text-text-main tabular-nums">{{ (group.include_nodes || []).length }}</strong></div>
                  <div>组引用: <span class="text-text-main tabular-nums">{{ (group.include_group_ids || []).length }}</span></div>
                  <div>展开: <span class="text-text-main tabular-nums">{{ (group.include_group_nodes_ids || []).length }}</span></div>
                </td>

                <!-- Resolved Status Badge -->
                <td class="py-2 px-4 text-center whitespace-nowrap">
                  <StatusBadge
                    v-if="resolvedCount(group.id) > 0"
                    type="success"
                    :text="`${resolvedCount(group.id)} 节点`"
                  />
                  <StatusBadge
                    v-else
                    type="warning"
                    text="0 节点"
                  />
                </td>

                <!-- Action Buttons -->
                <td class="py-2 px-4 text-right whitespace-nowrap">
                  <div class="inline-flex items-center gap-1.5">
                    <Button
                      variant="secondary"
                      size="sm"
                      :icon="Eye"
                      title="预览解析节点"
                      @click="openPreview(group)"
                    >
                      预览
                    </Button>
                    <Button
                      variant="secondary"
                      size="sm"
                      :icon="ArrowUp"
                      :disabled="!canReorderGroups || originalIndex(group.id) === 0 || reordering"
                      title="上移"
                      @click="moveById(group.id, -1)"
                    >
                      上移
                    </Button>
                    <Button
                      variant="secondary"
                      size="sm"
                      :icon="ArrowDown"
                      :disabled="!canReorderGroups || originalIndex(group.id) === groups.length - 1 || reordering"
                      title="下移"
                      @click="moveById(group.id, 1)"
                    >
                      下移
                    </Button>
                    <Button
                      variant="secondary"
                      size="sm"
                      :icon="Edit2"
                      title="编辑策略组"
                      @click="openEdit(group)"
                    >
                      编辑
                    </Button>
                    <Button
                      variant="danger"
                      size="sm"
                      :icon="Trash2"
                      title="删除策略组"
                      @click="remove(group)"
                    >
                      删除
                    </Button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Mobile & Tablet Responsive List Cards -->
        <div class="lg:hidden space-y-3">
          <article
            v-for="group in filteredGroups"
            :key="group.id"
            class="rounded-lg border border-border-subtle bg-surface-base p-4 space-y-3 shadow-xs"
            :class="[
              (previewById(group.id)?.resolved_count || 0) === 0 ? 'border-status-warning/30 bg-status-warning/5' : '',
              draggingGroupId === group.id ? 'opacity-50 ring-2 ring-accent/40' : ''
            ]"
            @dragover.prevent
            @drop="onGroupDrop(group)"
          >
            <!-- Card Head -->
            <div class="flex items-start justify-between gap-2 pb-2 border-b border-border-subtle">
              <div class="flex items-start gap-2.5 min-w-0 flex-1">
                <button
                  type="button"
                  class="flex min-h-[44px] min-w-[44px] items-center justify-center rounded-lg border border-border-subtle bg-surface text-text-muted hover:text-text-main transition-colors select-none shrink-0"
                  :class="canReorderGroups ? 'cursor-grab active:cursor-grabbing' : 'opacity-40 cursor-not-allowed'"
                  :title="canReorderGroups ? '拖拽排序' : '清空筛选后再拖拽排序'"
                  data-drag-handle
                  :disabled="!canReorderGroups"
                  :draggable="canReorderGroups"
                  @dragstart="onGroupDragStart($event, group)"
                  @dragend="draggingGroupId = null"
                  @click.stop
                  @mousedown.stop
                >
                  <GripVertical class="h-4 w-4" aria-hidden="true" />
                </button>
                <div class="min-w-0 space-y-1">
                  <div class="flex items-center gap-1.5 flex-wrap">
                    <span class="text-xs font-mono text-text-sub tabular-nums">#{{ originalIndex(group.id) + 1 }}</span>
                    <h3 class="text-sm font-semibold text-text-main truncate max-w-[200px]" :title="group.name">
                      {{ group.name }}
                    </h3>
                  </div>
                  <div class="flex items-center gap-1.5 flex-wrap text-[10px] font-mono">
                    <span class="px-1.5 py-0.5 rounded bg-accent/10 text-accent border border-accent/20 uppercase font-semibold">
                      {{ group.group_type }}
                    </span>
                    <span class="px-1.5 py-0.5 rounded bg-surface-active text-text-sub border border-border-subtle">
                      {{ group.kind || 'manual' }}
                    </span>
                    <span
                      class="px-1.5 py-0.5 rounded border font-medium"
                      :class="resolvedCount(group.id) ? 'bg-status-success/10 text-status-success border-status-success/20' : 'bg-status-warning/10 text-status-warning border-status-warning/20'"
                    >
                      {{ resolvedCount(group.id) }} 节点
                    </span>
                  </div>
                </div>
              </div>
            </div>

            <!-- Group Meta Details -->
            <div class="grid grid-cols-3 gap-2 text-xs font-mono text-text-muted py-1 border-t border-b border-border-subtle">
              <div>静态: <strong class="text-text-main tabular-nums">{{ (group.include_nodes || []).length }}</strong></div>
              <div>组引用: <span class="text-text-main tabular-nums">{{ (group.include_group_ids || []).length }}</span></div>
              <div>展开组: <span class="text-text-main tabular-nums">{{ (group.include_group_nodes_ids || []).length }}</span></div>
            </div>

            <!-- Entries line -->
            <div
              v-if="(previewById(group.id)?.include_entries || group.include_entries || []).length"
              class="flex flex-wrap gap-1 text-[10px] font-mono"
            >
              <span
                v-for="(entry, eidx) in (previewById(group.id)?.include_entries || group.include_entries || []).slice(0, 6)"
                :key="`${entry.type}-${entry.value}-${eidx}`"
                class="px-1.5 py-0.5 rounded bg-surface-active border border-border-subtle text-text-muted truncate max-w-[130px]"
                :title="formatEntryFull(entry)"
              >
                {{ formatEntry(entry) }}
              </span>
              <span
                v-if="(previewById(group.id)?.include_entries || group.include_entries || []).length > 6"
                class="px-1.5 py-0.5 rounded bg-accent/10 border border-accent/20 text-accent font-medium"
              >
                +{{ (previewById(group.id)?.include_entries || group.include_entries || []).length - 6 }}
              </span>
            </div>

            <!-- Action buttons (min-h-[44px] touch targets) -->
            <div class="grid grid-cols-2 sm:grid-cols-3 gap-2 pt-1">
              <Button
                variant="secondary"
                size="md"
                class="min-h-[44px] justify-center"
                :icon="Eye"
                @click="openPreview(group)"
              >
                预览
              </Button>
              <Button
                variant="secondary"
                size="md"
                class="min-h-[44px] justify-center"
                :icon="ArrowUp"
                :disabled="!canReorderGroups || originalIndex(group.id) === 0 || reordering"
                @click="moveById(group.id, -1)"
              >
                上移
              </Button>
              <Button
                variant="secondary"
                size="md"
                class="min-h-[44px] justify-center"
                :icon="ArrowDown"
                :disabled="!canReorderGroups || originalIndex(group.id) === groups.length - 1 || reordering"
                @click="moveById(group.id, 1)"
              >
                下移
              </Button>
              <Button
                variant="secondary"
                size="md"
                class="min-h-[44px] justify-center"
                :icon="Edit2"
                @click="openEdit(group)"
              >
                编辑
              </Button>
              <Button
                variant="danger"
                size="md"
                class="min-h-[44px] justify-center col-span-2 sm:col-span-1"
                :icon="Trash2"
                @click="remove(group)"
              >
                删除
              </Button>
            </div>
          </article>
        </div>
      </div>
    </template>

    <!-- Node Group Edit/Create Modal -->
    <NodeGroupModal v-if="showModal" :group="editing" @saved="onSaved" @close="showModal = false" />

    <!-- Preview BaseDrawer (Adaptive Bottom Sheet on Mobile with pb-safe) -->
    <BaseDrawer
      :model-value="previewGroup !== null"
      :title="previewTitle"
      @close="closePreview"
    >
      <div v-if="previewGroup" class="space-y-4 text-xs font-mono">
        <p class="text-xs text-text-muted">
          {{ resolvedCount(previewGroup.id) }} 个解析节点
        </p>
        <div v-if="previewById(previewGroup.id)?.include_group_names?.length" class="text-text-muted">
          引用组：{{ previewById(previewGroup.id).include_group_names.join('、') }}
        </div>
        <div v-if="previewById(previewGroup.id)?.include_group_nodes_names?.length" class="text-text-muted">
          展开组节点：{{ previewById(previewGroup.id).include_group_nodes_names.join('、') }}
        </div>
        <div v-if="previewById(previewGroup.id)?.exclude_group_names?.length" class="text-text-muted">
          减去：{{ previewById(previewGroup.id).exclude_group_names.join('、') }}
        </div>
        <div v-if="previewById(previewGroup.id)?.resolve_reasons?.length" class="text-text-muted">
          解析：{{ previewById(previewGroup.id).resolve_reasons.slice(0, 6).join('；') }}
          <span v-if="previewById(previewGroup.id).resolve_reasons.length > 6"> …</span>
        </div>

        <NodePreviewList
          :nodes="previewById(previewGroup.id)?.resolved_nodes || []"
          :collapsed-limit="40"
          placeholder="在本组内搜索节点"
        />
      </div>
    </BaseDrawer>
  </section>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  Plus,
  RefreshCw,
  Edit2,
  Trash2,
  Eye,
  ArrowUp,
  ArrowDown,
  CheckCircle2,
  GripVertical
} from 'lucide-vue-next'
import { useAppStore } from '../stores/app'
import { useUrlState } from '../utils/urlState'
import {
  deleteNodeGroup,
  getApiErrorMessage,
  getNodeGroups,
  previewNodeGroups,
  reorderNodeGroups,
  validateNodeGroups,
} from '../api'
import NodePreviewList from '../components/NodePreviewList.vue'
import PageToolbar from '../components/PageToolbar.vue'
import UiState from '../components/UiState.vue'
import NodeGroupModal from '../components/NodeGroupModal.vue'
import { Button, StatusBadge, MetricCard, BaseDrawer } from '../components/ui'
import { setDragGhost, shouldAllowDragStart } from '../utils/drag'

const store = useAppStore()

const groups = ref([])
const previews = ref([])
const showModal = ref(false)
const editing = ref(null)
const loading = ref(false)
const working = ref('')
const error = ref('')
const search = useUrlState('q', '')
const typeFilter = useUrlState('type', '')
const onlyEmpty = ref(false)
const previewGroup = ref(null)
const draggingGroupId = ref(null)
const reordering = ref(false)

const previewTitle = computed(() => {
  if (!previewGroup.value) return '组预览'
  return `${previewGroup.value.name} · 解析预览`
})

onMounted(load)

function onKeydown(e) {
  if (e.key !== 'Escape') return
  if (previewGroup.value) {
    closePreview()
    return
  }
  if (showModal.value) showModal.value = false
}
window.addEventListener('keydown', onKeydown)
onUnmounted(() => window.removeEventListener('keydown', onKeydown))

const previewMap = computed(() => new Map(previews.value.map((item) => [item.id, item])))
const indexMap = computed(() => new Map(groups.value.map((g, i) => [g.id, i])))

function previewById(id) {
  return previewMap.value.get(id)
}

function originalIndex(id) {
  return indexMap.value.get(id) ?? -1
}

function resolvedCount(id) {
  return previewById(id)?.resolved_count || 0
}

const typeOptions = computed(() => {
  const set = new Set(groups.value.map((g) => g.group_type).filter(Boolean))
  return [...set].sort()
})

const totalResolved = computed(() =>
  previews.value.reduce((sum, p) => sum + (p.resolved_count || 0), 0),
)

const groupSearchIndex = computed(() => {
  const map = new Map()
  for (const group of groups.value) {
    const preview = previewMap.value.get(group.id)
    const entryText = (preview?.include_entries || group.include_entries || [])
      .map((e) => `${e.type}:${e.value}:${e.name || ''}`)
      .join(' ')
    const nodes = (preview?.resolved_nodes || [])
      .map((n) => (typeof n === 'string' ? n : n?.name || ''))
      .join(' ')
    map.set(group.id, [group.name, group.group_type, group.kind, entryText, nodes].join(' ').toLowerCase())
  }
  return map
})

const filteredGroups = computed(() => {
  const q = String(search.value || '').trim().toLowerCase()
  return groups.value.filter((group) => {
    if (typeFilter.value && group.group_type !== typeFilter.value) return false
    const preview = previewById(group.id)
    if (onlyEmpty.value && (preview?.resolved_count || 0) > 0) return false
    if (!q) return true
    return (groupSearchIndex.value.get(group.id) || '').includes(q)
  })
})

const canReorderGroups = computed(
  () =>
    !String(search.value || '').trim()
    && !typeFilter.value
    && !onlyEmpty.value
    && filteredGroups.value.length === groups.value.length,
)

function clearFilters() {
  search.value = ''
  typeFilter.value = ''
  onlyEmpty.value = false
}

function openPreview(group) {
  previewGroup.value = group
}

function closePreview() {
  previewGroup.value = null
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [groupsRes, previewsRes] = await Promise.all([getNodeGroups(), previewNodeGroups()])
    groups.value = groupsRes.data || []
    previews.value = previewsRes.data || []
  } catch (err) {
    error.value = getApiErrorMessage(err, '加载策略组失败')
  } finally {
    loading.value = false
  }
}

async function loadPreview() {
  working.value = 'preview'
  error.value = ''
  try {
    const { data } = await previewNodeGroups()
    previews.value = data || []
    store.toast('解析预览已刷新', 'success')
  } catch (err) {
    error.value = getApiErrorMessage(err, '刷新预览失败')
  } finally {
    working.value = ''
  }
}

function openCreate() {
  editing.value = null
  showModal.value = true
}

function openEdit(group) {
  editing.value = { ...group }
  showModal.value = true
}

function onSaved(payload = {}) {
  showModal.value = false
  store.toast(payload.created ? '策略组已创建' : '策略组已保存', 'success')
  load()
}

async function remove(group) {
  const preview = previewById(group.id)
  const refs = []
  if (preview?.include_group_names?.length) refs.push(`关联引用: ${preview.include_group_names.join('、')}`)
  if (preview?.resolved_count) refs.push(`当前解析 ${preview.resolved_count} 个节点`)
  const ok = await store.confirm({
    title: '删除策略组',
    message: `确定删除「${group.name}」？${refs.length ? '\n\n' + refs.join('\n') : '\n\n若仍被其他组或规则引用，后端会拒绝。'}`,
    confirmText: '删除',
    danger: true,
  })
  if (!ok) return
  error.value = ''
  try {
    await deleteNodeGroup(group.id)
    store.toast(`已删除 ${group.name}`, 'success')
    await load()
  } catch (err) {
    error.value = getApiErrorMessage(err, '删除失败')
  }
}

function onGroupDragStart(event, group) {
  if (!canReorderGroups.value || !shouldAllowDragStart(event, { requireHandle: true })) {
    event.preventDefault()
    draggingGroupId.value = null
    return
  }
  draggingGroupId.value = group.id
  setDragGhost(event, group.name || '策略组排序')
}

async function onGroupDrop(targetGroup) {
  const fromId = draggingGroupId.value
  draggingGroupId.value = null
  if (!fromId || fromId === targetGroup.id) return
  const from = originalIndex(fromId)
  const to = originalIndex(targetGroup.id)
  if (from < 0 || to < 0 || from === to) return
  await applyGroupOrder(from, to)
}

async function moveById(id, delta) {
  const index = originalIndex(id)
  if (index < 0) return
  const to = index + delta
  if (to < 0 || to >= groups.value.length) return
  await applyGroupOrder(index, to)
}

async function applyGroupOrder(from, to) {
  if (reordering.value) return
  const copy = [...groups.value]
  if (from < 0 || to < 0 || from >= copy.length || to >= copy.length || from === to) return
  error.value = ''
  reordering.value = true
  try {
    const [moved] = copy.splice(from, 1)
    copy.splice(to, 0, moved)
    groups.value = copy
    const items = copy.map((group, i) => ({ id: group.id, sort_order: i }))
    await reorderNodeGroups(items)
    await load()
  } catch (err) {
    error.value = getApiErrorMessage(err, '排序失败')
    await load()
  } finally {
    reordering.value = false
  }
}

async function validateRefs() {
  working.value = 'validate'
  error.value = ''
  try {
    await validateNodeGroups()
    store.toast('引用校验通过', 'success')
  } catch (err) {
    error.value = getApiErrorMessage(err, '校验失败')
  } finally {
    working.value = ''
  }
}

function truncateText(text, max = 28) {
  const value = String(text || '')
  if (value.length <= max) return value
  return `${value.slice(0, Math.max(1, max - 1))}…`
}

function regexLabel(entry) {
  const label = String(entry?.name || entry?.label || '').trim()
  if (label) return label
  return '正则'
}

function formatEntryFull(entry) {
  if (entry.type === 'node') return `节点:${entry.value}`
  if (entry.type === 'group') {
    const g = groups.value.find((item) => item.id === entry.value)
    return `组:${g ? g.name : `#${entry.value}`}`
  }
  if (entry.type === 'group_nodes') {
    const g = groups.value.find((item) => item.id === entry.value)
    return `组节点:${g ? g.name : `#${entry.value}`}`
  }
  if (entry.type === 'exclude_group_nodes') {
    const g = groups.value.find((item) => item.id === entry.value)
    return `减去:${g ? g.name : `#${entry.value}`}`
  }
  if (entry.type === 'regex') return `${regexLabel(entry)} · /${entry.value}/`
  return JSON.stringify(entry)
}

function formatEntry(entry) {
  if (entry.type === 'node') return truncateText(`节点:${entry.value}`, 24)
  if (entry.type === 'group') {
    const g = groups.value.find((item) => item.id === entry.value)
    return truncateText(`组:${g ? g.name : `#${entry.value}`}`, 24)
  }
  if (entry.type === 'group_nodes') {
    const g = groups.value.find((item) => item.id === entry.value)
    return truncateText(`组节点:${g ? g.name : `#${entry.value}`}`, 24)
  }
  if (entry.type === 'exclude_group_nodes') {
    const g = groups.value.find((item) => item.id === entry.value)
    return truncateText(`减:${g ? g.name : `#${entry.value}`}`, 24)
  }
  if (entry.type === 'regex') return regexLabel(entry)
  return truncateText(JSON.stringify(entry), 24)
}
</script>
