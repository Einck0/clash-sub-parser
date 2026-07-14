<template>
  <div class="modal-backdrop" @click.self="close">
    <div class="modal node-group-modal" role="dialog" aria-modal="true">
      <div class="form-header">
        <div>
          <p class="eyebrow">{{ group?.id ? 'Edit Group' : 'New Group' }}</p>
          <h3 style="margin:0">{{ group?.id ? '编辑节点组' : '新增节点组' }}</h3>
          <p class="section-hint">正则改完后直接点底部「保存」即可，不必再单独点“保存正则”。</p>
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
            <strong>正则来源</strong>
            <p class="section-hint">按节点最终名动态匹配。编辑后保存整组即可生效。</p>
          </div>
          <span class="muted">匹配 {{ regexMatches.length }}</span>
        </div>
        <textarea
          v-model="regexText"
          placeholder="香港\n美国|US\n^(?!.*(官网|套餐)).*$"
          @input="onRegexInput"
        ></textarea>
        <div class="row" style="margin-top:8px;gap:8px;flex-wrap:wrap">
          <button @click="previewRegexMatches" :disabled="!!regexError">预览匹配</button>
          <button @click="freezeRegexMatchesAsEntries" :disabled="!regexMatches.length">冻结为静态节点</button>
          <span v-if="regexError" class="form-alert form-alert-error" style="margin:0;padding:6px 8px">{{ regexError }}</span>
        </div>
        <div v-if="regexMatches.length" class="mono final-preview" style="margin-top:8px">
          {{ regexMatches.slice(0, 80).join(' | ') }}
          <span v-if="regexMatches.length > 80"> … +{{ regexMatches.length - 80 }}</span>
        </div>
      </div>

      <div class="selector-section">
        <div class="row space">
          <div>
            <strong>兜底节点</strong>
            <p class="section-hint">开启后在组末尾追加 REJECT，避免空组误放行。</p>
          </div>
        </div>
        <label class="settings-toggle" style="margin-top:8px">
          <input type="checkbox" v-model="form.add_fallback" />
          <span>
            <strong>添加兜底 REJECT</strong>
            <small>默认开启</small>
          </span>
        </label>
      </div>

      <div class="selector-section">
        <div class="row space">
          <div>
            <strong>添加来源条目</strong>
            <p class="section-hint">节点 / 节点组引用 / 节点组节点，可统一排序。</p>
          </div>
        </div>

        <div class="node-search-row" style="margin-top:8px">
          <select v-model="selectedNodeName">
            <option value="">选择节点</option>
            <option v-for="name in selectableNodeNames" :key="name" :value="name">{{ name }}</option>
          </select>
          <button @click="addNode" :disabled="!selectedNodeName">加入节点</button>
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
          </div>
        </div>
      </div>

      <div class="selector-section">
        <div class="row space">
          <strong>统一排序条目</strong>
          <span class="muted">{{ form.include_entries.length }} 项</span>
        </div>
        <div v-if="!form.include_entries.length" class="empty-mini">暂无静态条目，可只靠正则动态匹配。</div>
        <div v-else class="node-select-list">
          <div
            v-for="(entry, idx) in form.include_entries"
            :key="`${entry.type}-${entry.value}-${idx}`"
            class="node-select-row"
            draggable="true"
            @dragstart="onDragStart(idx)"
            @dragover.prevent
            @drop="onDrop(idx)"
          >
            <div class="node-select-name mono">
              <strong>{{ idx + 1 }}. {{ formatEntry(entry) }}</strong>
              <span>{{ typeLabel(entry.type) }}</span>
            </div>
            <div class="node-select-actions">
              <button :disabled="idx === 0" @click="moveEntry(idx, -1)">上</button>
              <button :disabled="idx === form.include_entries.length - 1" @click="moveEntry(idx, 1)">下</button>
              <button class="danger" @click="removeEntry(idx)">删</button>
            </div>
          </div>
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
        <button class="primary" @click="save" :disabled="saving || !!regexError || !form.name.trim()">
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
const regexText = ref('')
const regexMatches = ref([])
const regexError = ref('')
const draggingIndex = ref(-1)
const saving = ref(false)
const error = ref('')
const form = ref(defaultForm())

watch(
  () => props.group,
  async (value) => {
    error.value = ''
    await loadSources()
    if (value) {
      form.value = {
        id: value.id,
        name: value.name || '',
        kind: value.kind || 'manual',
        group_type: value.group_type || 'select',
        sort_order: value.sort_order || 0,
        regex_rules: [...(value.regex_rules || [])],
        include_entries: normalizeEntries(value.include_entries || buildEntriesFallback(value)),
        add_fallback: value.add_fallback !== false,
        exclude_nodes: [...(value.exclude_nodes || [])],
        url_test_config: value.url_test_config || {},
        load_balance_config: value.load_balance_config || {},
        fallback_config: value.fallback_config || {},
      }
      regexText.value = form.value.regex_rules.join('\n')
      validateRegexText()
      regexMatches.value = collectRegexMatches(form.value.regex_rules)
    } else {
      form.value = defaultForm()
      regexText.value = ''
      regexMatches.value = []
      regexError.value = ''
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

const selectableGroups = computed(() => allGroups.value.filter((item) => item.id !== form.value.id))
const selectableNodeNames = computed(() =>
  allNodes.value.map((node) => String(node.name || '').trim()).filter(Boolean)
)

function onRegexInput() {
  validateRegexText()
}

function validateRegexText() {
  regexError.value = ''
  const rules = parseRegexText()
  for (const [idx, rule] of rules.entries()) {
    try {
      new RegExp(rule)
    } catch (err) {
      regexError.value = `第 ${idx + 1} 条正则无效：${err.message}`
      return
    }
  }
}

function parseRegexText() {
  return regexText.value
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
}

function syncRegexIntoForm() {
  const rules = parseRegexText()
  form.value.regex_rules = rules
  form.value.kind = rules.length ? 'regex' : 'manual'
  return rules
}

function collectRegexMatches(rules) {
  const matches = []
  for (const node of allNodes.value) {
    const name = String(node.name || '')
    if (!name) continue
    for (const rule of rules) {
      try {
        if (new RegExp(rule, 'i').test(name)) {
          matches.push(name)
          break
        }
      } catch (_) {
        continue
      }
    }
  }
  return uniq(matches)
}

function previewRegexMatches() {
  validateRegexText()
  if (regexError.value) return
  const rules = parseRegexText()
  regexMatches.value = collectRegexMatches(rules)
}

function freezeRegexMatchesAsEntries() {
  previewRegexMatches()
  for (const name of regexMatches.value) {
    pushEntry({ type: 'node', value: name })
  }
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
  if (type === 'node') return '节点'
  if (type === 'group') return '节点组'
  if (type === 'group_nodes') return '节点组节点'
  return type
}

function formatEntry(entry) {
  if (entry.type === 'node') return `${entry.value}`
  if (entry.type === 'group') return `${groupNameById(Number(entry.value))}`
  if (entry.type === 'group_nodes') return `${groupNameById(Number(entry.value))}(节点)`
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
      regex_rules: uniq(parsed.regex_rules || []),
      include_entries: normalizeEntries(parsed.include_entries || []),
      exclude_nodes: uniq(parsed.exclude_nodes || []),
    }
    regexText.value = form.value.regex_rules.join('\n')
    validateRegexText()
    regexMatches.value = collectRegexMatches(form.value.regex_rules)
  } catch (err) {
    error.value = `Raw JSON 格式错误: ${err.message}`
  }
}

async function save() {
  if (saving.value) return
  validateRegexText()
  if (regexError.value) {
    error.value = regexError.value
    return
  }
  const name = String(form.value.name || '').trim()
  if (!name) {
    error.value = '名称不能为空'
    return
  }

  // Critical: always sync textarea regex into payload on save.
  const rules = syncRegexIntoForm()
  const payload = {
    name,
    kind: rules.length ? 'regex' : 'manual',
    group_type: form.value.group_type || 'select',
    sort_order: form.value.sort_order || 0,
    regex_rules: rules,
    include_entries: normalizeEntries(form.value.include_entries || []),
    add_fallback: form.value.add_fallback !== false,
    exclude_nodes: uniq(form.value.exclude_nodes || []),
    url_test_config: form.value.url_test_config || {},
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
    store.success(form.value.id ? '节点组已保存' : '节点组已创建')
    emit('saved')
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
    regex_rules: [],
    include_entries: [],
    add_fallback: true,
    exclude_nodes: [],
    url_test_config: {},
    load_balance_config: {},
    fallback_config: {},
  }
}

function normalizeEntries(entries) {
  const allowed = new Set(['node', 'group', 'group_nodes'])
  const out = []
  for (const item of entries) {
    const type = String(item?.type || '').trim()
    if (!allowed.has(type)) continue
    let value = item?.value
    if (type === 'node') {
      value = String(value || '').trim()
      if (!value) continue
    } else {
      value = Number(value)
      if (!Number.isInteger(value)) continue
    }
    out.push({ type, value })
  }
  return uniqBy(out, (item) => `${item.type}:${item.value}`)
}

function buildEntriesFallback(value) {
  const entries = []
  for (const name of value.include_nodes || []) entries.push({ type: 'node', value: name })
  for (const id of value.include_group_ids || []) entries.push({ type: 'group', value: id })
  for (const id of value.include_group_nodes_ids || []) entries.push({ type: 'group_nodes', value: id })
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
