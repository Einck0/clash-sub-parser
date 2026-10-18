<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  XMarkIcon,
  PlusIcon,
  TrashIcon,
  MagnifyingGlassIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  ChevronUpIcon,
  ChevronDownIcon,
  Bars3Icon,
  ArrowPathIcon,
  ExclamationTriangleIcon,
  ListBulletIcon,
  EyeIcon,
} from '@heroicons/vue/24/outline'
import type {
  BuiltinName,
  FilterCondition,
  FilterField,
  FilterOp,
  GroupDraftPreviewPayload,
  GroupDraftPreviewResponse,
  GroupType,
  MemberSource,
  MemberSourceKind,
  PolicyGroup,
  ResolvedGroupMember,
  SourceOperation,
} from './policyTypes'
import {
  ALL_GROUP_TYPES,
  SUPPORTED_FILTER_FIELDS,
  VALID_BUILTIN_NAMES,
  groupTypeLabel,
  groupTypeSupportedTargets,
  normalizeGroupSources,
  sourceKindLabel,
  sourceOperationLabel,
  computeSourceSemantics,
  validateConditionInput,
  validateSourceInput,
} from './policyTypes'
import { api, ApiError } from '../../api/client'
import { useBodyScrollLock } from '../../composables/useBodyScrollLock'
import { t } from '../../locales'

interface Props {
  open: boolean
  group: PolicyGroup | null
  allGroups: PolicyGroup[]
  mode?: 'group' | 'edges'
  saving?: boolean
  availableNodes?: Array<{
    logical_id?: string
    logicalId?: string
    display_name?: string
    displayName?: string
    active?: boolean
    protocol?: string
  }>
  conflictError?: { currentRevision: number; message: string } | null
  cycleError?: { cyclePath: string[]; message: string } | null
  serverError?: string | null
}

const props = withDefaults(defineProps<Props>(), {
  mode: 'group',
  saving: false,
  availableNodes: () => [],
  conflictError: null,
  cycleError: null,
  serverError: null,
})

// Engage mobile body scroll lock when sheet is open
useBodyScrollLock(computed(() => props.open))

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'save-group', data: {
    name: string
    type: GroupType
    position?: number
    emptyFallbackPass: boolean
    inheritGlobalFilter: boolean
    sources: MemberSource[]
    expectedRevision?: number
  }): void
  (e: 'reload-group', id: string): void
  (e: 'clear-conflict'): void
}>()

// Active Drawer Tab: 'editor' | 'preview'
const activeSheetTab = ref<'editor' | 'preview'>('editor')

// Group metadata
const name = ref('')
const position = ref<number>(0)
const groupType = ref<GroupType>('select')
const emptyFallbackPass = ref(false)
const inheritGlobalFilter = ref(true)
const expectedRevision = ref<number>(1)

// Ordered sources pipeline
const sources = ref<MemberSource[]>([])
const filterConditions = ref<FilterCondition[]>([])
const editingFilterSource = ref<MemberSource | null>(null)

// Local conflict / cycle state
const localConflict = ref<{ currentRevision: number; message: string } | null>(null)
const localCycle = ref<{ cyclePath: string[]; message: string } | null>(null)

watch(() => props.conflictError, (c) => {
  if (c) localConflict.value = c
}, { immediate: true })
watch(() => props.cycleError, (c) => {
  if (c) localCycle.value = c
}, { immediate: true })

// Conditions remain a local source draft until explicitly added or updated.
const newField = ref<FilterField>('display_name')
const newOp = ref<FilterOp>('contains')
const newValue = ref('')
const newProbeKind = ref<'baseline' | 'geo' | 'streaming' | 'ai' | 'speed' | 'ip_risk'>('baseline')
const conditionError = ref('')

// New source entry form state
const newEdgeTarget = ref('')
const edgeError = ref('')

const selectedSourceKind = ref<'node' | 'group' | 'group_nodes' | 'regex' | 'filter' | 'typed_builtin'>('node')
const newSourceOp = ref<SourceOperation>('add')
const newSourcePattern = ref('')
const newSourceBuiltin = ref<BuiltinName>('DIRECT')
const sourceFormError = ref('')

// Search and pagination for candidate nodes
const nodeSearchQuery = ref('')
const nodePage = ref(1)
const nodePageSize = 50
const nodeTotal = ref(0)
const internalNodes = ref<Array<{ logicalId: string; displayName: string; active: boolean; protocol?: string }>>([])
const loadingNodes = ref(false)
const knownNodes = ref<Map<string, { logicalId: string; displayName: string; active: boolean; protocol?: string }>>(new Map())

let candidateSearchDebounceTimer: ReturnType<typeof setTimeout> | null = null
let candidateQuerySeq = 0

function registerKnownNode(n: { logicalId: string; displayName: string; active?: boolean; protocol?: string }) {
  if (n.logicalId) {
    knownNodes.value.set(n.logicalId, {
      logicalId: n.logicalId,
      displayName: n.displayName || n.logicalId,
      active: n.active !== false,
      protocol: n.protocol,
    })
  }
}

async function fetchCandidateNodes(p = 1, search = nodeSearchQuery.value) {
  const seq = ++candidateQuerySeq
  loadingNodes.value = true
  try {
    const params: Record<string, string | number> = {
      page: p,
      page_size: nodePageSize,
      scope: 'enabled_subscriptions',
      active_only: 'true',
    }
    if (search && search.trim()) {
      params.search = search.trim()
    }
    const result = await api.get<{
      items: Array<{ logical_id: string; display_name: string; active: boolean; protocol?: string }>
      page?: number
      total?: number
    }>('/api/v1/nodes', { params })

    if (seq !== candidateQuerySeq) return

    if (result && Array.isArray(result.items)) {
      const mapped = result.items.map((n) => ({
        logicalId: n.logical_id,
        displayName: n.display_name?.trim() || n.logical_id,
        protocol: n.protocol,
        active: n.active !== false,
      }))
      mapped.forEach(registerKnownNode)
      internalNodes.value = mapped
      nodePage.value = result.page || p
      nodeTotal.value = typeof result.total === 'number' ? result.total : mapped.length
    }
  } catch {
    // Graceful fallback
  } finally {
    if (seq === candidateQuerySeq) {
      loadingNodes.value = false
    }
  }
}

function onNodeSearchInput(e: Event) {
  const target = e.target as HTMLInputElement
  const query = target.value
  nodeSearchQuery.value = query
  if (candidateSearchDebounceTimer) clearTimeout(candidateSearchDebounceTimer)
  candidateSearchDebounceTimer = setTimeout(() => {
    nodePage.value = 1
    fetchCandidateNodes(1, query)
  }, 300)
}

function prevCandidatePage() {
  if (nodePage.value > 1) {
    fetchCandidateNodes(nodePage.value - 1)
  }
}

function nextCandidatePage() {
  if (nodePage.value * nodePageSize < nodeTotal.value) {
    fetchCandidateNodes(nodePage.value + 1)
  }
}

// Populate available nodes from props if provided
watch(
  () => props.availableNodes,
  (avail) => {
    if (avail && avail.length > 0) {
      avail.forEach((n) => {
        const id = ('logicalId' in n ? n.logicalId : undefined) || ('logical_id' in n ? n.logical_id : undefined) || ''
        const name = ('displayName' in n ? n.displayName : undefined) || ('display_name' in n ? n.display_name : undefined) || id
        if (id) {
          registerKnownNode({ logicalId: id, displayName: name, active: n.active !== false, protocol: n.protocol })
        }
      })
    }
  },
  { immediate: true }
)

const activeNodes = computed(() => {
  if (internalNodes.value.length > 0) {
    return internalNodes.value
  }
  const source = (props.availableNodes && props.availableNodes.length > 0)
    ? props.availableNodes
    : []

  const mapped = source
    .map((n) => ({
      logicalId: ('logicalId' in n ? n.logicalId : undefined) || ('logical_id' in n ? n.logical_id : undefined) || '',
      displayName: ('displayName' in n ? n.displayName : undefined) || ('display_name' in n ? n.display_name : undefined) || ('logicalId' in n ? n.logicalId : undefined) || ('logical_id' in n ? n.logical_id : undefined) || '',
      protocol: n.protocol,
      active: n.active !== false,
    }))
    .filter((n) => n.logicalId.length > 0)

  mapped.forEach(registerKnownNode)
  const activeOnly = mapped.filter((n) => n.active)
  return activeOnly.length > 0 ? activeOnly : mapped
})

// Candidate groups cache for group_ref / group_nodes (supports 150+ groups)
const candidateGroups = ref<PolicyGroup[]>([])
const loadingCandidateGroups = ref(false)

async function fetchCandidateGroups() {
  loadingCandidateGroups.value = true
  try {
    const all: PolicyGroup[] = []
    let p = 1
    const ps = 100
    while (true) {
      const res = await api.get<{
        items?: PolicyGroup[]
        groups?: PolicyGroup[]
        total?: number
      }>('/api/v1/policies/groups', {
        params: { page: p, page_size: ps },
      })
      const items = res.items || res.groups || []
      all.push(...items)
      if (items.length === 0 || all.length >= (res.total || 0) || items.length < ps) {
        break
      }
      p++
    }
    candidateGroups.value = all
  } catch {
    candidateGroups.value = props.allGroups || []
  } finally {
    loadingCandidateGroups.value = false
  }
}

const availableChildGroups = computed(() => {
  const base = candidateGroups.value.length > 0 ? candidateGroups.value : (props.allGroups || [])
  if (!props.group) return base
  return base.filter((g) => g.id !== props.group?.id)
})

function getGroupName(id?: string): string {
  if (!id) return ''
  const base = candidateGroups.value.length > 0 ? candidateGroups.value : props.allGroups
  const g = base.find((x) => x.id === id)
  return g ? `${g.name} (${groupTypeLabel(g.group_type)})` : id
}

function getNodeDisplayName(logicalId?: string): string {
  if (!logicalId) return ''
  const known = knownNodes.value.get(logicalId)
  if (known) {
    return `${known.displayName} (${logicalId})`
  }
  const n = activeNodes.value.find((x) => x.logicalId === logicalId)
  return n ? `${n.displayName} (${logicalId})` : logicalId
}

// Semantics analysis for current sources pipeline
const computedSemantics = computed(() => computeSourceSemantics(sources.value))

// Initialize form when sheet opens
watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      localConflict.value = props.conflictError || null
      localCycle.value = props.cycleError || null
      sourceFormError.value = ''
      conditionError.value = ''
      edgeError.value = ''
      nodeSearchQuery.value = ''
      newEdgeTarget.value = ''
      nodePage.value = 1
      activeSheetTab.value = 'editor'

      if (props.group) {
        name.value = props.group.name
        position.value = props.group.position ?? 0
        emptyFallbackPass.value = props.group.empty_fallback_pass === true
        inheritGlobalFilter.value = props.group.inherit_global_filter !== false
        groupType.value = props.group.group_type
        expectedRevision.value = props.group.revision ?? 1

        const normalized = normalizeGroupSources(props.group)
        sources.value = normalized.map((s, idx) => ({
          ...s, position: idx,
          ...(s.filter_spec ? { filter_spec: { conditions: s.filter_spec.conditions.map(c => ({ ...c })) } } : {}),
        }))
      } else {
        name.value = ''
        position.value = 0
        emptyFallbackPass.value = false
        inheritGlobalFilter.value = true
        groupType.value = 'select'
        expectedRevision.value = 1
        sources.value = []

      }

      discardSourceDraft()
      selectedSourceKind.value = 'node'
      newSourceOp.value = 'add'
      fetchCandidateNodes(1, '')
      fetchCandidateGroups()
    }
  },
  { immediate: true }
)

// Condition builder watch
watch(newField, (f) => {
  conditionError.value = ''
  if (f === 'display_name' || f === 'source_subscription_ids') {
    newOp.value = 'contains'
  } else if (f === 'probe_latency_ms') {
    newOp.value = 'lte'
  } else {
    newOp.value = 'equals'
  }
}, { flush: 'sync' })

function addFilterCondition() {
  conditionError.value = ''
  const cond: FilterCondition = {
    field: newField.value,
    op: newOp.value,
    value: newValue.value.trim(),
  }
  if (newField.value === 'probe_verdict' || newField.value === 'probe_latency_ms') {
    cond.probe_kind = newProbeKind.value
  }

  const err = validateConditionInput(cond)
  if (err) {
    conditionError.value = err
    return
  }

  if (filterConditions.value.length >= 32) {
    conditionError.value = '最多允许添加 32 条筛选条件'
    return
  }

  filterConditions.value.push(cond)

  newValue.value = ''
}

function editFilterCondition(idx: number) {
  const cond = filterConditions.value[idx]
  newField.value = cond.field
  newOp.value = cond.op
  newValue.value = cond.value || ''
  newProbeKind.value = cond.probe_kind || 'baseline'
  filterConditions.value.splice(idx, 1)

}

function removeFilterCondition(idx: number) {
  filterConditions.value.splice(idx, 1)
}

const hasPendingSourceDraft = computed(() =>
  editingFilterSource.value !== null || filterConditions.value.length > 0 ||
  newValue.value.trim().length > 0 || newSourcePattern.value.trim().length > 0
)

function discardSourceDraft() {
  editingFilterSource.value = null
  filterConditions.value = []
  newField.value = 'display_name'
  newOp.value = 'contains'
  newValue.value = ''
  newProbeKind.value = 'baseline'
  newSourcePattern.value = ''
  sourceFormError.value = ''
  conditionError.value = ''
  edgeError.value = ''
}

function editFilterSource(idx: number) {
  if (hasPendingSourceDraft.value) {
    sourceFormError.value = '已有来源草稿尚未应用，请先添加、更新或放弃来源草稿。'
    return
  }
  const source = sources.value[idx]
  editingFilterSource.value = source
  filterConditions.value = (source.filter_spec?.conditions || []).map(c => ({ ...c }))
  selectedSourceKind.value = 'filter'
  newSourceOp.value = source.operation
}

function checkPendingSourceDraft(): boolean {
  if (!hasPendingSourceDraft.value) return true
  sourceFormError.value = '来源草稿尚未应用：请先点击「添加来源」或「更新来源」，或明确放弃来源草稿后再保存。'
  return false
}

// Add new source item to the ordered pipeline
function addSourceItem() {
  sourceFormError.value = ''
  edgeError.value = ''
  let actualKind: MemberSourceKind = 'manual_node'
  if (selectedSourceKind.value === 'node') actualKind = 'manual_node'
  else if (selectedSourceKind.value === 'group') actualKind = 'group_ref'
  else actualKind = selectedSourceKind.value as MemberSourceKind

  const target = newEdgeTarget.value.trim()
  const nextPos = sources.value.length
  const newSource: MemberSource = {
    operation: newSourceOp.value,
    kind: actualKind,
    position: nextPos,
  }

  if (actualKind === 'manual_node') {
    newSource.node_logical_id = target
  } else if (actualKind === 'group_ref' || actualKind === 'group_nodes') {
    newSource.target_group_id = target
  } else if (actualKind === 'regex') {
    newSource.pattern = newSourcePattern.value.trim()
  } else if (actualKind === 'filter') {
    newSource.filter_spec = {
      conditions: [...filterConditions.value],
    }
  } else if (actualKind === 'typed_builtin') {
    newSource.builtin_name = newSourceBuiltin.value
  }

  const parentId = props.group?.id || ''
  const validation = validateSourceInput(newSource, parentId)
  if (validation) {
    sourceFormError.value = validation
    edgeError.value = validation
    return
  }

  if (actualKind === 'filter' && newValue.value.trim()) {
    sourceFormError.value = '还有未添加的匹配条件，请先点击「添加条件」。'
    return
  }

  if (actualKind === 'filter' && editingFilterSource.value) {
    const idx = sources.value.indexOf(editingFilterSource.value)
    newSource.position = idx
    sources.value[idx] = { ...editingFilterSource.value, ...newSource }
  } else {
    sources.value.push(newSource)
  }

  if (actualKind === 'filter') {
    editingFilterSource.value = null
    filterConditions.value = []
    newValue.value = ''
  } else if (actualKind === 'regex') {
    newSourcePattern.value = ''
  }
}

// Select all matching nodes on current candidate page
function selectAllCurrentPageNodes() {
  if (activeNodes.value.length === 0) return
  for (const node of activeNodes.value) {
    const nextPos = sources.value.length
    const source: MemberSource = {
      operation: newSourceOp.value,
      kind: 'manual_node',
      position: nextPos,
      node_logical_id: node.logicalId,
    }
    sources.value.push(source)
  }
}

// Select all filtered nodes across pages
const loadingAllCandidates = ref(false)

async function selectAllFilteredNodes() {
  if (loadingAllCandidates.value || loadingNodes.value) return
  loadingAllCandidates.value = true
  const seq = ++candidateQuerySeq
  try {
    const allNodes: Array<{ logicalId: string; displayName: string }> = []
    let p = 1
    const ps = 100
    while (true) {
      const params: Record<string, string | number> = {
        page: p,
        page_size: ps,
        scope: 'enabled_subscriptions',
        active_only: 'true',
      }
      if (nodeSearchQuery.value && nodeSearchQuery.value.trim()) {
        params.search = nodeSearchQuery.value.trim()
      }
      const res = await api.get<{
        items?: Array<{ logical_id: string; display_name: string; active?: boolean; protocol?: string }>
        total?: number
      }>('/api/v1/nodes', { params })

      if (seq !== candidateQuerySeq) return

      if (res && Array.isArray(res.items)) {
        res.items.forEach((n) => {
          registerKnownNode({ logicalId: n.logical_id, displayName: n.display_name?.trim() || n.logical_id, active: n.active !== false, protocol: n.protocol })
          allNodes.push({ logicalId: n.logical_id, displayName: n.display_name?.trim() || n.logical_id })
        })
        if (res.items.length === 0 || allNodes.length >= (res.total || 0) || res.items.length < ps) {
          break
        }
      } else {
        break
      }
      p++
    }

    if (seq !== candidateQuerySeq) return

    for (const node of allNodes) {
      const nextPos = sources.value.length
      const source: MemberSource = {
        operation: newSourceOp.value,
        kind: 'manual_node',
        position: nextPos,
        node_logical_id: node.logicalId,
      }
      sources.value.push(source)
    }
  } catch {
    // fallback
  } finally {
    if (seq === candidateQuerySeq) {
      loadingAllCandidates.value = false
    }
  }
}

// Reorder & toggle helpers
function moveSourceUp(idx: number) {
  if (idx <= 0) return
  const item = sources.value[idx]
  sources.value.splice(idx, 1)
  sources.value.splice(idx - 1, 0, item)
  sources.value.forEach((s, i) => { s.position = i })
}

function moveSourceDown(idx: number) {
  if (idx >= sources.value.length - 1) return
  const item = sources.value[idx]
  sources.value.splice(idx, 1)
  sources.value.splice(idx + 1, 0, item)
  sources.value.forEach((s, i) => { s.position = i })
}

function toggleSourceOperation(idx: number) {
  const s = sources.value[idx]
  s.operation = s.operation === 'add' ? 'remove' : 'add'
}

function removeSourceItem(idx: number) {
  if (sources.value[idx] === editingFilterSource.value) discardSourceDraft()
  sources.value.splice(idx, 1)
  sources.value.forEach((s, i) => { s.position = i })
}

// HTML5 Drag and Drop
const draggedIndex = ref<number | null>(null)

function onDragStart(idx: number, e: DragEvent) {
  draggedIndex.value = idx
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', String(idx))
  }
}

function onDragOver(idx: number, e: DragEvent) {
  e.preventDefault()
  if (e.dataTransfer) {
    e.dataTransfer.dropEffect = 'move'
  }
}

function onDrop(idx: number, e: DragEvent) {
  e.preventDefault()
  if (draggedIndex.value === null || draggedIndex.value === idx) return
  const item = sources.value[draggedIndex.value]
  sources.value.splice(draggedIndex.value, 1)
  sources.value.splice(idx, 0, item)
  sources.value.forEach((s, i) => { s.position = i })
  draggedIndex.value = null
}

const selectedManualNodes = computed(() => {
  return sources.value
    .map((s, idx) => ({ s, idx }))
    .filter(({ s }) => s.kind === 'manual_node' && s.node_logical_id)
})

function removeManualNode(logicalId: string) {
  const foundIdx = sources.value.findIndex(
    (s) => s.kind === 'manual_node' && s.node_logical_id === logicalId
  )
  if (foundIdx !== -1) {
    sources.value.splice(foundIdx, 1)
  }
}

function isNodeSelected(logicalId: string): boolean {
  return sources.value.some((s) => s.kind === 'manual_node' && s.node_logical_id === logicalId)
}

function quickToggleNode(logicalId: string) {
  if (isNodeSelected(logicalId)) {
    removeManualNode(logicalId)
  } else {
    newEdgeTarget.value = logicalId
    addSourceItem()
  }
}

const COMMON_REGEX_PRESETS = [
  { label: '🇭🇰 香港', pattern: '.*(HK|HongKong|香港).*' },
  { label: '🇯🇵 日本', pattern: '.*(JP|Japan|日本).*' },
  { label: '🇺🇸 美国', pattern: '.*(US|USA|United States|美国).*' },
  { label: '🇸🇬 新加坡', pattern: '.*(SG|Singapore|狮城|新加坡).*' },
  { label: '🇨🇦 加拿大', pattern: '.*(CA|Canada|加拿大).*' },
  { label: '🇹🇼 台湾', pattern: '.*(TW|Taiwan|台湾).*' },
]

function applyRegexPreset(pattern: string) {
  newSourcePattern.value = pattern
}

// Conflict resolutions
function handleReloadFromServer() {
  if (props.group) {
    emit('reload-group', props.group.id)
  }
  localConflict.value = null
  emit('clear-conflict')
}

function handlePreserveDraft() {
  if (localConflict.value) {
    expectedRevision.value = localConflict.value.currentRevision
  }
  localConflict.value = null
  emit('clear-conflict')
}

// Snapshot preview state
const previewLoading = ref(false)
const snapshotPreview = ref<GroupDraftPreviewResponse | null>(null)
const previewDiagnostics = ref<string[]>([])
const previewError = ref('')

function sanitizeErrorMessage(msg: string): string {
  if (!msg) return ''
  return msg
    .replace(/(?:token|secret|password|key|auth|bearer|credential)[=:\s]+[A-Za-z0-9_\-\.]{8,}/gi, (match) => {
      const prefix = match.split(/[=:\s]+/)[0]
      return `${prefix}: [REDACTED]`
    })
    .replace(/[A-Za-z0-9_\-]{32,}/g, '[REDACTED]')
}

async function loadSnapshotPreview() {
  previewLoading.value = true
  previewError.value = ''
  previewDiagnostics.value = []
  try {
    const payload: GroupDraftPreviewPayload = {
      group_id: props.group?.id,
      expected_revision: expectedRevision.value,
      name: name.value.trim() || (props.group?.name ?? '未命名策略组'),
      group_type: groupType.value,
      sources: sources.value.map((s, idx) => ({ ...s, position: idx })),
      target: 'mihomo',
    }
    const res = await api.post<GroupDraftPreviewResponse>('/api/v1/policies/groups/preview', payload)
    snapshotPreview.value = res
    previewDiagnostics.value = res.diagnostics || []
  } catch (err: any) {
    snapshotPreview.value = null
    if (err instanceof ApiError) {
      if (err.status === 422 && err.code === 'cycle_detected') {
        const cycle = (err.details as any)?.cycle_path || []
        previewError.value = `检测到循环依赖：${cycle.join(' → ')}`
      } else {
        const raw = err.message || `HTTP ${err.status}`
        previewError.value = `快照解析失败: ${sanitizeErrorMessage(raw)}`
      }
    } else {
      const raw = err instanceof Error ? err.message : '未知异常'
      previewError.value = `快照解析失败: ${sanitizeErrorMessage(raw)}`
    }
  } finally {
    previewLoading.value = false
  }
}

watch(activeSheetTab, (tab) => {
  if (tab === 'preview') {
    loadSnapshotPreview()
  }
})

// Submission handlers
function handleSave() {
  sourceFormError.value = ''
  if (!name.value.trim()) {
    sourceFormError.value = '策略组名称不能为空'
    return
  }

  if (!checkPendingSourceDraft()) return

  emit('save-group', {
    name: name.value.trim(),
    type: groupType.value,
    position: position.value,
    emptyFallbackPass: emptyFallbackPass.value,
    inheritGlobalFilter: inheritGlobalFilter.value,
    sources: sources.value.map((s, idx) => ({ ...s, position: idx })),
    expectedRevision: expectedRevision.value,
  })
}

function close() {
  emit('close')
}
</script>

<template>
  <!-- Soft Backdrop Blur Overlay -->
  <Transition
    enter-active-class="transition-opacity duration-200 ease-out"
    enter-from-class="opacity-0"
    enter-to-class="opacity-100"
    leave-active-class="transition-opacity duration-150 ease-in"
    leave-from-class="opacity-100"
    leave-to-class="opacity-0"
  >
    <div
      v-if="open"
      class="fixed inset-0 z-40 bg-black/40 backdrop-blur-sm"
      @click="close"
    />
  </Transition>

  <!-- Bottom Sheet / Modal Dialog -->
  <Transition
    enter-active-class="transition-all duration-200 ease-out"
    enter-from-class="translate-y-full md:translate-y-0 md:opacity-0 md:scale-95"
    enter-to-class="translate-y-0 md:opacity-100 md:scale-100"
    leave-active-class="transition-all duration-150 ease-in"
    leave-from-class="translate-y-0 md:opacity-100 md:scale-100"
    leave-to-class="translate-y-full md:translate-y-0 md:opacity-0 md:scale-95"
  >
    <section
      v-if="open"
      role="dialog"
      aria-modal="true"
      aria-labelledby="editor-title"
      class="fixed bottom-0 left-0 right-0 z-50 adaptive-surface-sheet md:max-h-[85vh] md:bottom-auto md:top-1/2 md:left-1/2 md:-translate-x-1/2 md:-translate-y-1/2 md:w-full md:max-w-2xl bg-base-100 rounded-t-2xl md:rounded-2xl border-t md:border border-base-300 shadow-2xl flex flex-col overflow-hidden max-w-full"
    >
      <!-- Grab Handle for Mobile Touch -->
      <div class="md:hidden pt-3 pb-1 flex justify-center flex-shrink-0 cursor-grab">
        <div class="w-12 h-1.5 rounded-full bg-base-content/20" />
      </div>

      <!-- Header with Tabs and Close Button -->
      <header class="flex flex-col border-b border-base-300 shrink-0 bg-base-100">
        <div class="flex items-start justify-between gap-3 p-4 sm:p-5 pb-2">
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <span class="text-xs font-semibold uppercase tracking-wider text-primary">策略编辑器</span>
              <span v-if="group?.revision !== undefined" class="badge badge-xs badge-ghost font-mono">
                v{{ expectedRevision }}
              </span>
            </div>
            <h2 id="editor-title" class="mt-0.5 text-lg sm:text-xl font-bold truncate">
              {{ mode === 'group' ? (group ? '编辑策略组' : '新建策略组') : '管理策略组来源' }}
            </h2>
          </div>
          <button
            type="button"
            class="btn btn-ghost btn-sm btn-circle shrink-0"
            aria-label="关闭"
            @click="close"
          >
            <XMarkIcon class="w-5 h-5" />
          </button>
        </div>

        <!-- Navigation Tabs: Editor vs Preview -->
        <div class="flex items-center gap-2 px-4 sm:px-5 pb-2 border-t border-base-300/40 pt-2 text-xs">
          <button
            type="button"
            class="btn btn-xs gap-1.5 font-sans"
            :class="activeSheetTab === 'editor' ? 'btn-primary' : 'btn-ghost'"
            @click="activeSheetTab = 'editor'"
          >
            <ListBulletIcon class="w-3.5 h-3.5" />
            编排配置 ({{ sources.length }} 来源项)
          </button>
          <button
            type="button"
            class="btn btn-xs gap-1.5 font-sans"
            :class="activeSheetTab === 'preview' ? 'btn-primary' : 'btn-ghost'"
            @click="activeSheetTab = 'preview'"
          >
            <EyeIcon class="w-3.5 h-3.5" />
            解析快照预览
          </button>
        </div>
      </header>

      <!-- Main Content Container -->
      <div class="flex-1 min-h-0 p-4 sm:p-5 overflow-y-auto overscroll-contain space-y-4">
        <div v-if="serverError" role="alert" class="alert alert-error text-xs p-3">
          <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
          <span>保存失败，来源草稿已保留：{{ sanitizeErrorMessage(serverError) }}</span>
        </div>

        <!-- 409 Revision Conflict Banner -->
        <div
          v-if="localConflict"
          role="alert"
          class="alert alert-warning shadow-sm text-xs p-3 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-2"
        >
          <div class="flex items-start gap-2 min-w-0">
            <ExclamationTriangleIcon class="w-5 h-5 shrink-0 text-warning" />
            <div>
              <p class="font-bold">版本冲突 (409 Conflict)</p>
              <p class="opacity-80 mt-0.5">
                {{ localConflict.message || `服务端数据已被其他会话更新至 v${localConflict.currentRevision}（当前预期 v${expectedRevision}）。` }}
              </p>
            </div>
          </div>
          <div class="flex items-center gap-2 shrink-0 self-end sm:self-auto">
            <button
              type="button"
              class="btn btn-xs btn-outline"
              @click="handleReloadFromServer"
            >
              重读服务端数据
            </button>
            <button
              type="button"
              class="btn btn-xs btn-warning"
              @click="handlePreserveDraft"
            >
              保留本地草稿
            </button>
          </div>
        </div>

        <!-- 422 Cycle Detected Banner -->
        <div
          v-if="localCycle"
          role="alert"
          class="alert alert-error shadow-sm text-xs p-3 space-y-1"
        >
          <div class="flex items-center gap-2">
            <ExclamationTriangleIcon class="w-5 h-5 shrink-0" />
            <span class="font-bold">检测到拓扑成环 (422 Unprocessable Entity)</span>
          </div>
          <p class="opacity-90 leading-relaxed">
            策略组间存在循环依赖引用：
            <span v-if="localCycle.cyclePath?.length" class="font-mono font-semibold">
              {{ localCycle.cyclePath.join(' → ') }}
            </span>
          </p>
        </div>

        <!-- TAB 1: Editor View (Group form or Edges form) -->
        <div v-if="activeSheetTab === 'editor'">
          <form id="policy-editor-group-form" class="space-y-4" @submit.prevent="handleSave">
            <!-- 1. Policy Group Name: Always FIRST input in the dialog! -->
            <div class="grid grid-cols-1 sm:grid-cols-4 gap-3">
              <label class="form-control sm:col-span-3">
                <span class="label-text font-semibold text-xs">策略组名称</span>
                <input
                  v-model="name"
                  required
                  placeholder="例如：代理节点、自动优选、流媒体解锁"
                  class="input input-bordered input-sm mt-1"
                />
              </label>
              <label class="form-control sm:col-span-1">
                <span class="label-text font-semibold text-xs">排序序号</span>
                <input
                  v-model.number="position"
                  type="number"
                  min="0"
                  placeholder="0"
                  class="input input-bordered input-sm mt-1 font-mono"
                  data-testid="group-position-input"
                />
              </label>
            </div>

            <!-- 2. Group Type Selector -->
            <div>
              <span class="label-text font-semibold text-xs block mb-1.5">策略组类型</span>
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
                <label
                  v-for="item in ALL_GROUP_TYPES"
                  :key="item.type"
                  class="flex items-start gap-2.5 p-2.5 rounded-lg border border-base-300 bg-base-200/50 cursor-pointer hover:border-primary/40 transition-colors"
                  :class="{ 'border-primary bg-primary/5': groupType === item.type }"
                >
                  <input
                    v-model="groupType"
                    type="radio"
                    :value="item.type"
                    class="radio radio-primary radio-sm mt-0.5"
                  />
                  <div class="min-w-0">
                    <span class="font-medium text-xs block leading-tight">{{ item.label }}</span>
                    <span class="text-[11px] opacity-60 block mt-0.5 leading-snug">{{ item.desc }}</span>
                    <span class="text-[10px] text-primary/90 font-mono block mt-1 leading-snug">
                      支持目标：{{ item.supportedTargets.join(', ') }}
                    </span>
                  </div>
                </label>
              </div>
            </div>

            <!-- 3. Inherit Global Filter & Empty Fallback Pass Checkboxes -->
            <div class="space-y-2">
              <label class="flex items-start gap-3 rounded-xl border border-base-300 p-3 bg-base-200/50 cursor-pointer">
                <input
                  v-model="inheritGlobalFilter"
                  type="checkbox"
                  class="checkbox checkbox-primary checkbox-sm mt-0.5"
                  data-testid="inherit-global-filter"
                />
                <span class="min-w-0 text-sm">
                  <span class="font-semibold block text-xs">继承全局过滤</span>
                  <span class="text-[11px] opacity-70 leading-snug block mt-0.5">
                    开启时仅匹配符合全局过滤条件的节点；关闭后可从全量接入节点中匹配（如用于 WARP 或专用线路）。
                  </span>
                </span>
              </label>

              <label class="flex items-start gap-3 rounded-xl border border-base-300 p-3 bg-base-200/50 cursor-pointer">
                <input
                  v-model="emptyFallbackPass"
                  type="checkbox"
                  class="checkbox checkbox-primary checkbox-sm mt-0.5"
                  data-testid="empty-fallback-pass"
                />
                <span class="min-w-0 text-sm">
                  <span class="font-semibold block text-xs">{{ t('policy.emptyFallbackPass') }}</span>
                  <span class="text-[11px] opacity-70 leading-snug block mt-0.5">
                    {{ t('policy.emptyFallbackPassHelp') }}
                  </span>
                </span>
              </label>
            </div>

            <!-- 5. Ordered Sources Pipeline Section -->
            <div class="p-3.5 rounded-xl bg-base-200/80 border border-base-300 space-y-3">
              <div class="flex items-center justify-between">
                <div>
                  <span class="font-bold text-xs">策略组成员节点与来源</span>
                  <p class="text-[11px] opacity-60 mt-0.5">
                    按配置顺序生成成员。支持指定具体节点、按关键词/正则匹配节点、引用子策略组及内建项。
                  </p>
                </div>
                <span class="badge badge-sm badge-info font-mono">{{ sources.length }} 项</span>
              </div>

              <!-- Sources List -->
              <div v-if="sources.length > 0" class="space-y-1.5">
                <div
                  v-for="(item, idx) in computedSemantics"
                  :key="item.source.id || idx"
                  data-testid="source-row"
                  draggable="true"
                  class="flex flex-col sm:flex-row sm:items-center justify-between gap-2 p-2 rounded-lg bg-base-100 border border-base-300 text-xs font-mono hover:border-primary/40"
                  :class="{ 'opacity-50 border-dashed': draggedIndex === idx }"
                  @dragstart="onDragStart(idx, $event)"
                  @dragover="onDragOver(idx, $event)"
                  @drop="onDrop(idx, $event)"
                >
                  <div class="flex items-center gap-2 min-w-0 flex-1 truncate">
                    <button type="button" class="cursor-grab active:cursor-grabbing p-1 opacity-40 hover:opacity-100 shrink-0" title="拖拽排序">
                      <Bars3Icon class="w-3.5 h-3.5" />
                    </button>
                    <span class="badge badge-xs badge-ghost font-mono shrink-0">#{{ idx + 1 }}</span>
                    <button
                      type="button"
                      class="btn btn-xs shrink-0 font-bold"
                      :class="item.source.operation === 'remove' ? 'btn-error' : 'btn-primary'"
                      :title="item.source.operation === 'remove' ? '切换为加入 (+)' : '切换为排除 (-)'"
                      @click="toggleSourceOperation(idx)"
                    >
                      {{ item.source.operation === 'remove' ? '- 排除' : '+ 加入' }}
                    </button>
                    <span class="badge badge-xs badge-outline shrink-0 font-sans">
                      {{ sourceKindLabel(item.source.kind) }}
                    </span>
                    <div class="min-w-0 flex-1 truncate text-[11px]">
                      <span v-if="item.source.kind === 'manual_node'" class="text-primary truncate">
                        {{ getNodeDisplayName(item.source.node_logical_id) }}
                      </span>
                      <span v-else-if="item.source.kind === 'group_ref'" class="text-secondary truncate">
                        组引用: {{ getGroupName(item.source.target_group_id) }}
                      </span>
                      <span v-else-if="item.source.kind === 'group_nodes'" class="text-secondary truncate">
                        解包子组: {{ getGroupName(item.source.target_group_id) }}
                      </span>
                      <span v-else-if="item.source.kind === 'regex'" class="text-accent truncate">
                        /{{ item.source.pattern }}/
                      </span>
                      <span v-else-if="item.source.kind === 'filter'" class="text-info truncate">
                        {{ item.source.filter_spec?.conditions?.map(c => `${c.field} ${c.op} "${c.value}"`).join(' && ') || '空条件' }}
                      </span>
                      <span v-else-if="item.source.kind === 'typed_builtin'" class="text-warning truncate">
                        {{ item.source.builtin_name }}
                      </span>
                    </div>
                  </div>

                  <div class="flex items-center gap-1 shrink-0 self-end sm:self-auto">
                    <span v-if="item.isReEntry" class="badge badge-xs badge-secondary font-sans">重入入选</span>
                    <button v-if="item.source.kind === 'filter'" type="button" class="btn btn-ghost btn-xs" title="编辑匹配条件" @click="editFilterSource(idx)">编辑条件</button>
                    <button type="button" class="btn btn-ghost btn-xs p-1" :disabled="idx === 0" title="上移" @click="moveSourceUp(idx)">
                      <ChevronUpIcon class="w-3.5 h-3.5" />
                    </button>
                    <button type="button" class="btn btn-ghost btn-xs p-1" :disabled="idx === sources.length - 1" title="下移" @click="moveSourceDown(idx)">
                      <ChevronDownIcon class="w-3.5 h-3.5" />
                    </button>
                    <button type="button" class="btn btn-ghost btn-xs text-error p-1" title="删除" @click="removeSourceItem(idx)">
                      <TrashIcon class="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              </div>
              <p v-else class="text-xs opacity-50 italic">
                尚未添加来源项（空策略组）。
              </p>

              <!-- Source Add Controls: Radio Selection -->
              <div class="pt-2 border-t border-base-300 space-y-2">
                <div class="flex flex-wrap items-center gap-3 text-xs">
                  <div class="flex items-center gap-2 border-r border-base-300 pr-3">
                    <label class="flex items-center gap-1 cursor-pointer">
                      <input
                        v-model="newSourceOp"
                        type="radio"
                        value="add"
                        class="radio radio-primary radio-xs"
                      />
                      <span class="font-semibold text-primary">+ 加入</span>
                    </label>
                    <label class="flex items-center gap-1 cursor-pointer">
                      <input
                        v-model="newSourceOp"
                        type="radio"
                        value="remove"
                        class="radio radio-error radio-xs"
                      />
                      <span class="font-semibold text-error">- 排除</span>
                    </label>
                  </div>

                  <label class="flex items-center gap-1.5 cursor-pointer">
                    <input
                      v-model="selectedSourceKind"
                      type="radio"
                      value="node"
                      class="radio radio-primary radio-xs"
                    />
                    <span>节点选择</span>
                  </label>
                  <label class="flex items-center gap-1.5 cursor-pointer">
                    <input
                      v-model="selectedSourceKind"
                      type="radio"
                      value="regex"
                      class="radio radio-primary radio-xs"
                    />
                    <span>名称正则</span>
                  </label>
                  <label class="flex items-center gap-1.5 cursor-pointer">
                    <input
                      v-model="selectedSourceKind"
                      type="radio"
                      value="group"
                      class="radio radio-primary radio-xs"
                    />
                    <span>策略组引用</span>
                  </label>
                  <label class="flex items-center gap-1.5 cursor-pointer">
                    <input
                      v-model="selectedSourceKind"
                      type="radio"
                      value="group_nodes"
                      class="radio radio-primary radio-xs"
                    />
                    <span>解包子组</span>
                  </label>
                  <label class="flex items-center gap-1.5 cursor-pointer">
                    <input
                      v-model="selectedSourceKind"
                      type="radio"
                      value="filter"
                      class="radio radio-primary radio-xs"
                    />
                    <span>高级过滤</span>
                  </label>
                  <label class="flex items-center gap-1.5 cursor-pointer">
                    <input
                      v-model="selectedSourceKind"
                      type="radio"
                      value="typed_builtin"
                      class="radio radio-primary radio-xs"
                    />
                    <span>内建流控</span>
                  </label>
                </div>

                <!-- Group target dropdown (group_ref or group_nodes) -->
                <div v-if="selectedSourceKind === 'group' || selectedSourceKind === 'group_nodes'" class="space-y-1.5">
                  <div class="flex gap-2">
                    <select
                      v-model="newEdgeTarget"
                      class="select select-bordered select-sm flex-1 text-xs"
                      data-testid="edge-group-select"
                    >
                      <option disabled value="">请选择子策略组...</option>
                      <option
                        v-for="cg in availableChildGroups"
                        :key="cg.id"
                        :value="cg.id"
                      >
                        {{ cg.name }} ({{ groupTypeLabel(cg.group_type) }})
                      </option>
                    </select>

                    <button
                      type="button"
                      class="btn btn-primary btn-sm gap-1 shrink-0"
                      :disabled="!newEdgeTarget"
                      @click="addSourceItem"
                    >
                      <PlusIcon class="w-4 h-4" />
                      {{ selectedSourceKind === 'group_nodes' ? (newSourceOp === 'remove' ? '排除解包子组 (-)' : '添加解包子组 (+)') : (newSourceOp === 'remove' ? '排除组引用 (-)' : '添加组引用 (+)') }}
                    </button>
                  </div>
                  <p class="text-[11px] opacity-70">
                    <span v-if="selectedSourceKind === 'group_nodes'" class="text-secondary font-mono">
                      解包子组 (group_nodes)：将目标策略组中的叶子节点展开为独立节点加入当前组流水线。
                    </span>
                    <span v-else class="text-primary font-mono">
                      策略组引用 (group_ref)：将目标策略组作为一个整体子组加入，保留其内部结构。
                    </span>
                  </p>
                </div>

                <!-- Node candidate search & dropdown -->
                <div v-else-if="selectedSourceKind === 'node'" class="space-y-2">
                  <!-- Already Selected Nodes Quick-Remove Tag Cloud -->
                  <div v-if="selectedManualNodes.length > 0" class="p-2.5 rounded-lg bg-base-100 border border-base-300 space-y-1.5">
                    <div class="flex items-center justify-between text-xs">
                      <span class="font-semibold text-primary">已添加的具体节点 ({{ selectedManualNodes.length }} 项)：</span>
                      <span class="text-[10px] opacity-60">点击 ✕ 快速移除</span>
                    </div>
                    <div class="flex flex-wrap gap-1.5 max-h-36 overflow-y-auto">
                      <span
                        v-for="item in selectedManualNodes"
                        :key="item.s.node_logical_id"
                        class="badge badge-sm badge-neutral gap-1 py-2 font-mono text-[11px]"
                      >
                        <span class="truncate max-w-[200px]">{{ getNodeDisplayName(item.s.node_logical_id) }}</span>
                        <button
                          type="button"
                          class="btn btn-ghost btn-circle btn-xs text-error hover:bg-error/20 -mr-1"
                          title="从当前策略组移除此节点"
                          @click="removeManualNode(item.s.node_logical_id!)"
                        >
                          ✕
                        </button>
                      </span>
                    </div>
                  </div>

                  <div class="relative">
                    <input
                      :value="nodeSearchQuery"
                      data-testid="candidate-node-search-input"
                      type="search"
                      placeholder="搜索节点名称或协议（可检索全部候选节点）..."
                      class="input input-bordered input-xs sm:input-sm w-full pl-8 font-mono text-xs"
                      @input="onNodeSearchInput"
                    />
                    <MagnifyingGlassIcon class="w-4 h-4 opacity-50 absolute left-2.5 top-1/2 -translate-y-1/2 pointer-events-none" />
                  </div>

                  <!-- Quick Candidate Toggle Chips -->
                  <div v-if="activeNodes.length > 0" class="space-y-1 pt-0.5">
                    <span class="text-[11px] opacity-70 block">候选节点快捷选择（点击加入/移除）：</span>
                    <div class="flex flex-wrap gap-1.5 max-h-28 overflow-y-auto">
                      <button
                        v-for="node in activeNodes.slice(0, 16)"
                        :key="node.logicalId"
                        type="button"
                        class="btn btn-xs font-mono text-[11px]"
                        :class="isNodeSelected(node.logicalId) ? 'btn-primary' : 'btn-outline'"
                        :title="isNodeSelected(node.logicalId) ? '已加入（点击移除）' : '点击加入当前策略组'"
                        @click="quickToggleNode(node.logicalId)"
                      >
                        <span class="truncate max-w-[140px]">{{ node.displayName }}</span>
                        <span class="text-[10px] font-bold">{{ isNodeSelected(node.logicalId) ? '✓' : '+' }}</span>
                      </button>
                    </div>
                  </div>

                  <div class="flex gap-2">
                    <select
                      v-model="newEdgeTarget"
                      class="select select-bordered select-sm flex-1 text-xs font-mono"
                      data-testid="edge-node-select"
                    >
                      <option disabled value="">
                        {{ loadingNodes ? '加载候选节点中...' : (activeNodes.length ? '请选择活跃节点...' : '暂无匹配候选节点') }}
                      </option>
                      <option
                        v-if="newEdgeTarget && !activeNodes.some((n) => n.logicalId === newEdgeTarget)"
                        :value="newEdgeTarget"
                      >
                        [当前跨页已选] {{ getNodeDisplayName(newEdgeTarget) }}
                      </option>
                      <option
                        v-for="node in activeNodes"
                        :key="node.logicalId"
                        :value="node.logicalId"
                      >
                        {{ node.displayName }} ({{ node.protocol ? node.protocol.toUpperCase() + ' · ' : '' }}{{ node.logicalId }})
                      </option>
                    </select>

                    <button
                      type="button"
                      class="btn btn-primary btn-sm gap-1 shrink-0"
                      :disabled="!newEdgeTarget"
                      @click="addSourceItem"
                    >
                      <PlusIcon class="w-4 h-4" />
                      {{ newSourceOp === 'remove' ? '排除节点 (-)' : '添加节点 (+)' }}
                    </button>
                  </div>

                  <!-- Candidate Pagination Controls & Select All on Page -->
                  <div class="flex flex-wrap items-center justify-between gap-2 text-[11px] opacity-75 pt-0.5 px-0.5">
                    <span class="font-mono">
                      候选节点：第 {{ nodePage }} / {{ Math.max(1, Math.ceil(nodeTotal / nodePageSize)) }} 页
                      <span v-if="nodeTotal > 0">（池中共 {{ nodeTotal }} 项）</span>
                    </span>
                    <div class="flex items-center gap-2">
                      <button
                        type="button"
                        class="btn btn-ghost btn-xs text-primary font-medium"
                        :disabled="activeNodes.length === 0"
                        @click="selectAllCurrentPageNodes"
                      >
                        全选当前页 ({{ activeNodes.length }})
                      </button>
                      <button
                        v-if="nodeTotal > activeNodes.length"
                        type="button"
                        class="btn btn-outline btn-xs font-medium"
                        :class="{ loading: loadingAllCandidates }"
                        :disabled="loadingAllCandidates || loadingNodes"
                        @click="selectAllFilteredNodes"
                      >
                        全选全部筛选 (共 {{ nodeTotal }} 项)
                      </button>
                      <button
                        type="button"
                        data-testid="candidate-prev-page"
                        class="btn btn-ghost btn-xs btn-circle"
                        :disabled="loadingNodes || nodePage <= 1"
                        title="上一页候选"
                        @click="prevCandidatePage"
                      >
                        <ChevronLeftIcon class="w-3.5 h-3.5" />
                      </button>
                      <button
                        type="button"
                        data-testid="candidate-next-page"
                        class="btn btn-ghost btn-xs btn-circle"
                        :disabled="loadingNodes || nodePage * nodePageSize >= nodeTotal"
                        title="下一页候选"
                        @click="nextCandidatePage"
                      >
                        <ChevronRightIcon class="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>

                  <!-- Selected Node Retained Tag -->
                  <div
                    v-if="newEdgeTarget"
                    class="text-[11px] p-2 rounded-lg bg-base-100 border border-base-300 flex items-center justify-between gap-2"
                  >
                    <span class="truncate">
                      当前选定目标：<strong class="text-primary">{{ getNodeDisplayName(newEdgeTarget) }}</strong>
                    </span>
                    <span class="badge badge-xs badge-outline shrink-0 font-mono">跨页锁定已保留</span>
                  </div>
                </div>

                <!-- Regex input -->
                <div v-else-if="selectedSourceKind === 'regex'" class="space-y-2">
                  <div class="flex flex-wrap items-center gap-1.5">
                    <span class="text-[11px] opacity-70 mr-1">常用地域预设：</span>
                    <button
                      v-for="pre in COMMON_REGEX_PRESETS"
                      :key="pre.label"
                      type="button"
                      class="btn btn-xs btn-ghost border border-base-300"
                      @click="applyRegexPreset(pre.pattern)"
                    >
                      {{ pre.label }}
                    </button>
                  </div>
                  <div class="flex gap-2">
                    <input
                      v-model="newSourcePattern"
                      type="text"
                      placeholder="例如：.*(HK|香港|HongKong).*"
                      class="input input-bordered input-sm flex-1 font-mono text-xs"
                      @keyup.enter="addSourceItem"
                    />
                    <button
                      type="button"
                      class="btn btn-primary btn-sm gap-1 shrink-0"
                      :disabled="!newSourcePattern.trim()"
                      @click="addSourceItem"
                    >
                      <PlusIcon class="w-4 h-4" />
                      {{ newSourceOp === 'remove' ? '添加排除正则 (-)' : '添加加入正则 (+)' }}
                    </button>
                  </div>
                  <p class="text-[11px] opacity-60">
                    提示：支持简单关键词或正则，例如输入 <code>加拿大</code> 可自动包含所有名称含加拿大的节点。
                  </p>
                </div>

                <!-- Filter source input -->
                <div v-else-if="selectedSourceKind === 'filter'" data-testid="condition-draft" class="space-y-3 p-2.5 rounded-lg bg-base-100 border border-base-300 text-xs">
                  <div class="flex flex-wrap items-center justify-between gap-2">
                    <span class="font-semibold">条件来源草稿 / 匹配条件</span>
                    <span class="badge badge-xs badge-info font-mono">{{ filterConditions.length }}/32 条条件</span>
                  </div>
                  <p class="text-[11px] opacity-70">
                    多个条件同时匹配（AND），包括名称正则、协议及地域等探针结果。这里只编辑草稿，不是整组筛选器。
                    {{ editingFilterSource ? `点击「更新来源」后在第 ${sources.indexOf(editingFilterSource) + 1} 项原位置生效` : `仅点击「添加来源」后追加为第 ${sources.length + 1} 项` }}，按当前操作{{ newSourceOp === 'remove' ? '排除 (-)' : '加入 (+)' }}匹配节点；后续来源仍可重新加入。
                  </p>
                  <div v-if="filterConditions.length" class="space-y-1.5">
                    <div v-for="(cond, cIdx) in filterConditions" :key="cIdx" class="flex flex-wrap items-center justify-between gap-2 p-2 rounded-lg bg-base-200 border border-base-300 text-xs font-mono">
                      <span class="min-w-0 break-all">{{ cond.field }} {{ cond.op }} "{{ cond.value }}" <span v-if="cond.probe_kind">({{ cond.probe_kind }})</span></span>
                      <div class="flex gap-1 shrink-0">
                        <button type="button" class="btn btn-ghost btn-xs" @click="editFilterCondition(cIdx)">编辑</button>
                        <button type="button" class="btn btn-ghost btn-xs text-error" title="删除条件" @click="removeFilterCondition(cIdx)"><TrashIcon class="w-3.5 h-3.5" /></button>
                      </div>
                    </div>
                  </div>
                  <p v-else class="text-xs opacity-60">尚无匹配条件。先添加条件，再添加来源。</p>
                  <div class="space-y-2 pt-2 border-t border-base-300/60">
                    <div class="grid grid-cols-1 sm:grid-cols-3 gap-2">
                      <select v-model="newField" data-testid="condition-field" aria-label="匹配字段" class="select select-bordered select-xs">
                        <option v-for="f in SUPPORTED_FILTER_FIELDS" :key="f.field" :value="f.field">{{ f.label }}</option>
                      </select>
                      <select v-model="newOp" data-testid="condition-op" aria-label="匹配运算符" class="select select-bordered select-xs font-mono">
                        <template v-if="newField === 'display_name' || newField === 'source_subscription_ids'">
                          <option value="contains">包含 (contains)</option>
                          <option value="not_contains">不包含 (not_contains)</option>
                          <option v-if="newField === 'display_name'" value="regex">匹配正则 (regex)</option>
                          <option v-if="newField === 'display_name'" value="not_regex">不匹配正则 (not_regex)</option>
                        </template>
                        <option v-else-if="newField === 'probe_latency_ms'" value="lte">小于等于 (lte)</option>
                        <template v-else>
                          <option value="equals">等于 (equals)</option>
                          <option value="not_equals">不等于 (not_equals)</option>
                        </template>
                      </select>
                      <select v-if="newField === 'probe_verdict'" v-model="newValue" data-testid="condition-value" aria-label="探针判定值" class="select select-bordered select-xs">
                        <option value="" disabled>选择判定结果</option>
                        <option value="available">可用 (available)</option>
                        <option value="restricted">受限 (restricted)</option>
                        <option value="unknown">未知 (unknown)</option>
                        <option value="error">错误 (error)</option>
                        <option value="stale">已过期 (stale)</option>
                      </select>
                      <input v-else v-model="newValue" data-testid="condition-value" aria-label="匹配目标值" class="input input-bordered input-xs" placeholder="匹配目标值..." @keydown.enter.prevent="addFilterCondition" />
                    </div>
                    <div v-if="newField === 'probe_verdict' || newField === 'probe_latency_ms'">
                      <select v-model="newProbeKind" aria-label="探针类型" class="select select-bordered select-xs w-full">
                        <option value="baseline">基础连通性 (baseline)</option>
                        <option value="geo">地域与出口 IP (geo)</option>
                        <option value="streaming">流媒体解锁 (streaming)</option>
                        <option value="ai">AI 服务可用性 (ai)</option>
                        <option value="speed">带宽测速 (speed)</option>
                        <option value="ip_risk">IP 风险度 (ip_risk)</option>
                      </select>
                    </div>
                    <p v-if="conditionError" role="alert" class="text-error text-xs">{{ conditionError }}</p>
                    <div class="flex flex-wrap gap-2">
                      <button type="button" class="btn btn-outline btn-xs gap-1" @click="addFilterCondition"><PlusIcon class="w-3.5 h-3.5" />添加条件</button>
                      <button type="button" class="btn btn-primary btn-xs gap-1" :disabled="!filterConditions.length || !!newValue.trim()" @click="addSourceItem"><PlusIcon class="w-3.5 h-3.5" />{{ editingFilterSource ? '更新来源' : '添加来源' }}</button>
                    </div>
                  </div>
                </div>

                <!-- Typed builtin input -->
                <div v-else-if="selectedSourceKind === 'typed_builtin'" class="space-y-1.5">
                  <div class="flex gap-2">
                    <select v-model="newSourceBuiltin" class="select select-bordered select-sm flex-1 text-xs font-mono">
                      <option v-for="b in VALID_BUILTIN_NAMES" :key="b" :value="b">{{ b }}</option>
                    </select>
                    <button
                      type="button"
                      class="btn btn-primary btn-sm gap-1 shrink-0"
                      @click="addSourceItem"
                    >
                      <PlusIcon class="w-4 h-4" />
                      {{ newSourceOp === 'remove' ? '添加排除内建项 (-)' : '添加加入内建项 (+)' }}
                    </button>
                  </div>
                  <p class="text-[11px] text-warning/90 opacity-80">
                    提示：内建流控项仅在 Mihomo 目标生效作为策略组成员；其他导出目标（如 sing-box / Surge / Quantumult X）仅导出纯节点列表，不包含策略组与内建项。
                  </p>
                </div>

                <p v-if="edgeError || sourceFormError" class="text-error text-xs">
                  {{ edgeError || sourceFormError }}
                </p>
              </div>
            </div>
          </form>
        </div>

        <!-- TAB 2: Snapshot Preview -->
        <div v-else-if="activeSheetTab === 'preview'" class="space-y-4">
          <div class="p-3 rounded-xl bg-base-200/70 border border-base-300 text-xs space-y-2">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2">
                <span class="badge badge-warning badge-sm font-semibold">草稿预览 (Draft Preview)</span>
                <span class="font-bold text-xs">实时未保存草稿解析</span>
                <span v-if="snapshotPreview" class="badge badge-xs badge-ghost font-mono">
                  v{{ snapshotPreview.revision ?? expectedRevision }}
                </span>
                <span v-if="snapshotPreview" class="badge badge-xs badge-outline font-mono">
                  共 {{ snapshotPreview.node_count }} 节点
                </span>
              </div>
              <button
                type="button"
                class="btn btn-ghost btn-xs gap-1"
                :class="{ loading: previewLoading }"
                @click="loadSnapshotPreview"
              >
                <ArrowPathIcon class="w-3.5 h-3.5" />
                重新计算草稿预览
              </button>
            </div>
            <p class="text-[11px] opacity-75">
              当前展示基于本表单未保存来源项的草稿解析结果。点击取消或关闭抽屉将放弃草稿，不持久化保存。
            </p>
          </div>

          <!-- Empty sources warning vs No query matches diagnostic -->
          <div v-if="sources.length === 0" class="alert alert-warning text-xs p-3">
            <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
            <span>空来源项 (Empty Sources)：当前策略组未配置任何成员来源项。请在编排配置页添加来源项。</span>
          </div>
          <div v-else-if="snapshotPreview && snapshotPreview.node_count === 0" class="alert alert-info text-xs p-3">
            <span>无匹配节点 (No Nodes Matched)：当前规则或筛选条件下未匹配到任何可用节点。</span>
          </div>

          <div v-if="previewError" class="alert alert-error text-xs p-3">
            <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
            <span>{{ previewError }}</span>
          </div>

          <!-- Backend diagnostics messages -->
          <div v-if="previewDiagnostics.length > 0" class="space-y-1">
            <div
              v-for="(diag, dIdx) in previewDiagnostics"
              :key="dIdx"
              class="alert alert-warning text-xs p-2.5 font-mono"
            >
              {{ diag }}
            </div>
          </div>

          <div v-if="snapshotPreview" class="space-y-3">
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 text-xs">
              <div class="p-2.5 rounded-lg bg-base-200 border border-base-300 font-mono space-y-0.5">
                <span class="text-[10px] uppercase opacity-60 block">选择集指纹 (Selection Digest)</span>
                <span class="font-bold text-[11px] truncate block" :title="snapshotPreview.selection_digest">
                  {{ (snapshotPreview.selection_digest || '').slice(0, 24) }}...
                </span>
              </div>

              <div class="p-2.5 rounded-lg bg-base-200 border border-base-300 font-mono space-y-0.5">
                <span class="text-[10px] uppercase opacity-60 block">内容指纹 (Content Digest)</span>
                <span class="font-bold text-[11px] truncate block" :title="snapshotPreview.content_digest">
                  {{ (snapshotPreview.content_digest || '').slice(0, 24) }}...
                </span>
              </div>
            </div>

            <!-- Resolved Members List from Server -->
            <div class="space-y-1.5 p-3 rounded-xl bg-base-200/50 border border-base-300">
              <div class="flex items-center justify-between text-xs">
                <span class="font-semibold">已解析成员清单 (Resolved Members)</span>
                <span class="badge badge-xs badge-info font-mono">{{ snapshotPreview.members?.length || 0 }} 项</span>
              </div>
              <div v-if="snapshotPreview.members && snapshotPreview.members.length > 0" class="space-y-1 max-h-48 overflow-y-auto">
                <div
                  v-for="(m, mIdx) in snapshotPreview.members"
                  :key="mIdx"
                  class="flex items-center justify-between p-1.5 rounded bg-base-100 border border-base-300/80 text-xs font-mono"
                >
                  <div class="flex items-center gap-1.5 truncate">
                    <span class="badge badge-xs badge-outline">{{ sourceKindLabel(m.kind) }}</span>
                    <span class="truncate text-primary">
                      {{ m.display_name || m.node_logical_id || m.group_id || m.builtin_name || '-' }}
                    </span>
                  </div>
                  <span class="text-[10px] opacity-50 font-mono">#{{ mIdx + 1 }}</span>
                </div>
              </div>
              <p v-else class="text-xs opacity-50 italic">
                解析器未返回任何成员项。
              </p>
            </div>

            <div class="space-y-1">
              <span class="text-xs font-semibold opacity-70">编译配置预览 (部分)：</span>
              <pre class="p-3 rounded-xl bg-base-300/80 font-mono text-[11px] max-h-48 overflow-y-auto whitespace-pre-wrap select-all">{{ snapshotPreview.content?.slice(0, 3000) || '无预览内容' }}</pre>
            </div>
          </div>
        </div>
      </div>

      <div v-if="hasPendingSourceDraft" role="status" class="px-4 sm:px-5 py-2 border-t border-base-300 bg-base-200 text-xs space-y-1">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span>来源草稿尚未应用，不参与预览或保存。请先添加 / 更新来源。</span>
          <button type="button" class="btn btn-ghost btn-xs" @click="discardSourceDraft">放弃来源草稿</button>
        </div>
        <p v-if="sourceFormError" class="text-error">{{ sourceFormError }}</p>
      </div>

      <!-- Footer Action Buttons -->
      <footer class="flex flex-wrap items-center justify-between gap-2 p-4 sm:p-5 border-t border-base-300 shrink-0 bg-base-100">
        <div class="flex items-center gap-2 text-xs opacity-60">
          <span class="font-mono">来源项: {{ sources.length }}</span>
          <span>·</span>
          <span class="font-mono">版本: v{{ expectedRevision }}</span>
        </div>

        <div class="flex items-center gap-2">
          <button
            type="button"
            class="btn btn-ghost btn-sm"
            :disabled="saving"
            @click="close"
          >
            取消
          </button>

          <button
            type="submit"
            form="policy-editor-group-form"
            class="btn btn-primary btn-sm"
            @click.prevent="handleSave"
            :class="{ loading: saving }"
            :disabled="saving || !name.trim()"
          >
            {{ mode === 'edges' ? '保存来源' : (group ? '保存策略组' : '创建策略组') }}
          </button>
        </div>
      </footer>
    </section>
  </Transition>
</template>
