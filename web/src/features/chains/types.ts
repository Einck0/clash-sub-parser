import { uiText } from '../../locales/redesign'
import { displayName } from '../../utils/displayNames'

export type ProxyChainTargetType = 'subscription' | 'node_group' | 'node'
export type ProxyChainDialerType = 'node' | 'node_group'

export interface ProxyChainBinding {
  id: string
  target_type: ProxyChainTargetType
  target_id: string
  dialer_type: ProxyChainDialerType
  dialer_id: string
  enabled: boolean
  sort_order: number
  note: string
  revision: number
}

export interface ProxyChainConfig {
  bindings: ProxyChainBinding[]
  version: string
}

export interface OptionItem {
  id: string
  name: string
}

export interface OptionsResult {
  nodes: OptionItem[]
  groups: OptionItem[]
  subscriptions: OptionItem[]
}

export interface EffectNode {
  logical_id: string
  display_name: string
  dialer_proxy?: string
  chain_binding_id?: string
  chain_source?: string
}

export interface Diagnostic {
  severity?: 'info' | 'warning' | 'error' | string
  level?: string
  code: string
  message: string
  field?: string
  target?: string
  reason?: string
  excluded_count?: number
}

export interface ChainEffect {
  binding_id: string
  target_count: number
  chain_count: number
  skip_count: number
  overridden_count: number
  chain_samples?: string[]
  skip_samples?: string[]
  diagnostics?: Diagnostic[]
}

export interface EffectsResult {
  version: string
  effects: ChainEffect[]
  nodes: EffectNode[]
}

export interface DisplayNameResult {
  name: string
  isMissing: boolean
  isUnverified: boolean
}

export function getTargetDisplayName(
  type: ProxyChainTargetType,
  id: string,
  options: OptionsResult,
  verified = true
): DisplayNameResult {
  if (!id) {
    return { name: '', isMissing: false, isUnverified: false }
  }
  const list = type === 'subscription'
    ? options.subscriptions
    : type === 'node_group' ? options.groups : options.nodes
  const found = list.find((item) => item.id === id)
  if (found) {
    return {
      name: displayName(found.name, uiText('未命名目标', 'Unnamed target'), id),
      isMissing: false,
      isUnverified: !verified,
    }
  }
  return {
    name: verified
      ? uiText('目标已失效，请重新选择', 'Target unavailable; select again')
      : uiText('目标待确认', 'Target unverified'),
    isMissing: verified,
    isUnverified: !verified,
  }
}

export function getDialerDisplayName(
  type: ProxyChainDialerType,
  id: string,
  options: OptionsResult,
  verified = true
): DisplayNameResult {
  if (!id) {
    return { name: '', isMissing: false, isUnverified: false }
  }
  const list = type === 'node_group' ? options.groups : options.nodes
  const found = list.find((item) => item.id === id)
  if (found) {
    return {
      name: displayName(found.name, uiText('未命名跳板', 'Unnamed dialer'), id),
      isMissing: false,
      isUnverified: !verified,
    }
  }
  return {
    name: verified
      ? uiText('跳板已失效，请重新选择', 'Dialer unavailable; select again')
      : uiText('跳板待确认', 'Dialer unverified'),
    isMissing: verified,
    isUnverified: !verified,
  }
}

export function getTargetTypeLabel(type: ProxyChainTargetType): string {
  switch (type) {
    case 'node':
      return uiText('单节点', 'Node')
    case 'node_group':
      return uiText('策略组', 'Group')
    case 'subscription':
      return uiText('订阅', 'Subscription')
    default:
      return type
  }
}

export function getDialerTypeLabel(type: ProxyChainDialerType): string {
  switch (type) {
    case 'node':
      return uiText('节点跳板', 'Node Dialer')
    case 'node_group':
      return uiText('策略组跳板', 'Group Dialer')
    default:
      return type
  }
}

export function getPriorityRank(type: ProxyChainTargetType): number {
  switch (type) {
    case 'node':
      return 1
    case 'node_group':
      return 2
    case 'subscription':
      return 3
    default:
      return 9
  }
}
