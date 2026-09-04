<template>
  <section class="space-y-6">
    <!-- Top Header -->
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 pb-4 border-b border-white/10">
      <div>
        <p class="text-xs font-mono text-blue-400 uppercase tracking-wider">Proxy Policy Groups</p>
        <h2 class="text-xl font-bold text-white tracking-tight">策略组</h2>
        <p class="text-xs text-slate-400 mt-1">
          编辑与排序在列表完成。按住 ☰ 拖拽或用上移/下移调整顺序，解析预览点「预览」查看。
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2.5 w-full sm:w-auto">
        <button
          class="inline-flex items-center justify-center min-h-[44px] gap-1.5 rounded-lg border border-white/10 bg-slate-800/40 px-3.5 py-2 text-xs font-medium text-slate-300 hover:bg-slate-800 transition-colors cursor-pointer flex-1 sm:flex-initial"
          :disabled="loading || !!working"
          @click="validateRefs"
        >
          {{ working === 'validate' ? '校验中…' : '校验引用' }}
        </button>
        <button
          class="inline-flex items-center justify-center min-h-[44px] gap-1.5 rounded-lg border border-white/10 bg-slate-800/40 px-3.5 py-2 text-xs font-medium text-slate-300 hover:bg-slate-800 transition-colors cursor-pointer flex-1 sm:flex-initial"
          :disabled="loading || !!working"
          @click="loadPreview"
        >
          {{ working === 'preview' ? '刷新中…' : '刷新解析' }}
        </button>
        <button
          class="inline-flex items-center justify-center min-h-[44px] gap-1.5 rounded-lg border border-blue-500/40 bg-blue-600/20 px-4 py-2 text-xs font-medium text-blue-400 hover:bg-blue-600/30 transition-colors cursor-pointer flex-1 sm:flex-initial"
          @click="openCreate"
        >
          添加策略组
        </button>
      </div>
    </div>

    <!-- Alert / Error Message -->
    <UiState v-if="error" type="error" title="策略组操作失败" :description="error" compact>
      <template #actions>
        <button
          class="min-h-[44px] px-4 py-2 rounded-lg bg-blue-600 text-white text-xs font-medium cursor-pointer"
          @click="load"
        >
          重新加载
        </button>
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
          description="组引用 = 输出策略组名；组节点 = 展开叶子节点。排序请拖 ☰ 或上移/下移；点「预览」弹窗查节点。"
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
            class="min-h-[44px] rounded-lg border border-white/10 bg-slate-950/60 px-3 py-2 text-xs text-slate-300 focus:border-blue-500 focus:outline-hidden font-mono cursor-pointer"
          >
            <option value="">全部类型</option>
            <option v-for="t in typeOptions" :key="t" :value="t">{{ t }}</option>
          </select>
          <label class="min-h-[44px] inline-flex items-center gap-2 px-3 py-2 rounded-lg border border-white/10 bg-slate-950/60 text-xs text-slate-300 cursor-pointer select-none">
            <input
              v-model="onlyEmpty"
              type="checkbox"
              class="rounded border-white/20 bg-slate-900 text-blue-500 focus:ring-0"
            />
            <span>仅空组</span>
          </label>
        </template>
      </PageToolbar>

      <!-- Group Cards List -->
      <div class="space-y-3">
        <article
          v-for="group in filteredGroups"
          :key="group.id"
          class="flex flex-col justify-between p-4 sm:p-5 rounded-xl border transition-all backdrop-blur-md space-y-3.5"
          :class="[
            (previewById(group.id)?.resolved_count || 0) === 0
              ? 'border-amber-500/30 bg-amber-950/10'
              : 'border-white/10 bg-slate-900/60 hover:border-white/20',
            draggingGroupId === group.id ? 'opacity-50 ring-2 ring-blue-500/40' : ''
          ]"
          @dragover.prevent
          @drop="onGroupDrop(group)"
        >
          <!-- Head of Group Card -->
          <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-3 pb-3 border-b border-white/5">
            <div class="flex items-start gap-3 min-w-0 flex-1">
              <button
                type="button"
                class="flex min-h-[44px] min-w-[44px] items-center justify-center rounded-lg border border-white/10 bg-slate-800/60 text-slate-300 hover:text-white transition-colors cursor-grab active:cursor-grabbing select-none"
                :title="canReorderGroups ? '拖拽排序' : '清空筛选后再拖拽排序'"
                data-drag-handle
                :disabled="!canReorderGroups"
                :draggable="canReorderGroups"
                @dragstart="onGroupDragStart($event, group)"
                @dragend="draggingGroupId = null"
                @click.stop
                @mousedown.stop
              >
                ☰
              </button>
              <div class="min-w-0 space-y-1">
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="text-xs font-mono text-slate-500">#{{ originalIndex(group.id) + 1 }}</span>
                  <h3 class="text-base font-semibold text-white tracking-tight truncate" :title="group.name">
                    {{ group.name }}
                  </h3>
                </div>
                <div class="flex items-center gap-1.5 flex-wrap text-xs font-mono">
                  <span class="px-2 py-0.5 rounded text-[11px] bg-blue-500/10 text-blue-400 border border-blue-500/20 uppercase font-semibold">
                    {{ group.group_type }}
                  </span>
                  <span class="px-2 py-0.5 rounded text-[11px] bg-slate-800/60 text-slate-300 border border-white/5">
                    {{ group.kind || 'manual' }}
                  </span>
                  <span
                    class="px-2 py-0.5 rounded text-[11px] border font-medium"
                    :class="resolvedCount(group.id) ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20' : 'bg-amber-500/10 text-amber-400 border-amber-500/20'"
                  >
                    {{ resolvedCount(group.id) }} 节点
                  </span>
                  <span v-if="group.add_fallback" class="px-2 py-0.5 rounded text-[11px] bg-purple-500/10 text-purple-400 border border-purple-500/20">
                    空组 PASS
                  </span>
                </div>
              </div>
            </div>

            <!-- Group Card Actions -->
            <div class="grid grid-cols-2 sm:flex sm:flex-wrap items-center gap-2 w-full sm:w-auto">
              <button
                class="min-h-[44px] px-3 py-1.5 rounded-lg border border-white/10 bg-slate-800/40 text-xs font-medium text-slate-300 hover:bg-slate-800 transition-colors cursor-pointer flex-1 sm:flex-initial text-center justify-center"
                @click="openPreview(group)"
              >
                预览
              </button>
              <button
                class="min-h-[44px] px-3 py-1.5 rounded-lg border border-white/10 bg-slate-800/40 text-xs font-medium text-slate-300 hover:bg-slate-800 transition-colors cursor-pointer flex-1 sm:flex-initial text-center justify-center"
                :disabled="!canReorderGroups || originalIndex(group.id) === 0 || reordering"
                @click="moveById(group.id, -1)"
              >
                上移
              </button>
              <button
                class="min-h-[44px] px-3 py-1.5 rounded-lg border border-white/10 bg-slate-800/40 text-xs font-medium text-slate-300 hover:bg-slate-800 transition-colors cursor-pointer flex-1 sm:flex-initial text-center justify-center"
                :disabled="!canReorderGroups || originalIndex(group.id) === groups.length - 1 || reordering"
                @click="moveById(group.id, 1)"
              >
                下移
              </button>
              <button
                class="min-h-[44px] px-3.5 py-1.5 rounded-lg border border-blue-500/40 bg-blue-600/20 text-xs font-medium text-blue-400 hover:bg-blue-600/30 transition-colors cursor-pointer flex-1 sm:flex-initial text-center justify-center"
                @click="openEdit(group)"
              >
                编辑
              </button>
              <button
                class="min-h-[44px] px-3.5 py-1.5 rounded-lg border border-rose-500/40 bg-rose-600/20 text-xs font-medium text-rose-400 hover:bg-rose-600/30 transition-colors cursor-pointer flex-1 sm:flex-initial text-center justify-center col-span-2 sm:col-span-1"
                @click="remove(group)"
              >
                删除
              </button>
            </div>
          </div>

          <!-- Group Meta Stats Line -->
          <div class="flex flex-wrap gap-x-4 gap-y-1.5 text-xs font-mono text-slate-400">
            <span>静态节点: <strong class="text-slate-300 font-medium">{{ (group.include_nodes || []).length }}</strong></span>
            <span>组引用: <strong class="text-slate-300 font-medium">{{ (group.include_group_ids || []).length }}</strong></span>
            <span>展开组节点: <strong class="text-slate-300 font-medium">{{ (group.include_group_nodes_ids || []).length }}</strong></span>
            <span>条目总数: <strong class="text-slate-300 font-medium">{{ (previewById(group.id)?.include_entries || group.include_entries || []).length }}</strong></span>
          </div>

          <!-- Entry Tags -->
          <div
            v-if="(previewById(group.id)?.include_entries || group.include_entries || []).length"
            class="flex flex-wrap gap-1.5 text-xs font-mono pt-1"
          >
            <span
              v-for="(entry, eidx) in (previewById(group.id)?.include_entries || group.include_entries || []).slice(0, 8)"
              :key="`${entry.type}-${entry.value}-${eidx}`"
              class="px-2 py-0.5 rounded bg-slate-950/60 border border-white/5 text-slate-300"
              :title="formatEntryFull(entry)"
            >
              {{ formatEntry(entry) }}
            </span>
            <span
              v-if="(previewById(group.id)?.include_entries || group.include_entries || []).length > 8"
              class="px-2 py-0.5 rounded bg-blue-500/10 border border-blue-500/20 text-blue-400 font-medium"
            >
              +{{ (previewById(group.id)?.include_entries || group.include_entries || []).length - 8 }}
            </span>
          </div>
        </article>

        <!-- Empty States -->
        <UiState
          v-if="!filteredGroups.length"
          type="empty"
          :title="groups.length ? '没有匹配的策略组' : '暂无策略组'"
          :description="groups.length ? '换个关键词或筛选条件试试。' : '创建后点「预览」弹窗查解析节点。'"
        >
          <template #actions>
            <button
              v-if="!groups.length"
              class="min-h-[44px] px-4 py-2 rounded-lg bg-blue-600 text-white text-xs font-medium cursor-pointer"
              @click="openCreate"
            >
              添加策略组
            </button>
            <button
              v-else
              class="min-h-[44px] px-4 py-2 rounded-lg border border-white/10 bg-slate-800 text-slate-300 text-xs font-medium cursor-pointer"
              @click="search = ''; typeFilter = ''; onlyEmpty = false"
            >
              清空筛选
            </button>
          </template>
        </UiState>
      </div>
    </template>

    <!-- Node Group Edit/Create Modal -->
    <NodeGroupModal v-if="showModal" :group="editing" @saved="onSaved" @close="showModal = false" />

    <!-- Preview Modal (Adaptive Bottom Sheet on Mobile) -->
    <div
      v-if="previewGroup"
      class="fixed inset-0 z-50 flex items-end sm:items-center justify-center bg-black/70 backdrop-blur-sm p-0 sm:p-4"
      @click.self="closePreview"
    >
      <div
        class="relative flex w-full max-w-4xl flex-col rounded-t-2xl sm:rounded-2xl border border-white/10 bg-[#0F172A] text-[#F8FAFC] shadow-2xl overflow-hidden max-h-[90vh] sm:max-h-[85vh] pb-safe"
        role="dialog"
        aria-modal="true"
        :aria-label="previewTitle"
      >
        <!-- Mobile drag indicator -->
        <div class="sm:hidden mx-auto my-2.5 h-1 w-12 rounded-full bg-white/20" aria-hidden="true" />

        <div class="flex items-center justify-between border-b border-white/10 px-6 py-4">
          <div>
            <p class="text-[10px] font-mono tracking-wider text-blue-400 uppercase">Group Preview</p>
            <h3 class="text-base font-semibold text-white tracking-tight">
              {{ previewTitle }}
            </h3>
            <p class="text-xs text-slate-400 mt-0.5">
              {{ resolvedCount(previewGroup.id) }} 个解析节点
            </p>
          </div>
          <button
            class="flex min-h-[44px] min-w-[44px] items-center justify-center rounded-lg text-slate-400 hover:bg-slate-800 hover:text-white cursor-pointer transition-colors"
            @click="closePreview"
          >
            ✕
          </button>
        </div>

        <div class="p-4 sm:p-6 overflow-y-auto flex-1 space-y-3">
          <div v-if="previewById(previewGroup.id)?.include_group_names?.length" class="text-xs font-mono text-slate-400">
            引用组：{{ previewById(previewGroup.id).include_group_names.join('、') }}
          </div>
          <div v-if="previewById(previewGroup.id)?.include_group_nodes_names?.length" class="text-xs font-mono text-slate-400">
            展开组节点：{{ previewById(previewGroup.id).include_group_nodes_names.join('、') }}
          </div>
          <div v-if="previewById(previewGroup.id)?.exclude_group_names?.length" class="text-xs font-mono text-slate-400">
            减去：{{ previewById(previewGroup.id).exclude_group_names.join('、') }}
          </div>
          <div v-if="previewById(previewGroup.id)?.resolve_reasons?.length" class="text-xs font-mono text-slate-400">
            解析：{{ previewById(previewGroup.id).resolve_reasons.slice(0, 6).join('；') }}
            <span v-if="previewById(previewGroup.id).resolve_reasons.length > 6"> …</span>
          </div>

          <NodePreviewList
            :nodes="previewById(previewGroup.id)?.resolved_nodes || []"
            :collapsed-limit="40"
            placeholder="在本组内搜索节点"
          />
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
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
import MetricCard from '../components/ui/MetricCard.vue'
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
    store.success('解析预览已刷新')
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
  store.success(payload.created ? '策略组已创建' : '策略组已保存')
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
    store.success(`已删除 ${group.name}`)
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
    store.success('引用校验通过')
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
