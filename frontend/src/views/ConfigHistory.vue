<template>
  <section class="page config-history-page">
    <div class="page-head">
      <div>
        <p class="eyebrow">Config History</p>
        <h2>配置版本历史</h2>
        <p class="page-desc">每次批量保存规则/分类时自动创建快照，支持手动创建和回滚。最多保留 50 个版本。</p>
      </div>
      <div class="head-actions">
        <button @click="load" :disabled="loading">{{ loading ? '刷新中...' : '刷新' }}</button>
        <button class="primary" @click="createManual" :disabled="creating">
          {{ creating ? '创建中...' : '手动创建快照' }}
        </button>
      </div>
    </div>

    <UiState v-if="error" type="error" title="加载失败" :description="error" compact>
      <template #actions><button @click="load">重试</button></template>
    </UiState>

    <UiState v-if="loading && !snapshots.length" type="loading" title="加载中" />

    <div class="snapshot-timeline" v-else>
      <div v-if="!snapshots.length" class="empty-state">
        暂无快照。首次批量保存时会自动创建，也可以手动创建。
      </div>

      <div
        v-for="snap in snapshots"
        :key="snap.id"
        class="snapshot-card"
        :class="{ expanded: expandedId === snap.id }"
      >
        <div class="snapshot-header" @click="toggleExpand(snap.id)">
          <div class="snapshot-info">
            <span class="snapshot-id">#{{ snap.id }}</span>
            <div>
              <strong>{{ snap.label || '自动快照' }}</strong>
              <p class="muted" v-if="snap.description">{{ snap.description }}</p>
            </div>
          </div>
          <div class="snapshot-meta">
            <span class="muted">{{ formatTime(snap.created_at) }}</span>
            <span class="snapshot-badge" v-if="snap.label?.startsWith('auto')">自动</span>
            <span class="snapshot-badge manual" v-else>手动</span>
          </div>
        </div>

        <div v-if="expandedId === snap.id" class="snapshot-detail">
          <div v-if="loadingData" class="muted">加载快照数据...</div>
          <div v-else-if="snapData" class="snapshot-data-grid">
            <div v-for="(rows, table) in snapData.tables" :key="table" class="snapshot-table-info">
              <span class="badge">{{ table }}</span>
              <span class="muted">{{ rows.length }} 条</span>
            </div>
          </div>

          <div class="snapshot-actions">
            <button class="danger" @click="doRestore(snap)" :disabled="restoring">
              {{ restoring === snap.id ? '回滚中...' : '回滚到此版本' }}
            </button>
            <button @click="doDelete(snap)" :disabled="deleting">
              {{ deleting === snap.id ? '删除中...' : '删除快照' }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useAppStore } from '../stores/app'
import {
  getSnapshots,
  createSnapshot,
  getSnapshotData,
  restoreSnapshot,
  deleteSnapshot,
  getApiErrorMessage,
} from '../api'
import UiState from '../components/UiState.vue'

const store = useAppStore()
const snapshots = ref([])
const loading = ref(false)
const creating = ref(false)
const restoring = ref(null)
const deleting = ref(null)
const expandedId = ref(null)
const snapData = ref(null)
const loadingData = ref(false)
const error = ref('')

onMounted(load)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const { data } = await getSnapshots()
    snapshots.value = data
  } catch (err) {
    error.value = getApiErrorMessage(err, '加载快照列表失败')
  } finally {
    loading.value = false
  }
}

async function createManual() {
  creating.value = true
  try {
    const label = `手动快照 ${new Date().toLocaleString()}`
    await createSnapshot({ label, description: '' })
    store.success('快照已创建')
    await load()
  } catch (err) {
    store.error(getApiErrorMessage(err, '创建快照失败'))
  } finally {
    creating.value = false
  }
}

async function toggleExpand(id) {
  if (expandedId.value === id) {
    expandedId.value = null
    snapData.value = null
    return
  }
  expandedId.value = id
  snapData.value = null
  loadingData.value = true
  try {
    const { data } = await getSnapshotData(id)
    snapData.value = data
  } catch (err) {
    store.error(getApiErrorMessage(err, '加载快照数据失败'))
  } finally {
    loadingData.value = false
  }
}

async function doRestore(snap) {
  const ok = await store.confirm({
    title: '回滚配置',
    message: `确定要回滚到快照 #${snap.id}（${snap.label || '自动快照'}）吗？\n\n当前配置会被覆盖，建议先创建一个快照备份。`,
    confirmText: '确认回滚',
    danger: true,
  })
  if (!ok) return
  restoring.value = snap.id
  try {
    const { data } = await restoreSnapshot(snap.id)
    const summary = Object.entries(data.imported || {})
      .map(([t, c]) => `${t}: ${c}`)
      .join('、')
    store.success(`已回滚到 #${snap.id}，${summary}`)
  } catch (err) {
    store.error(getApiErrorMessage(err, '回滚失败'))
  } finally {
    restoring.value = null
  }
}

async function doDelete(snap) {
  const ok = await store.confirm({
    title: '删除快照',
    message: `确定要删除快照 #${snap.id} 吗？此操作不可撤销。`,
    confirmText: '删除',
    danger: true,
  })
  if (!ok) return
  deleting.value = snap.id
  try {
    await deleteSnapshot(snap.id)
    store.success('快照已删除')
    await load()
  } catch (err) {
    store.error(getApiErrorMessage(err, '删除快照失败'))
  } finally {
    deleting.value = null
  }
}

function formatTime(iso) {
  if (!iso) return '-'
  return new Date(iso).toLocaleString()
}
</script>

<style scoped>
.snapshot-timeline {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.snapshot-card {
  border: 1px solid var(--border);
  border-radius: 10px;
  overflow: hidden;
  transition: border-color 0.15s;
}
.snapshot-card.expanded {
  border-color: var(--brand);
}
.snapshot-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  cursor: pointer;
}
.snapshot-header:hover {
  background: var(--surface-2);
}
.snapshot-info {
  display: flex;
  align-items: center;
  gap: 10px;
}
.snapshot-id {
  font-family: monospace;
  font-size: 12px;
  color: var(--ink-soft);
  min-width: 36px;
}
.snapshot-meta {
  display: flex;
  align-items: center;
  gap: 8px;
}
.snapshot-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 10px;
  background: var(--bg-1);
  color: var(--ink-soft);
}
.snapshot-badge.manual {
  background: rgba(99, 102, 241, 0.1);
  color: var(--brand);
}
.snapshot-detail {
  padding: 12px 16px;
  border-top: 1px solid var(--border);
}
.snapshot-data-grid {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.snapshot-table-info {
  display: flex;
  align-items: center;
  gap: 6px;
}
.snapshot-actions {
  display: flex;
  gap: 8px;
  margin-top: 12px;
}
</style>
