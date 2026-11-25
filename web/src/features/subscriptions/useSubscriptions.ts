import { computed, ref } from 'vue'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { generateUUID } from '../../utils/uuid'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'
import { displayText } from '../../utils/displayNames'

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

export type EntryKind = 'proxy' | 'notice' | 'unknown' | 'format_error'

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
  const searchText = ref('')
  const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

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
        message: uiText('已设为主订阅', 'Primary subscription updated'),
        tone: 'success',
      })
      await reloadAfterMutation()
      return true
    } catch (cause) {
      const msg = displayText(cause instanceof Error ? cause.message : uiText('设置主订阅失败', 'Failed to set primary subscription'))
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
        params: { page: page.value, page_size: pageSize, search_text: searchText.value || undefined },
      })
      if (thisSeq !== loadSequence) return false
      items.value = result?.items ?? []
      total.value = result?.total ?? items.value.length
      if (page.value > pageCount.value) {
        page.value = pageCount.value
        return load()
      }
      return true
    } catch (cause) {
      if (thisSeq !== loadSequence) return false
      error.value = displayText(cause instanceof Error ? cause.message : uiText('加载订阅源列表失败', 'Failed to load subscriptions'))
      return false
    } finally {
      if (thisSeq === loadSequence) {
        loading.value = false
      }
    }
  }

  async function setPage(nextPage: number) {
    page.value = Math.max(1, Math.min(pageCount.value, nextPage))
    items.value = []
    return load()
  }

  async function search(value: string) {
    searchText.value = value.trim()
    page.value = 1
    items.value = []
    return load()
  }

  async function reloadAfterMutation() {
    const ok = await load()
    if (!ok && error.value) {
      toastStore.push({
        message: uiText(`操作已完成，但刷新列表失败: ${error.value}`, `The change succeeded, but reloading the list failed: ${error.value}`),
        tone: 'warning',
      })
    }
    return ok
  }

  async function save(draft: SubscriptionDraft, current?: SubscriptionRecord): Promise<boolean> {
    if (saving.value) return false
    saving.value = true
    try {
      if (current) {
        await api.patch(`/api/v1/subscriptions/${encodeURIComponent(current.id)}`, subscriptionPatchPayload(draft), {
          headers: { 'If-Match': current.revision },
        })
        toastStore.push({ message: uiText('订阅源已更新', 'Subscription updated'), tone: 'success' })
      } else {
        await api.post('/api/v1/subscriptions', draft)
        toastStore.push({ message: uiText('订阅源已添加', 'Subscription added'), tone: 'success' })
      }
      await reloadAfterMutation()
      return true
    } catch (cause) {
      toastStore.push({ message: displayText(cause instanceof Error ? cause.message : uiText('保存订阅源失败', 'Failed to save subscription')), tone: 'error' })
      throw cause
    } finally {
      saving.value = false
    }
  }

  async function remove(subscription: SubscriptionRecord): Promise<{ ok: boolean; error?: string }> {
    try {
      await api.delete(`/api/v1/subscriptions/${encodeURIComponent(subscription.id)}`)
    } catch (cause) {
      const msg = displayText(cause instanceof Error ? cause.message : uiText('删除订阅源失败', 'Failed to delete subscription'))
      toastStore.push({ message: msg, tone: 'error' })
      return { ok: false, error: msg }
    }

    // Immediately remove successfully deleted ID and adjust total to eliminate old row
    // and prevent repeated DELETE attempts even if subsequent reload fails
    items.value = items.value.filter((s) => s.id !== subscription.id)
    total.value = Math.max(0, total.value - 1)

    toastStore.push({ message: uiText('订阅源已删除', 'Subscription deleted'), tone: 'success' })
    clearRowError(subscription.id)

    const reloadOk = await load()
    if (!reloadOk && error.value) {
      toastStore.push({
        message: uiText(`订阅源已删除，但刷新列表失败: ${error.value}`, `Subscription deleted, but reloading the list failed: ${error.value}`),
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
      await reloadAfterMutation()
    } catch (cause) {
      const msg = displayText(cause instanceof Error ? cause.message : t('subscriptions.refreshFailed'))
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
        message: !subscription.enabled ? uiText('订阅源已启用', 'Subscription enabled') : uiText('订阅源已停用', 'Subscription disabled'),
        tone: 'success',
      })
      await reloadAfterMutation()
      return true
    } catch (cause) {
      const msg = displayText(cause instanceof Error ? cause.message : uiText('切换订阅状态失败', 'Failed to change subscription status'))
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
    page,
    pageSize,
    pageCount,
    searchText,
    setPage,
    search,
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

export function isFormatErrorEntry(entry: SubscriptionEntryDTO): boolean {
  return entry.entry_kind === 'format_error' || entry.effective_kind === 'format_error'
}

export function useSubscriptionEntries() {
  const entries = ref<SubscriptionEntryDTO[]>([])
  const loading = ref(false)
  const error = ref('')
  const total = ref(0)
  const entryErrors = ref<Record<string, string>>({})
  const pendingKeys = ref(new Set<string>())
  const pendingOperations = new Map<string, symbol>()
  let activeSubscriptionId = ''
  let selectionSequence = 0
  let loadSequence = 0

  function isOverriding(subscriptionId: string, entryId: string) {
    return pendingKeys.value.has(JSON.stringify([subscriptionId, entryId]))
  }

  function resetEntries() {
    ++selectionSequence
    ++loadSequence
    activeSubscriptionId = ''
    entries.value = []
    total.value = 0
    error.value = ''
    entryErrors.value = {}
    pendingOperations.clear()
    pendingKeys.value = new Set()
    loading.value = false
  }

  async function loadEntries(subscriptionId: string) {
    if (activeSubscriptionId !== subscriptionId) {
      resetEntries()
      activeSubscriptionId = subscriptionId
    }
    const sequence = ++loadSequence
    loading.value = true
    error.value = ''
    try {
      const res = await api.get<SubscriptionEntriesResponse>(
        `/api/v1/subscriptions/${encodeURIComponent(subscriptionId)}/entries`
      )
      if (sequence !== loadSequence) return
      entries.value = res?.items ?? []
      total.value = res?.total ?? entries.value.length
    } catch (cause) {
      if (sequence !== loadSequence) return
      error.value = displayText(cause instanceof Error ? cause.message : uiText('加载来源条目失败', 'Failed to load source entries'))
    } finally {
      if (sequence === loadSequence) loading.value = false
    }
  }

  async function setOverride(
    subscriptionId: string,
    entryId: string,
    userKindOverride: EntryKind | null,
    reason?: string
  ): Promise<SubscriptionEntryDTO | null> {
    const entry = entries.value.find((item) => item.entry_id === entryId)
    if (activeSubscriptionId !== subscriptionId || !entry || isFormatErrorEntry(entry) || userKindOverride === 'format_error' || isOverriding(subscriptionId, entryId)) return null
    const key = JSON.stringify([subscriptionId, entryId])
    const operation = Symbol(key)
    const selection = selectionSequence
    pendingOperations.set(key, operation)
    pendingKeys.value = new Set(pendingKeys.value).add(key)
    const nextErrors = { ...entryErrors.value }
    delete nextErrors[entryId]
    entryErrors.value = nextErrors
    try {
      const payload: EntryOverridePayload = { user_kind_override: userKindOverride }
      if (reason) payload.reason = reason
      const updated = await api.post<SubscriptionEntryDTO>(
        `/api/v1/subscriptions/${encodeURIComponent(subscriptionId)}/entries/${encodeURIComponent(entryId)}/override`,
        payload
      )
      if (updated && selection === selectionSequence && activeSubscriptionId === subscriptionId) {
        const idx = entries.value.findIndex((item) => item.entry_id === entryId)
        if (idx !== -1) entries.value[idx] = updated
      }
      toastStore.push({ message: uiText('分类覆盖已生效', 'Classification override saved'), tone: 'success' })
      return updated
    } catch (cause) {
      const message = displayText(cause instanceof Error ? cause.message : uiText('更新分类覆盖失败', 'Failed to update classification override'))
      if (selection === selectionSequence && activeSubscriptionId === subscriptionId) {
        entryErrors.value = { ...entryErrors.value, [entryId]: message }
      }
      toastStore.push({ message, tone: 'error' })
      throw cause
    } finally {
      if (pendingOperations.get(key) === operation) {
        pendingOperations.delete(key)
        const pending = new Set(pendingKeys.value)
        pending.delete(key)
        pendingKeys.value = pending
      }
    }
  }

  return {
    entries,
    loading,
    error,
    total,
    entryErrors,
    isOverriding,
    resetEntries,
    loadEntries,
    setOverride,
  }
}
