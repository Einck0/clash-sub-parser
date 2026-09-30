<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  ArrowPathIcon,
  ClipboardDocumentCheckIcon,
  ClipboardDocumentIcon,
  ArrowDownTrayIcon,
  PaperAirplaneIcon,
  ExclamationTriangleIcon,
  CheckBadgeIcon,
  XCircleIcon,
  DocumentDuplicateIcon,
  Cog6ToothIcon,
  CubeTransparentIcon,
  ShieldCheckIcon,
} from '@heroicons/vue/24/outline'
import { usePublications } from './usePublications'
import {
  COMPILER_TARGETS,
  auditEventLabel,
  formatDigest,
  getTargetMetadata,
  groupTypeLabel,
  preflightCheckLabel,
  publicationStateLabel,
  targetFileExt,
  type CompilerTarget,
} from './publicationTypes'
import ModalDialog from '../../ui/ModalDialog.vue'
import ConfirmModal from '../../ui/ConfirmModal.vue'
import ErrorStateCard from '../../ui/ErrorStateCard.vue'
import { t } from '../../locales'

const router = (() => {
  try {
    return useRouter()
  } catch {
    return null
  }
})()

const {
  selectedTarget,
  preview,
  activePublication,
  loadingPreview,
  publishing,
  revoking,
  error,
  errorDetail,
  preflightDiagnostics,
  isNoActiveRevision,
  copied,
  fetchPreview,
  publish,
  revoke,
  copyToClipboard,
  downloadFile,
  restoreActivePublication,
  getFullExportUrl,
} = usePublications()

const publishModalOpen = ref(false)
const copiedToken = ref(false)

const currentTargetMeta = computed(() => getTargetMetadata(selectedTarget.value))

async function switchTarget(target: CompilerTarget) {
  selectedTarget.value = target
  restoreActivePublication(target)
  await fetchPreview(target)
}

async function handleCopyContent() {
  if (!preview.value?.content) return
  await copyToClipboard(preview.value.content)
}

function handleDownload() {
  if (!preview.value) return
  const ext = targetFileExt(preview.value.target)
  downloadFile(
    preview.value.filename || `config-${preview.value.target}.${ext}`,
    preview.value.content,
    preview.value.content_type
  )
}

async function handlePublish() {
  const target = selectedTarget.value
  try {
    await publish(target)
    if (selectedTarget.value === target) publishModalOpen.value = true
  } catch {
    // handled in composable
  }
}

async function copyPublicationURL() {
  const fullUrl = getFullExportUrl(activePublication.value)
  if (!fullUrl) return
  await copyToClipboard(fullUrl)
  copiedToken.value = true
  setTimeout(() => {
    copiedToken.value = false
  }, 2000)
}

const confirmRevokeOpen = ref(false)

function handleRevoke() {
  if (!activePublication.value?.id) return
  confirmRevokeOpen.value = true
}

async function confirmRevokePublication() {
  if (!activePublication.value?.id) return
  try {
    await revoke(activePublication.value.id)
    confirmRevokeOpen.value = false
  } catch {
    // handled in composable
  }
}

onMounted(() => {
  restoreActivePublication(selectedTarget.value)
  fetchPreview(selectedTarget.value)
})
</script>

<template>
  <section class="space-y-5" aria-labelledby="publications-title">
    <!-- Header -->
    <div class="flex flex-col sm:flex-row sm:items-end justify-between gap-3 w-full min-w-0">
      <div class="min-w-0">
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-primary">多目标编译与发布</p>
        <h2 id="publications-title" class="mt-1 text-2xl font-bold truncate">{{ t('publications.title') }}</h2>
        <p class="mt-1 text-sm opacity-70">
          {{ t('publications.subtitle') }}
        </p>
      </div>

      <div class="flex flex-wrap items-center gap-2 shrink-0">
        <button
          type="button"
          class="btn btn-ghost btn-sm btn-square"
          :title="t('common.refresh')"
          :disabled="loadingPreview"
          @click="fetchPreview(selectedTarget)"
        >
          <ArrowPathIcon class="w-4 h-4" :class="{ 'animate-spin': loadingPreview }" />
        </button>

        <button
          v-if="activePublication && !activePublication.revoked_at && activePublication.export_url"
          type="button"
          class="btn btn-secondary btn-sm gap-1.5 touch-manipulation"
          data-testid="copy-subscription-url-btn"
          @click="copyPublicationURL"
        >
          <ClipboardDocumentIcon class="w-4 h-4" />
          {{ copiedToken ? '已复制！' : '复制订阅链接' }}
        </button>

        <button
          type="button"
          class="btn btn-outline btn-sm gap-1.5"
          :disabled="!preview || loadingPreview"
          @click="handleDownload"
        >
          <ArrowDownTrayIcon class="w-4 h-4" />
          {{ t('publications.download') }}
        </button>

        <button
          type="button"
          class="btn btn-primary btn-sm gap-1.5"
          :disabled="!preview || loadingPreview || publishing"
          :class="{ loading: publishing }"
          @click="handlePublish"
        >
          <PaperAirplaneIcon class="w-4 h-4" />
          {{ t('publications.createPub') }}
        </button>
      </div>
    </div>

    <!-- Error Alert / Degraded State Card -->
    <!-- 409 no_active_revision: Actionable recoverable pre-condition -->
    <div
      v-if="isNoActiveRevision"
      role="alert"
      class="rounded-xl border border-warning/30 bg-warning/10 p-4 sm:p-5 text-base-content shadow-sm transition-all duration-200 min-w-0 w-full overflow-hidden"
      data-testid="no-active-revision-card"
    >
      <div class="flex items-start gap-3.5 min-w-0">
        <div class="p-2 rounded-xl bg-warning/20 text-warning shrink-0 mt-0.5">
          <ExclamationTriangleIcon class="w-5 h-5" />
        </div>
        <div class="min-w-0 flex-1 space-y-1.5">
          <div class="flex flex-wrap items-center gap-2">
            <h3 class="font-bold text-sm sm:text-base text-warning">
              {{ t('publications.noActiveRevisionTitle') }}
            </h3>
            <span class="badge badge-warning badge-sm font-mono text-[11px] h-auto py-0.5">
              HTTP 409
            </span>
            <span class="badge badge-outline badge-warning badge-sm font-mono text-[11px] h-auto py-0.5">
              no_active_revision
            </span>
          </div>
          <p class="text-xs sm:text-sm text-base-content/80 leading-relaxed break-words">
            {{ t('publications.noActiveRevisionDesc') }}
          </p>
          <div class="pt-2 flex flex-wrap items-center gap-2">
            <button
              type="button"
              class="btn btn-sm btn-outline btn-warning gap-1.5 touch-manipulation"
              :disabled="loadingPreview"
              :class="{ loading: loadingPreview }"
              @click="() => fetchPreview(selectedTarget)"
            >
              <ArrowPathIcon class="w-4 h-4" :class="{ 'animate-spin': loadingPreview }" />
              <span>{{ t('common.retry') }}</span>
            </button>
            <button
              type="button"
              class="btn btn-sm btn-warning gap-1.5 touch-manipulation"
              @click="router?.push ? router.push('/policy') : undefined"
            >
              <Cog6ToothIcon class="w-4 h-4" />
              <span>{{ t('publications.goToPolicy') }}</span>
            </button>
            <button
              type="button"
              class="btn btn-sm btn-ghost border border-base-300 gap-1.5 touch-manipulation hover:bg-base-200"
              @click="router?.push ? router.push('/subscriptions') : undefined"
            >
              <CubeTransparentIcon class="w-4 h-4" />
              <span>{{ t('publications.goToSubscriptions') }}</span>
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Generic Error (401, 403, 422, 500, network, etc.) -->
    <ErrorStateCard
      v-else-if="error"
      :error="errorDetail || error"
      :retrying="loadingPreview"
      @retry="() => fetchPreview(selectedTarget)"
    />

    <!-- Preflight Diagnostics Block (e.g. Empty Route Group Rejection) -->
    <div
      v-if="preflightDiagnostics && preflightDiagnostics.length > 0"
      class="p-4 rounded-xl bg-error/10 border border-error/30 text-error text-xs space-y-2"
    >
      <div class="font-bold flex items-center gap-2 text-sm">
        <ExclamationTriangleIcon class="w-5 h-5 flex-shrink-0" />
        预览或发布已阻断：筛选、路由组或安全预检未通过
      </div>
      <p class="opacity-85">
        筛选后可能没有可导出的节点，也可能是路由组为空或安全预检拦截。请查看下方诊断并检查筛选条件与探针观测；当前不能预览或新发布配置。
      </p>
      <ul class="list-disc list-inside font-mono space-y-1 pl-1">
        <li v-for="(diag, idx) in preflightDiagnostics" :key="idx">
          <span v-if="diag.code" class="badge badge-xs badge-error badge-outline mr-1.5 font-sans">
            {{ preflightCheckLabel(diag.code) }}
          </span>
          <strong>{{ diag.target ? `[${diag.target}] ` : '' }}{{ diag.message }}</strong>
          <span v-if="diag.reason" class="opacity-75 block pl-4">原因：{{ diag.reason }}</span>
        </li>
      </ul>
    </div>

    <!-- Target Selector Tabs -->
    <div class="flex flex-wrap items-center gap-2 pb-1 text-xs w-full min-w-0" role="tablist" aria-label="目标导出格式">
      <button
        v-for="item in COMPILER_TARGETS"
        :key="item.target"
        type="button"
        role="tab"
        :aria-selected="selectedTarget === item.target"
        class="btn btn-sm rounded-xl font-medium transition-all duration-200 ease-out active:scale-[0.98]"
        :class="selectedTarget === item.target ? 'btn-primary shadow-sm' : 'btn-ghost bg-base-200/60 hover:bg-base-300/60'"
        @click="switchTarget(item.target)"
      >
        <span>{{ item.label }}</span>
        <span
          class="badge badge-xs uppercase font-mono"
          :class="selectedTarget === item.target ? 'badge-primary-content text-primary' : 'badge-ghost'"
        >
          .{{ item.ext }}
        </span>
      </button>
    </div>

    <!-- Target Capability Boundary Card -->
    <div
      data-testid="target-capability-boundary"
      class="rounded-xl border border-base-300 bg-base-200/70 p-3.5 sm:p-4 text-xs space-y-2 min-w-0 w-full"
    >
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex items-center gap-2 font-semibold text-base-content">
          <ShieldCheckIcon class="w-4 h-4 text-primary shrink-0" />
          <span>{{ t('publications.capabilityBoundaryTitle') }} · {{ currentTargetMeta.label }}</span>
        </div>
        <span class="text-xs opacity-80">{{ currentTargetMeta.desc }}</span>
      </div>
      <div class="grid grid-cols-1 md:grid-cols-3 gap-2 font-mono text-[11px]">
        <div class="p-2 rounded-lg bg-base-100/70 border border-base-300/60">
          <span class="opacity-60 font-sans block mb-1">支持节点协议 ({{ currentTargetMeta.protocols.length }})</span>
          <div class="flex flex-wrap gap-1">
            <span
              v-for="proto in currentTargetMeta.protocols"
              :key="proto"
              class="badge badge-xs badge-primary badge-outline"
            >
              {{ proto }}
            </span>
          </div>
        </div>
        <div class="p-2 rounded-lg bg-base-100/70 border border-base-300/60">
          <span class="opacity-60 font-sans block mb-1">策略组支持 ({{ currentTargetMeta.groupTypes.length }})</span>
          <div v-if="currentTargetMeta.groupTypes.length > 0" class="flex flex-wrap gap-1">
            <span
              v-for="gt in currentTargetMeta.groupTypes"
              :key="gt"
              class="badge badge-xs badge-secondary badge-outline"
            >
              {{ groupTypeLabel(gt) }}
            </span>
          </div>
          <div v-else class="text-base-content/75 font-sans">
            仅导出节点格式（忽略策略组与规则）
          </div>
        </div>
        <div class="p-2 rounded-lg bg-base-100/70 border border-base-300/60">
          <span class="opacity-60 font-sans block mb-1">导出范围与能力边界</span>
          <p class="leading-snug text-base-content/85">{{ currentTargetMeta.ruleSummary }}</p>
          <p class="mt-1 text-warning leading-snug">{{ currentTargetMeta.unsupportedSummary }}</p>
        </div>
      </div>
    </div>

    <!-- Main Preview Card (200ms ease-out) -->
    <div class="card border border-base-300 bg-base-200 shadow-sm transition-all duration-200 ease-out min-w-0 w-full overflow-hidden">
      <div class="card-body p-4 sm:p-5 gap-3 min-w-0 max-w-full">
        <!-- Metadata Header Bar -->
        <div class="flex flex-wrap items-center justify-between gap-2 border-b border-base-300 pb-3 text-xs min-w-0">
          <div class="flex flex-wrap items-center gap-3 min-w-0">
            <span class="font-bold text-sm flex items-center gap-1.5 truncate max-w-[200px] sm:max-w-xs">
              {{ preview?.filename || `config.${selectedTarget}` }}
            </span>
            <span v-if="preview?.content_type" class="badge badge-sm badge-outline font-mono text-[11px] shrink-0">
              {{ preview.content_type }}
            </span>
          </div>

          <div class="flex items-center gap-2 font-mono text-[11px] opacity-70 shrink-0">
            <span v-if="preview?.content_digest" title="配置内容 SHA-256 摘要">
              内容摘要: <strong>{{ formatDigest(preview.content_digest) }}</strong>
            </span>
            <span v-if="preview?.snapshot_digest" class="hidden sm:inline" title="策略快照摘要">
              · 快照摘要: <strong>{{ formatDigest(preview.snapshot_digest) }}</strong>
            </span>
          </div>
        </div>

        <!-- Filter Layer Counts Bar (if available) -->
        <div
          v-if="preview?.filter_counts"
          class="flex flex-col gap-2 p-2.5 rounded-lg bg-base-100 border border-base-300 text-xs font-mono"
        >
          <div class="flex flex-wrap items-center gap-3">
            <span class="opacity-60 font-sans">过滤流水线：</span>
            <span>原始节点: <strong>{{ preview.filter_counts.raw_total ?? '--' }}</strong></span>
            <span class="opacity-40">→</span>
            <span>准入通过: <strong>{{ preview.filter_counts.admitted_total ?? '--' }}</strong></span>
            <span class="opacity-40">→</span>
            <span class="text-primary font-semibold">
              全局保留: <strong>{{ preview.filter_counts.global_filtered_total ?? '--' }}</strong>
            </span>
            <span class="opacity-40">→</span>
            <span class="text-success font-semibold">
              策略组保留: <strong>{{ preview.filter_counts.group_filtered_total ?? '--' }}</strong>
            </span>
          </div>

          <div
            v-if="preview.filter_counts.group_counts && Object.keys(preview.filter_counts.group_counts).length > 0"
            class="flex flex-wrap items-center gap-1.5 pt-1.5 border-t border-base-300/40 text-[11px]"
          >
            <span class="opacity-60 font-sans">各策略组统计：</span>
            <span
              v-for="(gc, gName) in preview.filter_counts.group_counts"
              :key="gName"
              class="badge badge-xs font-mono badge-ghost"
              :title="`候选: ${gc.candidate}, 保留: ${gc.kept}, 排除: ${gc.excluded}`"
            >
              {{ gName }}: {{ gc.kept }}/{{ gc.candidate }}
            </span>
          </div>
        </div>

        <!-- Diagnostics Warnings if any -->
        <div
          v-if="preview?.diagnostics && preview.diagnostics.length > 0"
          class="p-3 rounded-lg bg-warning/10 border border-warning/30 text-warning text-xs space-y-1"
        >
          <div class="font-bold flex items-center gap-1.5">
            <ExclamationTriangleIcon class="w-4 h-4" />
            编译器诊断提示
          </div>
          <ul class="list-disc list-inside font-mono space-y-0.5">
            <li v-for="(diag, idx) in preview.diagnostics" :key="idx">
              <span v-if="diag.code" class="badge badge-xs badge-warning badge-outline mr-1.5 font-sans">
                {{ preflightCheckLabel(diag.code) }}
              </span>
              <span>{{ diag.message }}</span>
              <span v-if="diag.reason" class="opacity-75 block text-[11px] pl-4">
                原因：{{ diag.reason }}
              </span>
              <span v-if="diag.excluded_count !== undefined" class="opacity-75 block text-[11px] pl-4">
                已排除：{{ diag.excluded_count }} 个节点
              </span>
            </li>
          </ul>
        </div>

        <!-- Code Preview Container with Quick Copy Floating Button -->
        <div class="relative group min-w-0 w-full">
          <div
            v-if="loadingPreview"
            class="skeleton h-80 w-full rounded-xl"
          />

          <div
            v-else
            class="relative rounded-xl bg-base-300/40 border border-base-300 overflow-hidden min-w-0 w-full"
          >
            <!-- Quick Copy Button (Sticky Top Right) -->
            <button
              type="button"
              class="absolute top-3 right-3 btn btn-xs gap-1.5 shadow-md z-10 transition-all duration-200"
              :disabled="!preview?.content"
              :class="copied ? 'btn-success' : 'btn-neutral bg-base-100/90 text-base-content hover:bg-base-100'"
              @click="handleCopyContent"
            >
              <ClipboardDocumentCheckIcon v-if="copied" class="w-4 h-4 text-success-content" />
              <ClipboardDocumentIcon v-else class="w-4 h-4" />
              <span>{{ copied ? '已复制！' : '复制配置' }}</span>
            </button>

            <!-- Pre Code Block -->
            <pre class="p-4 sm:p-5 text-xs font-mono overflow-auto adaptive-preview-box leading-relaxed select-text text-base-content/90 max-w-full">{{ preview?.content || '暂无渲染配置内容。' }}</pre>
          </div>
        </div>

        <!-- Bottom Actions Bar -->
        <div class="flex items-center justify-between pt-1 text-xs opacity-70">
          <span>{{ t('publications.target') }}: {{ selectedTarget }}</span>
          <div class="flex items-center gap-2">
            <button
              type="button"
              class="btn btn-ghost btn-xs gap-1"
              :disabled="!preview?.content"
              @click="handleCopyContent"
            >
              <DocumentDuplicateIcon class="w-3.5 h-3.5" />
              {{ t('publications.copyFull') }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Publish Success / Token Modal (Bottom sheet on mobile) -->
    <ModalDialog
      v-model="publishModalOpen"
      title="订阅发布链接已生成"
      description="绑定目标编译格式的不可变订阅地址（含 SHA-256 令牌鉴权）"
    >
      <div v-if="activePublication" class="space-y-4">
        <div class="p-3.5 rounded-xl bg-success/10 border border-success/30 text-success text-xs flex items-center gap-2">
          <CheckBadgeIcon class="w-5 h-5 flex-shrink-0" />
          <span>不可变订阅发布已成功注册（审计事件：{{ auditEventLabel('publication.create') }}），严格拒绝过期配置回退。</span>
        </div>

        <div class="space-y-1.5">
          <label class="text-xs font-semibold">客户端订阅地址</label>
          <div class="flex flex-wrap gap-2 min-w-0">
            <input
              readonly
              :value="activePublication.export_url ? `${activePublication.export_url}` : ''"
              class="input input-bordered input-sm font-mono text-xs flex-1 min-w-0 bg-base-200"
            />
            <button
              type="button"
              class="btn btn-primary btn-sm gap-1"
              data-testid="modal-copy-subscription-url-btn"
              @click="copyPublicationURL"
            >
              <ClipboardDocumentIcon class="w-4 h-4" />
              {{ copiedToken ? '已复制！' : '复制订阅链接' }}
            </button>
          </div>
          <p class="text-[11px] opacity-60">
            客户端可通过 Bearer 请求头或 URL 查询参数 token 拉取此订阅配置。
          </p>
        </div>

        <div class="grid grid-cols-2 gap-2 text-xs font-mono p-3 rounded-xl bg-base-200 border border-base-300 min-w-0 break-all">
          <div>
            <span class="opacity-60 block">导出目标</span>
            <div class="flex items-center gap-1.5 mt-0.5">
              <strong class="uppercase text-primary">
                {{ activePublication.target }}
              </strong>
            </div>
          </div>
          <div>
            <span class="opacity-60 block">发布状态</span>
            <strong :class="activePublication.revoked_at ? 'text-error' : 'text-success'">
              {{ publicationStateLabel(activePublication.state, activePublication.revoked_at) }}
            </strong>
          </div>
        </div>

      </div>
      <template #footer>
        <div v-if="activePublication" class="flex flex-wrap items-center justify-between gap-2 w-full">
          <button
            v-if="!activePublication.revoked_at"
            type="button"
            class="btn btn-error btn-outline btn-sm gap-1"
            :disabled="revoking"
            :class="{ loading: revoking }"
            @click="handleRevoke"
          >
            <XCircleIcon class="w-4 h-4" />
            撤销发布
          </button>
          <div v-else />

          <button
            type="button"
            class="btn btn-ghost btn-sm"
            @click="publishModalOpen = false"
          >
            完成
          </button>
        </div>
      </template>
    </ModalDialog>

    <!-- Confirm Revoke Publication Modal -->
    <ConfirmModal
      v-model="confirmRevokeOpen"
      title="确认撤销发布"
      message="确定立即撤销该发布记录吗？撤销后状态将变为“已撤销”，客户端将无法继续下载此订阅配置。"
      confirm-text="立即撤销"
      cancel-text="保持生效"
      tone="danger"
      :loading="revoking"
      @confirm="confirmRevokePublication"
    />
  </section>
</template>
