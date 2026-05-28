<template>
  <div class="modal-backdrop" @click.self="$emit('close')">
    <div class="modal">
      <div class="row space">
        <h3 style="margin:0">{{ group?.id ? '编辑节点组' : '新增节点组' }}</h3>
        <button @click="$emit('close')">关闭</button>
      </div>

      <div class="grid-2" style="margin-top:10px">
        <label>
          <div class="muted">名称</div>
          <input v-model="form.name" />
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

      <div class="card" style="margin-top:10px">
        <div class="row space">
          <strong>正则来源</strong>
          <span class="muted">默认作为动态匹配规则，不再自动写入静态节点</span>
        </div>
        <textarea v-model="regexText" placeholder="香港\n美国|US"></textarea>
        <div class="row" style="margin-top:6px">
          <button @click="previewRegexMatches">预览正则匹配</button>
          <button @click="applyRegexRules">保存正则规则</button>
          <button @click="freezeRegexMatchesAsEntries" :disabled="!regexMatches.length">将匹配结果冻结为静态节点</button>
          <span class="muted">当前匹配 {{ regexMatches.length }} 项</span>
        </div>
        <div class="mono" v-if="regexMatches.length" style="margin-top:8px;max-height:120px;overflow:auto;font-size:12px">
          {{ regexMatches.join(' | ') }}
        </div>
      </div>

      <div class="card" style="margin-top:10px">
        <div class="row space">
          <strong>兜底节点</strong>
          <span class="muted">开启后会在最终所有节点之后追加 REJECT</span>
        </div>
        <label class="settings-toggle" style="margin-top:8px">
          <input type="checkbox" v-model="form.add_fallback" />
          <span><strong>添加兜底 REJECT</strong><small>默认开启；用于避免策略组无可用节点时继续放行。</small></span>
        </label>
      </div>

      <div class="card" style="margin-top:10px">
        <div class="row space">
          <strong>添加来源条目</strong>
          <span class="muted">节点 / 节点组 / 节点组节点 统一排序</span>
        </div>

        <div style="margin-top:8px">
          <div class="muted">添加节点</div>
          <div class="row">
            <select v-model="selectedNodeName" style="min-width:320px">
              <option value="">选择节点</option>
              <option v-for="name in selectableNodeNames" :key="name" :value="name">{{ name }}</option>
            </select>
            <button @click="addNode">加入节点</button>
            <button @click="addBuiltin('DIRECT')">DIRECT</button>
            <button @click="addBuiltin('PASS')">PASS</button>
            <button @click="addBuiltin('REJECT')">REJECT</button>
          </div>
        </div>

        <div style="margin-top:8px">
          <div class="muted">添加节点组</div>
          <div class="row">
            <select v-model.number="selectedGroupId" style="min-width:320px">
              <option :value="null">选择节点组</option>
              <option v-for="g in selectableGroups" :key="g.id" :value="g.id">{{ g.name }}</option>
            </select>
            <button @click="addGroupRef">添加节点组引用</button>
            <button @click="addGroupNodes">添加节点组节点</button>
          </div>
        </div>
      </div>

      <div class="card" style="margin-top:10px">
        <div class="row space">
          <strong>统一排序条目</strong>
          <span class="muted">{{ form.include_entries.length }} 项</span>
        </div>
        <div class="mono" style="font-size:12px;max-height:260px;overflow:auto;margin-top:8px">
          <div v-for="(entry, idx) in form.include_entries" :key="`${entry.type}-${entry.value}-${idx}`" class="row space" style="padding:4px 0;border-bottom:1px solid var(--border)">
            <span
              draggable="true"
              style="cursor:grab"
              @dragstart="onDragStart(idx)"
              @dragover.prevent
              @drop="onDrop(idx)"
            >
              {{ idx + 1 }}. <span class="badge">{{ typeLabel(entry.type) }}</span> {{ formatEntry(entry) }}
            </span>
            <div class="row">
              <button :disabled="idx===0" @click="moveEntry(idx,-1)">上移</button>
              <button :disabled="idx===form.include_entries.length-1" @click="moveEntry(idx,1)">下移</button>
              <button class="danger" @click="removeEntry(idx)">删除</button>
            </div>
          </div>
        </div>
      </div>

      <div class="card" style="margin-top:10px" v-if="showRaw">
        <div class="row space">
          <strong>Raw JSON</strong>
          <button @click="syncFromRaw">应用 Raw</button>
        </div>
        <textarea v-model="rawJson"></textarea>
      </div>

      <div class="row" style="margin-top:12px">
        <button class="primary" @click="save">保存</button>
        <button @click="showRaw = !showRaw">{{ showRaw ? '隐藏 Raw' : '显示 Raw' }}</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { createNodeGroup, getAllSubscriptionNodes, getNodeGroups, updateNodeGroup } from '../api'

const props = defineProps({ group: { type: Object, default: null } })
const emit = defineEmits(['saved', 'close'])

const allGroups = ref([])
const allNodes = ref([])
const selectedGroupId = ref(null)
const selectedNodeName = ref('')
const showRaw = ref(false)
const rawJson = ref('')
const regexText = ref('')
const regexMatches = ref([])
const draggingIndex = ref(-1)

const form = ref(defaultForm())

watch(
  () => props.group,
  async (value) => {
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
      regexMatches.value = collectRegexMatches(form.value.regex_rules)
    } else {
      form.value = defaultForm()
      regexText.value = ''
      regexMatches.value = []
    }
    rawJson.value = JSON.stringify(form.value, null, 2)
  },
  { immediate: true }
)

watch(
  form,
  (value) => {
    rawJson.value = JSON.stringify(value, null, 2)
  },
  { deep: true }
)

const selectableGroups = computed(() => allGroups.value.filter((item) => item.id !== form.value.id))
const selectableNodeNames = computed(() => allNodes.value.map((node) => String(node.name || '').trim()).filter(Boolean))

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
  const rules = regexText.value.split('\n').map((line) => line.trim()).filter(Boolean)
  regexMatches.value = collectRegexMatches(rules)
}

function applyRegexRules() {
  const rules = regexText.value.split('\n').map((line) => line.trim()).filter(Boolean)
  form.value.regex_rules = rules
  form.value.kind = rules.length ? 'regex' : 'manual'
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
  const exists = form.value.include_entries.some((item) => item.type === entry.type && String(item.value) === String(entry.value))
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
    regexMatches.value = collectRegexMatches(form.value.regex_rules)
  } catch (err) {
    alert(`Raw JSON 格式错误: ${err.message}`)
  }
}

async function save() {
  const payload = {
    ...form.value,
    name: String(form.value.name || '').trim(),
    kind: (form.value.regex_rules || []).length ? 'regex' : 'manual',
    regex_rules: uniq(form.value.regex_rules || []),
    include_entries: normalizeEntries(form.value.include_entries || []),
    exclude_nodes: uniq(form.value.exclude_nodes || []),
  }

  if (payload.id) {
    await updateNodeGroup(payload.id, payload)
  } else {
    await createNodeGroup(payload)
  }
  emit('saved')
  emit('close')
}

async function loadSources() {
  const [groupsRes, nodesRes] = await Promise.all([getNodeGroups(), getAllSubscriptionNodes()])
  allGroups.value = groupsRes.data
  allNodes.value = nodesRes.data
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
