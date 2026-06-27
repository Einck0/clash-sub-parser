const TOKEN_PARAM = 'token'
let sessionToken = ''

export function syncTokenFromUrl() {
  const url = new URL(window.location.href)
  const token = url.searchParams.get(TOKEN_PARAM)
  if (!token) return getAuthToken()

  setAuthToken(token)  // Store token from URL
  url.searchParams.delete(TOKEN_PARAM)
  const clean = `${url.pathname}${url.search}${url.hash}`
  window.history.replaceState({}, '', clean || '/')
  return token
}

export function getAuthToken() {
  return sessionToken
}

export function setAuthToken(token) {
  sessionToken = token || ''
}

export function authHeaders() {
  const token = getAuthToken()
  return token ? { 'X-Clash-Token': token } : {}
}

export function withAuthToken(url, enabled = true) {
  const token = getAuthToken()
  if (!enabled || !token) return url
  const parsed = new URL(url, window.location.origin)
  parsed.searchParams.set(TOKEN_PARAM, token)
  return parsed.toString()
}
