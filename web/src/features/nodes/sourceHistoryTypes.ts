import { uiText } from '../../locales/redesign'
import type { ToastTone } from '../../ui/toast'

export type RelationState = 'verified' | 'conflict' | 'unknown'

export type AttributionCause =
  | 'legacy_import'
  | 'refresh_removed'
  | 'subscription_deleted'
  | 'manual_confirmed'
  | 'unresolved'

export type AttributionStatus =
  | 'current'
  | 'historical_verified'
  | 'manual_confirmed'
  | 'conflict'
  | 'unknown'

export interface NodeCurrentSource {
  subscription_id: string
  name: string
  enabled: boolean
}

export interface NodeSourceHistoryItem {
  source_label: string
  subscription_id?: string
  relation_state: RelationState
  cause: AttributionCause
  evidence_kind: string
  first_observed_at?: string
  last_observed_at?: string
  connection_revision?: number
  source_deleted: boolean
  source_unmapped?: boolean
}

export interface NodeSourceHistoryData {
  current_sources: NodeCurrentSource[]
  history: NodeSourceHistoryItem[]
  attribution_status: AttributionStatus
}

export function attributionStatusBadge(status: AttributionStatus | undefined): {
  label: string
  tone: ToastTone
} {
  switch (status) {
    case 'current':
      return { label: uiText('当前实时归属', 'Current attribution'), tone: 'success' }
    case 'historical_verified':
      return { label: uiText('历史已核验归属', 'Verified historical attribution'), tone: 'info' }
    case 'manual_confirmed':
      return { label: uiText('人工已核验归属', 'Manually confirmed attribution'), tone: 'success' }
    case 'conflict':
      return { label: uiText('归属存在冲突', 'Attribution conflict'), tone: 'warning' }
    case 'unknown':
    default:
      return { label: uiText('来源证据不足', 'Insufficient source evidence'), tone: 'info' }
  }
}

export function attributionCauseLabel(cause: AttributionCause | string | undefined): string {
  switch (cause) {
    case 'legacy_import':
      return uiText('历史系统导入', 'Imported from legacy system')
    case 'refresh_removed':
      return uiText('刷新下线移除', 'Removed during refresh')
    case 'subscription_deleted':
      return uiText('订阅源已删除', 'Subscription source deleted')
    case 'manual_confirmed':
      return uiText('人工核验确认', 'Manually confirmed')
    case 'unresolved':
      return uiText('未决归属', 'Unresolved attribution')
    default:
      return cause || uiText('未留记录', 'No record')
  }
}

export function relationStateBadge(state: RelationState | string | undefined): {
  label: string
  tone: ToastTone
} {
  switch (state) {
    case 'verified':
      return { label: uiText('已核验', 'Verified'), tone: 'success' }
    case 'conflict':
      return { label: uiText('冲突', 'Conflict'), tone: 'warning' }
    case 'unknown':
    default:
      return { label: uiText('待核验/未知', 'Pending verification / unknown'), tone: 'info' }
  }
}

export function formatObservedTime(isoStr?: string | null): string {
  if (!isoStr || typeof isoStr !== 'string' || !isoStr.trim()) {
    return uiText('未留记录', 'No record')
  }
  try {
    const d = new Date(isoStr)
    if (Number.isNaN(d.getTime())) return uiText('未留记录', 'No record')
    return d.toLocaleString(uiText('zh-CN', 'en-US'), {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    })
  } catch {
    return uiText('未留记录', 'No record')
  }
}
