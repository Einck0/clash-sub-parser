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
      return { label: '当前实时归属', tone: 'success' }
    case 'historical_verified':
      return { label: '历史已核验归属', tone: 'info' }
    case 'manual_confirmed':
      return { label: '人工已核验归属', tone: 'success' }
    case 'conflict':
      return { label: '归属存在冲突', tone: 'warning' }
    case 'unknown':
    default:
      return { label: '来源证据不足', tone: 'info' }
  }
}

export function attributionCauseLabel(cause: AttributionCause | string | undefined): string {
  switch (cause) {
    case 'legacy_import':
      return '历史系统导入'
    case 'refresh_removed':
      return '刷新下线移除'
    case 'subscription_deleted':
      return '订阅源已删除'
    case 'manual_confirmed':
      return '人工核验确认'
    case 'unresolved':
      return '未决归属'
    default:
      return cause || '未留记录'
  }
}

export function relationStateBadge(state: RelationState | string | undefined): {
  label: string
  tone: ToastTone
} {
  switch (state) {
    case 'verified':
      return { label: '已核验', tone: 'success' }
    case 'conflict':
      return { label: '冲突', tone: 'warning' }
    case 'unknown':
    default:
      return { label: '待核验/未知', tone: 'info' }
  }
}

export function formatObservedTime(isoStr?: string | null): string {
  if (!isoStr || typeof isoStr !== 'string' || !isoStr.trim()) {
    return '未留记录'
  }
  try {
    const d = new Date(isoStr)
    if (Number.isNaN(d.getTime())) return '未留记录'
    return d.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    })
  } catch {
    return '未留记录'
  }
}
