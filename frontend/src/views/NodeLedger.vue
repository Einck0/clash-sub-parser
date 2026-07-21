<template>
  <section class="page">
    <div class="page-head">
      <div>
        <p class="eyebrow">Node Ledger</p>
        <h2>节点台账</h2>
        <p class="page-desc">
          处理后的最终节点一览，含生效中的 dialer-proxy。可搜索，并快捷给单节点设跳板。
        </p>
      </div>
      <button class="primary" @click="reload" :disabled="loading">刷新</button>
    </div>

    <div v-if="error" class="alert error">{{ error }}</div>

    <div class="dns-section">
      <div class="row space" style="gap:12px;flex-wrap:wrap">
        <input v-model="search" placeholder="搜索节点 / 订阅 / 跳板" style="min-width:220px;flex:1" />
        <label class="settings-toggle">
          <input type="checkbox" v-model="onlyChained" />
          <span>仅看已挂链</span>
        </label>
        <span class="muted">{{ filtered.length }} / {{ rows.length }}</span>
      </div>
    </div>

    <div v-if="loading && !rows.length" class="empty-mini">加载中…</div>
    <div v-else-if="!filtered.length" class="empty-mini">没有匹配的节点。</div>
    <div v-else class="table-like">
      <div v-for="item in filtered" :key="item.name" class="table-row ledger-row">
        <div class="ledger-main">
          <div class="ledger-name mono">{{ item.name }}</div>
          <div class="ledger-meta muted">
            <span v-if="item.subscription_name">{{ item.subscription_name }}</span>
            <span v-if="item.type">{{ item.type }}</span>
            <span v-if="item.server">{{ item.server }}</span>
          </div>
          <div class="ledger-chain" v-if="item.dialer_proxy">
            <span class="pill secondary">dialer → {{ item.dialer_proxy }}</span>
            <span v-if="item.chain_source" class="pill">来源: {{ sourceLabel(item.chain_source) }}</span>
          </div>
          <div v-else class="muted small-line">未挂链</div>
        </div>
        <div class="action-row compact-actions">
          <button @click="openChain(item)">设跳板</button>
          <button v-if="item.dialer_proxy && item.chain_source === 'node'" class="danger" @click="clearNodeChain(item)">
            清节点链
          </button>
        </div>
      </div>
    </div>

    <div v-if="chainTarget" class="modal-mask" @click.self="closeChain">
      <div class="modal-card">
        <h3>给节点设跳板</h3>
        <p class="section-hint mono">{{ chainTarget.name }}</p>
        <label class="field">
          <span>跳板类型</span>
          <select v-model="chainForm.dialer_type">
            <option value="node">节点</option>
            <option value="node_group">策略组</option>
          </select>
        </label>
        <label v-if="chainForm.dialer_type === 'node'" class="field">
          <span>跳板节点</span>
          <input v-model="dialerSearch" placeholder="搜索" class="search-input" />
          <select v-model="chainForm.dialer_ref">
            <option value="">选择节点</option>
            <option v-for="n in dialerNodeOptions" :key="n.name" :value="n.name">{{ n.name }}</option>
          </select>
        </label>
        <label v-else class="field">
          <span>跳板策略组</span>
          <input v-model="dialerSearch" placeholder="搜索" class="search-input" />
          <select v-model="chainForm.dialer_ref">
            <option value="">选择策略组</option>
            <option v-for="g in dialerGroupOptions" :key="g.id" :value="g.name">{{ g.name }}</option>
          </select>
        </label>
        <div class="template-actions" style="margin-top:12px">
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
import { computed, onMounted, reactive, ref } from 'vue'
import {
  createProxyChain,
  deleteProxyChain,
  getApiErrorMessage,
  getNodeGroups,
  getNodeLedger,
  getProxyChains,
} from '../api'

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const rows = ref([])
const bindings = ref([])
const nodeGroups = ref([])
const search = ref('')
const onlyChained = ref(false)
const chainTarget = ref(null)
const dialerSearch = ref('')
const chainForm = reactive({
  dialer_type: 'node',
  dialer_ref: '',
})

const filtered = computed(() => {
  const q = String(search.value || '').trim().toLowerCase()
  return (rows.value || []).filter((item) => {
    if (onlyChained.value && !item.dialer_proxy) return false
    if (!q) return true
    const hay = [item.name, item.subscription_name, item.dialer_proxy, item.type, item.server]
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
    error.value = getApiErrorMessage(err, '加载台账失败')
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
    // Replace existing node-level binding for this target if present.
    const existing = (bindings.value || []).filter(
      (b) => b.target_type === 'node' && b.target_name === chainTarget.value.name,
    )
    for (const b of existing) {
      await deleteProxyChain(b.id)
    }
    await createProxyChain({
      target_type: 'node',
      target_name: chainTarget.value.name,
      dialer_type: chainForm.dialer_type,
      dialer_ref: chainForm.dialer_ref,
      enabled: true,
      note: 'from node ledger',
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
  error.value = ''
  try {
    const existing = (bindings.value || []).filter(
      (b) => b.target_type === 'node' && b.target_name === item.name,
    )
    for (const b of existing) {
      await deleteProxyChain(b.id)
    }
    await reload()
  } catch (err) {
    error.value = getApiErrorMessage(err, '清除失败')
  }
}
</script>

<style scoped>
.ledger-row {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
  padding: 12px 0;
  border-bottom: 1px solid var(--border, #3333);
}
.ledger-main {
  min-width: 0;
  flex: 1;
}
.ledger-name {
  font-size: 13px;
  word-break: break-all;
}
.ledger-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 4px;
  font-size: 12px;
}
.ledger-chain {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 6px;
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
.modal-mask {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 50;
  padding: 16px;
}
.modal-card {
  width: min(480px, 100%);
  background: var(--card, #1a1a1e);
  border: 1px solid var(--border, #3333);
  border-radius: 12px;
  padding: 16px;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 10px;
}
.search-input {
  margin-bottom: 4px;
}
.small-line {
  font-size: 12px;
  margin-top: 4px;
}
</style>
