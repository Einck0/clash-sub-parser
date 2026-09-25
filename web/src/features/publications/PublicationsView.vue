<script setup lang="ts">
import { onMounted, ref } from 'vue'
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
} from '@heroicons/vue/24/outline'
import { usePublications } from './usePublications'
import {
  COMPILER_TARGETS,
  formatDigest,
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
  isNoActiveRevision,
  copied,
  fetchPreview,
  publish,
  revoke,
  copyToClipboard,
  downloadFile,
} = usePublications()

const publishModalOpen = ref(false)
const copiedToken = ref(false)

async function switchTarget(target: CompilerTarget) {
  selectedTarget.value = target
  await fetchPreview(target)
}

async function handleCopyContent() {
  if (!preview.value?.content) return
  await copyToClipboard(preview.value.content)
}

function handleDownload() {
  if (!preview.value) return
  downloadFile(
    preview.value.filename || `config-${preview.value.target}.txt`,
    preview.value.content,
    preview.value.content_type
  )
}

async function handlePublish() {
  try {
    await publish(selectedTarget.value)
    publishModalOpen.value = true
  } catch {
    // handled in composable
  }
}

async function copyPublicationURL() {
  if (!activePublication.value?.export_url) return
  const fullUrl = `${window.location.origin}${activePublication.value.export_url}`
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
  fetchPreview(selectedTarget.value)
})
</script>

<template>
  <section class="space-y-5" aria-labelledby="publications-title">
    <!-- Header -->
    <div class="flex flex-col sm:flex-row sm:items-end justify-between gap-3 w-full min-w-0">
      <div class="min-w-0">
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-primary">{{ t('publications.tag') }}</p>
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

    <!-- Generic Error (401, 403, 500, network, etc.) -->
    <ErrorStateCard
      v-else-if="error"
      :error="errorDetail || error"
      :retrying="loadingPreview"
      @retry="() => fetchPreview(selectedTarget)"
    />

    <!-- Target Selector Tabs (Zashboard-inspired sleek pills) -->
    <div class="flex flex-wrap items-center gap-2 pb-1 text-xs w-full min-w-0" role="tablist" aria-label="Target formats">
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
            <span v-if="preview?.content_digest" title="Content Digest SHA-256">
              Content: <strong>{{ formatDigest(preview.content_digest) }}</strong>
            </span>
            <span v-if="preview?.snapshot_digest" class="hidden sm:inline" title="Snapshot Digest">
              · Snapshot: <strong>{{ formatDigest(preview.snapshot_digest) }}</strong>
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
            Compiler Diagnostics
          </div>
          <ul class="list-disc list-inside font-mono">
            <li v-for="(diag, idx) in preview.diagnostics" :key="idx">
              {{ diag.message }}
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
              <span>{{ copied ? 'Copied!' : 'Copy Code' }}</span>
            </button>

            <!-- Pre Code Block -->
            <pre class="p-4 sm:p-5 text-xs font-mono overflow-auto adaptive-preview-box leading-relaxed select-text text-base-content/90 max-w-full">{{ preview?.content || 'No configuration rendered.' }}</pre>
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
      title="Publication Endpoint Generated"
      description="Immutable subscriber URL bound to compiler target with SHA-256 token authorization"
    >
      <div v-if="activePublication" class="space-y-4">
        <div class="p-3.5 rounded-xl bg-success/10 border border-success/30 text-success text-xs flex items-center gap-2">
          <CheckBadgeIcon class="w-5 h-5 flex-shrink-0" />
          <span>Immutable publication registered successfully. Stale configuration fallbacks strictly refused.</span>
        </div>

        <div class="space-y-1.5">
          <label class="text-xs font-semibold">Client Subscription URL</label>
          <div class="flex gap-2">
            <input
              readonly
              :value="activePublication.export_url ? `${activePublication.export_url}` : ''"
              class="input input-bordered input-sm font-mono text-xs flex-1 bg-base-200"
            />
            <button
              type="button"
              class="btn btn-primary btn-sm gap-1"
              @click="copyPublicationURL"
            >
              <ClipboardDocumentIcon class="w-4 h-4" />
              {{ copiedToken ? 'Copied!' : 'Copy' }}
            </button>
          </div>
          <p class="text-[11px] opacity-60">
            Clients can fetch this subscription via Bearer header or the query token parameter.
          </p>
        </div>

        <div class="grid grid-cols-2 gap-2 text-xs font-mono p-3 rounded-xl bg-base-200 border border-base-300">
          <div>
            <span class="opacity-60 block">Target</span>
            <strong class="uppercase text-primary">{{ activePublication.target }}</strong>
          </div>
          <div>
            <span class="opacity-60 block">Status</span>
            <strong :class="activePublication.revoked_at ? 'text-error' : 'text-success'">
              {{ activePublication.revoked_at ? 'Revoked' : 'Active' }}
            </strong>
          </div>
        </div>

        <div class="modal-action border-t border-base-300 pt-3 flex items-center justify-between">
          <button
            v-if="!activePublication.revoked_at"
            type="button"
            class="btn btn-error btn-outline btn-sm gap-1"
            :disabled="revoking"
            :class="{ loading: revoking }"
            @click="handleRevoke"
          >
            <XCircleIcon class="w-4 h-4" />
            Revoke Access
          </button>
          <div v-else />

          <button
            type="button"
            class="btn btn-ghost btn-sm"
            @click="publishModalOpen = false"
          >
            Done
          </button>
        </div>
      </div>
    </ModalDialog>

    <!-- Confirm Revoke Publication Modal -->
    <ConfirmModal
      v-model="confirmRevokeOpen"
      title="Revoke Publication"
      message="Revoke this publication immediately? Clients will no longer be able to download this config."
      confirm-text="Revoke Immediately"
      cancel-text="Keep Active"
      tone="danger"
      :loading="revoking"
      @confirm="confirmRevokePublication"
    />
  </section>
</template>
