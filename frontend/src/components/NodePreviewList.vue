<template>
  <div class="node-preview-box">
    <div class="node-preview-toolbar">
      <label class="node-preview-search">
        <span>搜索节点</span>
        <input v-model.trim="query" :placeholder="placeholder" />
      </label>
      <div class="node-preview-summary">
        <span class="count-pill">{{ filteredNodes.length }} / {{ normalizedNodes.length }}</span>
        <button @click="runFullProbe" :disabled="probing || !normalizedNodes.length" class="primary">
          {{ probing ? '探测中…' : '⚡ 综合探测' }}
        </button>
        <button @click="runTcpProbe" :disabled="probing || !normalizedNodes.length">
          TCP 探活
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
      {{ isFullProbe ? '⚡ 出口探测' : 'TCP 探活' }}：ok {{ probeSummary.ok }} / fail {{ probeSummary.fail }} / timeout {{ probeSummary.timeout }} / skip {{ probeSummary.skip }}
      <span v-if="isFullProbe">· 经隔离 sing-box 验证握手、落地地区、流媒体与测速</span>
      <span v-else>· 仅测端口可达，不是代理延迟</span>
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

        <!-- 详细能力徽标 (流媒体/测速/出口) -->
        <template v-if="probeStatus(node) && (probeStatus(node).latency_ms !== undefined || probeStatus(node).handshake_ms !== undefined)">
          <span
            v-if="probeStatus(node).status === 'ok'"
            class="probe-pill probe-ok"
            :title="`握手延迟: ${probeStatus(node).latency_ms ?? probeStatus(node).handshake_ms}ms | 出口 IP: ${probeStatus(node).ip || probeStatus(node).outbound_ip || '-'} (${probeStatus(node).country || probeStatus(node).country_code || '-'})`"
          >
            ⚡ {{ probeStatus(node).latency_ms ?? probeStatus(node).handshake_ms }}ms
          </span>
          <span
            v-else-if="probeStatus(node).status === 'timeout'"
            class="probe-pill probe-timeout"
            :title="probeStatus(node).error || '握手超时'"
          >
            timeout
          </span>
          <span
            v-else
            class="probe-pill probe-fail"
            :title="probeStatus(node).error || '握手失败'"
          >
            fail
          </span>

          <span
            v-if="probeStatus(node).speed_mbps || probeStatus(node).download_speed_mbps"
            class="probe-pill probe-speed"
            :title="`测速: ${probeStatus(node).speed_mbps ?? probeStatus(node).download_speed_mbps} Mbps`"
          >
            🚀 {{ probeStatus(node).speed_mbps ?? probeStatus(node).download_speed_mbps }}M
          </span>

          <!-- 流媒体徽标 -->
          <template v-if="probeStatus(node).media || probeStatus(node).streaming_unlock">
            <span
              v-if="(probeStatus(node).media?.youtube || probeStatus(node).streaming_unlock?.youtube)?.status === 'ok' || (probeStatus(node).media?.youtube || probeStatus(node).streaming_unlock?.youtube)?.unlocked"
              class="probe-tag tag-yt"
              :title="`YouTube: ${(probeStatus(node).media?.youtube || probeStatus(node).streaming_unlock?.youtube)?.region || 'OK'}`"
            >
              YT:{{ (probeStatus(node).media?.youtube || probeStatus(node).streaming_unlock?.youtube)?.region || 'OK' }}
            </span>
            <span
              v-if="(probeStatus(node).media?.netflix || probeStatus(node).streaming_unlock?.netflix)?.status === 'full' || (probeStatus(node).media?.netflix || probeStatus(node).streaming_unlock?.netflix)?.status === 'originals' || (probeStatus(node).media?.netflix || probeStatus(node).streaming_unlock?.netflix)?.unlocked"
              class="probe-tag tag-nf"
              :title="`Netflix: ${(probeStatus(node).media?.netflix || probeStatus(node).streaming_unlock?.netflix)?.label || (probeStatus(node).media?.netflix || probeStatus(node).streaming_unlock?.netflix)?.region || 'OK'}`"
            >
              NF:{{ (probeStatus(node).media?.netflix || probeStatus(node).streaming_unlock?.netflix)?.region || 'OK' }}
            </span>
            <span
              v-if="(probeStatus(node).media?.chatgpt || probeStatus(node).streaming_unlock?.chatgpt)?.status === 'ok' || (probeStatus(node).media?.chatgpt || probeStatus(node).streaming_unlock?.chatgpt)?.unlocked"
              class="probe-tag tag-gpt"
              title="ChatGPT 解锁正常"
            >
              GPT
            </span>
            <span
              v-if="(probeStatus(node).media?.gemini || probeStatus(node).streaming_unlock?.gemini)?.status === 'ok' || (probeStatus(node).media?.gemini || probeStatus(node).streaming_unlock?.gemini)?.unlocked"
              class="probe-tag tag-gemini"
              title="Google Gemini 解锁正常"
            >
              Gemini
            </span>
            <span
              v-if="(probeStatus(node).media?.disney || probeStatus(node).streaming_unlock?.disney)?.status === 'ok' || (probeStatus(node).media?.disney || probeStatus(node).streaming_unlock?.disney)?.unlocked"
              class="probe-tag tag-disney"
              title="Disney+ 解锁正常"
            >
              Disney
            </span>
            <span
              v-if="(probeStatus(node).media?.meta_ai || probeStatus(node).streaming_unlock?.meta_ai)?.status === 'ok' || (probeStatus(node).media?.meta_ai || probeStatus(node).streaming_unlock?.meta_ai)?.unlocked"
              class="probe-tag tag-meta"
              title="Meta AI 解锁正常"
            >
              Meta
            </span>
            <span
              v-if="(probeStatus(node).media?.bilibili || probeStatus(node).streaming_unlock?.bilibili)?.status === 'ok' || (probeStatus(node).media?.bilibili || probeStatus(node).streaming_unlock?.bilibili)?.unlocked"
              class="probe-tag tag-bili"
              title="Bilibili 港澳台解锁正常"
            >
              Bili
            </span>
          </template>
        </template>

        <!-- 普通 TCP 探活徽标 -->
        <span
          v-else-if="probeStatus(node)"
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
import { computed, onMounted, ref, watch } from 'vue'
import { getApiErrorMessage, getProbeResults, probeNodesFull, probeTcp } from '../api'
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
const isFullProbe = ref(false)
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

onMounted(async () => {
  try {
    const res = await getProbeResults()
    const data = res?.data?.results || res?.data
    if (data && typeof data === 'object' && Object.keys(data).length) {
      const next = { ...probeMap.value }
      for (const [name, item] of Object.entries(data)) {
        if (!item || typeof item !== 'object') continue
        const key = `${item.name || name}|${item.server || ''}|${item.port || ''}`
        next[key] = item
        next[name] = item
      }
      probeMap.value = next
    }
  } catch (_) {}
})

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
  isFullProbe.value = false
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

async function runFullProbe() {
  if (probing.value || !props.nodes.length) return
  probing.value = true
  isFullProbe.value = true
  probeError.value = ''
  try {
    // 传递原始节点对象（包含所有连接属性），以便转换 sing-box 配置
    const { data } = await probeNodesFull({
      nodes: props.nodes,
      use_settings: true,
    })
    const next = {}
    for (const item of data.results || []) {
      const key = `${item.name || ''}|${item.server || ''}|${item.port || ''}`
      next[key] = item
    }
    probeMap.value = next
    probeSummary.value = data.summary || null
  } catch (err) {
    probeError.value = getApiErrorMessage(err, '综合探测失败')
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
.probe-speed {
  color: #38bdf8;
  border-color: color-mix(in srgb, #38bdf8 40%, transparent);
  font-weight: 600;
}
.probe-tag {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 4px;
  font-weight: bold;
}
.tag-yt {
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
  border: 1px solid rgba(239, 68, 68, 0.3);
}
.tag-nf {
  background: rgba(220, 38, 38, 0.2);
  color: #ef4444;
  border: 1px solid rgba(220, 38, 38, 0.4);
}
.tag-gpt {
  background: rgba(16, 185, 129, 0.15);
  color: #34d399;
  border: 1px solid rgba(16, 185, 129, 0.3);
}
.tag-gemini {
  background: rgba(59, 130, 246, 0.15);
  color: #60a5fa;
  border: 1px solid rgba(59, 130, 246, 0.3);
}
.tag-meta {
  background: rgba(168, 85, 247, 0.15);
  color: #c084fc;
  border: 1px solid rgba(168, 85, 247, 0.3);
}
</style>
