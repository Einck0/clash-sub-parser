<script setup lang="ts">
import {
  ArrowDownTrayIcon,
  ClipboardDocumentCheckIcon,
  ClipboardDocumentIcon,
  CloudArrowUpIcon,
  EyeIcon,
} from '@heroicons/vue/24/outline'
import ModalDialog from '../../ui/ModalDialog.vue'
import PageToolbar from '../../ui/PageToolbar.vue'
import { ApiError } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'
import { displayText } from '../../utils/displayNames'
import {
  COMPILER_TARGETS,
  type CompilerTarget,
  targetLabel,
} from './publicationTypes'
import { usePublications } from './usePublications'

const {
  selectedTarget,
  preview,
  activePublication,
  loadingPreview,
  publishing,
  error,
  errorDetail,
  preflightDiagnostics,
  fetchPreview,
  publish,
  downloadFile,
  getFullExportUrl,
  copyToClipboard,
  getDirectUrl,
  copyDirectUrl,
  copiedTarget,
  manualCopyUrl,
  manualCopyModalOpen,
} = usePublications()

async function handlePreview(target: CompilerTarget) {
  selectedTarget.value = target
  await fetchPreview(target)
}

function canUsePreview(target: CompilerTarget): boolean {
  return Boolean(
    !loadingPreview.value
      && !error.value
      && preview.value
      && preview.value.target === target
      && preview.value.snapshot_id
      && preview.value.manifest,
  )
}

async function handlePublish() {
  if (!canUsePreview(selectedTarget.value) || !preview.value?.snapshot_id) return
  try {
    await publish(selectedTarget.value, preview.value.snapshot_id)
    toastStore.push({ message: uiText('已按当前预览创建发布', 'Publication created from the current preview'), tone: 'success' })
  } catch (err: any) {
    toastStore.push({ message: displayText(err?.message || uiText('发布失败', 'Publication failed')), tone: 'error' })
  }
}

function handlePreviewDownload(target: CompilerTarget) {
  if (!canUsePreview(target) || !preview.value) return
  downloadFile(preview.value.filename, preview.value.content, preview.value.content_type)
  toastStore.push({ message: uiText(`已下载 ${targetLabel(target)} 预览配置`, `Downloaded ${targetLabel(target)} preview`), tone: 'success' })
}

async function handleCopyPublication() {
  const url = getFullExportUrl()
  if (!url) return
  if (await copyToClipboard(url)) {
    toastStore.push({ message: uiText('已复制冻结发布链接', 'Frozen publication link copied'), tone: 'success' })
  } else {
    manualCopyUrl.value = url
    manualCopyModalOpen.value = true
  }
}

async function handleCopy(target: CompilerTarget) {
  const result = await copyDirectUrl(target)
  if (result.copied) {
    toastStore.push({ message: t('publications.copyExportUrl'), tone: 'success' })
  }
}

function handleDirectDownload(target: CompilerTarget) {
  if (!canUsePreview(target) || !preview.value) {
    toastStore.push({ message: uiText('请先生成当前目标的预览', 'Generate a preview for the current target first'), tone: 'error' })
    return
  }
  handlePreviewDownload(target)
}
</script>

<template>
  <section class="mx-auto w-full max-w-5xl space-y-5" aria-labelledby="publications-title" data-testid="client-subscriptions-page">
    <h2 id="publications-title" class="sr-only">{{ t('publications.title') }}</h2>

    <PageToolbar :title="t('publications.title')" :description="t('publications.subtitle')" sr-only-title>
      <template #default>
        <div class="flex items-center gap-2">
          <span class="text-xs font-semibold uppercase tracking-[0.16em] text-primary">{{ t('publications.tag') }}</span>
          <span class="text-xs text-base-content/60">{{ uiText('4 种客户端直链导出与配置下载', '4 Client Direct Links & Configuration Downloads') }}</span>
        </div>
      </template>
    </PageToolbar>

    <div class="rounded-2xl border border-base-300 bg-base-100 p-4 shadow-sm space-y-4" data-testid="publication-preview-panel">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 class="font-semibold">{{ uiText('导出预览', 'Export preview') }}</h3>
          <p class="text-xs text-base-content/60">{{ uiText('先确认目标，再发布或下载同一份冻结快照', 'Confirm the target before publishing or downloading the same frozen snapshot') }}</p>
        </div>
      </div>
      <div class="flex flex-wrap items-center gap-3 text-xs font-mono" data-testid="preview-context">
        <span>{{ uiText('目标客户端', 'Client') }}：{{ targetLabel(selectedTarget) }}</span>
        <span v-if="preview?.snapshot_id">{{ uiText('冻结快照已就绪', 'Frozen snapshot ready') }}</span>
        <span v-if="preview?.manifest">{{ uiText('已排除', 'Excluded') }} {{ preview.manifest.excluded_count }}</span>
      </div>
      <div v-if="error" class="alert alert-error text-sm" data-testid="preview-error">
        <div>
          <p class="font-semibold">{{ displayText(errorDetail instanceof Error ? errorDetail.message : error) }}</p>
          <p v-if="errorDetail instanceof ApiError && errorDetail.requestId" class="mt-1 break-all font-mono text-xs" data-testid="preview-error-request-id">
            {{ uiText('请求 ID', 'Request ID') }}：{{ errorDetail.requestId }}
          </p>
          <ul v-if="preflightDiagnostics.length" class="mt-2 list-disc pl-5 text-xs">
            <li v-for="(diagnostic, index) in preflightDiagnostics" :key="`${diagnostic.code}-${index}`">{{ diagnostic.target ? `${targetLabel(diagnostic.target as CompilerTarget)}: ` : '' }}{{ displayText(diagnostic.message) }}</li>
          </ul>
        </div>
      </div>
      <div v-if="preview?.manifest?.excluded?.length" class="rounded-xl border border-warning/40 bg-warning/10 p-3 text-sm" data-testid="preview-exclusions">
        <p class="font-semibold">{{ uiText(`已跳过 ${preview.manifest.excluded_count} 个节点`, `${preview.manifest.excluded_count} node(s) excluded`) }}</p>
        <ul class="mt-2 space-y-1 text-xs">
          <li v-for="(item, index) in preview.manifest.excluded" :key="`${item.node_id}-${item.code}`">
            {{ uiText('已排除节点', 'Excluded node') }} {{ index + 1 }} · {{ displayText(item.reason) || uiText('未提供排除原因', 'No reason supplied') }} · {{ targetLabel(preview.target) }}
          </li>
        </ul>
      </div>
      <div v-if="canUsePreview(selectedTarget)" class="flex flex-wrap gap-2" data-testid="preview-actions">
        <button type="button" class="btn btn-primary btn-sm gap-1.5" data-testid="publish-preview-btn" :disabled="loadingPreview || publishing" @click="handlePublish">
          <CloudArrowUpIcon class="h-4 w-4" />{{ uiText('发布当前预览', 'Publish preview') }}
        </button>
        <button type="button" class="btn btn-outline btn-sm gap-1.5" data-testid="download-preview-btn" @click="handlePreviewDownload(selectedTarget)">
          <ArrowDownTrayIcon class="h-4 w-4" />{{ uiText('下载当前预览', 'Download preview') }}
        </button>
      </div>
    </div>

    <div v-if="getFullExportUrl()" class="rounded-2xl border border-success/40 bg-base-100 p-4 space-y-3" data-testid="frozen-publication-result">
      <h3 class="font-semibold">{{ uiText('冻结发布链接', 'Frozen publication link') }}</h3>
      <p class="text-xs text-base-content/60">{{ uiText('此链接始终下载已发布的快照；下方客户端订阅直链随库存更新', 'This link downloads the published snapshot; client subscription links below follow inventory updates') }}</p>
      <p class="text-xs font-mono">target={{ activePublication?.target }}</p>
      <div class="flex flex-wrap gap-2">
        <input type="text" readonly :value="getFullExportUrl()" :aria-label="uiText('冻结发布链接', 'Frozen publication link')" class="input input-sm min-w-0 flex-1 font-mono text-xs select-all" data-testid="frozen-publication-url" @focus="($event.target as HTMLInputElement).select()" />
        <button type="button" class="btn btn-outline btn-sm" data-testid="copy-frozen-publication-btn" @click="handleCopyPublication">{{ uiText('复制冻结链接', 'Copy frozen link') }}</button>
      </div>
    </div>

    <div class="divide-y divide-base-300 rounded-2xl border border-base-300 bg-base-100 shadow-sm" data-testid="client-subscription-list">
      <article
        v-for="item in COMPILER_TARGETS"
        :key="item.target"
        class="flex flex-col gap-4 p-4 sm:flex-row sm:items-center sm:justify-between sm:p-5"
        :data-target="item.target"
        :data-testid="`client-subscription-row-${item.target}`"
      >
        <div class="min-w-0 flex-1 space-y-2">
          <div class="flex flex-wrap items-center gap-2">
            <h3 class="font-semibold text-base">{{ item.label }}</h3>
            <span class="badge badge-sm badge-ghost font-mono">.{{ item.ext }}</span>
            <span class="badge badge-sm font-medium" :class="item.target === 'mihomo' ? 'badge-primary badge-outline' : 'badge-ghost'">
              {{ item.target === 'mihomo' ? t('publications.fullConfig') : t('publications.nodesOnly') }}
            </span>
          </div>
          <p class="text-sm opacity-70">{{ item.desc }}</p>
          <div class="flex items-center gap-2">
            <input type="text" readonly :value="getDirectUrl(item.target)" class="input input-xs w-full max-w-md font-mono text-xs select-all bg-base-200/60" :data-testid="`direct-url-input-${item.target}`" @focus="($event.target as HTMLInputElement).select()" />
          </div>
          <div class="flex flex-wrap items-center gap-2 text-xs font-mono text-base-content/60">
            <span>target={{ item.target }}</span>
            <span v-if="preview?.target === item.target">{{ uiText('使用当前冻结快照', 'Using current frozen snapshot') }}</span>
            <span v-else>{{ uiText('尚未生成当前目标预览', 'No preview for the current target') }}</span>
          </div>
          <p class="text-[11px] text-base-content/50 font-mono">
            {{ item.target === 'mihomo' ? uiText('✓ 包含完整分流规则与策略组', '✓ Includes full rules and policy groups') : uiText('ℹ 仅导出节点格式（忽略策略组与规则）', 'ℹ Nodes-only export (ignores groups and rules)') }}
          </p>
        </div>

        <div class="flex shrink-0 flex-wrap items-center gap-2">
          <button type="button" class="btn btn-primary btn-sm min-w-32 gap-1.5 touch-manipulation" data-testid="copy-client-subscription-btn" :data-target="item.target" :data-testid-target="`copy-client-subscription-${item.target}`" @click="handleCopy(item.target)">
            <ClipboardDocumentCheckIcon v-if="copiedTarget === item.target" class="h-4 w-4" />
            <ClipboardDocumentIcon v-else class="h-4 w-4" />
            <span>{{ copiedTarget === item.target ? '已复制！' : '复制订阅链接' }}</span>
          </button>
          <button type="button" class="btn btn-outline btn-sm gap-1 text-xs" :disabled="!canUsePreview(item.target)" data-testid="download-client-subscription-btn" :data-target="item.target" :data-testid-target="`download-client-subscription-${item.target}`" @click="handleDirectDownload(item.target)">
            <ArrowDownTrayIcon class="h-3.5 w-3.5" />
            <span>{{ uiText('下载当前预览', 'Download preview') }}</span>
          </button>
          <button type="button" class="btn btn-ghost btn-sm gap-1.5" :disabled="loadingPreview" :data-testid="`preview-client-subscription-${item.target}`" @click="handlePreview(item.target)">
            <EyeIcon class="h-4 w-4" />{{ loadingPreview && selectedTarget === item.target ? uiText('预览中...', 'Previewing...') : uiText('预览', 'Preview') }}
          </button>
          <button type="button" class="btn btn-outline btn-sm gap-1 text-xs" :disabled="!canUsePreview(item.target)" :data-testid="`download-preview-${item.target}`" @click="handlePreviewDownload(item.target)">
            <ArrowDownTrayIcon class="h-3.5 w-3.5" />{{ uiText('下载预览', 'Download preview') }}
          </button>
        </div>
      </article>
    </div>

    <ModalDialog v-model="manualCopyModalOpen" title="手动复制订阅链接" description="浏览器剪贴板权限受限或非安全上下文，请在下方全选复制订阅链接：">
      <div class="space-y-3">
        <textarea readonly rows="3" :value="manualCopyUrl" data-testid="manual-copy-textarea" class="textarea w-full font-mono text-xs select-all bg-base-200" @focus="($event.target as HTMLTextAreaElement).select()" />
        <p class="text-[11px] opacity-70">请复制上方完整地址后粘贴至客户端（Clash / Sing-box / Surge / Quantumult X）中。</p>
      </div>
      <template #footer>
        <button type="button" class="btn btn-primary btn-sm" @click="manualCopyModalOpen = false">完成</button>
      </template>
    </ModalDialog>
  </section>
</template>
