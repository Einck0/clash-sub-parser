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
  PencilSquareIcon,
  KeyIcon,
  GlobeAltIcon,
  CheckCircleIcon,
} from '@heroicons/vue/24/outline'
import { usePublications, clearStoredPublication } from './usePublications'
import { fetchAuthSettings } from '../auth/authSettings'
import { toastStore } from '../../ui/toast'
import {
  COMPILER_TARGETS,
  auditEventLabel,
  formatDigest,
  getTargetMetadata,
  groupTypeLabel,
  preflightCheckLabel,
  publicationStateLabel,
  subscriptionLinkStateLabel,
  formatDateTime,
  targetFileExt,
  type CompilerTarget,
  type SubscriptionLink,
  type SubscriptionLinkCheckResult,
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
  compatMode,
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
  fetchPublication,
  publish,
  revoke,
  copyToClipboard,
  downloadFile,
  restoreActivePublication,
  getFullExportUrl,
  // Client subscription links
  subscriptionLinks,
  loadingLinks,
  copyingLink,
  rotatingLinkId,
  revokingLinkId,
  checkingLinkId,
  updatingLinkId,
  linkCheckResults,
  publicBaseUrl,
  publicBaseUrlMissing,
  savingPublicBaseUrl,
  generationWarning,
  manualCopyUrl,
  manualCopyModalOpen,
  linkError,
  defaultLinkForSelectedTarget,
  fetchSubscriptionLinks,
  fetchPublicBaseUrl,
  savePublicBaseUrl,
  ensureDefaultSubscriptionLink,
  copySubscriptionLink,
  copyDefaultClientSubscription,
  rotateSubscriptionToken,
  revokeSubscriptionLink,
  updateSubscriptionLinkLabel,
  checkSubscriptionLink,
} = usePublications()

const publishModalOpen = ref(false)
const copiedToken = ref(false)
const exportProtected = ref<boolean | null>(null)
const authSettingsError = ref(false)
const verifyingPublication = ref(false)
const publicationLookupFailed = ref(false)
const suppliedToken = ref('')
const copyUrl = computed(() => exportProtected.value === null ? '' : getFullExportUrl(activePublication.value, exportProtected.value, suppliedToken.value))

// Client subscription view state
const copiedClientSub = ref(false)
const inputPublicBaseUrl = ref('')
const rotateConfirmModalOpen = ref(false)
const targetLinkToRotate = ref<SubscriptionLink | null>(null)
const revokeConfirmModalOpen = ref(false)
const targetLinkToRevoke = ref<SubscriptionLink | null>(null)
const editLabelModalOpen = ref(false)
const targetLinkToEdit = ref<SubscriptionLink | null>(null)
const inputLinkLabel = ref('')
const checkDetailModalOpen = ref(false)
const activeCheckResult = ref<SubscriptionLinkCheckResult | null>(null)
const showAdvancedOptions = ref(true)

async function refreshExportMode() {
  exportProtected.value = null
  authSettingsError.value = false
  try {
    const settings = await fetchAuthSettings()
    if (typeof settings?.export_auth_enabled !== 'boolean') throw new Error('Invalid export auth settings')
    exportProtected.value = settings.export_auth_enabled
  } catch {
    authSettingsError.value = true
  }
}

async function verifySavedPublication() {
  const saved = activePublication.value
  if (!saved) return
  verifyingPublication.value = true
  publicationLookupFailed.value = false
  const detail = await fetchPublication(saved.id)
  if (selectedTarget.value === saved.target && activePublication.value?.id === saved.id) {
    if (!detail) publicationLookupFailed.value = true
    else if (detail.state === 'revoked' || detail.revoked_at) {
      clearStoredPublication(saved.target)
      activePublication.value = null
    }
  }
  verifyingPublication.value = false
}

const currentTargetMeta = computed(() => getTargetMetadata(selectedTarget.value))

async function switchTarget(target: CompilerTarget) {
  selectedTarget.value = target
  restoreActivePublication(target)
  suppliedToken.value = ''
  void verifySavedPublication()
  await fetchPreview(target, undefined, compatMode.value)
}

async function switchCompatMode(mode: 'strict' | 'compatible') {
  if (compatMode.value === mode) return
  compatMode.value = mode
  await fetchPreview(selectedTarget.value, undefined, mode)
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
  const snapshotId = preview.value?.snapshot_id
  try {
    await publish(target, snapshotId)
    if (selectedTarget.value === target) {
      publicationLookupFailed.value = false
      publishModalOpen.value = true
    }
  } catch {
    // handled in composable
  }
}

async function copyPublicationURL() {
  const fullUrl = copyUrl.value
  if (!fullUrl || verifyingPublication.value || publicationLookupFailed.value) return
  if (!await copyToClipboard(fullUrl)) {
    toastStore.push({ message: t('publications.copyFailed'), tone: 'error' })
    return
  }
  toastStore.push({ message: t(exportProtected.value ? 'publications.copyProtectedSuccess' : 'publications.copyPublicSuccess'), tone: 'success' })
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

// Client subscription actions
async function handleCopyDefaultClientSub() {
  try {
    const res = await copyDefaultClientSubscription(selectedTarget.value)
    if (res.copied) {
      copiedClientSub.value = true
      setTimeout(() => {
        copiedClientSub.value = false
      }, 2000)
      if (res.warning) {
        toastStore.push({ message: `链接已复制：${res.warning}`, tone: 'warning' })
      } else {
        toastStore.push({ message: '客户端订阅链接已复制到剪贴板', tone: 'success' })
      }
    } else if (res.url) {
      manualCopyUrl.value = res.url
      manualCopyModalOpen.value = true
    }
  } catch (err: any) {
    if (publicBaseUrlMissing.value) {
      inputPublicBaseUrl.value = publicBaseUrl.value || ''
      toastStore.push({ message: '请先配置可信的公网访问基准地址', tone: 'warning' })
    } else {
      toastStore.push({ message: err?.message || '复制订阅链接失败', tone: 'error' })
    }
  }
}

async function handleCheckClientSub(linkId?: string) {
  const targetId = linkId || defaultLinkForSelectedTarget.value?.id
  if (!targetId) {
    try {
      const ensured = await ensureDefaultSubscriptionLink(selectedTarget.value)
      const check = await checkSubscriptionLink(ensured.id)
      activeCheckResult.value = check
      checkDetailModalOpen.value = true
    } catch (err: any) {
      toastStore.push({ message: err?.message || '检查失败', tone: 'error' })
    }
    return
  }

  try {
    const check = await checkSubscriptionLink(targetId)
    activeCheckResult.value = check
    checkDetailModalOpen.value = true
  } catch (err: any) {
    toastStore.push({ message: err?.message || '检查失败', tone: 'error' })
  }
}

async function handleCopySpecificLink(link: SubscriptionLink) {
  try {
    const res = await copySubscriptionLink(link.id, link.target)
    if (res.copied) {
      if (res.warning) {
        toastStore.push({ message: `链接已复制：${res.warning}`, tone: 'warning' })
      } else {
        toastStore.push({ message: `${link.label || link.target} 订阅链接已复制`, tone: 'success' })
      }
    } else if (res.url) {
      manualCopyUrl.value = res.url
      manualCopyModalOpen.value = true
    }
  } catch (err: any) {
    if (publicBaseUrlMissing.value) {
      inputPublicBaseUrl.value = publicBaseUrl.value || ''
      toastStore.push({ message: '请先配置可信的公网访问基准地址', tone: 'warning' })
    } else {
      toastStore.push({ message: err?.message || '复制链接失败', tone: 'error' })
    }
  }
}

function suggestCurrentOrigin() {
  if (typeof window !== 'undefined' && window.location?.origin) {
    inputPublicBaseUrl.value = window.location.origin
  }
}

async function handleSavePublicBaseUrl() {
  if (!inputPublicBaseUrl.value.trim()) return
  try {
    await savePublicBaseUrl(inputPublicBaseUrl.value)
    toastStore.push({ message: '公网访问基准地址已更新', tone: 'success' })
  } catch (err: any) {
    toastStore.push({ message: err?.message || '保存公网地址失败', tone: 'error' })
  }
}

function openRotateModal(link: SubscriptionLink) {
  targetLinkToRotate.value = link
  rotateConfirmModalOpen.value = true
}

async function confirmRotateToken() {
  if (!targetLinkToRotate.value?.id) return
  try {
    await rotateSubscriptionToken(targetLinkToRotate.value.id)
    rotateConfirmModalOpen.value = false
    toastStore.push({ message: '订阅访问令牌已轮换，旧链接已失效', tone: 'success' })
  } catch (err: any) {
    toastStore.push({ message: err?.message || '轮换失败', tone: 'error' })
  }
}

function openRevokeModal(link: SubscriptionLink) {
  targetLinkToRevoke.value = link
  revokeConfirmModalOpen.value = true
}

async function confirmRevokeLink() {
  if (!targetLinkToRevoke.value?.id) return
  try {
    await revokeSubscriptionLink(targetLinkToRevoke.value.id)
    revokeConfirmModalOpen.value = false
    toastStore.push({ message: '订阅链接已停用', tone: 'success' })
  } catch (err: any) {
    toastStore.push({ message: err?.message || '停用失败', tone: 'error' })
  }
}

function openEditLabelModal(link: SubscriptionLink) {
  targetLinkToEdit.value = link
  inputLinkLabel.value = link.label || ''
  editLabelModalOpen.value = true
}

async function confirmSaveLabel() {
  if (!targetLinkToEdit.value?.id) return
  try {
    await updateSubscriptionLinkLabel(targetLinkToEdit.value.id, inputLinkLabel.value.trim())
    editLabelModalOpen.value = false
    toastStore.push({ message: '订阅名称已更新', tone: 'success' })
  } catch (err: any) {
    toastStore.push({ message: err?.message || '修改名称失败', tone: 'error' })
  }
}

onMounted(() => {
  restoreActivePublication(selectedTarget.value)
  void verifySavedPublication()
  void refreshExportMode()
  fetchPreview(selectedTarget.value)
  void fetchSubscriptionLinks()
  void fetchPublicBaseUrl()
})
</script>

<template>
  <section class="space-y-6" aria-labelledby="publications-title">
    <!-- Header -->
    <div class="flex flex-col sm:flex-row sm:items-end justify-between gap-3 w-full min-w-0">
      <div class="min-w-0">
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-primary">客户端订阅发布</p>
        <h2 id="publications-title" class="mt-1 text-2xl font-bold truncate">{{ t('publications.title') }}</h2>
        <p class="mt-1 text-sm opacity-70">
          客户端动态订阅（推荐）与多内核目标不可变配置发布
        </p>
      </div>

      <div class="flex flex-wrap items-center gap-2 shrink-0">
        <button
          type="button"
          class="btn btn-ghost btn-sm btn-square"
          :title="t('common.refresh')"
          :disabled="loadingPreview || loadingLinks"
          @click="() => { fetchPreview(selectedTarget); fetchSubscriptionLinks(); fetchPublicBaseUrl(); }"
        >
          <ArrowPathIcon class="w-4 h-4" :class="{ 'animate-spin': loadingPreview || loadingLinks }" />
        </button>

        <button
          v-if="activePublication && !activePublication.revoked_at && activePublication.export_url"
          type="button"
          class="btn btn-secondary btn-sm gap-1.5 touch-manipulation"
          data-testid="copy-subscription-url-btn"
          :disabled="!copyUrl || verifyingPublication || publicationLookupFailed"
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

    <!-- Target Selector Tabs (Modern 4 targets) -->
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

    <!-- Primary Workflow: Client Subscription (LIVE-1 / LIVE-2) -->
    <div
      data-testid="client-subscription-section"
      class="card border border-primary/20 bg-base-100 shadow-sm rounded-2xl overflow-hidden"
    >
      <div class="card-body p-4 sm:p-6 space-y-4">
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
          <div class="space-y-1">
            <div class="flex items-center gap-2">
              <span class="badge badge-primary badge-sm font-semibold">推荐流程</span>
              <h3 class="font-bold text-base sm:text-lg">客户端实时订阅 · {{ currentTargetMeta.label }}</h3>
            </div>
            <p class="text-xs sm:text-sm opacity-70">
              客户端导入一次直链，后续更新自动按当前可用事实渲染；无需反复预检与重新发布。
            </p>
          </div>

          <div class="flex flex-wrap items-center gap-2 shrink-0">
            <button
              type="button"
              class="btn btn-sm btn-outline gap-1.5"
              data-testid="check-client-subscription-btn"
              :disabled="checkingLinkId !== null"
              :class="{ loading: checkingLinkId !== null }"
              @click="() => handleCheckClientSub()"
            >
              <ShieldCheckIcon class="w-4 h-4" />
              <span>检查可用性</span>
            </button>

            <button
              type="button"
              class="btn btn-sm btn-primary gap-1.5 touch-manipulation shadow"
              data-testid="copy-client-subscription-btn"
              :disabled="copyingLink"
              :class="{ loading: copyingLink }"
              @click="handleCopyDefaultClientSub"
            >
              <ClipboardDocumentCheckIcon v-if="copiedClientSub" class="w-4 h-4" />
              <ClipboardDocumentIcon v-else class="w-4 h-4" />
              <span>{{ copiedClientSub ? '已复制！' : '复制客户端订阅链接' }}</span>
            </button>
          </div>
        </div>

        <!-- Generation Warning Alert -->
        <div
          v-if="generationWarning"
          data-testid="generation-warning-banner"
          class="rounded-xl border border-warning/40 bg-warning/10 p-3 sm:p-4 text-xs space-y-1 text-warning"
          role="alert"
        >
          <div class="font-bold flex items-center gap-1.5 text-sm">
            <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
            <span>链接已创建，当前不可更新</span>
          </div>
          <p class="opacity-90 leading-relaxed">
            该订阅链接已成功建立，但当前规则或策略树未通过编译校验（或缺少活跃修订）。客户端目前可拉取该直链，但在配置修复通过前不会更新到新内容。
          </p>
        </div>

        <!-- Public Base URL Not Configured Setup Box -->
        <div
          v-if="publicBaseUrlMissing"
          data-testid="public-base-url-guide"
          class="rounded-xl border border-error/30 bg-error/10 p-4 space-y-3 text-xs"
          role="alert"
        >
          <div class="font-bold flex items-center gap-2 text-error text-sm">
            <GlobeAltIcon class="w-5 h-5 shrink-0" />
            <span>未配置公网可信基准地址 (Public Base URL)</span>
          </div>
          <p class="opacity-80">
            服务端生成客户端直链需要明确配置外部可信访问地址。请在下方输入并确认保存（系统绝不隐式保存浏览器主机名）。
          </p>
          <div class="flex flex-col sm:flex-row gap-2 max-w-xl">
            <input
              v-model="inputPublicBaseUrl"
              data-testid="public-base-url-input"
              class="input input-sm input-bordered flex-1 font-mono text-xs"
              placeholder="https://sub.example.com"
            />
            <button
              type="button"
              class="btn btn-sm btn-outline btn-neutral"
              data-testid="suggest-current-origin-btn"
              @click="suggestCurrentOrigin"
            >
              填入当前地址建议
            </button>
            <button
              type="button"
              class="btn btn-sm btn-primary"
              data-testid="save-public-base-url-btn"
              :disabled="savingPublicBaseUrl || !inputPublicBaseUrl.trim()"
              :class="{ loading: savingPublicBaseUrl }"
              @click="handleSavePublicBaseUrl"
            >
              确认保存
            </button>
          </div>
        </div>

        <!-- Current Default Link Summary Info -->
        <div class="grid grid-cols-1 sm:grid-cols-4 gap-2 pt-2 border-t border-base-200 text-xs">
          <div class="p-2.5 rounded-xl bg-base-200/50">
            <span class="opacity-60 block text-[11px]">导出目标格式</span>
            <span class="font-semibold uppercase text-primary">{{ currentTargetMeta.label }} (.{{ currentTargetMeta.ext }})</span>
          </div>
          <div class="p-2.5 rounded-xl bg-base-200/50">
            <span class="opacity-60 block text-[11px]">默认链接状态</span>
            <span v-if="defaultLinkForSelectedTarget" class="font-semibold text-success">
              {{ subscriptionLinkStateLabel(defaultLinkForSelectedTarget.state) }}
            </span>
            <span v-else class="opacity-60 font-semibold">未初始化（点击复制自动生成）</span>
          </div>
          <div class="p-2.5 rounded-xl bg-base-200/50">
            <span class="opacity-60 block text-[11px]">公网基准地址</span>
            <span class="font-mono truncate block" :title="publicBaseUrl || '未配置'">{{ publicBaseUrl || '--' }}</span>
          </div>
          <div class="p-2.5 rounded-xl bg-base-200/50">
            <span class="opacity-60 block text-[11px]">动态渲染保证</span>
            <span class="text-base-content/80">每次更新按最新事实拉取</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Server Subscription Links Management Table (Cross-Browser Persistence) -->
    <div
      data-testid="server-subscription-links-section"
      class="card border border-base-300 bg-base-100 shadow-sm rounded-2xl overflow-hidden"
    >
      <div class="card-body p-4 sm:p-5 space-y-3">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <KeyIcon class="w-4 h-4 text-primary" />
            <h4 class="font-bold text-sm sm:text-base">服务端订阅链接管理</h4>
            <span class="badge badge-sm badge-ghost font-mono">{{ subscriptionLinks.length }}</span>
          </div>
          <button
            type="button"
            class="btn btn-ghost btn-xs gap-1"
            :disabled="loadingLinks"
            @click="fetchSubscriptionLinks"
          >
            <ArrowPathIcon class="w-3.5 h-3.5" :class="{ 'animate-spin': loadingLinks }" />
            <span>刷新列表</span>
          </button>
        </div>

        <div v-if="subscriptionLinks.length === 0 && !loadingLinks" class="py-6 text-center text-xs opacity-60">
          暂无已注册的服务端订阅链接。点击上方“复制客户端订阅链接”将自动创建默认链接。
        </div>

        <div v-else class="overflow-x-auto">
          <table class="table table-sm w-full text-xs">
            <thead>
              <tr class="opacity-70 border-b border-base-300">
                <th>名称 / 标识</th>
                <th>目标格式</th>
                <th>凭据掩码</th>
                <th>状态</th>
                <th>类型</th>
                <th>创建时间</th>
                <th class="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="link in subscriptionLinks"
                :key="link.id"
                :data-testid="`subscription-link-row-${link.id}`"
                class="hover:bg-base-200/40"
              >
                <td class="font-medium">
                  <div class="flex items-center gap-1.5">
                    <span>{{ link.label || `${link.target} 默认订阅` }}</span>
                    <button
                      type="button"
                      class="btn btn-ghost btn-xs btn-square opacity-60 hover:opacity-100"
                      title="重命名"
                      :data-testid="`edit-label-btn-${link.id}`"
                      @click="openEditLabelModal(link)"
                    >
                      <PencilSquareIcon class="w-3.5 h-3.5" />
                    </button>
                  </div>
                </td>
                <td>
                  <span class="badge badge-xs uppercase font-mono badge-outline badge-primary">
                    {{ link.target }}
                  </span>
                </td>
                <td class="font-mono text-[11px] opacity-80">
                  {{ link.token_masked || 'sec_***' }}
                </td>
                <td>
                  <span
                    class="badge badge-xs font-medium"
                    :class="link.state === 'active' ? 'badge-success badge-outline' : 'badge-error badge-outline'"
                  >
                    {{ subscriptionLinkStateLabel(link.state) }}
                  </span>
                </td>
                <td>
                  <span v-if="link.is_default" class="badge badge-xs badge-ghost">默认</span>
                  <span v-else class="badge badge-xs badge-ghost opacity-60">自定义</span>
                </td>
                <td class="font-mono text-[11px] opacity-60">
                  {{ formatDateTime(link.created_at) }}
                </td>
                <td class="text-right space-x-1">
                  <button
                    v-if="link.state === 'active'"
                    type="button"
                    class="btn btn-ghost btn-xs text-primary"
                    :data-testid="`copy-link-btn-${link.id}`"
                    @click="handleCopySpecificLink(link)"
                  >
                    复制
                  </button>
                  <button
                    v-if="link.state === 'active'"
                    type="button"
                    class="btn btn-ghost btn-xs"
                    :data-testid="`check-link-btn-${link.id}`"
                    :disabled="checkingLinkId === link.id"
                    @click="handleCheckClientSub(link.id)"
                  >
                    检查
                  </button>
                  <button
                    v-if="link.state === 'active'"
                    type="button"
                    class="btn btn-ghost btn-xs text-warning"
                    :data-testid="`rotate-link-btn-${link.id}`"
                    :disabled="rotatingLinkId === link.id"
                    @click="openRotateModal(link)"
                  >
                    轮换
                  </button>
                  <button
                    v-if="link.state === 'active'"
                    type="button"
                    class="btn btn-ghost btn-xs text-error"
                    :data-testid="`revoke-link-btn-${link.id}`"
                    :disabled="revokingLinkId === link.id"
                    @click="openRevokeModal(link)"
                  >
                    停用
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Advanced Section Divider & Header -->
    <div class="pt-2">
      <div class="flex items-center justify-between pb-2 border-b border-base-300">
        <h3 class="font-bold text-sm text-base-content/80 flex items-center gap-2">
          <CubeTransparentIcon class="w-4 h-4 text-primary" />
          <span>高级选项：配置预览与历史不可变发布 (Immutable Publication)</span>
        </h3>
        <button
          type="button"
          class="btn btn-ghost btn-xs"
          @click="showAdvancedOptions = !showAdvancedOptions"
        >
          {{ showAdvancedOptions ? '收起高级区' : '展开高级区' }}
        </button>
      </div>
    </div>

    <!-- Advanced Section Body -->
    <div v-show="showAdvancedOptions" class="space-y-5">
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
            <span v-if="diag.node_id" class="badge badge-xs badge-neutral mr-1 font-mono">
              {{ diag.node_id }}
            </span>
            <strong>{{ diag.target ? `[${diag.target}] ` : '' }}{{ diag.message }}</strong>
            <span v-if="diag.reason" class="opacity-75 block pl-4">原因：{{ diag.reason }}</span>
          </li>
        </ul>
      </div>

      <div v-if="!activePublication && !loadingPreview && !error" data-testid="no-publication-guide" class="rounded-xl border border-warning/30 bg-warning/10 p-4 space-y-2" role="status">
        <h3 class="font-bold">{{ t('publications.noPublicationTitle') }}</h3>
        <p class="text-sm">{{ t('publications.noPublicationDesc') }}</p>
        <button type="button" class="btn btn-primary btn-sm" :disabled="!preview || publishing" @click="handlePublish">{{ t('publications.createPub') }}</button>
      </div>
      <p v-if="publicationLookupFailed" role="alert" class="text-error">{{ t('publications.publicationLookupFailed') }}</p>
      <p v-if="authSettingsError" role="alert" class="text-error">{{ t('settings.invalidSettings') }}</p>
      <div v-if="activePublication && exportProtected !== null" data-testid="publication-link-state" class="rounded-xl bg-base-200 p-4 space-y-2">
        <p>{{ t(exportProtected ? 'publications.protectedLink' : 'publications.publicLink') }}</p>
        <p v-if="exportProtected && !copyUrl" class="text-warning text-sm">{{ t('publications.missingCredential') }}</p>
        <label v-if="exportProtected" class="block text-sm space-y-1">
          <span>{{ t('publications.sharedTokenHint') }}</span>
          <input v-model="suppliedToken" type="password" autocomplete="off" data-testid="export-token-input" class="input input-bordered w-full" :placeholder="t('publications.enterExportToken')" />
        </label>
      </div>

      <!-- Compatibility Mode Selector -->
      <div class="flex flex-wrap items-center justify-between gap-2 p-3 rounded-xl bg-base-200/50 border border-base-300 text-xs w-full min-w-0">
        <div class="flex items-center gap-2">
          <span class="font-semibold text-base-content">{{ t('publications.compatModeLabel') }}:</span>
          <div class="join" role="group" aria-label="编译模式切换">
            <button
              type="button"
              class="btn btn-xs join-item transition-all"
              :class="compatMode === 'strict' ? 'btn-primary font-bold shadow-sm' : 'btn-ghost bg-base-100 hover:bg-base-300'"
              data-testid="mode-strict-btn"
              @click="switchCompatMode('strict')"
            >
              {{ t('publications.compatStrict') }}
            </button>
            <button
              type="button"
              class="btn btn-xs join-item transition-all"
              :class="compatMode === 'compatible' ? 'btn-primary font-bold shadow-sm' : 'btn-ghost bg-base-100 hover:bg-base-300'"
              data-testid="mode-compatible-btn"
              @click="switchCompatMode('compatible')"
            >
              {{ t('publications.compatCompatible') }}
            </button>
          </div>
        </div>
        <p class="text-[11px] opacity-75">
          {{ compatMode === 'strict' ? t('publications.compatStrictDesc') : t('publications.compatCompatibleDesc') }}
        </p>
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

      <!-- Main Preview Card -->
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
              <span v-if="preview?.snapshot_id" class="badge badge-sm badge-ghost font-mono text-[10px]" data-testid="preview-snapshot-id" title="不可变快照 ID">
                快照: {{ formatDigest(preview.snapshot_id) }}
              </span>
              <span v-if="preview?.content_digest" title="配置内容 SHA-256 摘要">
                内容摘要: <strong>{{ formatDigest(preview.content_digest) }}</strong>
              </span>
              <span v-if="preview?.snapshot_digest" class="hidden sm:inline" title="策略快照摘要">
                · 快照摘要: <strong>{{ formatDigest(preview.snapshot_digest) }}</strong>
              </span>
            </div>
          </div>

          <!-- Manifest Excluded Nodes -->
          <div
            v-if="preview?.manifest?.excluded && preview.manifest.excluded.length > 0"
            data-testid="excluded-nodes-card"
            class="p-3 rounded-lg bg-warning/10 border border-warning/30 text-warning text-xs space-y-1.5"
          >
            <div class="font-bold flex items-center justify-between gap-1.5 text-xs">
              <div class="flex items-center gap-1.5">
                <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
                <span>{{ t('publications.excludedNodesTitle') }} ({{ preview.manifest.excluded.length }} 个节点)</span>
              </div>
              <span class="badge badge-xs badge-warning badge-outline font-normal">
                {{ t('publications.compatCompatible') }}
              </span>
            </div>
            <p class="opacity-80 text-[11px]">
              {{ t('publications.compatCompatibleDesc') }}
            </p>
            <div class="divide-y divide-warning/20 font-mono text-[11px] pt-1">
              <div
                v-for="(item, idx) in preview.manifest.excluded"
                :key="idx"
                data-testid="excluded-node-item"
                class="py-1 flex flex-wrap items-center justify-between gap-2"
              >
                <strong class="text-base-content/90">{{ item.node_id }}</strong>
                <div class="flex items-center gap-1.5">
                  <span v-if="item.code" class="badge badge-xs badge-outline badge-warning">
                    {{ item.code }}
                  </span>
                  <span v-if="item.reason" class="text-base-content/75 font-sans">
                    {{ item.reason }}
                  </span>
                </div>
              </div>
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
              <!-- Quick Copy Button -->
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
    </div>

    <!-- Manual Copy Fallback Modal -->
    <ModalDialog
      v-model="manualCopyModalOpen"
      title="手动复制客户端订阅直链"
      description="浏览器剪贴板权限受限或非安全上下文，请在下方全选复制订阅链接："
    >
      <div class="space-y-3">
        <textarea
          readonly
          rows="3"
          :value="manualCopyUrl"
          data-testid="manual-copy-textarea"
          class="textarea textarea-bordered w-full font-mono text-xs select-all bg-base-200"
          @focus="($event.target as HTMLTextAreaElement).select()"
        />
        <p class="text-[11px] opacity-70">
          请复制上方完整地址后粘贴至客户端（Clash / Sing-box / Surge / Quantumult X）中。
        </p>
      </div>
      <template #footer>
        <button
          type="button"
          class="btn btn-primary btn-sm"
          @click="manualCopyModalOpen = false"
        >
          完成
        </button>
      </template>
    </ModalDialog>

    <!-- Rotate Token Confirm Modal -->
    <ConfirmModal
      v-model="rotateConfirmModalOpen"
      title="确认轮换订阅访问令牌"
      message="确定轮换该订阅链接的 Token 吗？轮换后旧链接立即失效，所有已导入该链接的客户端将无法拉取更新，需要重新导入新链接。"
      confirm-text="确认轮换"
      cancel-text="取消"
      tone="warning"
      :loading="rotatingLinkId !== null"
      @confirm="confirmRotateToken"
    />

    <!-- Revoke Link Confirm Modal -->
    <ConfirmModal
      v-model="revokeConfirmModalOpen"
      title="确认停用订阅链接"
      message="确定永久停用此订阅链接吗？停用后该链接将永久失效并记录撤销墓碑，客户端拉取将返回 403 错误拒绝服务。"
      confirm-text="确认停用"
      cancel-text="保持生效"
      tone="danger"
      :loading="revokingLinkId !== null"
      @confirm="confirmRevokeLink"
    />

    <!-- Edit Label Modal -->
    <ModalDialog
      v-model="editLabelModalOpen"
      title="修改订阅链接名称"
      description="为该客户端订阅链接设置易于识别的备注名称："
    >
      <div class="space-y-3">
        <input
          v-model="inputLinkLabel"
          data-testid="edit-link-label-input"
          class="input input-sm input-bordered w-full text-xs"
          placeholder="例如：办公室笔记本 / 手机 Mihomo"
          maxlength="64"
        />
      </div>
      <template #footer>
        <div class="flex justify-end gap-2 w-full">
          <button
            type="button"
            class="btn btn-ghost btn-sm"
            @click="editLabelModalOpen = false"
          >
            取消
          </button>
          <button
            type="button"
            class="btn btn-primary btn-sm"
            data-testid="confirm-save-label-btn"
            :disabled="updatingLinkId !== null"
            :class="{ loading: updatingLinkId !== null }"
            @click="confirmSaveLabel"
          >
            保存名称
          </button>
        </div>
      </template>
    </ModalDialog>

    <!-- Check Result Modal -->
    <ModalDialog
      v-model="checkDetailModalOpen"
      title="客户端订阅可用性检测结果"
      description="仅检测当前事实与配置规则可用性，绝不创建草稿或配置修订。"
    >
      <div v-if="activeCheckResult" class="space-y-3 text-xs">
        <div
          class="p-3 rounded-xl border flex items-center gap-2"
          :class="activeCheckResult.allowed ? 'bg-success/10 border-success/30 text-success' : 'bg-warning/10 border-warning/30 text-warning'"
        >
          <CheckCircleIcon v-if="activeCheckResult.allowed" class="w-5 h-5 shrink-0" />
          <ExclamationTriangleIcon v-else class="w-5 h-5 shrink-0" />
          <div class="font-bold">
            {{ activeCheckResult.allowed ? '配置可用，客户端拉取将正常生成' : (activeCheckResult.message || '链接已创建，当前不可更新') }}
          </div>
        </div>

        <div class="grid grid-cols-2 gap-2 font-mono text-[11px] p-3 rounded-xl bg-base-200">
          <div>
            <span class="opacity-60 block">有效节点数</span>
            <strong class="text-sm font-sans">{{ activeCheckResult.node_count ?? 0 }} 个</strong>
          </div>
          <div>
            <span class="opacity-60 block">检测时间戳 (As Of)</span>
            <span class="text-[11px]">{{ formatDateTime(activeCheckResult.as_of) }}</span>
          </div>
          <div v-if="activeCheckResult.content_digest" class="col-span-2">
            <span class="opacity-60 block">渲染内容指纹 (SHA-256)</span>
            <span class="break-all">{{ activeCheckResult.content_digest }}</span>
          </div>
        </div>

        <div v-if="activeCheckResult.diagnostics && activeCheckResult.diagnostics.length > 0" class="space-y-1">
          <span class="font-bold text-error">阻断诊断信息：</span>
          <ul class="list-disc list-inside font-mono text-[11px] space-y-0.5 text-error">
            <li v-for="(d, idx) in activeCheckResult.diagnostics" :key="idx">
              {{ d.code ? `[${d.code}] ` : '' }}{{ d.message }}
            </li>
          </ul>
        </div>
      </div>
      <template #footer>
        <button
          type="button"
          class="btn btn-ghost btn-sm"
          @click="checkDetailModalOpen = false"
        >
          关闭
        </button>
      </template>
    </ModalDialog>

    <!-- Publish Success / Token Modal (Historical Publish) -->
    <ModalDialog
      v-model="publishModalOpen"
      title="订阅发布链接已生成"
      :description="t(exportProtected ? 'publications.protectedLink' : 'publications.publicLink')"
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
              :value="copyUrl"
              class="input input-bordered input-sm font-mono text-xs flex-1 min-w-0 bg-base-200"
            />
            <button
              type="button"
              class="btn btn-primary btn-sm gap-1"
              data-testid="modal-copy-subscription-url-btn"
              :disabled="!copyUrl || verifyingPublication || publicationLookupFailed"
              @click="copyPublicationURL"
            >
              <ClipboardDocumentIcon class="w-4 h-4" />
              {{ copiedToken ? '已复制！' : '复制订阅链接' }}
            </button>
          </div>
          <p class="text-[11px] opacity-60">
            {{ t(exportProtected ? 'publications.sharedTokenHint' : 'publications.publicLink') }}
          </p>
        </div>

        <div class="grid grid-cols-2 sm:grid-cols-3 gap-2 text-xs font-mono p-3 rounded-xl bg-base-200 border border-base-300 min-w-0 break-all">
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
          <div class="col-span-2 sm:col-span-1">
            <span class="opacity-60 block">历史快照语义</span>
            <span class="badge badge-xs badge-neutral font-sans mt-0.5" title="历史发布快照具有不可变性，其节点计数为发布时的定格快照，与当前实时订阅库存解耦">
              不可变定格快照
            </span>
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
