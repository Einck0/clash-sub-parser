import { computed, ref } from 'vue'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { generateUUID } from '../../utils/uuid'

export { generateUUID }

export interface RenameRule {
  pattern: string
  replace: string
}

export interface FilterRule {
  type: 'include' | 'exclude'
  pattern: string
}

export interface SubscriptionConfig {
  cron_schedule?: string
  auto_test?: boolean
  rename_rules?: RenameRule[]
  filter_rules?: FilterRule[]
  target_groups?: string[]
}

export type EntryKind = 'proxy' | 'notice' | 'unknown'

export interface SubscriptionEntryDTO {
  entry_id: string
  subscription_id: string
  payload_id: string
  ordinal: number
  name: string
  entry_kind: EntryKind
  user_kind_override?: EntryKind | null
  effective_kind: EntryKind
  node_logical_id?: string
  classification_reason?: string
  rule_version?: string
  override_reason?: string
  override_at?: string // RFC3339
  conflict?: string
}

export interface SubscriptionEntriesResponse {
  items: SubscriptionEntryDTO[]
  total: number
}

export interface EntryOverridePayload {
  user_kind_override: EntryKind | null
  reason?: string
}

export interface SubscriptionRecord {
  id: string
  name: string
  source_url_secret_ref: string
  enabled: boolean
  refresh_policy: {
    interval_seconds: number
    user_agent_policy: string
    fetch_proxy_ref?: string
    timeout_seconds: number
    max_response_bytes: number
  }
  config?: SubscriptionConfig
  revision: string
  created_at: string
  updated_at: string
  last_refreshed_at?: string | null
  last_refresh_outcome?: 'success' | 'partial' | 'failed' | string | null
  node_count?: number
  source_node_count?: number
  counts_scope?: 'enabled_subscriptions' | string
}

interface Page<T> {
  items: T[]
  page: number
  page_size: number
  total: number
}

export interface SubscriptionDraft {
  name: string
  source_url_secret_ref: string
  enabled: boolean
  refresh_policy: SubscriptionRecord['refresh_policy']
  config: SubscriptionConfig
}

export const defaultPolicy = () => ({
  interval_seconds: 86400,
  user_agent_policy: 'default',
  timeout_seconds: 30,
  max_response_bytes: 10485760,
})

export const defaultConfig = (): SubscriptionConfig => ({
  cron_schedule: '',
  auto_test: false,
  rename_rules: [],
  filter_rules: [],
  target_groups: [],
})

export function formatRefreshTime(iso: string | null | undefined, locale: string = 'zh-CN'): string {
  if (!iso) return ''
  try {
    const d = new Date(iso)
    if (isNaN(d.getTime())) return iso
    const now = Date.now()
    const diffMs = now - d.getTime()
    if (diffMs < 0 || diffMs < 60_000) return locale === 'zh-CN' ? '刚刚' : 'just now'
    const mins = Math.floor(diffMs / 60_000)
    if (mins < 60) {
      return locale === 'zh-CN' ? `${mins} 分钟前` : `${mins} min${mins > 1 ? 's' : ''} ago`
    }
    const hrs = Math.floor(mins / 60)
    if (hrs < 24) {
      return locale === 'zh-CN' ? `${hrs} 小时前` : `${hrs} hr${hrs > 1 ? 's' : ''} ago`
    }
    const days = Math.floor(hrs / 24)
    if (days < 30) {
      return locale === 'zh-CN' ? `${days} 天前` : `${days} day${days > 1 ? 's' : ''} ago`
    }
    return d.toLocaleDateString(locale === 'zh-CN' ? 'zh-CN' : 'en-US')
  } catch {
    return iso
  }
}

export function formatFullDateTime(iso: string | null | undefined, locale: string = 'zh-CN'): string {
  if (!iso) return ''
  try {
    const d = new Date(iso)
    if (isNaN(d.getTime())) return iso
    return d.toLocaleString(locale === 'zh-CN' ? 'zh-CN' : 'en-US', { hour12: false })
  } catch {
    return iso
  }
}

export function subscriptionPatchPayload(draft: SubscriptionDraft): Partial<SubscriptionDraft> {

  const payload: Partial<SubscriptionDraft> = {
    name: draft.name,
    enabled: draft.enabled,
    refresh_policy: draft.refresh_policy,
    config: draft.config,
  }
  if (draft.source_url_secret_ref.trim()) {
    payload.source_url_secret_ref = draft.source_url_secret_ref.trim()
  }
  return payload
}

export function useSubscriptions() {
  const items = ref<SubscriptionRecord[]>([])
  const loading = ref(false)
  const saving = ref(false)
  const error = ref('')
  const page = ref(1)
  const pageSize = 50
  const total = ref(0)

  const hasItems = computed(() => items.value.length > 0)
  const refreshingIDs = ref(new Set<string>())
  const togglingIDs = ref(new Set<string>())

  async function load() {
    loading.value = true
    error.value = ''
    try {
      const result = await api.get<Page<SubscriptionRecord>>('/api/v1/subscriptions', {
        params: { page: page.value, page_size: pageSize },
      })
      items.value = result.items
      total.value = result.total
    } catch (cause) {
      error.value = cause instanceof Error ? cause.message : '加载订阅源列表失败'
    } finally {
      loading.value = false
    }
  }

  async function save(draft: SubscriptionDraft, current?: SubscriptionRecord) {
    saving.value = true
    try {
      if (current) {
        await api.patch(`/api/v1/subscriptions/${encodeURIComponent(current.id)}`, subscriptionPatchPayload(draft), {
          headers: { 'If-Match': current.revision },
        })
        toastStore.push({ message: '订阅源已更新', tone: 'success' })
      } else {
        await api.post('/api/v1/subscriptions', draft)
        toastStore.push({ message: '订阅源已添加', tone: 'success' })
      }
      await load()
    } catch (cause) {
      toastStore.push({ message: cause instanceof Error ? cause.message : '保存订阅源失败', tone: 'error' })
      throw cause
    } finally {
      saving.value = false
    }
  }

  async function remove(subscription: SubscriptionRecord) {
    try {
      await api.delete(`/api/v1/subscriptions/${encodeURIComponent(subscription.id)}`)
      toastStore.push({ message: '订阅源已删除', tone: 'success' })
      await load()
    } catch (cause) {
      toastStore.push({ message: cause instanceof Error ? cause.message : '删除订阅源失败', tone: 'error' })
    }
  }

  async function refresh(subscription: SubscriptionRecord) {
    if (refreshingIDs.value.has(subscription.id)) return
    refreshingIDs.value = new Set(refreshingIDs.value).add(subscription.id)
    try {
      await api.post(`/api/v1/subscriptions/${encodeURIComponent(subscription.id)}/refresh`, undefined, {
        headers: { 'Idempotency-Key': generateUUID() },
      })
      toastStore.push({ message: '订阅刷新任务已加入队列', tone: 'success' })
      await load()
    } catch (cause) {
      toastStore.push({ message: cause instanceof Error ? cause.message : '刷新订阅源失败', tone: 'error' })
      await load().catch(() => {})
    } finally {
      const pending = new Set(refreshingIDs.value)
      pending.delete(subscription.id)
      refreshingIDs.value = pending
    }
  }

  async function toggle(subscription: SubscriptionRecord): Promise<boolean> {
    if (togglingIDs.value.has(subscription.id)) return false
    togglingIDs.value = new Set(togglingIDs.value).add(subscription.id)
    try {
      await api.patch(
        `/api/v1/subscriptions/${encodeURIComponent(subscription.id)}`,
        { enabled: !subscription.enabled },
        {
          headers: { 'If-Match': subscription.revision },
        }
      )
      toastStore.push({
        message: !subscription.enabled ? '订阅源已启用' : '订阅源已停用',
        tone: 'success',
      })
      await load()
      return true
    } catch (cause) {
      toastStore.push({
        message: cause instanceof Error ? cause.message : '切换订阅状态失败',
        tone: 'error',
      })
      await load().catch(() => {})
      throw cause
    } finally {
      const pending = new Set(togglingIDs.value)
      pending.delete(subscription.id)
      togglingIDs.value = pending
    }
  }

  return { items, loading, saving, error, total, hasItems, refreshingIDs, togglingIDs, load, save, remove, refresh, toggle }
}

export function useSubscriptionEntries() {
  const entries = ref<SubscriptionEntryDTO[]>([])
  const loading = ref(false)
  const error = ref('')
  const total = ref(0)
  const overridingId = ref<string | null>(null)

  async function loadEntries(subscriptionId: string) {
    loading.value = true
    error.value = ''
    try {
      const res = await api.get<SubscriptionEntriesResponse>(
        `/api/v1/subscriptions/${encodeURIComponent(subscriptionId)}/entries`
      )
      entries.value = res?.items ?? []
      total.value = res?.total ?? entries.value.length
    } catch (cause) {
      error.value = cause instanceof Error ? cause.message : '加载来源条目失败'
      entries.value = []
      total.value = 0
    } finally {
      loading.value = false
    }
  }

  async function setOverride(
    subscriptionId: string,
    entryId: string,
    userKindOverride: EntryKind | null,
    reason?: string
  ): Promise<SubscriptionEntryDTO | null> {
    overridingId.value = entryId
    try {
      const payload: EntryOverridePayload = {
        user_kind_override: userKindOverride,
      }
      if (reason) payload.reason = reason
      const updated = await api.post<SubscriptionEntryDTO>(
        `/api/v1/subscriptions/${encodeURIComponent(subscriptionId)}/entries/${encodeURIComponent(entryId)}/override`,
        payload
      )
      if (updated) {
        const idx = entries.value.findIndex((e) => e.entry_id === entryId)
        if (idx !== -1) {
          entries.value[idx] = updated
        }
      }
      toastStore.push({ message: '分类覆盖已生效', tone: 'success' })
      return updated
    } catch (cause) {
      toastStore.push({
        message: cause instanceof Error ? cause.message : '更新分类覆盖失败',
        tone: 'error',
      })
      throw cause
    } finally {
      overridingId.value = null
    }
  }

  return {
    entries,
    loading,
    error,
    total,
    overridingId,
    loadEntries,
    setOverride,
  }
}
