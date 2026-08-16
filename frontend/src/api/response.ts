
export async function parseResponse(response: Response): Promise<unknown> {
  if (response.status === 204 || response.status === 205) return null

  const contentType = response.headers.get('content-type') || ''
  const text = await response.text()
  if (!text) return null
  if (!contentType.includes('application/json')) return text

  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}

export function buildErrorMessage(data: unknown, fallback: string): string {
  const detail = data && typeof data === 'object' && 'detail' in data
    ? data.detail
    : undefined
  if (Array.isArray(detail)) {
    return detail.map((item) => {
      if (item && typeof item === 'object' && 'msg' in item) return String(item.msg)
      return JSON.stringify(item)
    }).join('; ')
  }
  if (detail !== undefined && detail !== null && detail !== '') {
    return typeof detail === 'object' ? JSON.stringify(detail) : String(detail)
  }
  return fallback
}
