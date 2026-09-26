<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  XMarkIcon,
  PlusIcon,
  TrashIcon,
} from '@heroicons/vue/24/outline'
import type { FilterCondition, FilterField, FilterOp, GroupEdge, GroupType, NodeFilterSpec, PolicyGroup } from './policyTypes'
import { ALL_GROUP_TYPES, SUPPORTED_FILTER_FIELDS, groupTypeLabel, validateConditionInput, validateEdgeInput } from './policyTypes'
import { api } from '../../api/client'

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

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'save-group', data: { name: string; type: GroupType; nodeFilter?: NodeFilterSpec | null }): void
  (e: 'save-edges', data: { groupId: string; edges: GroupEdge[] }): void
}>()

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
const internalNodes = ref<Array<{ logicalId: string; displayName: string; active: boolean; protocol?: string }>>([])
const loadingNodes = ref(false)

async function fetchNodesIfEmpty() {
  if (props.availableNodes && props.availableNodes.length > 0) return
  if (internalNodes.value.length > 0) return
  try {
    loadingNodes.value = true
    const result = await api.get<{ items: Array<{ logical_id: string; display_name: string; active: boolean; protocol: string }> }>('/api/v1/nodes', {
      params: { page: 1, page_size: 200 },
    })
    if (result && Array.isArray(result.items)) {
      internalNodes.value = result.items.map((n) => ({
        logicalId: n.logical_id,
        displayName: n.display_name?.trim() || n.logical_id,
        protocol: n.protocol,
        active: n.active !== false,
      }))
    }
  } catch {
    // Graceful fallback
  } finally {
    loadingNodes.value = false
  }
}

const activeNodes = computed(() => {
  const source = (props.availableNodes && props.availableNodes.length > 0)
    ? props.availableNodes
    : internalNodes.value

  const mapped = source
    .map((n) => ({
      logicalId: ('logicalId' in n ? n.logicalId : undefined) || ('logical_id' in n ? n.logical_id : undefined) || '',
      displayName: ('displayName' in n ? n.displayName : undefined) || ('display_name' in n ? n.display_name : undefined) || ('logicalId' in n ? n.logicalId : undefined) || ('logical_id' in n ? n.logical_id : undefined) || '',
      protocol: n.protocol,
      active: n.active !== false,
    }))
    .filter((n) => n.logicalId.length > 0)

  const activeOnly = mapped.filter((n) => n.active)
  return activeOnly.length > 0 ? activeOnly : mapped
})

function getGroupName(id?: string): string {
  if (!id) return ''
  const g = props.allGroups.find((x) => x.id === id)
  return g ? `${g.name} (${groupTypeLabel(g.group_type)})` : id
}

function getNodeDisplayName(logicalId?: string): string {
  if (!logicalId) return ''
  const n = activeNodes.value.find((x) => x.logicalId === logicalId)
  return n ? `${n.displayName} (${logicalId})` : logicalId
}

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      if (props.group) {
        name.value = props.group.name
        groupType.value = props.group.group_type
        edges.value = (props.group.edges || []).map((e) => ({ ...e }))
        filterConditions.value = (props.group.node_filter?.conditions || []).map((c) => ({ ...c }))
      } else {
        name.value = ''
        groupType.value = 'select'
        edges.value = []
        filterConditions.value = []
      }
      newEdgeTarget.value = ''
      edgeError.value = ''
      conditionError.value = ''
      if (props.mode === 'edges') {
        fetchNodesIfEmpty()
      }
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
})

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

  emit('save-group', { name: name.value, type: groupType.value, nodeFilter })
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
      <header class="flex items-start justify-between p-4 sm:p-5 border-b border-base-300 flex-shrink-0">
        <div>
          <span class="text-xs font-semibold uppercase tracking-wider text-primary">策略编辑器</span>
          <h2 id="editor-title" class="mt-0.5 text-lg sm:text-xl font-bold">
            {{ mode === 'group' ? (group ? '编辑策略组' : '新建策略组') : '管理策略组连接边' }}
          </h2>
        </div>
        <button
          type="button"
          class="btn btn-ghost btn-sm btn-circle"
          aria-label="关闭"
          @click="close"
        >
          <XMarkIcon class="w-5 h-5" />
        </button>
      </header>

      <!-- Content -->
      <div class="flex-1 p-4 sm:p-5 overflow-y-auto space-y-4">
        <!-- Group Mode Form -->
        <form v-if="mode === 'group'" class="space-y-4" @submit.prevent="handleSaveGroup">
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

          <div class="modal-action pt-2">
            <button type="button" class="btn btn-ghost btn-sm" :disabled="saving" @click="close">取消</button>
            <button
              type="submit"
              class="btn btn-primary btn-sm"
              :class="{ loading: saving }"
              :disabled="!name.trim() || saving"
            >
              {{ group ? '保存策略组' : '创建策略组' }}
            </button>
          </div>
        </form>

        <!-- Edges Mode Form -->
        <div v-else class="space-y-4">
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

            <div class="flex gap-2">
              <select
                v-if="newEdgeType === 'group'"
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

              <select
                v-else
                v-model="newEdgeTarget"
                class="select select-bordered select-sm flex-1 text-xs font-mono"
                data-testid="edge-node-select"
              >
                <option disabled value="">
                  {{ loadingNodes ? '加载节点中...' : (activeNodes.length ? '请选择活跃节点...' : '暂无活跃节点') }}
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
                class="btn btn-primary btn-sm gap-1"
                :disabled="!newEdgeTarget"
                @click="addEdge"
              >
                <PlusIcon class="w-4 h-4" />
                添加
              </button>
            </div>

            <p v-if="edgeError" class="text-error text-xs">
              {{ edgeError }}
            </p>
          </div>

          <div class="modal-action pt-2">
            <button type="button" class="btn btn-ghost btn-sm" :disabled="saving" @click="close">取消</button>
            <button
              type="button"
              class="btn btn-primary btn-sm"
              :class="{ loading: saving }"
              :disabled="saving"
              @click="handleSaveEdges"
            >
              保存连接边
            </button>
          </div>
        </div>
      </div>
    </section>
  </Transition>
</template>
