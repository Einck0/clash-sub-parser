<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  XMarkIcon,
  PlusIcon,
  TrashIcon,
  MagnifyingGlassIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
} from '@heroicons/vue/24/outline'
import type { FilterCondition, FilterField, FilterOp, GroupEdge, GroupType, NodeFilterSpec, PolicyGroup } from './policyTypes'
import { ALL_GROUP_TYPES, SUPPORTED_FILTER_FIELDS, groupTypeLabel, validateConditionInput, validateEdgeInput } from './policyTypes'
import { api } from '../../api/client'
import { useBodyScrollLock } from '../../composables/useBodyScrollLock'
import { t } from '../../locales'

interface Props {
  open: boolean
  group: PolicyGroup | null
  allGroups: PolicyGroup[]
  mode: 'group' | 'edges'
  saving?: boolean
  availableNodes?: Array<{
    logical_id?: string
    logicalId?: string
    display_name?: string
    displayName?: string
    active?: boolean
    protocol?: string
  }>
}

const props = withDefaults(defineProps<Props>(), {
  saving: false,
  availableNodes: () => [],
})

// Engage mobile body scroll lock when sheet is open
useBodyScrollLock(computed(() => props.open))

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'save-group', data: { name: string; type: GroupType; nodeFilter?: NodeFilterSpec | null; emptyFallbackPass: boolean }): void
  (e: 'save-edges', data: { groupId: string; edges: GroupEdge[] }): void
}>()

const emptyFallbackPass = ref(false)
const name = ref('')
const groupType = ref<GroupType>('select')
const edges = ref<GroupEdge[]>([])
const filterConditions = ref<FilterCondition[]>([])

// New condition form state
const newField = ref<FilterField>('display_name')
const newOp = ref<FilterOp>('contains')
const newValue = ref('')
const newProbeKind = ref<'baseline' | 'geo' | 'streaming' | 'ai' | 'speed' | 'ip_risk'>('baseline')
const newFreshnessSeconds = ref<number | undefined>(undefined)
const conditionError = ref('')

const newEdgeType = ref<'group' | 'node'>('group')
const newEdgeTarget = ref('')
const edgeError = ref('')

// Debounced asynchronous search and paginated candidate nodes
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

// Membership Resolution Semantics Indicator
const hasEdges = computed(() => edges.value.length > 0)
const hasChildGroupEdges = computed(() => edges.value.some((e) => Boolean(e.child_group_id)))
const hasFilters = computed(() => filterConditions.value.length > 0)

const membershipMode = computed(() => {
  if (hasEdges.value) {
    if (hasChildGroupEdges.value) {
      return {
        key: 'cascade',
        label: '子策略组级联模式',
        tone: 'badge-secondary',
        desc: '包含子策略组连接边：父级策略组的筛选条件将向下级子策略组递归继承。',
        secondaryNotice: hasFilters.value
          ? '注意：组筛选条件仅作为入选边之二次过滤，不会从全局活跃池引入额外节点。'
          : undefined,
      }
    }
    return {
      key: 'explicit',
      label: '显式连接边模式',
      tone: 'badge-accent',
      desc: '包含显式指定的目标连接边：策略组仅在此候选边集合中评估。',
      secondaryNotice: hasFilters.value
        ? '注意：组筛选条件仅作为入选边之二次过滤，不会从全局活跃池引入额外节点。'
        : undefined,
    }
  }
  if (hasFilters.value) {
    return {
      key: 'dynamic',
      label: '全池动态匹配模式',
      tone: 'badge-info',
      desc: '未配置显式连接边：策略组将自动遍历全局活跃节点池，动态匹配所有符合筛选条件的节点。',
      secondaryNotice: undefined,
    }
  }
  return {
    key: 'empty',
    label: '未配置成员（空策略组）',
    tone: 'badge-ghost',
    desc: '尚未配置显式连接边或筛选条件。',
    secondaryNotice: undefined,
  }
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

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      if (props.group) {
        name.value = props.group.name
        emptyFallbackPass.value = props.group.empty_fallback_pass === true
        groupType.value = props.group.group_type
        edges.value = (props.group.edges || []).map((e) => ({ ...e }))
        filterConditions.value = (props.group.node_filter?.conditions || []).map((c) => ({ ...c }))
      } else {
        name.value = ''
        emptyFallbackPass.value = false
        groupType.value = 'select'
        edges.value = []
        filterConditions.value = []
      }
      newEdgeTarget.value = ''
      edgeError.value = ''
      conditionError.value = ''
      nodeSearchQuery.value = ''
      nodePage.value = 1
      fetchCandidateNodes(1, '')
    }
  },
  { immediate: true }
)

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
    if (newFreshnessSeconds.value !== undefined && newFreshnessSeconds.value > 0) {
      cond.freshness_seconds = Number(newFreshnessSeconds.value)
    }
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
  newFreshnessSeconds.value = cond.freshness_seconds
  filterConditions.value.splice(idx, 1)
}

function removeFilterCondition(idx: number) {
  filterConditions.value.splice(idx, 1)
}

function handleSaveGroup() {
  const hasConditions = filterConditions.value.length > 0
  const hadFilter = props.group?.node_filter !== undefined && props.group?.node_filter !== null
  let nodeFilter: NodeFilterSpec | null | undefined = undefined

  if (hasConditions) {
    nodeFilter = { conditions: filterConditions.value }
  } else if (hadFilter) {
    nodeFilter = null
  }

  emit('save-group', { name: name.value, type: groupType.value, nodeFilter, emptyFallbackPass: emptyFallbackPass.value })
}

function handleSaveEdges() {
  if (!props.group) return
  emit('save-edges', { groupId: props.group.id, edges: edges.value })
}

function addEdge() {
  edgeError.value = ''
  const target = newEdgeTarget.value.trim()
  if (!target) {
    edgeError.value = '目标标识不能为空'
    return
  }

  const nextPos = edges.value.length
  const newEdge: GroupEdge = {
    position: nextPos,
  }

  if (newEdgeType.value === 'group') {
    newEdge.child_group_id = target
  } else {
    newEdge.node_logical_id = target
  }

  const parentId = props.group?.id || ''
  const validation = validateEdgeInput(newEdge, parentId)
  if (validation) {
    edgeError.value = validation
    return
  }

  edges.value.push(newEdge)
  newEdgeTarget.value = ''
}

function removeEdge(index: number) {
  edges.value.splice(index, 1)
  edges.value.forEach((e, idx) => {
    e.position = idx
  })
}

const availableChildGroups = computed(() => {
  if (!props.group) return props.allGroups
  return props.allGroups.filter((g) => g.id !== props.group?.id)
})

function close() {
  emit('close')
}
</script>

<template>
  <!-- 40% Soft Backdrop Blur Overlay -->
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

  <!-- Bottom Sheet / Modal -->
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
      class="fixed bottom-0 left-0 right-0 z-50 adaptive-surface-sheet md:max-h-[85vh] md:bottom-auto md:top-1/2 md:left-1/2 md:-translate-x-1/2 md:-translate-y-1/2 md:w-full md:max-w-xl bg-base-100 rounded-t-2xl md:rounded-2xl border-t md:border border-base-300 shadow-2xl flex flex-col overflow-hidden"
    >
      <!-- Grab Handle for Mobile Touch -->
      <div class="md:hidden pt-3 pb-1 flex justify-center flex-shrink-0 cursor-grab">
        <div class="w-12 h-1.5 rounded-full bg-base-content/20" />
      </div>

      <!-- Header -->
      <header class="flex items-start justify-between gap-3 p-4 sm:p-5 border-b border-base-300 min-h-0 overflow-y-auto">
        <div class="min-w-0 flex-1">
          <span class="text-xs font-semibold uppercase tracking-wider text-primary">策略编辑器</span>
          <h2 id="editor-title" class="mt-0.5 text-lg sm:text-xl font-bold">
            {{ mode === 'group' ? (group ? '编辑策略组' : '新建策略组') : '管理策略组连接边' }}
          </h2>
        </div>
        <button
          type="button"
          class="btn btn-ghost btn-sm btn-circle shrink-0 sticky top-0"
          aria-label="关闭"
          @click="close"
        >
          <XMarkIcon class="w-5 h-5" />
        </button>
      </header>

      <!-- Content -->
      <div class="flex-1 min-h-0 p-4 sm:p-5 overflow-y-auto overscroll-contain space-y-4">
        <!-- Group Mode Form -->
        <form v-if="mode === 'group'" id="policy-editor-group-form" class="space-y-4" @submit.prevent="handleSaveGroup">
          <!-- Membership Resolution Semantics Indicator -->
          <div data-testid="membership-mode-indicator" class="p-3 rounded-xl bg-base-200/80 border border-base-300 space-y-1 text-xs">
            <div class="flex items-center justify-between gap-2">
              <span class="font-bold">成员解析模式</span>
              <span class="badge badge-xs font-semibold" :class="membershipMode.tone">
                {{ membershipMode.label }}
              </span>
            </div>
            <p class="text-[11px] opacity-75 leading-relaxed">{{ membershipMode.desc }}</p>
            <p v-if="membershipMode.secondaryNotice" class="text-[11px] text-accent font-medium">
              {{ membershipMode.secondaryNotice }}
            </p>
            <p class="text-[10px] opacity-50 pt-0.5">
              代数规则守恒：不执行任何自动静默删除边或筛选条件的破坏性操作。
            </p>
          </div>

          <label class="form-control">
            <span class="label-text font-semibold text-xs">策略组名称</span>
            <input
              v-model="name"
              required
              placeholder="例如：代理节点、自动优选、流媒体解锁"
              class="input input-bordered input-sm mt-1"
            />
          </label>

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

          <label class="flex items-start gap-3 rounded-xl border border-base-300 p-3">
            <input v-model="emptyFallbackPass" type="checkbox" class="checkbox checkbox-primary checkbox-sm" data-testid="empty-fallback-pass" />
            <span class="min-w-0 text-sm">
              <span class="font-semibold block">{{ t('policy.emptyFallbackPass') }}</span>
              <span class="text-xs opacity-70">{{ t('policy.emptyFallbackPassHelp') }}</span>
            </span>
          </label>

          <!-- Group Node Filter Section -->
          <div class="p-3 rounded-xl bg-base-200/70 border border-base-300 space-y-3">
            <div class="flex items-center justify-between">
              <div>
                <span class="font-bold text-xs">策略组节点筛选条件</span>
                <p class="text-[11px] opacity-60 mt-0.5">
                  在全局筛选之后生效。留空则允许所有候选节点。若未配置显式节点连接边，该策略组将动态从全局候选节点中筛选匹配项。
                </p>
                <p class="text-[10px] opacity-50 mt-0.5">
                  探针条件要求观测记录与当前节点凭据版本一致；过期或未验证的观测将按安全闭合（Fail-Closed）原则排除。
                </p>
              </div>
              <span class="badge badge-sm badge-ghost font-mono">
                {{ filterConditions.length }}/32
              </span>
            </div>

            <!-- Existing Conditions list -->
            <div v-if="filterConditions.length > 0" class="space-y-1.5">
              <div
                v-for="(cond, cIdx) in filterConditions"
                :key="cIdx"
                class="flex items-center justify-between p-2 rounded-lg bg-base-100 border border-base-300 text-xs font-mono"
              >
                <div class="flex items-center gap-1.5 truncate min-w-0">
                  <span class="badge badge-xs badge-outline">{{ cond.field }}</span>
                  <span class="text-primary font-semibold">{{ cond.op }}</span>
                  <span class="truncate">"{{ cond.value }}"</span>
                  <span v-if="cond.probe_kind" class="badge badge-xs badge-ghost">
                    {{ cond.probe_kind }}
                  </span>
                  <span v-if="cond.freshness_seconds" class="opacity-60 text-[10px]">
                    ≤{{ cond.freshness_seconds }}s
                  </span>
                </div>
                <button type="button" class="btn btn-ghost btn-xs" @click="editFilterCondition(cIdx)">编辑</button>
                <button
                  type="button"
                  class="btn btn-ghost btn-xs text-error p-1"
                  @click="removeFilterCondition(cIdx)"
                >
                  <TrashIcon class="w-3.5 h-3.5" />
                </button>
              </div>
            </div>
            <p v-else class="text-xs opacity-50 italic">
              未设置筛选条件，所有通过全局筛选的节点均允许进入。
            </p>

            <!-- Add Condition Inline Control -->
            <div class="space-y-2 pt-2 border-t border-base-300/60">
              <div class="grid grid-cols-1 sm:grid-cols-3 gap-2">
                <select
                  v-model="newField"
                  class="select select-bordered select-xs"
                >
                  <option
                    v-for="f in SUPPORTED_FILTER_FIELDS"
                    :key="f.field"
                    :value="f.field"
                  >
                    {{ f.label }}
                  </option>
                </select>

                <select
                  v-model="newOp"
                  class="select select-bordered select-xs font-mono"
                >
                  <template v-if="newField === 'display_name' || newField === 'source_subscription_ids'">
                    <option value="contains">包含 (contains)</option>
                    <option value="not_contains">不包含 (not_contains)</option>
                    <option v-if="newField === 'display_name'" value="regex">匹配正则 (regex)</option>
                    <option v-if="newField === 'display_name'" value="not_regex">不匹配正则 (not_regex)</option>
                  </template>
                  <template v-else-if="newField === 'probe_latency_ms'">
                    <option value="lte">小于等于 (lte)</option>
                  </template>
                  <template v-else>
                    <option value="equals">等于 (equals)</option>
                    <option value="not_equals">不等于 (not_equals)</option>
                  </template>
                </select>

                <select
                  v-if="newField === 'probe_verdict'"
                  v-model="newValue"
                  class="select select-bordered select-xs"
                >
                  <option value="available">可用 (available)</option>
                  <option value="restricted">受限 (restricted)</option>
                  <option value="unknown">未知 (unknown)</option>
                  <option value="error">错误 (error)</option>
                  <option value="stale">已过期 (stale)</option>
                </select>
                <input
                  v-else
                  v-model="newValue"
                  class="input input-bordered input-xs"
                  placeholder="匹配目标值..."
                />
              </div>

              <!-- Extra fields for probe kinds -->
              <div
                v-if="newField === 'probe_verdict' || newField === 'probe_latency_ms'"
                class="grid grid-cols-2 gap-2"
              >
                <select
                  v-model="newProbeKind"
                  class="select select-bordered select-xs"
                >
                  <option value="baseline">基础连通性 (baseline)</option>
                  <option value="geo">地域与出口 IP (geo)</option>
                  <option value="streaming">流媒体解锁 (streaming)</option>
                  <option value="ai">AI 服务可用性 (ai)</option>
                  <option value="speed">带宽测速 (speed)</option>
                  <option value="ip_risk">IP 风险度 (ip_risk)</option>
                </select>

                <input
                  v-model.number="newFreshnessSeconds"
                  type="number"
                  placeholder="最大有效期（秒，可选）"
                  class="input input-bordered input-xs font-mono"
                />
              </div>

              <div class="flex items-center justify-between pt-1">
                <span v-if="conditionError" class="text-error text-xs">
                  {{ conditionError }}
                </span>
                <span v-else />
                <button
                  type="button"
                  class="btn btn-outline btn-xs gap-1 ml-auto"
                  @click="addFilterCondition"
                >
                  <PlusIcon class="w-3.5 h-3.5" />
                  添加条件
                </button>
              </div>
            </div>
          </div>


        </form>

        <!-- Edges Mode Form -->
        <div v-else class="space-y-4">
          <!-- Membership Resolution Semantics Indicator in Edges Mode -->
          <div data-testid="membership-mode-indicator" class="p-3 rounded-xl bg-base-200/80 border border-base-300 space-y-1 text-xs">
            <div class="flex items-center justify-between gap-2">
              <span class="font-bold">成员解析模式</span>
              <span class="badge badge-xs font-semibold" :class="membershipMode.tone">
                {{ membershipMode.label }}
              </span>
            </div>
            <p class="text-[11px] opacity-75 leading-relaxed">{{ membershipMode.desc }}</p>
            <p v-if="membershipMode.secondaryNotice" class="text-[11px] text-accent font-medium">
              {{ membershipMode.secondaryNotice }}
            </p>
            <p class="text-[10px] opacity-50 pt-0.5">
              代数规则守恒：不执行任何自动静默删除边或筛选条件的破坏性操作。
            </p>
          </div>

          <p class="text-xs opacity-70">
            配置从 <strong>{{ group?.name }}</strong> 指向子策略组或特定节点的有向连接边。
          </p>

          <!-- Existing Edges List -->
          <div class="space-y-2 max-h-48 overflow-y-auto">
            <div
              v-for="(edge, idx) in edges"
              :key="idx"
              class="flex items-center justify-between p-2.5 rounded-lg bg-base-200 border border-base-300 text-xs font-mono"
            >
              <div class="flex items-center gap-2 min-w-0">
                <span class="badge badge-sm badge-ghost">{{ edge.position }}</span>
                <span v-if="edge.child_group_id" class="text-secondary truncate">
                  子策略组：{{ getGroupName(edge.child_group_id) }}
                </span>
                <span v-else class="text-primary truncate">
                  节点：{{ getNodeDisplayName(edge.node_logical_id) }}
                </span>
              </div>
              <button
                type="button"
                class="btn btn-ghost btn-xs text-error"
                title="移除连接边"
                @click="removeEdge(idx)"
              >
                <TrashIcon class="w-3.5 h-3.5" />
              </button>
            </div>
            <p v-if="edges.length === 0" class="text-xs opacity-50 italic text-center py-2">
              尚未挂载任何连接边，请在下方添加。
            </p>
          </div>

          <!-- Add Edge Control -->
          <div class="p-3 rounded-xl bg-base-200/60 border border-base-300 space-y-3">
            <div class="flex items-center gap-3 text-xs">
              <label class="flex items-center gap-1.5 cursor-pointer">
                <input
                  v-model="newEdgeType"
                  type="radio"
                  value="group"
                  class="radio radio-primary radio-xs"
                />
                <span>子策略组</span>
              </label>
              <label class="flex items-center gap-1.5 cursor-pointer">
                <input
                  v-model="newEdgeType"
                  type="radio"
                  value="node"
                  class="radio radio-primary radio-xs"
                />
                <span>活跃节点</span>
              </label>
            </div>

              <div v-if="newEdgeType === 'group'" class="flex gap-2">
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
                  @click="addEdge"
                >
                  <PlusIcon class="w-4 h-4" />
                  添加
                </button>
              </div>

              <!-- Node Candidate Asynchronous Search, Debounce & Pagination -->
              <div v-else class="space-y-2">
                <div class="relative">
                  <input
                    :value="nodeSearchQuery"
                    data-testid="candidate-node-search-input"
                    type="search"
                    placeholder="异步搜索候选节点（支持可检索超100/250项）..."
                    class="input input-bordered input-xs sm:input-sm w-full pl-8 font-mono text-xs"
                    @input="onNodeSearchInput"
                  />
                  <MagnifyingGlassIcon class="w-4 h-4 opacity-50 absolute left-2.5 top-1/2 -translate-y-1/2 pointer-events-none" />
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
                    @click="addEdge"
                  >
                    <PlusIcon class="w-4 h-4" />
                    添加
                  </button>
                </div>

                <!-- Candidate Pagination Controls -->
                <div class="flex items-center justify-between text-[11px] opacity-75 pt-0.5 px-0.5">
                  <span class="font-mono">
                    候选节点：第 {{ nodePage }} / {{ Math.max(1, Math.ceil(nodeTotal / nodePageSize)) }} 页
                    <span v-if="nodeTotal > 0">（池中共 {{ nodeTotal }} 项）</span>
                  </span>
                  <div class="flex items-center gap-1">
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

                <!-- Current Selection Confirmation Tag -->
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

            <p v-if="edgeError" class="text-error text-xs">
              {{ edgeError }}
            </p>
          </div>


        </div>
      </div>
      <div class="flex flex-wrap justify-end gap-2 p-4 sm:p-5 border-t border-base-300 shrink-0 bg-base-100">
        <button type="button" class="btn btn-ghost btn-sm" :disabled="saving" @click="close">取消</button>
        <button
          v-if="mode === 'group'"
          type="submit"
          form="policy-editor-group-form"
          class="btn btn-primary btn-sm"
          :class="{ loading: saving }"
          :disabled="saving || !name.trim()"
        >{{ group ? '保存策略组' : '创建策略组' }}</button>
        <button v-else type="button" class="btn btn-primary btn-sm" :class="{ loading: saving }" :disabled="saving" @click="handleSaveEdges">保存连接边</button>
      </div>
    </section>
  </Transition>
</template>
