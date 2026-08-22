<template>
  <section class="page nodes-page">
    <div class="page-head">
      <div>
        <p class="eyebrow">Node Management</p>
        <h2>节点管理</h2>
        <p class="page-desc">
          处理后最终节点：协议/地址、所属订阅与策略组、生效 dialer。可搜索筛选，并快捷设节点级跳板。
        </p>
      </div>
      <button class="primary" @click="reload" :disabled="loading">刷新</button>
    </div>

    <div v-if="error" class="alert error">{{ error }}</div>

    <div class="stats-row">
      <div class="stat-chip">
        <strong>{{ rows.length }}</strong>
        <span>节点</span>
      </div>
      <div class="stat-chip">
        <strong>{{ chainedCount }}</strong>
        <span>已挂链</span>
      </div>
      <div class="stat-chip">
        <strong>{{ subOptions.length }}</strong>
        <span>订阅</span>
      </div>
      <div class="stat-chip">
        <strong>{{ typeOptions.length }}</strong>
        <span>协议</span>
      </div>
    </div>

    <PageToolbar
      v-model="search"
      placeholder="搜索节点 / 订阅 / 地址 / 跳板 / 组"
      :count-text="`${filtered.length} / ${rows.length}`"
    >
      <template #filters>
        <select v-model="subFilter">
          <option value="">全部订阅</option>
          <option v-for="s in subOptions" :key="s" :value="s">{{ s }}</option>
        </select>
        <select v-model="typeFilter">
          <option value="">全部协议</option>
          <option v-for="t in typeOptions" :key="t" :value="t">{{ t }}</option>
        </select>
        <select v-model="chainFilter">
          <option value="all">全部链式</option>
          <option value="chained">仅已挂链</option>
          <option value="plain">仅未挂链</option>
        </select>
      </template>
    </PageToolbar>

    <div class="pager-card">
      <div class="action-row">
        <button :disabled="page <= 1" @click="page--">上一页</button>
        <span class="muted">第 {{ normalizedPage }} / {{ totalPages }} 页</span>
        <button :disabled="page >= totalPages" @click="page++">下一页</button>
        <span class="muted">每页</span>
        <select v-model.number="pageSize" class="page-size-select">
          <option :value="50">50</option>
          <option :value="100">100</option>
          <option :value="200">200</option>
        </select>
      </div>
    </div>

    <div v-if="loading && !rows.length" class="empty-mini">加载中…</div>
    <div v-else-if="!filtered.length" class="empty-mini">没有匹配的节点。</div>

    <div v-else class="node-list">
      <article v-for="item in pagedRows" :key="item.name" class="node-card">
        <div class="node-top">
          <div class="node-title mono">{{ item.name }}</div>
          <div class="badge-row">
            <span v-if="item.type" class="pill">{{ item.type }}</span>
            <span v-if="item.subscription_name" class="pill soft">{{ item.subscription_name }}</span>
            <span v-if="item.udp" class="pill soft">UDP</span>
            <span v-if="item.tls" class="pill soft">TLS</span>
          </div>
        </div>

        <div class="meta-grid">
          <div v-if="item.server" class="meta-item">
            <span class="meta-label">地址</span>
            <span class="meta-value mono">{{ item.server }}<template v-if="item.port">:{{ item.port }}</template></span>
          </div>
          <div v-if="item.cipher" class="meta-item">
            <span class="meta-label">加密</span>
            <span class="meta-value">{{ item.cipher }}</span>
          </div>
          <div v-if="item.network" class="meta-item">
            <span class="meta-label">传输</span>
            <span class="meta-value">{{ item.network }}</span>
          </div>
          <div v-if="item.sni" class="meta-item">
            <span class="meta-label">SNI</span>
            <span class="meta-value mono">{{ item.sni }}</span>
          </div>
        </div>

        <div v-if="item.group_names?.length" class="groups-line">
          <span class="meta-label">策略组</span>
          <div class="badge-row">
            <span v-for="g in item.group_names.slice(0, 8)" :key="g" class="pill soft">{{ g }}</span>
            <span v-if="item.group_names.length > 8" class="muted">+{{ item.group_names.length - 8 }}</span>
          </div>
        </div>

        <div class="chain-line">
          <template v-if="item.dialer_proxy">
            <span class="pill ok">dialer → {{ item.dialer_proxy }}</span>
            <span v-if="item.chain_source" class="pill">来源: {{ sourceLabel(item.chain_source) }}</span>
          </template>
          <span v-else class="muted">未挂链</span>
        </div>

        <div class="action-row compact-actions">
          <button @click="openChain(item)">设跳板</button>
          <button
            v-if="item.dialer_proxy && item.chain_source === 'node'"
            class="danger"
            @click="clearNodeChain(item)"
          >
            清节点链
          </button>
        </div>
      </article>
    </div>

    <div v-if="chainTarget" class="modal-mask" @click.self="closeChain">
      <div class="modal-card">
        <h3>给节点设跳板</h3>
        <p class="section-hint mono target-name">{{ chainTarget.name }}</p>

        <div class="seg">
          <button
            type="button"
            :class="{ active: chainForm.dialer_type === 'node' }"
            @click="chainForm.dialer_type = 'node'; chainForm.dialer_ref = ''"
          >
            节点
          </button>
          <button
            type="button"
            :class="{ active: chainForm.dialer_type === 'node_group' }"
            @click="chainForm.dialer_type = 'node_group'; chainForm.dialer_ref = ''"
          >
            策略组
          </button>
        </div>

        <label v-if="chainForm.dialer_type === 'node'" class="field">
          <span>跳板节点</span>
          <input v-model="dialerSearch" placeholder="搜索" />
          <select v-model="chainForm.dialer_ref">
            <option value="">选择节点</option>
            <option v-for="n in dialerNodeOptions" :key="n.name" :value="n.name">{{ n.name }}</option>
          </select>
        </label>
        <label v-else class="field">
          <span>跳板策略组</span>
          <input v-model="dialerSearch" placeholder="搜索" />
          <select v-model="chainForm.dialer_ref">
            <option value="">选择策略组</option>
            <option v-for="g in dialerGroupOptions" :key="g.id" :value="g.name">{{ g.name }}</option>
          </select>
        </label>

        <div class="template-actions sheet-actions">
          <button class="primary" @click="saveChain" :disabled="saving || !chainForm.dialer_ref">
            {{ saving ? '保存中…' : '保存节点绑定' }}
          </button>
          <button @click="closeChain">取消</button>
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
  getNodeGroups,
  getNodeLedger,
  getProxyChains,
} from '../api'
import PageToolbar from '../components/PageToolbar.vue'
import { useAppStore } from '../stores/app'

const store = useAppStore()

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const rows = ref([])
const bindings = ref([])
const nodeGroups = ref([])
const search = ref('')
const subFilter = ref('')
const typeFilter = ref('')
const chainFilter = ref('all')
const page = ref(1)
const pageSize = ref(50)
const chainTarget = ref(null)
const dialerSearch = ref('')
const chainForm = reactive({
  dialer_type: 'node',
  dialer_ref: '',
})

const subOptions = computed(() => {
  const set = new Set()
  for (const row of rows.value) {
    if (row.subscription_name) set.add(row.subscription_name)
  }
  return [...set].sort((a, b) => a.localeCompare(b, 'zh'))
})

const typeOptions = computed(() => {
  const set = new Set()
  for (const row of rows.value) {
    if (row.type) set.add(row.type)
  }
  return [...set].sort()
})

const chainedCount = computed(() => rows.value.filter((r) => r.dialer_proxy).length)

const totalPages = computed(() => Math.max(1, Math.ceil(filtered.value.length / pageSize.value)))
const normalizedPage = computed(() => Math.min(page.value, totalPages.value))
const pagedRows = computed(() =>
  filtered.value.slice((normalizedPage.value - 1) * pageSize.value, normalizedPage.value * pageSize.value),
)
watch([search, subFilter, typeFilter, chainFilter, pageSize], () => { page.value = 1 })

const filtered = computed(() => {
  const q = String(search.value || '').trim().toLowerCase()
  return (rows.value || []).filter((item) => {
    if (subFilter.value && item.subscription_name !== subFilter.value) return false
    if (typeFilter.value && item.type !== typeFilter.value) return false
    if (chainFilter.value === 'chained' && !item.dialer_proxy) return false
    if (chainFilter.value === 'plain' && item.dialer_proxy) return false
    if (!q) return true
    const hay = [
      item.name,
      item.subscription_name,
      item.dialer_proxy,
      item.type,
      item.server,
      item.cipher,
      item.network,
      item.sni,
      ...(item.group_names || []),
    ]
      .filter(Boolean)
      .join(' ')
      .toLowerCase()
    return hay.includes(q)
  })
})

const dialerNodeOptions = computed(() => {
  const q = String(dialerSearch.value || '').trim().toLowerCase()
  const list = (rows.value || []).filter((n) => n.name !== chainTarget.value?.name)
  if (!q) return list
  return list.filter((n) => String(n.name || '').toLowerCase().includes(q))
})

const dialerGroupOptions = computed(() => {
  const q = String(dialerSearch.value || '').trim().toLowerCase()
  const list = nodeGroups.value || []
  if (!q) return list
  return list.filter((g) => String(g.name || '').toLowerCase().includes(q))
})

onMounted(reload)

function sourceLabel(src) {
  if (src === 'node') return '节点绑定'
  if (src === 'node_group') return '策略组'
  if (src === 'subscription') return '订阅'
  return src || ''
}

async function reload() {
  loading.value = true
  error.value = ''
  try {
    const [ledger, chains, groups] = await Promise.all([
      getNodeLedger(),
      getProxyChains(),
      getNodeGroups(),
    ])
    rows.value = ledger.data || []
    bindings.value = chains.data || []
    nodeGroups.value = groups.data || []
  } catch (err) {
    error.value = getApiErrorMessage(err, '加载节点失败')
  } finally {
    loading.value = false
  }
}

function openChain(item) {
  chainTarget.value = item
  chainForm.dialer_type = 'node'
  chainForm.dialer_ref = ''
  dialerSearch.value = ''
  error.value = ''
}

function closeChain() {
  chainTarget.value = null
  chainForm.dialer_ref = ''
}

async function saveChain() {
  if (!chainTarget.value || !chainForm.dialer_ref || saving.value) return
  saving.value = true
  error.value = ''
  try {
    const existing = (bindings.value || []).filter(
      (b) => b.target_type === 'node' && b.target_name === chainTarget.value.name,
    )
    // 旧绑定可能已被其他会话删除，逐条失败不阻断保存
    await Promise.allSettled(existing.map((b) => deleteProxyChain(b.id)))
    await createProxyChain({
      target_type: 'node',
      target_name: chainTarget.value.name,
      dialer_type: chainForm.dialer_type,
      dialer_ref: chainForm.dialer_ref,
      enabled: true,
      note: 'from node management',
    })
    closeChain()
    await reload()
  } catch (err) {
    error.value = getApiErrorMessage(err, '设跳板失败')
  } finally {
    saving.value = false
  }
}

async function clearNodeChain(item) {
  const ok = await store.confirm({
    title: '清除节点跳板',
    message: `确定要清除节点「${item.name}」的跳板绑定吗？`,
    confirmText: '清除',
    danger: true,
  })
  if (!ok) return
  error.value = ''
  try {
    const existing = (bindings.value || []).filter(
      (b) => b.target_type === 'node' && b.target_name === item.name,
    )
    await Promise.allSettled(existing.map((b) => deleteProxyChain(b.id)))
    await reload()
  } catch (err) {
    error.value = getApiErrorMessage(err, '清除失败')
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
.filter-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 10px;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.count-line {
  margin-top: 10px;
  font-size: 12px;
}
.node-list {
  display: grid;
  gap: 10px;
}
.node-card {
  border: 1px solid var(--border, #3333);
  border-radius: 14px;
  padding: 12px;
  display: grid;
  gap: 10px;
}
.node-title {
  font-size: 13px;
  overflow-wrap: anywhere;
  font-weight: 700;
}
.badge-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 6px;
}
.meta-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 8px;
}
.meta-item {
  min-width: 0;
}
.meta-label {
  display: block;
  font-size: 11px;
  opacity: 0.65;
  margin-bottom: 2px;
}
.meta-value {
  display: block;
  font-size: 12px;
  overflow-wrap: anywhere;
}
.groups-line,
.chain-line {
  display: grid;
  gap: 6px;
}
.pill {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--primary, #4f8cff) 16%, transparent);
  font-size: 12px;
  overflow-wrap: anywhere;
}
.pill.soft {
  background: color-mix(in srgb, #94a3b8 18%, transparent);
}
.pill.ok {
  background: color-mix(in srgb, #22c55e 18%, transparent);
}
.seg {
  display: flex;
  gap: 6px;
  margin: 10px 0;
}
.seg button {
  border-radius: 999px;
  min-height: 36px;
}
.seg button.active {
  background: color-mix(in srgb, var(--primary, #4f8cff) 22%, transparent);
  font-weight: 700;
}
.modal-mask {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: flex-end;
  justify-content: center;
  z-index: 50;
  padding: 0;
}
.modal-card {
  width: min(520px, 100%);
  background: var(--card, #1a1a1e);
  border: 1px solid var(--border, #3333);
  border-radius: 16px 16px 0 0;
  padding: 16px;
  max-height: 90vh;
  overflow: auto;
}
.target-name {
  overflow-wrap: anywhere;
}
.sheet-actions {
  margin-top: 12px;
}
@media (min-width: 761px) {
  .modal-mask {
    align-items: center;
    padding: 16px;
  }
  .modal-card {
    border-radius: 14px;
  }
}
@media (max-width: 760px) {
  .action-row {
    width: 100%;
  }
  .action-row button {
    flex: 1 1 auto;
    min-height: 40px;
  }
  .filter-grid {
    grid-template-columns: 1fr 1fr;
  }
}
</style>
