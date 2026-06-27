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
        <button @click="load" :disabled="loading">{{ loading ? '刷新中...' : '刷新' }}</button>
      </div>
    </div>

    <UiState v-if="error" type="error" title="订阅加载失败" :description="error" compact>
      <template #actions>
        <button @click="load">重试</button>
      </template>
    </UiState>

    <UiState v-if="loading && !subscriptions.length" type="loading" title="正在加载订阅" description="正在读取订阅列表和节点缓存，请稍等。" />

    <div class="subscription-grid" v-else>
      <article v-for="sub in subscriptions" :key="sub.id" class="subscription-card">
        <div class="subscription-card-head">
          <div>
            <div class="row" style="gap:6px">
              <h3>{{ sub.name }}</h3>
              <span class="badge" v-if="sub.is_primary">主订阅</span>
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
          <span class="badge">包含 {{ (sub.include_node_names || []).length }}</span>
          <span class="badge">排除 {{ (sub.exclude_node_names || []).length }}</span>
          <span class="badge">正则 {{ (sub.filter_regex || []).length || '全选' }}</span>
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

        <div class="action-row compact-actions">
          <button @click="openEdit(sub)">编辑</button>
          <button @click="doFetch(sub.id)" :disabled="loadingFetchId === sub.id">{{ loadingFetchId === sub.id ? '拉取中...' : '拉取' }}</button>
          <button @click="showNodes(sub)">节点</button>
          <button class="danger" @click="remove(sub)">删除</button>
        </div>
      </article>

      <UiState v-if="!subscriptions.length && !loading" type="empty" title="暂无订阅" description="添加第一个订阅后，就能拉取节点、查看流量和配置筛选规则。">
        <template #actions>
          <button class="primary" @click="openCreate">添加订阅</button>
        </template>
      </UiState>
    </div>

    <div class="modal-backdrop" v-if="viewingNodes.length" @click.self="closeNodePreview">
      <div class="modal node-preview-modal" role="dialog" aria-modal="true" :aria-label="nodePreviewTitle || '节点预览'">
        <div class="row space preview-title">
          <div>
            <p class="eyebrow">Node Preview</p>
            <h3>{{ nodePreviewTitle || '节点预览' }}（{{ viewingNodes.length }}）</h3>
            <p class="section-hint">支持按节点名、协议、服务器或端口搜索。点背景或关闭按钮即可返回订阅页。</p>
          </div>
          <button @click="closeNodePreview">关闭</button>
        </div>
        <NodePreviewList :nodes="viewingNodes" :collapsed-limit="60" />
      </div>
    </div>

    <div class="modal-backdrop" v-if="showForm" @click.self="showForm = false">
      <div class="modal">
        <SubscriptionForm
          :subscription="editing"
          :all-nodes="allNodes"
          @save="save"
          @cancel="showForm = false"
        />
      </div>
    </div>
  </section>
</template>

<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import { useAppStore } from '../stores/app'
import { formatBytes, short, formatLocalTime } from '../utils/format'
import NodePreviewList from '../components/NodePreviewList.vue'
import SubscriptionForm from '../components/SubscriptionForm.vue'
import UiState from '../components/UiState.vue'
import {
  createSubscription,
  deleteSubscription,
  fetchSubscription,
  getAllSubscriptionNodes,
  getApiErrorMessage,
  getSubscriptionNodes,
  getSubscriptions,
  updateSubscription,
} from '../api'

const store = useAppStore()

const subscriptions = ref([])
const allNodes = ref([])
const viewingNodes = ref([])
const nodePreviewTitle = ref('')
const showForm = ref(false)
const editing = ref(null)
const loadingFetchId = ref(null)
const loading = ref(false)
const error = ref('')

onMounted(load)

// Esc to close modals
function onKeydown(e) {
  if (e.key === 'Escape') {
    if (showForm.value) showForm.value = false
    else if (viewingNodes.value.length) closeNodePreview()
  }
}
window.addEventListener('keydown', onKeydown)
onUnmounted(() => window.removeEventListener('keydown', onKeydown))

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [sRes, nRes] = await Promise.all([getSubscriptions(), getAllSubscriptionNodes()])
    subscriptions.value = sRes.data
    allNodes.value = nRes.data
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

async function doFetch(id) {
  loadingFetchId.value = id
  error.value = ''
  try {
    await fetchSubscription(id)
    store.success('订阅拉取成功')
  } catch (err) {
    store.error(getApiErrorMessage(err, '拉取订阅失败'))
  } finally {
    loadingFetchId.value = null
  }
  await load()
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
    nodePreviewTitle.value = `${item.name || '订阅'}节点预览`
  } catch (err) {
    store.error(getApiErrorMessage(err, '加载节点失败'))
  }
}

function closeNodePreview() {
  viewingNodes.value = []
  nodePreviewTitle.value = ''
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
