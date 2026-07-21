<template>
  <div class="modal-backdrop" @click.self="close">
    <div class="modal node-group-modal" role="dialog" aria-modal="true">
      <div class="form-header">
        <div>
          <p class="eyebrow">{{ group?.id ? 'Edit Group' : 'New Group' }}</p>
          <h3 style="margin:0">{{ group?.id ? '编辑节点组' : '新增节点组' }}</h3>
          <p class="section-hint">
            正则是<strong>虚拟筛选</strong>：加入条目列表后会按最终节点名动态匹配，不会冻结成静态节点。
          </p>
        </div>
        <button @click="close">关闭</button>
      </div>

      <div v-if="error" class="form-alert form-alert-error">{{ error }}</div>

      <div class="grid-2" style="margin-top:10px">
        <label>
          <div class="muted">名称</div>
          <input v-model="form.name" placeholder="例如：自动选择" />
        </label>
        <label>
          <div class="muted">类型</div>
          <select v-model="form.group_type">
            <option value="select">select</option>
            <option value="url-test">url-test</option>
            <option value="fallback">fallback</option>
            <option value="load-balance">load-balance</option>
          </select>
        </label>
      </div>

      <div class="selector-section">
        <div class="row space">
          <div>
            <strong>兜底节点</strong>
            <p class="section-hint">仅当策略组最终没有任何节点时才追加 PASS；有节点时不会加。</p>
          </div>
        </div>
        <label class="settings-toggle" style="margin-top:8px">
          <input type="checkbox" v-model="form.add_fallback" />
          <span>
            <strong>空组时追加 PASS</strong>
            <small>默认关闭</small>
          </span>
        </label>
      </div>

      <div
        class="selector-section"
        v-if="['url-test', 'fallback', 'load-balance'].includes(form.group_type)"
      >
        <div class="row space">
          <div>
            <strong>{{ form.group_type }} 参数</strong>
            <p class="section-hint">导出到 Clash 时写入对应字段；空值用默认。</p>
          </div>
        </div>
        <div class="grid-2" style="margin-top:8px;gap:8px">
          <label>
            <div class="muted">url</div>
            <input v-model="urlTestUrl" placeholder="https://www.gstatic.com/generate_204" />
          </label>
          <label>
            <div class="muted">interval (秒)</div>
            <input v-model.number="urlTestInterval" type="number" min="1" placeholder="300" />
          </label>
          <label>
            <div class="muted">tolerance (ms)</div>
            <input v-model.number="urlTestTolerance" type="number" min="0" placeholder="50" />
          </label>
        </div>
      </div>

      <div class="selector-section">
        <div class="row space">
          <div>
            <strong>添加来源条目</strong>
            <p class="section-hint">
              可添加：静态节点、节点组引用、节点组节点、正则筛选（虚拟）。
              正则只记录规则本身，输出时动态展开匹配到的节点。
            </p>
          </div>
        </div>

        <div class="node-search-row" style="margin-top:8px">
          <select v-model="selectedNodeName">
            <option value="">选择节点</option>
            <option v-for="name in selectableNodeNames" :key="name" :value="name">{{ name }}</option>
          </select>
          <button @click="addNode" :disabled="!selectedNodeName">加入静态节点</button>
        </div>
        <div class="row" style="gap:8px;flex-wrap:wrap;margin-bottom:8px">
          <button @click="addBuiltin('DIRECT')">DIRECT</button>
          <button @click="addBuiltin('PASS')">PASS</button>
          <button @click="addBuiltin('REJECT')">REJECT</button>
        </div>

        <div class="node-search-row">
          <select v-model.number="selectedGroupId">
            <option :value="null">选择节点组</option>
            <option v-for="g in selectableGroups" :key="g.id" :value="g.id">{{ g.name }}</option>
          </select>
          <div class="row" style="gap:6px">
            <button @click="addGroupRef" :disabled="!selectedGroupId">添加组引用</button>
            <button @click="addGroupNodes" :disabled="!selectedGroupId">添加组节点</button>
            <button @click="addExcludeGroupNodes" :disabled="!selectedGroupId">减去组节点</button>
          </div>
        </div>

        <div style="margin-top:10px">
          <div class="muted">添加正则筛选（虚拟）</div>
          <div class="grid-2" style="margin-top:6px;gap:8px">
            <input v-model="regexDraftName" placeholder="名称（可选，空则自动 正则1/正则2）" />
            <input
              v-model="regexDraft"
              placeholder="正则，例如：香港  或  ^(?!.*(官网|套餐)).*$"
              @keyup.enter="addRegexEntry"
            />
          </div>
          <div class="node-search-row" style="margin-top:6px">
            <button @click="addRegexEntry" :disabled="!regexDraft.trim() || !!regexDraftError">加入正则</button>
          </div>
          <div v-if="regexDraftError" class="form-alert form-alert-error" style="margin-top:6px">
            {{ regexDraftError }}
          </div>
          <div class="row" style="margin-top:6px;gap:8px;flex-wrap:wrap">
            <button @click="previewDraftRegex" :disabled="!regexDraft.trim() || !!regexDraftError">
              预览该正则匹配
            </button>
            <span class="muted">匹配 {{ draftMatches.length }}</span>
          </div>
          <div v-if="draftMatches.length" class="mono final-preview" style="margin-top:8px">
            {{ draftMatches.slice(0, 60).join(' | ') }}
            <span v-if="draftMatches.length > 60"> … +{{ draftMatches.length - 60 }}</span>
          </div>
        </div>
      </div>

      <div class="selector-section">
        <div class="row space">
          <div>
            <strong>统一排序条目</strong>
            <p class="section-hint">拖拽/上下调整顺序。正则项显示为虚拟筛选，不是冻结节点列表。</p>
          </div>
          <span class="muted">{{ form.include_entries.length }} 项</span>
        </div>
        <div v-if="!form.include_entries.length" class="empty-mini">还没有条目。可先加正则筛选或静态节点。</div>
        <div v-else class="node-select-list">
          <div
            v-for="(entry, idx) in form.include_entries"
            :key="`${entry.type}-${entry.value}-${idx}`"
            class="node-select-row"
            :class="{ 'regex-entry-row': entry.type === 'regex' }"
            draggable="true"
            @dragstart="onDragStart(idx)"
            @dragover.prevent
            @drop="onDrop(idx)"
          >
            <div class="node-select-name mono" style="width:100%">
              <template v-if="entry.type === 'regex' && editingRegexIndex === idx">
                <div class="regex-edit-box">
                  <input
                    v-model="editingRegexName"
                    class="regex-edit-input"
                    placeholder="名称（可选）"
                    style="margin-bottom:6px"
                  />
                  <input
                    v-model="editingRegexValue"
                    class="regex-edit-input"
                    placeholder="输入正则，例如 香港 或 ^(?!.*(官网|套餐)).*$"
                    @keyup.enter="saveRegexEdit(idx)"
                    @keyup.escape="cancelRegexEdit"
                  />
                  <div class="row" style="gap:6px;margin-top:6px;flex-wrap:wrap">
                    <button class="primary" @click="saveRegexEdit(idx)" :disabled="!!editingRegexError || !editingRegexValue.trim()">
                      确定
                    </button>
                    <button @click="previewEditingRegex" :disabled="!!editingRegexError || !editingRegexValue.trim()">预览</button>
                    <button @click="cancelRegexEdit">取消</button>
                    <span v-if="editingRegexError" class="form-alert form-alert-error" style="margin:0;padding:4px 8px">
                      {{ editingRegexError }}
                    </span>
                    <span v-else class="muted">动态匹配 {{ countRegexMatches(editingRegexValue) }} 个</span>
                  </div>
                </div>
              </template>
              <template v-else>
                <strong>{{ idx + 1 }}. {{ formatEntryTitle(entry, idx) }}</strong>
                <span>
                  {{ typeLabel(entry.type) }}
                  <template v-if="entry.type === 'regex'">
                    · {{ truncateText(String(entry.value || ''), 48) }}
                    · 匹配 {{ countRegexMatches(entry.value) }} 个
                  </template>
                </span>
              </template>
            </div>
            <div class="node-select-actions" v-if="!(entry.type === 'regex' && editingRegexIndex === idx)">
              <button v-if="entry.type === 'regex'" class="primary" @click="startRegexEdit(idx)">编辑</button>
              <button v-if="entry.type === 'regex'" @click="previewEntryRegex(entry.value)">预览</button>
              <button :disabled="idx === 0" @click="moveEntry(idx, -1)">上</button>
              <button :disabled="idx === form.include_entries.length - 1" @click="moveEntry(idx, 1)">下</button>
              <button class="danger" @click="removeEntry(idx)">删</button>
            </div>
          </div>
        </div>
      </div>

      <div class="selector-section" v-if="previewMatches.length">
        <div class="row space">
          <strong>正则预览结果</strong>
          <span class="muted">{{ previewMatches.length }} 个</span>
        </div>
        <div class="mono final-preview">
          {{ previewMatches.slice(0, 100).join(' | ') }}
          <span v-if="previewMatches.length > 100"> … +{{ previewMatches.length - 100 }}</span>
        </div>
      </div>

      <div class="selector-section" v-if="showRaw">
        <div class="row space">
          <strong>Raw JSON</strong>
          <button @click="syncFromRaw">应用 Raw</button>
        </div>
        <textarea v-model="rawJson"></textarea>
      </div>

      <div class="form-footer">
        <button class="primary" @click="save" :disabled="saving || !form.name.trim()">
          {{ saving ? '保存中...' : '保存' }}
        </button>
        <button @click="showRaw = !showRaw">{{ showRaw ? '隐藏 Raw' : '显示 Raw' }}</button>
        <button @click="close">取消</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import {
  createNodeGroup,
  getAllSubscriptionNodes,
  getApiErrorMessage,
  getNodeGroups,
  updateNodeGroup,
} from '../api'
import { useAppStore } from '../stores/app'

const props = defineProps({ group: { type: Object, default: null } })
const emit = defineEmits(['saved', 'close'])
const store = useAppStore()

const allGroups = ref([])
const allNodes = ref([])
const selectedGroupId = ref(null)
const selectedNodeName = ref('')
const showRaw = ref(false)
const rawJson = ref('')
const regexDraft = ref('')
const regexDraftName = ref('')
const regexDraftError = ref('')
const draftMatches = ref([])
const previewMatches = ref([])
const editingRegexIndex = ref(-1)
const editingRegexValue = ref('')
const editingRegexName = ref('')
const editingRegexError = ref('')
const draggingIndex = ref(-1)
const saving = ref(false)
const error = ref('')
const form = ref(defaultForm())

const urlTestUrl = computed({
  get: () => form.value.url_test_config?.url || '',
  set: (v) => {
    form.value.url_test_config = { ...(form.value.url_test_config || {}), url: v }
  },
})
const urlTestInterval = computed({
  get: () => form.value.url_test_config?.interval ?? '',
  set: (v) => {
    form.value.url_test_config = { ...(form.value.url_test_config || {}), interval: v }
  },
})
const urlTestTolerance = computed({
  get: () => form.value.url_test_config?.tolerance ?? '',
  set: (v) => {
    form.value.url_test_config = { ...(form.value.url_test_config || {}), tolerance: v }
  },
})

watch(
  () => props.group,
  async (value) => {
    error.value = ''
    draftMatches.value = []
    previewMatches.value = []
    regexDraft.value = ''
    regexDraftName.value = ''
    regexDraftError.value = ''
    cancelRegexEdit()
    await loadSources()
    if (value) {
      const entries = normalizeEntries(value.include_entries || buildEntriesFallback(value))
      form.value = {
        id: value.id,
        name: value.name || '',
        kind: value.kind || 'manual',
        group_type: value.group_type || 'select',
        sort_order: value.sort_order || 0,
        include_entries: entries,
        add_fallback: value.add_fallback === true,
        exclude_nodes: [...(value.exclude_nodes || [])],
        url_test_config: value.url_test_config || {},
        load_balance_config: value.load_balance_config || {},
        fallback_config: value.fallback_config || {},
      }
    } else {
      form.value = defaultForm()
    }
    rawJson.value = JSON.stringify(form.value, null, 2)
  },
  { immediate: true }
)

watch(
  form,
  (value) => {
    if (showRaw.value) rawJson.value = JSON.stringify(value, null, 2)
  },
  { deep: true }
)

watch(regexDraft, () => {
  regexDraftError.value = validateRegex(regexDraft.value.trim())
  draftMatches.value = []
})

watch(editingRegexValue, () => {
  editingRegexError.value = validateRegex(editingRegexValue.value.trim())
})

const selectableGroups = computed(() => allGroups.value.filter((item) => item.id !== form.value.id))
const selectableNodeNames = computed(() =>
  allNodes.value.map((node) => String(node.name || '').trim()).filter(Boolean),
)

function validateRegex(rule) {
  if (!rule) return ''
  try {
    new RegExp(rule)
    return ''
  } catch (err) {
    return `正则无效：${err.message}`
  }
}

function collectRegexMatches(rule) {
  const text = String(rule || '').trim()
  if (!text) return []
  let pattern
  try {
    pattern = new RegExp(text, 'i')
  } catch (_) {
    return []
  }
  const matches = []
  for (const node of allNodes.value) {
    const name = String(node.name || '')
    if (name && pattern.test(name)) matches.push(name)
  }
  return uniq(matches)
}

function countRegexMatches(rule) {
  return collectRegexMatches(rule).length
}

function previewDraftRegex() {
  regexDraftError.value = validateRegex(regexDraft.value.trim())
  if (regexDraftError.value) return
  draftMatches.value = collectRegexMatches(regexDraft.value.trim())
  previewMatches.value = draftMatches.value
}

function previewEntryRegex(rule) {
  previewMatches.value = collectRegexMatches(rule)
}

function startRegexEdit(index) {
  const entry = form.value.include_entries[index]
  if (!entry || entry.type !== 'regex') return
  editingRegexIndex.value = index
  editingRegexValue.value = String(entry.value || '')
  editingRegexName.value = String(entry.name || entry.label || '')
  editingRegexError.value = validateRegex(editingRegexValue.value.trim())
}

function cancelRegexEdit() {
  editingRegexIndex.value = -1
  editingRegexValue.value = ''
  editingRegexName.value = ''
  editingRegexError.value = ''
}

function previewEditingRegex() {
  editingRegexError.value = validateRegex(editingRegexValue.value.trim())
  if (editingRegexError.value) return
  previewMatches.value = collectRegexMatches(editingRegexValue.value.trim())
}

function saveRegexEdit(index) {
  const next = editingRegexValue.value.trim()
  editingRegexError.value = validateRegex(next)
  if (!next || editingRegexError.value) return

  const exists = form.value.include_entries.some(
    (item, i) => i !== index && item.type === 'regex' && String(item.value) === next
  )
  if (exists) {
    editingRegexError.value = '已存在相同正则条目'
    return
  }

  const copy = [...form.value.include_entries]
  const label = String(editingRegexName.value || '').trim()
  const item = { type: 'regex', value: next }
  if (label) item.name = label
  copy[index] = item
  form.value.include_entries = copy
  previewMatches.value = collectRegexMatches(next)
  cancelRegexEdit()
}

function addRegexEntry() {
  const rule = regexDraft.value.trim()
  regexDraftError.value = validateRegex(rule)
  if (!rule || regexDraftError.value) return
  const label = String(regexDraftName.value || '').trim() || nextRegexLabel()
  pushEntry({ type: 'regex', value: rule, name: label })
  regexDraft.value = ''
  regexDraftName.value = ''
  draftMatches.value = []
}

function nextRegexLabel() {
  const used = new Set(
    (form.value.include_entries || [])
      .filter((item) => item.type === 'regex')
      .map((item) => String(item.name || '').trim())
      .filter(Boolean),
  )
  let i = 1
  while (used.has(`正则${i}`)) i += 1
  return `正则${i}`
}

function truncateText(text, max = 40) {
  const value = String(text || '')
  if (value.length <= max) return value
  return `${value.slice(0, Math.max(1, max - 1))}…`
}

function formatEntryTitle(entry, idx = 0) {
  if (entry.type === 'regex') {
    return String(entry.name || entry.label || '').trim() || `正则${regexIndex(entry, idx)}`
  }
  return formatEntry(entry)
}

function regexIndex(entry, fallbackIdx = 0) {
  let n = 0
  for (const item of form.value.include_entries || []) {
    if (item.type !== 'regex') continue
    n += 1
    if (item === entry) return n
  }
  return fallbackIdx + 1
}

function addNode() {
  if (!selectedNodeName.value) return
  pushEntry({ type: 'node', value: selectedNodeName.value })
}

function addBuiltin(name) {
  pushEntry({ type: 'node', value: name })
}

function addGroupRef() {
  if (!selectedGroupId.value) return
  pushEntry({ type: 'group', value: Number(selectedGroupId.value) })
}

function addGroupNodes() {
  if (!selectedGroupId.value) return
  pushEntry({ type: 'group_nodes', value: Number(selectedGroupId.value) })
}

function addExcludeGroupNodes() {
  if (!selectedGroupId.value) return
  const gid = Number(selectedGroupId.value)
  if (gid === form.value.id) {
    error.value = '不能减去自己'
    return
  }
  const entry = { type: 'exclude_group_nodes', value: gid }
  const exists = form.value.include_entries.some(
    (item) => item.type === entry.type && String(item.value) === String(entry.value)
  )
  if (!exists) {
    form.value.include_entries.push(entry)
  }
}

function pushEntry(entry) {
  const exists = form.value.include_entries.some(
    (item) => item.type === entry.type && String(item.value) === String(entry.value)
  )
  if (exists) return
  form.value.include_entries.push(entry)
}

function moveEntry(index, direction) {
  const to = index + direction
  if (to < 0 || to >= form.value.include_entries.length) return
  const copy = [...form.value.include_entries]
  ;[copy[index], copy[to]] = [copy[to], copy[index]]
  form.value.include_entries = copy
}

function removeEntry(index) {
  form.value.include_entries.splice(index, 1)
}

function onDragStart(index) {
  draggingIndex.value = index
}

function onDrop(targetIndex) {
  if (draggingIndex.value < 0 || draggingIndex.value === targetIndex) return
  const copy = [...form.value.include_entries]
  const [moved] = copy.splice(draggingIndex.value, 1)
  copy.splice(targetIndex, 0, moved)
  form.value.include_entries = copy
  draggingIndex.value = -1
}

function typeLabel(type) {
  if (type === 'node') return '静态节点'
  if (type === 'group') return '节点组引用'
  if (type === 'group_nodes') return '节点组节点'
  if (type === 'exclude_group_nodes') return '减去组节点(动态)'
  if (type === 'regex') return '正则筛选(虚拟)'
  return type
}

function formatEntry(entry) {
  if (entry.type === 'node') return `${entry.value}`
  if (entry.type === 'group') return `${groupNameById(Number(entry.value))}`
  if (entry.type === 'group_nodes') return `${groupNameById(Number(entry.value))}(节点)`
  if (entry.type === 'exclude_group_nodes') return `减${groupNameById(Number(entry.value))}节点`
  if (entry.type === 'regex') {
    const label = String(entry.name || entry.label || '').trim() || '正则'
    return `${label} · ${truncateText(String(entry.value || ''), 48)}`
  }
  return JSON.stringify(entry)
}

function groupNameById(id) {
  return allGroups.value.find((item) => item.id === id)?.name || `#${id}`
}

function syncFromRaw() {
  try {
    const parsed = JSON.parse(rawJson.value)
    form.value = {
      ...defaultForm(),
      ...parsed,
      include_entries: normalizeEntries(parsed.include_entries || buildEntriesFallback(parsed)),
      exclude_nodes: uniq(parsed.exclude_nodes || []),
    }
  } catch (err) {
    error.value = `Raw JSON 格式错误: ${err.message}`
  }
}


function detectReferenceCycle(entries) {
  // Client-side soft check. Backend still enforces the same graph.
  const selfId = form.value.id == null ? null : Number(form.value.id)
  const graph = new Map()
  for (const group of allGroups.value) {
    const edges = []
    const seen = new Set()
    const push = (raw) => {
      const id = Number(raw)
      if (!Number.isInteger(id) || seen.has(id)) return
      seen.add(id)
      edges.push(id)
    }
    const sourceEntries =
      selfId != null && group.id === selfId
        ? entries
        : normalizeEntries(group.include_entries || buildEntriesFallback(group))
    for (const entry of sourceEntries) {
      if (entry.type === 'group' || entry.type === 'group_nodes' || entry.type === 'exclude_group_nodes') push(entry.value)
    }
    graph.set(group.id, edges)
  }
  // New group not yet in allGroups
  if (selfId == null) {
    const edges = []
    const seen = new Set()
    const push = (raw) => {
      const id = Number(raw)
      if (!Number.isInteger(id) || seen.has(id)) return
      seen.add(id)
      edges.push(id)
    }
    for (const entry of entries) {
      if (
        entry.type === 'group'
        || entry.type === 'group_nodes'
        || entry.type === 'exclude_group_nodes'
      ) {
        push(entry.value)
      }
    }
    // use temporary id 0 for draft
    graph.set(0, edges)
  }

  const visiting = new Set()
  const visited = new Set()
  const path = []
  const label = (id) => {
    if (id === 0) return form.value.name || '当前策略组'
    return groupNameById(id)
  }
  const dfs = (node) => {
    if (visited.has(node)) return null
    if (visiting.has(node)) {
      const start = path.indexOf(node)
      const cycle = path.slice(start >= 0 ? start : 0).concat(node)
      return `策略组引用存在循环：${cycle.map(label).join(' → ')}。加/减策略组引用都不能形成环。`
    }
    visiting.add(node)
    path.push(node)
    for (const child of graph.get(node) || []) {
      const hit = dfs(child)
      if (hit) return hit
    }
    path.pop()
    visiting.delete(node)
    visited.add(node)
    return null
  }
  for (const node of graph.keys()) {
    const hit = dfs(node)
    if (hit) return hit
  }
  return ''
}

async function save() {
  if (saving.value) return
  const name = String(form.value.name || '').trim()
  if (!name) {
    error.value = '名称不能为空'
    return
  }

  const entries = normalizeEntries(form.value.include_entries || [])
  // Validate all regex entries before save.
  for (const [idx, entry] of entries.entries()) {
    if (entry.type !== 'regex') continue
    const err = validateRegex(String(entry.value || ''))
    if (err) {
      error.value = `第 ${idx + 1} 条正则无效：${err}`
      return
    }
  }

  // Same graph check as backend: include edges + exclude edges.
  const cycleHint = detectReferenceCycle(entries)
  if (cycleHint) {
    error.value = cycleHint
    store.error(cycleHint)
    return
  }

  const urlCfg = { ...(form.value.url_test_config || {}) }
  if (urlTestUrl.value) urlCfg.url = String(urlTestUrl.value).trim()
  if (urlTestInterval.value !== '' && urlTestInterval.value != null) {
    urlCfg.interval = Number(urlTestInterval.value) || 300
  }
  if (urlTestTolerance.value !== '' && urlTestTolerance.value != null) {
    urlCfg.tolerance = Number(urlTestTolerance.value) || 0
  }
  const payload = {
    name,
    group_type: form.value.group_type || 'select',
    sort_order: form.value.sort_order || 0,
    // include_entries is source of truth; backend derives regex_rules/kind.
    include_entries: entries,
    add_fallback: form.value.add_fallback === true,
    exclude_nodes: uniq(form.value.exclude_nodes || []),
    url_test_config: urlCfg,
    load_balance_config: form.value.load_balance_config || {},
    fallback_config: form.value.fallback_config || {},
  }

  saving.value = true
  error.value = ''
  try {
    if (form.value.id) {
      await updateNodeGroup(form.value.id, payload)
    } else {
      await createNodeGroup(payload)
    }
    // Parent page owns the success toast to avoid double notifications.
    emit('saved', { created: !form.value.id, name })
    emit('close')
  } catch (err) {
    error.value = getApiErrorMessage(err, '保存节点组失败')
    store.error(error.value)
  } finally {
    saving.value = false
  }
}

async function loadSources() {
  const [groupsRes, nodesRes] = await Promise.all([getNodeGroups(), getAllSubscriptionNodes()])
  allGroups.value = groupsRes.data
  allNodes.value = nodesRes.data
}

function close() {
  emit('close')
}

function defaultForm() {
  return {
    id: null,
    name: '',
    kind: 'manual',
    group_type: 'select',
    sort_order: 0,
    include_entries: [],
    add_fallback: false,
    exclude_nodes: [],
    url_test_config: {},
    load_balance_config: {},
    fallback_config: {},
  }
}

function normalizeEntries(entries) {
  const allowed = new Set(['node', 'group', 'group_nodes', 'exclude_group_nodes', 'regex'])
  const out = []
  for (const item of entries) {
    const type = String(item?.type || '').trim()
    if (!allowed.has(type)) continue
    if (type === 'node') {
      const value = String(item?.value || '').trim()
      if (!value) continue
      out.push({ type, value })
      continue
    }
    if (type === 'regex') {
      const value = String(item?.value || '').trim()
      if (!value) continue
      const name = String(item?.name || item?.label || '').trim()
      const row = { type, value }
      if (name) row.name = name
      out.push(row)
      continue
    }
    const value = Number(item?.value)
    if (!Number.isInteger(value)) continue
    out.push({ type, value })
  }
  return uniqBy(out, (item) => `${item.type}:${item.value}`)
}

function buildEntriesFallback(value) {
  // Data already migrated to include_entries. Keep a tiny fallback for empty groups.
  const entries = []
  for (const name of value.include_nodes || []) entries.push({ type: 'node', value: name })
  for (const id of value.include_group_ids || []) entries.push({ type: 'group', value: id })
  for (const id of value.include_group_nodes_ids || []) entries.push({ type: 'group_nodes', value: id })
  for (const id of value.exclude_group_ids || []) entries.push({ type: 'exclude_group_nodes', value: id })
  return entries
}

function uniq(items) {
  return [...new Set(items)]
}


function uniqBy(items, getKey) {
  const seen = new Set()
  const out = []
  for (const item of items) {
    const key = getKey(item)
    if (seen.has(key)) continue
    seen.add(key)
    out.push(item)
  }
  return out
}
</script>
