// DTO identifiers stay in keys/request values. Names and diagnostic prose are separate boundaries.
const objectToken = /\b(node|group|sub|subscription|payload|binding|snapshot|run|batch|obs|probe|chain|policy|rule|entry|fetch|parse)_[a-z0-9_-]+\b/gi
const legacyObjectToken = /\b(node|group|sub|payload|binding|snapshot|run|batch|obs|probe|entry|fetch)-[a-z0-9]+(?:-[a-z0-9]+)*\b/g
const uuidToken = /\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b/gi
const fingerprintToken = /\b(?:sha256:[a-z0-9_-]+|[0-9a-f]{32,})\b/gi
const objectLabels: Record<string, string> = {
  node: '节点', group: '策略组', sub: '订阅', subscription: '订阅', payload: '订阅内容',
  binding: '绑定', snapshot: '快照', run: '检测任务', batch: '巡检批次', obs: '检测记录',
  probe: '检测记录', chain: '链式代理', policy: '策略', rule: '规则', entry: '订阅条目', fetch: '获取记录', parse: '解析记录',
}

export function displayName(name: string | null | undefined, fallback: string, internalId?: string): string {
  const value = name?.trim()
  if (!value || value === internalId) return fallback
  // A machine key is not a business name. Never rewrite legitimate user-authored names.
  if (displayText(value) !== value) return fallback
  return value
}

/** Redact only recognizable object-key tokens, preserving the diagnostic and its recovery advice. */
export function displayText(text: string | null | undefined): string {
  return (text || '')
    .replace(objectToken, (_token, kind: string) => `${objectLabels[kind.toLowerCase()] || '内部标识'}（标识已省略）`)
    .replace(legacyObjectToken, (_token, kind: string) => `${objectLabels[kind] || '内部标识'}（标识已省略）`)
    .replace(uuidToken, '内部标识（已省略）')
    .replace(fingerprintToken, '校验指纹（已省略）')
}
