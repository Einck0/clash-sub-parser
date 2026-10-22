<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  PlusIcon,
  TrashIcon,
  MagnifyingGlassIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  ChevronUpIcon,
  ChevronDownIcon,
  Bars3Icon,
  ExclamationTriangleIcon,
} from '@heroicons/vue/24/outline'
import type {
  BuiltinName,
  FilterCondition,
  GroupType,
  MemberSource,
  MemberSourceKind,
  PolicyGroup,
  SourceOperation,
} from './policyTypes'
import {
  ALL_GROUP_TYPES,
  VALID_BUILTIN_NAMES,
  groupTypeLabel,
  groupTypeSupportedTargets,
  sourceKindLabel,
  sourceOperationLabel,
  computeSourceSemantics,
  validateRegexPattern,
  validateSourceInput,
} from './policyTypes'
import { getSourceKey } from './useGroupDraft'
import StructuredFilterEditor from './StructuredFilterEditor.vue'
import { api } from '../../api/client'
import { uiText } from '../../locales/redesign'
import { t } from '../../locales'

const props = withDefaults(
  defineProps<{
    name: string
    groupType: GroupType
    position: number
    emptyFallbackPass: boolean
    inheritGlobalFilter: boolean
    sources: MemberSource[]
    hasInvalidFilterSources?: boolean
    formError?: string
    allGroups?: PolicyGroup[]
    availableNodes?: any[]
    groupId?: string
    mode?: 'group' | 'edges'
  }>(),
  {
    hasInvalidFilterSources: false,
    formError: '',
    allGroups: () => [],
    availableNodes: () => [],
    groupId: '',
    mode: 'group',
  }
)

const emit = defineEmits<{
  (e: 'update:name', val: string): void
  (e: 'update:groupType', val: GroupType): void
  (e: 'update:position', val: number): void
  (e: 'update:emptyFallbackPass', val: boolean): void
  (e: 'update:inheritGlobalFilter', val: boolean): void
  (e: 'add-source', source: MemberSource): void
  (e: 'add-filter', op: SourceOperation): void
  (e: 'remove-source', index: number): void
  (e: 'move-source-up', index: number): void
  (e: 'move-source-down', index: number): void
  (e: 'move-source', fromIndex: number, toIndex: number): void
  (e: 'toggle-source-op', index: number): void
  (e: 'filter-invalid', key: string, isInvalid: boolean): void
}>()

// Candidate selection form state
const selectedSourceKind = ref<'filter' | 'node' | 'group' | 'group_nodes' | 'regex' | 'typed_builtin'>('filter')
const newSourceOp = ref<SourceOperation>('add')
const newEdgeTarget = ref('')
const newSourcePattern = ref('')
const newSourceBuiltin = ref<BuiltinName>('DIRECT')
const localValidationError = ref('')

// Candidate nodes query
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

// Watch props.availableNodes
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
  const source = props.availableNodes && props.availableNodes.length > 0 ? props.availableNodes : []
  const mapped = source
    .map((n) => ({
      logicalId: ('logicalId' in n ? n.logicalId : undefined) || ('logical_id' in n ? n.logical_id : undefined) || '',
      displayName: ('displayName' in n ? n.displayName : undefined) || ('display_name' in n ? n.display_name : undefined) || '',
      protocol: n.protocol,
      active: n.active !== false,
    }))
    .filter((n) => n.logicalId.length > 0)

  mapped.forEach(registerKnownNode)
  const activeOnly = mapped.filter((n) => n.active)
  return activeOnly.length > 0 ? activeOnly : mapped
})

const availableChildGroups = computed(() => {
  return (props.allGroups || []).filter((g) => g.id !== props.groupId)
})

function getGroupName(id?: string): string {
  if (!id) return ''
  const g = props.allGroups.find((x) => x.id === id)
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

// Regex presets
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

function handleAddSourceItem() {
  localValidationError.value = ''
  let actualKind: MemberSourceKind = 'manual_node'
  if (selectedSourceKind.value === 'node') actualKind = 'manual_node'
  else if (selectedSourceKind.value === 'group') actualKind = 'group_ref'
  else actualKind = selectedSourceKind.value as MemberSourceKind

  const target = newEdgeTarget.value.trim()
  const newSource: MemberSource = {
    operation: newSourceOp.value,
    kind: actualKind,
    position: props.sources.length,
  }

  if (actualKind === 'manual_node') {
    newSource.node_logical_id = target
  } else if (actualKind === 'group_ref' || actualKind === 'group_nodes') {
    newSource.target_group_id = target
  } else if (actualKind === 'regex') {
    newSource.pattern = newSourcePattern.value.trim()
  } else if (actualKind === 'filter') {
    emit('add-filter', newSourceOp.value)
    return
  } else if (actualKind === 'typed_builtin') {
    newSource.builtin_name = newSourceBuiltin.value
  }

  const err = validateSourceInput(newSource, props.groupId)
  if (err) {
    localValidationError.value = err
    return
  }

  emit('add-source', newSource)
  if (actualKind === 'regex') {
    newSourcePattern.value = ''
  }
}

// Select all on current page
function selectAllCurrentPageNodes() {
  if (activeNodes.value.length === 0) return
  for (const node of activeNodes.value) {
    emit('add-source', {
      operation: newSourceOp.value,
      kind: 'manual_node',
      position: props.sources.length,
      node_logical_id: node.logicalId,
    })
  }
}

// Select all filtered across pages
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
      emit('add-source', {
        operation: newSourceOp.value,
        kind: 'manual_node',
        position: props.sources.length,
        node_logical_id: node.logicalId,
      })
    }
  } catch {
    // fallback
  } finally {
    if (seq === candidateQuerySeq) {
      loadingAllCandidates.value = false
    }
  }
}

// Drag & drop
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
  emit('move-source', draggedIndex.value, idx)
  draggedIndex.value = null
}

const computedSemantics = computed(() => computeSourceSemantics(props.sources))

onMounted(() => {
  fetchCandidateNodes(1, '')
})
</script>

<template>
  <div class="space-y-5 text-xs font-sans min-w-0" data-testid="source-pipeline-editor">
    <!-- Validation error alert -->
    <div
      v-if="formError || localValidationError"
      role="alert"
      class="p-2.5 rounded-lg bg-error/10 border border-error/30 text-error text-xs flex items-center gap-2"
    >
      <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
      <span>{{ formError || localValidationError }}</span>
    </div>

    <!-- Stage 1: Basic Configuration (Name, Type, Position, Toggles) -->
    <section class="space-y-3" aria-labelledby="stage-1-heading">
      <h3 id="stage-1-heading" class="font-bold text-xs uppercase tracking-wider text-primary">
        {{ uiText('阶段 1: 策略组基础配置', 'Stage 1: Basic Configuration') }}
      </h3>

      <div class="grid grid-cols-1 sm:grid-cols-12 gap-3 p-3.5 rounded-xl bg-base-200/50 border border-base-300">
        <!-- Name -->
        <label class="flex flex-col gap-1 sm:col-span-6">
          <span class="text-xs font-semibold">策略组名称</span>
          <input
            :value="name"
            type="text"
            placeholder="例如：Proxy / 节点选择"
            class="input input-sm mt-1"
            @input="emit('update:name', ($event.target as HTMLInputElement).value)"
          />
        </label>

        <!-- Group Type -->
        <label class="flex flex-col gap-1 sm:col-span-4">
          <span class="text-xs font-semibold">策略组类型</span>
          <select
            :value="groupType"
            class="select select-sm mt-1 w-full text-xs font-medium"
            data-testid="group-type-select"
            @change="emit('update:groupType', ($event.target as HTMLSelectElement).value as GroupType)"
          >
            <option v-for="item in ALL_GROUP_TYPES" :key="item.type" :value="item.type">
              {{ item.label }} ({{ item.type }})
            </option>
          </select>
        </label>

        <!-- Position -->
        <label class="flex flex-col gap-1 sm:col-span-2">
          <span class="text-xs font-semibold">排序序号</span>
          <input
            :value="position"
            type="number"
            min="0"
            class="input input-sm mt-1 font-mono"
            @input="emit('update:position', Number(($event.target as HTMLInputElement).value) || 0)"
          />
        </label>

        <!-- Toggles: inherit_global_filter & empty_fallback_pass -->
        <div class="sm:col-span-12 space-y-2 pt-2 border-t border-base-300">
          <label class="flex items-start gap-2.5 cursor-pointer">
            <input
              type="checkbox"
              class="checkbox checkbox-primary checkbox-xs mt-0.5"
              data-testid="inherit-global-filter"
              :checked="inheritGlobalFilter"
              @change="emit('update:inheritGlobalFilter', ($event.target as HTMLInputElement).checked)"
            />
            <div class="min-w-0">
              <span class="font-semibold text-xs">继承全局节点筛选</span>
              <p class="text-[11px] opacity-70 mt-0.5">
                关闭后软豁免全局软筛选，直接从全量候选节点中匹配（如 WARP 或专用线路）。
              </p>
            </div>
          </label>

          <label class="flex items-start gap-2.5 cursor-pointer">
            <input
              type="checkbox"
              class="checkbox checkbox-primary checkbox-xs mt-0.5"
              data-testid="empty-fallback-pass"
              :checked="emptyFallbackPass"
              @change="emit('update:emptyFallbackPass', ($event.target as HTMLInputElement).checked)"
            />
            <div class="min-w-0">
              <span class="font-semibold text-xs">空成员回退放行 (empty_fallback_pass)</span>
              <p class="text-[11px] opacity-70 mt-0.5">
                当筛选后无可用节点时回退放行。注意：空成员回退是放行流量 (PASS)，不是直连 (DIRECT)。
              </p>
            </div>
          </label>
        </div>
      </div>
    </section>

    <!-- Stage 2: Quick Filter Source Addition -->
    <section class="space-y-2" aria-labelledby="stage-2-heading">
      <div class="flex items-center justify-between">
        <h3 id="stage-2-heading" class="font-bold text-xs uppercase tracking-wider text-primary">
          {{ uiText('阶段 2: 属性过滤条件', 'Stage 2: Attribute Filters') }}
        </h3>
        <button
          type="button"
          class="btn btn-primary btn-xs gap-1"
          @click="emit('add-filter', 'add')"
        >
          <PlusIcon class="w-3.5 h-3.5" />
          {{ uiText('添加属性过滤来源', 'Add Filter Source') }}
        </button>
      </div>
      <p class="text-[11px] opacity-70">
        {{ uiText('属性过滤按流水线顺序执行；在下方流水线中可直接行内编辑过滤条件。', 'Filters execute in pipeline order. Edit conditions directly inline below.') }}
      </p>
    </section>

    <!-- Stage 3: Candidate Selection / Source Addition -->
    <section class="space-y-3" aria-labelledby="stage-3-heading">
      <h3 id="stage-3-heading" class="font-bold text-xs uppercase tracking-wider text-primary">
        {{ uiText('阶段 3: 添加成员来源项', 'Stage 3: Add Member Source Item') }}
      </h3>

      <div class="p-3.5 rounded-xl bg-base-200/50 border border-base-300 space-y-3">
        <!-- Kind Radio Selector -->
        <div class="flex flex-wrap items-center gap-3">
          <label class="flex items-center gap-1.5 cursor-pointer">
            <input
              v-model="selectedSourceKind"
              type="radio"
              value="filter"
              class="radio radio-primary radio-xs"
            />
            <span class="font-medium text-xs">同级属性过滤</span>
          </label>

          <label class="flex items-center gap-1.5 cursor-pointer">
            <input
              v-model="selectedSourceKind"
              type="radio"
              value="node"
              class="radio radio-primary radio-xs"
            />
            <span class="font-medium text-xs">手工节点</span>
          </label>

          <label class="flex items-center gap-1.5 cursor-pointer">
            <input
              v-model="selectedSourceKind"
              type="radio"
              value="group"
              class="radio radio-primary radio-xs"
            />
            <span class="font-medium text-xs">策略组引用</span>
          </label>

          <label class="flex items-center gap-1.5 cursor-pointer">
            <input
              v-model="selectedSourceKind"
              type="radio"
              value="group_nodes"
              class="radio radio-primary radio-xs"
            />
            <span class="font-medium text-xs">展开子组</span>
          </label>

          <label class="flex items-center gap-1.5 cursor-pointer">
            <input
              v-model="selectedSourceKind"
              type="radio"
              value="regex"
              class="radio radio-primary radio-xs"
            />
            <span class="font-medium text-xs">节点名称正则</span>
          </label>

          <label class="flex items-center gap-1.5 cursor-pointer">
            <input
              v-model="selectedSourceKind"
              type="radio"
              value="typed_builtin"
              class="radio radio-primary radio-xs"
            />
            <span class="font-medium text-xs">内建流控项</span>
          </label>
        </div>

        <!-- Operation (Add vs Remove) -->
        <div class="flex items-center gap-2">
          <span class="text-xs font-semibold">操作方式:</span>
          <select v-model="newSourceOp" class="select select-xs text-xs font-semibold">
            <option value="add">加入 (+)</option>
            <option value="remove">排除 (-)</option>
          </select>
        </div>

        <!-- Dynamic Inputs based on selectedSourceKind -->
        <!-- 1. Node selection -->
        <div v-if="selectedSourceKind === 'node'" class="space-y-2">
          <div class="flex flex-col sm:flex-row gap-2">
            <div class="relative flex-1">
              <input
                :value="nodeSearchQuery"
                type="search"
                data-testid="candidate-node-search-input"
                placeholder="搜索候选节点名称、ID 或协议..."
                class="input input-xs sm:input-sm w-full pl-7 text-xs font-mono"
                @input="onNodeSearchInput"
              />
              <MagnifyingGlassIcon class="w-3.5 h-3.5 opacity-50 absolute left-2 top-1/2 -translate-y-1/2 pointer-events-none" />
            </div>

            <div class="flex items-center gap-1 text-[11px] font-mono opacity-80 shrink-0">
              <button
                type="button"
                class="btn btn-ghost btn-xs"
                :disabled="loadingNodes || nodePage <= 1"
                @click="prevCandidatePage"
              >
                <ChevronLeftIcon class="w-3.5 h-3.5" />
              </button>
              <span>{{ nodePage }} / {{ Math.max(1, Math.ceil(nodeTotal / nodePageSize)) }}</span>
              <button
                type="button"
                class="btn btn-ghost btn-xs"
                :disabled="loadingNodes || nodePage * nodePageSize >= nodeTotal"
                @click="nextCandidatePage"
              >
                <ChevronRightIcon class="w-3.5 h-3.5" />
              </button>
            </div>
          </div>

          <div class="flex gap-2">
            <select
              v-model="newEdgeTarget"
              class="select select-sm flex-1 text-xs font-mono"
              data-testid="edge-node-select"
            >
              <option disabled value="">
                {{ loadingNodes ? '加载候选节点中...' : (activeNodes.length ? '请选择活跃节点...' : '暂无匹配候选节点') }}
              </option>
              <option
                v-if="newEdgeTarget && !activeNodes.some((n) => n.logicalId === newEdgeTarget)"
                :value="newEdgeTarget"
              >
                {{ getNodeDisplayName(newEdgeTarget) }} (跨页锁定已保留)
              </option>
              <option
                v-for="node in activeNodes"
                :key="node.logicalId"
                :value="node.logicalId"
              >
                {{ node.displayName }} ({{ node.logicalId }}) {{ node.protocol ? `[${node.protocol}]` : '' }}
              </option>
            </select>

            <button
              type="button"
              class="btn btn-primary btn-sm gap-1 shrink-0"
              data-testid="add-edge-btn"
              :disabled="!newEdgeTarget.trim()"
              @click="handleAddSourceItem"
            >
              <PlusIcon class="w-4 h-4" />
              添加节点
            </button>
          </div>

          <div class="flex flex-wrap items-center justify-between gap-2 pt-1">
            <div class="flex gap-2">
              <button
                type="button"
                class="btn btn-ghost btn-xs"
                :disabled="activeNodes.length === 0"
                @click="selectAllCurrentPageNodes"
              >
                全选当页 ({{ activeNodes.length }})
              </button>
              <button
                type="button"
                class="btn btn-ghost btn-xs"
                :disabled="loadingAllCandidates || nodeTotal === 0"
                @click="selectAllFilteredNodes"
              >
                {{ loadingAllCandidates ? '正在全选...' : `全选过滤结果 (${nodeTotal})` }}
              </button>
            </div>

            <span v-if="newEdgeTarget && !activeNodes.some((n) => n.logicalId === newEdgeTarget)" class="text-info text-[11px] font-mono">
              跨页锁定已保留
            </span>
          </div>
        </div>

        <!-- 2. Regex pattern -->
        <div v-else-if="selectedSourceKind === 'regex'" class="space-y-2">
          <div class="flex gap-2">
            <input
              v-model="newSourcePattern"
              type="text"
              placeholder="例如：.*(HK|香港|HongKong).*"
              class="input input-sm flex-1 font-mono text-xs"
              data-testid="regex-pattern-input"
            />
            <button
              type="button"
              class="btn btn-primary btn-sm gap-1 shrink-0"
              data-testid="add-edge-btn"
              :disabled="!newSourcePattern.trim()"
              @click="handleAddSourceItem"
            >
              <PlusIcon class="w-4 h-4" />
              添加正则
            </button>
          </div>

          <div class="flex flex-wrap items-center gap-1.5">
            <span class="text-[11px] opacity-70">快捷预设:</span>
            <button
              v-for="pre in COMMON_REGEX_PRESETS"
              :key="pre.label"
              type="button"
              class="btn btn-ghost btn-xs"
              @click="applyRegexPreset(pre.pattern)"
            >
              {{ pre.label }}
            </button>
          </div>
        </div>

        <!-- 3. Subgroup ref or unpack -->
        <div v-else-if="selectedSourceKind === 'group' || selectedSourceKind === 'group_nodes'" class="space-y-2">
          <div class="flex gap-2">
            <select
              v-model="newEdgeTarget"
              class="select select-sm flex-1 text-xs font-mono"
            >
              <option disabled value="">请选择目标策略组...</option>
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
              data-testid="add-edge-btn"
              :disabled="!newEdgeTarget.trim()"
              @click="handleAddSourceItem"
            >
              <PlusIcon class="w-4 h-4" />
              {{ selectedSourceKind === 'group_nodes' ? '添加解包子组' : '添加引用' }}
            </button>
          </div>
        </div>

        <!-- 4. Typed Builtin -->
        <div v-else-if="selectedSourceKind === 'typed_builtin'" class="space-y-2">
          <div class="flex gap-2">
            <select
              v-model="newSourceBuiltin"
              class="select select-sm flex-1 text-xs font-mono font-bold"
            >
              <option v-for="b in VALID_BUILTIN_NAMES" :key="b" :value="b">
                {{ b }}
              </option>
            </select>
            <button
              type="button"
              class="btn btn-primary btn-sm gap-1 shrink-0"
              data-testid="add-edge-btn"
              @click="handleAddSourceItem"
            >
              <PlusIcon class="w-4 h-4" />
              添加内建项
            </button>
          </div>
        </div>

        <!-- 5. Filter -->
        <div v-else-if="selectedSourceKind === 'filter'" class="flex items-center justify-between">
          <span class="text-xs opacity-80">点击右侧按钮添加新的属性过滤来源项，并可在流水线中行内编辑。</span>
          <button
            type="button"
            class="btn btn-primary btn-sm gap-1 shrink-0"
            data-testid="add-edge-btn"
            @click="handleAddSourceItem"
          >
            <PlusIcon class="w-4 h-4" />
            添加过滤来源
          </button>
        </div>
      </div>
    </section>

    <!-- Stage 4: Ordered Sources Pipeline -->
    <section class="space-y-3" aria-labelledby="stage-4-heading">
      <div class="flex items-center justify-between">
        <h3 id="stage-4-heading" class="font-bold text-xs uppercase tracking-wider text-primary">
          {{ uiText('阶段 4: 有序成员流水线 (Sources Pipeline)', 'Stage 4: Ordered Sources Pipeline') }}
        </h3>
        <span class="badge badge-xs badge-neutral font-mono">{{ sources.length }} 步</span>
      </div>

      <div v-if="sources.length === 0" class="p-6 rounded-xl border border-dashed border-base-300 text-center text-xs opacity-60">
        {{ uiText('当前尚无来源项。在上方添加属性过滤、手工节点或引用以配置流水线。', 'No sources configured yet. Add filters, manual nodes, or group refs above.') }}
      </div>

      <div v-else class="space-y-2">
        <div
          v-for="(item, idx) in computedSemantics"
          :key="getSourceKey(item.source)"
          draggable="true"
          data-testid="source-row"
          class="p-3 rounded-xl bg-base-200/60 border border-base-300 space-y-2 transition-all duration-150 ease-out"
          :class="{ 'opacity-50 border-primary': draggedIndex === idx }"
          @dragstart="onDragStart(idx, $event)"
          @dragover="onDragOver(idx, $event)"
          @drop="onDrop(idx, $event)"
        >
          <!-- Item Header & Reordering Controls -->
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div class="flex items-center gap-2 min-w-0 flex-1">
              <span class="cursor-grab active:cursor-grabbing text-base-content/40 hover:text-base-content" title="拖拽排序">
                <Bars3Icon class="w-4 h-4" />
              </span>

              <span class="badge badge-xs badge-ghost font-mono font-bold shrink-0">
                #{{ idx + 1 }}
              </span>

              <!-- Operation toggle button (+ / -) -->
              <button
                type="button"
                class="badge badge-sm font-bold cursor-pointer shrink-0 transition-colors"
                :class="item.source.operation === 'remove' ? 'badge-error text-error-content' : 'badge-primary text-primary-content'"
                :title="`点击切换操作类型：当前为 ${item.source.operation === 'remove' ? '排除 (-)' : '加入 (+)'}`"
                @click="emit('toggle-source-op', idx)"
              >
                {{ item.source.operation === 'remove' ? '-' : '+' }}
              </button>

              <span class="badge badge-xs badge-outline shrink-0 font-sans">
                {{ sourceKindLabel(item.source.kind) }}
              </span>

              <!-- Source Target Summary -->
              <div class="min-w-0 truncate text-xs font-mono flex-1">
                <span v-if="item.source.kind === 'manual_node'" class="text-primary truncate">
                  {{ getNodeDisplayName(item.source.node_logical_id) }}
                </span>
                <span v-else-if="item.source.kind === 'group_ref'" class="text-secondary truncate">
                  {{ getGroupName(item.source.target_group_id) }}
                </span>
                <span v-else-if="item.source.kind === 'group_nodes'" class="text-secondary truncate">
                  解包: {{ getGroupName(item.source.target_group_id) }}
                </span>
                <span v-else-if="item.source.kind === 'regex'" class="text-accent truncate">
                  /{{ item.source.pattern }}/
                </span>
                <span v-else-if="item.source.kind === 'typed_builtin'" class="text-warning truncate font-bold">
                  {{ item.source.builtin_name }}
                </span>
                <span v-else-if="item.source.kind === 'filter'" class="text-info truncate">
                  {{ item.source.filter_spec?.conditions?.length || 0 }} 条条件
                </span>
              </div>
            </div>

            <!-- Controls: Up, Down, Re-entry Badge, Delete -->
            <div class="flex items-center gap-1 shrink-0">
              <span
                v-if="item.isReEntry"
                class="badge badge-xs badge-secondary font-sans shrink-0"
                title="先前排除后重新加入"
              >
                重入入选
              </span>

              <button
                type="button"
                class="btn btn-ghost btn-xs p-1"
                title="上移"
                :disabled="idx === 0"
                @click="emit('move-source-up', idx)"
              >
                <ChevronUpIcon class="w-3.5 h-3.5" />
              </button>

              <button
                type="button"
                class="btn btn-ghost btn-xs p-1"
                title="下移"
                :disabled="idx === sources.length - 1"
                @click="emit('move-source-down', idx)"
              >
                <ChevronDownIcon class="w-3.5 h-3.5" />
              </button>

              <button
                type="button"
                class="btn btn-ghost btn-xs text-error p-1"
                title="移除来源项"
                @click="emit('remove-source', idx)"
              >
                <TrashIcon class="w-3.5 h-3.5" />
              </button>
            </div>
          </div>

          <!-- Inline Structured Filter Editor for filter sources -->
          <div
            v-if="item.source.kind === 'filter' && item.source.filter_spec"
            data-testid="inline-filter-editor"
            class="pt-2 border-t border-base-300/60"
          >
            <StructuredFilterEditor
              v-model="item.source.filter_spec.conditions"
              inline
              :compact="true"
              @invalid="(isInv) => emit('filter-invalid', getSourceKey(item.source), isInv)"
            />
          </div>

          <!-- Semantic analysis caption -->
          <div v-if="item.semanticText" class="text-[11px] text-base-content/60 font-sans">
            {{ item.semanticText }}
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
