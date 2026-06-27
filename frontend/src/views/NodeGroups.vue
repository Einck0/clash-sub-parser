<template>
  <section class="page node-groups-page">
    <div class="page-head">
      <div>
        <p class="eyebrow">Proxy Groups</p>
        <h2>节点组管理</h2>
        <p class="page-desc">按输出顺序管理策略组；预览区展示最终解析后的节点与引用关系。</p>
      </div>
      <div class="head-actions">
        <button @click="validateRefs" :disabled="loading || working">{{ working === 'validate' ? '校验中...' : '校验引用' }}</button>
        <button @click="loadPreview" :disabled="loading || working">{{ working === 'preview' ? '刷新中...' : '刷新预览' }}</button>
        <button class="primary" @click="openCreate">添加节点组</button>
      </div>
    </div>

    <UiState v-if="error" type="error" title="节点组操作失败" :description="error" compact>
      <template #actions>
        <button @click="load">重新加载</button>
      </template>
    </UiState>
    <UiState v-if="loading && !groups.length" type="loading" title="正在加载节点组" description="正在同步策略组和预览数据。" />

    <template v-else>
    <div class="summary-grid node-summary">
      <div class="metric-card">
        <span class="metric-label">节点组</span>
        <strong>{{ groups.length }}</strong>
      </div>
      <div class="metric-card">
        <span class="metric-label">预览组</span>
        <strong>{{ previews.length }}</strong>
      </div>
      <div class="metric-card wide">
        <span class="metric-label">说明</span>
        <span class="metric-tip">"节点组引用"输出策略组名；"节点组节点"会展开引用组内的节点。</span>
      </div>
    </div>

    <div class="node-group-grid">
      <article v-for="(group, idx) in groups" :key="group.id" class="node-group-card">
        <div class="node-group-head">
          <div>
            <span class="category-index">#{{ idx + 1 }}</span>
            <h3>{{ group.name }}</h3>
          </div>
          <span class="count-pill">{{ group.group_type }}</span>
        </div>

        <div class="node-group-meta">
          <div><span>kind</span><strong>{{ group.kind }}</strong></div>
          <div><span>静态节点</span><strong>{{ (group.include_nodes || []).length }}</strong></div>
          <div><span>组引用</span><strong>{{ (group.include_group_ids || []).length }}</strong></div>
          <div><span>组节点</span><strong>{{ (group.include_group_nodes_ids || []).length }}</strong></div>
          <div><span>兜底</span><strong>{{ group.add_fallback === false ? '关闭' : 'REJECT' }}</strong></div>
        </div>

        <div class="node-group-tags" v-if="previewById(group.id)?.include_entries?.length">
          <span v-for="entry in previewById(group.id).include_entries.slice(0, 6)" :key="`${entry.type}-${entry.value}`" class="badge">
            {{ formatEntry(entry) }}
          </span>
          <span v-if="previewById(group.id).include_entries.length > 6" class="badge">+{{ previewById(group.id).include_entries.length - 6 }}</span>
        </div>

        <div class="action-row compact-actions">
          <button @click="move(idx,-1)" :disabled="idx===0">上移</button>
          <button @click="move(idx,1)" :disabled="idx===groups.length-1">下移</button>
          <button class="primary" @click="openEdit(group)">编辑</button>
          <button class="danger" @click="remove(group)">删除</button>
        </div>
      </article>
      <UiState v-if="!groups.length" type="empty" title="暂无节点组" description="创建策略组后，可以在预览区检查引用关系和最终输出节点。">
        <template #actions>
          <button class="primary" @click="openCreate">添加节点组</button>
        </template>
      </UiState>
    </div>

    <div class="preview-panel">
      <div class="row space preview-title">
        <div>
          <h3>节点组预览</h3>
          <p class="section-hint">展示每个节点组最终会输出哪些节点。长列表默认只显示前 18 个。</p>
        </div>
        <span class="muted">{{ previews.length }} 组</span>
      </div>

      <div class="preview-grid">
        <article v-for="preview in previews" :key="preview.id" class="preview-card">
          <div class="row space">
            <strong>{{ preview.name }}</strong>
            <span class="count-pill">{{ preview.resolved_count }} 节点</span>
          </div>
          <div class="muted small-line" v-if="preview.include_group_names?.length">
            引用节点组：{{ preview.include_group_names.join('、') }}
          </div>
          <div class="muted small-line" v-if="preview.include_group_nodes_names?.length">
            引入节点组节点：{{ preview.include_group_nodes_names.map((n) => `${n}(节点)`).join('、') }}
          </div>
          <NodePreviewList :nodes="preview.resolved_nodes || []" :collapsed-limit="18" placeholder="搜索此组节点" />
        </article>
      </div>
    </div>

    </template>

    <NodeGroupModal v-if="showModal" :group="editing" @saved="onSaved" @close="showModal = false" />
  </section>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useAppStore } from '../stores/app'
import { deleteNodeGroup, getApiErrorMessage, getNodeGroups, previewNodeGroups, reorderNodeGroups, validateNodeGroups } from '../api'
import NodePreviewList from '../components/NodePreviewList.vue'
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

onMounted(load)

// Esc to close modal
function onKeydown(e) {
  if (e.key === 'Escape' && showModal.value) showModal.value = false
}
window.addEventListener('keydown', onKeydown)
onUnmounted(() => window.removeEventListener('keydown', onKeydown))

const previewMap = computed(() => new Map(previews.value.map((item) => [item.id, item])))
function previewById(id) { return previewMap.value.get(id) }

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [groupsRes, previewsRes] = await Promise.all([getNodeGroups(), previewNodeGroups()])
    groups.value = groupsRes.data
    previews.value = previewsRes.data
  } catch (err) {
    error.value = getApiErrorMessage(err, '加载节点组失败')
  } finally {
    loading.value = false
  }
}

async function loadPreview() {
  working.value = 'preview'
  error.value = ''
  try {
    const { data } = await previewNodeGroups()
    previews.value = data
    store.success('预览已刷新')
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

function onSaved() {
  showModal.value = false
  store.success('节点组已保存')
  load()
}

async function remove(group) {
  const ok = await store.confirm({
    title: '删除节点组',
    message: `确定要删除节点组 "${group.name}" 吗？`,
    confirmText: '删除',
    danger: true,
  })
  if (!ok) return
  error.value = ''
  try {
    await deleteNodeGroup(group.id)
    store.success(`已删除节点组 ${group.name}`)
    await load()
  } catch (err) {
    error.value = getApiErrorMessage(err, '删除节点组失败')
  }
}

async function move(index, delta) {
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
    error.value = getApiErrorMessage(err, '移动节点组失败')
  }
}

async function validateRefs() {
  working.value = 'validate'
  error.value = ''
  try {
    await validateNodeGroups()
    store.success('节点组引用校验通过')
  } catch (err) {
    error.value = getApiErrorMessage(err, '校验失败')
  } finally {
    working.value = ''
  }
}

function formatEntry(entry) {
  if (entry.type === 'node') return `节点:${entry.value}`
  if (entry.type === 'group') {
    const g = groups.value.find((item) => item.id === entry.value)
    return `组:${g ? g.name : `#${entry.value}`}`
  }
  if (entry.type === 'group_nodes') {
    const g = groups.value.find((item) => item.id === entry.value)
    return `组节点:${g ? g.name : `#${entry.value}`}`
  }
  return JSON.stringify(entry)
}
</script>
