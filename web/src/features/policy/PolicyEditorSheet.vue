<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
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
  NodeFilterSpec,
  PolicyGroup,
  ResolvedGroupMember,
  ResolverDiagnostic,
  SourceOperation,
} from './policyTypes'
import StructuredFilterEditor from './StructuredFilterEditor.vue'
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

// Snapshot preview state
const previewLoading = ref(false)
const snapshotPreview = ref<GroupDraftPreviewResponse | null>(null)
const previewDiagnostics = ref<Array<ResolverDiagnostic | string>>([])
const previewError = ref('')
const previewGeneration = ref(0)
let previewDebounceTimer: ReturnType<typeof setTimeout> | null = null

function clearPreviewTimer() {
  if (previewDebounceTimer) {
    clearTimeout(previewDebounceTimer)
    previewDebounceTimer = null
  }
}

// Group metadata
const name = ref('')
const position = ref<number>(0)
const groupType = ref<GroupType>('select')
const emptyFallbackPass = ref(false)
const inheritGlobalFilter = ref(true)
const expectedRevision = ref<number>(1)

// Ordered sources pipeline
const sources = ref<MemberSource[]>([])
const invalidFilterSources = ref(new Set<string>())

// Local conflict / cycle state
const localConflict = ref<{ currentRevision: number; message: string } | null>(null)
const localCycle = ref<{ cyclePath: string[]; message: string } | null>(null)

watch(() => props.conflictError, (c) => {
  if (c) localConflict.value = c
}, { immediate: true })
watch(() => props.cycleError, (c) => {
  if (c) localCycle.value = c
}, { immediate: true })


// New source entry form state
const newEdgeTarget = ref('')
const edgeError = ref('')

const selectedSourceKind = ref<'node' | 'group' | 'group_nodes' | 'regex' | 'filter' | 'typed_builtin'>('filter')
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

// Stable client key management and payload sanitization
let sourceKeyCounter = 0
const sourceKeyMap = new WeakMap<MemberSource, string>()

function getSourceKey(source: MemberSource): string {
  let key = sourceKeyMap.get(source)
  if (!key) {
    key = `src_${++sourceKeyCounter}_${source.id || Math.random().toString(36).slice(2, 7)}`
    sourceKeyMap.set(source, key)
  }
  return key
}

function sanitizeSourceForPayload(source: MemberSource): MemberSource {
  const clean = { ...source } as any
  delete clean._clientKey
  return clean
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
        sources.value = normalized.map((s, idx) => {
          const item: MemberSource = {
            ...s,
            position: idx,
            ...(s.kind === 'filter'
              ? { filter_spec: { conditions: (s.filter_spec?.conditions || []).map((c) => ({ ...c })) } }
              : (s.filter_spec ? { filter_spec: { conditions: s.filter_spec.conditions.map((c) => ({ ...c })) } } : {})),
          }
          getSourceKey(item)
          return item
        })
        invalidFilterSources.value = new Set()
      } else {
        name.value = ''
        position.value = 0
        emptyFallbackPass.value = false
        inheritGlobalFilter.value = true
        groupType.value = 'select'
        expectedRevision.value = 1
        sources.value = []
        invalidFilterSources.value = new Set()
      }

      discardSourceDraft()
      selectedSourceKind.value = 'filter'
      newSourceOp.value = 'add'
      fetchCandidateNodes(1, '')
      fetchCandidateGroups()
    } else {
      clearPreviewTimer()
      previewGeneration.value++
      snapshotPreview.value = null
      previewDiagnostics.value = []
    }
  },
  { immediate: true }
)

function ensureFilterSpec(source: MemberSource): NodeFilterSpec {
  if (!source.filter_spec) {
    source.filter_spec = { conditions: [] }
  }
  if (!source.filter_spec.conditions) {
    source.filter_spec.conditions = []
  }
  return source.filter_spec
}

const hasPendingSourceDraft = computed(() => newSourcePattern.value.trim().length > 0)

function discardSourceDraft() {
  newSourcePattern.value = ''
  sourceFormError.value = ''
  edgeError.value = ''
}

function setFilterSourceInvalid(key: string, invalid: boolean) {
  const next = new Set(invalidFilterSources.value)
  if (invalid) {
    next.add(key)
    clearPreviewTimer()
    previewGeneration.value++
    snapshotPreview.value = null
    previewDiagnostics.value = []
    previewLoading.value = false
  } else {
    next.delete(key)
  }
  invalidFilterSources.value = next
}

const hasInvalidFilterSources = computed(() => {
  return invalidFilterSources.value.size > 0
})

function checkPendingSourceDraft(): boolean {
  if (hasInvalidFilterSources.value) {
    sourceFormError.value = '存在未完成或无效的筛选条件，请完善或删除后再保存。'
    return false
  }
  if (!hasPendingSourceDraft.value) return true
  sourceFormError.value = '来源草稿尚未应用：请先添加正则来源或明确放弃来源草稿后再保存。'
  return false
}

const filterSources = computed(() => {
  return sources.value
    .map((source, originalIndex) => ({ source, originalIndex }))
    .filter((item) => item.source.kind === 'filter')
})

function addFilterSource(op: SourceOperation = 'add') {
  const nextPos = sources.value.length
  const newSource: MemberSource = {
    operation: op,
    kind: 'filter',
    position: nextPos,
    filter_spec: { conditions: [{ field: 'display_name', op: 'contains', value: '' }] },
  }
  getSourceKey(newSource)
  sources.value.push(newSource)
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
    newSource.filter_spec = { conditions: [{ field: 'display_name', op: 'contains', value: '' }] }
    getSourceKey(newSource)
    sources.value.push(newSource)
    return
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

  getSourceKey(newSource)
  sources.value.push(newSource)

  if (actualKind === 'regex') {
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
    getSourceKey(source)
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
      getSourceKey(source)
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
  const item = sources.value[idx]
  if (item) {
    const next = new Set(invalidFilterSources.value)
    next.delete(getSourceKey(item))
    invalidFilterSources.value = next
  }
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

function sanitizeErrorMessage(msg: string): string {
  if (!msg) return ''
  return msg
    .replace(/(?:token|secret|password|key|auth|bearer|credential)[=:\s]+[A-Za-z0-9_\-\.]{8,}/gi, (match) => {
      const prefix = match.split(/[=:\s]+/)[0]
      return `${prefix}: [REDACTED]`
    })
    .replace(/[A-Za-z0-9_\-]{32,}/g, '[REDACTED]')
}

async function requestLivePreview(forcedGen?: number) {
  if (!props.open) return
  if (hasInvalidFilterSources.value) {
    clearPreviewTimer()
    previewGeneration.value++
    snapshotPreview.value = null
    previewDiagnostics.value = []
    previewLoading.value = false
    return
  }
  const currentGen = forcedGen ?? ++previewGeneration.value
  previewLoading.value = true
  previewError.value = ''
  try {
    const payload: GroupDraftPreviewPayload = {
      group_id: props.group?.id,
      expected_revision: expectedRevision.value,
      name: name.value.trim() || (props.group?.name ?? '未命名策略组'),
      group_type: groupType.value,
      empty_fallback_pass: emptyFallbackPass.value,
      inherit_global_filter: inheritGlobalFilter.value,
      sources: sources.value.map((s, idx) => sanitizeSourceForPayload({ ...s, position: idx })),
      target: 'mihomo',
    }
    const res = await api.post<GroupDraftPreviewResponse>('/api/v1/policies/groups/preview', payload)
    if (currentGen !== previewGeneration.value || !props.open || hasInvalidFilterSources.value) return
    snapshotPreview.value = res
    previewDiagnostics.value = res.diagnostics || []
  } catch (err: any) {
    if (currentGen !== previewGeneration.value || !props.open || hasInvalidFilterSources.value) return
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
    if (currentGen === previewGeneration.value) {
      previewLoading.value = false
    }
  }
}

function triggerDebouncedPreview(delayMs = 250) {
  clearPreviewTimer()
  if (hasInvalidFilterSources.value) {
    previewGeneration.value++
    snapshotPreview.value = null
    previewDiagnostics.value = []
    previewLoading.value = false
    return
  }
  const gen = ++previewGeneration.value
  previewDebounceTimer = setTimeout(() => {
    if (gen === previewGeneration.value && !hasInvalidFilterSources.value) {
      requestLivePreview(gen)
    }
  }, delayMs)
}

async function loadSnapshotPreview() {
  clearPreviewTimer()
  if (hasInvalidFilterSources.value) {
    previewGeneration.value++
    snapshotPreview.value = null
    previewDiagnostics.value = []
    return
  }
  await requestLivePreview()
}

watch(hasInvalidFilterSources, (invalid) => {
  if (invalid) {
    clearPreviewTimer()
    previewGeneration.value++
    snapshotPreview.value = null
    previewDiagnostics.value = []
    previewLoading.value = false
  } else if (props.open) {
    triggerDebouncedPreview(250)
  }
})

// Watch draft changes for debounced preview
watch(
  [
    () => name.value,
    () => groupType.value,
    () => emptyFallbackPass.value,
    () => inheritGlobalFilter.value,
    () => sources.value,
  ],
  () => {
    if (props.open) {
      triggerDebouncedPreview(250)
    }
  },
  { deep: true }
)

onUnmounted(() => {
  clearPreviewTimer()
  previewGeneration.value++
})

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
    sources: sources.value.map((s, idx) => sanitizeSourceForPayload({ ...s, position: idx })),
    expectedRevision: expectedRevision.value,
  })
}

function close() {
  clearPreviewTimer()
  previewGeneration.value++
  snapshotPreview.value = null
  previewDiagnostics.value = []
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
            <!-- Top Compact Config: Name, Type (select), Position, and Candidate Scope (Inherit) -->
            <div data-testid="stage-candidate-scope" class="p-3.5 rounded-xl bg-base-200/80 border border-base-300 space-y-3">
              <!-- Row 1: Name, Type (<select>), Position -->
              <div class="grid grid-cols-1 sm:grid-cols-12 gap-3">
                <label class="form-control sm:col-span-6">
                  <span class="label-text font-semibold text-xs">策略组名称</span>
                  <input
                    v-model="name"
                    required
                    placeholder="例如：代理节点、自动优选、流媒体解锁"
                    class="input input-bordered input-sm mt-1"
                  />
                </label>

                <label class="form-control sm:col-span-4">
                  <span class="label-text font-semibold text-xs">策略组类型</span>
                  <select
                    v-model="groupType"
                    class="select select-bordered select-sm mt-1 w-full text-xs font-medium"
                    data-testid="group-type-select"
                  >
                    <option v-for="item in ALL_GROUP_TYPES" :key="item.type" :value="item.type">
                      {{ item.label }} ({{ item.type }})
                    </option>
                  </select>
                </label>

                <label class="form-control sm:col-span-2">
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

              <!-- Row 2: Candidate Scope (Inherit vs All Candidates) - Unique Control at Top -->
              <div class="space-y-1.5 pt-2 border-t border-base-300/50">
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <span class="label-text font-semibold text-xs">候选范围与软筛选继承</span>
                  <div class="flex items-center gap-3 text-xs">
                    <label
                      data-testid="scope-inherit"
                      class="flex items-center gap-1.5 cursor-pointer"
                    >
                      <input
                        v-model="inheritGlobalFilter"
                        type="radio"
                        :value="true"
                        class="radio radio-primary radio-xs"
                      />
                      <span class="font-medium">继承全局筛选 (Inherit)</span>
                    </label>

                    <label
                      data-testid="scope-all"
                      class="flex items-center gap-1.5 cursor-pointer"
                    >
                      <input
                        v-model="inheritGlobalFilter"
                        type="radio"
                        :value="false"
                        class="radio radio-primary radio-xs"
                      />
                      <span class="font-medium">全部候选节点 (不应用全局筛选)</span>
                    </label>
                  </div>
                </div>
                <p class="text-[11px] opacity-70 leading-snug">
                  {{ inheritGlobalFilter ? '仅匹配符合全局过滤条件的节点（默认推荐）。' : '关闭继承软筛选，可匹配符合启用状态与风险限制的全部候选节点（用于 WARP 或专用免检线路）。' }}
                  说明：选择「全部候选节点(不应用全局筛选)」只绕过全局软筛选，绝不绕过节点停用、来源订阅禁用、显式风险排除或凭据/当前连接版本校验。
                </p>
              </div>
            </div>

            <!-- Primary Inline Filter Sources Section (Top of Sheet) -->
            <div data-testid="primary-filter-section" class="p-3.5 rounded-xl bg-base-200/80 border border-base-300 space-y-3">
              <div class="flex items-center justify-between">
                <div>
                  <h4 class="font-bold text-xs text-primary">属性筛选条件 (Filter Conditions)</h4>
                  <p class="text-[11px] opacity-70 mt-0.5">
                    直接配置策略组节点属性过滤规则；修改即时触发 250ms 快照预览，支持一键保存。
                  </p>
                </div>
                <button
                  type="button"
                  class="btn btn-outline btn-xs gap-1"
                  data-testid="add-filter-source-btn"
                  @click="addFilterSource('add')"
                >
                  <PlusIcon class="w-3.5 h-3.5" />
                  添加属性筛选
                </button>
              </div>

              <div v-if="filterSources.length > 0" class="space-y-3">
                <div
                  v-for="item in filterSources"
                  :key="getSourceKey(item.source)"
                  data-testid="inline-filter-editor"
                  class="p-2.5 rounded-lg bg-base-100 border border-base-300 space-y-2 font-sans"
                >
                  <div class="flex items-center justify-between text-xs pb-1 border-b border-base-300/40">
                    <div class="flex items-center gap-2">
                      <span
                        class="badge badge-xs font-mono font-bold"
                        :class="item.source.operation === 'remove' ? 'badge-error' : 'badge-primary'"
                      >
                        {{ item.source.operation === 'remove' ? '- 排除筛选' : '+ 加入筛选' }}
                      </span>
                      <span class="text-[11px] opacity-70 font-mono">流水线位置 #{{ item.originalIndex + 1 }}</span>
                    </div>
                    <button
                      type="button"
                      class="btn btn-ghost btn-xs text-error p-1"
                      title="删除此筛选来源"
                      @click="removeSourceItem(item.originalIndex)"
                    >
                      <TrashIcon class="w-3.5 h-3.5" />
                    </button>
                  </div>

                  <StructuredFilterEditor
                    inline
                    v-model="ensureFilterSpec(item.source).conditions"
                    @invalid="(inv) => setFilterSourceInvalid(getSourceKey(item.source), inv)"
                  />
                </div>
              </div>
              <div v-else class="p-3 rounded-lg bg-base-100/60 border border-dashed border-base-300 text-xs text-base-content/60 flex items-center justify-between">
                <span>当前暂无属性筛选条件。空条件默认匹配全部候选节点。</span>
                <button
                  type="button"
                  class="btn btn-primary btn-xs gap-1"
                  @click="addFilterSource('add')"
                >
                  <PlusIcon class="w-3.5 h-3.5" />
                  添加筛选规则
                </button>
              </div>
            </div>

            <!-- Advanced Configuration & Execution Pipeline (Default Closed) -->
            <details
              data-testid="advanced-pipeline-details"
              class="rounded-xl border border-base-300 bg-base-200/60 overflow-hidden"
            >
              <summary class="p-3.5 font-bold text-xs cursor-pointer flex items-center justify-between hover:bg-base-200/90 transition-colors select-none">
                <div class="flex items-center gap-2">
                  <span class="text-primary font-bold">高级配置与完整执行流水线 (Advanced Pipeline)</span>
                  <span class="badge badge-xs badge-info font-mono">{{ sources.length }} 项</span>
                </div>
                <span class="text-[11px] font-normal opacity-60">展开查看/调整具体节点手选、正则、引用与执行原序 &darr;</span>
              </summary>

              <div class="p-3.5 pt-2 space-y-4 border-t border-base-300/60">
                <!-- Advanced Candidate Scope Setting: Empty fallback pass -->
                <div class="p-2.5 rounded-lg bg-base-100 border border-base-300 text-xs space-y-1">
                  <span class="font-semibold block text-xs">高级回退策略</span>
                  <label class="flex items-start gap-3 cursor-pointer">
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

            <!-- STAGE 2: Add Members (Source Add Controls) -->
            <div data-testid="stage-add-members" class="p-3.5 rounded-xl bg-base-200/80 border border-base-300 space-y-3">
              <div class="flex items-center justify-between">
                <div>
                  <h4 class="font-bold text-xs text-primary">阶段 2：添加成员 (Add Members)</h4>
                  <p class="text-[11px] opacity-60 mt-0.5">
                    选择成员类型与操作（加入/排除），配置后追加或插入到策略组流水线。
                  </p>
                </div>
              </div>
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

                <!-- Filter source input: Add direct filter source -->
                <div v-else-if="selectedSourceKind === 'filter'" data-testid="condition-draft" class="space-y-2 p-2.5 rounded-lg bg-base-100 border border-base-300 text-xs">
                  <div class="flex items-center justify-between gap-2">
                    <span class="font-semibold">属性筛选来源</span>
                    <button
                      type="button"
                      class="btn btn-primary btn-sm gap-1"
                      @click="addSourceItem"
                    >
                      <PlusIcon class="w-4 h-4" />
                      {{ newSourceOp === 'remove' ? '添加排除过滤来源 (-)' : '添加加入过滤来源 (+)' }}
                    </button>
                  </div>
                  <p class="text-[11px] opacity-70">
                    点击添加后将在下方流水线中生成过滤来源项，可直接在行内配置匹配条件（AND 关系）。空条件默认匹配全部候选节点。
                  </p>
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

            <!-- STAGE 3: Exclude Members & Pipeline -->
            <div data-testid="stage-exclude-members" class="p-3.5 rounded-xl bg-base-200/80 border border-base-300 space-y-3">
              <div class="flex items-center justify-between">
                <div>
                  <h4 class="font-bold text-xs text-primary">阶段 3：排除成员与执行流水线 (Exclude Members & Pipeline)</h4>
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
                  :key="getSourceKey(item.source)"
                  data-testid="source-row"
                  draggable="true"
                  class="flex flex-col gap-2 p-2 rounded-lg bg-base-100 border border-base-300 text-xs font-mono hover:border-primary/40"
                  :class="{ 'opacity-50 border-dashed': draggedIndex === idx }"
                  @dragstart="onDragStart(idx, $event)"
                  @dragover="onDragOver(idx, $event)"
                  @drop="onDrop(idx, $event)"
                >
                  <div class="flex flex-wrap sm:flex-nowrap items-center justify-between gap-2 w-full">
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

                  <!-- Filter Row Summary: Avoid double editor! -->
                  <div
                    v-if="item.source.kind === 'filter'"
                    class="w-full pt-1.5 border-t border-base-300/40 text-[11px] text-base-content/70 font-sans flex items-center justify-between"
                    data-testid="filter-row-summary"
                  >
                    <span>
                      条件摘要：{{ item.source.filter_spec?.conditions?.map(c => `${c.field} ${c.op} "${c.value}"`).join(' && ') || '空条件 (匹配全部候选节点)' }}
                    </span>
                    <span class="badge badge-xs badge-ghost">已在上方筛选区编辑</span>
                  </div>
                </div>
              </div>
              <p v-else class="text-xs opacity-50 italic">
                尚未添加来源项（空策略组）。
              </p>
            </div>
          </div>
        </details>

            <!-- STAGE 4: Real-time Results & Preview -->
            <div data-testid="stage-realtime-results" class="p-3.5 rounded-xl bg-base-200/80 border border-base-300 space-y-3">
              <div class="flex items-center justify-between">
                <div>
                  <h4 class="font-bold text-xs text-primary">阶段 4：实时结果与解析预览 (Real-time Results)</h4>
                  <p class="text-[11px] opacity-70 mt-0.5">
                    基于未保存草稿的实时解析统计与成员清单；草稿变动自动触发防抖解析。
                  </p>
                </div>
                <button
                  type="button"
                  class="btn btn-ghost btn-xs gap-1"
                  :class="{ loading: previewLoading }"
                  :disabled="hasInvalidFilterSources"
                  @click="loadSnapshotPreview"
                >
                  <ArrowPathIcon class="w-3.5 h-3.5" />
                  刷新计算
                </button>
              </div>

              <!-- Metrics cards -->
              <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs">
                <div data-testid="candidate-count" class="p-2.5 rounded-lg bg-base-100 border border-base-300">
                  <span class="text-[10px] opacity-70 block">候选节点数</span>
                  <div class="text-base font-bold font-mono text-primary mt-0.5">
                    {{ snapshotPreview ? (snapshotPreview.candidate_count ?? ((snapshotPreview.node_count ?? 0) + (snapshotPreview.excluded_count ?? 0))) : '-' }}
                  </div>
                </div>

                <div data-testid="member-count" class="p-2.5 rounded-lg bg-base-100 border border-base-300">
                  <span class="text-[10px] opacity-70 block">生效成员数</span>
                  <div class="text-base font-bold font-mono text-success mt-0.5">
                    {{ snapshotPreview ? (snapshotPreview.member_count ?? snapshotPreview.members?.length ?? snapshotPreview.node_count ?? 0) : '-' }}
                  </div>
                  <span v-if="snapshotPreview && snapshotPreview.node_count !== undefined" class="text-[10px] opacity-60 block mt-0.5 font-mono">
                    {{ snapshotPreview.node_count }} 节点
                  </span>
                </div>

                <div data-testid="excluded-count" class="p-2.5 rounded-lg bg-base-100 border border-base-300">
                  <span class="text-[10px] opacity-70 block">排除节点数</span>
                  <div class="text-base font-bold font-mono text-warning mt-0.5">
                    {{ snapshotPreview ? (snapshotPreview.excluded_count !== undefined ? snapshotPreview.excluded_count : Math.max(0, (snapshotPreview.candidate_count ?? 0) - (snapshotPreview.node_count ?? 0))) : '-' }}
                  </div>
                </div>

                <div class="p-2.5 rounded-lg bg-base-100 border border-base-300">
                  <span class="text-[10px] opacity-70 block">来源流水线</span>
                  <div class="text-base font-bold font-mono text-info mt-0.5">
                    {{ sources.length }} 项
                  </div>
                </div>
              </div>

              <!-- Exclusion Summary -->
              <div data-testid="exclusion-summary" class="p-2.5 rounded-lg bg-base-100 border border-base-300 text-xs space-y-1.5">
                <div class="flex items-center justify-between">
                  <span class="text-[10px] uppercase font-semibold opacity-70 block">排除原因摘要 (Exclusion Summary)</span>
                  <span class="text-[10px] opacity-50 font-mono">流水线事件数</span>
                </div>
                <div v-if="snapshotPreview && snapshotPreview.exclusion_summary && Object.keys(snapshotPreview.exclusion_summary).length > 0" class="flex flex-wrap gap-1.5">
                  <span
                    v-for="(count, reason) in snapshotPreview.exclusion_summary"
                    :key="reason"
                    class="badge badge-sm badge-outline gap-1 font-mono text-[11px]"
                  >
                    <span class="text-warning font-semibold">{{ reason }}:</span>
                    <span>{{ count }}</span>
                  </span>
                </div>
                <div v-else-if="snapshotPreview && snapshotPreview.excluded_reasons && snapshotPreview.excluded_reasons.length > 0" class="flex flex-wrap gap-1.5">
                  <span
                    v-for="(reason, rIdx) in snapshotPreview.excluded_reasons"
                    :key="rIdx"
                    class="badge badge-sm badge-outline font-mono text-[11px]"
                  >
                    {{ reason }}
                  </span>
                </div>
                <p v-else class="text-[11px] opacity-60 italic">
                  <template v-if="!snapshotPreview">等待草稿解析中…</template>
                  <template v-else-if="(snapshotPreview.excluded_count ?? 0) > 0">已排除 {{ snapshotPreview.excluded_count }} 个节点（未提供细因明细）</template>
                  <template v-else>当前草稿无排除节点</template>
                </p>
              </div>

              <!-- Diagnostics & Alerts -->
              <div v-if="previewError" class="alert alert-error text-xs p-3">
                <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
                <span>{{ previewError }}</span>
              </div>

              <div v-if="previewDiagnostics.length > 0" class="space-y-1">
                <div
                  v-for="(diag, dIdx) in previewDiagnostics"
                  :key="dIdx"
                  class="alert alert-warning text-xs p-2 font-mono"
                >
                  {{ typeof diag === 'string' ? diag : diag.message }}
                </div>
              </div>

              <!-- Resolved Members Snippet -->
              <div v-if="snapshotPreview?.members && snapshotPreview.members.length > 0" class="space-y-1 p-2 rounded-lg bg-base-100 border border-base-300">
                <div class="flex items-center justify-between text-xs font-semibold">
                  <span>实时解析成员 (前 {{ Math.min(snapshotPreview.members.length, 10) }} 项 / 共 {{ snapshotPreview.members.length }} 项)</span>
                  <button type="button" class="btn btn-ghost btn-xs text-primary" @click="activeSheetTab = 'preview'">查看完整快照 &rarr;</button>
                </div>
                <div class="flex flex-wrap gap-1 pt-1 max-h-24 overflow-y-auto">
                  <span
                    v-for="(m, mIdx) in snapshotPreview.members.slice(0, 10)"
                    :key="mIdx"
                    class="badge badge-xs badge-neutral font-mono truncate max-w-[200px]"
                  >
                    {{ m.display_name || m.node_logical_id || m.group_id || m.builtin_name }}
                  </span>
                </div>
              </div>
            </div>
          </form>
        </div>

        <!-- TAB 2: Snapshot Preview -->
        <div v-else-if="activeSheetTab === 'preview'" class="space-y-4">
          <div class="p-3 rounded-xl bg-base-200/70 border border-base-300 text-xs space-y-2">
            <div class="flex flex-wrap items-center justify-between gap-2.5">
              <div class="flex flex-wrap items-center gap-1.5 sm:gap-2 min-w-0">
                <span class="badge badge-warning badge-sm font-semibold shrink-0">草稿预览 (Draft Preview)</span>
                <span class="font-bold text-xs shrink-0">实时未保存草稿解析</span>
                <span v-if="snapshotPreview" class="badge badge-xs badge-ghost font-mono shrink-0">
                  v{{ snapshotPreview.revision ?? expectedRevision }}
                </span>
                <span v-if="snapshotPreview" class="badge badge-xs badge-outline font-mono shrink-0">
                  共 {{ snapshotPreview.node_count }} 节点
                </span>
              </div>
              <button
                type="button"
                class="btn btn-ghost btn-xs gap-1 shrink-0 ml-auto sm:ml-0"
                :class="{ loading: previewLoading }"
                :disabled="hasInvalidFilterSources"
                @click="loadSnapshotPreview"
              >
                <ArrowPathIcon class="w-3.5 h-3.5 shrink-0" />
                <span>重新计算草稿预览</span>
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
              {{ typeof diag === 'string' ? diag : diag.message }}
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
            :disabled="saving || !name.trim() || hasInvalidFilterSources"
          >
            {{ mode === 'edges' ? '保存来源' : (group ? '保存策略组' : '创建策略组') }}
          </button>
        </div>
      </footer>
    </section>
  </Transition>
</template>
