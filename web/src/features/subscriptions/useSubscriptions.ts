import { computed, ref } from 'vue'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'

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

export function subscriptionPatchPayload(draft: SubscriptionDraft): Partial<SubscriptionDraft> {
  const payload: Partial<SubscriptionDraft> = {
    name: draft.name,
    enabled: draft.enabled,
    refresh_policy: draft.refresh_policy,
    config: draft.config,
  }
  if (draft.source_url_secret_ref.trim() && draft.source_url_secret_ref !== '***') {
    payload.source_url_secret_ref = draft.source_url_secret_ref
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
      error.value = cause instanceof Error ? cause.message : 'Unable to load subscriptions'
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
        toastStore.push({ message: 'Subscription updated', tone: 'success' })
      } else {
        await api.post('/api/v1/subscriptions', draft)
        toastStore.push({ message: 'Subscription added', tone: 'success' })
      }
      await load()
    } catch (cause) {
      toastStore.push({ message: cause instanceof Error ? cause.message : 'Unable to save subscription', tone: 'error' })
      throw cause
    } finally {
      saving.value = false
    }
  }

  async function remove(subscription: SubscriptionRecord) {
    try {
      await api.delete(`/api/v1/subscriptions/${encodeURIComponent(subscription.id)}`)
      toastStore.push({ message: 'Subscription deleted', tone: 'success' })
      await load()
    } catch (cause) {
      toastStore.push({ message: cause instanceof Error ? cause.message : 'Unable to delete subscription', tone: 'error' })
    }
  }

  async function refresh(subscription: SubscriptionRecord) {
    if (refreshingIDs.value.has(subscription.id)) return
    refreshingIDs.value = new Set(refreshingIDs.value).add(subscription.id)
    try {
      await api.post(`/api/v1/subscriptions/${encodeURIComponent(subscription.id)}/refresh`, undefined, {
        headers: { 'Idempotency-Key': crypto.randomUUID() },
      })
      toastStore.push({ message: 'Refresh queued', tone: 'success' })
    } catch (cause) {
      toastStore.push({ message: cause instanceof Error ? cause.message : 'Unable to refresh subscription', tone: 'error' })
    } finally {
      const pending = new Set(refreshingIDs.value)
      pending.delete(subscription.id)
      refreshingIDs.value = pending
    }
  }

  return { items, loading, saving, error, total, hasItems, refreshingIDs, load, save, remove, refresh }
}
