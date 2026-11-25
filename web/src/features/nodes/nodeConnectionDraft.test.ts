// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import NodeDetailDrawer from './NodeDetailDrawer.vue'
import { normalizeNode, nodeCapabilityLabel, nodeHealthDiagnostic, nodeRiskBadge, validateNodeConnectionProfile, type NodeConnectionProfile, type NodeRecord, type NormalizedNode } from './nodeView'
import { attributionCauseLabel, attributionStatusBadge, relationStateBadge, formatObservedTime } from './sourceHistoryTypes'
import { setLocale } from '../../locales'
import { api } from '../../api/client'
import { useNodes } from './useNodes'

vi.mock('../../ui/DrawerCard.vue', () => ({ default: {
  props: ['modelValue', 'title', 'description'],
  template: '<section v-if="modelValue"><button data-testid="shell-close" @click="$emit(\'update:modelValue\', false); $emit(\'close\')">Close</button><slot/><slot name="footer"/></section>',
} }))
vi.mock('../../ui/ConfirmModal.vue', () => ({ default: {
  props: ['modelValue', 'title', 'message', 'confirmText', 'tone'],
  template: '<section v-if="modelValue"><button data-testid="discard-confirm" @click="$emit(\'confirm\')">Confirm</button><button data-testid="discard-cancel" @click="$emit(\'update:modelValue\', false); $emit(\'cancel\')">Cancel</button></section>',
} }))

const fixture = (id = 'node-a', server = 'initial.example') => normalizeNode({
  logical_id: id, display_name: id === 'node-a' ? 'Alpha' : 'Beta', protocol: 'ss', active: true,
  connection_revision: 1, server, port: 443, credentials: { method: 'aes-128-gcm', password: 'fixture-only' },
})
type SavePayload = { draft: Partial<NodeConnectionProfile> & { displayName?: string }; onSuccess: (node: NormalizedNode) => void; onError: (error: string) => void }
let app: ReturnType<typeof createApp> | null = null
let host: HTMLDivElement
beforeEach(() => { setLocale('zh-CN'); host = document.createElement('div'); document.body.appendChild(host) })
afterEach(() => { app?.unmount(); app = null; host.remove(); setLocale('zh-CN'); vi.restoreAllMocks() })

function mountEditor(loading = false, initialNode = fixture()) {
  const node = ref(initialNode)
  const loadingDetail = ref(loading)
  const savingConnection = ref(false)
  const close = vi.fn()
  const save = vi.fn<(payload: SavePayload) => void>()
  app = createApp({ render: () => h(NodeDetailDrawer, {
    modelValue: true, node: node.value, sourceHistory: null, loadingSourceHistory: false,
    sourceHistoryError: '', probeState: 'idle', probeFeedback: '', savingConnection: savingConnection.value,
    loadingDetail: loadingDetail.value,
    'onUpdate:modelValue': close, onSaveConnection: save,
  }) })
  app.mount(host)
  return { node, loadingDetail, savingConnection, close, save }
}
function button(id: string) { return host.querySelector<HTMLButtonElement>(`[data-testid="${id}"]`)! }
function server() { return host.querySelector<HTMLInputElement>('[data-testid="node-server-input"]')! }
async function editServer(value = 'draft.example') {
  server().value = value
  server().dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
}
async function submit() {
  host.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  await nextTick()
}

describe('connection draft isolation', () => {
  it('switches visible editor and source labels without altering the draft or user data', async () => {
    const { node, save } = mountEditor()
    node.value = { ...node.value, displayName: '用户原名' }
    await editServer('kept.example')
    expect(host.textContent).toContain('服务器地址 / IP')
    expect(host.textContent).toContain('历史归属仅用于追溯，不代表当前订阅成员或启用状态')
    setLocale('en-US')
    await nextTick()
    expect(host.textContent).toContain('Server address / IP')
    expect(host.textContent).toContain('Health: Unknown')
    expect(host.textContent).toContain('Latency:')
    expect(host.textContent).toContain('AI access: Unknown')
    expect(host.textContent).toContain('IP risk: Unknown risk')
    expect(host.textContent).toContain('Subscription provenance and automatic synchronization')
    expect(host.textContent).toContain('Historical attribution is for auditing only')
    expect(host.textContent).toContain('Node export format preview')
    expect(host.querySelector<HTMLInputElement>('[data-testid="node-password-input"]')?.placeholder).toBe('Enter the protocol password or key')
    expect(server().value).toBe('kept.example')
    expect(node.value.displayName).toBe('用户原名')
    expect(host.textContent).toContain('用户原名')
    await submit()
    save.mock.calls[0]![0].onSuccess(fixture('node-a', 'kept.example'))
    await nextTick()
    expect(host.textContent).toContain('Connection parameters saved')
    setLocale('zh-CN')
    await nextTick()
    expect(host.textContent).toContain('连接参数已保存')
    expect(host.textContent).toContain('服务器地址 / IP')
    expect(server().value).toBe('kept.example')
  })

  it('translates protocol editor options and placeholders but preserves field IDs', async () => {
    const { node } = mountEditor()
    setLocale('en-US')
    node.value = normalizeNode({ logical_id: 'wg', protocol: 'wireguard', active: true, server: 'wg.example', port: 51820 })
    await nextTick()
    expect(host.textContent).toContain('WireGuard endpoint and peer configuration')
    expect(host.querySelector<HTMLInputElement>('[data-testid="wg-public-key-input"]')?.placeholder).toBe('Base64 peer public key')
    node.value = normalizeNode({ logical_id: 'tuic', protocol: 'tuic', active: true, server: 'tuic.example', port: 443 })
    await nextTick()
    expect(host.textContent).toContain('TUIC v5 connection and QUIC transport parameters')
    expect(host.querySelector('option')?.textContent).toBe('(Default / not set)')
    expect(host.textContent).toContain('Disable SNI (disable_sni)')
    setLocale('zh-CN')
    await nextTick()
    expect(host.querySelector('option')?.textContent).toBe('（默认 / 未设置）')
  })

  it('localizes node diagnostics, validation and attribution at call time', () => {
    const node = normalizeNode({ logical_id: 'diag', protocol: 'ss', active: true, capabilities: { baseline: { verdict: 'error', summary: 'reason=private_target_rejected' }, ip_risk: { verdict: 'available', risk_score: '25' } } })
    expect(attributionStatusBadge('historical_verified').label).toBe('历史已核验归属')
    expect(validateNodeConnectionProfile('ss', {})).toBe('服务器地址不能为空')
    setLocale('en-US')
    expect(nodeHealthDiagnostic(node)?.shortLabel).toBe('Security rejection: private target address')
    expect(nodeHealthDiagnostic(node)?.detail).toContain('No outbound connection was attempted')
    expect(nodeCapabilityLabel(node, 'baseline').label).toBe('Probe failed')
    expect(nodeRiskBadge(node).label).toBe('Medium risk (25)')
    expect(validateNodeConnectionProfile('ss', {})).toBe('Server address is required')
    expect(attributionStatusBadge('historical_verified').label).toBe('Verified historical attribution')
    expect(attributionCauseLabel('subscription_deleted')).toBe('Subscription source deleted')
    expect(relationStateBadge('unknown').label).toBe('Pending verification / unknown')
    expect(formatObservedTime()).toBe('No record')
    setLocale('zh-CN')
    expect(nodeHealthDiagnostic(node)?.shortLabel).toBe('安全拒绝：私有目标地址')
    expect(nodeRiskBadge(node).label).toBe('中风险 (25)')
  })

  it('waits for the first detail hydration, then ignores same-entity DTO and pool replacements', async () => {
    const { node, loadingDetail } = mountEditor(true)
    await nextTick()
    expect(server()).toBeNull()
    node.value = fixture('node-a', 'hydrated.example')
    loadingDetail.value = false
    await nextTick()
    expect(server().value).toBe('hydrated.example')
    await editServer()
    node.value = { ...fixture('node-a', 'polled.example'), probeState: 'queued' }
    await nextTick()
    expect(server().value).toBe('draft.example')
    expect(host.querySelector('[data-testid="node-connection-dirty"]')).not.toBeNull()
  })

  it('handles shell close once, keeps edits on cancel, and emits close only after discard confirmation', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const { close } = mountEditor()
    await editServer()
    button('shell-close').click()
    await nextTick()
    expect(close).not.toHaveBeenCalled()
    button('discard-cancel').click()
    await nextTick()
    expect(server().value).toBe('draft.example')
    expect(button('discard-confirm')).toBeNull()
    button('node-drawer-close-btn').click()
    await nextTick()
    button('discard-confirm').click()
    await nextTick()
    expect(close).toHaveBeenCalledOnce()
    expect(close).toHaveBeenCalledWith(false)
    expect(confirm).not.toHaveBeenCalled()
  })

  it('blocks close while a save is pending and registers no routerless navigation warning', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const { savingConnection, close } = mountEditor()
    await editServer()
    savingConnection.value = true
    await nextTick()
    button('shell-close').click()
    button('node-drawer-close-btn').click()
    await nextTick()
    expect(close).not.toHaveBeenCalled()
    expect(button('discard-confirm')).toBeNull()
    expect(server().value).toBe('draft.example')
    expect(warn.mock.calls.flat().join(' ')).not.toContain('onBeforeRouteLeave')
    const unload = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(unload)
    expect(unload.defaultPrevented).toBe(true)
  })

  it.each(['ss', 'tuic'])('distinguishes unchanged %s optional TLS fields from explicit clears', async (protocol) => {
    const existing = normalizeNode({
      logical_id: 'node-a', display_name: 'Alpha', protocol, active: true,
      connection_revision: 2, server: 'fixture.example', port: 443,
      credentials: { method: 'aes-128-gcm', uuid: '11111111-2222-4333-8444-555555555555', password: 'fixture-password', sni: 'sni.example', alpn: ['h2'] },
    })
    const { save } = mountEditor(false, existing)
    expect(host.textContent).toContain('自定义 SNI（可选），留空恢复默认')
    expect(host.textContent).toContain('自定义 ALPN（可选），留空恢复默认')
    expect(host.querySelector('[data-testid="node-preview-scope-note"]')?.textContent).toContain('不是完整编译输出')
    setLocale('en-US')
    await nextTick()
    expect(host.textContent).toContain('Custom SNI (optional); leave blank to restore defaults')
    expect(host.textContent).toContain('Custom ALPN (optional); leave blank to restore defaults')
    expect(host.querySelector('[data-testid="node-preview-scope-note"]')?.textContent).toContain('without disabling SNI or certificate verification')
    setLocale('zh-CN')
    await nextTick()
    await submit()
    expect(save.mock.calls[0]![0].draft).toMatchObject({ alpn: undefined, sni: undefined })

    const prefix = protocol === 'tuic' ? 'tuic' : 'node'
    const alpn = host.querySelector<HTMLInputElement>(`[data-testid="${prefix}-alpn-input"]`)!
    const sni = host.querySelector<HTMLInputElement>(`[data-testid="${prefix}-sni-input"]`)!
    expect(alpn.value).toBe('h2')
    expect(sni.value).toBe('sni.example')
    alpn.value = ''
    alpn.dispatchEvent(new Event('input', { bubbles: true }))
    sni.value = ''
    sni.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    await submit()
    expect(save.mock.calls[1]![0].draft).toMatchObject({ alpn: [], sni: '' })
    save.mock.calls[1]![0].onSuccess({ ...existing, connection: { ...existing.connection, alpn: [], sni: '' } })
    await nextTick()
    expect(host.querySelector('[data-testid="node-connection-dirty"]')).toBeNull()
    expect(alpn.value).toBe('')
    expect(sni.value).toBe('')
  })

  it.each(['trojan', 'tuic'])('hydrates the %s editor from an omitted-empty HTTP DTO and keeps it empty after reread', async (protocol) => {
    const authentication = {
      password: 'fixture-password',
      ...(protocol === 'tuic' ? { uuid: '11111111-2222-4333-8444-555555555555' } : {}),
    }
    const transport = { network: protocol === 'tuic' ? 'quic' : 'tcp', tls: 'true' }
    const imported = normalizeNode({
      logical_id: 'node-a', display_name: 'Imported TLS', protocol, active: true,
      connection_revision: 1, server: 'fixture.example', port: 443,
      credentials: {
        ...authentication,
        ...(protocol === 'tuic' ? { sni: 'old.example', alpn: ['h2'] } : {}),
        transport: { ...transport, sni: 'old.example', alpn: 'h2' },
      },
    })
    const savedRecord: NodeRecord = JSON.parse(JSON.stringify({
      logical_id: 'node-a', display_name: 'Imported TLS', protocol, active: true,
      connection_revision: 2, server: 'fixture.example', port: 443,
      credentials: { ...authentication, transport },
    }))
    expect(savedRecord.credentials).not.toHaveProperty('sni')
    expect(savedRecord.credentials).not.toHaveProperty('alpn')
    const patch = vi.spyOn(api, 'patch').mockResolvedValue({ node: savedRecord })
    const get = vi.spyOn(api, 'get').mockResolvedValue({ node: savedRecord })
    const nodes = useNodes()
    nodes.items.value = [imported]
    nodes.selectedNode.value = imported
    const editor = mountEditor(false, imported)
    editor.save.mockImplementation(({ draft, onSuccess, onError }) => {
      void nodes.updateNodeConnection('node-a', draft).then((result) => {
        if (result.ok && result.node) {
          editor.node.value = result.node
          onSuccess(result.node)
        } else onError(result.error || 'Save failed')
      })
    })
    const prefix = protocol === 'tuic' ? 'tuic' : 'node'
    const input = (field: string) => host.querySelector<HTMLInputElement>(`[data-testid="${prefix}-${field}-input"]`)!
    expect(input('sni').value).toBe('old.example')
    expect(input('alpn').value).toBe('h2')
    for (const field of ['sni', 'alpn']) {
      input(field).value = ''
      input(field).dispatchEvent(new Event('input', { bubbles: true }))
    }
    await nextTick()
    await submit()
    await vi.waitFor(() => expect(host.querySelector('[data-testid="node-connection-saved"]')).not.toBeNull())
    expect(patch).toHaveBeenCalledWith('/api/v1/nodes/node-a/connection', expect.objectContaining({ sni: '', alpn: [] }))
    expect(host.querySelector('[data-testid="node-connection-dirty"]')).toBeNull()
    expect(input('sni').value).toBe('')
    expect(input('alpn').value).toBe('')
    await nodes.fetchNodeDetail('node-a')
    expect(get).toHaveBeenCalledWith('/api/v1/nodes/node-a')
    for (const node of [nodes.items.value[0], nodes.selectedNode.value]) {
      expect(node?.connection).toMatchObject({ sni: '', alpn: [] })
    }
    app?.unmount()
    app = null
    mountEditor(false, nodes.selectedNode.value!)
    expect(input('sni').value).toBe('')
    expect(input('alpn').value).toBe('')
  })

  it.each(['ss', 'tuic'])('keeps cancelled %s TLS clears out of the saved connection', async (protocol) => {
    const existing = normalizeNode({
      logical_id: 'node-a', display_name: 'Alpha', protocol, active: true,
      server: 'fixture.example', port: 443,
      credentials: { method: 'aes-128-gcm', uuid: '11111111-2222-4333-8444-555555555555', password: 'fixture-password', sni: 'sni.example', alpn: ['h2'] },
    })
    const { node, save, close } = mountEditor(false, existing)
    const prefix = protocol === 'tuic' ? 'tuic' : 'node'
    const alpn = host.querySelector<HTMLInputElement>(`[data-testid="${prefix}-alpn-input"]`)!
    const sni = host.querySelector<HTMLInputElement>(`[data-testid="${prefix}-sni-input"]`)!
    alpn.value = ''
    alpn.dispatchEvent(new Event('input', { bubbles: true }))
    sni.value = ''
    sni.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    button('node-drawer-close-btn').click()
    await nextTick()
    button('discard-cancel').click()
    await nextTick()
    expect(close).not.toHaveBeenCalled()
    expect(alpn.value).toBe('')
    expect(sni.value).toBe('')
    button('node-drawer-close-btn').click()
    await nextTick()
    button('discard-confirm').click()
    await nextTick()
    expect(save).not.toHaveBeenCalled()
    expect(node.value.connection).toMatchObject({ sni: 'sni.example', alpn: ['h2'] })
    expect(alpn.value).toBe('h2')
    expect(sni.value).toBe('sni.example')
  })

  it('retains failed saves and resets only on a confirmed reload or a successful save', async () => {
    const { node, save } = mountEditor()
    await editServer()
    await submit()
    save.mock.calls[0]![0].onError('Fixture validation failed')
    await nextTick()
    expect(host.querySelector('[data-testid="node-connection-error"]')?.textContent).toContain('Fixture validation failed')
    node.value = fixture('node-a', 'latest.example')
    await nextTick()
    expect(server().value).toBe('draft.example')
    button('node-reload-draft').click()
    await nextTick()
    button('discard-cancel').click()
    await nextTick()
    expect(server().value).toBe('draft.example')
    button('node-reload-draft').click()
    await nextTick()
    button('discard-confirm').click()
    await nextTick()
    expect(server().value).toBe('latest.example')
    await editServer('saved.example')
    await submit()
    save.mock.calls[1]![0].onSuccess(fixture('node-a', 'saved.example'))
    await nextTick()
    expect(host.querySelector('[data-testid="node-connection-dirty"]')).toBeNull()
  })

  it('does not apply an old save callback after A-B-A selection or overwrite edits made while saving', async () => {
    const { node, save } = mountEditor()
    await editServer('submitted.example')
    await submit()
    const old = save.mock.calls[0]![0]
    node.value = fixture('node-b', 'b.example')
    await nextTick()
    node.value = fixture('node-a', 'reopened.example')
    await nextTick()
    await editServer('new-draft.example')
    old.onSuccess(fixture('node-a', 'late-response.example'))
    await nextTick()
    expect(server().value).toBe('new-draft.example')
    await submit()
    await editServer('newer-draft.example')
    save.mock.calls[1]![0].onSuccess(fixture('node-a', 'new-draft.example'))
    await nextTick()
    expect(server().value).toBe('newer-draft.example')
    expect(host.querySelector('[data-testid="node-connection-dirty"]')).not.toBeNull()
  })
})
