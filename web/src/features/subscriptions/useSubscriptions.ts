import { computed, ref } from 'vue'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { generateUUID } from '../../utils/uuid'
import { t } from '../../locales'

export { generateUUID }

export interface RenameRule {
  pattern: string
  replace: string
}

export interface FilterRule {
  type: 'include' | 'exclude'
  pattern: string
}

export type ProbePolicyMode = 'inherit' | 'disabled' | 'custom'

export interface SubscriptionProbePolicy {
  mode: ProbePolicyMode
  interval_seconds?: number
}

export interface SubscriptionConfig {
  cron_schedule?: string
  auto_test?: boolean
  rename_rules?: RenameRule[]
  filter_rules?: FilterRule[]
  target_groups?: string[]
  probe_policy?: SubscriptionProbePolicy
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
  is_primary?: boolean
  userinfo?: string
  prefix?: string
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
  prefix?: string
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
  rename_rules: [],
  filter_rules: [],
  target_groups: [],
  probe_policy: {
    mode: 'inherit',
  },
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
    prefix: draft.prefix,
    refresh_policy: draft.refresh_policy,
    config: draft.config,
  }
  if (draft.source_url_secret_ref.trim()) {
    payload.source_url_secret_ref = draft.source_url_secret_ref.trim()
  }
  return payload
}

export interface ParsedUserInfo {
  upload: number
  download: number
  used: number
  total: number
  remaining: number
  expire: number | null
  expireDate: string | null
}

export function parseUserInfo(header?: string | null): ParsedUserInfo | null {
  if (!header || typeof header !== 'string') return null
  const parts = header.split(';')
  const map: Record<string, number> = {}
  for (const part of parts) {
    const [k, v] = part.split('=').map((s) => s.trim())
    if (k && v && !isNaN(Number(v))) {
      map[k.toLowerCase()] = Number(v)
    }
  }
  const upload = map['upload'] || 0
  const download = map['download'] || 0
  const total = map['total'] || 0
  const used = upload + download
  const remaining = total > used ? total - used : 0
  const expire = map['expire'] || null
  let expireDate: string | null = null
  if (expire && expire > 0) {
    const d = new Date(expire * 1000)
    if (!isNaN(d.getTime())) {
      const year = d.getFullYear()
      const month = String(d.getMonth() + 1).padStart(2, '0')
      const day = String(d.getDate()).padStart(2, '0')
      expireDate = `${year}-${month}-${day}`
    }
  }
  return { upload, download, used, total, remaining, expire, expireDate }
}

export function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  const idx = Math.min(Math.max(0, i), units.length - 1)
  const val = bytes / Math.pow(1024, idx)
  return `${val.toFixed(val >= 100 || idx === 0 ? 0 : 1)} ${units[idx]}`
}

/**
 * Extract second-level label from a URL hostname and capitalize its first letter.
 * Following project experience contract:
 * - https://z.7li7li.com/sub -> '7li7li'
 * - https://msub.example.com/api -> 'Example'
 * - https://www.example.com -> 'Example'
 * - https://example.com -> 'Example'
 */
export function extractSecondLevelDomainName(urlStr: string): string {
  if (!urlStr || typeof urlStr !== 'string') return ''
  try {
    const trimmed = urlStr.trim()
    if (!trimmed) return ''
    const normalized = trimmed.includes('://') ? trimmed : `https://${trimmed}`
    const parsed = new URL(normalized)
    const hostname = parsed.hostname.toLowerCase()
    if (!hostname) return ''

    // If it is an IPv4, IPv6, or localhost address
    if (/^(\d{1,3}\.){3}\d{1,3}$/.test(hostname) || hostname.startsWith('[') || hostname === 'localhost') {
      return hostname.charAt(0).toUpperCase() + hostname.slice(1)
    }

    const parts = hostname.split('.').filter(Boolean)
    if (parts.length === 0) return ''
    let label = ''
    if (parts.length === 1) {
      label = parts[0]
    } else {
      label = parts[parts.length - 2]
    }
    if (!label) return ''
    return label.charAt(0).toUpperCase() + label.slice(1)
  } catch {
    return ''
  }
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
  const settingPrimaryIDs = ref(new Set<string>())
  const rowErrors = ref<Record<string, string>>({})

  function clearRowError(id: string) {
    if (rowErrors.value[id]) {
      const copy = { ...rowErrors.value }
      delete copy[id]
      rowErrors.value = copy
    }
  }

  function setRowError(id: string, message: string) {
    rowErrors.value = { ...rowErrors.value, [id]: message }
  }

  let loadSequence = 0

  async function setPrimary(subscription: SubscriptionRecord): Promise<boolean> {
    if (settingPrimaryIDs.value.has(subscription.id)) return false
    clearRowError(subscription.id)
    settingPrimaryIDs.value = new Set(settingPrimaryIDs.value).add(subscription.id)
    try {
      await api.patch(
        `/api/v1/subscriptions/${encodeURIComponent(subscription.id)}`,
        { is_primary: true },
        {
          headers: { 'If-Match': subscription.revision },
        }
      )
      toastStore.push({
        message: '已设为主订阅',
        tone: 'success',
      })
      await load()
      return true
    } catch (cause) {
      const msg = cause instanceof Error ? cause.message : '设置主订阅失败'
      setRowError(subscription.id, msg)
      toastStore.push({
        message: msg,
        tone: 'error',
      })
      await load().catch(() => {})
      throw cause
    } finally {
      const pending = new Set(settingPrimaryIDs.value)
      pending.delete(subscription.id)
      settingPrimaryIDs.value = pending
    }
  }

  async function load(): Promise<boolean> {
    const thisSeq = ++loadSequence
    loading.value = true
    error.value = ''
    try {
      const result = await api.get<Page<SubscriptionRecord>>('/api/v1/subscriptions', {
        params: { page: page.value, page_size: pageSize },
      })
      if (thisSeq !== loadSequence) return false
      items.value = result?.items ?? []
      total.value = result?.total ?? items.value.length
      return true
    } catch (cause) {
      if (thisSeq !== loadSequence) return false
      error.value = cause instanceof Error ? cause.message : '加载订阅源列表失败'
      return false
    } finally {
      if (thisSeq === loadSequence) {
        loading.value = false
      }
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

  async function remove(subscription: SubscriptionRecord): Promise<{ ok: boolean; error?: string }> {
    try {
      await api.delete(`/api/v1/subscriptions/${encodeURIComponent(subscription.id)}`)
    } catch (cause) {
      const msg = cause instanceof Error ? cause.message : '删除订阅源失败'
      toastStore.push({ message: msg, tone: 'error' })
      return { ok: false, error: msg }
    }

    // Immediately remove successfully deleted ID and adjust total to eliminate old row
    // and prevent repeated DELETE attempts even if subsequent reload fails
    items.value = items.value.filter((s) => s.id !== subscription.id)
    total.value = Math.max(0, total.value - 1)

    toastStore.push({ message: '订阅源已删除', tone: 'success' })
    clearRowError(subscription.id)

    const reloadOk = await load()
    if (!reloadOk && error.value) {
      toastStore.push({
        message: `订阅源已删除，但刷新列表失败: ${error.value}`,
        tone: 'warning',
      })
    }

    return { ok: true }
  }

  async function refresh(subscription: SubscriptionRecord) {
    if (refreshingIDs.value.has(subscription.id)) return
    clearRowError(subscription.id)
    refreshingIDs.value = new Set(refreshingIDs.value).add(subscription.id)
    try {
      await api.post(`/api/v1/subscriptions/${encodeURIComponent(subscription.id)}/refresh`, undefined, {
        headers: { 'Idempotency-Key': generateUUID() },
      })
      toastStore.push({ message: t('subscriptions.refreshCompleted'), tone: 'success' })
      await load()
    } catch (cause) {
      const msg = cause instanceof Error ? cause.message : t('subscriptions.refreshFailed')
      setRowError(subscription.id, msg)
      toastStore.push({ message: msg, tone: 'error' })
      await load().catch(() => {})
    } finally {
      const pending = new Set(refreshingIDs.value)
      pending.delete(subscription.id)
      refreshingIDs.value = pending
    }
  }

  async function toggle(subscription: SubscriptionRecord): Promise<boolean> {
    if (togglingIDs.value.has(subscription.id)) return false
    clearRowError(subscription.id)
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
      const msg = cause instanceof Error ? cause.message : '切换订阅状态失败'
      setRowError(subscription.id, msg)
      toastStore.push({
        message: msg,
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

  return {
    items,
    loading,
    saving,
    error,
    total,
    hasItems,
    refreshingIDs,
    togglingIDs,
    settingPrimaryIDs,
    rowErrors,
    clearRowError,
    load,
    save,
    remove,
    refresh,
    toggle,
    setPrimary,
  }
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
