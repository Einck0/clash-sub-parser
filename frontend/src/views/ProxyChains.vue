<template>
  <section class="page chain-page">
    <div class="page-head">
      <div>
        <p class="eyebrow">Proxy Chain</p>
        <h2>链式代理</h2>
        <p class="page-desc">
          后置挂跳板。优先级：节点 &gt; 策略组 &gt; 订阅。单节点快捷操作请到「节点」。
        </p>
      </div>
      <div class="head-actions">
        <button @click="showComposer = !showComposer">
          {{ showComposer ? '收起新建' : '新建绑定' }}
        </button>
        <button class="primary" @click="reload" :disabled="loading">刷新</button>
      </div>
    </div>

    <div v-if="error" class="alert error">{{ error }}</div>

    <div class="stats-row">
      <div class="stat-chip">
        <strong>{{ bindings.length }}</strong>
        <span>绑定</span>
      </div>
      <div class="stat-chip">
        <strong>{{ enabledCount }}</strong>
        <span>启用</span>
      </div>
      <div class="stat-chip">
        <strong>{{ subscriptionBindCount }}</strong>
        <span>订阅级</span>
      </div>
      <div class="stat-chip">
        <strong>{{ groupBindCount }}</strong>
        <span>组级</span>
      </div>
      <div class="stat-chip">
        <strong>{{ nodeBindCount }}</strong>
        <span>节点级</span>
      </div>
    </div>

    <div v-if="showComposer" class="dns-section composer">
      <h3>新建绑定</h3>
      <p class="section-hint">先选「给谁挂」，再选「经谁出去」。保存前可预览会挂多少、跳过多少。</p>

      <div class="composer-steps">
        <div class="step-card">
          <div class="step-title">1. 目标（出口）</div>
          <div class="seg">
            <button
              v-for="opt in targetTypeOptions"
              :key="opt.value"
              type="button"
              :class="{ active: form.target_type === opt.value }"
              @click="setTargetType(opt.value)"
            >
              {{ opt.label }}
            </button>
          </div>

          <label v-if="form.target_type === 'node'" class="field">
            <span>目标节点</span>
            <input v-model="nodeSearch" placeholder="搜索节点名" />
            <select v-model="form.target_name">
              <option value="">选择最终节点</option>
              <option v-for="n in filteredTargetNodes" :key="n.name" :value="n.name">
                {{ n.name }}
                <template v-if="n.subscription_name"> · {{ n.subscription_name }}</template>
              </option>
            </select>
          </label>

          <label v-else-if="form.target_type === 'node_group'" class="field">
            <span>目标策略组</span>
            <input v-model="groupSearch" placeholder="搜索策略组" />
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
        </div>

        <div class="step-arrow" aria-hidden="true">→</div>

        <div class="step-card">
          <div class="step-title">2. 跳板（入口）</div>
          <div class="seg">
            <button
              type="button"
              :class="{ active: form.dialer_type === 'node' }"
              @click="form.dialer_type = 'node'; form.dialer_ref = ''"
            >
              节点
            </button>
            <button
              type="button"
              :class="{ active: form.dialer_type === 'node_group' }"
              @click="form.dialer_type = 'node_group'; form.dialer_ref = ''"
            >
              策略组
            </button>
          </div>

          <label v-if="form.dialer_type === 'node'" class="field">
            <span>跳板节点</span>
            <input v-model="dialerNodeSearch" placeholder="搜索跳板节点" />
            <select v-model="form.dialer_ref">
              <option value="">选择节点</option>
              <option v-for="n in filteredDialerNodes" :key="'d-' + n.name" :value="n.name">
                {{ n.name }}
              </option>
            </select>
          </label>
          <label v-else class="field">
            <span>跳板策略组</span>
            <input v-model="dialerGroupSearch" placeholder="搜索跳板组" />
            <select v-model="form.dialer_ref">
              <option value="">选择策略组</option>
              <option v-for="g in filteredDialerGroups" :key="'dg-' + g.id" :value="g.name">
                {{ g.name }}
              </option>
            </select>
          </label>

          <label class="field">
            <span>备注</span>
            <input v-model="form.note" placeholder="可选" />
          </label>
        </div>
      </div>

      <div class="template-actions composer-actions">
        <button @click="runPreview" :disabled="!canCreate || previewing">
          {{ previewing ? '预览中…' : '预览' }}
        </button>
        <button class="primary" @click="createBinding" :disabled="saving || !canCreate">
          {{ saving ? '保存中…' : '保存绑定' }}
        </button>
      </div>
    </div>

    <div v-if="showPreviewModal && preview" class="modal-backdrop" @click.self="closePreviewModal">
      <div class="modal preview-modal" role="dialog" aria-modal="true" aria-label="链式生效预览">
        <div class="row space preview-modal-head">
          <div>
            <p class="eyebrow">Chain Preview</p>
            <h3>生效预览</h3>
            <p class="section-hint">
              目标 {{ preview.target_count }} · 挂链 <b>{{ preview.chain_count }}</b> · 跳过
              <b>{{ preview.skip_count }}</b>
            </p>
          </div>
          <button @click="closePreviewModal">关闭</button>
        </div>
        <div v-if="preview.chain_samples?.length" class="sample-block">
          <strong>挂链样例</strong>
          <div class="sample-line">{{ preview.chain_samples.slice(0, 12).join('、') }}</div>
        </div>
        <div v-if="preview.skip_samples?.length" class="sample-block">
          <strong>跳过样例</strong>
          <div class="sample-line muted">{{ preview.skip_samples.slice(0, 12).join('、') }}</div>
        </div>
        <div v-if="!preview.chain_samples?.length && !preview.skip_samples?.length" class="empty-mini">
          没有可展示的样例节点。
        </div>
      </div>
    </div>

    <PageToolbar
      v-model="listSearch"
      placeholder="搜索目标 / 跳板 / 备注"
      :count-text="`${filteredBindings.length} / ${bindings.length} 条绑定`"
    />

    <div class="dns-section">
      <div class="row space filter-bar">
        <h3>绑定列表</h3>
      </div>

      <div v-if="!filteredBindings.length" class="empty-mini">
        {{ bindings.length ? '没有匹配的绑定。' : '还没有链式绑定。点「新建绑定」开始。' }}
      </div>

      <div v-else class="bind-list">
        <article
          v-for="item in filteredBindings"
          :key="item.id"
          class="bind-card"
          :class="{ disabled: !item.enabled }"
        >
          <div class="bind-flow">
            <div class="endpoint">
              <span class="endpoint-kind">{{ targetKind(item) }}</span>
              <strong class="endpoint-name">{{ targetName(item) }}</strong>
            </div>
            <div class="flow-mid">
              <span class="flow-arrow">via</span>
            </div>
            <div class="endpoint dialer">
              <span class="endpoint-kind">{{ dialerKind(item) }}</span>
              <strong class="endpoint-name">{{ item.dialer_ref }}</strong>
            </div>
          </div>

          <div class="bind-meta">
            <span v-if="!item.enabled" class="pill danger">已禁用</span>
            <span v-else class="pill ok">启用</span>
            <span v-if="item.note" class="muted note">{{ item.note }}</span>
          </div>

          <div class="action-row compact-actions">
            <button @click="toggleEnabled(item)">{{ item.enabled ? '禁用' : '启用' }}</button>
            <button class="danger" @click="removeBinding(item)">删除</button>
          </div>
        </article>
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
import PageToolbar from '../components/PageToolbar.vue'

const loading = ref(false)
const saving = ref(false)
const previewing = ref(false)
const error = ref('')
const showComposer = ref(false)
const bindings = ref([])
const finalNodes = ref([])
const nodeGroups = ref([])
const subscriptions = ref([])
const preview = ref(null)
const showPreviewModal = ref(false)
const listSearch = ref('')

const nodeSearch = ref('')
const groupSearch = ref('')
const dialerNodeSearch = ref('')
const dialerGroupSearch = ref('')

const targetTypeOptions = [
  { value: 'subscription', label: '订阅' },
  { value: 'node_group', label: '策略组' },
  { value: 'node', label: '节点' },
]

const form = reactive({
  target_type: 'subscription',
  target_id: null,
  target_name: '',
  dialer_type: 'node_group',
  dialer_ref: '',
  note: '',
  enabled: true,
})

const canCreate = computed(() => {
  if (!form.dialer_ref) return false
  if (form.target_type === 'node') return !!form.target_name
  return form.target_id != null
})

const enabledCount = computed(() => bindings.value.filter((b) => b.enabled).length)
const subscriptionBindCount = computed(
  () => bindings.value.filter((b) => b.target_type === 'subscription').length,
)
const groupBindCount = computed(
  () => bindings.value.filter((b) => b.target_type === 'node_group').length,
)
const nodeBindCount = computed(() => bindings.value.filter((b) => b.target_type === 'node').length)

const filteredTargetNodes = computed(() => filterNodes(finalNodes.value, nodeSearch.value))
const filteredDialerNodes = computed(() => filterNodes(finalNodes.value, dialerNodeSearch.value))
const filteredGroups = computed(() => filterGroups(nodeGroups.value, groupSearch.value))
const filteredDialerGroups = computed(() => filterGroups(nodeGroups.value, dialerGroupSearch.value))

const filteredBindings = computed(() => {
  const q = String(listSearch.value || '').trim().toLowerCase()
  if (!q) return bindings.value
  return bindings.value.filter((item) => {
    const hay = [
      item.target_type,
      item.target_name,
      item.target_id,
      item.dialer_type,
      item.dialer_ref,
      item.note,
    ]
      .filter((x) => x != null && x !== '')
      .join(' ')
      .toLowerCase()
    return hay.includes(q)
  })
})

onMounted(async () => {
  await reload()
  if (!bindings.value.length) showComposer.value = true
})

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

function setTargetType(value) {
  form.target_type = value
  form.target_id = null
  form.target_name = ''
  preview.value = null
}

function targetKind(item) {
  if (item.target_type === 'node') return '节点'
  if (item.target_type === 'node_group') return '策略组'
  return '订阅'
}

function targetName(item) {
  return item.target_name || String(item.target_id || '?')
}

function dialerKind(item) {
  return item.dialer_type === 'node_group' ? '策略组跳板' : '节点跳板'
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
    showPreviewModal.value = true
  } catch (err) {
    error.value = getApiErrorMessage(err, '预览失败')
    preview.value = null
    showPreviewModal.value = false
  } finally {
    previewing.value = false
  }
}

function closePreviewModal() {
  showPreviewModal.value = false
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
    showPreviewModal.value = false
    showComposer.value = false
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
.stats-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 12px 0;
}
.stat-chip {
  display: flex;
  flex-direction: column;
  min-width: 72px;
  padding: 8px 12px;
  border-radius: 12px;
  border: 1px solid var(--border, #3333);
  background: color-mix(in srgb, var(--primary, #4f8cff) 6%, transparent);
}
.stat-chip strong {
  font-size: 18px;
  line-height: 1.1;
}
.stat-chip span {
  font-size: 12px;
  opacity: 0.75;
}
.composer-steps {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  gap: 12px;
  align-items: stretch;
  margin-top: 12px;
}
.step-card {
  border: 1px solid var(--border, #3333);
  border-radius: 14px;
  padding: 12px;
  background: color-mix(in srgb, var(--surface, #fff) 90%, transparent);
}
.step-title {
  font-weight: 700;
  margin-bottom: 10px;
}
.step-arrow {
  align-self: center;
  opacity: 0.55;
  font-weight: 700;
}
.seg {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 10px;
}
.seg button {
  border-radius: 999px;
  min-height: 36px;
}
.seg button.active {
  background: color-mix(in srgb, var(--primary, #4f8cff) 22%, transparent);
  border-color: color-mix(in srgb, var(--primary, #4f8cff) 45%, transparent);
  font-weight: 700;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 8px;
}
.preview-modal {
  width: min(720px, 96vw);
  max-height: 90vh;
  overflow: auto;
  display: grid;
  gap: 12px;
}
.preview-modal-head h3 {
  margin: 0 0 4px;
}
.sample-block {
  display: grid;
  gap: 6px;
}
.sample-line {
  font-size: 12px;
  overflow-wrap: anywhere;
}
.composer-actions {
  margin-top: 12px;
}
.filter-bar {
  gap: 10px;
  flex-wrap: wrap;
}
.filter-input {
  min-width: min(100%, 240px);
  flex: 1;
}
.bind-list {
  display: grid;
  gap: 10px;
  margin-top: 10px;
}
.bind-card {
  border: 1px solid var(--border, #3333);
  border-radius: 14px;
  padding: 12px;
  display: grid;
  gap: 10px;
}
.bind-card.disabled {
  opacity: 0.72;
}
.bind-flow {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  gap: 8px;
  align-items: center;
}
.endpoint {
  min-width: 0;
  padding: 8px 10px;
  border-radius: 12px;
  background: color-mix(in srgb, var(--primary, #4f8cff) 10%, transparent);
}
.endpoint.dialer {
  background: color-mix(in srgb, #22c55e 12%, transparent);
}
.endpoint-kind {
  display: block;
  font-size: 11px;
  opacity: 0.7;
  margin-bottom: 2px;
}
.endpoint-name {
  display: block;
  overflow-wrap: anywhere;
  font-size: 13px;
}
.flow-mid {
  text-align: center;
  opacity: 0.65;
  font-size: 12px;
  font-weight: 700;
}
.bind-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}
.note {
  overflow-wrap: anywhere;
}
.pill {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 12px;
  background: color-mix(in srgb, var(--primary, #4f8cff) 16%, transparent);
}
.pill.ok {
  background: color-mix(in srgb, #22c55e 18%, transparent);
}
.pill.danger {
  background: color-mix(in srgb, #ef4444 22%, transparent);
}
@media (max-width: 760px) {
  .composer-steps {
    grid-template-columns: 1fr;
  }
  .step-arrow {
    display: none;
  }
  .bind-flow {
    grid-template-columns: 1fr;
  }
  .flow-mid {
    text-align: left;
  }
  .action-row {
    width: 100%;
  }
  .action-row button {
    flex: 1 1 auto;
    min-height: 40px;
  }
}
</style>
