/**
 * Shared formatting utilities — single source of truth.
 */

export function formatBytes(value) {
  const n = Number(value || 0)
  if (!Number.isFinite(n) || n <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const idx = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)))
  return `${(n / Math.pow(1024, idx)).toFixed(idx === 0 ? 0 : 2)} ${units[idx]}`
}

export function formatDate(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

export function short(value, length = 44) {
  if (!value) return ''
  return value.length > length ? `${value.slice(0, length)}...` : value
}

export function formatLocalTime(utcString) {
  if (!utcString) return '-'
  const d = new Date(String(utcString).endsWith('Z') ? utcString : utcString + 'Z')
  if (Number.isNaN(d.getTime())) return '-'
  return d.toLocaleString()
}
