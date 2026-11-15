// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, type Component } from 'vue'
import { api } from '../../api/client'
import { setLocale } from '../../locales'
import { displayName, displayText } from '../../utils/displayNames'
import GroupCard from './GroupCard.vue'
import GroupPreviewPanel from './GroupPreviewPanel.vue'
import SourcePipelineEditor from './SourcePipelineEditor.vue'
import type { GroupDraftPreviewResponse, PolicyGroup } from './policyTypes'

const mounted: Array<{ app: ReturnType<typeof createApp>; host: HTMLDivElement }> = []
async function mount(component: Component, props: Record<string, unknown>) {
  setLocale('zh-CN')
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp({ render: () => h(component, props) })
  app.mount(host)
  mounted.push({ app, host })
  await nextTick()
  return host
}

afterEach(() => {
  mounted.splice(0).forEach(({ app, host }) => { app.unmount(); host.remove() })
  vi.restoreAllMocks()
})

describe('business labels instead of internal identifiers', () => {
  it('preserves names and diagnostic meaning without identifier/fingerprint fallbacks', () => {
    expect(displayName('香港专线 x3', '未知节点', 'node_secret')).toBe('香港专线 x3')
    for (const name of ['', 'node_secret', '8ef0ce60-c175-4265-8fa1-1a1bc5658bf0']) {
      expect(displayName(name, '未知节点')).toBe('未知节点')
    }
    expect(displayName('legacy-key', '未知节点', 'legacy-key')).toBe('未知节点')
    expect(displayText('节点 node_secret 引用 group_hidden 已失效，sha256:abc123')).toBe('节点 节点（标识已省略） 引用 策略组（标识已省略） 已失效，校验指纹（已省略）')
    expect(displayText('连接超时 HTTP 503')).toBe('连接超时 HTTP 503')
  })

  it('shows names in expanded group sources, including missing references, and preserves delete keys', async () => {
    const group: PolicyGroup = {
      id: 'group_private', name: '我的策略', group_type: 'select', sources: [
        { operation: 'add', kind: 'manual_node', position: 0, node_logical_id: 'node_private' },
        { operation: 'add', kind: 'group_ref', position: 1, target_group_id: 'group_child' },
        { operation: 'remove', kind: 'group_nodes', position: 2, target_group_id: 'group_missing' },
      ],
    }
    const onDelete = vi.fn()
    const host = await mount(GroupCard, {
      group, onDelete,
      availableNodes: [{ logicalId: 'node_private', displayName: '香港专线' }],
      availableGroups: [{ id: 'group_child', name: '地区线路', group_type: 'select' }],
    })
    host.querySelector<HTMLElement>('.cursor-pointer')!.click()
    await nextTick()
    expect(host.textContent).toContain('香港专线')
    expect(host.textContent).toContain('地区线路')
    expect(host.textContent).toContain('未知策略组')
    expect(host.textContent).not.toMatch(/group_private|node_private|group_child|group_missing/)
    host.querySelector<HTMLButtonElement>('[title="删除策略组"]')!.click()
    expect(onDelete).toHaveBeenCalledWith('group_private')
  })

  it('distinguishes zero direct nodes from retained group references and recursive destinations', async () => {
    const snapshot: GroupDraftPreviewResponse = {
      group_id: 'group_private', selection_digest: 'selection_private', content_digest: 'content_private',
      member_count: 11, node_count: 93, candidate_count: 93, excluded_count: 0,
      members: Array.from({ length: 11 }, (_, index) => ({ kind: 'group', group_id: `group_child_${index}`, display_name: `地区 ${index + 1}` })),
    }
    const host = await mount(GroupPreviewPanel, { snapshot, sourcesCount: 13 })
    expect(host.querySelector('[data-testid="member-composition"]')?.textContent).toContain('直接节点 0 · 子组引用 11 · 递归可达节点 93')
    expect(host.querySelector('[data-testid="group-reference-explanation"]')).not.toBeNull()
    expect(host.textContent).not.toContain('无匹配节点')
    expect(host.textContent).toContain('地区 1')
    expect(host.textContent).not.toMatch(/group_private|group_child_|selection_private|content_private/)
  })

  it('uses unknown labels for unnamed preview members rather than raw keys', async () => {
    const host = await mount(GroupPreviewPanel, {
      sourcesCount: 2,
      snapshot: { selection_digest: 'private_digest', node_count: 1, members: [
        { kind: 'node', node_logical_id: 'node_private' },
        { kind: 'group', group_id: 'group_private' },
      ] },
    })
    expect(host.textContent).toContain('未命名节点')
    expect(host.textContent).toContain('未知策略组')
    expect(host.textContent).not.toMatch(/node_private|group_private|private_digest/)
  })

  it('keeps source values stable while showing business names and explaining ordered union/subtraction', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path) => path === '/api/v1/nodes'
      ? { items: [{ logical_id: 'node_private', display_name: '香港专线', active: true }], total: 1 }
      : { items: [], total: 0 })
    const onAdd = vi.fn()
    const host = await mount(SourcePipelineEditor, {
      name: '选择节点', groupType: 'select', position: 0, inheritGlobalFilter: true,
      sources: [{ operation: 'add', kind: 'manual_node', position: 0, node_logical_id: 'node_private' }],
      availableNodes: [{ logicalId: 'node_private', displayName: '香港专线', active: true }],
      'onAdd-source': onAdd,
    })
    await Promise.resolve()
    await nextTick()
    const semantics = host.querySelector('[data-testid="source-pipeline-semantics"]')?.textContent
    expect(semantics).toContain('请先加入候选')
    expect(semantics).toContain('取并集')
    expect(semantics).toContain('条件取交集')
    expect(semantics).toContain('不会删除已保留的子组引用')
    const radio = host.querySelector<HTMLInputElement>('input[type="radio"][value="node"]')!
    radio.click()
    await nextTick()
    const select = host.querySelector<HTMLSelectElement>('[data-testid="edge-node-select"]')!
    expect(select.querySelector('option[value="node_private"]')?.textContent).toContain('香港专线')
    select.value = 'node_private'
    select.dispatchEvent(new Event('change', { bubbles: true }))
    await nextTick()
    host.querySelector<HTMLButtonElement>('[data-testid="add-edge-btn"]')!.click()
    expect(onAdd).toHaveBeenCalledWith(expect.objectContaining({ kind: 'manual_node', node_logical_id: 'node_private' }))
    expect(host.textContent).not.toContain('node_private')
  })
})
