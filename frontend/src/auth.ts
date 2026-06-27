const TOKEN_KEY = '***'
const TOKEN_PARAM = 'token'
let sessionToken = ''

export function getAuthToken(): string {
  if (sessionToken) return sessionToken
  return localStorage.getItem(TOKEN_KEY) || ''
}

export function setAuthToken(token: string): void {
  sessionToken = token || ''
  localStorage.setItem(TOKEN_KEY, token || '')
}

export function authHeaders(): Record<string, string> {
  const token = getAuthToken()
  return token ? { 'X-Clash-Token': token } : {}
}

export function withAuthToken(url: string, enabled: boolean = true): string {
  const token = getAuthToken()
  if (!enabled || !token) return url
  const separator = url.includes('?') ? '&' : '?'
  return `${url}${separator}token=${encodeURIComponent(token)}`
}

export function syncTokenFromUrl(): string {
  const url = new URL(window.location.href)
  const token = url.searchParams.get(TOKEN_PARAM)
  if (!token) return getAuthToken()

  setAuthToken(token)
  url.searchParams.delete(TOKEN_PARAM)
  const clean = `${url.pathname}${url.search}${url.hash}`
  window.history.replaceState({}, '', clean || '/')
  return token
}
