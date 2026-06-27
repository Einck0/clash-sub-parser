import { authHeaders } from '../auth'
import { mockFetchExport, mockRequest } from './mock'

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

async function parseResponse(response: Response): Promise<any> {
  const contentType = response.headers.get('content-type') || ''
  if (contentType.includes('application/json')) {
    return response.json()
  }
  return response.text()
}

function buildErrorMessage(data: any, fallback: string): string {
  const detail = data?.detail
  if (Array.isArray(detail)) {
    return detail.map((item: any) => item.msg || JSON.stringify(item)).join('; ')
  }
  if (detail) return String(detail)
  return fallback
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
  if (err?.userMessage) return err.userMessage
  return buildErrorMessage(err?.response?.data, err?.message || fallback)
}

// Subscriptions
export const getSubscriptions = () => api.get('/subscriptions')
export const createSubscription = (data: any) => api.post('/subscriptions', data)
export const createManualNodeSubscription = (data: any) => api.post('/subscriptions/manual-node', data)
export const updateSubscription = (id: number, data: any) => api.patch(`/subscriptions/${id}`, data)
export const deleteSubscription = (id: number) => api.delete(`/subscriptions/${id}`)
export const fetchSubscription = (id: number) => api.post(`/subscriptions/${id}/fetch`)
export const getSubscriptionNodes = (id: number) => api.get(`/subscriptions/${id}/nodes`)
export const getAllSubscriptionNodes = () => api.get('/subscriptions/nodes/all')

// Node Groups
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
export const getRule = (id: number) => api.get(`/rules/${id}`)
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

// Latency
export const checkLatency = (hosts: string[], timeoutMs: number = 3000) =>
  api.post('/latency/check', { hosts, timeout_ms: timeoutMs })

export default api
