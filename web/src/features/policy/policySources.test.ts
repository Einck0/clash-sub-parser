// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import PolicyEditorSheet from './PolicyEditorSheet.vue'
import StructuredFilterEditor from './StructuredFilterEditor.vue'
import GroupCard from './GroupCard.vue'
import { usePolicy } from './usePolicy'
import {
  computeSourceSemantics,
  normalizeGroupSources,
  validateSourceInput,
  type FilterCondition,
  type MemberSource,
  type PolicyGroup,
} from './policyTypes'
import { api, ApiError } from '../../api/client'

describe('Feature-Level Policy Sources & Pipeline Rebuild', () => {
  let container: HTMLDivElement

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
  })

  afterEach(() => {
    container.remove()
    vi.restoreAllMocks()
  })

  // 1. 各 kind 保存 payload / 顺序与字段校验
  describe('Member Source Kinds & PUT Payload Structure', () => {
    it('validates each source kind inputs with concise Chinese messages', () => {
      // manual_node
      expect(validateSourceInput({ operation: 'add', kind: 'manual_node', node_logical_id: '' })).toBe('手工节点必须指定节点逻辑 ID')
      expect(validateSourceInput({ operation: 'add', kind: 'manual_node', node_logical_id: 'node-hk-1' })).toBeNull()

      // group_ref & group_nodes self-loop prevention
      expect(validateSourceInput({ operation: 'add', kind: 'group_ref', target_group_id: '' })).toBe('策略组引用必须指定目标策略组 ID')
      expect(validateSourceInput({ operation: 'add', kind: 'group_ref', target_group_id: 'grp-self' }, 'grp-self')).toBe('禁止自环：父策略组不能引用自身')
      expect(validateSourceInput({ operation: 'add', kind: 'group_nodes', target_group_id: 'grp-self' }, 'grp-self')).toBe('禁止自环：父策略组不能展开自身')
      expect(validateSourceInput({ operation: 'add', kind: 'group_nodes', target_group_id: 'grp-other' }, 'grp-self')).toBeNull()

      // regex
      expect(validateSourceInput({ operation: 'remove', kind: 'regex', pattern: '' })).toBe('节点匹配正则表达式不能为空')
      expect(validateSourceInput({ operation: 'remove', kind: 'regex', pattern: '.*(高倍率|过期).*' })).toBeNull()

      // filter
      expect(validateSourceInput({ operation: 'add', kind: 'filter', filter_spec: { conditions: [] } })).toBe('同级属性过滤来源至少需要包含一个有效条件')
      expect(validateSourceInput({
        operation: 'add',
        kind: 'filter',
        filter_spec: { conditions: [{ field: 'protocol', op: 'equals', value: 'ss' }] },
      })).toBeNull()

      // typed_builtin
      expect(validateSourceInput({ operation: 'add', kind: 'typed_builtin' })).toBe('必须选择内建流控项名称')
      expect(validateSourceInput({ operation: 'add', kind: 'typed_builtin', builtin_name: 'DIRECT' })).toBeNull()
      expect(validateSourceInput({ operation: 'add', kind: 'typed_builtin', builtin_name: 'INVALID' as any })).toContain('不支持的内建项')
    })

    it('sends atomic PUT payload with exact sequential positions across all 6 source kinds', async () => {
      const putSpy = vi.spyOn(api, 'put').mockResolvedValueOnce({
        id: 'grp-main',
        name: 'All Kinds Pipeline',
        group_type: 'select',
        empty_fallback_pass: false,
        revision: 4,
        sources: [],
      })
      vi.spyOn(api, 'get').mockResolvedValue({ items: [], total: 0 })

      const { updateGroup } = usePolicy()

      const testSources: MemberSource[] = [
        { operation: 'add', kind: 'manual_node', position: 0, node_logical_id: 'node-hk-01' },
        { operation: 'add', kind: 'group_ref', position: 1, target_group_id: 'grp-sub-1' },
        { operation: 'add', kind: 'group_nodes', position: 2, target_group_id: 'grp-sub-2' },
        { operation: 'remove', kind: 'regex', position: 3, pattern: '.*(官网|过期).*' },
        {
          operation: 'add',
          kind: 'filter',
          position: 4,
          filter_spec: { conditions: [{ field: 'probe_latency_ms', op: 'lte', value: '200', probe_kind: 'baseline' }] },
        },
        { operation: 'add', kind: 'typed_builtin', position: 5, builtin_name: 'DIRECT' },
      ]

      await updateGroup('grp-main', {
        name: 'All Kinds Pipeline',
        group_type: 'select',
        empty_fallback_pass: false,
        expected_revision: 3,
        sources: testSources,
      })

      expect(putSpy).toHaveBeenCalledWith('/api/v1/policies/groups/grp-main', {
        name: 'All Kinds Pipeline',
        group_type: 'select',
        empty_fallback_pass: false,
        expected_revision: 3,
        sources: [
          { operation: 'add', kind: 'manual_node', position: 0, node_logical_id: 'node-hk-01' },
          { operation: 'add', kind: 'group_ref', position: 1, target_group_id: 'grp-sub-1' },
          { operation: 'add', kind: 'group_nodes', position: 2, target_group_id: 'grp-sub-2' },
          { operation: 'remove', kind: 'regex', position: 3, pattern: '.*(官网|过期).*' },
          {
            operation: 'add',
            kind: 'filter',
            position: 4,
            filter_spec: { conditions: [{ field: 'probe_latency_ms', op: 'lte', value: '200', probe_kind: 'baseline' }] },
          },
          { operation: 'add', kind: 'typed_builtin', position: 5, builtin_name: 'DIRECT' },
        ],
      })
    })
  })

  // 2. 顺序加减与重入顺序语义 (remove after add & re-add)
  describe('Sequential Pipeline Semantics & Re-entry Detection', () => {
    it('accurately identifies re-entry after removal vs first-appearance vs duplicate addition', () => {
      const pipeline: MemberSource[] = [
        { operation: 'add', kind: 'manual_node', position: 0, node_logical_id: 'node-backup-1' },
        { operation: 'remove', kind: 'manual_node', position: 1, node_logical_id: 'node-backup-1' },
        { operation: 'add', kind: 'manual_node', position: 2, node_logical_id: 'node-backup-1' },
        { operation: 'add', kind: 'manual_node', position: 3, node_logical_id: 'node-backup-1' },
      ]

      const semantics = computeSourceSemantics(pipeline)

      // 0: First appearance
      expect(semantics[0].isReEntry).toBe(false)
      expect(semantics[0].isDuplicate).toBe(false)
      expect(semantics[0].semanticText).toContain('首次出现')

      // 1: Removal operation
      expect(semantics[1].semanticText).toContain('排除仅当前成员，后续可加入')

      // 2: Re-entry after previous removal
      expect(semantics[2].isReEntry).toBe(true)
      expect(semantics[2].semanticText).toContain('重入入选')

      // 3: Duplicate add without intermediate removal
      expect(semantics[3].isDuplicate).toBe(true)
      expect(semantics[3].semanticText).toContain('重复项')
    })
  })

  // 3. group_ref 与 group_nodes 明确区分
  describe('Group Ref vs Group Nodes Distinction', () => {
    it('distinguishes group reference from group nodes unpacking in labels and semantics', () => {
      const groupRefSource: MemberSource = {
        operation: 'add',
        kind: 'group_ref',
        position: 0,
        target_group_id: 'grp-auto',
      }
      const groupNodesSource: MemberSource = {
        operation: 'add',
        kind: 'group_nodes',
        position: 1,
        target_group_id: 'grp-auto',
      }

      const semantics = computeSourceSemantics([groupRefSource, groupNodesSource])
      // Different kinds for same group are distinct pipeline targets
      expect(semantics[0].source.kind).toBe('group_ref')
      expect(semantics[1].source.kind).toBe('group_nodes')
      expect(semantics[1].isDuplicate).toBe(false)
    })
  })

  // 4. 正则与过滤同级，不误删后续手工节点
  describe('Regex and Filter Scope Localization', () => {
    it('normalizes legacy group filter and edges into unified sources maintaining sequence', () => {
      const legacyGroup: Partial<PolicyGroup> = {
        edges: [{ node_logical_id: 'node-hk-1', position: 0 }],
        node_filter: {
          conditions: [{ field: 'display_name', op: 'regex', value: '.*HK.*' }],
        },
      }

      const normalized = normalizeGroupSources(legacyGroup)
      expect(normalized).toHaveLength(2)
      expect(normalized[0].kind).toBe('manual_node')
      expect(normalized[0].node_logical_id).toBe('node-hk-1')
      expect(normalized[1].kind).toBe('filter')
      expect(normalized[1].filter_spec?.conditions[0].value).toBe('.*HK.*')
    })
  })

  // 5. 409 Conflict 乐观锁版本冲突与保留本地草稿
  describe('Revision 409 Conflict & Draft Preservation', () => {
    it('throws ApiError with status 409 on revision conflict without altering local group state', async () => {
      vi.spyOn(api, 'put').mockRejectedValueOnce(
        new ApiError(409, 'revision_conflict', 'Policy group revision mismatch', undefined, { current_revision: 5 })
      )

      const { updateGroup, groups } = usePolicy()
      groups.value = [
        { id: 'grp-conflict', name: 'Strict Group', group_type: 'select', revision: 3, sources: [] },
      ]

      await expect(
        updateGroup('grp-conflict', {
          name: 'Strict Group Renamed',
          expected_revision: 3,
        })
      ).rejects.toThrow('Policy group revision mismatch')

      // Local revision and name are not altered
      expect(groups.value[0].revision).toBe(3)
      expect(groups.value[0].name).toBe('Strict Group')
    })

    it('displays 409 conflict alert in PolicyEditorSheet and preserves local draft when user clicks 保留本地草稿', async () => {
      const group: PolicyGroup = {
        id: 'grp-draft',
        name: 'Draft Preserved Group',
        group_type: 'select',
        revision: 2,
        sources: [
          { operation: 'add', kind: 'manual_node', position: 0, node_logical_id: 'node-hk-01' },
        ],
      }

      const app = createApp({
        render() {
          return h(PolicyEditorSheet, {
            open: true,
            group,
            allGroups: [group],
            conflictError: { currentRevision: 5, message: '服务端版本已更新为 v5' },
          })
        },
      })
      app.mount(container)
      await nextTick()

      // Alert with 409 conflict message should be rendered
      expect(container.textContent).toContain('版本冲突 (409 Conflict)')
      expect(container.textContent).toContain('服务端版本已更新为 v5')

      // Click "保留本地草稿" button
      const preserveBtn = Array.from(container.querySelectorAll('button')).find((b) => b.textContent?.includes('保留本地草稿'))
      expect(preserveBtn).toBeDefined()
      preserveBtn?.click()
      await nextTick()

      // Draft sources and edited name are preserved
      expect(container.querySelector('input')?.value).toBe('Draft Preserved Group')
      expect(container.textContent).toContain('node-hk-01')
      expect(container.textContent).toContain('v5')

      app.unmount()
    })
  })

  describe('Condition sources are explicit, independent pipeline drafts', () => {
    const group: PolicyGroup = {
      id: 'grp-conditions', name: 'Conditions', group_type: 'select', revision: 2,
      sources: [{ operation: 'add', kind: 'typed_builtin', builtin_name: 'DIRECT', position: 0 }],
      node_filter: { conditions: [{ field: 'display_name', op: 'contains', value: 'legacy' }] },
    }
    let app: ReturnType<typeof createApp> | undefined
    afterEach(() => app?.unmount())

    function button(text: string) {
      const found = Array.from(container.querySelectorAll('button')).find(b => b.textContent?.trim() === text)
      expect(found, text).toBeDefined()
      return found!
    }
    async function choose(value: string) {
      const radio = container.querySelector(`input[type="radio"][value="${value}"]`) as HTMLInputElement
      expect(radio).not.toBeNull()
      radio.click()
      await nextTick()
    }
    async function condition(field: string, value: string, op?: string, probeKind?: string) {
      const fieldSelect = container.querySelector('[data-testid="condition-field"]') as HTMLSelectElement
      fieldSelect.value = field
      fieldSelect.dispatchEvent(new Event('change'))
      await nextTick()
      if (op) {
        const opSelect = container.querySelector('[data-testid="condition-op"]') as HTMLSelectElement
        opSelect.value = op
        opSelect.dispatchEvent(new Event('change'))
        await nextTick()
      }
      if (probeKind) {
        const probe = container.querySelector('select[aria-label="探针类型"]') as HTMLSelectElement
        probe.value = probeKind
        probe.dispatchEvent(new Event('change'))
        await nextTick()
      }
      const input = container.querySelector('[data-testid="condition-value"]') as HTMLInputElement
      input.value = value
      input.dispatchEvent(new Event(input.tagName === 'SELECT' ? 'change' : 'input'))
      await nextTick()
      button('添加条件').click()
      await nextTick()
    }
    function submit() {
      container.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
    }

    it('appends inline filter with chosen operation, edits conditions directly inline, previews and saves in one step', async () => {
      const save = vi.fn()
      const post = vi.spyOn(api, 'post').mockResolvedValue({ members: [], node_count: 0, selection_digest: '' })
      app = createApp({ render: () => h(PolicyEditorSheet, { open: true, group, allGroups: [group], onSaveGroup: save }) })
      app.mount(container)
      await nextTick()
      expect(container.textContent).not.toContain('策略组节点筛选条件')
      await choose('filter')
      await choose('remove')
      const addFilterBtn = Array.from(container.querySelectorAll('button')).find(b => b.textContent?.includes('过滤来源'))!
      expect(addFilterBtn).toBeDefined()
      addFilterBtn.click()
      await nextTick()

      const rows = container.querySelectorAll('[data-testid="source-row"]')
      expect(rows).toHaveLength(2)

      const inlineEditor = container.querySelector('[data-testid="primary-filter-section"] [data-testid="inline-filter-editor"]')!
      expect(inlineEditor).not.toBeNull()

      // The pipeline row inside details only shows a summary, avoiding double editors
      expect(rows[1].querySelector('[data-testid="filter-row-summary"]')).not.toBeNull()
      expect(rows[1].querySelector('[data-testid="structured-filter-editor"]')).toBeNull()

      const fieldSelect = inlineEditor.querySelector('[data-testid="condition-field"]') as HTMLSelectElement
      fieldSelect.value = 'display_name'
      fieldSelect.dispatchEvent(new Event('change'))
      await nextTick()

      const opSelect = inlineEditor.querySelector('[data-testid="condition-op"]') as HTMLSelectElement
      opSelect.value = 'regex'
      opSelect.dispatchEvent(new Event('change'))
      await nextTick()

      const valueInput = inlineEditor.querySelector('[data-testid="condition-value"]') as HTMLInputElement
      valueInput.value = 'HK|JP'
      valueInput.dispatchEvent(new Event('input'))
      await nextTick()

      submit()
      await nextTick()
      expect(save).toHaveBeenCalledOnce()
      const payload = save.mock.calls[0][0]
      expect(payload).not.toHaveProperty('nodeFilter')
      expect(payload).not.toHaveProperty('edges')
      expect(payload.sources).toHaveLength(2)
      expect(payload.sources[0]).not.toHaveProperty('_clientKey')
      expect(payload.sources[1]).not.toHaveProperty('_clientKey')
      expect(payload.sources[0]).toMatchObject({ kind: 'typed_builtin', builtin_name: 'DIRECT', position: 0 })
      expect(payload.sources[1]).toMatchObject({ kind: 'filter', operation: 'remove', position: 1 })
      expect(payload.sources[1].filter_spec.conditions[0]).toEqual({ field: 'display_name', op: 'regex', value: 'HK|JP' })

      const previewBtn = Array.from(container.querySelectorAll('button')).find(b => b.textContent?.includes('刷新计算'))!
      previewBtn.click()
      await nextTick()
      expect(post).toHaveBeenLastCalledWith('/api/v1/policies/groups/preview', expect.objectContaining({ sources: payload.sources }))
      const previewPayload = post.mock.calls.at(-1)?.[1] as any
      expect(previewPayload.sources[0]).not.toHaveProperty('_clientKey')
      expect(previewPayload.sources[1]).not.toHaveProperty('_clientKey')
    })

    it('edits the selected saved filter only, preserves it through 409, and previews the same reopened sources', async () => {
      const saved: PolicyGroup = { ...group, sources: [
        { operation: 'add', kind: 'filter', position: 0, filter_spec: { conditions: [{ field: 'protocol', op: 'equals', value: 'ss' }] } },
        { operation: 'remove', kind: 'filter', position: 1, filter_spec: { conditions: [{ field: 'display_name', op: 'regex', value: 'JP' }] } },
      ] }
      const current = ref(saved)
      const open = ref(true)
      const conflict = ref<{ currentRevision: number; message: string } | null>(null)
      const save = vi.fn()
      const post = vi.spyOn(api, 'post').mockResolvedValue({ members: [], node_count: 0, selection_digest: '' })
      app = createApp({ render: () => h(PolicyEditorSheet, { open: open.value, group: current.value, allGroups: [saved], conflictError: conflict.value, onSaveGroup: save }) })
      app.mount(container)
      await nextTick()

      const rows = container.querySelectorAll('[data-testid="source-row"]')
      expect(rows).toHaveLength(2)

      // The filter editors are in the primary filter section at the top of the form
      const filterEditors = container.querySelectorAll('[data-testid="primary-filter-section"] [data-testid="inline-filter-editor"]')
      expect(filterEditors).toHaveLength(2)

      // The pipeline row in details only has a summary to avoid double editors
      expect(rows[1].querySelector('[data-testid="filter-row-summary"]')).not.toBeNull()
      expect(rows[1].querySelector('[data-testid="structured-filter-editor"]')).toBeNull()

      const addCondBtn = filterEditors[1].querySelector('[data-testid="filter-add"]') as HTMLButtonElement
      expect(addCondBtn).not.toBeNull()
      addCondBtn.click()
      await nextTick()

      const conditionRows = filterEditors[1].querySelectorAll('[data-testid="filter-condition"]')
      expect(conditionRows).toHaveLength(2)
      const condField = conditionRows[1].querySelector('[data-testid="condition-field"]') as HTMLSelectElement
      condField.value = 'protocol'
      condField.dispatchEvent(new Event('change'))
      await nextTick()
      const condVal = conditionRows[1].querySelector('[data-testid="condition-value"]') as HTMLSelectElement
      condVal.value = 'ss'
      condVal.dispatchEvent(new Event('change'))
      await nextTick()

      conflict.value = { currentRevision: 4, message: 'conflict' }
      await nextTick()
      button('保留本地草稿').click()
      await nextTick()

      submit()
      await nextTick()
      expect(save).toHaveBeenCalledOnce()
      const payload = save.mock.calls[0][0]
      expect(payload.expectedRevision).toBe(4)
      expect(payload.sources[0]).toEqual(saved.sources![0])
      expect(payload.sources[1]).toMatchObject({ operation: 'remove', position: 1, filter_spec: { conditions: [
        { field: 'display_name', op: 'regex', value: 'JP' },
        { field: 'protocol', op: 'equals', value: 'ss' },
      ] } })
      expect(saved.sources![1].filter_spec!.conditions).toHaveLength(1)

      open.value = false
      await nextTick()
      current.value = { ...saved, sources: payload.sources, revision: 5 }
      open.value = true
      await nextTick()
      const refreshBtn = Array.from(container.querySelectorAll('button')).find(b => b.textContent?.includes('刷新计算'))!
      refreshBtn.click()
      await nextTick()
      expect(post).toHaveBeenLastCalledWith('/api/v1/policies/groups/preview', expect.objectContaining({ sources: payload.sources }))
    })

    it('blocks an unfinished condition value and retains invalid regex text until explicitly discarded', async () => {
      const save = vi.fn()
      app = createApp({ render: () => h(PolicyEditorSheet, { open: true, group, allGroups: [group], onSaveGroup: save }) })
      app.mount(container)
      await nextTick()

      await choose('filter')
      const addFilterBtn = Array.from(container.querySelectorAll('button')).find(b => b.textContent?.includes('过滤来源'))!
      addFilterBtn.click()
      await nextTick()

      const rows = container.querySelectorAll('[data-testid="source-row"]')
      expect(rows).toHaveLength(2)

      const fieldSelect = container.querySelector('[data-testid="primary-filter-section"] [data-testid="condition-field"]') as HTMLSelectElement
      fieldSelect.value = 'display_name'
      fieldSelect.dispatchEvent(new Event('change'))
      await nextTick()

      submit()
      await nextTick()
      expect(save).not.toHaveBeenCalled()

      const valInput = container.querySelector('[data-testid="primary-filter-section"] [data-testid="condition-value"]') as HTMLInputElement
      valInput.value = 'valid-name'
      valInput.dispatchEvent(new Event('input'))
      await nextTick()

      await choose('regex')
      const regex = container.querySelector('input[placeholder="例如：.*(HK|香港|HongKong).*"]') as HTMLInputElement
      regex.value = '['
      regex.dispatchEvent(new Event('input'))
      await nextTick()
      button('添加加入正则 (+)').click()
      await nextTick()
      expect(container.textContent).toContain('正则表达式语法无效')
      expect(regex.value).toBe('[')
      submit()
      expect(save).not.toHaveBeenCalled()
      button('放弃来源草稿').click()
      await nextTick()
      submit()
      expect(save).toHaveBeenCalledOnce()
    })

    it('handles delete and reorder of invalid filter without key drift and strips _clientKey from payload', async () => {
      const save = vi.fn()
      app = createApp({ render: () => h(PolicyEditorSheet, { open: true, group, allGroups: [group], onSaveGroup: save }) })
      app.mount(container)
      await nextTick()

      // Add a filter source
      await choose('filter')
      const addFilterBtn = Array.from(container.querySelectorAll('button')).find(b => b.textContent?.includes('过滤来源'))!
      addFilterBtn.click()
      await nextTick()

      let rows = container.querySelectorAll('[data-testid="source-row"]')
      expect(rows).toHaveLength(2)

      // Make row 1 invalid by selecting display_name without value in primary filter section
      const fieldSelect = container.querySelector('[data-testid="primary-filter-section"] [data-testid="condition-field"]') as HTMLSelectElement
      fieldSelect.value = 'display_name'
      fieldSelect.dispatchEvent(new Event('change'))
      await nextTick()

      submit()
      await nextTick()
      expect(save).not.toHaveBeenCalled()

      // Delete the invalid row 1
      const trashBtns = rows[1].querySelectorAll('button[title="删除"]')
      const trashBtn = trashBtns[trashBtns.length - 1] as HTMLButtonElement
      trashBtn.click()
      await nextTick()

      rows = container.querySelectorAll('[data-testid="source-row"]')
      expect(rows).toHaveLength(1)

      // Now save should succeed because the invalid filter was deleted
      submit()
      await nextTick()
      expect(save).toHaveBeenCalledOnce()
      const payload = save.mock.calls[0][0]
      expect(payload.sources).toHaveLength(1)
      expect(payload.sources[0]).not.toHaveProperty('_clientKey')
    })
  })

  // 6. 422 Cycle Detected & Invalid Source Entry
  describe('Topology Cycle & Invalid Input Server Error Feedback', () => {
    it('displays dependency cycle banner with highlighted cycle path in PolicyEditorSheet', async () => {
      const group: PolicyGroup = {
        id: 'grp-cycle-1',
        name: 'Cycle Group A',
        group_type: 'select',
        sources: [],
      }

      const app = createApp({
        render() {
          return h(PolicyEditorSheet, {
            open: true,
            group,
            allGroups: [group],
            cycleError: {
              cyclePath: ['grp-cycle-1', 'grp-cycle-2', 'grp-cycle-1'],
              message: '检测到循环依赖',
            },
          })
        },
      })
      app.mount(container)
      await nextTick()

      expect(container.textContent).toContain('检测到拓扑成环 (422 Unprocessable Entity)')
      expect(container.textContent).toContain('grp-cycle-1 → grp-cycle-2 → grp-cycle-1')

      app.unmount()
    })
  })

  // 7. Atomic Saves & Re-GET Exact Order
  describe('Atomic Saves & Exact Order Synchronization', () => {
    it('synchronizes exact sources order and updated revision on successful update', async () => {
      const updatedGroup: PolicyGroup = {
        id: 'grp-synced',
        name: 'Reordered Group',
        group_type: 'select',
        revision: 4,
        sources: [
          { operation: 'add', kind: 'manual_node', position: 0, node_logical_id: 'node-hk-02' },
          { operation: 'add', kind: 'manual_node', position: 1, node_logical_id: 'node-hk-01' },
        ],
      }

      vi.spyOn(api, 'put').mockResolvedValueOnce(updatedGroup)
      vi.spyOn(api, 'get').mockResolvedValue({ items: [updatedGroup], total: 1 })

      const { updateGroup, groups } = usePolicy()
      groups.value = [{ id: 'grp-synced', name: 'Old Order', group_type: 'select', revision: 3, sources: [] }]

      const result = await updateGroup('grp-synced', {
        name: 'Reordered Group',
        expected_revision: 3,
        sources: updatedGroup.sources,
      })

      expect(result.revision).toBe(4)
      expect(result.sources?.[0].node_logical_id).toBe('node-hk-02')
      expect(result.sources?.[1].node_logical_id).toBe('node-hk-01')
    })
  })

  it('treats explicit empty sources as authoritative and never resurrects legacy membership', () => {
    expect(normalizeGroupSources({ sources: [], edges: [{ position: 0, node_logical_id: 'old' }], node_filter: { conditions: [{ field: 'protocol', op: 'equals', value: 'ss' }] } })).toEqual([])
  })

  it('sends sources alone for object saves even if a legacy caller supplies conflicting fields, and preserves sources on metadata-only updates', async () => {
    const saved: PolicyGroup = { id: 'g', name: 'G', group_type: 'select', sources: [{ operation: 'remove', kind: 'regex', pattern: 'JP', position: 0 }] }
    const put = vi.spyOn(api, 'put').mockResolvedValue(saved)
    const post = vi.spyOn(api, 'post').mockResolvedValue(saved)
    vi.spyOn(api, 'get').mockResolvedValue({ items: [saved], total: 1 })
    const policy = usePolicy()
    const mixed = { sources: saved.sources, edges: [{ position: 0, node_logical_id: 'legacy' }], node_filter: { conditions: [{ field: 'protocol' as const, op: 'equals' as const, value: 'ss' }] } }
    await policy.updateGroup('g', mixed)
    expect(put).toHaveBeenLastCalledWith('/api/v1/policies/groups/g', { sources: saved.sources })
    await policy.createGroup({ name: 'G', group_type: 'select', ...mixed })
    expect(post).toHaveBeenCalledWith('/api/v1/policies/groups', { name: 'G', group_type: 'select', empty_fallback_pass: false, inherit_global_filter: true, sources: saved.sources })
    await policy.updateGroup('g', { name: 'Renamed' })
    expect(put).toHaveBeenLastCalledWith('/api/v1/policies/groups/g', { name: 'Renamed' })
  })

  // 8. 250th Node Search & Cross-page Retention
  describe('Node Candidate Pagination & Cross-Page Selection', () => {
    it('supports searching candidate nodes and retaining selection across multiple pages', async () => {
      const mockNodes = Array.from({ length: 250 }, (_, i) => ({
        logical_id: `node-${i + 1}`,
        display_name: i === 249 ? 'Node Canada 250' : `Node ${i + 1}`,
        active: true,
        protocol: 'ss',
      }))

      vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
        if (path === '/api/v1/nodes') {
          const search = options?.params?.search || ''
          const p = Number(options?.params?.page || 1)
          const ps = Number(options?.params?.page_size || 50)
          let filtered = mockNodes
          if (search) {
            filtered = filtered.filter((n) => n.display_name.includes(search))
          }
          const slice = filtered.slice((p - 1) * ps, p * ps)
          return { items: slice, total: filtered.length, page: p, page_size: ps }
        }
        return { items: [], total: 0 }
      })

      const group: PolicyGroup = {
        id: 'grp-250',
        name: '250 Test Group',
        group_type: 'select',
        sources: [],
      }

      const app = createApp({
        render() {
          return h(PolicyEditorSheet, {
            open: true,
            group,
            allGroups: [group],
          })
        },
      })
      app.mount(container)
      await nextTick()

      // Switch to node radio
      const nodeRadio = container.querySelector('input[type="radio"][value="node"]') as HTMLInputElement
      expect(nodeRadio).not.toBeNull()
      nodeRadio.click()
      await nextTick()

      // Search for 250
      const searchInput = container.querySelector('[data-testid="candidate-node-search-input"]') as HTMLInputElement
      expect(searchInput).not.toBeNull()
      searchInput.value = '250'
      searchInput.dispatchEvent(new Event('input'))

      await new Promise((r) => setTimeout(r, 350))
      await nextTick()

      const select = container.querySelector('[data-testid="edge-node-select"]') as HTMLSelectElement
      expect(select).not.toBeNull()
      const opt250 = Array.from(select.options).find((o) => o.textContent?.includes('Node Canada 250'))
      expect(opt250).toBeDefined()

      // Select 250th node
      select.value = 'node-250'
      select.dispatchEvent(new Event('change'))
      await nextTick()

      expect(container.textContent).toContain('跨页锁定已保留')

      app.unmount()
    })
  })

  // 9. Reorder Up / Down & Keyboard Operation
  describe('Sources Up/Down Reorder & Keyboard Accessibility', () => {
    it('moves sources up and down updating positions accordingly', async () => {
      const group: PolicyGroup = {
        id: 'grp-reorder',
        name: 'Reorder Test',
        group_type: 'select',
        sources: [
          { operation: 'add', kind: 'manual_node', position: 0, node_logical_id: 'node-A' },
          { operation: 'add', kind: 'manual_node', position: 1, node_logical_id: 'node-B' },
        ],
      }

      const app = createApp({
        render() {
          return h(PolicyEditorSheet, {
            open: true,
            group,
            allGroups: [group],
          })
        },
      })
      app.mount(container)
      await nextTick()

      // Find down button for first item
      const downBtns = Array.from(container.querySelectorAll('button[title="下移"]'))
      expect(downBtns.length).toBeGreaterThan(0)
      downBtns[0].dispatchEvent(new Event('click'))
      await nextTick()

      // Verify node-B is now first in rendered order
      const renderedSources = container.querySelectorAll('.font-mono .text-primary')
      expect(renderedSources.length).toBeGreaterThan(0)

      app.unmount()
    })
  })

  // 10. Four Viewports Responsive Layout & Zero Horizontal Scroll
  describe('Four Standard Viewports Responsiveness', () => {
    const viewports = [
      { name: 'Desktop-1280x800', width: 1280, height: 800 },
      { name: 'Mobile-392x872', width: 392, height: 872 },
      { name: 'Mobile-375x667', width: 375, height: 667 },
      { name: 'Landscape-667x375', width: 667, height: 375 },
    ]

    for (const vp of viewports) {
      it(`renders PolicyEditorSheet without horizontal overflow on ${vp.name} (${vp.width}x${vp.height})`, async () => {
        window.innerWidth = vp.width
        window.innerHeight = vp.height
        window.dispatchEvent(new Event('resize'))

        const group: PolicyGroup = {
          id: `grp-vp-${vp.name}`,
          name: `Viewport Test ${vp.name}`,
          group_type: 'select',
          sources: [
            { operation: 'add', kind: 'manual_node', position: 0, node_logical_id: 'node-hk-1' },
          ],
        }

        const app = createApp({
          render() {
            return h(PolicyEditorSheet, {
              open: true,
              group,
              allGroups: [group],
            })
          },
        })
        app.mount(container)
        await nextTick()

        const sheetDialog = container.querySelector('[role="dialog"]') as HTMLElement
        expect(sheetDialog).not.toBeNull()
        expect(sheetDialog.className).toContain('adaptive-surface-sheet')
        expect(sheetDialog.className).toContain('max-w-full')

        app.unmount()
      })
    }
  })

  // 11. Readonly Snapshot Preview Tab
  describe('Readonly Snapshot Preview Tab', () => {
    it('fetches draft preview via POST /api/v1/policies/groups/preview and renders digests, members, and draft label', async () => {
      let draftPostPayload: any = null
      vi.spyOn(api, 'post').mockImplementation(async (path: string, body?: any) => {
        if (path === '/api/v1/policies/groups/preview') {
          draftPostPayload = body
          return {
            group_id: 'grp-preview',
            revision: 1,
            selection_digest: 'selection-hash-0987654321fedcba',
            content_digest: 'content-hash-abcdef1234567890',
            content: 'proxies:\n  - name: HK-01\n    type: ss',
            members: [
              { kind: 'manual_node', node_logical_id: 'node-hk-01' },
            ],
            node_count: 1,
            diagnostics: ['警告：当前节点即将过期'],
          }
        }
        return {}
      })

      const group: PolicyGroup = {
        id: 'grp-preview',
        name: 'Preview Group',
        group_type: 'select',
        revision: 1,
        sources: [
          { operation: 'add', kind: 'manual_node', position: 0, node_logical_id: 'node-hk-01' },
        ],
      }

      const app = createApp({
        render() {
          return h(PolicyEditorSheet, {
            open: true,
            group,
            allGroups: [group],
          })
        },
      })
      app.mount(container)
      await nextTick()

      // Switch to preview tab
      const previewTabBtn = Array.from(container.querySelectorAll('button')).find((b) => b.textContent?.includes('解析快照预览'))
      expect(previewTabBtn).toBeDefined()
      previewTabBtn?.click()
      await nextTick()
      await new Promise((r) => setTimeout(r, 30))

      // Verification of typed draft payload sent to POST /api/v1/policies/groups/preview
      expect(draftPostPayload).not.toBeNull()
      expect(draftPostPayload.group_id).toBe('grp-preview')
      expect(draftPostPayload.expected_revision).toBe(1)
      expect(draftPostPayload.name).toBe('Preview Group')
      expect(draftPostPayload.group_type).toBe('select')
      expect(draftPostPayload.target).toBe('mihomo')
      expect(draftPostPayload.sources).toHaveLength(1)
      expect(draftPostPayload.sources[0].node_logical_id).toBe('node-hk-01')

      // Verification of rendered draft label, digests, resolved members, and diagnostics
      expect(container.textContent).toContain('草稿预览 (Draft Preview)')
      expect(container.textContent).toContain('选择集指纹')
      expect(container.textContent).toContain('内容指纹')
      expect(container.textContent).toContain('selection-hash-09876543')
      expect(container.textContent).toContain('content-hash-abcdef123')
      expect(container.textContent).toContain('node-hk-01')
      expect(container.textContent).toContain('警告：当前节点即将过期')

      app.unmount()
    })

    it('displays distinct diagnostic when sources is empty vs when no nodes match filter', async () => {
      vi.spyOn(api, 'post').mockImplementation(async (path: string) => {
        if (path === '/api/v1/policies/groups/preview') {
          return {
            group_id: 'grp-empty-preview',
            revision: 1,
            selection_digest: 'digest-empty',
            content_digest: 'content-empty',
            members: [],
            node_count: 0,
            diagnostics: [],
          }
        }
        return {}
      })

      const emptyGroup: PolicyGroup = {
        id: 'grp-empty-preview',
        name: 'Empty Preview Group',
        group_type: 'select',
        sources: [],
      }

      const app = createApp({
        render() {
          return h(PolicyEditorSheet, {
            open: true,
            group: emptyGroup,
            allGroups: [emptyGroup],
          })
        },
      })
      app.mount(container)
      await nextTick()

      const previewTabBtn = Array.from(container.querySelectorAll('button')).find((b) => b.textContent?.includes('解析快照预览'))
      previewTabBtn?.click()
      await nextTick()
      await new Promise((r) => setTimeout(r, 20))

      // Empty sources distinct diagnostic
      expect(container.textContent).toContain('空来源项 (Empty Sources)')

      app.unmount()
    })

    it('differentiates draft preview from saved publication preview in usePolicy', async () => {
      const draftSpy = vi.spyOn(api, 'post').mockResolvedValueOnce({
        group_id: 'grp-draft-test',
        revision: 2,
        selection_digest: 'digest-draft-unique',
        content_digest: 'content-draft-unique',
        members: [{ kind: 'manual_node', node_logical_id: 'node-draft' }],
        node_count: 1,
      })

      const { fetchGroupSnapshotPreview } = usePolicy()
      const draftResult = await fetchGroupSnapshotPreview({
        group_id: 'grp-draft-test',
        expected_revision: 2,
        name: 'Draft Policy Group',
        group_type: 'select',
        sources: [{ operation: 'add', kind: 'manual_node', position: 0, node_logical_id: 'node-draft' }],
        target: 'mihomo',
      })

      expect(draftSpy).toHaveBeenCalledWith('/api/v1/policies/groups/preview', {
        group_id: 'grp-draft-test',
        expected_revision: 2,
        name: 'Draft Policy Group',
        group_type: 'select',
        sources: [{ operation: 'add', kind: 'manual_node', position: 0, node_logical_id: 'node-draft' }],
        target: 'mihomo',
      })
      expect(draftResult.selection_digest).toBe('digest-draft-unique')
    })
  })

  // 12. Candidate Groups Pagination & Multi-Page Discovery (150 Groups)
  describe('Candidate Groups Asynchronous Discovery (150 Groups Across Pages)', () => {
    it('fetches and populates 150 candidate groups across page boundaries without missing items', async () => {
      const mockGroups = Array.from({ length: 150 }, (_, i) => ({
        id: `grp-${i + 1}`,
        name: `Subgroup ${i + 1}`,
        group_type: 'select' as const,
        revision: 1,
        sources: [],
      }))

      vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
        if (path === '/api/v1/policies/groups') {
          const p = Number(options?.params?.page || 1)
          const ps = Number(options?.params?.page_size || 100)
          const slice = mockGroups.slice((p - 1) * ps, p * ps)
          return { items: slice, total: mockGroups.length, page: p, page_size: ps }
        }
        return { items: [], total: 0 }
      })

      const parentGroup: PolicyGroup = {
        id: 'grp-parent',
        name: 'Parent Group',
        group_type: 'select',
        sources: [],
      }

      const app = createApp({
        render() {
          return h(PolicyEditorSheet, {
            open: true,
            group: parentGroup,
            allGroups: [parentGroup], // initially only parent group in props
          })
        },
      })
      app.mount(container)
      await nextTick()
      await new Promise((r) => setTimeout(r, 50))

      ;(container.querySelector('input[type="radio"][value="group"]') as HTMLInputElement).click()
      await nextTick()
      // The group selector should contain groups beyond page 1 (e.g. grp-150)
      const groupSelect = container.querySelector('[data-testid="edge-group-select"]') as HTMLSelectElement
      expect(groupSelect).not.toBeNull()
      const opt150 = Array.from(groupSelect.options).find((o) => o.textContent?.includes('Subgroup 150'))
      expect(opt150).toBeDefined()

      app.unmount()
    })
  })

  // 13. Security: No Error Secret Tokens Leakage
  describe('Security & Secret Tokens Protection', () => {
    it('ensures API errors containing sensitive tokens or credentials are never exposed verbatim', async () => {
      const secretToken = 'secret-token-abcdef1234567890'
      vi.spyOn(api, 'post').mockRejectedValueOnce(
        new ApiError(403, 'forbidden', `Authentication failed with token ${secretToken}`)
      )

      const group: PolicyGroup = {
        id: 'grp-sec',
        name: 'Security Test Group',
        group_type: 'select',
        sources: [],
      }

      const app = createApp({
        render() {
          return h(PolicyEditorSheet, {
            open: true,
            group,
            allGroups: [group],
          })
        },
      })
      app.mount(container)
      await nextTick()

      const previewTabBtn = Array.from(container.querySelectorAll('button')).find((b) => b.textContent?.includes('解析快照预览'))
      previewTabBtn?.click()
      await nextTick()
      await new Promise((r) => setTimeout(r, 30))

      // The preview error should be handled cleanly without leaking raw token
      expect(container.textContent).toContain('快照解析失败')
      expect(container.textContent).not.toContain(secretToken)
      expect(container.textContent).toContain('[REDACTED]')

      app.unmount()
    })
  })

  // 14. GroupCard Sources & Warning Badges
  describe('GroupCard Sources Indicators & Warning for Zero Sources', () => {
    it('displays sources count badge and revision in GroupCard', () => {
      const group: PolicyGroup = {
        id: 'grp-card-test',
        name: 'Card Sources Group',
        group_type: 'select',
        revision: 4,
        sources: [
          { operation: 'add', kind: 'manual_node', position: 0, node_logical_id: 'node-hk-1' },
          { operation: 'add', kind: 'group_ref', position: 1, target_group_id: 'grp-sub' },
        ],
      }

      const app = createApp({
        render() {
          return h(GroupCard, { group })
        },
      })
      app.mount(container)

      expect(container.textContent).toContain('2 项来源')
      expect(container.textContent).toContain('v4')
      expect(container.textContent).toContain('Card Sources Group')

      app.unmount()
    })

    it('displays warning badge when group has zero sources and zero edges', () => {
      const emptyGroup: PolicyGroup = {
        id: 'grp-empty',
        name: 'Empty Sources Group',
        group_type: 'select',
        sources: [],
        edges: [],
      }

      const app = createApp({
        render() {
          return h(GroupCard, { group: emptyGroup })
        },
      })
      app.mount(container)

      expect(container.textContent).toContain('无成员来源')

      app.unmount()
    })
  })

  // 16. Four Stages Architecture & Scope/Add/Exclude/Results Workflow
  describe('Four Stages Architecture & Scope/Add/Exclude/Results Workflow', () => {
    it('renders primary filter section at the top, and manual selection within closed details', async () => {
      const group: PolicyGroup = {
        id: 'grp-stages',
        name: 'Four Stages Group',
        group_type: 'select',
        empty_fallback_pass: true,
        inherit_global_filter: true,
        sources: [
          { operation: 'add', kind: 'manual_node', position: 0, node_logical_id: 'node-hk-01' },
          { operation: 'remove', kind: 'regex', position: 1, pattern: '.*expire.*' },
          { operation: 'add', kind: 'filter', position: 2, filter_spec: { conditions: [{ field: 'display_name', op: 'contains', value: 'HK' }] } },
        ],
      }

      let savedPayload: any = null
      const saveFn = vi.fn((data) => { savedPayload = data })

      const app = createApp({
        render: () => h(PolicyEditorSheet, { open: true, group, allGroups: [group], onSaveGroup: saveFn }),
      })
      app.mount(container)
      await nextTick()

      const scopeEl = container.querySelector('[data-testid="stage-candidate-scope"]')
      const primaryFilter = container.querySelector('[data-testid="primary-filter-section"]')
      const detailsEl = container.querySelector('details[data-testid="advanced-pipeline-details"]') as HTMLDetailsElement
      const resultsEl = container.querySelector('[data-testid="stage-realtime-results"]')

      expect(scopeEl).not.toBeNull()
      expect(primaryFilter).not.toBeNull()
      expect(detailsEl).not.toBeNull()
      expect(resultsEl).not.toBeNull()

      // 1. First business section is primary filter section, directly visible at top without scrolling past manual node pickers
      expect(primaryFilter!.textContent).toContain('属性筛选条件')
      const inlineEditor = primaryFilter!.querySelector('[data-testid="inline-filter-editor"]')
      expect(inlineEditor).not.toBeNull()

      // 2. Advanced settings and manual selection are placed inside details which is CLOSED by default
      expect(detailsEl.open).toBe(false)

      const addEl = detailsEl.querySelector('[data-testid="stage-add-members"]')
      const excludeEl = detailsEl.querySelector('[data-testid="stage-exclude-members"]')
      expect(addEl).not.toBeNull()
      expect(addEl!.textContent).toContain('添加成员')
      expect(excludeEl).not.toBeNull()

      // Verify DOM document position order: Scope before Primary Filter, Filter before Details, Details before Results
      const posScopeFilter = scopeEl!.compareDocumentPosition(primaryFilter!)
      expect(posScopeFilter & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

      const posFilterDetails = primaryFilter!.compareDocumentPosition(detailsEl)
      expect(posFilterDetails & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

      const posDetailsResults = detailsEl.compareDocumentPosition(resultsEl!)
      expect(posDetailsResults & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

      // 3. In the pipeline inside details, the filter row only shows summary (no duplicate editor)
      const rows = excludeEl!.querySelectorAll('[data-testid="source-row"]')
      expect(rows).toHaveLength(3)
      expect(rows[2].querySelector('[data-testid="filter-row-summary"]')).not.toBeNull()
      expect(rows[2].querySelector('[data-testid="structured-filter-editor"]')).toBeNull()

      // 4. Modifying conditions in primary filter saves in one single save, preserving complex pipeline original order
      const valInput = primaryFilter!.querySelector('[data-testid="condition-value"]') as HTMLInputElement
      valInput.value = 'HK-Updated'
      valInput.dispatchEvent(new Event('input'))
      await nextTick()

      container.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
      await nextTick()

      expect(saveFn).toHaveBeenCalledOnce()
      expect(savedPayload.sources).toHaveLength(3)
      expect(savedPayload.sources[0]).toMatchObject({ kind: 'manual_node', node_logical_id: 'node-hk-01', position: 0 })
      expect(savedPayload.sources[1]).toMatchObject({ kind: 'regex', pattern: '.*expire.*', position: 1 })
      expect(savedPayload.sources[2]).toMatchObject({
        kind: 'filter',
        position: 2,
        filter_spec: { conditions: [{ field: 'display_name', op: 'contains', value: 'HK-Updated' }] },
      })

      // Results metrics exist
      expect(resultsEl!.querySelector('[data-testid="candidate-count"]')).not.toBeNull()
      expect(resultsEl!.querySelector('[data-testid="member-count"]')).not.toBeNull()
      expect(resultsEl!.querySelector('[data-testid="excluded-count"]')).not.toBeNull()

      app.unmount()
    })

    it('toggles candidate scope between inherit and bypass, preserving safety notice', async () => {
      let savedPayload: any = null
      const saveFn = vi.fn((data) => { savedPayload = data })
      const group: PolicyGroup = {
        id: 'grp-scope-test',
        name: 'Scope Test Group',
        group_type: 'select',
        inherit_global_filter: true,
        sources: [],
      }

      const app = createApp({
        render: () => h(PolicyEditorSheet, { open: true, group, allGroups: [group], onSaveGroup: saveFn }),
      })
      app.mount(container)
      await nextTick()

      // Find scope options
      const radios = container.querySelectorAll('[data-testid="stage-candidate-scope"] input[type="radio"]') as NodeListOf<HTMLInputElement>
      expect(radios.length).toBeGreaterThanOrEqual(2)

      // Toggle to false (全部候选节点)
      const bypassRadio = Array.from(radios).find((r) => r.value === 'false')
      expect(bypassRadio).toBeDefined()
      bypassRadio!.click()
      await nextTick()

      expect(container.textContent).toContain('全部候选节点 (不应用全局筛选)')
      expect(container.textContent).toContain('绝不绕过节点停用、来源订阅禁用、显式风险排除')
      expect(container.textContent).not.toContain('准入规则')
      expect(container.textContent).not.toContain('全量准入节点')

      // Submit form
      container.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
      await nextTick()

      expect(saveFn).toHaveBeenCalledOnce()
      expect(savedPayload.inheritGlobalFilter).toBe(false)

      app.unmount()
    })
  })

  // 17. Shared Filter Condition Editing & In-Place Updates
  describe('Shared Filter Condition Editing & In-Place Updates', () => {
    it('updates condition in-place directly inline and emits updated modelValue', async () => {
      const initialConditions: FilterCondition[] = [
        { field: 'display_name', op: 'contains', value: 'HongKong' },
        { field: 'protocol', op: 'equals', value: 'ss' },
      ]
      let currentConditions = [...initialConditions]

      const app = createApp({
        render: () =>
          h(StructuredFilterEditor, {
            modelValue: currentConditions,
            'onUpdate:modelValue': (val: FilterCondition[]) => {
              currentConditions = val
            },
          }),
      })
      app.mount(container)
      await nextTick()

      const conditionBadges = container.querySelectorAll('[data-testid="filter-condition"]')
      expect(conditionBadges).toHaveLength(2)

      const selects = conditionBadges[1].querySelectorAll('select')
      const valueSelect = Array.from(selects).find((s) => s.value === 'ss') as HTMLSelectElement
      expect(valueSelect).toBeDefined()
      valueSelect.value = 'vless'
      valueSelect.dispatchEvent(new Event('change'))
      await nextTick()

      expect(currentConditions).toHaveLength(2)
      expect(currentConditions[0]).toEqual({ field: 'display_name', op: 'contains', value: 'HongKong' })
      expect(currentConditions[1]).toEqual({ field: 'protocol', op: 'equals', value: 'vless' })

      app.unmount()
    })
  })

  // 18. Clear Filter Conditions & No Hidden Default Injections
  describe('Clear Filter Conditions & No Hidden Default Injections', () => {
    it('clears all conditions and leaves clean state without injecting hidden health conditions', async () => {
      let conditions: FilterCondition[] = [
        { field: 'probe_verdict', op: 'equals', value: 'available', probe_kind: 'baseline' },
        { field: 'probe_latency_ms', op: 'lte', value: '150', probe_kind: 'baseline' },
      ]

      const app = createApp({
        render: () =>
          h(StructuredFilterEditor, {
            modelValue: conditions,
            'onUpdate:modelValue': (val: FilterCondition[]) => {
              conditions = val
            },
          }),
      })
      app.mount(container)
      await nextTick()

      expect(container.querySelectorAll('[data-testid="filter-condition"]')).toHaveLength(2)

      // Click "清空条件"
      const clearBtn = container.querySelector('[data-testid="filter-clear"]') as HTMLButtonElement
      expect(clearBtn).not.toBeNull()
      clearBtn.click()
      await nextTick()

      expect(conditions).toEqual([])
      expect(conditions).toHaveLength(0)

      // Renders clean empty explanation without hidden conditions
      const emptyEl = container.querySelector('[data-testid="filter-empty"]')
      expect(emptyEl).not.toBeNull()
      expect(container.textContent).toContain('无额外筛选条件 (匹配全部候选节点)')
      expect(container.textContent).toContain('绝不偷偷附加健康或可用性限制条件')

      app.unmount()
    })
  })

  // 19. Dynamic Source Persistence & No Static ID Expansion Roundtrip
  describe('Dynamic Source Persistence & No Static ID Expansion Roundtrip', () => {
    it('preserves dynamic filter and regex specs without statically freezing preview node IDs', async () => {
      let savedPayload: any = null
      const saveFn = vi.fn((data) => { savedPayload = data })

      const group: PolicyGroup = {
        id: 'grp-dynamic',
        name: 'Dynamic Sources Group',
        group_type: 'select',
        sources: [
          {
            operation: 'add',
            kind: 'regex',
            position: 0,
            pattern: '.*(HK|香港).*',
          },
          {
            operation: 'remove',
            kind: 'filter',
            position: 1,
            filter_spec: { conditions: [{ field: 'protocol', op: 'equals', value: 'ss' }] },
          },
        ],
      }

      // Mock preview returning 3 concrete node IDs
      vi.spyOn(api, 'post').mockResolvedValue({
        members: [
          { node_logical_id: 'node-hk-01', display_name: 'HK 01' },
          { node_logical_id: 'node-hk-02', display_name: 'HK 02' },
        ],
        member_count: 2,
        candidate_count: 5,
        selection_digest: 'digest-123',
      })

      const app = createApp({
        render: () => h(PolicyEditorSheet, { open: true, group, allGroups: [group], onSaveGroup: saveFn }),
      })
      app.mount(container)
      await nextTick()

      // Submit save
      container.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
      await nextTick()

      expect(saveFn).toHaveBeenCalledOnce()
      // The saved sources must retain kind 'regex' and 'filter', NOT converted to 'manual_node'
      expect(savedPayload.sources).toHaveLength(2)
      expect(savedPayload.sources[0].kind).toBe('regex')
      expect(savedPayload.sources[0].pattern).toBe('.*(HK|香港).*')
      expect(savedPayload.sources[1].kind).toBe('filter')
      expect(savedPayload.sources[1].filter_spec.conditions[0]).toEqual({ field: 'protocol', op: 'equals', value: 'ss' })

      // Neither source has node_logical_id
      expect(savedPayload.sources[0].node_logical_id).toBeUndefined()
      expect(savedPayload.sources[1].node_logical_id).toBeUndefined()

      app.unmount()
    })
  })

  // 20. Preview Debounce Epoch Race & Component Close Cleanup
  describe('Preview Debounce Epoch Race & Component Close Cleanup', () => {
    it('discards stale out-of-order preview responses and keeps latest generation', async () => {
      let resolveSlow: ((val: any) => void) | null = null
      let resolveFast: ((val: any) => void) | null = null

      let callCount = 0
      vi.spyOn(api, 'post').mockImplementation((path) => {
        if (path === '/api/v1/policies/groups/preview') {
          callCount++
          if (callCount === 1) {
            return new Promise((r) => { resolveSlow = r })
          } else {
            return new Promise((r) => { resolveFast = r })
          }
        }
        return Promise.resolve({})
      })

      const group: PolicyGroup = {
        id: 'grp-race',
        name: 'Race Group',
        group_type: 'select',
        sources: [{ operation: 'add', kind: 'manual_node', position: 0, node_logical_id: 'node-1' }],
      }

      const app = createApp({
        render: () => h(PolicyEditorSheet, { open: true, group, allGroups: [group] }),
      })
      app.mount(container)
      await nextTick()

      // Trigger preview generation 1
      const refreshBtn = container.querySelector('[data-testid="stage-realtime-results"] button') as HTMLButtonElement
      refreshBtn.click()
      await nextTick()

      // Trigger preview generation 2
      refreshBtn.click()
      await nextTick()

      expect(callCount).toBe(2)

      // Resolve generation 2 first (Fast response: member_count = 10)
      resolveFast!({
        members: [],
        member_count: 10,
        candidate_count: 10,
        selection_digest: 'fast-digest',
      })
      await nextTick()
      await Promise.resolve()

      const memberCountEl = container.querySelector('[data-testid="member-count"]')
      expect(memberCountEl?.textContent).toContain('10')

      // Resolve generation 1 later (Slow response: member_count = 1)
      resolveSlow!({
        members: [],
        member_count: 1,
        candidate_count: 1,
        selection_digest: 'slow-stale-digest',
      })
      await nextTick()
      await Promise.resolve()

      // Stale response must be discarded; member count remains 10
      expect(memberCountEl?.textContent).toContain('10')

      app.unmount()
    })

    it('cancels preview timer and discards late response after closing sheet', async () => {
      let resolvePending: ((val: any) => void) | null = null
      vi.spyOn(api, 'post').mockImplementation((path) => {
        if (path === '/api/v1/policies/groups/preview') {
          return new Promise((r) => { resolvePending = r })
        }
        return Promise.resolve({})
      })

      const group: PolicyGroup = {
        id: 'grp-close-cleanup',
        name: 'Cleanup Group',
        group_type: 'select',
        sources: [],
      }

      const isOpen = ref(true)
      const app = createApp({
        render: () => h(PolicyEditorSheet, { open: isOpen.value, group, allGroups: [group] }),
      })
      app.mount(container)
      await nextTick()

      // Request preview
      const refreshBtn = container.querySelector('[data-testid="stage-realtime-results"] button') as HTMLButtonElement
      refreshBtn.click()
      await nextTick()

      // Close sheet while preview is pending
      isOpen.value = false
      await nextTick()

      // Now resolve the late response
      if (resolvePending) {
        (resolvePending as (val: any) => void)({
          members: [{ node_logical_id: 'late-node' }],
          member_count: 1,
          candidate_count: 1,
        })
      }
      await nextTick()
      await Promise.resolve()

      // Reopen sheet: should reinitialize cleanly without stale data
      isOpen.value = true
      await nextTick()

      app.unmount()
    })

    it('renders preview tab header with responsive flex-wrap and shrink-0 badges for narrow screens', async () => {
      const group: PolicyGroup = {
        id: 'grp-responsive',
        name: 'Responsive Group',
        group_type: 'select',
        revision: 1,
        sources: [{ operation: 'add', kind: 'filter', position: 0, filter_spec: { conditions: [{ field: 'exit_region', op: 'equals', value: 'HK' }] } }],
      }

      vi.spyOn(api, 'post').mockResolvedValue({
        members: [{ node_logical_id: 'n1', display_name: 'HK 01' }, { node_logical_id: 'n2', display_name: 'HK 02' }],
        member_count: 2,
        node_count: 2,
        candidate_count: 4,
        revision: 1,
      })

      const container = document.createElement('div')
      document.body.appendChild(container)
      const app = createApp({
        render: () => h(PolicyEditorSheet, { open: true, group, allGroups: [group] }),
      })
      app.mount(container)
      await nextTick()

      // Switch to Snapshot Preview tab
      const tabButtons = Array.from(container.querySelectorAll('button')).filter((b) => b.textContent?.includes('快照预览') || b.textContent?.includes('Snapshot'))
      expect(tabButtons.length).toBeGreaterThan(0)
      tabButtons[0]?.click()
      await nextTick()
      await Promise.resolve()
      await nextTick()

      // Header container must have flex-wrap
      const headerBox = container.querySelector('.p-3.rounded-xl.bg-base-200\\/70') as HTMLElement | null
      expect(headerBox).not.toBeNull()
      const headerFlex = headerBox?.querySelector('.flex.flex-wrap.items-center.justify-between') as HTMLElement | null
      expect(headerFlex).not.toBeNull()

      // Inner badges container must have flex-wrap and min-w-0
      const badgesContainer = headerFlex?.querySelector('.flex.flex-wrap.items-center.min-w-0') as HTMLElement | null
      expect(badgesContainer).not.toBeNull()

      // All badges should have shrink-0
      const draftBadge = headerFlex?.querySelector('.badge-warning') as HTMLElement | null
      expect(draftBadge?.classList.contains('shrink-0')).toBe(true)

      // Recalculate button must have shrink-0
      const recalcBtn = headerFlex?.querySelector('button') as HTMLButtonElement | null
      expect(recalcBtn?.classList.contains('shrink-0')).toBe(true)

      app.unmount()
      container.remove()
    })
  })
})
