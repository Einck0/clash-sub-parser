<template>
  <div class="node-preview-box">
    <div class="node-preview-toolbar">
      <label class="node-preview-search">
        <span>搜索节点</span>
        <input v-model.trim="query" :placeholder="placeholder" />
      </label>
      <div class="node-preview-summary">
        <span class="count-pill">{{ filteredNodes.length }} / {{ normalizedNodes.length }}</span>
        <button v-if="editable" @click="toggleEditMode" :class="{ primary: editMode }">
          {{ editMode ? '完成改名' : '改名' }}
        </button>
        <button v-if="normalizedNodes.length" @click="loadCountries" :disabled="loadingGeo">
          {{ loadingGeo ? '归属查询中...' : '🌍 归属国' }}
        </button>
        <button v-if="normalizedNodes.length" @click="checkLatencies" :disabled="checking">
          {{ checking ? '测速中...' : '⚡ 测速' }}
        </button>
        <button v-if="filteredNodes.length > collapsedLimit" @click="expanded = !expanded">
          {{ expanded ? '收起' : `展开全部 ${filteredNodes.length}` }}
        </button>
      </div>
    </div>

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
        <span v-if="geoLabel(node)" class="geo-pill" :title="geoTitle(node)">{{ geoLabel(node) }}</span>
        <span v-if="node.type" class="node-type-pill">{{ node.type }}</span>
        <span v-if="latencyOf(node) !== undefined" class="latency-pill" :class="latencyClassValue(latencyOf(node))">
          {{ latencyOf(node) === null ? '超时' : latencyOf(node) + 'ms' }}
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
import { checkLatency, lookupGeoIp } from '../api'

const props = defineProps({
  nodes: { type: Array, default: () => [] },
  collapsedLimit: { type: Number, default: 18 },
  placeholder: { type: String, default: '输入地区、协议、域名或端口' },
  autoGeo: { type: Boolean, default: true },
  // When true, names are treated as post-prefix final names and can be renamed.
  editable: { type: Boolean, default: false },
  // Existing renames map: { currentFinalName: newFinalName }
  renames: { type: Object, default: () => ({}) },
  saving: { type: Boolean, default: false },
})

const emit = defineEmits(['save-renames'])

const query = ref('')
const expanded = ref(false)
const checking = ref(false)
const loadingGeo = ref(false)
const editMode = ref(false)
const draftRenames = ref({})
const latencyMap = ref({})  // key: node final name or host:port -> ms|null
const geoMap = ref({}) // key: node final name or host -> {country, country_code, ip, error, mode}
let lastAutoKey = ''

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

watch(
  normalizedNodes,
  (nodes) => {
    if (!props.autoGeo) return
    const key = nodes
      .map((n) => n.server || n.meta)
      .filter(Boolean)
      .slice(0, 40)
      .join('|')
    if (!key || key === lastAutoKey) return
    lastAutoKey = key
    loadCountries()
  },
  { immediate: true }
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
    // leaving edit mode without explicit save keeps draft in memory for this modal open
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

function hostOf(node) {
  return String(node?.server || '').trim() || String(node?.meta || '').split(':')[0] || ''
}

function nodeKey(node) {
  return displayName(node) || node?.baseName || hostOf(node) || node?.meta || ''
}

function geoInfo(node) {
  const key = nodeKey(node)
  return geoMap.value[key] || geoMap.value[hostOf(node)] || null
}

function geoLabel(node) {
  const info = geoInfo(node)
  if (!info) return ''
  if (info.country_code && info.country_code !== 'LAN') return info.country_code
  if (info.country) return info.country
  if (info.error) return '?'
  return ''
}

function geoTitle(node) {
  const info = geoInfo(node)
  if (!info) return ''
  const mode = info.mode === 'exit' ? '出口IP' : '服务器IP'
  const parts = [mode, info.country, info.country_code, info.ip, info.error].filter(Boolean)
  return parts.join(' / ')
}

function latencyOf(node) {
  const key = nodeKey(node)
  if (Object.prototype.hasOwnProperty.call(latencyMap.value, key)) return latencyMap.value[key]
  if (node?.meta && Object.prototype.hasOwnProperty.call(latencyMap.value, node.meta)) {
    return latencyMap.value[node.meta]
  }
  return undefined
}

async function loadCountries() {
  if (loadingGeo.value) return
  const nodes = normalizedNodes.value.filter((n) => nodeKey(n))
  if (!nodes.length) return
  loadingGeo.value = true
  try {
    const map = { ...geoMap.value }
    // Prefer exit-IP via mihomo when names exist; otherwise server-IP fallback.
    const names = [...new Set(nodes.map((n) => displayName(n) || n.baseName).filter(Boolean))]
    if (names.length) {
      for (let i = 0; i < names.length; i += 20) {
        const chunk = names.slice(i, i + 20)
        const { data } = await lookupGeoIp({ names: chunk })
        for (const item of data || []) {
          const key = item?.name || item?.host
          if (!key) continue
          map[key] = {
            country: item.country || null,
            country_code: item.country_code || null,
            ip: item.ip || null,
            error: item.error || null,
            mode: item.mode || 'exit',
          }
        }
      }
    } else {
      const hosts = [...new Set(nodes.map((n) => hostOf(n)).filter(Boolean))]
      for (let i = 0; i < hosts.length; i += 40) {
        const chunk = hosts.slice(i, i + 40)
        const { data } = await lookupGeoIp({ hosts: chunk })
        for (const item of data || []) {
          if (!item?.host) continue
          map[item.host] = {
            country: item.country || null,
            country_code: item.country_code || null,
            ip: item.ip || null,
            error: item.error || null,
            mode: item.mode || 'server',
          }
        }
      }
    }
    geoMap.value = map
  } catch {
    // ignore lookup failure; keep previous results
  } finally {
    loadingGeo.value = false
  }
}

async function checkLatencies() {
  checking.value = true
  try {
    const names = [
      ...new Set(
        normalizedNodes.value
          .map((n) => displayName(n) || n.baseName)
          .filter(Boolean),
      ),
    ].slice(0, 30)
    const map = {}
    if (names.length) {
      const { data } = await checkLatency({ names, timeoutMs: 8000 })
      for (const r of data || []) {
        if (!r?.name) continue
        map[r.name] = r.latency_ms ?? null
      }
    } else {
      const hosts = normalizedNodes.value.map((n) => n.meta).filter((m) => m && m.includes(':')).slice(0, 30)
      if (!hosts.length) return
      const { data } = await checkLatency({ hosts, timeoutMs: 5000 })
      for (const r of data || []) {
        if (r?.host == null) continue
        map[`${r.host}:${r.port}`] = r.latency_ms ?? null
      }
    }
    latencyMap.value = map
  } catch {
    // Ignore errors
  } finally {
    checking.value = false
  }
}

function latencyClassValue(ms) {
  if (ms === null || ms === undefined) return ''
  if (ms < 200) return 'good'
  if (ms < 500) return 'ok'
  return 'bad'
}
</script>

<style scoped>
.latency-pill,
.geo-pill {
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 8px;
  font-weight: 600;
  white-space: nowrap;
}
.latency-pill.good {
  color: #1f7a3f;
  background: rgba(46, 160, 90, 0.12);
}
.latency-pill.ok {
  color: #9a6b00;
  background: rgba(230, 170, 40, 0.14);
}
.latency-pill.bad {
  color: #b23a3a;
  background: rgba(196, 72, 72, 0.12);
}
.geo-pill {
  color: #2b5f9e;
  background: rgba(54, 120, 200, 0.12);
  border: 1px solid rgba(54, 120, 200, 0.2);
}
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
</style>
