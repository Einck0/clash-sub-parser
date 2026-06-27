<template>
  <section class="page rule-detail-page">
    <div class="page-head sticky-head">
      <div class="title-cluster">
        <button @click="goBack">← 返回</button>
        <div>
          <p class="eyebrow">Rule Category</p>
          <h2>{{ categoryName }}</h2>
          <p class="page-desc">{{ filteredRules.length }} / {{ rules.length }} 条规则 · {{ saveStatus }}</p>
        </div>
      </div>
      <div class="head-actions">
        <button @click="loadWithConfirm">刷新</button>
        <button @click="showPresets = true">规则模板</button>
        <button @click="showImport = true">批量导入</button>
        <button @click="createRuleRow">新增规则</button>
        <button class="primary" @click="saveAllRules" :disabled="saving || !rules.length">
          {{ saving ? '保存中...' : '保存全部' }}
        </button>
      </div>
    </div>

    <div class="alert error" v-if="error" role="alert" aria-live="assertive">{{ error }}</div>
    <div class="alert" v-if="hasUnsavedChanges && !error" role="status" aria-live="polite">有未保存更改。编辑、新增、删除、排序都会先留在本页，点击“保存全部”后同步。</div>

    <div class="toolbar-card">
      <div class="toolbar-grid">
        <label class="field search-field">
          <span>搜索</span>
          <input v-model="search" placeholder="名称 / 类型 / 规则值 / 目标 / 参数" />
        </label>
        <label class="field">
          <span>类型</span>
          <select v-model="typeFilter">
            <option value="">全部类型</option>
            <option v-for="type in availableTypes" :key="type" :value="type">{{ type }}</option>
          </select>
        </label>
        <label class="field">
          <span>目标</span>
          <select v-model="proxyFilter">
            <option value="">全部目标</option>
            <option v-for="proxy in availableProxies" :key="proxy" :value="proxy">{{ proxy }}</option>
          </select>
        </label>
        <label class="field small-field">
          <span>状态</span>
          <select v-model="enabledFilter">
            <option value="">全部</option>
            <option value="enabled">启用</option>
            <option value="disabled">禁用</option>
          </select>
        </label>
        <button class="toolbar-button" @click="resetFilters">清空</button>
      </div>
    </div>

    <div class="pager-card">
      <div class="action-row">
        <button :disabled="page <= 1" @click="page--">上一页</button>
        <span class="pager-text">第 {{ normalizedPage }} / {{ totalPages }} 页</span>
        <button :disabled="page >= totalPages" @click="page++">下一页</button>
      </div>
      <div class="action-row">
        <span class="muted">每页</span>
        <select v-model.number="pageSize" class="page-size-select">
          <option :value="20">20</option>
          <option :value="50">50</option>
          <option :value="100">100</option>
          <option :value="200">200</option>
        </select>
        <span class="muted">共 {{ filteredRules.length }} 条</span>
      </div>
    </div>

    <div class="table-card rules-table-card desktop-rules-table">
      <table class="rules-table">
        <thead>
          <tr>
            <th class="col-index">#</th>
            <th class="col-enabled">启用</th>
            <th class="col-name">名称</th>
            <th class="col-type">类型</th>
            <th class="col-value">规则值</th>
            <th class="col-proxy">目标</th>
            <th class="col-options">参数</th>
            <th class="col-actions">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="(item, idx) in pagedRules"
            :key="item._clientId"
            :class="{ dragging: draggingRuleKey === item._clientId }"
            draggable="true"
            @dragstart="onRuleDragStart(item)"
            @dragover.prevent
            @drop="onRuleDrop(item)"
            @dragend="draggingRuleKey = null"
          >
            <td class="col-index muted"><span class="drag-mini">☰</span> {{ pageStart + idx + 1 }}</td>
            <td class="col-enabled"><input type="checkbox" v-model="item.enabled" /></td>
            <td><input v-model="item.name" placeholder="可选" /></td>
            <td><select v-model="item.type"><option v-for="type in ruleTypes" :key="type" :value="type">{{ type }}</option></select></td>
            <td>
              <input
                v-model="item.value"
                :disabled="normalizeRuleType(item.type) === 'MATCH'"
                :placeholder="normalizeRuleType(item.type) === 'MATCH' ? 'MATCH 无需值' : '规则值'"
              />
            </td>
            <td>
              <select v-model="item.proxy">
                <option value="">选择目标</option>
                <option v-for="target in proxyTargets" :key="target" :value="target">{{ target }}</option>
              </select>
            </td>
            <td><input v-model="item.optionsText" placeholder="逗号分隔" /></td>
            <td>
              <div class="action-row compact-actions no-wrap">
                <select class="inline-move-select" :value="ruleIndex(item)" @change="moveRuleToIndex(item, Number($event.target.value))">
                  <option v-for="(_, targetIdx) in rules" :key="targetIdx" :value="targetIdx">#{{ targetIdx + 1 }}</option>
                </select>
                <button class="danger" @click="deleteRuleRow(item)">移除</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="pagedRules.length === 0" class="empty-state">没有匹配的规则</div>


    </div>

    <div class="mobile-rule-list">
      <article
        v-for="(item, idx) in pagedRules"
        :key="`mobile-${item._clientId}`"
        class="mobile-rule-card sortable-card"
        :class="{ dragging: draggingRuleKey === item._clientId }"
        draggable="true"
        @dragstart="onRuleDragStart(item)"
        @dragover.prevent
        @drop="onRuleDrop(item)"
        @dragend="draggingRuleKey = null"
      >
        <div class="mobile-rule-head">
          <div class="sortable-title">
            <button class="drag-handle" title="拖拽排序" @click.stop>☰</button>
            <div>
              <span class="category-index">#{{ pageStart + idx + 1 }}</span>
              <strong>{{ item.name || item.type || '未命名规则' }}</strong>
            </div>
          </div>
          <label class="switch-line"><input type="checkbox" v-model="item.enabled" /> 启用</label>
        </div>

        <div class="mobile-rule-fields">
          <label class="field"><span>名称</span><input v-model="item.name" placeholder="可选" /></label>
          <label class="field"><span>类型</span><select v-model="item.type"><option v-for="type in ruleTypes" :key="type" :value="type">{{ type }}</option></select></label>
          <label class="field wide-field">
            <span>规则值</span>
            <input
              v-model="item.value"
              :disabled="normalizeRuleType(item.type) === 'MATCH'"
              :placeholder="normalizeRuleType(item.type) === 'MATCH' ? 'MATCH 无需值' : '规则值'"
            />
          </label>
          <label class="field"><span>目标</span><select v-model="item.proxy"><option value="">选择目标</option><option v-for="target in proxyTargets" :key="target" :value="target">{{ target }}</option></select></label>
          <label class="field"><span>参数</span><input v-model="item.optionsText" placeholder="逗号分隔" /></label>
          <label class="field"><span>移动到</span><select :value="ruleIndex(item)" @change="moveRuleToIndex(item, Number($event.target.value))"><option v-for="(_, targetIdx) in rules" :key="targetIdx" :value="targetIdx">第 {{ targetIdx + 1 }} 位</option></select></label>
        </div>

        <div class="action-row compact-actions">
          <button class="danger" @click="deleteRuleRow(item)">移除</button>
        </div>
      </article>
      <div v-if="pagedRules.length === 0" class="empty-state">没有匹配的规则</div>
    </div>

    <RulePresetsModal
      v-if="showPresets"
      :proxy-options="proxyTargets"
      :categories="[]"
      @apply="applyPresets"
      @close="showPresets = false"
    />

    <RuleImportModal
      v-if="showImport"
      :proxy-options="proxyTargets"
      :categories="[]"
      @apply="applyPresets"
      @close="showImport = false"
    />

    <FabSave :visible="hasUnsavedChanges" :saving="saving" @save="saveAllRules" />
  </section>
</template>

<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore } from '../stores/app'
import { useUrlState } from '../utils/urlState'

const store = useAppStore()
import { createRule, deleteRule, getApiErrorMessage, getNodeGroups, getRules, updateRule, batchRules } from '../api'
import { BUILTINS, RULE_TYPES, normalizeRuleType, parseOptions, proxyTargetsFromGroups } from '../utils/ruleUtils'
import RulePresetsModal from '../components/RulePresetsModal.vue'
import RuleImportModal from '../components/RuleImportModal.vue'
import FabSave from '../components/FabSave.vue'

const route = useRoute()
const router = useRouter()
const categoryName = computed(() => String(route.params.name || 'default'))
const rules = ref([])
const nodeGroups = ref([])
const deletedRuleIds = ref([])
const error = ref('')
const search = useUrlState('q', '')
const typeFilter = useUrlState('type', '')
const proxyFilter = useUrlState('proxy', '')
const enabledFilter = useUrlState('status', '')
const page = ref(1)
const pageSize = ref(50)
const hasUnsavedChanges = ref(false)
const saving = ref(false)
const suppressDirty = ref(false)
const draggingRuleKey = ref(null)
const showPresets = ref(false)
const showImport = ref(false)
let clientIdSeq = 1

const builtins = BUILTINS
const ruleTypes = RULE_TYPES
const proxyTargets = computed(() => proxyTargetsFromGroups(nodeGroups.value))
const saveStatus = computed(() => saving.value ? '正在同步' : (hasUnsavedChanges.value ? '有未保存更改' : '已同步'))

onMounted(() => {
  load()
  window.addEventListener('keydown', handleGlobalKeydown)
})
onUnmounted(() => window.removeEventListener('keydown', handleGlobalKeydown))

function handleGlobalKeydown(e) {
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault()
    if (hasUnsavedChanges.value && !saving.value) saveAllRules()
  }
}
watch([search, typeFilter, proxyFilter, enabledFilter, pageSize], () => { page.value = 1 })
watch(categoryName, () => load())
watch(
  rules,
  () => {
    if (!suppressDirty.value) hasUnsavedChanges.value = true
  },
  { deep: true }
)

const availableTypes = computed(() => [...new Set(rules.value.map((item) => item.type).filter(Boolean))].sort())
const availableProxies = computed(() => [...new Set(rules.value.map((item) => item.proxy).filter(Boolean))].sort())

const filteredRules = computed(() => {
  const q = search.value.trim().toLowerCase()
  return rules.value.filter((item) => {
    if (typeFilter.value && item.type !== typeFilter.value) return false
    if (proxyFilter.value && item.proxy !== proxyFilter.value) return false
    if (enabledFilter.value === 'enabled' && !item.enabled) return false
    if (enabledFilter.value === 'disabled' && item.enabled) return false
    if (!q) return true
    const haystack = [item.name, item.type, item.value, item.proxy, ...(item.options || []), item.optionsText].join(' ').toLowerCase()
    return haystack.includes(q)
  })
})
const totalPages = computed(() => Math.max(1, Math.ceil(filteredRules.value.length / pageSize.value)))
const normalizedPage = computed(() => Math.min(page.value, totalPages.value))
const pageStart = computed(() => (normalizedPage.value - 1) * pageSize.value)
const pagedRules = computed(() => filteredRules.value.slice(pageStart.value, pageStart.value + pageSize.value))

async function load() {
  error.value = ''
  suppressDirty.value = true
  try {
    const [rulesRes, groupsRes] = await Promise.all([getRules(), getNodeGroups()])
    nodeGroups.value = groupsRes.data
    rules.value = rulesRes.data
      .filter((item) => (item.category || 'default') === categoryName.value)
      .sort(compareRuleOrder)
      .map(toEditableRule)
    deletedRuleIds.value = []
    page.value = 1
    await nextTick()
    hasUnsavedChanges.value = false
  } catch (err) {
    error.value = getApiErrorMessage(err, '加载规则失败')
  } finally {
    suppressDirty.value = false
  }
}

function toEditableRule(item) {
  return {
    enabled: true,
    options: [],
    ...item,
    _clientId: `rule-${item.id || clientIdSeq++}`,
    optionsText: (item.options || []).join(','),
  }
}

function createRuleRow() {
  rules.value.unshift({
    id: null,
    _clientId: `new-${clientIdSeq++}`,
    name: '',
    category: categoryName.value,
    type: 'DOMAIN-SUFFIX',
    value: '',
    proxy: 'PROXY',
    options: [],
    optionsText: '',
    sort_order: 0,
    enabled: true,
  })
  normalizeSortOrder()
  page.value = 1
}

async function saveAllRules() {
  error.value = ''
  saving.value = true
  const sorted = [...rules.value].sort(compareRuleOrder)
  sorted.forEach((rule, index) => { rule.sort_order = index })

  try {
    const batch = { delete: [], create: [], update: [], reorder: [] }

    // Pending deletes
    const toDelete = [...deletedRuleIds.value]
    deletedRuleIds.value = []
    batch.delete = toDelete

    for (const item of sorted) {
      const payload = normalizePayload(item)
      if (item.id) {
        batch.update.push({ id: item.id, ...payload })
      } else {
        batch.create.push(payload)
      }
    }

    batch.reorder = sorted
      .filter((rule) => rule.id)
      .map((rule) => ({ id: rule.id, sort_order: rule.sort_order }))

    const { data } = await batchRules(batch)
    // Sync local state with server response - filter to current category only
    suppressDirty.value = true
    const allRules = (data || []).map(toEditableRule)
    rules.value = allRules.filter((r) => (r.category || 'default') === categoryName.value)
    deletedRuleIds.value = []
    hasUnsavedChanges.value = false
    await nextTick()
    suppressDirty.value = false
    store.success(`已保存 ${sorted.length} 条规则`)
  } catch (err) {
    error.value = getApiErrorMessage(err, '保存全部规则失败')
  } finally {
    saving.value = false
  }
}

async function deleteRuleRow(item) {
  const ok = await store.confirm({
    title: '移除规则',
    message: `从当前草稿移除规则 ${item.name || item.type || ''}？点击“保存全部”后才会真正同步删除。`,
    confirmText: '移除',
    danger: true,
  })
  if (!ok) return
  if (item.id && !deletedRuleIds.value.includes(item.id)) deletedRuleIds.value.push(item.id)
  rules.value = rules.value.filter((rule) => rule !== item)
  normalizeSortOrder()
  hasUnsavedChanges.value = true
}

function onRuleDragStart(item) {
  draggingRuleKey.value = item._clientId
}

function onRuleDrop(targetItem) {
  const fromKey = draggingRuleKey.value
  draggingRuleKey.value = null
  if (!fromKey || fromKey === targetItem._clientId) return
  const sorted = [...rules.value].sort(compareRuleOrder)
  const from = sorted.findIndex((rule) => rule._clientId === fromKey)
  const to = sorted.findIndex((rule) => rule._clientId === targetItem._clientId)
  if (from < 0 || to < 0) return
  reorderRulesLocal(sorted, from, to)
}

function moveRuleToIndex(item, targetIndex) {
  const sorted = [...rules.value].sort(compareRuleOrder)
  const from = sorted.findIndex((rule) => rule === item || rule._clientId === item._clientId)
  if (from < 0 || targetIndex < 0 || targetIndex >= sorted.length || from === targetIndex) return
  reorderRulesLocal(sorted, from, targetIndex)
}

function reorderRulesLocal(sorted, from, to) {
  const [moved] = sorted.splice(from, 1)
  sorted.splice(to, 0, moved)
  sorted.forEach((rule, i) => { rule.sort_order = i })
  rules.value = sorted
  hasUnsavedChanges.value = true
}

function ruleIndex(item) {
  // Cache: use the pre-computed sorted array instead of sorting on every call
  return sortedRules.value.findIndex((rule) => rule === item || rule._clientId === item._clientId)
}

const sortedRules = computed(() => [...rules.value].sort(compareRuleOrder))

function normalizeSortOrder() {
  rules.value.sort(compareRuleOrder).forEach((rule, index) => { rule.sort_order = index })
}

function resetFilters() {
  search.value = ''
  typeFilter.value = ''
  proxyFilter.value = ''
  enabledFilter.value = ''
}

async function goBack() {
  if (hasUnsavedChanges.value) {
    const ok = await store.confirm({
      title: '未保存更改',
      message: '有未保存更改，确定返回吗？',
      confirmText: '返回',
      danger: true,
    })
    if (!ok) return
  }
  router.push('/rules')
}

async function loadWithConfirm() {
  if (hasUnsavedChanges.value) {
    const ok = await store.confirm({
      title: '刷新确认',
      message: '刷新会丢弃当前未保存更改，确定刷新吗？',
      confirmText: '刷新',
      danger: true,
    })
    if (!ok) return
  }
  load()
}

function normalizePayload(item) {
  const type = normalizeRuleType(item.type)
  return {
    name: String(item.name || '').trim(),
    category: categoryName.value,
    type,
    value: type === 'MATCH' ? '' : String(item.value || '').trim(),
    proxy: String(item.proxy || '').trim(),
    options: parseOptions(item.optionsText),
    sort_order: item.sort_order ?? 0,
    enabled: !!item.enabled,
  }
}

function compareRuleOrder(a, b) {
  return (a.sort_order ?? 0) - (b.sort_order ?? 0) || (a.id ?? Number.MAX_SAFE_INTEGER) - (b.id ?? Number.MAX_SAFE_INTEGER)
}

function applyPresets(presetRules) {
  for (const rule of presetRules) {
    rules.value.push({
      id: null,
      _clientId: `preset-${clientIdSeq++}`,
      name: rule.name || '',
      category: categoryName.value,
      type: rule.type,
      value: rule.value,
      proxy: rule.proxy,
      options: rule.options || [],
      optionsText: (rule.options || []).join(','),
      sort_order: rules.value.length,
      enabled: true,
    })
  }
  hasUnsavedChanges.value = true
  store.success(`已添加 ${presetRules.length} 条规则，记得点"保存全部"同步到服务端`)
}
</script>
