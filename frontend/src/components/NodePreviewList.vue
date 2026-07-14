<template>
  <div class="node-preview-box">
    <div class="node-preview-toolbar">
      <label class="node-preview-search">
        <span>搜索节点</span>
        <input v-model.trim="query" :placeholder="placeholder" />
      </label>
      <div class="node-preview-summary">
        <span class="count-pill">{{ filteredNodes.length }} / {{ normalizedNodes.length }}</span>
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

    <div v-if="!normalizedNodes.length" class="empty-mini">暂无节点可预览</div>
    <div v-else-if="!filteredNodes.length" class="empty-mini">没有匹配的节点</div>
    <div v-else class="node-preview-list" :class="{ expanded }">
      <div v-for="(node, idx) in visibleNodes" :key="`${node.name}-${idx}`" class="node-preview-item">
        <div class="node-preview-main">
          <strong :title="node.name">{{ node.name || '(无名节点)' }}</strong>
          <span v-if="node.meta" class="node-preview-meta" :title="node.meta">{{ node.meta }}</span>
        </div>
        <span v-if="geoLabel(node)" class="geo-pill" :title="geoTitle(node)">{{ geoLabel(node) }}</span>
        <span v-if="node.type" class="node-type-pill">{{ node.type }}</span>
        <span v-if="latencyMap[node.meta] !== undefined" class="latency-pill" :class="latencyClass(node.meta)">
          {{ latencyMap[node.meta] === null ? '超时' : latencyMap[node.meta] + 'ms' }}
        </span>
      </div>
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
})

const query = ref('')
const expanded = ref(false)
const checking = ref(false)
const loadingGeo = ref(false)
const latencyMap = ref({})  // { "host:port": ms|null }
const geoMap = ref({}) // { host: {country, country_code, ip, error} }
let lastAutoKey = ''

const normalizedNodes = computed(() => props.nodes.map(normalizeNode).filter(Boolean))
const filteredNodes = computed(() => {
  const q = query.value.toLowerCase()
  if (!q) return normalizedNodes.value
  return normalizedNodes.value.filter((node) => node.searchText.includes(q))
})
const visibleNodes = computed(() =>
  expanded.value ? filteredNodes.value : filteredNodes.value.slice(0, props.collapsedLimit)
)

watch(query, () => {
  expanded.value = false
})

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
    return { name, type: '', meta: '', searchText: name.toLowerCase(), server: '', port: '' }
  }

  const name = String(node?.name || '').trim()
  const type = String(node?.type || '').trim()
  const server = String(node?.server || '').trim()
  const port = node?.port ? String(node.port).trim() : ''
  const meta = [server, port].filter(Boolean).join(':')
  const searchText = [name, type, server, port].join(' ').toLowerCase()
  return { name, type, meta, searchText, server, port }
}

function hostOf(node) {
  return String(node?.server || '').trim() || String(node?.meta || '').split(':')[0] || ''
}

function geoLabel(node) {
  const host = hostOf(node)
  const info = geoMap.value[host]
  if (!info) return ''
  if (info.country_code && info.country_code !== 'LAN') return info.country_code
  if (info.country) return info.country
  if (info.error) return '?'
  return ''
}

function geoTitle(node) {
  const host = hostOf(node)
  const info = geoMap.value[host]
  if (!info) return ''
  const parts = [info.country, info.country_code, info.ip, info.error].filter(Boolean)
  return parts.join(' / ')
}

async function loadCountries() {
  if (loadingGeo.value) return
  const hosts = [...new Set(normalizedNodes.value.map((n) => hostOf(n)).filter(Boolean))]
  if (!hosts.length) return
  loadingGeo.value = true
  try {
    // Batch in chunks of 40
    const map = { ...geoMap.value }
    for (let i = 0; i < hosts.length; i += 40) {
      const chunk = hosts.slice(i, i + 40)
      const { data } = await lookupGeoIp(chunk)
      for (const item of data || []) {
        if (!item?.host) continue
        map[item.host] = {
          country: item.country || null,
          country_code: item.country_code || null,
          ip: item.ip || null,
          error: item.error || null,
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
  const hosts = normalizedNodes.value
    .map((n) => n.meta)
    .filter((m) => m && m.includes(':'))
  if (!hosts.length) {
    checking.value = false
    return
  }
  try {
    const { data } = await checkLatency(hosts.slice(0, 30), 5000)
    const map = {}
    for (const r of data) {
      const key = `${r.host}:${r.port}`
      map[key] = r.latency_ms ?? null
    }
    latencyMap.value = map
  } catch {
    // Ignore errors
  } finally {
    checking.value = false
  }
}

function latencyClass(meta) {
  const ms = latencyMap.value[meta]
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
</style>
