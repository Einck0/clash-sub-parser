<template>
  <section class="page node-groups-page">
    <div class="page-head">
      <div>
        <p class="eyebrow">Proxy Groups</p>
        <h2>策略组</h2>
        <p class="page-desc">预览贴在每组上；编辑、排序、看解析结果都在同一张卡里完成。</p>
      </div>
      <div class="head-actions">
        <button @click="validateRefs" :disabled="loading || !!working">
          {{ working === 'validate' ? '校验中…' : '校验引用' }}
        </button>
        <button @click="loadPreview" :disabled="loading || !!working">
          {{ working === 'preview' ? '刷新中…' : '刷新解析' }}
        </button>
        <button class="primary" @click="openCreate">添加策略组</button>
      </div>
    </div>

    <UiState v-if="error" type="error" title="策略组操作失败" :description="error" compact>
      <template #actions>
        <button @click="load">重新加载</button>
      </template>
    </UiState>
    <UiState
      v-if="loading && !groups.length"
      type="loading"
      title="正在加载策略组"
      description="同步组配置与解析预览。"
    />

    <template v-else>
      <PageToolbar
        v-model="search"
        placeholder="搜索组名 / 类型 / 条目 / 节点…"
        :count-text="`${filteredGroups.length} / ${groups.length} 组`"
      >
        <template #filters>
          <select v-model="typeFilter" class="toolbar-select">
            <option value="">全部类型</option>
            <option v-for="t in typeOptions" :key="t" :value="t">{{ t }}</option>
          </select>
          <label class="toolbar-check">
            <input v-model="onlyEmpty" type="checkbox" />
            <span>仅空组</span>
          </label>
          <label class="toolbar-check">
            <input v-model="expandAllPreview" type="checkbox" />
            <span>默认展开预览</span>
          </label>
        </template>
      </PageToolbar>

      <div class="summary-grid node-summary">
        <div class="metric-card">
          <span class="metric-label">策略组</span>
          <strong>{{ groups.length }}</strong>
        </div>
        <div class="metric-card">
          <span class="metric-label">总解析节点</span>
          <strong>{{ totalResolved }}</strong>
        </div>
        <div class="metric-card wide">
          <span class="metric-label">心智</span>
          <span class="metric-tip">
            组引用 = 输出策略组名；组节点 = 展开叶子节点。预览就在本组下方，不用滚到页面底部。
          </span>
        </div>
      </div>

      <div class="group-list">
        <article
          v-for="(group, idx) in filteredGroups"
          :key="group.id"
          class="group-card"
          :class="{ 'is-empty': (previewById(group.id)?.resolved_count || 0) === 0 }"
        >
          <div class="group-card-head">
            <div class="group-title-block">
              <span class="category-index">#{{ originalIndex(group.id) + 1 }}</span>
              <div>
                <h3>{{ group.name }}</h3>
                <div class="badge-row">
                  <span class="count-pill">{{ group.group_type }}</span>
                  <span class="badge">{{ group.kind || 'manual' }}</span>
                  <span class="badge" :class="resolvedCount(group.id) ? 'badge-ok' : 'badge-warn'">
                    {{ resolvedCount(group.id) }} 节点
                  </span>
                  <span v-if="group.add_fallback" class="badge">空组 PASS</span>
                </div>
              </div>
            </div>
            <div class="group-head-actions">
              <button @click="moveById(group.id, -1)" :disabled="originalIndex(group.id) === 0">上移</button>
              <button
                @click="moveById(group.id, 1)"
                :disabled="originalIndex(group.id) === groups.length - 1"
              >
                下移
              </button>
              <button class="primary" @click="openEdit(group)">编辑</button>
              <button class="danger" @click="remove(group)">删除</button>
            </div>
          </div>

          <div class="group-meta-line">
            <span>静态 {{ (group.include_nodes || []).length }}</span>
            <span>组引用 {{ (group.include_group_ids || []).length }}</span>
            <span>组节点 {{ (group.include_group_nodes_ids || []).length }}</span>
            <span>条目 {{ (previewById(group.id)?.include_entries || group.include_entries || []).length }}</span>
          </div>

          <div
            class="entry-tags"
            v-if="(previewById(group.id)?.include_entries || group.include_entries || []).length"
          >
            <span
              v-for="(entry, eidx) in (previewById(group.id)?.include_entries || group.include_entries || []).slice(0, 8)"
              :key="`${entry.type}-${entry.value}-${eidx}`"
              class="badge"
              :title="formatEntryFull(entry)"
            >
              {{ formatEntry(entry) }}
            </span>
            <span
              v-if="(previewById(group.id)?.include_entries || group.include_entries || []).length > 8"
              class="badge"
            >
              +{{ (previewById(group.id)?.include_entries || group.include_entries || []).length - 8 }}
            </span>
          </div>

          <div class="inline-preview">
            <button class="preview-toggle" type="button" @click="togglePreview(group.id)">
              <span>{{ isPreviewOpen(group.id) ? '收起预览' : '展开预览' }}</span>
              <span class="muted">{{ resolvedCount(group.id) }} 个解析节点</span>
            </button>

            <div v-if="isPreviewOpen(group.id)" class="preview-body">
              <div class="muted small-line" v-if="previewById(group.id)?.include_group_names?.length">
                引用组：{{ previewById(group.id).include_group_names.join('、') }}
              </div>
              <div class="muted small-line" v-if="previewById(group.id)?.include_group_nodes_names?.length">
                展开组节点：{{
                  previewById(group.id).include_group_nodes_names.map((n) => `${n}`).join('、')
                }}
              </div>
              <div class="muted small-line" v-if="previewById(group.id)?.exclude_group_names?.length">
                减去：{{ previewById(group.id).exclude_group_names.join('、') }}
              </div>
              <div class="muted small-line" v-if="previewById(group.id)?.resolve_reasons?.length">
                解析：{{ previewById(group.id).resolve_reasons.slice(0, 4).join('；') }}
                <span v-if="previewById(group.id).resolve_reasons.length > 4"> …</span>
              </div>
              <NodePreviewList
                :nodes="previewById(group.id)?.resolved_nodes || []"
                :collapsed-limit="12"
                placeholder="在本组内搜索节点"
              />
            </div>
          </div>
        </article>

        <UiState
          v-if="!filteredGroups.length"
          type="empty"
          :title="groups.length ? '没有匹配的策略组' : '暂无策略组'"
          :description="groups.length ? '换个关键词或筛选条件试试。' : '创建后可直接在卡片上展开解析预览。'"
        >
          <template #actions>
            <button v-if="!groups.length" class="primary" @click="openCreate">添加策略组</button>
            <button v-else @click="search = ''; typeFilter = ''; onlyEmpty = false">清空筛选</button>
          </template>
        </UiState>
      </div>
    </template>

    <NodeGroupModal v-if="showModal" :group="editing" @saved="onSaved" @close="showModal = false" />
  </section>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useAppStore } from '../stores/app'
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
import NodeGroupModal from './NodeGroupModal.vue'

const store = useAppStore()

const groups = ref([])
const previews = ref([])
const showModal = ref(false)
const editing = ref(null)
const loading = ref(false)
const working = ref('')
const error = ref('')
const search = ref('')
const typeFilter = ref('')
const onlyEmpty = ref(false)
const expandAllPreview = ref(false)
const openPreviewIds = ref(new Set())

onMounted(load)

function onKeydown(e) {
  if (e.key === 'Escape' && showModal.value) showModal.value = false
}
window.addEventListener('keydown', onKeydown)
onUnmounted(() => window.removeEventListener('keydown', onKeydown))

const previewMap = computed(() => new Map(previews.value.map((item) => [item.id, item])))
function previewById(id) {
  return previewMap.value.get(id)
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

const filteredGroups = computed(() => {
  const q = String(search.value || '').trim().toLowerCase()
  return groups.value.filter((group) => {
    if (typeFilter.value && group.group_type !== typeFilter.value) return false
    const preview = previewById(group.id)
    if (onlyEmpty.value && (preview?.resolved_count || 0) > 0) return false
    if (!q) return true
    const entryText = (preview?.include_entries || group.include_entries || [])
      .map((e) => `${e.type}:${e.value}:${e.name || ''}`)
      .join(' ')
    const nodes = (preview?.resolved_nodes || [])
      .map((n) => (typeof n === 'string' ? n : n?.name || ''))
      .join(' ')
    const hay = [group.name, group.group_type, group.kind, entryText, nodes]
      .join(' ')
      .toLowerCase()
    return hay.includes(q)
  })
})

watch(expandAllPreview, (on) => {
  if (on) {
    openPreviewIds.value = new Set(groups.value.map((g) => g.id))
  } else {
    openPreviewIds.value = new Set()
  }
})

function isPreviewOpen(id) {
  return openPreviewIds.value.has(id)
}

function togglePreview(id) {
  const next = new Set(openPreviewIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  openPreviewIds.value = next
}

function originalIndex(id) {
  return groups.value.findIndex((g) => g.id === id)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [groupsRes, previewsRes] = await Promise.all([getNodeGroups(), previewNodeGroups()])
    groups.value = groupsRes.data || []
    previews.value = previewsRes.data || []
    if (expandAllPreview.value) {
      openPreviewIds.value = new Set(groups.value.map((g) => g.id))
    }
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

async function moveById(id, delta) {
  const index = originalIndex(id)
  if (index < 0) return
  const copy = [...groups.value]
  const to = index + delta
  if (to < 0 || to >= copy.length) return
  error.value = ''
  try {
    ;[copy[index], copy[to]] = [copy[to], copy[index]]
    const items = copy.map((group, i) => ({ id: group.id, sort_order: i }))
    await reorderNodeGroups(items)
    await load()
  } catch (err) {
    error.value = getApiErrorMessage(err, '排序失败')
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

<style scoped>
.group-list {
  display: grid;
  gap: 12px;
}
.group-card {
  border: 1px solid var(--border);
  border-radius: 16px;
  padding: 14px;
  background: var(--surface);
  display: grid;
  gap: 10px;
}
.group-card.is-empty {
  border-color: color-mix(in srgb, var(--warning, #b45309) 35%, var(--border));
}
.group-card-head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
  flex-wrap: wrap;
}
.group-title-block {
  display: flex;
  gap: 10px;
  min-width: 0;
}
.group-title-block h3 {
  margin: 0 0 6px;
  overflow-wrap: anywhere;
}
.badge-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.group-head-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.group-meta-line {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  font-size: 12px;
  color: var(--ink-soft);
}
.entry-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.inline-preview {
  border-top: 1px dashed var(--border);
  padding-top: 10px;
}
.preview-toggle {
  width: 100%;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  text-align: left;
  min-height: 40px;
}
.preview-body {
  margin-top: 10px;
  display: grid;
  gap: 6px;
}
.badge-ok {
  background: color-mix(in srgb, var(--ok, #0f8a5f) 18%, transparent);
}
.badge-warn {
  background: color-mix(in srgb, var(--warning, #b45309) 18%, transparent);
}
.toolbar-select {
  min-width: 120px;
}
.toolbar-check {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--ink-soft);
}
@media (max-width: 760px) {
  .group-head-actions {
    width: 100%;
  }
  .group-head-actions button {
    flex: 1 1 calc(50% - 6px);
    min-height: 40px;
  }
}
</style>
