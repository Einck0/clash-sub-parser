import type { ToastTone } from '../../ui/toast'

export type CapabilityStatus = 'available' | 'restricted' | 'unknown' | 'error' | 'stale' | 'missing'

export interface NodeRecord {
  logical_id: string
  protocol: string
  display_name: string
  active: boolean
  credential_version?: number
  created_at?: string
  updated_at?: string
  capabilities?: Record<string, CapabilityStatus>
  probe_stale?: boolean
  probe_missing?: boolean
  credential_mismatch?: boolean
}

export interface NormalizedNode {
  logicalId: string
  protocol: string
  displayName: string
  active: boolean
  credentialVersion?: number
  createdAt?: string
  updatedAt?: string
  capabilities: Record<string, CapabilityStatus>
  probeStale?: boolean
  probeMissing?: boolean
  credentialMismatch?: boolean
}

export function maskSecretReference(_value: string): string {
  return '***'
}

export function normalizeNode(node: NodeRecord): NormalizedNode {
  const probeMissing = node.probe_missing ?? (!node.capabilities || Object.keys(node.capabilities).length === 0)
  const probeStale = node.probe_stale ?? Object.values(node.capabilities ?? {}).some((s) => s === 'stale')
  return {
    logicalId: node.logical_id,
    protocol: node.protocol,
    displayName: node.display_name.trim() || node.logical_id,
    active: node.active,
    credentialVersion: node.credential_version,
    createdAt: node.created_at,
    updatedAt: node.updated_at,
    capabilities: node.capabilities ?? {},
    probeStale,
    probeMissing,
    credentialMismatch: Boolean(node.credential_mismatch),
  }
}

const capabilityLabels: Record<CapabilityStatus, { label: string; tone: ToastTone }> = {
  available: { label: 'Available', tone: 'success' },
  restricted: { label: 'Restricted', tone: 'warning' },
  unknown: { label: 'Unknown', tone: 'info' },
  error: { label: 'Error', tone: 'error' },
  stale: { label: 'Stale', tone: 'warning' },
  missing: { label: 'Missing', tone: 'info' },
}

export function nodeCapabilityLabel(node: NodeRecord | NormalizedNode, capability: string) {
  return capabilityLabels[node.capabilities?.[capability] ?? 'unknown']
}
