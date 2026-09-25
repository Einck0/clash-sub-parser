import type { ToastTone } from '../../ui/toast'

export type CapabilityStatus = 'available' | 'restricted' | 'unknown' | 'error' | 'stale'

export interface NodeRecord {
  logical_id: string
  protocol: string
  display_name: string
  active: boolean
  created_at?: string
  updated_at?: string
  capabilities?: Record<string, CapabilityStatus>
}

export interface NormalizedNode {
  logicalId: string
  protocol: string
  displayName: string
  active: boolean
  createdAt?: string
  updatedAt?: string
  capabilities: Record<string, CapabilityStatus>
}

export function maskSecretReference(_value: string): string {
  return '***'
}

export function normalizeNode(node: NodeRecord): NormalizedNode {
  return {
    logicalId: node.logical_id,
    protocol: node.protocol,
    displayName: node.display_name.trim() || node.logical_id,
    active: node.active,
    createdAt: node.created_at,
    updatedAt: node.updated_at,
    capabilities: node.capabilities ?? {},
  }
}

const capabilityLabels: Record<CapabilityStatus, { label: string; tone: ToastTone }> = {
  available: { label: 'Available', tone: 'success' },
  restricted: { label: 'Restricted', tone: 'warning' },
  unknown: { label: 'Unknown', tone: 'info' },
  error: { label: 'Error', tone: 'error' },
  stale: { label: 'Stale', tone: 'warning' },
}

export function nodeCapabilityLabel(node: NodeRecord | NormalizedNode, capability: string) {
  return capabilityLabels[node.capabilities?.[capability] ?? 'unknown']
}
