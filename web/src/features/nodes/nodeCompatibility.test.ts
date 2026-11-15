import { describe, expect, it } from 'vitest'
import { normalizeNode, nodeSupportedTargets, nodeTargetDiagnostics, renderNodePreview } from './nodeView'

describe('target-scoped node compatibility', () => {
  it('uses static protocol support only when backend compatibility is absent', () => {
    const record = { logical_id: 'old', display_name: 'Legacy API', protocol: 'ss', active: true }
    expect(nodeSupportedTargets(normalizeNode(record))).toEqual(['mihomo', 'singbox', 'surge', 'qx'])
    const withoutTarget = normalizeNode({ ...record, target_compatibility: {} })
    expect(nodeSupportedTargets(withoutTarget)).toEqual([])
    expect(renderNodePreview(withoutTarget, 'mihomo')).toContain('后端未提供该目标兼容状态')
    expect(nodeSupportedTargets(normalizeNode({ ...record, target_compatibility: {
      mihomo: { status: 'incompatible' }, singbox: { status: 'compatible' },
    } }))).toEqual(['singbox'])
  })

  it('shows a stored format error without offering an export preview', () => {
    const node = normalizeNode({
      logical_id: 'corrupt', display_name: 'Corrupt node', protocol: 'ss', active: true,
      server: '192.0.2.1', port: 8388,
      target_compatibility: {
        mihomo: { status: 'format_error', diagnostics: [{ code: 'format_error', message: 'missing_credential: password' }] },
      },
    })
    expect(nodeSupportedTargets(node)).toEqual([])
    expect(nodeTargetDiagnostics(node)).toEqual(['mihomo: missing_credential: password'])
    expect(renderNodePreview(node, 'mihomo')).toContain('不参与 mihomo 导出')
    expect(renderNodePreview(node, 'mihomo')).not.toContain('proxies:')
  })

  it('keeps a saved node visible but replaces incompatible target preview with its reason', () => {
    const node = normalizeNode({
      logical_id: 'legacy', display_name: 'Legacy SS', protocol: 'ss', active: true,
      server: '192.0.2.1', port: 8388,
      credentials: { method: 'aes-128-gcm', password: 'test' },
      target_compatibility: {
        mihomo: { status: 'incompatible', diagnostics: [{ target: 'mihomo', code: 'unsupported_target_capability', message: 'unsupported transport skip_cert_verify' }] },
        singbox: { status: 'compatible' },
      },
    })
    expect(node.active).toBe(true)
    expect(nodeSupportedTargets(node)).toEqual(['singbox'])
    expect(nodeTargetDiagnostics(node)).toEqual(['mihomo: unsupported transport skip_cert_verify'])
    expect(renderNodePreview(node, 'mihomo')).toContain('不参与 mihomo 导出')
    expect(renderNodePreview(node, 'mihomo')).not.toContain('proxies:')
    expect(renderNodePreview(node, 'singbox')).toContain('"outbounds"')
  })
})
