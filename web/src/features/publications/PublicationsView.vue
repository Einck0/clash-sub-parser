<script setup lang="ts">
import {
  ClipboardDocumentIcon,
  ClipboardDocumentCheckIcon,
  ArrowDownTrayIcon,
} from '@heroicons/vue/24/outline'
import ModalDialog from '../../ui/ModalDialog.vue'
import PageToolbar from '../../ui/PageToolbar.vue'
import { toastStore } from '../../ui/toast'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'
import {
  COMPILER_TARGETS,
  type CompilerTarget,
  targetLabel,
} from './publicationTypes'
import { usePublications } from './usePublications'

const {
  getDirectUrl,
  copyDirectUrl,
  downloadDirectConfig,
  copiedTarget,
  manualCopyUrl,
  manualCopyModalOpen,
  downloadingTargets,
} = usePublications()

async function handleCopy(target: CompilerTarget) {
  const result = await copyDirectUrl(target)
  if (result.copied) {
    toastStore.push({
      message: t('publications.copyExportUrl'),
      tone: 'success',
    })
  }
}

async function handleDownload(target: CompilerTarget) {
  if (downloadingTargets.value[target]) return
  try {
    await downloadDirectConfig(target)
    toastStore.push({
      message: uiText(
        `已成功开始下载 ${targetLabel(target)} 配置文件`,
        `Started downloading ${targetLabel(target)} configuration file`
      ),
      tone: 'success',
    })
  } catch (err: any) {
    const errorMsg =
      err?.message ||
      uiText('配置编译或下载失败', 'Configuration compilation or download failed')
    toastStore.push({
      message: errorMsg,
      tone: 'error',
    })
  }
}
</script>

<template>
  <section class="mx-auto w-full max-w-5xl space-y-5" aria-labelledby="publications-title" data-testid="client-subscriptions-page">
    <h2 id="publications-title" class="sr-only">{{ t('publications.title') }}</h2>

    <!-- Compact Toolbar without redundant header -->
    <PageToolbar :title="t('publications.title')" :description="t('publications.subtitle')" sr-only-title>
      <template #default>
        <div class="flex items-center gap-2">
          <span class="text-xs font-semibold uppercase tracking-[0.16em] text-primary">{{ t('publications.tag') }}</span>
          <span class="text-xs text-base-content/60">{{ uiText('4 种客户端直链导出与配置下载', '4 Client Direct Links & Configuration Downloads') }}</span>
        </div>
      </template>
    </PageToolbar>

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
            <span
              class="badge badge-sm font-medium"
              :class="item.target === 'mihomo' ? 'badge-primary badge-outline' : 'badge-ghost'"
            >
              {{ item.target === 'mihomo' ? t('publications.fullConfig') : t('publications.nodesOnly') }}
            </span>
          </div>
          <p class="text-sm opacity-70">{{ item.desc }}</p>
          <div class="flex items-center gap-2">
            <input
              type="text"
              readonly
              :value="getDirectUrl(item.target)"
              class="input input-xs w-full max-w-md font-mono text-xs select-all bg-base-200/60"
              :data-testid="`direct-url-input-${item.target}`"
              @focus="($event.target as HTMLInputElement).select()"
            />
          </div>
          <!-- Truthful near-action capability hint -->
          <p class="text-[11px] text-base-content/50 font-mono">
            {{ item.target === 'mihomo'
              ? uiText('✓ 包含完整分流规则与策略组', '✓ Includes full rules and policy groups')
              : uiText('ℹ 仅导出节点格式（忽略策略组与规则）', 'ℹ Nodes-only export (ignores groups and rules)') }}
          </p>
        </div>

        <div class="flex shrink-0 items-center gap-2">
          <button
            type="button"
            class="btn btn-primary btn-sm min-w-32 gap-1.5 touch-manipulation"
            data-testid="copy-client-subscription-btn"
            :data-target="item.target"
            :data-testid-target="`copy-client-subscription-${item.target}`"
            @click="handleCopy(item.target)"
          >
            <ClipboardDocumentCheckIcon v-if="copiedTarget === item.target" class="h-4 w-4" />
            <ClipboardDocumentIcon v-else class="h-4 w-4" />
            <span>{{ copiedTarget === item.target ? '已复制！' : '复制订阅链接' }}</span>
          </button>
          <button
            type="button"
            class="btn btn-outline btn-sm gap-1 text-xs"
            :disabled="downloadingTargets[item.target]"
            data-testid="download-client-subscription-btn"
            :data-target="item.target"
            :data-testid-target="`download-client-subscription-${item.target}`"
            @click="handleDownload(item.target)"
          >
            <span v-if="downloadingTargets[item.target]" class="loading loading-spinner loading-xs" />
            <ArrowDownTrayIcon v-else class="h-3.5 w-3.5" />
            <span>{{ downloadingTargets[item.target] ? uiText('下载中...', 'Downloading...') : '下载配置' }}</span>
          </button>
        </div>
      </article>
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
          class="textarea w-full font-mono text-xs select-all bg-base-200"
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
  </section>
</template>
