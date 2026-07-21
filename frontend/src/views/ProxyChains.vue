<template>
  <section class="page">
    <div class="page-head">
      <div>
        <p class="eyebrow">Proxy Chain</p>
        <h2>链式代理</h2>
        <p class="page-desc">
          在订阅 / 策略组 / 节点都配置完后，再给出口挂跳板。跳板可以是节点或策略组。
          优先级：节点 &gt; 策略组 &gt; 订阅。
        </p>
      </div>
      <button class="primary" @click="reload" :disabled="loading">刷新</button>
    </div>

    <div v-if="error" class="alert error">{{ error }}</div>

    <div class="dns-section">
      <h3>新增绑定</h3>
      <div class="form-grid">
        <label class="field">
          <span>目标类型</span>
          <select v-model="form.target_type" @change="onTargetTypeChange">
            <option value="node">节点</option>
            <option value="node_group">策略组</option>
            <option value="subscription">订阅</option>
          </select>
        </label>

        <label v-if="form.target_type === 'node'" class="field">
          <span>目标节点</span>
          <input v-model="nodeSearch" placeholder="搜索节点名" class="search-input" />
          <select v-model="form.target_name">
            <option value="">选择最终节点</option>
            <option v-for="n in filteredTargetNodes" :key="n.name" :value="n.name">
              {{ n.name }}
              <template v-if="n.subscription_name">({{ n.subscription_name }})</template>
            </option>
          </select>
        </label>

        <label v-else-if="form.target_type === 'node_group'" class="field">
          <span>目标策略组</span>
          <input v-model="groupSearch" placeholder="搜索策略组" class="search-input" />
          <select v-model.number="form.target_id">
            <option :value="null">选择策略组</option>
            <option v-for="g in filteredGroups" :key="g.id" :value="g.id">{{ g.name }}</option>
          </select>
        </label>

        <label v-else class="field">
          <span>目标订阅</span>
          <select v-model.number="form.target_id">
            <option :value="null">选择订阅</option>
            <option v-for="s in subscriptions" :key="s.id" :value="s.id">{{ s.name }}</option>
          </select>
        </label>

        <label class="field">
          <span>跳板类型</span>
          <select v-model="form.dialer_type">
            <option value="node">节点</option>
            <option value="node_group">策略组</option>
          </select>
        </label>

        <label v-if="form.dialer_type === 'node'" class="field">
          <span>跳板节点</span>
          <input v-model="dialerNodeSearch" placeholder="搜索跳板节点" class="search-input" />
          <select v-model="form.dialer_ref">
            <option value="">选择节点</option>
            <option v-for="n in filteredDialerNodes" :key="'d-' + n.name" :value="n.name">{{ n.name }}</option>
          </select>
        </label>
        <label v-else class="field">
          <span>跳板策略组</span>
          <input v-model="dialerGroupSearch" placeholder="搜索跳板组" class="search-input" />
          <select v-model="form.dialer_ref">
            <option value="">选择策略组</option>
            <option v-for="g in filteredDialerGroups" :key="'dg-' + g.id" :value="g.name">{{ g.name }}</option>
          </select>
        </label>

        <label class="field">
          <span>备注</span>
          <input v-model="form.note" placeholder="可选" />
        </label>
      </div>

      <div v-if="preview" class="preview-box">
        <strong>生效预览</strong>
        <p class="section-hint">
          目标 {{ preview.target_count }} 个 · 将挂链
          <b>{{ preview.chain_count }}</b>
          · 跳过自环/成员
          <b>{{ preview.skip_count }}</b>
        </p>
        <div v-if="preview.chain_samples?.length" class="muted small-line">
          挂链样例：{{ preview.chain_samples.slice(0, 5).join('、') }}
        </div>
        <div v-if="preview.skip_samples?.length" class="muted small-line">
          跳过样例：{{ preview.skip_samples.slice(0, 5).join('、') }}
        </div>
      </div>

      <div class="template-actions" style="margin-top: 12px">
        <button @click="runPreview" :disabled="!canCreate || previewing">
          {{ previewing ? '预览中…' : '预览生效' }}
        </button>
        <button class="primary" @click="createBinding" :disabled="saving || !canCreate">
          {{ saving ? '保存中…' : '添加绑定' }}
        </button>
      </div>
    </div>

    <div class="dns-section">
      <div class="row space">
        <h3>已有绑定</h3>
        <span class="muted">{{ bindings.length }} 条</span>
      </div>
      <div v-if="!bindings.length" class="empty-mini">还没有链式绑定。先配完节点和策略组，再来这里挂跳板。</div>
      <div v-else class="table-like">
        <div v-for="item in bindings" :key="item.id" class="table-row chain-row">
          <div class="chain-main">
            <span class="pill">{{ targetLabel(item) }}</span>
            <span class="muted">→</span>
            <span class="pill secondary">{{ dialerLabel(item) }}</span>
            <span v-if="!item.enabled" class="pill danger">已禁用</span>
            <span v-if="item.note" class="muted">{{ item.note }}</span>
          </div>
          <div class="action-row compact-actions">
            <button @click="toggleEnabled(item)">{{ item.enabled ? '禁用' : '启用' }}</button>
            <button class="danger" @click="removeBinding(item)">删除</button>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  createProxyChain,
  deleteProxyChain,
  getApiErrorMessage,
  getFinalNodes,
  getNodeGroups,
  getProxyChains,
  getSubscriptions,
  previewProxyChain,
  updateProxyChain,
} from '../api'

const loading = ref(false)
const saving = ref(false)
const previewing = ref(false)
const error = ref('')
const bindings = ref([])
const finalNodes = ref([])
const nodeGroups = ref([])
const subscriptions = ref([])
const preview = ref(null)

const nodeSearch = ref('')
const groupSearch = ref('')
const dialerNodeSearch = ref('')
const dialerGroupSearch = ref('')

const form = reactive({
  target_type: 'node',
  target_id: null,
  target_name: '',
  dialer_type: 'node',
  dialer_ref: '',
  note: '',
  enabled: true,
})

const canCreate = computed(() => {
  if (!form.dialer_ref) return false
  if (form.target_type === 'node') return !!form.target_name
  return form.target_id != null
})

const filteredTargetNodes = computed(() => filterNodes(finalNodes.value, nodeSearch.value))
const filteredDialerNodes = computed(() => filterNodes(finalNodes.value, dialerNodeSearch.value))
const filteredGroups = computed(() => filterGroups(nodeGroups.value, groupSearch.value))
const filteredDialerGroups = computed(() => filterGroups(nodeGroups.value, dialerGroupSearch.value))

onMounted(reload)

watch(
  () => [form.target_type, form.target_id, form.target_name, form.dialer_type, form.dialer_ref],
  () => {
    preview.value = null
  },
)

async function reload() {
  loading.value = true
  error.value = ''
  try {
    const [b, n, g, s] = await Promise.all([
      getProxyChains(),
      getFinalNodes(),
      getNodeGroups(),
      getSubscriptions(),
    ])
    bindings.value = b.data || []
    finalNodes.value = n.data || []
    nodeGroups.value = g.data || []
    subscriptions.value = s.data || []
  } catch (err) {
    error.value = getApiErrorMessage(err, '加载失败')
  } finally {
    loading.value = false
  }
}

function filterNodes(list, q) {
  const query = String(q || '').trim().toLowerCase()
  if (!query) return list
  return (list || []).filter((n) => String(n.name || '').toLowerCase().includes(query))
}

function filterGroups(list, q) {
  const query = String(q || '').trim().toLowerCase()
  if (!query) return list
  return (list || []).filter((g) => String(g.name || '').toLowerCase().includes(query))
}

function onTargetTypeChange() {
  form.target_id = null
  form.target_name = ''
  preview.value = null
}

function targetLabel(item) {
  if (item.target_type === 'node') return `节点: ${item.target_name || '?'}`
  if (item.target_type === 'node_group') return `策略组: ${item.target_name || item.target_id}`
  return `订阅: ${item.target_name || item.target_id}`
}

function dialerLabel(item) {
  const kind = item.dialer_type === 'node_group' ? '组' : '节点'
  return `跳板(${kind}): ${item.dialer_ref}`
}

function buildPayload() {
  const payload = {
    target_type: form.target_type,
    dialer_type: form.dialer_type,
    dialer_ref: form.dialer_ref,
    enabled: true,
    note: form.note || null,
  }
  if (form.target_type === 'node') {
    payload.target_name = form.target_name
  } else {
    payload.target_id = form.target_id
  }
  return payload
}

async function runPreview() {
  if (!canCreate.value || previewing.value) return
  previewing.value = true
  error.value = ''
  try {
    const { data } = await previewProxyChain(buildPayload())
    preview.value = data
  } catch (err) {
    error.value = getApiErrorMessage(err, '预览失败')
    preview.value = null
  } finally {
    previewing.value = false
  }
}

async function createBinding() {
  if (!canCreate.value || saving.value) return
  saving.value = true
  error.value = ''
  try {
    await createProxyChain(buildPayload())
    form.target_name = ''
    form.target_id = null
    form.dialer_ref = ''
    form.note = ''
    preview.value = null
    await reload()
  } catch (err) {
    error.value = getApiErrorMessage(err, '创建失败')
  } finally {
    saving.value = false
  }
}

async function toggleEnabled(item) {
  try {
    await updateProxyChain(item.id, { enabled: !item.enabled })
    await reload()
  } catch (err) {
    error.value = getApiErrorMessage(err, '更新失败')
  }
}

async function removeBinding(item) {
  try {
    await deleteProxyChain(item.id)
    await reload()
  } catch (err) {
    error.value = getApiErrorMessage(err, '删除失败')
  }
}
</script>

<style scoped>
.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 12px;
}
.search-input {
  margin-bottom: 6px;
}
.preview-box {
  margin-top: 12px;
  padding: 10px 12px;
  border-radius: 10px;
  border: 1px solid var(--border, #3333);
  background: color-mix(in srgb, var(--primary, #4f8cff) 8%, transparent);
}
.chain-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 10px 0;
  border-bottom: 1px solid var(--border, #3333);
}
.chain-main {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}
.pill {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--primary, #4f8cff) 18%, transparent);
  font-size: 12px;
}
.pill.secondary {
  background: color-mix(in srgb, #22c55e 18%, transparent);
}
.pill.danger {
  background: color-mix(in srgb, #ef4444 22%, transparent);
}
.table-like {
  margin-top: 8px;
}
.small-line {
  font-size: 12px;
  margin-top: 4px;
}
</style>
