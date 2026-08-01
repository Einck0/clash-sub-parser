<template>
  <section class="page subscriptions-page">
    <div class="page-head">
      <div>
        <p class="eyebrow">Subscriptions</p>
        <h2>订阅管理</h2>
        <p class="page-desc">拉取订阅时会保留上游流量/到期响应头，并在这里展示。</p>
      </div>
      <div class="head-actions">
        <button class="primary" @click="openCreate">添加订阅</button>
        <button @click="openManualNode">添加自定义节点</button>
        <button @click="load" :disabled="loading">{{ loading ? '刷新中...' : '刷新' }}</button>
      </div>
    </div>

    <UiState v-if="error" type="error" title="订阅加载失败" :description="error" compact>
      <template #actions>
        <button @click="load">重试</button>
      </template>
    </UiState>

    <UiState v-if="loading && !subscriptions.length" type="loading" title="正在加载订阅" description="正在读取订阅列表和节点缓存，请稍等。" />

    <template v-else>
    <PageToolbar
      v-model="search"
      placeholder="搜索订阅名 / URL / 节点数…"
      :count-text="`${filteredSubscriptions.length} / ${subscriptions.length}`"
    >
      <template #filters>
        <select v-model="enabledFilter">
          <option value="">全部状态</option>
          <option value="enabled">仅启用</option>
          <option value="disabled">仅禁用</option>
        </select>
      </template>
    </PageToolbar>

    <div class="subscription-grid">
      <article
        v-for="sub in filteredSubscriptions"
        :key="sub.id"
        class="subscription-card"
        :class="{ 'is-disabled': !sub.enabled, 'is-primary-card': sub.is_primary }"
      >
        <div class="subscription-card-head">
          <div>
            <div class="row" style="gap:6px;flex-wrap:wrap">
              <h3>{{ sub.name }}</h3>
              <span class="badge badge-primary" v-if="sub.is_primary">主订阅</span>
              <span class="badge badge-danger" v-if="!sub.enabled">已禁用</span>
            </div>
            <div class="mono sub-url" :title="sub.url">{{ short(sub.url, 72) }}</div>
          </div>
          <span class="node-count">{{ (sub.raw_nodes || []).length }} 节点</span>
        </div>

        <div class="traffic-block" v-if="parseUserinfo(sub.subscription_userinfo)">
          <div class="traffic-top">
            <strong>{{ trafficSummary(sub) }}</strong>
            <span>{{ trafficPercent(sub) }}%</span>
          </div>
          <div class="traffic-bar">
            <span :style="{ width: `${trafficPercent(sub)}%` }"></span>
          </div>
          <div class="traffic-meta">
            <span>上传 {{ formatBytes(parseUserinfo(sub.subscription_userinfo).upload) }}</span>
            <span>下载 {{ formatBytes(parseUserinfo(sub.subscription_userinfo).download) }}</span>
            <span>剩余 {{ remainingTraffic(sub) }}</span>
          </div>
        </div>
        <div v-else class="empty-mini">暂无流量信息，拉取成功后如果上游返回 header 会显示在这里</div>

        <div class="sub-selection-line">
          <span class="badge">候选 {{ (sub.source_nodes || []).length || (sub.raw_nodes || []).length }}</span>
          <span class="badge">正则 {{ (sub.filter_regex || []).length || '全选' }}</span>
          <span class="badge">包含 {{ (sub.include_node_names || []).length }}</span>
          <span class="badge">排除 {{ (sub.exclude_node_names || []).length }}</span>
          <span class="badge">重命名 {{ Object.keys(sub.node_renames || {}).length }}</span>
        </div>

        <div class="sub-info-grid">
          <div>
            <span class="metric-label">过期时间</span>
            <strong>{{ expireText(sub) }}</strong>
          </div>
          <div>
            <span class="metric-label">更新周期</span>
            <strong>{{ sub.update_interval ? `${sub.update_interval} 分钟` : '-' }}</strong>
          </div>
          <div>
            <span class="metric-label">上次拉取</span>
            <strong>{{ sub.last_fetched_at ? formatLocalTime(sub.last_fetched_at) : '-' }}</strong>
          </div>
        </div>

        <div class="status-line">
          <span v-if="sub.last_fetch_error" class="status-error" :title="sub.last_fetch_error">
            失败 {{ sub.fetch_failed_count || 1 }} 次：{{ short(sub.last_fetch_error, 80) }}
          </span>
          <span v-else class="status-ok">拉取正常</span>
          <a v-if="sub.profile_web_page_url" :href="sub.profile_web_page_url" target="_blank" rel="noreferrer">订阅主页</a>
        </div>

        <div class="action-row compact-actions sub-actions">
          <button class="primary" @click="openEdit(sub)">编辑</button>
          <button @click="doFetch(sub.id)" :disabled="loadingFetchId === sub.id">
            {{ loadingFetchId === sub.id ? '拉取中...' : '拉取' }}
          </button>
          <button @click="showNodes(sub)">节点</button>
          <button
            :class="{ primary: !sub.is_primary }"
            :disabled="sub.is_primary || loadingPrimaryId === sub.id"
            @click="setPrimary(sub)"
          >
            {{ sub.is_primary ? '当前主订阅' : (loadingPrimaryId === sub.id ? '设置中...' : '设为主订阅') }}
          </button>
          <button
            :class="{ danger: sub.enabled }"
            :disabled="loadingToggleId === sub.id"
            @click="toggleEnabled(sub)"
          >
            {{ loadingToggleId === sub.id ? '...' : (sub.enabled ? '禁用' : '启用') }}
          </button>
          <button class="danger" @click="remove(sub)">删除</button>
        </div>
      </article>

      <UiState v-if="!subscriptions.length && !loading" type="empty" title="暂无订阅" description="添加第一个订阅后，就能拉取节点、查看流量和配置筛选规则。">
        <template #actions>
          <button class="primary" @click="openCreate">添加订阅</button>
        </template>
      </UiState>
      <UiState
        v-else-if="subscriptions.length && !filteredSubscriptions.length"
        type="empty"
        title="没有匹配的订阅"
        description="换个关键词或清空筛选。"
      >
        <template #actions>
          <button @click="search = ''; enabledFilter = ''">清空筛选</button>
        </template>
      </UiState>
    </div>
    </template>

    <div class="modal-backdrop" v-if="viewingSub" @click.self="closeNodePreview">
      <div class="modal node-preview-modal" role="dialog" aria-modal="true" :aria-label="nodePreviewTitle || '节点预览'">
        <div class="row space preview-title">
          <div>
            <p class="eyebrow">Node Preview</p>
            <h3>{{ nodePreviewTitle || '节点预览' }}（{{ viewingNodes.length }}）</h3>
            <p class="section-hint">
              支持搜索、TCP 端口探活；可点「改名」直接修改前缀后的最终节点名。
              TCP 通 ≠ 代理可用。
            </p>
          </div>
          <button @click="closeNodePreview">关闭</button>
        </div>
        <NodePreviewList
          :nodes="viewingBaseNodes"
          :collapsed-limit="60"
          :auto-geo="true"
          :editable="true"
          :renames="viewingSub.node_renames || {}"
          :saving="renamingSaving"
          @save-renames="saveNodeRenames"
        />
      </div>
    </div>

    <div class="modal-backdrop" v-if="showForm" @click.self="showForm = false">
      <div class="modal">
        <SubscriptionForm
          :subscription="editing"
          @save="save"
          @cancel="showForm = false"
          @fetched="onFormFetched"
        />
      </div>
    </div>

    <div class="modal-backdrop" v-if="showManualNode" @click.self="showManualNode = false">
      <div class="modal">
        <div class="card subscription-form-card">
          <div class="form-header">
            <div>
              <p class="eyebrow">Manual Node / Raw Mode</p>
              <h3>添加自定义节点（支持 Raw 模式）</h3>
              <p class="section-hint">支持粘贴多行分享链接、Base64 编码订阅内容或 YAML 节点配置。</p>
            </div>
            <button @click="showManualNode = false">关闭</button>
          </div>
          <div class="grid-2">
            <label>
              <div class="muted">节点/订阅名称</div>
              <input v-model="manualForm.name" placeholder="请输入名称，例：我的WARP节点" />
            </label>
            <label>
              <div class="muted">节点前缀（可选）</div>
              <input v-model="manualForm.node_prefix" placeholder="留空则使用订阅名" />
            </label>
          </div>
          <label style="display:block;margin-top:12px">
            <div class="muted">Raw 节点内容 / 链接文本</div>
            <textarea
              v-model="manualForm.node_links"
              class="secret-textarea"
              style="min-height:160px"
              placeholder="支持 ss://, trojan://, vless://, vmess://, wireguard://，或粘贴 Base64 / YAML 原始内容"
            ></textarea>
          </label>
          <p v-if="manualFormError" class="form-alert form-alert-error">{{ manualFormError }}</p>
          <div class="form-footer">
            <button
              class="primary"
              :disabled="manualSaving || !manualForm.node_links.trim()"
              @click="saveManualNode"
            >
              {{ manualSaving ? '保存中...' : '保存并解析' }}
            </button>
            <button @click="showManualNode = false">取消</button>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useAppStore } from '../stores/app'
import { formatBytes, short, formatLocalTime } from '../utils/format'
import NodePreviewList from '../components/NodePreviewList.vue'
import PageToolbar from '../components/PageToolbar.vue'
import SubscriptionForm from '../components/SubscriptionForm.vue'
import UiState from '../components/UiState.vue'
import {
  createManualNodeSubscription,
  createSubscription,
  deleteSubscription,
  fetchSubscription,
  getApiErrorMessage,
  getSubscriptionNodes,
  getSubscriptions,
  updateSubscription,
} from '../api'

const store = useAppStore()

const subscriptions = ref([])
const viewingNodes = ref([])
const viewingSub = ref(null)
const nodePreviewTitle = ref('')
const showForm = ref(false)
const editing = ref(null)
const showManualNode = ref(false)
const manualForm = ref({ name: '', node_prefix: '', node_links: '' })
const manualFormError = ref('')
const manualSaving = ref(false)
const loadingFetchId = ref(null)
const loadingPrimaryId = ref(null)
const loadingToggleId = ref(null)
const renamingSaving = ref(false)
const loading = ref(false)
const error = ref('')
const search = ref('')
const enabledFilter = ref('')

const filteredSubscriptions = computed(() => {
  const q = String(search.value || '').trim().toLowerCase()
  return (subscriptions.value || []).filter((sub) => {
    if (enabledFilter.value === 'enabled' && !sub.enabled) return false
    if (enabledFilter.value === 'disabled' && sub.enabled) return false
    if (!q) return true
    const hay = [sub.name, sub.url, String((sub.raw_nodes || []).length)]
      .join(' ')
      .toLowerCase()
    return hay.includes(q)
  })
})

// Build post-prefix base names for rename editor. Keys in node_renames are
// always these base names, never already-renamed display names.
const viewingBaseNodes = computed(() => {
  if (!viewingSub.value) return viewingNodes.value
  const sub = viewingSub.value
  const prefix = resolvePrefix(sub)
  const source = (sub.source_nodes || []).length
    ? sub.source_nodes
    : (sub.manual_nodes || []).length
      ? sub.manual_nodes
      : null
  if (source && source.length) {
    return source.map((node) => {
      const original = String(node?.name || '').trim()
      const baseName = prefix ? `${prefix}-${original}` : original
      return { ...node, name: baseName }
    })
  }
  // Fallback: invert existing renames so editor shows base keys when possible.
  const reverse = {}
  for (const [base, finalName] of Object.entries(sub.node_renames || {})) {
    if (base && finalName) reverse[String(finalName)] = String(base)
  }
  return (viewingNodes.value || []).map((node) => {
    const current = String(node?.name || '').trim()
    const baseName = reverse[current] || current
    return { ...node, name: baseName }
  })
})

function resolvePrefix(sub) {
  const custom = String(sub?.node_prefix || '').trim()
  if (custom) return custom
  if (sub?.is_primary) return ''
  return String(sub?.name || '').trim()
}

onMounted(load)

// Esc to close modals
function onKeydown(e) {
  if (e.key === 'Escape') {
    if (showForm.value) showForm.value = false
    else if (showManualNode.value) showManualNode.value = false
    else if (viewingNodes.value.length) closeNodePreview()
  }
}
window.addEventListener('keydown', onKeydown)
onUnmounted(() => window.removeEventListener('keydown', onKeydown))

async function load() {
  loading.value = true
  error.value = ''
  try {
    const sRes = await getSubscriptions()
    subscriptions.value = sRes.data
  } catch (err) {
    error.value = getApiErrorMessage(err, '加载订阅失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  error.value = ''
  showForm.value = true
}

function openEdit(item) {
  editing.value = { ...item }
  error.value = ''
  showForm.value = true
}

function openManualNode() {
  manualForm.value = { name: '', node_prefix: '', node_links: '' }
  manualFormError.value = ''
  showManualNode.value = true
}

async function saveManualNode() {
  const links = manualForm.value.node_links.trim()
  if (!links || manualSaving.value) return
  manualSaving.value = true
  manualFormError.value = ''
  try {
    await createManualNodeSubscription({
      name: manualForm.value.name.trim() || '手动节点',
      node_prefix: manualForm.value.node_prefix.trim() || null,
      node_links: links,
    })
    showManualNode.value = false
    store.success('自定义节点已保存')
    await load()
  } catch (err) {
    manualFormError.value = getApiErrorMessage(err, '保存自定义节点失败')
  } finally {
    manualSaving.value = false
  }
}

async function save(payload) {
  error.value = ''
  try {
    if (editing.value?.id) {
      await updateSubscription(editing.value.id, payload)
    } else {
      await createSubscription(payload)
    }
    showForm.value = false
    store.success('订阅已保存')
    await load()
  } catch (err) {
    store.error(getApiErrorMessage(err, '保存订阅失败'))
  }
}

async function onFormFetched(data) {
  // Keep list cache in sync when user fetches inside the edit form.
  if (data?.id) {
    editing.value = { ...editing.value, ...data }
  }
  store.success('订阅拉取成功')
  await load()
}

async function doFetch(id) {
  loadingFetchId.value = id
  error.value = ''
  try {
    await fetchSubscription(id)
    store.success('订阅拉取成功')
    await load()
    // Auto-show nodes after successful fetch
    const sub = subscriptions.value.find((s) => s.id === id)
    if (sub) await showNodes(sub)
  } catch (err) {
    store.error(getApiErrorMessage(err, '拉取订阅失败'))
  } finally {
    loadingFetchId.value = null
  }
}

async function setPrimary(item) {
  if (!item?.id || item.is_primary) return
  loadingPrimaryId.value = item.id
  error.value = ''
  try {
    await updateSubscription(item.id, { is_primary: true })
    store.success(`已将 ${item.name} 设为主订阅`)
    await load()
  } catch (err) {
    store.error(getApiErrorMessage(err, '设置主订阅失败'))
  } finally {
    loadingPrimaryId.value = null
  }
}

async function toggleEnabled(item) {
  if (!item?.id) return
  loadingToggleId.value = item.id
  error.value = ''
  try {
    await updateSubscription(item.id, { enabled: !item.enabled })
    store.success(item.enabled ? `已禁用 ${item.name}` : `已启用 ${item.name}`)
    await load()
  } catch (err) {
    store.error(getApiErrorMessage(err, '切换启用状态失败'))
  } finally {
    loadingToggleId.value = null
  }
}

async function remove(item) {
  const ok = await store.confirm({
    title: '删除订阅',
    message: `确定要删除订阅 "${item.name}" 吗？此操作不可撤销。`,
    confirmText: '删除',
    danger: true,
  })
  if (!ok) return
  error.value = ''
  try {
    await deleteSubscription(item.id)
    store.success(`已删除订阅 ${item.name}`)
    await load()
  } catch (err) {
    store.error(getApiErrorMessage(err, '删除订阅失败'))
  }
}

async function showNodes(item) {
  error.value = ''
  try {
    const res = await getSubscriptionNodes(item.id)
    viewingNodes.value = res.data
    // Keep full subscription context so renames can be saved as post-prefix map.
    viewingSub.value = { ...item }
    nodePreviewTitle.value = `${item.name || '订阅'}节点预览`
  } catch (err) {
    store.error(getApiErrorMessage(err, '加载节点失败'))
  }
}

function closeNodePreview() {
  viewingNodes.value = []
  viewingSub.value = null
  nodePreviewTitle.value = ''
  renamingSaving.value = false
}

async function saveNodeRenames(renames) {
  if (!viewingSub.value?.id || renamingSaving.value) return
  renamingSaving.value = true
  error.value = ''
  try {
    // renames keys are post-prefix base names from viewingBaseNodes.
    await updateSubscription(viewingSub.value.id, {
      node_renames: renames || {},
    })
    // Rebuild final names from source + prefix + renames when possible.
    if ((viewingSub.value.source_nodes || []).length || (viewingSub.value.manual_nodes || []).length) {
      try {
        await fetchSubscription(viewingSub.value.id)
      } catch (_) {
        // rename already saved; fetch failure shouldn't block UI refresh
      }
    }
    store.success('节点名称已保存')
    await load()
    const latest = subscriptions.value.find((item) => item.id === viewingSub.value.id)
    if (latest) {
      viewingSub.value = { ...latest }
      const res = await getSubscriptionNodes(latest.id)
      viewingNodes.value = res.data
    }
  } catch (err) {
    store.error(getApiErrorMessage(err, '保存节点名称失败'))
  } finally {
    renamingSaving.value = false
  }
}

function parseUserinfo(value) {
  if (!value) return null
  // Return cached result if available
  const cacheKey = value
  if (_userinfoCache.has(cacheKey)) return _userinfoCache.get(cacheKey)
  const result = {}
  for (const part of String(value).split(';')) {
    const [key, raw] = part.split('=').map((item) => item && item.trim())
    if (!key || raw === undefined) continue
    const number = Number(raw)
    result[key] = Number.isFinite(number) ? number : raw
  }
  const parsed = Object.keys(result).length ? result : null
  _userinfoCache.set(cacheKey, parsed)
  return parsed
}
const _userinfoCache = new Map()

function usedTraffic(sub) {
  const info = parseUserinfo(sub.subscription_userinfo)
  return Number(info?.upload || 0) + Number(info?.download || 0)
}

function trafficPercent(sub) {
  const info = parseUserinfo(sub.subscription_userinfo)
  const total = Number(info?.total || 0)
  if (!total) return 0
  return Math.min(100, Math.round((usedTraffic(sub) / total) * 1000) / 10)
}

function trafficSummary(sub) {
  const info = parseUserinfo(sub.subscription_userinfo)
  if (!info) return '-'
  return `${formatBytes(usedTraffic(sub))} / ${formatBytes(info.total)}`
}

function remainingTraffic(sub) {
  const info = parseUserinfo(sub.subscription_userinfo)
  if (!info?.total) return '-'
  return formatBytes(Math.max(0, Number(info.total) - usedTraffic(sub)))
}

function expireText(sub) {
  const info = parseUserinfo(sub.subscription_userinfo)
  const expire = Number(info?.expire || 0)
  if (!expire) return '-'
  const date = new Date(expire * 1000)
  const days = Math.ceil((date.getTime() - Date.now()) / 86400000)
  const suffix = days >= 0 ? `剩 ${days} 天` : `已过期 ${Math.abs(days)} 天`
  return `${date.toLocaleDateString()}（${suffix}）`
}
</script>
