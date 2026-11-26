<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  ExclamationTriangleIcon,
  EyeIcon,
  PencilSquareIcon,
} from '@heroicons/vue/24/outline'
import { api } from '../../api/client'
import Drawer from '../../ui/Drawer.vue'
import type {
  GroupDraftPreviewPayload,
  GroupType,
  MemberSource,
  PolicyGroup,
} from './policyTypes'
import { useGroupDraft } from './useGroupDraft'
import { useGroupPreview } from './useGroupPreview'
import SourcePipelineEditor from './SourcePipelineEditor.vue'
import GroupPreviewPanel from './GroupPreviewPanel.vue'
import { useUnsavedChanges } from '../../composables/useUnsavedChanges'
import { uiText } from '../../locales/redesign'
import { displayName, displayText } from '../../utils/displayNames'

interface Props {
  open: boolean
  group: PolicyGroup | null
  allGroups: PolicyGroup[]
  mode?: 'group' | 'edges'
  saving?: boolean
  reloading?: boolean
  availableNodes?: Array<{
    logical_id?: string
    logicalId?: string
    display_name?: string
    displayName?: string
    active?: boolean
    protocol?: string
  }>
  conflictError?: { currentRevision: number | null; message: string } | null
  cycleError?: { cyclePath: string[]; message: string } | null
  serverError?: string | null
  topologyStatus?: 'idle' | 'loading' | 'error' | 'ready'
  topologyError?: string
  isTopologyStale?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  mode: 'group',
  saving: false,
  reloading: false,
  availableNodes: () => [],
  conflictError: null,
  cycleError: null,
  serverError: null,
  topologyStatus: 'ready',
  topologyError: '',
  isTopologyStale: false,
})

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'save-group', data: {
    name: string
    type: GroupType
    position?: number
    emptyFallbackPass: boolean
    inheritGlobalFilter: boolean
    urltest_interval?: number
    urltest_tolerance?: number
    sources: MemberSource[]
    expectedRevision?: number
  }): void
  (e: 'reload-group', id: string): void
  (e: 'clear-conflict'): void
  (e: 'retry-topology'): void
}>()

// Draft and Preview composables
const draft = useGroupDraft()
const preview = useGroupPreview()
const rebasing = ref(false)
const busy = computed(() => props.saving || props.reloading || rebasing.value)
const unsaved = computed(() => props.open && draft.isDirty.value)

// Guard against unsaved changes on close or navigation
const { confirmDiscard } = useUnsavedChanges(
  unsaved,
  () => uiText('当前草稿尚未保存，确定要放弃修改吗？', 'Current group draft has unsaved changes. Discard?')
)

// Active mobile tab: 'editor' | 'preview'
const activeSheetTab = ref<'editor' | 'preview'>('editor')

// Sync props errors into draft
watch(
  () => props.conflictError,
  (c) => {
    draft.conflictError.value = c ?? null
  },
  { immediate: true }
)
watch(
  () => props.cycleError,
  (c) => {
    draft.cycleError.value = c ?? null
  },
  { immediate: true }
)
watch(
  () => props.serverError,
  (s) => {
    draft.serverError.value = s ?? null
  },
  { immediate: true }
)
watch(
  () => props.open,
  (isOpen) => {
    if (isOpen && (props.topologyStatus === 'idle' || props.topologyStatus === 'error')) {
      emit('retry-topology')
    }
  },
  { immediate: true }
)

function getPreviewPayload(): GroupDraftPreviewPayload {
  return {
    group_id: props.group?.id,
    expected_revision: draft.expectedRevision.value,
    name: draft.name.value.trim(),
    group_type: draft.groupType.value,
    inherit_global_filter: draft.inheritGlobalFilter.value,
    ...(draft.groupType.value === 'urltest'
      ? {
          urltest_interval: draft.urltestInterval.value === '' ? 300 : draft.urltestInterval.value,
          urltest_tolerance: draft.urltestTolerance.value === '' ? 50 : draft.urltestTolerance.value,
        }
      : {}),
    sources: draft.sources.value.map((s, idx) => ({ ...s, position: idx })),
    target: 'mihomo',
  }
}

function scheduleLivePreview() {
  if (draft.hasInvalidFilterSources.value) {
    preview.invalidatePreview()
    return
  }
  preview.schedulePreview(getPreviewPayload(), 250)
}

// Initialize on open or group change
watch(
  () => [props.open, props.group] as const,
  ([isOpen, grp]) => {
    if (isOpen) {
      activeSheetTab.value = 'editor'
      draft.initDraft(grp)
      if (props.conflictError) draft.conflictError.value = props.conflictError
      if (props.cycleError) draft.cycleError.value = props.cycleError
      if (props.serverError) draft.serverError.value = props.serverError
      // Trigger initial preview
      scheduleLivePreview()
    } else {
      preview.cleanup()
    }
  },
  { immediate: true }
)

// React to draft modifications: 250ms debounced live preview
watch(
  () => [
    draft.name.value,
    draft.groupType.value,
    draft.emptyFallbackPass.value,
    draft.inheritGlobalFilter.value,
    draft.urltestInterval.value,
    draft.urltestTolerance.value,
    draft.sources.value.length,
    draft.sources.value,
  ],
  () => {
    if (!props.open) return
    scheduleLivePreview()
  },
  { deep: true }
)

function handleTabSwitch(tab: 'editor' | 'preview') {
  activeSheetTab.value = tab
  if (tab === 'preview') {
    preview.triggerImmediate(getPreviewPayload())
  }
}

function handleTabKeydown(event: KeyboardEvent) {
  if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  const tab = event.key === 'Home' ? 'editor' : event.key === 'End' ? 'preview' : activeSheetTab.value === 'editor' ? 'preview' : 'editor'
  handleTabSwitch(tab)
  document.getElementById(tab === 'editor' ? 'policy-source-tab' : 'policy-preview-tab')?.focus()
}

function handleRefreshPreview() {
  preview.triggerImmediate(getPreviewPayload())
}

function handleClose() {
  if (busy.value) return
  if (confirmDiscard()) {
    preview.cleanup()
    emit('close')
  }
}

function handleSave() {
  if (busy.value || props.topologyStatus !== 'ready' || !draft.name.value.trim() || !draft.checkDraftValidity()) {
    return
  }
  const payload = draft.buildSavePayload()
  emit('save-group', payload)
}

function handleReloadFromServer() {
  if (props.group && !busy.value && confirmDiscard()) {
    emit('reload-group', props.group.id)
  }
}

async function handlePreserveLocalDraft() {
  if (!props.group || busy.value) return
  rebasing.value = true
  const groupId = props.group.id
  try {
    let revision = draft.conflictError.value?.currentRevision
    if (!Number.isInteger(revision) || (revision ?? 0) < 1) {
      const latest = await api.get<PolicyGroup>(`/api/v1/policies/groups/${encodeURIComponent(groupId)}`)
      if (latest.id !== groupId) {
        throw new Error(uiText('读取的策略组无效，草稿已保留，请重试。', 'The returned group is invalid. Your draft is preserved; retry.'))
      }
      revision = latest.revision
    }
    if (!Number.isInteger(revision) || (revision ?? 0) < 1) {
      throw new Error(uiText('服务端未返回有效版本，草稿已保留，请重试。', 'The server did not return a valid revision. Your draft is preserved; retry.'))
    }
    if (!props.open || props.group?.id !== groupId) return
    draft.resolveConflictKeepDraft(revision as number)
    draft.clearServerError()
    scheduleLivePreview()
    emit('clear-conflict')
  } catch (error) {
    if (props.open && props.group?.id === groupId) {
      draft.serverError.value = displayText(error instanceof Error ? error.message : uiText('读取最新版本失败，草稿已保留。', 'Failed to read the latest revision. Your draft is preserved.'))
    }
  } finally {
    rebasing.value = false
  }
}

function handleDiscardSourceDraft() {
  draft.clearPendingSourceDraft()
}
</script>

<template>
  <Drawer
    :model-value="open"
    :title="mode === 'edges' ? uiText('管理策略组来源', 'Manage Group Sources') : (group ? uiText('编辑策略组', 'Edit Policy Group') : uiText('创建策略组', 'Create Policy Group'))"
    :description="group ? uiText(`版本 ${draft.expectedRevision.value}`, `Revision ${draft.expectedRevision.value}`) : ''"
    width-class="md:max-w-4xl lg:max-w-6xl"
    @close="handleClose"
  >
      <div class="space-y-3" data-testid="policy-editor-sheet">
        <!-- Sheet Header with Title and Source / Preview tabs -->
        <header class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex items-center gap-2 min-w-0">
          <div class="w-8 h-8 rounded-lg bg-primary/10 text-primary flex items-center justify-center shrink-0">
            <PencilSquareIcon class="w-4 h-4" />
          </div>
          <div class="min-w-0">
            <h2 class="font-bold text-base truncate">
              {{ uiText('策略组编辑', 'Policy Group Editor') }}
            </h2>
          </div>
        </div>

        <!-- Middle: Tab switcher for mobile / narrow viewports -->
        <div role="tablist" :aria-label="uiText('策略组编辑与预览', 'Policy group editor and preview')" class="flex flex-wrap items-center gap-1 bg-base-200 p-1 rounded-xl lg:hidden" @keydown="handleTabKeydown">
          <button
            id="policy-source-tab"
            type="button"
            role="tab"
            aria-controls="policy-source-panel"
            :aria-selected="activeSheetTab === 'editor'"
            :tabindex="activeSheetTab === 'editor' ? 0 : -1"
            class="btn btn-xs rounded-lg gap-1.5 transition-all"
            :class="activeSheetTab === 'editor' ? 'btn-primary' : 'btn-ghost'"
            @click="handleTabSwitch('editor')"
          >
            <PencilSquareIcon class="w-3.5 h-3.5" />
            <span>{{ uiText('编辑来源', 'Edit Sources') }}</span>
          </button>

          <button
            id="policy-preview-tab"
            type="button"
            role="tab"
            aria-controls="policy-preview-panel"
            :aria-selected="activeSheetTab === 'preview'"
            :tabindex="activeSheetTab === 'preview' ? 0 : -1"
            class="btn btn-xs rounded-lg gap-1.5 transition-all"
            :class="activeSheetTab === 'preview' ? 'btn-primary' : 'btn-ghost'"
            @click="handleTabSwitch('preview')"
          >
            <EyeIcon class="w-3.5 h-3.5" />
            <span>{{ uiText('解析快照预览', 'Snapshot Preview') }}</span>
          </button>
        </div>

        </header>

      <!-- Alert Banners: 409 Conflict, 422 Cycle, Server Error -->
      <div class="space-y-2">
        <!-- 409 Conflict Banner -->
        <div
          v-if="draft.conflictError.value"
          role="alert"
          class="p-3.5 rounded-xl bg-error/10 border border-error/30 text-error text-xs flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 break-words"
        >
          <div class="flex items-start gap-2.5 min-w-0">
            <ExclamationTriangleIcon class="w-5 h-5 shrink-0 mt-0.5" />
            <div class="min-w-0 [overflow-wrap:anywhere]">
              <h4 class="font-bold">{{ uiText('版本冲突', 'Revision conflict') }} (409 Conflict)</h4>
              <p class="opacity-90 mt-0.5 whitespace-pre-wrap break-words">{{ displayText(draft.conflictError.value.message) }}</p>
            </div>
          </div>
          <div class="flex flex-wrap items-center gap-2 shrink-0">
            <button
              type="button"
              class="btn btn-xs btn-error btn-outline"
              :disabled="busy"
              @click="handlePreserveLocalDraft"
            >
              {{ uiText('保留本地草稿', 'Keep local draft') }}
            </button>
            <button
              type="button"
              class="btn btn-xs btn-error"
              :disabled="busy"
              @click="handleReloadFromServer"
            >
              {{ uiText('重新拉取服务端版本', 'Reload server version') }}
            </button>
          </div>
        </div>

        <!-- 422 Cycle Detected Banner -->
        <div
          v-if="draft.cycleError.value"
          role="alert"
          class="p-3.5 rounded-xl bg-error/10 border border-error/30 text-error text-xs flex items-start gap-2.5 break-words"
        >
          <ExclamationTriangleIcon class="w-5 h-5 shrink-0 mt-0.5" />
          <div class="min-w-0 [overflow-wrap:anywhere]">
            <h4 class="font-bold">{{ uiText('检测到拓扑成环', 'Topology cycle detected') }} (422 Unprocessable Entity)</h4>
            <p class="opacity-90 mt-0.5 whitespace-pre-wrap break-words">{{ displayText(draft.cycleError.value.message) }}</p>
            <p v-if="draft.cycleError.value.cyclePath && draft.cycleError.value.cyclePath.length > 0" class="font-mono mt-1 text-[11px] font-semibold bg-base-100/70 p-1.5 rounded whitespace-pre-wrap break-words">
              {{ draft.cycleError.value.cyclePath.map((id) => displayName(allGroups.find((g) => g.id === id)?.name, '未知策略组', id)).join(' → ') }}
            </p>
          </div>
        </div>

        <!-- Server Error Banner -->
        <div
          v-if="draft.serverError.value"
          role="alert"
          class="p-3 rounded-xl bg-error/10 border border-error/30 text-error text-xs flex items-start justify-between gap-2 break-words"
        >
          <div class="flex items-center gap-2 min-w-0">
            <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
            <span class="min-w-0 whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{{ displayText(draft.serverError.value) }}</span>
          </div>
          <button
            type="button"
            class="btn btn-ghost btn-xs"
            @click="draft.clearServerError"
          >
            ✕
          </button>
        </div>

        <!-- Topology Loading / Idle / Error / Stale Banner -->
        <div
          v-if="topologyStatus === 'loading' || topologyStatus === 'idle'"
          data-testid="sheet-topology-loading"
          class="p-3 rounded-xl bg-info/10 border border-info/30 flex items-center gap-2 text-xs text-info"
        >
          <span class="loading loading-spinner loading-xs" />
          <span>正在加载完整策略组拓扑，就绪前禁止保存来源以避免误写...</span>
        </div>

        <div
          v-else-if="topologyStatus === 'error'"
          data-testid="sheet-topology-error"
          class="p-3 rounded-xl bg-warning/10 border border-warning/30 flex items-start justify-between gap-2 text-xs text-warning-content break-words"
        >
          <div class="flex items-center gap-2 min-w-0">
            <ExclamationTriangleIcon class="w-4 h-4 text-warning shrink-0" />
            <span class="min-w-0 whitespace-pre-wrap break-words [overflow-wrap:anywhere]">
              {{ displayText(topologyError) || uiText('全量策略组拓扑加载失败', 'Failed to load the full policy topology') }}
              {{ isTopologyStale ? '（当前显示为过期拓扑缓存，禁止保存）' : '（拓扑未就绪，禁止保存以避免丢失引用）' }}
            </span>
          </div>
          <button
            type="button"
            data-testid="sheet-retry-topology-btn"
            class="btn btn-warning btn-xs shrink-0"
            @click="emit('retry-topology')"
          >
            重试
          </button>
        </div>
      </div>

      <!-- Main Body: Responsive side-by-side on wide screens (lg:), tabbed on mobile -->
      <main class="min-w-0">
        <form
          id="policy-editor-group-form"
          class="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start min-w-0"
          @submit.prevent="handleSave"
        >
          <fieldset :disabled="busy" class="contents">
          <!-- Left Column: Source Pipeline Editor (7 cols on lg) -->
          <div
            id="policy-source-panel"
            role="tabpanel"
            aria-labelledby="policy-source-tab"
            tabindex="0"
            class="lg:col-span-7 space-y-4 min-w-0"
            :class="{ 'hidden lg:block': activeSheetTab !== 'editor' }"
          >
            <section
              v-if="draft.groupType.value === 'urltest'"
              data-testid="urltest-settings"
              class="grid grid-cols-1 sm:grid-cols-2 gap-3 rounded-xl border border-base-300 bg-base-200/60 p-3"
            >
              <label class="flex flex-col gap-1">
                <span class="text-xs font-semibold">{{ uiText('测速间隔（秒）', 'Test interval (seconds)') }}</span>
                <input
                  :value="draft.urltestInterval.value"
                  type="number"
                  min="0"
                  max="86400"
                  step="1"
                  class="input input-sm"
                  data-testid="urltest-interval-input"
                  @input="draft.urltestInterval.value = ($event.target as HTMLInputElement).value === '' ? '' : Number(($event.target as HTMLInputElement).value)"
                />
                <span class="text-[11px] opacity-70">{{ uiText('留空使用默认值 300 秒；0 停止定时测速；最大 86400 秒。', 'Blank uses the 300-second default; 0 disables periodic tests; maximum 86400 seconds.') }}</span>
              </label>
              <label class="flex flex-col gap-1">
                <span class="text-xs font-semibold">{{ uiText('容忍抖动（毫秒）', 'Tolerance (milliseconds)') }}</span>
                <input
                  :value="draft.urltestTolerance.value"
                  type="number"
                  min="0"
                  max="60000"
                  step="1"
                  class="input input-sm"
                  data-testid="urltest-tolerance-input"
                  @input="draft.urltestTolerance.value = ($event.target as HTMLInputElement).value === '' ? '' : Number(($event.target as HTMLInputElement).value)"
                />
                <span class="text-[11px] opacity-70">{{ uiText('默认值 50 毫秒；0 表示不容忍抖动。', 'Defaults to 50 milliseconds; 0 disables tolerance.') }}</span>
              </label>
            </section>
            <SourcePipelineEditor
              v-model:name="draft.name.value"
              v-model:group-type="draft.groupType.value"
              v-model:position="draft.position.value"
              v-model:inherit-global-filter="draft.inheritGlobalFilter.value"
              v-model:pending-pattern="draft.pendingRegexPattern.value"
              :sources="draft.sources.value"
              :has-invalid-filter-sources="draft.hasInvalidFilterSources.value"
              :form-error="draft.formError.value"
              :all-groups="allGroups"
              :available-nodes="availableNodes"
              :group-id="group?.id || ''"
              :mode="mode"
              :topology-status="topologyStatus"
              @add-source="draft.addSource"
              @add-filter="draft.addFilterSource"
              @remove-source="draft.removeSource"
              @move-source-up="draft.moveSourceUp"
              @move-source-down="draft.moveSourceDown"
              @move-source="draft.moveSource"
              @toggle-source-op="draft.toggleSourceOp"
              @filter-invalid="draft.setFilterInvalid"
            />
          </div>

          <!-- Right Column: Live Snapshot Preview Panel (5 cols on lg) -->
          <div
            id="policy-preview-panel"
            role="tabpanel"
            aria-labelledby="policy-preview-tab"
            tabindex="0"
            class="lg:col-span-5 sticky top-0 min-w-0"
            :class="{ 'hidden lg:block': activeSheetTab !== 'preview' }"
          >
            <GroupPreviewPanel
              :loading="preview.previewLoading.value"
              :snapshot="preview.snapshotPreview.value"
              :error="preview.previewError.value"
              :diagnostics="preview.previewDiagnostics.value"
              :is-draft="true"
              :sources-count="draft.sources.value.length"
              @refresh="handleRefreshPreview"
            />
          </div>
          </fieldset>
        </form>
      </main>
      </div>

      <template #footer>
      <div class="w-full space-y-3">
      <!-- Pending source draft banner -->
      <div
        v-if="draft.hasPendingSourceDraft.value"
        role="status"
        class="px-4 py-2 border-t border-base-300 bg-base-200 text-xs space-y-1 shrink-0"
      >
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span>来源草稿尚未应用，不参与预览或保存。请先添加 / 更新来源。</span>
          <button
            type="button"
            class="btn btn-ghost btn-xs condition-action-btn"
            :disabled="busy"
            @click="handleDiscardSourceDraft"
          >
            放弃来源草稿
          </button>
        </div>
        <p v-if="draft.formError.value" class="text-error">{{ draft.formError.value }}</p>
      </div>

      <!-- Sheet Footer: Cancel & Atomic Save -->
      <footer class="pt-3 border-t border-base-300 flex flex-wrap items-center justify-between gap-3">
        <div class="text-xs text-base-content/60 font-mono truncate">
          <span v-if="draft.isDirty.value" class="text-warning font-semibold">
            ● {{ uiText('草稿已修改', 'Unsaved changes') }}
          </span>
          <span v-else class="opacity-60">
            {{ uiText('无未保存改动', 'No changes') }}
          </span>
        </div>

        <div class="flex items-center gap-2">
          <button
            type="button"
            class="btn btn-ghost btn-sm"
            :disabled="busy"
            @click="handleClose"
          >
            {{ uiText('取消', 'Cancel') }}
          </button>

          <button
            type="submit"
            form="policy-editor-group-form"
            data-testid="policy-sheet-save-btn"
            class="btn btn-primary btn-sm"
            :class="{ loading: saving }"
            :disabled="busy || !draft.name.value.trim() || draft.hasInvalidFilterSources.value || topologyStatus !== 'ready'"
            @click.prevent="handleSave"
          >
            {{ mode === 'edges' ? uiText('保存来源', 'Save Sources') : (group ? uiText('保存策略组', 'Save Group') : uiText('创建策略组', 'Create Group')) }}
          </button>
        </div>
      </footer>
      </div>
      </template>
  </Drawer>
</template>
