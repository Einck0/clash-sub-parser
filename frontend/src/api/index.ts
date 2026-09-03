import { authHeaders } from '../auth'
import { mockFetchExport, mockRequest } from './mock'
import { buildErrorMessage, parseResponse } from './response'

const API_BASE = '/api'
const DEMO_MODE = import.meta.env.VITE_DEMO_MODE === 'true'

interface ApiResponse<T = any> {
  data: T
  status: number
  headers: Headers
}

interface ApiError extends Error {
  response?: { status: number; data: any }
  userMessage?: string
}

async function request(method: string, url: string, data?: any): Promise<ApiResponse> {
  if (DEMO_MODE) return mockRequest(method, url, data) as Promise<ApiResponse>

  const headers: Record<string, string> = { 'Content-Type': 'application/json', ...authHeaders() }
  if (!['GET', 'HEAD', 'OPTIONS'].includes(method.toUpperCase())) {
    headers['X-Clash-CSRF'] = '1'
  }

  const options: RequestInit = {
    method,
    headers,
    credentials: 'same-origin',
  }

  if (data !== undefined) {
    options.body = JSON.stringify(data)
  }

  const response = await fetch(`${API_BASE}${url}`, options)
  const parsed = await parseResponse(response)

  if (!response.ok) {
    if (response.status === 401) {
      window.dispatchEvent(new CustomEvent('auth:unauthorized'))
    }
    const error: ApiError = new Error(buildErrorMessage(parsed, `HTTP ${response.status}`))
    error.response = { status: response.status, data: parsed }
    error.userMessage = error.message
    throw error
  }

  return { data: parsed, status: response.status, headers: response.headers }
}

const api = {
  get: (url: string) => request('GET', url),
  post: (url: string, data?: any) => request('POST', url, data),
  patch: (url: string, data?: any) => request('PATCH', url, data),
  delete: (url: string) => request('DELETE', url),
}

export function getApiErrorMessage(err: any, fallback: string = '请求失败'): string {
  if (err?.userMessage) {
    return humanizeGroupError(err.userMessage)
  }
  return humanizeGroupError(buildErrorMessage(err?.response?.data, err?.message || fallback))
}

function humanizeGroupError(message: string): string {
  const text = String(message || '')
  if (!text) return text
  if (text.includes('Circular node group reference detected')) {
    return text.replace(
      'Circular node group reference detected',
      '策略组引用存在循环',
    ) + '。加/减策略组引用都不能形成环。'
  }
  if (text.includes('Node group cannot include itself')) {
    return '策略组不能引用自己（加/减都不行）。'
  }
  if (text.includes('Referenced node groups do not exist')) {
    return text.replace('Referenced node groups do not exist', '引用的策略组不存在')
  }
  return text
}

// 订阅管理
export const getSubscriptions = () => api.get('/subscriptions')
export const createSubscription = (data: any) => api.post('/subscriptions', data)
export const createManualNodeSubscription = (data: any) => api.post('/subscriptions/manual-node', data)
export const updateSubscription = (id: number, data: any) => api.patch(`/subscriptions/${id}`, data)
export const deleteSubscription = (id: number) => api.delete(`/subscriptions/${id}`)
export const fetchSubscription = (id: number) => api.post(`/subscriptions/${id}/fetch`)
export const getSubscriptionNodes = (id: number) => api.get(`/subscriptions/${id}/nodes`)
export const getAllSubscriptionNodes = () => api.get('/subscriptions/nodes/all')
// 仅测试 TCP 连通性，非代理延迟或协议握手
export const probeTcp = (payload: { nodes: any[]; timeout_ms?: number; concurrency?: number }) =>
  api.post('/probe/tcp', payload)

// 节点深度能力与测速探针
export const getProbeSettings = () => api.get('/probe/settings')
export const updateProbeSettings = (data: any) => api.patch('/probe/settings', data)
export const probeNode = (data: { node: any; include_speed?: boolean; include_media?: boolean; use_cache?: boolean }) =>
  api.post('/probe/node', data)
export const probeBatch = (data: {
  nodes: any[]
  include_speed?: boolean
  include_media?: boolean
  concurrency?: number
  use_cache?: boolean
  timeout_ms?: number
}) => api.post('/probe/batch', data)
export const probeNodesFull = (data: {
  nodes: any[]
  use_settings?: boolean
  include_speed?: boolean
  include_media?: boolean
  concurrency?: number
  use_cache?: boolean
  timeout_ms?: number
}) => api.post('/probe/batch', data)
export const getProbeCache = () => api.get('/probe/cache')
export const getProbeResults = () => api.get('/probe/results')
export const clearProbeCache = () => api.delete('/probe/cache')
export const clearProbeResults = () => api.delete('/probe/results')

// 前置代理链
export const getProxyChains = () => api.get('/proxy-chains')
export const createProxyChain = (data: any) => api.post('/proxy-chains', data)
export const updateProxyChain = (id: number, data: any) => api.patch(`/proxy-chains/${id}`, data)
export const deleteProxyChain = (id: number) => api.delete(`/proxy-chains/${id}`)
export const getFinalNodes = () => api.get('/proxy-chains/meta/final-nodes')
export const getNodeLedger = () => api.get('/proxy-chains/meta/node-ledger')
export const previewProxyChain = (data: any) => api.post('/proxy-chains/meta/preview', data)

// 策略组
export const getNodeGroups = () => api.get('/node-groups')
export const createNodeGroup = (data: any) => api.post('/node-groups', data)
export const updateNodeGroup = (id: number, data: any) => api.patch(`/node-groups/${id}`, data)
export const deleteNodeGroup = (id: number) => api.delete(`/node-groups/${id}`)
export const reorderNodeGroups = (items: any[]) => api.post('/node-groups/reorder', { items })
export const validateNodeGroups = () => api.post('/node-groups/validate')
export const previewNodeGroups = () => api.get('/node-groups/_preview')

// Rule Categories
export const getRuleCategories = () => api.get('/rule-categories')
export const createRuleCategory = (data: any) => api.post('/rule-categories', data)
export const updateRuleCategory = (id: number, data: any) => api.patch(`/rule-categories/${id}`, data)
export const deleteRuleCategory = (id: number) => api.delete(`/rule-categories/${id}`)
export const reorderRuleCategories = (items: any[]) => api.post('/rule-categories/reorder', { items })
export const batchRuleCategories = (payload: any) => api.post('/rule-categories/batch', payload)

// Rules
export const getRules = () => api.get('/rules')
export const createRule = (data: any) => api.post('/rules', data)
export const updateRule = (id: number, data: any) => api.patch(`/rules/${id}`, data)
export const deleteRule = (id: number) => api.delete(`/rules/${id}`)
export const reorderRules = (items: any[]) => api.post('/rules/reorder', { items })
export const batchRules = (payload: any) => api.post('/rules/batch', payload)

// DNS
export const getDns = () => api.get('/dns')
export const updateDns = (data: any) => api.patch('/dns', data)

// Settings
export const getSecuritySettings = () => api.get('/settings/security')
export const updateSecuritySettings = (data: any) => api.patch('/settings/security', data)
export const checkAuthToken = (token: string) => api.post('/settings/auth/check', { token })
export const checkAuthSession = () => api.post('/settings/auth/check')
export const loginAuthToken = (token: string) => api.post('/settings/auth/login', { token })
export const logoutAuthToken = () => api.post('/settings/auth/logout')
export const exportAppConfig = (includeSubscriptions: boolean = true): Promise<Response> => {
  if (DEMO_MODE) return mockFetchExport(includeSubscriptions) as Promise<Response>
  return fetch(`/api/settings/export?include_subscriptions=${includeSubscriptions ? 'true' : 'false'}`, {
    headers: { ...authHeaders() },
    credentials: 'same-origin',
  })
}
export const resetAppConfig = () => api.post('/settings/reset')
export const importAppConfig = (data: any) => api.post('/settings/import', data)

// Downloads
export const getDownloads = () => api.get('/downloads')
export const downloadPresetAsset = (presetId: string) => api.post('/downloads/preset', { preset_id: presetId })
export const downloadCustomAsset = (url: string) => api.post('/downloads/custom', { url })

// Generate
export const getGenerateSettings = () => api.get('/generate/settings')
export const updateGenerateSettings = (data: any) => api.patch('/generate/settings', data)
export const generateScript = (data: any) => api.post('/generate/script', data)
export const generateYaml = (data: any) => api.post('/generate/yaml', data)
export const generateSubscriptionYaml = (id: number) => api.post(`/generate/subscription/${id}`)

// Snapshots
export const getSnapshots = () => api.get('/snapshots')
export const createSnapshot = (data: any) => api.post('/snapshots', data)
export const getSnapshot = (id: number) => api.get(`/snapshots/${id}`)
export const getSnapshotData = (id: number) => api.get(`/snapshots/${id}/data`)
export const restoreSnapshot = (id: number) => api.post(`/snapshots/${id}/restore`)
export const deleteSnapshot = (id: number) => api.delete(`/snapshots/${id}`)

// v2 规范化接口
export const getInventoryNodesV2 = (params?: { limit?: number; offset?: number; search?: string }) => {
  const query = new URLSearchParams()
  if (params?.limit) query.set('limit', String(params.limit))
  if (params?.offset) query.set('offset', String(params.offset))
  if (params?.search) query.set('search', params.search)
  const qs = query.toString()
  return api.get(`/v2/inventory/nodes${qs ? '?' + qs : ''}`)
}
export const getInventorySourcesV2 = () => api.get('/v2/inventory/sources')
export const preflightBundleV2 = (bundle: any) => api.post('/v2/bundle/preflight', bundle)
export const compileBundleV2 = (bundle: any) => api.post('/v2/compile', bundle)
export const getRevisionsV2 = () => api.get('/v2/revisions')
export const createRevisionV2 = (bundle: any, author: string = 'web-ui', changeSummary: string = 'web update') =>
  api.post('/v2/revisions', bundle)
export const rollbackRevisionV2 = (id: string) => api.post(`/v2/revisions/${id}/rollback`)
export const getProbeProfilesV2 = () => api.get('/v2/probe/profiles')
export const getProbeJobsV2 = () => api.get('/v2/probe/jobs')
export const getProbeObservationsV2 = (params?: { node_id?: string; limit?: number }) => {
  const query = new URLSearchParams()
  if (params?.node_id) query.set('node_id', params.node_id)
  if (params?.limit) query.set('limit', String(params.limit))
  const qs = query.toString()
  return api.get(`/v2/probe/observations${qs ? '?' + qs : ''}`)
}
export const getReadinessV2 = () => api.get('/v2/readiness')

export default api
