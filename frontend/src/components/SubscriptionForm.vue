<template>
  <div class="card subscription-form-card">
    <div class="form-header">
      <div>
        <p class="eyebrow">{{ form.id ? 'Edit' : 'Create' }}</p>
        <h3>{{ form.id ? '编辑订阅' : '添加订阅' }}</h3>
        <p class="section-hint">主订阅在列表卡片设置。高级能力默认收起，打开后才显示对应配置。</p>
      </div>
      <button
        v-if="form.id"
        class="primary"
        @click="handleFetch"
        :disabled="fetching || saveDisabled"
      >
        {{ fetching ? '拉取中...' : '拉取节点' }}
      </button>
    </div>

    <div class="grid-2">
      <label>
        <div class="muted">订阅名</div>
        <input v-model="form.name" placeholder="粘贴 URL 后自动填二级域名" @input="onNameInput" />
      </label>
      <label>
        <div class="muted">URL</div>
        <input v-model="form.url" placeholder="https://..." @input="onUrlInput" />
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

    <div v-if="fetchError" class="form-alert form-alert-error">{{ fetchError }}</div>

    <div class="feature-toggle-row">
      <button
        type="button"
        class="feature-chip"
        :class="{ active: featureManual }"
        @click="featureManual = !featureManual"
      >
        手动节点
      </button>
      <button
        type="button"
        class="feature-chip"
        :class="{ active: featureRegex }"
        @click="featureRegex = !featureRegex"
      >
        初筛正则
      </button>
      <button
        type="button"
        class="feature-chip"
        :class="{ active: featureRefine }"
        @click="featureRefine = !featureRefine"
      >
        精修筛选
      </button>
      <button
        type="button"
        class="feature-chip"
        :class="{ active: featureRename }"
        @click="featureRename = !featureRename"
      >
        节点重命名
      </button>
      <button
        type="button"
        class="feature-chip"
        :class="{ active: featureChain }"
        @click="featureChain = !featureChain"
      >
        链式代理
      </button>
    </div>
    <p class="section-hint">节点重命名作用在「加前缀之后」的名字上，策略组匹配的也是最终名。</p>

    <div v-if="featureChain" class="selector-section">
      <div class="row space">
        <div>
          <strong>链式代理（dialer-proxy）</strong>
          <p class="section-hint">
            订阅默认链：本订阅节点建连前先走这些 hop（P0 取最后一跳写入 dialer-proxy）。
            节点覆盖：每行 <code>最终节点名=hop</code>；空 hop 表示强制不链。
          </p>
        </div>
      </div>
      <label style="display:block;margin-top:8px">
        <div class="muted">订阅默认链（逗号/换行分隔 hop 名）</div>
        <textarea
          v-model="proxyChainText"
          rows="2"
          placeholder="例如：香港  或  手动选择"
        />
      </label>
      <label style="display:block;margin-top:8px">
        <div class="muted">节点覆盖（每行 节点名=hop，空 hop=不链）</div>
        <textarea
          v-model="nodeProxyChainsText"
          rows="4"
          placeholder="美国落地=香港入口\n观察节点="
        />
      </label>
    </div>

    <div v-if="featureManual" class="selector-section">
      <div class="row space">
        <div>
          <strong>手动节点</strong>
          <p class="section-hint">额外节点链接仅在保存时解析，不会回显原始链接。</p>
        </div>
        <span class="muted">{{ form.manual_nodes.length }} 个</span>
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
      <div v-else class="empty-mini">暂无手动节点</div>

      <label style="display:block;margin-top:10px">
        <div class="muted">新增节点链接</div>
        <textarea
          v-model="manualNodeLinks"
          class="secret-textarea"
          placeholder="ss:// / trojan:// / vless:// / vmess://，一行一个"
        ></textarea>
      </label>
    </div>

    <div v-if="featureRegex" class="selector-section">
      <div class="row space">
        <div>
          <strong>粗筛：正则</strong>
          <p class="section-hint">每行一条，按上游原始节点名匹配。留空 = 全选。</p>
        </div>
        <span class="muted">粗筛 {{ coarseNodes.length }} / {{ candidateNodes.length }}</span>
      </div>
      <textarea v-model="regexText" placeholder="香港\n日本.*Premium"></textarea>
      <div class="form-alert form-alert-error" v-if="regexError">{{ regexError }}</div>
    </div>

    <div v-if="featureRefine" class="selector-section">
      <div class="row space">
        <div>
          <strong>精修：包含 / 排除</strong>
          <p class="section-hint">最终候选 = 正则粗筛 + 手动包含 − 手动排除（按原始名）。</p>
        </div>
        <span class="muted">最终 {{ selectedOriginalNodes.length }} 个</span>
      </div>

      <div class="selector-stats">
        <span class="badge">候选 {{ candidateNodes.length }}</span>
        <span class="badge">包含 {{ form.include_node_names.length }}</span>
        <span class="badge">排除 {{ form.exclude_node_names.length }}</span>
      </div>

      <div class="node-search-row">
        <input v-model="nodeSearch" placeholder="搜索节点名" />
        <button
          @click="clearManualSelection"
          :disabled="!form.include_node_names.length && !form.exclude_node_names.length"
        >
          清空手动选择
        </button>
      </div>

      <div v-if="!candidateNodes.length" class="empty-mini">
        {{ form.id ? '先点「拉取节点」或添加手动节点。' : '先保存并编辑后拉取，或添加手动节点。' }}
      </div>
      <div v-else class="node-select-list">
        <div v-for="node in visibleCandidateNodes" :key="nodeName(node)" class="node-select-row">
          <div class="node-select-name mono">
            <strong>{{ nodeName(node) }}</strong>
            <span>{{ node.type || '-' }}</span>
          </div>
          <div class="node-select-actions">
            <button :class="{ primary: nodeMode(node) === 'auto' }" @click="setNodeMode(node, 'auto')">自动</button>
            <button :class="{ primary: nodeMode(node) === 'include' }" @click="setNodeMode(node, 'include')">包含</button>
            <button :class="{ danger: nodeMode(node) === 'exclude' }" @click="setNodeMode(node, 'exclude')">排除</button>
          </div>
        </div>
      </div>
    </div>

    <div v-if="featureRename" class="selector-section">
      <div class="row space">
        <div>
          <strong>节点重命名（前缀后）</strong>
          <p class="section-hint">
            左侧是加前缀后的名字，右侧是最终输出名。策略组按最终名匹配。
            当前前缀：<code>{{ effectivePrefix || '(无)' }}</code>
          </p>
        </div>
        <span class="muted">已改名 {{ renameCount }}</span>
      </div>

      <div class="node-search-row">
        <input v-model="renameSearch" placeholder="搜索前缀后名称" />
        <button @click="clearRenames" :disabled="!renameCount">清空重命名</button>
      </div>

      <div v-if="!prefixedPreviewNodes.length" class="empty-mini">
        还没有可选节点。先拉取订阅，或打开手动节点/筛选拿到候选。
      </div>
      <div v-else class="rename-list">
        <div v-for="item in visiblePrefixedNodes" :key="item.prefixed" class="rename-row">
          <div class="rename-from mono" :title="item.prefixed">{{ item.prefixed }}</div>
          <span class="rename-arrow">→</span>
          <input
            class="rename-to"
            :value="form.node_renames[item.prefixed] || item.prefixed"
            :placeholder="item.prefixed"
            @input="setRename(item.prefixed, $event.target.value)"
          />
        </div>
      </div>
    </div>

    <div class="selector-section">
      <div class="row space">
        <strong>最终节点预览</strong>
        <span class="muted">{{ finalPreviewNames.length }} 个</span>
      </div>
      <div class="mono final-preview">
        <div v-for="(name, idx) in finalPreviewNames.slice(0, 120)" :key="idx">{{ name }}</div>
        <div v-if="!finalPreviewNames.length" class="empty-mini">暂无节点</div>
      </div>
    </div>

    <div class="form-footer">
      <button class="primary" @click="handleSave" :disabled="saveDisabled || fetching">保存</button>
      <button @click="$emit('cancel')">取消</button>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { fetchSubscription, getApiErrorMessage } from '../api'

const props = defineProps({
  subscription: { type: Object, default: null },
})
const emit = defineEmits(['save', 'cancel', 'fetched'])

const form = ref(createDefault())
const regexText = ref('')
const regexError = ref('')
const nodeSearch = ref('')
const renameSearch = ref('')
const manualNodeLinks = ref('')
const nameEdited = ref(false)
const lastAutoName = ref('')

const featureManual = ref(false)
const featureRegex = ref(false)
const featureRefine = ref(false)
const featureRename = ref(false)
const featureChain = ref(false)
const proxyChainText = ref('')
const nodeProxyChainsText = ref('')
const fetching = ref(false)
const fetchError = ref('')

watch(
  () => props.subscription,
  (value) => {
    fetchError.value = ''
    if (!value) {
      form.value = createDefault()
      regexText.value = ''
      regexError.value = ''
      nodeSearch.value = ''
      renameSearch.value = ''
      manualNodeLinks.value = ''
      proxyChainText.value = ''
      nodeProxyChainsText.value = ''
      nameEdited.value = false
      lastAutoName.value = ''
      featureManual.value = false
      featureRegex.value = false
      featureRefine.value = false
      featureRename.value = false
      featureChain.value = false
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
      node_renames: { ...(value.node_renames || {}) },
      proxy_chain: [...(value.proxy_chain || [])],
      node_proxy_chains: { ...(value.node_proxy_chains || {}) },
      manual_nodes: value.manual_nodes || [],
      source_nodes: value.source_nodes || [],
      raw_nodes: value.raw_nodes || [],
    }
    proxyChainText.value = chainToText(value.proxy_chain || [])
    nodeProxyChainsText.value = nodeChainsToText(value.node_proxy_chains || {})
    regexText.value = (value.filter_regex || []).join('\n')
    nodeSearch.value = ''
    renameSearch.value = ''
    manualNodeLinks.value = ''
    nameEdited.value = true
    lastAutoName.value = ''
    featureManual.value = (value.manual_nodes || []).length > 0
    featureRegex.value = (value.filter_regex || []).length > 0
    featureRefine.value =
      (value.include_node_names || []).length > 0 || (value.exclude_node_names || []).length > 0
    featureRename.value = Object.keys(value.node_renames || {}).length > 0
    featureChain.value =
      (value.proxy_chain || []).length > 0 || Object.keys(value.node_proxy_chains || {}).length > 0
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
    : (form.value.raw_nodes || [])
  // When using raw_nodes fallback, names may already include prefix/rename.
  // Prefer source_nodes after fetch for correct selection/rename keys.
  return uniqueNodesByName([...(upstream || []), ...(form.value.manual_nodes || [])])
})

const regexPatterns = computed(() => {
  if (!featureRegex.value) return []
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
  if (!featureRegex.value || !regexPatterns.value.length) return candidateNodes.value
  return candidateNodes.value.filter((node) => regexPatterns.value.some((p) => p.test(nodeName(node))))
})

const selectedOriginalNodes = computed(() => {
  const selected = new Map(coarseNodes.value.map((node) => [nodeName(node), node]))
  if (featureRefine.value) {
    const include = new Set(form.value.include_node_names || [])
    const exclude = new Set(form.value.exclude_node_names || [])
    for (const node of candidateNodes.value) {
      const name = nodeName(node)
      if (include.has(name)) selected.set(name, node)
    }
    for (const name of exclude) selected.delete(name)
  }
  return [...selected.values()]
})

const effectivePrefix = computed(() => {
  const custom = (form.value.node_prefix || '').trim()
  if (custom) return custom
  if (form.value.is_primary) return ''
  return (form.value.name || '').trim()
})

const prefixedPreviewNodes = computed(() => {
  const prefix = effectivePrefix.value
  return selectedOriginalNodes.value
    .map((node) => {
      const original = nodeName(node)
      if (!original) return null
      const prefixed = prefix ? `${prefix}-${original}` : original
      return { original, prefixed, node }
    })
    .filter(Boolean)
})

const finalPreviewNames = computed(() => {
  const renames = featureRename.value ? form.value.node_renames || {} : {}
  return prefixedPreviewNodes.value.map((item) => {
    const mapped = String(renames[item.prefixed] || '').trim()
    return mapped || item.prefixed
  })
})

const renameCount = computed(() =>
  Object.entries(form.value.node_renames || {}).filter(([k, v]) => k && v && k !== v).length
)

const saveDisabled = computed(() => {
  return !!regexError.value || !form.value.name.trim() || !form.value.url.trim()
})

const visibleCandidateNodes = computed(() => {
  const q = nodeSearch.value.trim().toLowerCase()
  const nodes = candidateNodes.value
  if (!q) return nodes.slice(0, 240)
  return nodes.filter((node) => nodeName(node).toLowerCase().includes(q)).slice(0, 240)
})

const visiblePrefixedNodes = computed(() => {
  const q = renameSearch.value.trim().toLowerCase()
  const nodes = prefixedPreviewNodes.value
  if (!q) return nodes.slice(0, 240)
  return nodes
    .filter((item) => {
      const to = String(form.value.node_renames[item.prefixed] || item.prefixed).toLowerCase()
      return item.prefixed.toLowerCase().includes(q) || to.includes(q)
    })
    .slice(0, 240)
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

function setRename(prefixed, value) {
  const next = { ...(form.value.node_renames || {}) }
  const target = String(value || '').trim()
  if (!target || target === prefixed) delete next[prefixed]
  else next[prefixed] = target
  form.value.node_renames = next
}

function clearRenames() {
  form.value.node_renames = {}
}

function onNameInput() {
  nameEdited.value = true
}

function onUrlInput() {
  maybeAutofillNameFromUrl(form.value.url)
}

function nameFromUrl(url) {
  try {
    const parsed = new URL(url)
    let host = (parsed.hostname || '').trim().toLowerCase()
    if (!host) return ''
    if (host.startsWith('www.')) host = host.slice(4)
    const parts = host.split('.').filter(Boolean)
    const name = parts.length >= 2 ? parts[parts.length - 2] : host
    if (!name) return ''
    return name.charAt(0).toUpperCase() + name.slice(1)
  } catch {
    return ''
  }
}

function maybeAutofillNameFromUrl(url) {
  if (!url) return
  const current = (form.value.name || '').trim()
  if (nameEdited.value && current && current !== lastAutoName.value) return
  const auto = nameFromUrl(url)
  if (!auto) return
  form.value.name = auto
  lastAutoName.value = auto
  nameEdited.value = false
}

async function handleFetch() {
  if (!form.value.id || fetching.value) return
  fetching.value = true
  fetchError.value = ''
  try {
    const res = await fetchSubscription(form.value.id)
    const data = res.data || {}
    form.value.source_nodes = data.source_nodes || []
    form.value.raw_nodes = data.raw_nodes || []
    form.value.manual_nodes = data.manual_nodes || form.value.manual_nodes || []
    form.value.filter_regex = data.filter_regex || form.value.filter_regex || []
    form.value.include_node_names = data.include_node_names || form.value.include_node_names || []
    form.value.exclude_node_names = data.exclude_node_names || form.value.exclude_node_names || []
    form.value.node_renames = { ...(data.node_renames || form.value.node_renames || {}) }
    form.value.is_primary = !!data.is_primary
    form.value.node_prefix = data.node_prefix || form.value.node_prefix || ''
    regexText.value = (form.value.filter_regex || []).join('\n')
    emit('fetched', data)
  } catch (err) {
    fetchError.value = getApiErrorMessage(err, '拉取订阅失败')
  } finally {
    fetching.value = false
  }
}

function handleSave() {
  if (saveDisabled.value) return
  // Feature chips only hide UI. Closing a chip no longer wipes saved config.
  const payload = {
    name: form.value.name?.trim(),
    url: form.value.url?.trim(),
    update_interval: form.value.update_interval || null,
    node_prefix: form.value.node_prefix?.trim() || null,
    filter_regex: form.value.filter_regex || [],
    include_node_names: form.value.include_node_names || [],
    exclude_node_names: form.value.exclude_node_names || [],
    node_renames: normalizeRenames(form.value.node_renames || {}),
    proxy_chain: parseProxyChainText(proxyChainText.value),
    node_proxy_chains: parseNodeProxyChainsText(nodeProxyChainsText.value),
    manual_nodes: form.value.manual_nodes || [],
    manual_node_links: manualNodeLinks.value.trim() || null,
  }
  emit('save', payload)
}

function chainToText(chain) {
  return (chain || []).map((item) => String(item || '').trim()).filter(Boolean).join('\n')
}

function nodeChainsToText(mapping) {
  const lines = []
  for (const [key, value] of Object.entries(mapping || {})) {
    const name = String(key || '').trim()
    if (!name) continue
    if (Array.isArray(value)) {
      lines.push(`${name}=${value.map((v) => String(v || '').trim()).filter(Boolean).join(',')}`)
    } else if (value == null) {
      continue
    } else {
      lines.push(`${name}=${String(value).trim()}`)
    }
  }
  return lines.join('\n')
}

function parseProxyChainText(text) {
  const raw = String(text || '')
  const parts = raw
    .split(/[\n,，]/)
    .map((item) => item.trim())
    .filter(Boolean)
  const out = []
  for (const part of parts) {
    if (out.length && out[out.length - 1] === part) continue
    out.push(part)
  }
  return out
}

function parseNodeProxyChainsText(text) {
  const out = {}
  for (const line of String(text || '').split('\n')) {
    const trimmed = line.trim()
    if (!trimmed) continue
    const eq = trimmed.indexOf('=')
    if (eq < 0) continue
    const name = trimmed.slice(0, eq).trim()
    if (!name) continue
    const hopPart = trimmed.slice(eq + 1).trim()
    if (!hopPart) {
      out[name] = []
      continue
    }
    out[name] = hopPart
      .split(/[,，]/)
      .map((item) => item.trim())
      .filter(Boolean)
  }
  return out
}

function normalizeRenames(renames) {
  const out = {}
  for (const [key, value] of Object.entries(renames || {})) {
    const from = String(key || '').trim()
    const to = String(value || '').trim()
    if (!from || !to || from === to) continue
    out[from] = to
  }
  return out
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
    node_renames: {},
    proxy_chain: [],
    node_proxy_chains: {},
    manual_nodes: [],
    source_nodes: [],
    raw_nodes: [],
  }
}
</script>
