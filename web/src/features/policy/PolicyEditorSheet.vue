<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import {
  XMarkIcon,
  ExclamationTriangleIcon,
  EyeIcon,
  PencilSquareIcon,
} from '@heroicons/vue/24/outline'
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
import { useBodyScrollLock } from '../../composables/useBodyScrollLock'
import { useUnsavedChanges } from '../../composables/useUnsavedChanges'
import { uiText } from '../../locales/redesign'
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
  topologyStatus?: 'idle' | 'loading' | 'error' | 'ready'
  topologyError?: string
  isTopologyStale?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  mode: 'group',
  saving: false,
  availableNodes: () => [],
  conflictError: null,
  cycleError: null,
  serverError: null,
  topologyStatus: 'idle',
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
    sources: MemberSource[]
    expectedRevision?: number
  }): void
  (e: 'reload-group', id: string): void
  (e: 'clear-conflict'): void
  (e: 'retry-topology'): void
}>()

// Engage mobile body scroll lock when sheet is open
useBodyScrollLock(computed(() => props.open))

// Draft and Preview composables
const draft = useGroupDraft()
const preview = useGroupPreview()

// Guard against unsaved changes on close or navigation
const { confirmDiscard } = useUnsavedChanges(
  draft.isDirty,
  () => uiText('当前草稿尚未保存，确定要放弃修改吗？', 'Current group draft has unsaved changes. Discard?')
)

// Active mobile tab: 'editor' | 'preview'
const activeSheetTab = ref<'editor' | 'preview'>('editor')

// Sync props errors into draft
watch(
  () => props.conflictError,
  (c) => {
    if (c) draft.conflictError.value = c
  },
  { immediate: true }
)
watch(
  () => props.cycleError,
  (c) => {
    if (c) draft.cycleError.value = c
  },
  { immediate: true }
)
watch(
  () => props.serverError,
  (s) => {
    if (s) draft.serverError.value = s
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
    empty_fallback_pass: draft.emptyFallbackPass.value,
    inherit_global_filter: draft.inheritGlobalFilter.value,
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

function handleRefreshPreview() {
  preview.triggerImmediate(getPreviewPayload())
}

function handleClose() {
  if (confirmDiscard()) {
    preview.cleanup()
    emit('close')
  }
}

function handleSave() {
  if (!draft.checkDraftValidity()) {
    return
  }
  const payload = draft.buildSavePayload()
  emit('save-group', payload)
}

function handleReloadFromServer() {
  if (props.group) {
    emit('reload-group', props.group.id)
  }
}

function handlePreserveLocalDraft() {
  if (draft.conflictError.value?.currentRevision) {
    draft.resolveConflictKeepDraft(draft.conflictError.value.currentRevision)
  } else {
    draft.clearConflict()
  }
  emit('clear-conflict')
}

function handleDiscardSourceDraft() {
  draft.clearPendingSourceDraft()
}
</script>

<template>
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
      class="fixed inset-0 bg-neutral/40 z-40 backdrop-blur-xs transition-opacity"
      aria-hidden="true"
      @click="handleClose"
    />
  </Transition>

  <Transition
    enter-active-class="transition-all duration-300 ease-out"
    enter-from-class="translate-y-full md:translate-y-4 md:opacity-0"
    enter-to-class="translate-y-0 md:translate-y-0 md:opacity-100"
    leave-active-class="transition-all duration-200 ease-in"
    leave-from-class="translate-y-0 md:translate-y-0 md:opacity-100"
    leave-to-class="translate-y-full md:translate-y-4 md:opacity-0"
  >
    <section
      v-if="open"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="mode === 'edges' ? 'edges-editor-title' : 'policy-editor-title'"
      class="adaptive-surface-sheet fixed bottom-0 inset-x-0 z-50 rounded-t-2xl md:rounded-2xl bg-base-100 border border-base-300 shadow-2xl flex flex-col max-w-full md:max-w-4xl lg:max-w-6xl md:mx-auto md:bottom-6 md:inset-x-4 max-h-[92vh] md:max-h-[85vh] overflow-hidden"
    >
      <!-- Sheet Header with Title, Tab switcher, and Close button -->
      <header class="p-4 border-b border-base-300 flex flex-wrap items-center justify-between gap-3 shrink-0 bg-base-100">
        <div class="flex items-center gap-2 min-w-0">
          <div class="w-8 h-8 rounded-lg bg-primary/10 text-primary flex items-center justify-center shrink-0">
            <PencilSquareIcon class="w-4 h-4" />
          </div>
          <div class="min-w-0">
            <h2
              :id="mode === 'edges' ? 'edges-editor-title' : 'policy-editor-title'"
              class="font-bold text-base truncate"
            >
              {{ mode === 'edges' ? uiText('管理策略组来源', 'Manage Group Sources') : (group ? uiText('编辑策略组', 'Edit Policy Group') : uiText('创建策略组', 'Create Policy Group')) }}
            </h2>
            <p v-if="group" class="text-[11px] font-mono opacity-60 truncate">
              ID: {{ group.id }} · v{{ draft.expectedRevision.value }}
            </p>
          </div>
        </div>

        <!-- Middle: Tab switcher for mobile / narrow viewports -->
        <div class="flex items-center gap-1 bg-base-200 p-1 rounded-xl">
          <button
            type="button"
            class="btn btn-xs rounded-lg gap-1.5 transition-all"
            :class="activeSheetTab === 'editor' ? 'btn-primary' : 'btn-ghost'"
            @click="handleTabSwitch('editor')"
          >
            <PencilSquareIcon class="w-3.5 h-3.5" />
            <span>{{ uiText('编辑来源', 'Edit Sources') }}</span>
          </button>

          <button
            type="button"
            class="btn btn-xs rounded-lg gap-1.5 transition-all"
            :class="activeSheetTab === 'preview' ? 'btn-primary' : 'btn-ghost'"
            @click="handleTabSwitch('preview')"
          >
            <EyeIcon class="w-3.5 h-3.5" />
            <span>{{ uiText('解析快照预览', 'Snapshot Preview') }}</span>
          </button>
        </div>

        <!-- Close button -->
        <button
          type="button"
          class="btn btn-ghost btn-sm btn-circle shrink-0"
          aria-label="关闭"
          @click="handleClose"
        >
          <XMarkIcon class="w-5 h-5" />
        </button>
      </header>

      <!-- Alert Banners: 409 Conflict, 422 Cycle, Server Error -->
      <div class="shrink-0 px-4 pt-3 space-y-2">
        <!-- 409 Conflict Banner -->
        <div
          v-if="draft.conflictError.value"
          role="alert"
          class="p-3.5 rounded-xl bg-error/10 border border-error/30 text-error text-xs flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3"
        >
          <div class="flex items-start gap-2.5 min-w-0">
            <ExclamationTriangleIcon class="w-5 h-5 shrink-0 mt-0.5" />
            <div>
              <h4 class="font-bold">版本冲突 (409 Conflict)</h4>
              <p class="opacity-90 mt-0.5">{{ draft.conflictError.value.message }}</p>
            </div>
          </div>
          <div class="flex items-center gap-2 shrink-0">
            <button
              type="button"
              class="btn btn-xs btn-error btn-outline"
              @click="handlePreserveLocalDraft"
            >
              保留本地草稿
            </button>
            <button
              type="button"
              class="btn btn-xs btn-error"
              @click="handleReloadFromServer"
            >
              重新拉取服务端版本
            </button>
          </div>
        </div>

        <!-- 422 Cycle Detected Banner -->
        <div
          v-if="draft.cycleError.value"
          role="alert"
          class="p-3.5 rounded-xl bg-error/10 border border-error/30 text-error text-xs flex items-start gap-2.5"
        >
          <ExclamationTriangleIcon class="w-5 h-5 shrink-0 mt-0.5" />
          <div class="min-w-0">
            <h4 class="font-bold">检测到拓扑成环 (422 Unprocessable Entity)</h4>
            <p class="opacity-90 mt-0.5">{{ draft.cycleError.value.message }}</p>
            <p v-if="draft.cycleError.value.cyclePath && draft.cycleError.value.cyclePath.length > 0" class="font-mono mt-1 text-[11px] font-semibold bg-base-100/70 p-1.5 rounded">
              {{ draft.cycleError.value.cyclePath.join(' → ') }}
            </p>
          </div>
        </div>

        <!-- Server Error Banner -->
        <div
          v-if="draft.serverError.value"
          role="alert"
          class="p-3 rounded-xl bg-error/10 border border-error/30 text-error text-xs flex items-center justify-between gap-2"
        >
          <div class="flex items-center gap-2 min-w-0">
            <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
            <span class="truncate">{{ draft.serverError.value }}</span>
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
          class="p-3 rounded-xl bg-warning/10 border border-warning/30 flex items-center justify-between gap-2 text-xs text-warning-content"
        >
          <div class="flex items-center gap-2 min-w-0">
            <ExclamationTriangleIcon class="w-4 h-4 text-warning shrink-0" />
            <span class="truncate">
              {{ topologyError || '全量策略组拓扑加载失败' }}
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
      <main class="flex-1 overflow-y-auto p-4 min-w-0">
        <form
          id="policy-editor-group-form"
          class="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start min-w-0"
          @submit.prevent="handleSave"
        >
          <!-- Left Column: Source Pipeline Editor (7 cols on lg) -->
          <div
            class="lg:col-span-7 space-y-4 min-w-0"
            :class="{ 'hidden lg:block': activeSheetTab !== 'editor' }"
          >
            <SourcePipelineEditor
              v-model:name="draft.name.value"
              v-model:group-type="draft.groupType.value"
              v-model:position="draft.position.value"
              v-model:empty-fallback-pass="draft.emptyFallbackPass.value"
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
        </form>
      </main>

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
            @click="handleDiscardSourceDraft"
          >
            放弃来源草稿
          </button>
        </div>
        <p v-if="draft.formError.value" class="text-error">{{ draft.formError.value }}</p>
      </div>

      <!-- Sheet Footer: Cancel & Atomic Save -->
      <footer class="p-4 border-t border-base-300 flex items-center justify-between gap-3 shrink-0 bg-base-100">
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
            :disabled="saving || !draft.name.value.trim() || draft.hasInvalidFilterSources.value || topologyStatus !== 'ready'"
            @click.prevent="handleSave"
          >
            {{ mode === 'edges' ? uiText('保存来源', 'Save Sources') : (group ? uiText('保存策略组', 'Save Group') : uiText('创建策略组', 'Create Group')) }}
          </button>
        </div>
      </footer>
    </section>
  </Transition>
</template>
