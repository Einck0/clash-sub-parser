<template>
  <div class="manual-node-editor subscription-form-card">
    <div class="form-header">
      <div>
        <p class="eyebrow">Manual Node</p>
        <h3>添加自定义节点</h3>
        <p class="section-hint">表单模式按协议填写，Raw 模式继续支持分享链接、Base64 和 YAML。</p>
      </div>
      <button type="button" @click="$emit('cancel')">关闭</button>
    </div>

    <div class="grid-2">
      <label>
        <div class="muted">节点或订阅名称</div>
        <input v-model="name" placeholder="例如：我的 WARP 节点" />
      </label>
      <label>
        <div class="muted">节点前缀（可选）</div>
        <input v-model="nodePrefix" placeholder="留空则使用名称" />
      </label>
    </div>

    <div class="mode-tabs" role="tablist" aria-label="节点添加模式">
      <button type="button" class="feature-chip" :class="{ active: mode === 'form' }" @click="mode = 'form'">
        按协议填写
      </button>
      <button type="button" class="feature-chip" :class="{ active: mode === 'raw' }" @click="mode = 'raw'">
        Raw 模式
      </button>
    </div>

    <template v-if="mode === 'form'">
      <div class="selector-section">
        <div class="row space">
          <div>
            <strong>节点类型</strong>
            <p class="section-hint">先填一条节点，保存后可以继续在 Raw 模式导入复杂配置。</p>
          </div>
          <select v-model="draft.type" aria-label="节点类型">
            <option v-for="item in protocolOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
          </select>
        </div>

        <div class="grid-2">
          <label>
            <div class="muted">节点名称</div>
            <input v-model="draft.name" placeholder="例如：香港 01" />
          </label>
          <label>
            <div class="muted">服务器</div>
            <input v-model="draft.server" placeholder="example.com 或 IP" />
          </label>
          <label>
            <div class="muted">端口</div>
            <input v-model.number="draft.port" type="number" min="1" max="65535" placeholder="443" />
          </label>
          <label v-if="draft.type === 'ss'">
            <div class="muted">加密方式</div>
            <input v-model="draft.cipher" placeholder="例如：chacha20-ietf-poly1305" />
          </label>
          <label v-if="draft.type === 'ss'">
            <div class="muted">密码</div>
            <input v-model="draft.password" type="password" autocomplete="new-password" />
          </label>
          <label v-if="draft.type === 'trojan' || draft.type === 'vless'">
            <div class="muted">{{ draft.type === 'vless' ? 'UUID' : '密码' }}</div>
            <input v-model="draft[draft.type === 'vless' ? 'uuid' : 'password']" :type="draft.type === 'vless' ? 'text' : 'password'" />
          </label>
          <label v-if="draft.type === 'vmess' || draft.type === 'vless'">
            <div class="muted">UUID</div>
            <input v-model="draft.uuid" />
          </label>
          <label v-if="draft.type === 'vmess'">
            <div class="muted">加密方式</div>
            <select v-model="draft.cipher">
              <option value="auto">auto</option>
              <option value="aes-128-gcm">aes-128-gcm</option>
              <option value="chacha20-poly1305">chacha20-poly1305</option>
            </select>
          </label>
          <label v-if="draft.type === 'vmess'">
            <div class="muted">alterId</div>
            <input v-model.number="draft.alterId" type="number" min="0" />
          </label>
          <label v-if="draft.type === 'wireguard'">
            <div class="muted">私钥</div>
            <input v-model="draft['private-key']" type="password" autocomplete="new-password" />
          </label>
          <label v-if="draft.type === 'wireguard'">
            <div class="muted">隧道 IP</div>
            <input v-model="draft.ip" placeholder="例如：172.16.0.2/32" />
          </label>
        </div>

        <details class="manual-node-advanced">
          <summary>高级参数</summary>
          <div class="grid-2">
            <label v-if="draft.type === 'trojan' || draft.type === 'vless' || draft.type === 'vmess'">
              <div class="muted">传输协议</div>
              <select v-model="draft.network">
                <option value="tcp">TCP</option>
                <option value="ws">WebSocket</option>
                <option value="grpc">gRPC</option>
                <option v-if="draft.type === 'vmess'" value="h2">HTTP/2</option>
              </select>
            </label>
            <label v-if="draft.type === 'vless'">
              <div class="muted">安全层</div>
              <select v-model="draft.security">
                <option value="none">无</option>
                <option value="tls">TLS</option>
                <option value="reality">Reality</option>
              </select>
            </label>
            <label v-if="draft.type === 'trojan' || draft.type === 'vless' || draft.type === 'vmess'">
              <div class="muted">SNI</div>
              <input v-if="draft.type === 'trojan'" v-model="draft.sni" placeholder="可选" />
              <input v-else v-model="draft.servername" placeholder="可选" />
            </label>
            <label v-if="draft.type === 'vless'">
              <div class="muted">Flow</div>
              <input v-model="draft.flow" placeholder="例如：xtls-rprx-vision" />
            </label>
            <label v-if="draft.type === 'vless'">
              <div class="muted">客户端指纹</div>
              <input v-model="draft['client-fingerprint']" placeholder="例如：chrome" />
            </label>
            <label v-if="draft.type === 'vless' && draft.security === 'reality'">
              <div class="muted">Reality 公钥</div>
              <input v-model="draft['reality-public-key']" />
            </label>
            <label v-if="draft.type === 'vless' && draft.security === 'reality'">
              <div class="muted">Reality Short ID</div>
              <input v-model="draft['reality-short-id']" />
            </label>
            <label v-if="draft.type === 'trojan' || draft.type === 'vless' || draft.type === 'vmess'">
              <div class="muted">WebSocket 或 HTTP/2 路径</div>
              <input v-model="draft.path" placeholder="可选" />
            </label>
            <label v-if="draft.type === 'trojan' || draft.type === 'vless' || draft.type === 'vmess'">
              <div class="muted">Host</div>
              <input v-model="draft.host" placeholder="可选" />
            </label>
            <label v-if="draft.type === 'vless' || draft.type === 'vmess'">
              <div class="muted">gRPC 服务名</div>
              <input v-model="draft.grpcServiceName" placeholder="可选" />
            </label>
            <label v-if="draft.type === 'wireguard'">
              <div class="muted">IPv6</div>
              <input v-model="draft.ipv6" placeholder="可选" />
            </label>
            <label v-if="draft.type === 'wireguard'">
              <div class="muted">对端公钥</div>
              <input v-model="draft['public-key']" />
            </label>
            <label v-if="draft.type === 'wireguard'">
              <div class="muted">MTU</div>
              <input v-model.number="draft.mtu" type="number" min="576" />
            </label>
            <label v-if="draft.type === 'wireguard'">
              <div class="muted">Reserved</div>
              <input v-model="draft.reserved" placeholder="例如：1,2,3" />
            </label>
            <label v-if="draft.type === 'trojan' || draft.type === 'vless'" class="switch-line">
              <input v-model="draft['skip-cert-verify']" type="checkbox" /> 跳过证书校验
            </label>
            <label v-if="draft.type === 'vless' || draft.type === 'vmess'" class="switch-line">
              <input v-model="draft.tls" type="checkbox" /> 启用 TLS
            </label>
          </div>
        </details>

        <div v-if="formError" class="form-alert form-alert-error">{{ formError }}</div>
        <div class="form-footer">
          <button type="button" class="primary" @click="addDraftNode">加入待保存列表</button>
          <span class="muted">已添加 {{ draftNodes.length }} 个</span>
        </div>
      </div>

      <div v-if="draftNodes.length" class="selector-section">
        <div class="row space">
          <strong>待保存节点</strong>
          <span class="muted">拖动左侧手柄调整顺序</span>
        </div>
        <div class="node-select-list">
          <div
            v-for="(node, index) in draftNodes"
            :key="node._id"
            class="node-select-row"
            :class="{ dragging: draggingIndex === index }"
            @dragover.prevent
            @drop="dropDraftNode(index)"
          >
            <button
              type="button"
              class="drag-handle"
              data-drag-handle
              draggable="true"
              aria-label="拖动排序"
              @dragstart="startDraftDrag($event, index)"
              @dragend="draggingIndex = null"
            >⠿</button>
            <div class="node-select-name mono">
              <strong>{{ node.name }}</strong>
              <span>{{ node.type }} · {{ node.server }}:{{ node.port }}</span>
            </div>
            <button type="button" class="danger" @click="removeDraftNode(index)">移除</button>
          </div>
        </div>
      </div>
    </template>

    <label v-else style="display:block">
      <div class="muted">Raw 节点内容或链接文本</div>
      <textarea
        v-model="rawContent"
        class="secret-textarea"
        style="min-height:180px"
        placeholder="支持 ss://, trojan://, vless://, vmess://, wireguard://，或粘贴 Base64 / YAML 原始内容"
      ></textarea>
    </label>

    <p v-if="props.error" class="form-alert form-alert-error">{{ props.error }}</p>
    <div class="form-footer">
      <button type="button" class="primary" :disabled="props.saving" @click="submit">{{ props.saving ? '保存中...' : '保存并解析' }}</button>
      <button type="button" @click="$emit('cancel')">取消</button>
    </div>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue'
import yaml from 'js-yaml'

const props = defineProps({
  saving: { type: Boolean, default: false },
  error: { type: String, default: '' },
})
const emit = defineEmits(['save', 'cancel'])

const protocolOptions = [
  { value: 'ss', label: 'Shadowsocks' },
  { value: 'trojan', label: 'Trojan' },
  { value: 'vless', label: 'VLESS' },
  { value: 'vmess', label: 'VMess' },
  { value: 'wireguard', label: 'WireGuard' },
]

const mode = ref('form')
const name = ref('')
const nodePrefix = ref('')
const rawContent = ref('')
const formError = ref('')
const draggingIndex = ref(null)
const draftNodes = ref([])
const draft = ref(createDraft('ss'))

watch(() => props.error, (value) => {
  if (value) formError.value = ''
})
watch(() => draft.value.type, (type) => {
  draft.value = { ...createDraft(type), name: draft.value.name, server: draft.value.server, port: draft.value.port }
})

function createDraft(type) {
  return {
    _id: `${Date.now()}-${Math.random()}`,
    type,
    name: '',
    server: '',
    port: 443,
    cipher: type === 'vmess' ? 'auto' : '',
    password: '',
    uuid: '',
    alterId: 0,
    network: 'tcp',
    security: 'none',
    servername: '',
    sni: '',
    flow: '',
    'client-fingerprint': '',
    'reality-public-key': '',
    'reality-short-id': '',
    path: '',
    host: '',
    grpcServiceName: '',
    tls: false,
    'skip-cert-verify': false,
    'private-key': '',
    ip: '',
    ipv6: '',
    'public-key': '',
    mtu: 1420,
    reserved: '',
  }
}

function addDraftNode() {
  formError.value = ''
  const node = normalizeDraft(draft.value)
  const errorMessage = validateNode(node)
  if (errorMessage) {
    formError.value = errorMessage
    return
  }
  draftNodes.value.push({ ...node, _id: draft.value._id })
  draft.value = createDraft(node.type)
}

function normalizeDraft(value) {
  const node = { ...value }
  for (const key of Object.keys(node)) {
    if (key.startsWith('_')) continue
    if (node[key] === '' || node[key] === null || node[key] === undefined) delete node[key]
  }
  node.port = Number(value.port) || 443
  if (node.type === 'vmess') node.alterId = Number(value.alterId) || 0
  if (node.type === 'wireguard') {
    node.udp = true
    if (value.mtu) node.mtu = Number(value.mtu)
    if (value.reserved) node.reserved = String(value.reserved).split(',').map((item) => Number(item.trim())).filter(Number.isFinite)
  }
  if (node.type === 'vless') {
    if (value.security === 'tls' || value.security === 'reality') node.tls = true
    if (value.security === 'reality' && (value['reality-public-key'] || value['reality-short-id'])) {
      node['reality-opts'] = {}
      if (value['reality-public-key']) node['reality-opts']['public-key'] = value['reality-public-key']
      if (value['reality-short-id']) node['reality-opts']['short-id'] = value['reality-short-id']
    }
  }
  if (node.network === 'ws' && (value.path || value.host)) {
    node['ws-opts'] = {}
    if (value.path) node['ws-opts'].path = value.path
    if (value.host) node['ws-opts'].headers = { Host: value.host }
  }
  if ((node.network === 'grpc') && value.grpcServiceName) node['grpc-opts'] = { 'grpc-service-name': value.grpcServiceName }
  if (node.type === 'vmess' && (node.network === 'h2' || node.network === 'http') && (value.path || value.host)) {
    node['h2-opts'] = {}
    if (value.path) node['h2-opts'].path = value.path
    if (value.host) node['h2-opts'].host = [value.host]
  }
  delete node.security
  delete node['reality-public-key']
  delete node['reality-short-id']
  delete node.path
  delete node.host
  delete node.grpcServiceName
  return node
}

function validateNode(node) {
  if (!String(node.name || '').trim()) return '请填写节点名称'
  if (!String(node.server || '').trim()) return '请填写服务器地址'
  if (!Number.isInteger(node.port) || node.port < 1 || node.port > 65535) return '端口必须是 1 到 65535 的整数'
  if (node.type === 'ss' && (!node.cipher || !node.password)) return 'Shadowsocks 需要填写加密方式和密码'
  if (node.type === 'trojan' && !node.password) return 'Trojan 需要填写密码'
  if ((node.type === 'vless' || node.type === 'vmess') && !node.uuid) return `${node.type.toUpperCase()} 需要填写 UUID`
  if (node.type === 'wireguard' && (!node['private-key'] || !node.ip)) return 'WireGuard 需要填写私钥和隧道 IP'
  return ''
}

function startDraftDrag(event, index) {
  draggingIndex.value = index
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', String(index))
  }
}

function dropDraftNode(targetIndex) {
  const sourceIndex = draggingIndex.value
  draggingIndex.value = null
  if (sourceIndex === null || sourceIndex === targetIndex) return
  const next = [...draftNodes.value]
  const [item] = next.splice(sourceIndex, 1)
  next.splice(targetIndex, 0, item)
  draftNodes.value = next
}

function removeDraftNode(index) {
  draftNodes.value.splice(index, 1)
}

function serializableNodes() {
  return draftNodes.value.map((node) => {
    const copy = { ...node }
    delete copy._id
    return copy
  })
}

function submit() {
  formError.value = ''
  const content = mode.value === 'raw'
    ? rawContent.value.trim()
    : yaml.dump({ proxies: serializableNodes() }, { noCompatMode: true, lineWidth: -1, sortKeys: false })
  if (!name.value.trim()) {
    formError.value = '请填写节点或订阅名称'
    return
  }
  if (!content.trim() || (mode.value === 'form' && !draftNodes.value.length)) {
    formError.value = mode.value === 'form' ? '请至少加入一个节点' : '请填写 Raw 节点内容'
    return
  }
  emit('save', { name: name.value.trim(), node_prefix: nodePrefix.value.trim() || null, node_links: content })
}
</script>
