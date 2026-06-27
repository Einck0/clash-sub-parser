/**
 * Shared constants and utilities for rule editing views.
 */

export const BUILTINS = ['DIRECT', 'PASS', 'REJECT']

export const RULE_TYPES = [
  'DOMAIN', 'DOMAIN-SUFFIX', 'DOMAIN-KEYWORD', 'DOMAIN-REGEX',
  'PROCESS-NAME', 'PROCESS-PATH', 'IP-CIDR', 'IP-CIDR6',
  'GEOIP', 'GEOSITE', 'DST-PORT', 'SRC-IP-CIDR', 'SRC-PORT',
  'RULE-SET', 'MATCH',
]

export function normalizeRuleType(type) {
  return String(type || '').trim().toUpperCase()
}

export function parseOptions(text) {
  return String(text || '').split(',').map((s) => s.trim()).filter(Boolean)
}

export function proxyTargetsFromGroups(groups) {
  return [...BUILTINS, ...groups.map((g) => g.name)]
}
