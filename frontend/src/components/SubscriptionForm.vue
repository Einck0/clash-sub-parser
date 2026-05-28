<template>
  <div class="card subscription-form-card">
    <h3>{{ form.id ? '编辑订阅' : '添加订阅' }}</h3>

    <div class="grid-2">
      <label>
        <div class="muted">订阅名</div>
        <input v-model="form.name" placeholder="例如：主订阅" />
      </label>
      <label>
        <div class="muted">URL</div>
        <input v-model="form.url" placeholder="https://..." />
      </label>
      <label>
        <div class="muted">更新周期（分钟）</div>
        <input v-model.number="form.update_interval" type="number" min="1" placeholder="留空则不自动更新" />
      </label>
      <label>
        <div class="muted">节点前缀</div>
        <input v-model="form.node_prefix" placeholder="主订阅空=不加，其他空=订阅名" />
      </label>
    </div>

    <label style="display:block;margin-top:10px">
      <input type="checkbox" v-model="form.is_primary" />
      <span>设为主订阅（最终 YAML 头部注释与流量响应头来源）</span>
    </label>

    <div class="selector-section">
      <div class="row space">
        <div>
          <strong>手动节点</strong>
          <p class="section-hint">可给当前订阅额外添加节点。节点链接是隐私内容：保存后不会在页面展示原始链接，只保留解析后的节点配置。</p>
        </div>
        <span class="muted">手动 {{ form.manual_nodes.length }} 个</span>
      </div>

      <div v-if="form.manual_nodes.length" class="node-select-list manual-node-list">
        <div v-for="node in form.manual_nodes" :key="nodeName(node)" class="node-select-row">
          <div class="node-select-name mono">
            <strong>{{ nodeName(node) }}</strong>
            <span>{{ node.type || '-' }} {{ node.server ? `| ${node.server}:${node.port || ''}` : '' }}</span>
          </div>
          <div class="node-select-actions">
            <button class="danger" @click="removeManualNode(node)">移除</button>
          </div>
        </div>
      </div>
      <div v-else class="empty-mini">暂无手动节点。可以在下面粘贴分享链接添加到这个订阅。</div>

      <label style="display:block;margin-top:10px">
        <div class="muted">新增节点链接</div>
        <textarea v-model="manualNodeLinks" class="secret-textarea" placeholder="支持 ss://、trojan://、vless://、vmess:// 等常见分享链接；可一行一个。保存后自动解析并加入当前订阅。"></textarea>
      </label>
    </div>

    <div class="selector-section">
      <div class="row space">
        <div>
          <strong>粗筛：正则</strong>
          <p class="section-hint">每行一条，按节点名匹配。留空 = 默认全选。候选节点 = 上游订阅节点 + 手动节点。</p>
        </div>
        <span class="muted">粗筛 {{ coarseNodes.length }} / {{ candidateNodes.length }}</span>
      </div>
      <textarea v-model="regexText" placeholder="留空表示全选\n香港\n日本.*Premium"></textarea>
      <div class="muted" style="color: var(--danger); margin-top: 6px" v-if="regexError">{{ regexError }}</div>
    </div>

    <div class="selector-section">
      <div class="row space">
        <div>
          <strong>精修：手动包含 / 排除</strong>
          <p class="section-hint">最终节点 = 正则粗筛 + 手动包含 - 手动排除。按上游/手动节点原始节点名保存。</p>
        </div>
        <span class="muted">最终 {{ finalPreviewNodes.length }} 个节点</span>
      </div>

      <div class="selector-stats">
        <span class="badge">候选 {{ candidateNodes.length }}</span>
        <span class="badge">包含 {{ form.include_node_names.length }}</span>
        <span class="badge">排除 {{ form.exclude_node_names.length }}</span>
      </div>

      <div class="node-search-row">
        <input v-model="nodeSearch" placeholder="搜索节点名" />
        <button @click="clearManualSelection" :disabled="!form.include_node_names.length && !form.exclude_node_names.length">清空手动选择</button>
      </div>

      <div v-if="!candidateNodes.length" class="empty-mini">暂无候选节点。先保存订阅并拉取一次，或添加手动节点。</div>
      <div v-else class="node-select-list">
        <div v-for="node in visibleCandidateNodes" :key="nodeName(node)" class="node-select-row">
          <div class="node-select-name mono">
            <strong>{{ nodeName(node) }}</strong>
            <span>{{ node.type || '-' }} {{ node.server ? `| ${node.server}:${node.port || ''}` : '' }}</span>
          </div>
          <div class="node-select-actions">
            <button :class="{ primary: nodeMode(node) === 'auto' }" @click="setNodeMode(node, 'auto')">自动</button>
            <button :class="{ primary: nodeMode(node) === 'include' }" @click="setNodeMode(node, 'include')">包含</button>
            <button :class="{ danger: nodeMode(node) === 'exclude' }" @click="setNodeMode(node, 'exclude')">排除</button>
          </div>
        </div>
      </div>

      <div class="card" style="margin-top:10px">
        <div class="muted" style="margin-bottom:6px">最终预览（前 120 条）</div>
        <div class="mono" style="font-size:12px;max-height:220px;overflow:auto">
          <div v-for="(node, idx) in finalPreviewNodes.slice(0, 120)" :key="idx">{{ nodeName(node) || '(无名节点)' }}</div>
        </div>
      </div>
    </div>

    <div class="row" style="margin-top:12px">
      <button class="primary" @click="handleSave" :disabled="saveDisabled">保存</button>
      <button @click="$emit('cancel')">取消</button>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'

const props = defineProps({
  subscription: { type: Object, default: null },
  allNodes: { type: Array, default: () => [] },
})
const emit = defineEmits(['save', 'cancel'])

const form = ref(createDefault())
const regexText = ref('')
const regexError = ref('')
const nodeSearch = ref('')
const manualNodeLinks = ref('')

watch(
  () => props.subscription,
  (value) => {
    if (!value) {
      form.value = createDefault()
      regexText.value = ''
      regexError.value = ''
      nodeSearch.value = ''
      manualNodeLinks.value = ''
      return
    }
    form.value = {
      id: value.id,
      name: value.name || '',
      url: value.url || '',
      update_interval: value.update_interval,
      is_primary: !!value.is_primary,
      node_prefix: value.node_prefix || '',
      filter_regex: value.filter_regex || [],
      include_node_names: value.include_node_names || [],
      exclude_node_names: value.exclude_node_names || [],
      manual_nodes: value.manual_nodes || [],
      source_nodes: value.source_nodes || [],
      raw_nodes: value.raw_nodes || [],
    }
    regexText.value = (value.filter_regex || []).join('\n')
    nodeSearch.value = ''
    manualNodeLinks.value = ''
  },
  { immediate: true }
)

watch(regexText, (value) => {
  regexError.value = ''
  const lines = value
    .split('\n')
    .map((item) => item.trim())
    .filter(Boolean)
  for (const [idx, pattern] of lines.entries()) {
    try {
      new RegExp(pattern)
    } catch (err) {
      regexError.value = `第 ${idx + 1} 条正则无效：${err.message}`
      break
    }
  }
  form.value.filter_regex = lines
})

const candidateNodes = computed(() => {
  const upstream = form.value.source_nodes?.length
    ? form.value.source_nodes
    : (form.value.raw_nodes?.length ? form.value.raw_nodes : props.allNodes)
  return uniqueNodesByName([...(upstream || []), ...(form.value.manual_nodes || [])])
})

const regexPatterns = computed(() => {
  const patterns = []
  for (const item of form.value.filter_regex || []) {
    try {
      patterns.push(new RegExp(item, 'i'))
    } catch (_) {
      continue
    }
  }
  return patterns
})

const coarseNodes = computed(() => {
  if (!regexPatterns.value.length) return candidateNodes.value
  return candidateNodes.value.filter((node) => regexPatterns.value.some((p) => p.test(nodeName(node))))
})

const finalPreviewNodes = computed(() => {
  const selected = new Map(coarseNodes.value.map((node) => [nodeName(node), node]))
  const include = new Set(form.value.include_node_names || [])
  const exclude = new Set(form.value.exclude_node_names || [])
  for (const node of candidateNodes.value) {
    const name = nodeName(node)
    if (include.has(name)) selected.set(name, node)
  }
  for (const name of exclude) selected.delete(name)
  return [...selected.values()]
})

const saveDisabled = computed(() => {
  return !!regexError.value || !form.value.name.trim() || !form.value.url.trim()
})

const visibleCandidateNodes = computed(() => {
  const q = nodeSearch.value.trim().toLowerCase()
  const nodes = candidateNodes.value
  if (!q) return nodes.slice(0, 240)
  return nodes.filter((node) => nodeName(node).toLowerCase().includes(q)).slice(0, 240)
})

function nodeName(node) {
  return String(node?.name || '').trim()
}

function nodeMode(node) {
  const name = nodeName(node)
  if ((form.value.exclude_node_names || []).includes(name)) return 'exclude'
  if ((form.value.include_node_names || []).includes(name)) return 'include'
  return 'auto'
}

function setNodeMode(node, mode) {
  const name = nodeName(node)
  if (!name) return
  const include = new Set(form.value.include_node_names || [])
  const exclude = new Set(form.value.exclude_node_names || [])
  include.delete(name)
  exclude.delete(name)
  if (mode === 'include') include.add(name)
  if (mode === 'exclude') exclude.add(name)
  form.value.include_node_names = [...include]
  form.value.exclude_node_names = [...exclude]
}

function removeManualNode(node) {
  const name = nodeName(node)
  form.value.manual_nodes = (form.value.manual_nodes || []).filter((item) => nodeName(item) !== name)
  form.value.include_node_names = (form.value.include_node_names || []).filter((item) => item !== name)
  form.value.exclude_node_names = (form.value.exclude_node_names || []).filter((item) => item !== name)
}

function clearManualSelection() {
  form.value.include_node_names = []
  form.value.exclude_node_names = []
}

function handleSave() {
  if (saveDisabled.value) return
  const payload = {
    name: form.value.name?.trim(),
    url: form.value.url?.trim(),
    update_interval: form.value.update_interval || null,
    is_primary: !!form.value.is_primary,
    node_prefix: form.value.node_prefix?.trim() || null,
    filter_regex: form.value.filter_regex || [],
    include_node_names: form.value.include_node_names || [],
    exclude_node_names: form.value.exclude_node_names || [],
    manual_nodes: form.value.manual_nodes || [],
    manual_node_links: manualNodeLinks.value.trim() || null,
  }
  emit('save', payload)
}

function uniqueNodesByName(nodes) {
  const seen = new Set()
  const out = []
  for (const node of nodes || []) {
    const name = nodeName(node)
    if (!name || seen.has(name)) continue
    seen.add(name)
    out.push(node)
  }
  return out
}

function createDefault() {
  return {
    id: null,
    name: '',
    url: '',
    update_interval: null,
    is_primary: false,
    node_prefix: '',
    filter_regex: [],
    include_node_names: [],
    exclude_node_names: [],
    manual_nodes: [],
    source_nodes: [],
    raw_nodes: [],
  }
}
</script>
