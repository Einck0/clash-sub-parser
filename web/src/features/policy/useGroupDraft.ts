import { computed, ref, type ComputedRef, type Ref } from 'vue'
import type {
  GroupType,
  MemberSource,
  MemberSourceKind,
  PolicyGroup,
  SourceOperation,
} from './policyTypes'
import {
  normalizeGroupSources,
  validateSourceInput,
} from './policyTypes'

export interface GroupDraftSavePayload {
  name: string
  type: GroupType
  position?: number
  emptyFallbackPass: boolean
  inheritGlobalFilter: boolean
  sources: MemberSource[]
  expectedRevision?: number
}

// Stable client key management across drags and reorders
let sourceKeyCounter = 0
const sourceKeyMap = new WeakMap<MemberSource, string>()

export function getSourceKey(source: MemberSource): string {
  let key = sourceKeyMap.get(source)
  if (!key) {
    key = `src_${++sourceKeyCounter}_${source.id || Math.random().toString(36).slice(2, 7)}`
    sourceKeyMap.set(source, key)
  }
  return key
}

function cleanSourceForComparison(source: MemberSource): Record<string, unknown> {
  const clean: Record<string, unknown> = {
    operation: source.operation,
    kind: source.kind,
    position: source.position,
  }
  if (source.node_logical_id) clean.node_logical_id = source.node_logical_id
  if (source.target_group_id) clean.target_group_id = source.target_group_id
  if (source.pattern) clean.pattern = source.pattern
  if (source.builtin_name) clean.builtin_name = source.builtin_name
  if (source.filter_spec) {
    clean.filter_spec = {
      conditions: (source.filter_spec.conditions || []).map((c) => ({
        field: c.field,
        op: c.op,
        value: c.value,
        probe_kind: c.probe_kind,
        platform: c.platform,
      })),
    }
  }
  return clean
}

export function useGroupDraft() {
  const name = ref('')
  const groupType = ref<GroupType>('select')
  const position = ref(0)
  const emptyFallbackPass = ref(false)
  const inheritGlobalFilter = ref(true)
  const expectedRevision = ref(1)

  const sources = ref<MemberSource[]>([])
  const invalidFilterSources = ref<Set<string>>(new Set())

  // Pending draft input for regex or candidate entry
  const pendingRegexPattern = ref('')
  const formError = ref('')

  // Server error states
  const conflictError = ref<{ currentRevision: number; message: string } | null>(null)
  const cycleError = ref<{ cyclePath: string[]; message: string } | null>(null)
  const serverError = ref<string | null>(null)

  // Baseline serialization snapshot for isDirty detection
  const initialSnapshot = ref('')

  function captureSnapshot(): string {
    const data = {
      name: name.value.trim(),
      groupType: groupType.value,
      position: position.value,
      emptyFallbackPass: emptyFallbackPass.value,
      inheritGlobalFilter: inheritGlobalFilter.value,
      sources: sources.value.map(cleanSourceForComparison),
    }
    return JSON.stringify(data)
  }

  const isDirty = computed(() => {
    if (!initialSnapshot.value) return false
    if (pendingRegexPattern.value.trim().length > 0) return true
    return captureSnapshot() !== initialSnapshot.value
  })

  const hasInvalidFilterSources = computed(() => invalidFilterSources.value.size > 0)
  const hasPendingSourceDraft = computed(() => pendingRegexPattern.value.trim().length > 0)

  function initDraft(group?: PolicyGroup | null) {
    conflictError.value = null
    cycleError.value = null
    serverError.value = null
    formError.value = ''
    pendingRegexPattern.value = ''

    if (group) {
      name.value = group.name || ''
      groupType.value = group.group_type || 'select'
      position.value = group.position ?? 0
      emptyFallbackPass.value = group.empty_fallback_pass === true
      inheritGlobalFilter.value = group.inherit_global_filter !== false
      expectedRevision.value = group.revision ?? 1

      const normalized = normalizeGroupSources(group)
      sources.value = normalized.map((s, idx) => {
        const item: MemberSource = {
          ...s,
          position: idx,
          ...(s.filter_spec
            ? {
                filter_spec: {
                  conditions: (s.filter_spec.conditions || []).map((c) => ({ ...c })),
                },
              }
            : {}),
        }
        getSourceKey(item)
        return item
      })
    } else {
      name.value = ''
      groupType.value = 'select'
      position.value = 0
      emptyFallbackPass.value = false
      inheritGlobalFilter.value = true
      expectedRevision.value = 1
      sources.value = []
    }

    invalidFilterSources.value = new Set()
    initialSnapshot.value = captureSnapshot()
  }

  function resetDraft(group?: PolicyGroup | null) {
    initDraft(group)
  }

  function setFilterInvalid(key: string, isInvalid: boolean) {
    const next = new Set(invalidFilterSources.value)
    if (isInvalid) {
      next.add(key)
    } else {
      next.delete(key)
    }
    invalidFilterSources.value = next
  }

  function addSource(source: MemberSource) {
    formError.value = ''
    const nextPos = sources.value.length
    const item: MemberSource = {
      ...source,
      position: nextPos,
    }
    getSourceKey(item)
    sources.value.push(item)
  }

  function addFilterSource(op: SourceOperation = 'add') {
    const nextPos = sources.value.length
    const item: MemberSource = {
      operation: op,
      kind: 'filter',
      position: nextPos,
      filter_spec: {
        conditions: [{ field: 'display_name', op: 'contains', value: '' }],
      },
    }
    getSourceKey(item)
    sources.value.push(item)
  }

  function removeSource(index: number) {
    const item = sources.value[index]
    if (item) {
      const key = getSourceKey(item)
      const next = new Set(invalidFilterSources.value)
      next.delete(key)
      invalidFilterSources.value = next
    }
    sources.value.splice(index, 1)
    sources.value.forEach((s, i) => {
      s.position = i
    })
  }

  function moveSourceUp(index: number) {
    if (index <= 0) return
    const item = sources.value[index]
    sources.value.splice(index, 1)
    sources.value.splice(index - 1, 0, item)
    sources.value.forEach((s, i) => {
      s.position = i
    })
  }

  function moveSourceDown(index: number) {
    if (index >= sources.value.length - 1) return
    const item = sources.value[index]
    sources.value.splice(index, 1)
    sources.value.splice(index + 1, 0, item)
    sources.value.forEach((s, i) => {
      s.position = i
    })
  }

  function moveSource(fromIndex: number, toIndex: number) {
    if (fromIndex === toIndex || fromIndex < 0 || toIndex < 0 || fromIndex >= sources.value.length || toIndex >= sources.value.length) {
      return
    }
    const item = sources.value[fromIndex]
    sources.value.splice(fromIndex, 1)
    sources.value.splice(toIndex, 0, item)
    sources.value.forEach((s, i) => {
      s.position = i
    })
  }

  function toggleSourceOp(index: number) {
    const s = sources.value[index]
    if (s) {
      s.operation = s.operation === 'add' ? 'remove' : 'add'
    }
  }

  function resolveConflictKeepDraft(currentRevision: number) {
    expectedRevision.value = currentRevision
    conflictError.value = null
  }

  function clearConflict() {
    conflictError.value = null
  }

  function clearCycle() {
    cycleError.value = null
  }

  function clearServerError() {
    serverError.value = null
  }

  function checkDraftValidity(): boolean {
    if (hasInvalidFilterSources.value) {
      formError.value = '存在未完成或无效的筛选条件，请完善或删除后再保存。'
      return false
    }
    if (hasPendingSourceDraft.value) {
      formError.value = '来源草稿尚未应用：请先添加正则来源或明确放弃来源草稿后再保存。'
      return false
    }
    return true
  }

  function buildSavePayload(): GroupDraftSavePayload {
    return {
      name: name.value.trim(),
      type: groupType.value,
      position: position.value,
      emptyFallbackPass: emptyFallbackPass.value,
      inheritGlobalFilter: inheritGlobalFilter.value,
      expectedRevision: expectedRevision.value,
      sources: sources.value.map((s, idx) => {
        const clean: MemberSource = {
          operation: s.operation,
          kind: s.kind,
          position: idx,
        }
        if (s.id) clean.id = s.id
        if (s.group_id) clean.group_id = s.group_id
        if (s.node_logical_id) clean.node_logical_id = s.node_logical_id
        if (s.target_group_id) clean.target_group_id = s.target_group_id
        if (s.pattern) clean.pattern = s.pattern
        if (s.builtin_name) clean.builtin_name = s.builtin_name
        if (s.filter_spec) {
          clean.filter_spec = {
            conditions: (s.filter_spec.conditions || []).map((c) => ({ ...c })),
          }
        }
        return clean
      }),
    }
  }

  return {
    name,
    groupType,
    position,
    emptyFallbackPass,
    inheritGlobalFilter,
    expectedRevision,
    sources,
    invalidFilterSources,
    pendingRegexPattern,
    formError,
    conflictError,
    cycleError,
    serverError,
    isDirty,
    hasInvalidFilterSources,
    hasPendingSourceDraft,
    initDraft,
    resetDraft,
    setFilterInvalid,
    addSource,
    addFilterSource,
    removeSource,
    moveSourceUp,
    moveSourceDown,
    moveSource,
    toggleSourceOp,
    resolveConflictKeepDraft,
    clearConflict,
    clearCycle,
    clearServerError,
    checkDraftValidity,
    buildSavePayload,
  }
}

export type UseGroupDraftReturn = ReturnType<typeof useGroupDraft>
