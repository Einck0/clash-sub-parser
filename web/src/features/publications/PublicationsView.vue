<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ArrowDownTrayIcon, ClipboardDocumentCheckIcon, ClipboardDocumentIcon, CloudArrowUpIcon, EyeIcon } from '@heroicons/vue/24/outline'
import ModalDialog from '../../ui/ModalDialog.vue'
import ConfirmModal from '../../ui/ConfirmModal.vue'
import PageToolbar from '../../ui/PageToolbar.vue'
import { fetchAuthSettings } from '../auth/authSettings'
import { ApiError } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'
import { displayName, displayText } from '../../utils/displayNames'
import { COMPILER_TARGETS, type CompilerTarget, type ExcludedNode, targetLabel } from './publicationTypes'
import { publicationSession, usePublications } from './usePublications'

const {
  selectedTarget, preview, activePublication, loadingPreview, publishing, revoking,
  error, errorDetail, preflightDiagnostics, fetchPreview, publish, revoke,
  downloadFile, getFullExportUrl, copyToClipboard, getDirectUrl, copyDirectUrl,
  copiedTarget, manualCopyUrl, manualCopyModalOpen,
} = usePublications()
const exportMode = ref<'open' | 'protected' | null>(null)
const exportModeError = ref('')
const loadingExportMode = ref(false)
const exportModeSession = ref(-1)
const liveUrls = computed(() => {
  const confirmed = !loadingExportMode.value && exportModeSession.value === publicationSession.value
  return Object.fromEntries(COMPILER_TARGETS.map(item => [item.target,
    confirmed && exportMode.value ? getDirectUrl(item.target, { mode: exportMode.value }) : '',
  ])) as Record<CompilerTarget, string>
})
async function loadExportMode() {
  if (loadingExportMode.value) return
  loadingExportMode.value = true
  exportModeError.value = ''
  const session = publicationSession.value
  try {
    const settings = await fetchAuthSettings()
    if (!settings || !['open', 'protected'].includes(settings.export_mode)) throw new Error(uiText('无法确认订阅访问模式', 'Unable to confirm export access mode'))
    if (session === publicationSession.value) {
      exportMode.value = settings.export_mode
      exportModeSession.value = session
    }
  } catch (err) {
    if (session === publicationSession.value) {
      exportMode.value = null
      exportModeError.value = displayText(err instanceof Error ? err.message : uiText('读取订阅访问模式失败', 'Failed to read export access mode'))
    }
  } finally { loadingExportMode.value = false }
}
onMounted(() => { void loadExportMode() })
const exclusionSearch = ref('')
const revokeOpen = ref(false)
const revokeError = ref('')
const generatedAt = ref('')

function exclusionLabel(item: ExcludedNode): string {
  const id = /^[a-zA-Z0-9_-]{1,128}$/.test(item.node_id) ? item.node_id : ''
  const fallback = id.length > 24 ? `${id.slice(0,12)}…${id.slice(-6)}` : id
  return displayName(item.display_name, fallback || uiText('名称不可用', 'Name unavailable'), item.node_id)
}
const filteredExclusions = computed(() => {
  const q = exclusionSearch.value.trim().toLowerCase()
  return (preview.value?.manifest?.excluded || []).filter(item =>
    !q || `${exclusionLabel(item)} ${item.code || ''} ${item.reason || ''}`.toLowerCase().includes(q))
})
const ready = computed(() => Boolean(!loadingPreview.value && !error.value &&
  preview.value?.target === selectedTarget.value && preview.value?.snapshot_id && preview.value?.manifest))

async function handlePreview(target: CompilerTarget) {
  selectedTarget.value = target
  exclusionSearch.value = ''
  const result = await fetchPreview(target)
  if (result && preview.value?.snapshot_id === result.snapshot_id && selectedTarget.value === target) generatedAt.value = new Date().toLocaleTimeString()
}
async function handlePublish() {
  if (!ready.value || publishing.value || !preview.value?.snapshot_id) return
  try {
    await publish(selectedTarget.value, preview.value.snapshot_id)
    toastStore.push({ message: uiText('已按当前预览创建冻结发布', 'Frozen publication created from this preview'), tone: 'success' })
  } catch { /* The persistent error region contains the server diagnostics. */ }
}
function handlePreviewDownload() {
  if (!ready.value || !preview.value) return
  try {
    downloadFile(preview.value.filename, preview.value.content, preview.value.content_type)
    toastStore.push({ message: uiText('已开始下载当前预览', 'Download started for this preview'), tone: 'success' })
  } catch (err) {
    toastStore.push({ message: displayText(err instanceof Error ? err.message : uiText('下载失败，请重试', 'Download failed; retry')), tone: 'error' })
  }
}
async function handleCopyPublication() {
  const url = getFullExportUrl()
  if (!url) return
  if (await copyToClipboard(url)) toastStore.push({ message: uiText('已复制冻结发布链接', 'Frozen publication link copied'), tone: 'success' })
  else { manualCopyUrl.value = url; manualCopyModalOpen.value = true }
}
async function handleCopy(target: CompilerTarget) {
  if (!exportMode.value || !liveUrls.value[target]) return
  if ((await copyDirectUrl(target, { mode: exportMode.value })).copied) toastStore.push({ message: t('publications.copyExportUrl'), tone: 'success' })
}
async function handleRevoke() {
  if (!activePublication.value || revoking.value) return
  try { await revoke(activePublication.value.id); revokeOpen.value = false; revokeError.value = '' }
  catch (err) { revokeError.value = displayText(err instanceof Error ? err.message : uiText('撤销失败，请重试', 'Revocation failed; retry')) }
}
</script>

<template>
  <section class="mx-auto w-full max-w-5xl space-y-6" data-testid="client-subscriptions-page">
    <PageToolbar :title="t('publications.title')" :description="uiText('实时订阅随库存更新；冻结发布固定为已确认的预览。先选客户端，再核对导出结果。', 'Live subscriptions follow inventory changes; frozen publications keep a confirmed preview. Choose a client and review the result.')">
      <template #actions>
        <button class="btn btn-primary btn-sm gap-2" :disabled="loadingPreview || publishing" data-testid="generate-preview-btn" @click="handlePreview(selectedTarget)">
          <span v-if="loadingPreview" class="loading loading-spinner loading-xs" /><EyeIcon v-else class="h-4 w-4" />
          {{ loadingPreview ? uiText('生成预览中…', 'Generating preview…') : uiText('生成 / 刷新预览', 'Generate / refresh preview') }}
        </button>
      </template>
    </PageToolbar>

    <section class="rounded-2xl border border-base-300 bg-base-100 p-4 sm:p-5 space-y-4" data-testid="publication-preview-panel" :aria-busy="loadingPreview">
      <div class="flex flex-wrap items-end justify-between gap-3">
        <div class="space-y-1">
          <h2 class="font-semibold">{{ uiText('核对预览，再下载或冻结发布', 'Review, download or freeze') }}</h2>
          <p class="text-sm text-base-content/70">{{ uiText('下载和发布使用同一份预览；后续库存变化不会更新它。需要最新内容时重新生成。', 'Download and publish use the same preview. Inventory changes do not update it; generate again for the latest content.') }}</p>
        </div>
        <label class="form-control w-full sm:w-auto text-xs gap-1">
          <span>{{ uiText('目标客户端', 'Client') }}</span>
          <select v-model="selectedTarget" class="select select-sm" :disabled="publishing" data-testid="preview-target-select">
            <option v-for="item in COMPILER_TARGETS" :key="item.target" :value="item.target">{{ item.label }}</option>
          </select>
        </label>
      </div>
      <div role="status" class="flex flex-wrap gap-3 text-sm" data-testid="preview-context">
        <span v-if="loadingPreview">{{ uiText('正在按当前库存计算，旧预览暂不可操作…', 'Calculating current inventory; previous preview actions are unavailable…') }}</span>
        <template v-else-if="ready && preview?.manifest">
          <span class="badge badge-success badge-outline">{{ uiText('预览已就绪', 'Preview ready') }}</span>
          <span>{{ targetLabel(preview.target) }} · {{ generatedAt }}</span>
          <span>{{ uiText('可导出', 'Included') }} {{ preview.manifest.node_count }}</span>
          <span>{{ uiText('已排除', 'Excluded') }} {{ preview.manifest.excluded_count }}</span>
        </template>
        <span v-else-if="!error" class="text-base-content/70">{{ uiText('尚未生成当前客户端预览。点击上方“生成 / 刷新预览”开始。', 'No preview for this client. Use Generate / refresh preview above to start.') }}</span>
      </div>
      <div v-if="error" role="alert" class="alert alert-error text-sm break-words" data-testid="preview-error">
        <div class="min-w-0">
          <p class="font-semibold">{{ displayText(errorDetail instanceof Error ? errorDetail.message : error) }}</p>
          <p v-if="errorDetail instanceof ApiError && errorDetail.requestId" class="mt-1 break-all font-mono text-xs" data-testid="preview-error-request-id">{{ uiText('请求 ID', 'Request ID') }}：{{ errorDetail.requestId }}</p>
          <ul v-if="preflightDiagnostics.length" class="mt-2 list-disc pl-5 text-xs space-y-1">
            <li v-for="(diagnostic, index) in preflightDiagnostics" :key="index">{{ displayText(diagnostic.message) }}</li>
          </ul>
          <p class="mt-2 text-xs">{{ uiText('未生成可用预览，不会下载错误文件或发布旧结果。修复配置后重新生成。', 'No usable preview: no error file will be downloaded and no old result published. Correct the configuration and regenerate.') }}</p>
        </div>
      </div>
      <div v-if="ready && preview?.manifest?.excluded?.length" class="rounded-xl border border-warning/40 bg-warning/10 p-3 space-y-3" data-testid="preview-exclusions">
        <h3 class="font-semibold text-sm">{{ uiText(`已跳过 ${preview.manifest.excluded_count} 个节点`, `${preview.manifest.excluded_count} node(s) excluded`) }}</h3>
        <p class="text-xs">{{ uiText('以下节点不会进入这份配置。按节点名称和原因核对，避免误以为库存丢失。', 'These nodes are not in this configuration. Review their names and reasons; they have not been deleted from inventory.') }}</p>
        <input v-model="exclusionSearch" type="search" class="input input-sm w-full" :aria-label="uiText('查找排除节点或原因', 'Find excluded node or reason')" :placeholder="uiText('查找节点名称、原因…', 'Find node or reason…')" data-testid="exclusion-search" />
        <ul class="space-y-2 max-h-64 overflow-y-auto text-xs">
          <li v-for="item in filteredExclusions" :key="`${item.node_id}-${item.code}`" class="break-words border-t border-warning/20 pt-2">
            <span class="font-semibold">{{ exclusionLabel(item) }}</span> · {{ displayText(item.reason) || uiText('未提供排除原因', 'No reason supplied') }} · {{ targetLabel(preview.target) }}
          </li>
        </ul>
        <p v-if="!filteredExclusions.length" class="text-xs" role="status">{{ uiText('没有匹配的排除项', 'No matching exclusions') }}</p>
      </div>
      <div v-if="ready" class="flex flex-wrap gap-2" data-testid="preview-actions">
        <button class="btn btn-outline btn-sm gap-2" data-testid="download-preview-btn" :disabled="publishing" @click="handlePreviewDownload"><ArrowDownTrayIcon class="h-4 w-4" />{{ uiText('下载当前预览', 'Download preview') }}</button>
        <button class="btn btn-outline btn-sm gap-2" data-testid="publish-preview-btn" :disabled="publishing" @click="handlePublish"><span v-if="publishing" class="loading loading-spinner loading-xs" /><CloudArrowUpIcon v-else class="h-4 w-4" />{{ uiText('创建冻结链接', 'Create frozen link') }}</button>
      </div>
    </section>

    <section v-if="getFullExportUrl()" class="rounded-2xl border border-success/40 bg-base-100 p-4 space-y-3" data-testid="frozen-publication-result">
      <h2 class="font-semibold">{{ uiText('冻结发布链接', 'Frozen publication link') }} · {{ activePublication ? targetLabel(activePublication.target) : '' }}</h2>
      <p class="text-sm text-base-content/70">{{ uiText('固定下载已发布的快照，不随库存变化。链接含访问凭据，请仅交给可信客户端。', 'Downloads the published snapshot without inventory updates. This link contains access credentials; share only with trusted clients.') }}</p>
      <div class="flex flex-wrap gap-2">
        <input readonly :value="getFullExportUrl()" :aria-label="uiText('冻结发布链接', 'Frozen publication link')" class="input input-sm min-w-0 w-full sm:flex-1 font-mono text-xs select-all" data-testid="frozen-publication-url" @focus="($event.target as HTMLInputElement).select()" />
        <button class="btn btn-outline btn-sm" data-testid="copy-frozen-publication-btn" @click="handleCopyPublication">{{ uiText('复制冻结链接', 'Copy frozen link') }}</button>
        <button class="btn btn-ghost btn-sm text-error" @click="revokeError = ''; revokeOpen = true">{{ uiText('撤销此链接', 'Revoke link') }}</button>
      </div>
    </section>

    <section class="space-y-3">
      <h2 class="font-semibold">{{ uiText('实时客户端订阅', 'Live client subscriptions') }}</h2>
      <p class="text-sm text-base-content/70">{{ uiText('复制到客户端后，每次拉取都使用最新已保存的库存、DNS 和策略配置，与上方冻结预览独立。带凭据的链接请勿公开。', 'Each client refresh uses the latest saved inventory, DNS and policy configuration, independently of the frozen preview above. Keep credential-bearing links private.') }}</p>
      <div v-if="!liveUrls.mihomo" class="alert text-sm flex flex-wrap gap-2" role="status" data-testid="live-link-unavailable">
        <span class="min-w-0 whitespace-pre-wrap [overflow-wrap:anywhere]">{{ loadingExportMode ? uiText('正在确认订阅访问模式…', 'Checking export access mode…') : exportModeError || uiText('当前无法提供可用的实时链接。受保护的订阅需要当前会话令牌；请重新登录后重试，不能把缺少凭据的地址当作公开订阅。', 'A usable live link is unavailable. Protected exports need the current session token; sign in again and retry. A URL without credentials is not a public subscription.') }}</span>
        <button type="button" class="btn btn-sm btn-outline" :disabled="loadingExportMode" @click="loadExportMode">{{ t('common.retry') }}</button>
      </div>
      <div class="divide-y divide-base-300 rounded-2xl border border-base-300 bg-base-100" data-testid="client-subscription-list">
        <article v-for="item in COMPILER_TARGETS" :key="item.target" class="flex flex-col gap-3 p-4 lg:flex-row lg:items-center lg:justify-between" :data-target="item.target" :data-testid="`client-subscription-row-${item.target}`">
          <div class="min-w-0 flex-1 space-y-2">
            <div class="flex flex-wrap gap-2 items-center"><h3 class="font-semibold">{{ item.label }}</h3><span class="badge badge-sm badge-ghost">.{{ item.ext }}</span><span class="badge badge-sm badge-outline">{{ item.target === 'mihomo' ? t('publications.fullConfig') : t('publications.nodesOnly') }}</span></div>
            <p class="text-xs text-base-content/75">{{ item.target === 'mihomo' ? uiText('包含节点、策略组、DNS 与分流规则', 'Includes nodes, policy groups, DNS and routing rules') : uiText('仅节点格式，不包含策略组与分流规则；不支持的节点将在预览中说明', 'Nodes only, without groups or routing rules; review unsupported nodes in preview') }}</p>
            <input readonly :value="liveUrls[item.target]" :aria-label="uiText(`${item.label} 实时订阅链接`, `${item.label} live subscription URL`)" class="input input-sm w-full font-mono text-xs select-all bg-base-200/60" :data-testid="`direct-url-input-${item.target}`" @focus="($event.target as HTMLInputElement).select()" />
          </div>
          <div class="flex shrink-0 flex-wrap gap-2">
            <button class="btn btn-outline btn-sm gap-2" data-testid="copy-client-subscription-btn" :data-target="item.target" :data-testid-target="`copy-client-subscription-${item.target}`" :disabled="!liveUrls[item.target]" @click="handleCopy(item.target)"><ClipboardDocumentCheckIcon v-if="copiedTarget === item.target" class="h-4 w-4" /><ClipboardDocumentIcon v-else class="h-4 w-4" /><span aria-live="polite">{{ copiedTarget === item.target ? uiText('已复制！', 'Copied!') : uiText('复制实时链接', 'Copy live link') }}</span></button>
            <button class="btn btn-ghost btn-sm" :disabled="loadingPreview || publishing" :data-testid="`preview-client-subscription-${item.target}`" @click="handlePreview(item.target)">{{ uiText('核对预览', 'Review preview') }}</button>
          </div>
        </article>
      </div>
    </section>
    <ConfirmModal v-model="revokeOpen" :title="uiText('撤销冻结链接？', 'Revoke frozen link?')" :message="revokeError || uiText('此链接将立即失效，已下载的文件不受影响。实时订阅链接不变。', 'This link will stop working; downloaded files and live subscriptions are unchanged.')" tone="danger" :loading="revoking" @confirm="handleRevoke" />
    <ModalDialog v-model="manualCopyModalOpen" :title="uiText('手动复制订阅链接', 'Copy subscription link manually')" :description="uiText('浏览器无法访问剪贴板，请全选并复制完整链接。', 'Clipboard access is unavailable. Select and copy the complete link.')">
      <textarea readonly rows="3" :value="manualCopyUrl" :aria-label="uiText('订阅链接', 'Subscription URL')" data-testid="manual-copy-textarea" class="textarea w-full font-mono text-xs select-all bg-base-200" @focus="($event.target as HTMLTextAreaElement).select()" />
      <template #footer><button class="btn btn-primary btn-sm" @click="manualCopyModalOpen = false">{{ uiText('完成', 'Done') }}</button></template>
    </ModalDialog>
  </section>
</template>
