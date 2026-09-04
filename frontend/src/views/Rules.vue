<template>
  <section class="space-y-6">
    <!-- Top Header -->
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 pb-4 border-b border-white/10">
      <div>
        <p class="text-xs font-mono text-blue-400 uppercase tracking-wider">Routing Rules Matrix</p>
        <h2 class="text-xl font-bold text-white tracking-tight">规则分类</h2>
        <p class="text-xs text-slate-400 mt-1">
          按用途管理规则。按住 ☰ 拖拽排序；改名、新增、移除和排序都会先进入草稿，确认后点击“保存全部”。
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2.5 w-full sm:w-auto">
        <button
          class="inline-flex items-center justify-center min-h-[44px] gap-1.5 rounded-lg border border-white/10 bg-slate-800/40 px-3.5 py-2 text-xs font-medium text-slate-300 hover:bg-slate-800 transition-colors cursor-pointer flex-1 sm:flex-initial"
          @click="loadWithConfirm"
        >
          刷新
        </button>
        <button
          class="inline-flex items-center justify-center min-h-[44px] gap-1.5 rounded-lg border border-white/10 bg-slate-800/40 px-3.5 py-2 text-xs font-medium text-slate-300 hover:bg-slate-800 transition-colors cursor-pointer flex-1 sm:flex-initial"
          @click="createCategoryRow"
        >
          新增类别
        </button>
        <button
          class="inline-flex items-center justify-center min-h-[44px] gap-1.5 rounded-lg border border-blue-500/40 bg-blue-600/20 px-4 py-2 text-xs font-medium text-blue-400 hover:bg-blue-600/30 transition-colors cursor-pointer flex-1 sm:flex-initial"
          :disabled="saving || !categories.length"
          @click="saveAllCategories"
        >
          {{ saving ? '保存中...' : '保存全部' }}
        </button>
      </div>
    </div>

    <!-- Alert / Status Notice -->
    <div
      v-if="error"
      class="p-4 rounded-xl border border-rose-500/30 bg-rose-500/10 text-xs font-mono text-rose-400"
      role="alert"
      aria-live="assertive"
    >
      {{ error }}
    </div>
    <div
      v-if="hasUnsavedChanges && !error"
      class="p-4 rounded-xl border border-amber-500/30 bg-amber-500/10 text-xs font-mono text-amber-300 flex items-center gap-2"
      role="status"
      aria-live="polite"
    >
      <span class="h-2 w-2 rounded-full bg-amber-400 animate-pulse"></span>
      <span>有未保存更改。分类改名、新增、移除、排序都会先留在本页，点击“保存全部”后同步。</span>
    </div>

    <!-- Industrial MetricCards Grid (Responsive: 1 col mobile, 2 cols sm, 3 cols md+) -->
    <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-3 sm:gap-4">
      <MetricCard
        label="RULE CATEGORIES"
        :value="sortedCategories.length"
        subtext="已配置规则类别总数"
        status="info"
      />
      <MetricCard
        label="TOTAL RULES"
        :value="totalRules"
        subtext="全量分流规则条目"
        status="success"
      />
      <MetricCard
        label="SYNC STATUS"
        :value="saveStatus"
        description="点击卡片进入详情编辑；按住左侧 ☰ 拖拽柄或使用下拉位置选择器调整顺序。"
        :status="hasUnsavedChanges ? 'warning' : 'neutral'"
      />
    </div>

    <!-- Global Rule Search Card -->
    <section class="rounded-xl border border-white/10 bg-slate-900/60 backdrop-blur-md p-4 sm:p-5 space-y-4" aria-label="规则搜索">
      <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-2">
        <div>
          <h3 class="text-sm font-semibold text-white tracking-tight">全局规则搜索</h3>
          <p class="text-xs text-slate-400 mt-0.5">按规则名、类型、值、代理策略或分类搜索，展示具体匹配规则。</p>
        </div>
        <span class="rounded bg-slate-800/60 border border-white/5 px-2.5 py-1 text-xs font-mono text-slate-300">
          {{ ruleSearch.trim() ? `${ruleSearchResults.length} / ${allRules.length}` : `${allRules.length} 条规则` }}
        </span>
      </div>

      <div class="relative">
        <input
          v-model.trim="ruleSearch"
          type="search"
          placeholder="例如 openai / DOMAIN-SUFFIX / DIRECT / 广告…"
          class="w-full min-h-[44px] rounded-lg border border-white/10 bg-slate-950/60 px-3.5 py-2.5 pl-9 text-xs text-white placeholder-slate-500 focus:border-blue-500 focus:outline-hidden font-mono transition-colors"
        />
        <svg class="absolute left-3 top-3.5 h-4 w-4 text-slate-500 pointer-events-none" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
      </div>

      <!-- Search Results -->
      <div v-if="ruleSearch.trim()" class="space-y-3 pt-2" role="list" aria-live="polite">
        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          <article
            v-for="rule in limitedRuleSearchResults"
            :key="rule.id"
            class="flex flex-col justify-between p-3 rounded-lg border border-white/5 bg-slate-950/40 text-xs font-mono space-y-2 hover:border-white/10 transition-colors"
            role="listitem"
          >
            <div class="flex items-center justify-between gap-2">
              <div class="flex items-center gap-1.5 truncate">
                <span class="text-[10px] text-slate-500">来自分类:</span>
                <button
                  class="text-blue-400 hover:text-blue-300 hover:underline font-semibold cursor-pointer truncate"
                  @click="openCategoryByName(rule.category)"
                >
                  {{ rule.category || '未分类' }}
                </button>
              </div>
              <span
                class="px-1.5 py-0.5 rounded text-[10px] border"
                :class="rule.enabled ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20' : 'bg-slate-800 text-slate-500 border-white/5'"
              >
                {{ rule.enabled ? '启用' : '禁用' }}
              </span>
            </div>

            <div class="space-y-1">
              <strong class="text-white block truncate" :title="rule.name || rule.value">
                {{ rule.name || `${rule.type || 'RULE'} ${rule.value || ''}` }}
              </strong>
              <code class="block text-[11px] text-slate-400 bg-slate-900/60 p-1.5 rounded border border-white/5 truncate">
                {{ formatRule(rule) }}
              </code>
            </div>

            <div class="flex items-center justify-between text-[11px] text-slate-500 pt-1 border-t border-white/5">
              <span>{{ rule.type || '-' }}</span>
              <span class="text-blue-400 font-medium">{{ rule.proxy || '-' }}</span>
              <span v-if="(rule.options || []).length">{{ rule.options.join(', ') }}</span>
            </div>
          </article>
        </div>

        <UiState
          v-if="!ruleSearchResults.length"
          type="empty"
          title="没有匹配规则"
          description="换个关键词试试，比如域名、规则类型、代理组或分类名。"
          compact
        />
        <div v-if="ruleSearchResults.length > limitedRuleSearchResults.length" class="text-xs font-mono text-slate-500 text-center py-2">
          已显示前 {{ limitedRuleSearchResults.length }} 条结果，请输入更精确的关键词继续缩小范围。
        </div>
      </div>
    </section>

    <!-- Categories Grid (Responsive: 1 col mobile, 2 cols sm/md, 3 cols xl) -->
    <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-4">
      <article
        v-for="(cat, idx) in sortedCategories"
        :key="cat._clientId || cat.id || `new-${idx}`"
        class="flex flex-col justify-between p-4 sm:p-5 rounded-xl border transition-all backdrop-blur-md space-y-4"
        :class="[
          draggingCategoryKey === categoryKey(cat)
            ? 'opacity-50 ring-2 ring-blue-500/40 border-blue-500'
            : 'border-white/10 bg-slate-900/60 hover:border-white/20'
        ]"
        @dragover.prevent
        @drop="onCategoryDrop(cat)"
      >
        <!-- Category Top Block -->
        <div class="flex items-center justify-between gap-3 pb-3 border-b border-white/5 cursor-pointer" @click="cat.id && openCategory(cat)">
          <div class="flex items-center gap-2.5 min-w-0 flex-1">
            <button
              type="button"
              class="flex min-h-[44px] min-w-[44px] items-center justify-center rounded-lg border border-white/10 bg-slate-800/60 text-slate-300 hover:text-white cursor-grab active:cursor-grabbing select-none"
              title="拖拽排序"
              data-drag-handle
              draggable="true"
              @dragstart="onCategoryDragStart($event, cat)"
              @dragend="draggingCategoryKey = null"
              @click.stop
              @mousedown.stop
            >
              ☰
            </button>
            <div class="min-w-0">
              <span class="text-xs font-mono text-slate-500">#{{ idx + 1 }}</span>
              <h3 class="text-base font-semibold text-white tracking-tight truncate" :title="cat.name">
                {{ cat.name || '未命名类别' }}
              </h3>
            </div>
          </div>
          <span class="rounded-lg bg-blue-500/10 border border-blue-500/20 px-2.5 py-1 text-xs font-mono font-medium text-blue-400 whitespace-nowrap">
            {{ cat.rule_count || 0 }} 条
          </span>
        </div>

        <!-- Name Input -->
        <div class="space-y-1.5">
          <label class="text-xs font-mono text-slate-400">类别名</label>
          <input
            v-model="cat.name"
            class="w-full min-h-[44px] rounded-lg border border-white/10 bg-slate-950/60 px-3.5 py-2 text-xs text-white focus:border-blue-500 focus:outline-hidden font-mono"
            placeholder="类别名"
            @dragstart.stop.prevent
          />
        </div>

        <!-- Move Select Control -->
        <div class="flex items-center justify-between gap-2 text-xs font-mono text-slate-400 pt-1">
          <span>位置：第 {{ idx + 1 }} 位</span>
          <label class="inline-flex items-center gap-1.5">
            <span>移至</span>
            <select
              :value="idx"
              class="min-h-[44px] rounded-lg border border-white/10 bg-slate-950/60 px-2.5 py-1.5 text-xs text-slate-300 focus:border-blue-500 focus:outline-hidden font-mono cursor-pointer"
              @change="moveCategoryToIndex(cat, Number($event.target.value))"
            >
              <option v-for="(_, targetIdx) in sortedCategories" :key="targetIdx" :value="targetIdx">
                第 {{ targetIdx + 1 }} 位
              </option>
            </select>
          </label>
        </div>

        <!-- Action Row -->
        <div class="grid grid-cols-2 gap-2 pt-2 border-t border-white/5">
          <button
            class="min-h-[44px] px-3.5 py-2 rounded-lg border border-blue-500/40 bg-blue-600/20 text-xs font-medium text-blue-400 hover:bg-blue-600/30 transition-colors cursor-pointer text-center justify-center flex items-center"
            @click="openCategory(cat)"
          >
            进入编辑
          </button>
          <button
            class="min-h-[44px] px-3.5 py-2 rounded-lg border border-rose-500/40 bg-rose-600/20 text-xs font-medium text-rose-400 hover:bg-rose-600/30 transition-colors cursor-pointer text-center justify-center flex items-center"
            @click="removeCategoryRow(cat)"
          >
            移除
          </button>
        </div>
      </article>
    </div>

    <!-- Floating Save Button when Unsaved Changes exist -->
    <FabSave :visible="hasUnsavedChanges" :saving="saving" @save="saveAllCategories" />
  </section>
</template>

<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { onBeforeRouteLeave } from 'vue-router'
import { useAppStore } from '../stores/app'
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
import MetricCard from '../components/ui/MetricCard.vue'
import { setDragGhost, shouldAllowDragStart } from '../utils/drag'

const store = useAppStore()
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
  window.addEventListener('beforeunload', handleBeforeUnload)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleGlobalKeydown)
  window.removeEventListener('beforeunload', handleBeforeUnload)
})

onBeforeRouteLeave(async () => {
  if (!hasUnsavedChanges.value) return true
  const ok = await store.confirm({
    title: '未保存更改',
    message: '有未保存的分类更改，离开将丢失。确定离开吗？',
    confirmText: '丢弃并离开',
    danger: true,
  })
  return ok
})

function handleBeforeUnload(e) {
  if (hasUnsavedChanges.value) e.preventDefault()
}

watch(
  categories,
  () => {
    if (!suppressDirty.value) hasUnsavedChanges.value = true
  },
  { deep: true }
)

const sortedCategories = computed(() => [...categories.value].sort(compareCategoryOrder))
const totalRules = computed(() => allRules.value.length || sortedCategories.value.reduce((sum, cat) => sum + Number(cat.rule_count || 0), 0))
const saveStatus = computed(() => (saving.value ? '正在同步' : hasUnsavedChanges.value ? '有未保存更改' : '已同步'))

const ruleSearchResults = computed(() => {
  const q = debouncedRuleSearch.value.trim().toLowerCase()
  if (!q) return []
  return allRules.value.filter((rule) => (searchIndex.value.get(rule.id) || ruleSearchText(rule)).includes(q))
})

const searchIndex = computed(() => {
  const map = new Map()
  for (const rule of allRules.value) map.set(rule.id, ruleSearchText(rule))
  return map
})

const debouncedRuleSearch = ref('')
let _searchTimer = null
watch(
  ruleSearch,
  (val) => {
    clearTimeout(_searchTimer)
    _searchTimer = setTimeout(() => {
      debouncedRuleSearch.value = val
    }, 250)
  },
  { immediate: true }
)

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
      message: '刷新会丢弃当前未保存更改，确定刷新吗？',
      confirmText: '刷新',
      danger: true,
    })
    if (!ok) return
  }
  load()
}

function createCategoryRow() {
  categories.value.push({
    id: null,
    _clientId: `new-cat-${clientIdSeq++}`,
    name: `新类别${categories.value.length + 1}`,
    sort_order: categories.value.length * 10,
    rule_count: 0,
  })
}

async function saveAllCategories() {
  error.value = ''
  saving.value = true
  const sorted = sortedCategories.value
  sorted.forEach((cat, index) => {
    cat.sort_order = index * 10
  })

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

function onCategoryDragStart(event, cat) {
  if (!shouldAllowDragStart(event, { requireHandle: true })) {
    event.preventDefault()
    draggingCategoryKey.value = null
    return
  }
  draggingCategoryKey.value = categoryKey(cat)
  setDragGhost(event, cat.name || '分类排序')
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
  sorted.forEach((item, i) => {
    item.sort_order = i * 10
  })
  categories.value = sorted
  hasUnsavedChanges.value = true
}

function normalizeSortOrder() {
  categories.value.sort(compareCategoryOrder).forEach((cat, index) => {
    cat.sort_order = index * 10
  })
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
