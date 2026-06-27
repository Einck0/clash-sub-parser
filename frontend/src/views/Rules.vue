<template>
  <section class="page rules-home">
    <div class="page-head">
      <div>
        <p class="eyebrow">Rules</p>
        <h2>规则分类</h2>
        <p class="page-desc">按用途管理规则。拖动卡片可排序；改名、新增、移除和排序都会先进入草稿。</p>
      </div>
      <div class="head-actions">
        <button @click="loadWithConfirm">刷新</button>
        <button @click="createCategoryRow">新增类别</button>
        <button class="primary" @click="saveAllCategories" :disabled="saving || !categories.length">
          {{ saving ? '保存中...' : '保存全部' }}
        </button>
      </div>
    </div>

    <div class="alert error" v-if="error" role="alert" aria-live="assertive">{{ error }}</div>
    <div class="alert" v-if="hasUnsavedChanges && !error" role="status" aria-live="polite">有未保存更改。分类改名、新增、移除、排序都会先留在本页，点击“保存全部”后同步。</div>

    <div class="summary-grid">
      <div class="metric-card">
        <span class="metric-label">类别</span>
        <strong>{{ sortedCategories.length }}</strong>
      </div>
      <div class="metric-card">
        <span class="metric-label">规则总数</span>
        <strong>{{ totalRules }}</strong>
      </div>
      <div class="metric-card wide">
        <span class="metric-label">状态</span>
        <span class="metric-tip">{{ saveStatus }}。点击卡片进入编辑；按住左上角拖拽柄可以移动排序。</span>
      </div>
    </div>

    <section class="rule-search-card" aria-label="规则搜索">
      <div class="rule-search-head">
        <div>
          <h3>全局规则搜索</h3>
          <p class="section-hint">按规则名、类型、值、代理策略或分类搜索，结果会显示来源分类和具体规则内容。</p>
        </div>
        <span class="count-pill">{{ ruleSearch.trim() ? `${ruleSearchResults.length} / ${allRules.length}` : `${allRules.length} 条规则` }}</span>
      </div>
      <input v-model.trim="ruleSearch" class="rule-search-input" placeholder="例如 openai / DOMAIN-SUFFIX / DIRECT / 广告" />

      <div v-if="ruleSearch.trim()" class="rule-search-results" role="list" aria-live="polite">
        <article v-for="rule in limitedRuleSearchResults" :key="rule.id" class="rule-result-card" role="listitem">
          <div class="rule-result-top">
            <div>
              <span class="category-index">来自分类</span>
              <button class="link-button" @click="openCategoryByName(rule.category)">{{ rule.category || '未分类' }}</button>
            </div>
            <span class="count-pill" :class="{ muted: !rule.enabled }">{{ rule.enabled ? '启用' : '禁用' }}</span>
          </div>
          <div class="rule-result-main">
            <strong>{{ rule.name || `${rule.type || 'RULE'} ${rule.value || ''}` }}</strong>
            <code>{{ formatRule(rule) }}</code>
          </div>
          <div class="rule-result-meta">
            <span>{{ rule.type || '-' }}</span>
            <span>{{ rule.proxy || '-' }}</span>
            <span v-if="(rule.options || []).length">{{ rule.options.join(', ') }}</span>
          </div>
        </article>
        <UiState v-if="!ruleSearchResults.length" type="empty" title="没有匹配规则" description="换个关键词试试，比如域名、规则类型、代理组或分类名。" compact />
        <div v-if="ruleSearchResults.length > limitedRuleSearchResults.length" class="empty-mini">
          已显示前 {{ limitedRuleSearchResults.length }} 条结果，请输入更精确的关键词继续缩小范围。
        </div>
      </div>
    </section>

    <div class="category-grid sortable-grid">
      <article
        v-for="(cat, idx) in sortedCategories"
        :key="cat._clientId || cat.id || `new-${idx}`"
        class="category-card sortable-card"
        :class="{ dragging: draggingCategoryKey === categoryKey(cat) }"
        draggable="true"
        @dragstart="onCategoryDragStart(cat)"
        @dragover.prevent
        @drop="onCategoryDrop(cat)"
        @dragend="draggingCategoryKey = null"
      >
        <div class="category-top" @click="cat.id && openCategory(cat)">
          <div class="sortable-title">
            <button class="drag-handle" title="拖拽排序" @click.stop>☰</button>
            <div>
              <span class="category-index">#{{ idx + 1 }}</span>
              <h3>{{ cat.name || '未命名类别' }}</h3>
            </div>
          </div>
          <span class="count-pill">{{ cat.rule_count || 0 }} 条</span>
        </div>

        <label class="field-label">类别名</label>
        <input v-model="cat.name" placeholder="类别名" />

        <div class="sort-control-row">
          <span class="muted">位置：第 {{ idx + 1 }} 位</span>
          <label class="move-select">
            <span>移动到</span>
            <select :value="idx" @change="moveCategoryToIndex(cat, Number($event.target.value))">
              <option v-for="(_, targetIdx) in sortedCategories" :key="targetIdx" :value="targetIdx">第 {{ targetIdx + 1 }} 位</option>
            </select>
          </label>
        </div>

        <div class="action-row compact-actions">
          <button class="primary" @click="openCategory(cat)">进入编辑</button>
          <button class="danger" @click="removeCategoryRow(cat)">移除</button>
        </div>
      </article>
    </div>

    <FabSave :visible="hasUnsavedChanges" :saving="saving" @save="saveAllCategories" />
  </section>
</template>

<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useAppStore } from '../stores/app'

const store = useAppStore()
import {
  batchRuleCategories,
  createRuleCategory,
  deleteRuleCategory,
  getApiErrorMessage,
  getRuleCategories,
  getRules,
  reorderRuleCategories,
  updateRuleCategory,
} from '../api'
import UiState from '../components/UiState.vue'
import FabSave from '../components/FabSave.vue'

const router = useRouter()
const categories = ref([])
const deletedCategoryIds = ref([])
const allRules = ref([])
const ruleSearch = ref('')
const error = ref('')
const draggingCategoryKey = ref(null)
const hasUnsavedChanges = ref(false)
const saving = ref(false)
const suppressDirty = ref(false)
let clientIdSeq = 1

function handleGlobalKeydown(e) {
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault()
    if (hasUnsavedChanges.value && !saving.value) saveAllCategories()
  }
}
onMounted(() => {
  load()
  window.addEventListener('keydown', handleGlobalKeydown)
})
onUnmounted(() => window.removeEventListener('keydown', handleGlobalKeydown))

watch(
  categories,
  () => {
    if (!suppressDirty.value) hasUnsavedChanges.value = true
  },
  { deep: true }
)

const sortedCategories = computed(() => [...categories.value].sort(compareCategoryOrder))
const totalRules = computed(() => allRules.value.length || sortedCategories.value.reduce((sum, cat) => sum + Number(cat.rule_count || 0), 0))
const saveStatus = computed(() => saving.value ? '正在同步' : (hasUnsavedChanges.value ? '有未保存更改' : '已同步'))
const ruleSearchResults = computed(() => {
  const q = debouncedRuleSearch.value.trim().toLowerCase()
  if (!q) return []
  return allRules.value.filter((rule) => ruleSearchText(rule).includes(q))
})
const debouncedRuleSearch = ref('')
let _searchTimer = null
watch(ruleSearch, (val) => {
  clearTimeout(_searchTimer)
  _searchTimer = setTimeout(() => { debouncedRuleSearch.value = val }, 250)
}, { immediate: true })
const limitedRuleSearchResults = computed(() => ruleSearchResults.value.slice(0, 80))

async function load() {
  error.value = ''
  suppressDirty.value = true
  try {
    const [catRes, ruleRes] = await Promise.all([getRuleCategories(), getRules()])
    categories.value = catRes.data.map((cat) => ({ ...cat, _clientId: `cat-${cat.id}` }))
    allRules.value = ruleRes.data
    deletedCategoryIds.value = []
    await nextTick()
    hasUnsavedChanges.value = false
  } catch (err) {
    error.value = getApiErrorMessage(err, '加载类别失败')
  } finally {
    suppressDirty.value = false
  }
}

async function loadWithConfirm() {
  if (hasUnsavedChanges.value) {
    const ok = await store.confirm({
      title: '刷新确认',
      message: '刷新会丢弃当前未保存的分类草稿，确定刷新吗？',
      confirmText: '刷新',
      danger: true,
    })
    if (!ok) return
  }
  load()
}

function createCategoryRow() {
  categories.value.push({ id: null, _clientId: `new-cat-${clientIdSeq++}`, name: `新类别${categories.value.length + 1}`, sort_order: categories.value.length * 10, rule_count: 0 })
}

async function saveAllCategories() {
  error.value = ''
  saving.value = true
  const sorted = sortedCategories.value
  sorted.forEach((cat, index) => { cat.sort_order = index * 10 })

  try {
    for (const cat of sorted) {
      if (!String(cat.name || '').trim()) {
        throw new Error('类别名不能为空')
      }
    }

    const batch = { delete: [], create: [], update: [], reorder: [] }

    batch.delete = [...deletedCategoryIds.value]
    deletedCategoryIds.value = []

    for (const cat of sorted) {
      const payload = { name: String(cat.name || '').trim(), sort_order: cat.sort_order ?? 0 }
      if (cat.id) {
        batch.update.push({ id: cat.id, ...payload })
      } else {
        batch.create.push(payload)
      }
    }

    batch.reorder = sorted
      .filter((cat) => cat.id)
      .map((cat) => ({ id: cat.id, sort_order: cat.sort_order }))

    await batchRuleCategories(batch)
    store.success(`已保存 ${sorted.length} 个分类`)
    await load()
  } catch (err) {
    error.value = err?.userMessage || err?.message || getApiErrorMessage(err, '保存分类失败')
  } finally {
    saving.value = false
  }
}

async function removeCategoryRow(cat) {
  const count = cat.rule_count || 0
  const msg = cat.id && count > 0
    ? `从草稿中移除类别 ${cat.name}。点击“保存全部”后会同时删除其中 ${count} 条规则，确定继续？`
    : `从草稿中移除类别 ${cat.name || '未命名类别'}？`
  const ok = await store.confirm({ title: '移除类别', message: msg, confirmText: '移除', danger: true })
  if (!ok) return
  if (cat.id && !deletedCategoryIds.value.includes(cat.id)) deletedCategoryIds.value.push(cat.id)
  categories.value = categories.value.filter((item) => item !== cat)
  normalizeSortOrder()
  hasUnsavedChanges.value = true
}

function onCategoryDragStart(cat) {
  draggingCategoryKey.value = categoryKey(cat)
}

function onCategoryDrop(targetCat) {
  const fromKey = draggingCategoryKey.value
  draggingCategoryKey.value = null
  if (!fromKey || fromKey === categoryKey(targetCat)) return
  const sorted = sortedCategories.value
  const from = sorted.findIndex((item) => categoryKey(item) === fromKey)
  const to = sorted.findIndex((item) => categoryKey(item) === categoryKey(targetCat))
  if (from < 0 || to < 0) return
  reorderCategoriesLocal(sorted, from, to)
}

function moveCategoryToIndex(cat, targetIndex) {
  const sorted = sortedCategories.value
  const from = sorted.findIndex((item) => item === cat || categoryKey(item) === categoryKey(cat))
  if (from < 0 || targetIndex < 0 || targetIndex >= sorted.length || from === targetIndex) return
  reorderCategoriesLocal(sorted, from, targetIndex)
}

function reorderCategoriesLocal(sorted, from, to) {
  const [moved] = sorted.splice(from, 1)
  sorted.splice(to, 0, moved)
  sorted.forEach((item, i) => { item.sort_order = i * 10 })
  categories.value = sorted
  hasUnsavedChanges.value = true
}

function normalizeSortOrder() {
  categories.value.sort(compareCategoryOrder).forEach((cat, index) => { cat.sort_order = index * 10 })
}

function categoryKey(cat) {
  return cat._clientId || `cat-${cat.id}`
}

function compareCategoryOrder(a, b) {
  return (a.sort_order ?? 0) - (b.sort_order ?? 0) || (a.id ?? Number.MAX_SAFE_INTEGER) - (b.id ?? Number.MAX_SAFE_INTEGER)
}

function openCategory(cat) {
  if (!cat.id) {
    store.warning('请先点击“保存全部”保存新类别')
    return
  }
  openCategoryByName(cat.name)
}

async function openCategoryByName(categoryName) {
  if (!categoryName) return
  if (hasUnsavedChanges.value) {
    const ok = await store.confirm({
      title: '未保存更改',
      message: '当前分类页有未保存更改，进入编辑前建议先保存。仍要进入吗？',
      confirmText: '继续进入',
      danger: true,
    })
    if (!ok) return
  }
  router.push(`/rules/category/${encodeURIComponent(categoryName)}`)
}

function ruleSearchText(rule) {
  return [
    rule.name,
    rule.category,
    rule.type,
    rule.value,
    rule.proxy,
    ...(rule.options || []),
  ].filter(Boolean).join(' ').toLowerCase()
}

function formatRule(rule) {
  return [rule.type, rule.value, rule.proxy, ...(rule.options || [])].filter(Boolean).join(',')
}
</script>
