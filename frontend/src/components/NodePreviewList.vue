<template>
  <div class="node-preview-box">
    <div class="node-preview-toolbar">
      <label class="node-preview-search">
        <span>搜索节点</span>
        <input v-model.trim="query" :placeholder="placeholder" />
      </label>
      <div class="node-preview-summary">
        <span class="count-pill">{{ filteredNodes.length }} / {{ normalizedNodes.length }}</span>
        <button @click="runTcpProbe" :disabled="probing || !normalizedNodes.length">
          {{ probing ? '探活中…' : 'TCP 探活' }}
        </button>
        <button v-if="editable" @click="toggleEditMode" :class="{ primary: editMode }">
          {{ editMode ? '完成改名' : '改名' }}
        </button>
        <button v-if="filteredNodes.length > collapsedLimit" @click="expanded = !expanded">
          {{ expanded ? '收起' : `展开全部 ${filteredNodes.length}` }}
        </button>
      </div>
    </div>

    <p v-if="probeSummary" class="section-hint" style="margin: 0 0 8px">
      TCP 探活：ok {{ probeSummary.ok }} / fail {{ probeSummary.fail }} / timeout {{ probeSummary.timeout }} / skip {{ probeSummary.skip }}
      · 仅测端口可达，不是代理延迟
    </p>
    <p v-if="probeError" class="form-alert form-alert-error" style="margin: 0 0 8px">{{ probeError }}</p>

    <p v-if="editable && editMode" class="section-hint" style="margin: 0 0 8px">
      这里改的是<strong>前缀后的最终节点名</strong>。保存后策略组按新名字匹配。
    </p>

    <div v-if="!normalizedNodes.length" class="empty-mini">暂无节点可预览</div>
    <div v-else-if="!filteredNodes.length" class="empty-mini">没有匹配的节点</div>
    <div v-else class="node-preview-list" :class="{ expanded }">
      <div v-for="(node, idx) in visibleNodes" :key="`${node.baseName}-${idx}`" class="node-preview-item">
        <div class="node-preview-main">
          <template v-if="editable && editMode">
            <input
              class="node-rename-input"
              :value="displayName(node)"
              :placeholder="node.baseName"
              @input="onRenameInput(node.baseName, $event.target.value)"
            />
            <span class="node-preview-meta" v-if="displayName(node) !== node.baseName">
              原名：{{ node.baseName }}
            </span>
            <span v-if="node.meta" class="node-preview-meta" :title="node.meta">{{ node.meta }}</span>
          </template>
          <template v-else>
            <span class="node-flag">{{ getNodeFlag(displayName(node)) }}</span>
            <strong :title="displayName(node)">{{ displayName(node) || '(无名节点)' }}</strong>
            <span
              v-if="displayName(node) !== node.baseName"
              class="node-preview-meta"
              :title="`原名 ${node.baseName}`"
            >
              原名：{{ node.baseName }}
            </span>
            <span v-if="node.meta" class="node-preview-meta" :title="node.meta">{{ node.meta }}</span>
          </template>
        </div>
        <span v-if="node.type" class="node-type-pill">{{ node.type }}</span>
        <span
          v-if="probeStatus(node)"
          class="probe-pill"
          :class="`probe-${probeStatus(node).status}`"
          :title="probeTitle(node)"
        >
          {{ probeLabel(node) }}
        </span>
        <button
          v-if="editable && editMode && displayName(node) !== node.baseName"
          class="danger"
          @click="onRenameInput(node.baseName, node.baseName)"
        >
          还原
        </button>
      </div>
    </div>

    <div v-if="editable && editMode" class="node-rename-actions">
      <button class="primary" @click="saveRenames" :disabled="saving || !dirtyCount">
        {{ saving ? '保存中...' : `保存改名 (${dirtyCount})` }}
      </button>
      <button @click="resetDraft" :disabled="saving || !dirtyCount">重置未保存</button>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { getApiErrorMessage, probeTcp } from '../api'
import { getNodeFlag } from '../utils/format'

const props = defineProps({
  nodes: { type: Array, default: () => [] },
  collapsedLimit: { type: Number, default: 18 },
  placeholder: { type: String, default: '输入地区、协议、域名或端口' },
  // When true, names are treated as post-prefix final names and can be renamed.
  editable: { type: Boolean, default: false },
  // Existing renames map: { currentFinalName: newFinalName }
  renames: { type: Object, default: () => ({}) },
  saving: { type: Boolean, default: false },
})

const emit = defineEmits(['save-renames'])

const query = ref('')
const expanded = ref(false)
const editMode = ref(false)
const draftRenames = ref({})
const probing = ref(false)
const probeError = ref('')
const probeSummary = ref(null)
// key: name|server|port -> result
const probeMap = ref({})

const normalizedNodes = computed(() => props.nodes.map(normalizeNode).filter(Boolean))
const filteredNodes = computed(() => {
  const q = query.value.toLowerCase()
  if (!q) return normalizedNodes.value
  return normalizedNodes.value.filter((node) => {
    const shown = displayName(node).toLowerCase()
    return node.searchText.includes(q) || shown.includes(q)
  })
})
const visibleNodes = computed(() =>
  expanded.value ? filteredNodes.value : filteredNodes.value.slice(0, props.collapsedLimit)
)
const dirtyCount = computed(() => {
  let count = 0
  for (const node of normalizedNodes.value) {
    const base = node.baseName
    const current = String(draftRenames.value[base] || base).trim() || base
    const initial = String((props.renames || {})[base] || base).trim() || base
    if (current !== initial) count += 1
  }
  return count
})

watch(query, () => {
  expanded.value = false
})

watch(
  () => props.renames,
  (value) => {
    draftRenames.value = { ...(value || {}) }
  },
  { immediate: true, deep: true }
)

function normalizeNode(node) {
  if (typeof node === 'string') {
    const name = node.trim()
    if (!name) return null
    return {
      name,
      baseName: name,
      type: '',
      meta: '',
      searchText: name.toLowerCase(),
      server: '',
      port: '',
    }
  }

  const name = String(node?.name || '').trim()
  const type = String(node?.type || '').trim()
  const server = String(node?.server || '').trim()
  const port = node?.port ? String(node.port).trim() : ''
  const meta = [server, port].filter(Boolean).join(':')
  const searchText = [name, type, server, port].join(' ').toLowerCase()
  return { name, baseName: name, type, meta, searchText, server, port }
}

function displayName(node) {
  const base = node?.baseName || node?.name || ''
  const mapped = String(draftRenames.value[base] || '').trim()
  return mapped || base
}

function onRenameInput(baseName, value) {
  const next = { ...draftRenames.value }
  const target = String(value || '').trim()
  if (!target || target === baseName) delete next[baseName]
  else next[baseName] = target
  draftRenames.value = next
}

function resetDraft() {
  draftRenames.value = { ...(props.renames || {}) }
}

function toggleEditMode() {
  if (editMode.value) {
    editMode.value = false
    return
  }
  editMode.value = true
  expanded.value = true
}

function saveRenames() {
  const payload = {}
  for (const node of normalizedNodes.value) {
    const base = node.baseName
    const target = String(draftRenames.value[base] || '').trim()
    if (target && target !== base) payload[base] = target
  }
  emit('save-renames', payload)
}

function probeKey(node) {
  return `${node.baseName || node.name || ''}|${node.server || ''}|${node.port || ''}`
}

function probeStatus(node) {
  return probeMap.value[probeKey(node)] || null
}

function probeLabel(node) {
  const item = probeStatus(node)
  if (!item) return ''
  if (item.status === 'ok') return item.connect_ms != null ? `ok ${item.connect_ms}ms` : 'ok'
  if (item.status === 'timeout') return 'timeout'
  if (item.status === 'skip') return 'skip'
  return 'fail'
}

function probeTitle(node) {
  const item = probeStatus(node)
  if (!item) return ''
  const parts = [`TCP ${item.status}`]
  if (item.connect_ms != null) parts.push(`${item.connect_ms}ms`)
  if (item.error) parts.push(item.error)
  parts.push('仅端口可达，不是代理延迟')
  return parts.join(' · ')
}

async function runTcpProbe() {
  if (probing.value || !normalizedNodes.value.length) return
  probing.value = true
  probeError.value = ''
  try {
    const payloadNodes = normalizedNodes.value.map((node) => ({
      name: node.baseName || node.name,
      server: node.server,
      port: node.port ? Number(node.port) : undefined,
      type: node.type,
    }))
    const { data } = await probeTcp({ nodes: payloadNodes, timeout_ms: 2000, concurrency: 20 })
    const next = {}
    for (const item of data.results || []) {
      const key = `${item.name || ''}|${item.server || ''}|${item.port || ''}`
      next[key] = item
    }
    probeMap.value = next
    probeSummary.value = data.summary || null
  } catch (err) {
    probeError.value = getApiErrorMessage(err)
  } finally {
    probing.value = false
  }
}
</script>

<style scoped>
.node-rename-input {
  width: 100%;
  min-width: 0;
  font-size: 13px;
}
.node-rename-actions {
  display: flex;
  gap: 8px;
  margin-top: 10px;
}
.probe-pill {
  flex: 0 0 auto;
  font-size: 11px;
  padding: 2px 6px;
  border-radius: 999px;
  border: 1px solid var(--border, #334155);
}
.probe-ok {
  color: #16a34a;
  border-color: color-mix(in srgb, #16a34a 40%, transparent);
}
.probe-fail {
  color: #dc2626;
  border-color: color-mix(in srgb, #dc2626 40%, transparent);
}
.probe-timeout {
  color: #d97706;
  border-color: color-mix(in srgb, #d97706 40%, transparent);
}
.probe-skip {
  color: #64748b;
}
</style>
