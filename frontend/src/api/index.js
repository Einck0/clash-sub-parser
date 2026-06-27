import { authHeaders } from '../auth'
import { mockFetchExport, mockRequest } from './mock'

const API_BASE = '/api'
const DEMO_MODE = import.meta.env.VITE_DEMO_MODE === 'true'

async function parseResponse(response) {
  const contentType = response.headers.get('content-type') || ''
  if (contentType.includes('application/json')) {
    return response.json()
  }
  return response.text()
}

function buildErrorMessage(data, fallback) {
  const detail = data?.detail
  if (Array.isArray(detail)) {
    return detail.map((item) => item.msg || JSON.stringify(item)).join('; ')
  }
  if (detail) return String(detail)
  return fallback
}

async function request(method, url, data) {
  if (DEMO_MODE) return mockRequest(method, url, data)

  const headers = { 'Content-Type': 'application/json', ...authHeaders() }
  // CSRF defense-in-depth: custom header ensures browsers won't send this cross-origin.
  // The fixed value '1' is intentional - protection comes from the header being custom,
  // not from the value being secret. Cookie auth + CSRF header = safe combo.
  if (!['GET', 'HEAD', 'OPTIONS'].includes(method.toUpperCase())) {
    headers['X-Clash-CSRF'] = '1'
  }

  const options = {
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
    const error = new Error(buildErrorMessage(parsed, `HTTP ${response.status}`))
    error.response = { status: response.status, data: parsed }
    error.userMessage = error.message
    throw error
  }

  return { data: parsed, status: response.status, headers: response.headers }
}

const api = {
  get: (url) => request('GET', url),
  post: (url, data) => request('POST', url, data),
  patch: (url, data) => request('PATCH', url, data),
  delete: (url) => request('DELETE', url),
}

export function getApiErrorMessage(err, fallback = '请求失败') {
  if (err?.userMessage) return err.userMessage
  return buildErrorMessage(err?.response?.data, err?.message || fallback)
}

// Subscriptions
export const getSubscriptions = () => api.get('/subscriptions')
export const createSubscription = (data) => api.post('/subscriptions', data)
export const createManualNodeSubscription = (data) => api.post('/subscriptions/manual-node', data)
export const updateSubscription = (id, data) => api.patch(`/subscriptions/${id}`, data)
export const deleteSubscription = (id) => api.delete(`/subscriptions/${id}`)
export const fetchSubscription = (id) => api.post(`/subscriptions/${id}/fetch`)
export const getSubscriptionNodes = (id) => api.get(`/subscriptions/${id}/nodes`)
export const getAllSubscriptionNodes = () => api.get('/subscriptions/nodes/all')

// Node Groups
export const getNodeGroups = () => api.get('/node-groups')
export const createNodeGroup = (data) => api.post('/node-groups', data)
export const updateNodeGroup = (id, data) => api.patch(`/node-groups/${id}`, data)
export const deleteNodeGroup = (id) => api.delete(`/node-groups/${id}`)
export const reorderNodeGroups = (items) => api.post('/node-groups/reorder', { items })
export const validateNodeGroups = () => api.post('/node-groups/validate')
export const previewNodeGroups = () => api.get('/node-groups/_preview')

// Rule Categories
export const getRuleCategories = () => api.get('/rule-categories')
export const createRuleCategory = (data) => api.post('/rule-categories', data)
export const updateRuleCategory = (id, data) => api.patch(`/rule-categories/${id}`, data)
export const deleteRuleCategory = (id) => api.delete(`/rule-categories/${id}`)
export const reorderRuleCategories = (items) => api.post('/rule-categories/reorder', { items })

// Rules
export const getRules = () => api.get('/rules')
export const getRule = (id) => api.get(`/rules/${id}`)
export const createRule = (data) => api.post('/rules', data)
export const updateRule = (id, data) => api.patch(`/rules/${id}`, data)
export const deleteRule = (id) => api.delete(`/rules/${id}`)
export const reorderRules = (items) => api.post('/rules/reorder', { items })

// DNS
export const getDns = () => api.get('/dns')
export const updateDns = (data) => api.patch('/dns', data)

// Settings
export const getSecuritySettings = () => api.get('/settings/security')
export const updateSecuritySettings = (data) => api.patch('/settings/security', data)
export const checkAuthToken = (token) => api.post('/settings/auth/check', { token })
export const checkAuthSession = () => api.post('/settings/auth/check')
export const loginAuthToken = (token) => api.post('/settings/auth/login', { token })
export const logoutAuthToken = () => api.post('/settings/auth/logout')
export const exportAppConfig = (includeSubscriptions = true) => {
  if (DEMO_MODE) return mockFetchExport(includeSubscriptions)
  return fetch(`/api/settings/export?include_subscriptions=${includeSubscriptions ? 'true' : 'false'}`, {
    headers: { ...authHeaders() },
    credentials: 'same-origin',
  })
}
export const resetAppConfig = () => api.post('/settings/reset')
export const importAppConfig = (data) => api.post('/settings/import', data)

// Downloads
export const getDownloads = () => api.get('/downloads')
export const downloadPresetAsset = (presetId) => api.post('/downloads/preset', { preset_id: presetId })
export const downloadCustomAsset = (url) => api.post('/downloads/custom', { url })

// Generate
export const getGenerateSettings = () => api.get('/generate/settings')
export const updateGenerateSettings = (data) => api.patch('/generate/settings', data)
export const generateScript = (data) => api.post('/generate/script', data)
export const generateYaml = (data) => api.post('/generate/yaml', data)
export const generateSubscriptionYaml = (id) => api.post(`/generate/subscription/${id}`)

export default api
