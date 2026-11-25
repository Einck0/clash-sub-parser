<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  ArrowPathIcon,
  EyeIcon,
  InformationCircleIcon,
  XMarkIcon,
} from '@heroicons/vue/24/outline'
import { displayText } from '../../utils/displayNames'
import { uiText } from '../../locales/redesign'
import SettingsSections from '../settings/SettingsSections.vue'
import { useDnsDraft } from './useDnsDraft'
import {
  DNS_COMMON_FIELDS,
  DNS_ADVANCED_GROUPS,
  getUnknownKeys,
  matchesDnsField,
} from './dnsFields'
import DnsFieldControl from './DnsFieldControl.vue'
import DnsGroupCard from './DnsGroupCard.vue'

const {
  settings,
  enabled,
  raw,
  draft,
  mode,
  loading,
  saving,
  saveNeedsConfirmation,
  saveConflict,
  previewing,
  busy,
  previewYaml,
  isPreviewStale,
  error,
  notice,
  fieldErrors,
  invalidMappingInputs,
  hasValidationErrors,
  dirty,
  addListItem,
  removeListItem,
  pasteListItems,
  setListText,
  setYamlMappingText,
  setString,
  setBool,
  removeKey,
  onRawInput,
  load,
  preview,
  switchMode,
  save,
} = useDnsDraft()

const fieldSearch = ref('')
const matchingFieldCount = computed(() => [
  ...DNS_COMMON_FIELDS,
  ...DNS_ADVANCED_GROUPS.flatMap((group) => group.fields),
].filter((field) => matchesDnsField(field, fieldSearch.value)).length)
const commonMatches = computed(() => DNS_COMMON_FIELDS.some((field) => matchesDnsField(field, fieldSearch.value) || Boolean(fieldErrors.value[field.key])))
const advancedMatches = computed(() => DNS_ADVANCED_GROUPS.some((group) => group.fields.some((field) => matchesDnsField(field, fieldSearch.value) || Boolean(fieldErrors.value[field.key]))))

const unknownKeys = computed(() => {
  return getUnknownKeys(draft.value)
})

const dnsDisabledNotice = computed(() => {
  if (!settings.value || saveNeedsConfirmation.value || saveConflict.value) return null
  if (!enabled.value && !settings.value.enabled) {
    return {
      type: 'saved-disabled',
      text: uiText(
        '客户端 DNS 当前已停用。配置草稿已保留，导出的客户端订阅配置中将包含 enable: false。',
        'Client DNS is currently disabled. Draft settings are preserved, and exported client configuration contains enable: false.'
      ),
    }
  }
  if (!enabled.value && settings.value.enabled) {
    return {
      type: 'draft-disabled',
      text: uiText(
        '草稿中已关闭客户端 DNS（未保存）。需点击保存后，导出的客户端订阅配置才会变为 enable: false；当前草稿已保留。',
        'Client DNS is disabled in draft (unsaved). Exported client configuration will change to enable: false only after saving; draft is preserved.'
      ),
    }
  }
  if (enabled.value && !settings.value.enabled) {
    return {
      type: 'draft-enabled',
      text: uiText(
        '草稿中已开启客户端 DNS（未保存）。需点击保存后，导出的客户端订阅配置才会启用 DNS。',
        'Client DNS is enabled in draft (unsaved). Exported client configuration will enable DNS only after saving.'
      ),
    }
  }
  return null
})

function clearPreview() {
  previewYaml.value = ''
}

onMounted(() => {
  void load(true)
})
</script>

<template>
  <section
    class="space-y-4 pb-8 min-w-0"
    data-testid="dns-view"
    :aria-label="uiText('客户端 DNS 配置', 'Client DNS Settings')"
  >
    <!-- Top Shared Navigation: CSP / DNS / Chains -->
    <SettingsSections active="dns" />

    <!-- DNS-local responsive toolbar: Accessible SR-only title, natural wrapping on mobile -->
    <div
      class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2.5 sm:gap-3 min-w-0 w-full"
      data-testid="page-toolbar"
    >
      <!-- Left controls: Enable toggle and mode tabs -->
      <div class="flex flex-wrap items-center gap-2 min-w-0" data-testid="toolbar-controls">
        <label
          class="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-base-200/80 border border-base-300 cursor-pointer select-none transition-opacity shrink-0"
          :class="{ 'opacity-50 pointer-events-none': busy || !settings }"
          data-testid="dns-enabled-label"
        >
          <input
            v-model="enabled"
            data-testid="dns-enabled"
            type="checkbox"
            class="toggle toggle-primary toggle-sm shrink-0"
            :disabled="busy || !settings"
          />
          <span class="text-xs font-semibold whitespace-nowrap">{{ uiText('启用客户端 DNS', 'Enable Client DNS') }}</span>
        </label>

        <div class="join shrink-0" role="tablist" :aria-label="uiText('DNS 编辑模式', 'DNS Edit Mode')">
          <button
            role="tab"
            :aria-selected="mode === 'visual'"
            class="btn btn-sm join-item"
            :class="mode === 'visual' ? 'btn-primary' : 'btn-ghost'"
            data-testid="dns-visual-tab"
            :disabled="busy || !settings"
            @click="switchMode('visual')"
          >
            {{ uiText('可视配置', 'Visual Editor') }}
          </button>
          <button
            role="tab"
            :aria-selected="mode === 'raw'"
            class="btn btn-sm join-item"
            :class="mode === 'raw' ? 'btn-primary' : 'btn-ghost'"
            data-testid="dns-raw-tab"
            :disabled="busy || !settings"
            @click="switchMode('raw')"
          >
            Raw YAML
          </button>
        </div>
      </div>

      <!-- Right actions: Status badges and reload button -->
      <div
        v-if="settings"
        class="flex items-center justify-between sm:justify-end gap-2 w-full sm:w-auto shrink-0 pt-0.5 sm:pt-0"
        data-testid="toolbar-actions"
      >
        <div class="flex items-center gap-1.5">
          <!-- Dirty indicator badge -->
          <span
            v-if="dirty"
            class="badge badge-sm badge-warning font-semibold text-[11px]"
            data-testid="dns-dirty-badge"
          >
            {{ saveNeedsConfirmation ? uiText('保存待确认', 'Save unconfirmed') : saveConflict ? uiText('版本冲突', 'Revision conflict') : uiText('未保存', 'Unsaved') }}
          </span>
          <span
            v-else
            class="badge badge-sm badge-ghost opacity-60 text-[11px]"
            data-testid="dns-clean-badge"
          >
            {{ settings.source === 'default' ? uiText('默认配置', 'Default config') : uiText('已保存', 'Saved') }}
          </span>

          <span class="badge badge-sm badge-neutral font-mono text-[11px] opacity-75">
            rev {{ settings.revision }}
          </span>
        </div>

        <button
          class="btn btn-ghost btn-sm text-xs shrink-0"
          :disabled="busy"
          data-testid="dns-reload"
          @click="load(false)"
        >
          <ArrowPathIcon class="w-3.5 h-3.5 mr-1" />
          {{ uiText('重新载入', 'Reload') }}
        </button>
      </div>
    </div>

    <!-- DNS Status Alert (shows when either saved is off, or draft differs from saved) -->
    <div
      v-if="dnsDisabledNotice"
      role="status"
      data-testid="dns-disabled-notice"
      class="alert bg-base-200/90 border border-base-300 text-base-content/80 shadow-xs py-2 px-3 text-xs flex items-start gap-2 rounded-xl"
    >
      <InformationCircleIcon class="w-4 h-4 text-warning shrink-0 mt-0.5" aria-hidden="true" />
      <span class="min-w-0 flex-1 break-words [overflow-wrap:anywhere]">{{ dnsDisabledNotice.text }}</span>
    </div>

    <!-- Loading / Status Alerts -->
    <div v-if="loading" role="status" class="alert shadow-xs py-2 px-3">
      <span class="loading loading-spinner loading-xs text-primary" />
      <span class="text-xs font-medium">{{ uiText('正在读取 DNS 设置…', 'Loading DNS settings…') }}</span>
    </div>

    <div v-if="error" role="alert" data-testid="dns-error" class="alert alert-error shadow-xs py-2 px-3 flex flex-col sm:flex-row items-start">
      <span class="text-xs font-medium flex-1 min-w-0 break-words [overflow-wrap:anywhere]">{{ displayText(error) }}</span>
      <button
        class="btn btn-sm btn-ghost border-current/30 text-xs shrink-0"
        :disabled="busy"
        data-testid="dns-error-retry"
        @click="saveNeedsConfirmation ? save() : load(false)"
      >
        {{ saveNeedsConfirmation ? uiText('确认保存结果', 'Confirm save result') : saveConflict ? uiText('重载最新 revision', 'Reload latest revision') : uiText('重试 / 重载', 'Retry / Reload') }}
      </button>
    </div>

    <div v-if="notice" role="status" class="alert alert-success shadow-xs py-2 px-3">
      <span class="text-xs font-medium min-w-0 break-words">{{ notice }}</span>
    </div>

    <div v-if="saveNeedsConfirmation || saveConflict" role="status" data-testid="dns-save-state" class="rounded-xl border border-warning/40 bg-warning/10 p-3 text-xs break-words">
      {{ saveNeedsConfirmation
        ? uiText('DNS 已写入，但回读确认尚未完成。确认保存结果只会读取服务器，不会重复写入；仍可保留或复制当前草稿。', 'DNS was written, but read-back confirmation is pending. Confirm save only reads the server; it does not write again. You can keep or copy the draft.')
        : uiText('检测到服务器版本冲突。当前草稿与原 revision 已保留，保存已暂停；可切换 Raw YAML 复制草稿，再重新载入最新配置核对。', 'A server revision conflict was detected. Draft and original revision are preserved; saving is paused. Copy the draft in Raw YAML, then reload the latest configuration to compare.') }}
    </div>

    <!-- Main Configuration Card -->
    <template v-if="settings">
      <section class="card min-w-0 bg-base-200/80 border border-base-300 rounded-2xl shadow-xs">
        <div class="card-body p-4 sm:p-6 space-y-6">
          <div v-if="mode === 'visual'" class="space-y-2">
            <div class="flex items-center gap-2">
              <input
                v-model="fieldSearch"
                type="search"
                class="input input-sm w-full min-w-0"
                data-testid="dns-field-search"
                :aria-label="uiText('查找 DNS 字段', 'Find DNS fields')"
                :placeholder="uiText('查找字段名称、配置键或说明…', 'Find a field by name, key, or description…')"
              />
              <button v-if="fieldSearch" type="button" class="btn btn-sm btn-ghost shrink-0" @click="fieldSearch = ''">
                {{ uiText('清除', 'Clear') }}
              </button>
            </div>
            <p v-if="fieldSearch.trim()" role="status" class="text-xs text-base-content/65" data-testid="dns-field-search-result">
              {{ matchingFieldCount ? uiText('找到 ' + matchingFieldCount + ' 个字段；高级分区已展开，筛选不会修改草稿。', matchingFieldCount + ' fields found. Advanced sections are expanded; filtering does not change the draft.') : uiText('没有匹配的可视字段。可清除搜索，或在 Raw YAML 中查看全部自定义键。', 'No matching visual fields. Clear the search, or use Raw YAML to view all custom keys.') }}
            </p>
          </div>

          <!-- Keep visual controls mounted so unsubmitted quick-entry text survives mode switches. -->
          <div v-show="mode === 'visual'" class="space-y-6">
            <!-- Section 1: 常用核心配置 (Core Settings - Upfront & Not Folded) -->
            <div v-show="commonMatches" class="space-y-3" data-testid="dns-common-section">
              <div class="flex items-center justify-between pb-1.5 border-b border-base-300/60">
                <div>
                  <h3 class="text-xs sm:text-sm font-bold uppercase tracking-wider text-base-content/90">
                    {{ uiText('常用核心设置', 'Core DNS Settings') }}
                  </h3>
                  <p class="text-[11px] text-base-content/60 mt-0.5">
                    {{ uiText('主要上游、后备与引导解析器，Fake-IP 地址池与过滤列表；未配置项均遵循客户端内核缺省。', 'Primary, fallback, and bootstrap nameservers, Fake-IP pool and filters; unconfigured options use core defaults.') }}
                  </p>
                </div>
              </div>

              <div class="grid grid-cols-1 md:grid-cols-2 gap-3 sm:gap-4">
                <DnsFieldControl
                  v-for="field in DNS_COMMON_FIELDS"
                  v-show="matchesDnsField(field, fieldSearch) || Boolean(fieldErrors[field.key])"
                  :key="field.key"
                  :class="{ 'md:col-span-2': field.key === 'nameserver' || field.key === 'fake-ip-filter' }"
                  :field="field"
                  :draft="draft"
                  :field-error="fieldErrors[field.key]"
                  :invalid-mapping-input="invalidMappingInputs[field.key]"
                  :disabled="busy"
                  @update-list="setListText"
                  @add-list-item="addListItem"
                  @remove-list-item="removeListItem"
                  @paste-list-items="pasteListItems"
                  @update-scalar="setString"
                  @update-bool="setBool"
                  @update-yaml="setYamlMappingText"
                  @remove-key="removeKey"
                />
              </div>
            </div>

            <!-- Section 2: 高级与扩展配置 (Advanced Settings - YAML Mappings, Dedicated Servers, Switches) -->
            <div v-show="advancedMatches" class="space-y-3 pt-2" data-testid="dns-advanced-section">
              <div class="flex items-center justify-between pb-1.5 border-b border-base-300/60">
                <div>
                  <h3 class="text-xs sm:text-sm font-bold uppercase tracking-wider text-base-content/90">
                    {{ uiText('高级与扩展配置', 'Advanced & Policy Settings') }}
                  </h3>
                  <p class="text-[11px] text-base-content/60 mt-0.5">
                    {{ uiText('按需配置自然 YAML 解析策略映射、代理/直连专用解析服务器与客户端路由开关；未配置项遵循客户端缺省。', 'Configure natural YAML routing policies, dedicated nameservers, and routing flags as needed.') }}
                  </p>
                </div>
              </div>

              <div class="space-y-3">
                <DnsGroupCard
                  v-for="group in DNS_ADVANCED_GROUPS"
                  v-show="group.fields.some((field) => matchesDnsField(field, fieldSearch) || Boolean(fieldErrors[field.key]))"
                  :key="group.id"
                  :group="group"
                  :search="fieldSearch"
                  :draft="draft"
                  :field-errors="fieldErrors"
                  :invalid-mapping-inputs="invalidMappingInputs"
                  :disabled="busy"
                  @update-list="setListText"
                  @add-list-item="addListItem"
                  @remove-list-item="removeListItem"
                  @paste-list-items="pasteListItems"
                  @update-scalar="setString"
                  @update-bool="setBool"
                  @update-yaml="setYamlMappingText"
                  @remove-key="removeKey"
                />
              </div>
            </div>

            <!-- Section 3: 保全自定义/扩展字段 (Preserved Custom Keys) -->
            <div
              v-if="unknownKeys.length > 0"
              class="p-3 sm:p-4 rounded-xl bg-base-300/30 border border-base-300/70 text-xs space-y-2"
              data-testid="dns-unknown-keys"
            >
              <div class="flex flex-wrap items-center justify-between gap-2">
                <div class="flex flex-wrap items-center gap-1.5">
                  <span class="font-medium text-base-content/80">
                    {{ uiText('已保全自定义/扩展字段 (' + unknownKeys.length + ')：', 'Preserved custom/extended fields (' + unknownKeys.length + '):') }}
                  </span>
                  <span
                    v-for="key in unknownKeys"
                    :key="key"
                    class="badge badge-sm badge-outline font-mono text-[10px] h-auto whitespace-normal break-all"
                  >
                    {{ key }}
                  </span>
                </div>
                <button
                  class="btn btn-ghost btn-xs text-primary"
                  :disabled="busy"
                  @click="switchMode('raw')"
                >
                  {{ uiText('切换至 Raw YAML 模式编辑', 'Switch to Raw YAML to edit') }}
                </button>
              </div>
              <p class="text-[10px] text-base-content/50">
                {{ uiText('未知字段在可视模式中完整保留，保存时与当前草稿合并提交。', 'Custom keys are preserved in visual mode and merged atomically upon save.') }}
              </p>
            </div>
          </div>

          <!-- Mode 2: Raw YAML Mode -->
          <div v-if="mode === 'raw'" class="space-y-2">
            <div class="flex items-center justify-between">
              <div>
                <span class="text-xs sm:text-sm font-semibold text-base-content/90">
                  {{ uiText('完整 YAML 配置草稿', 'Full YAML Configuration Draft') }}
                </span>
                <p class="text-[11px] text-base-content/60 mt-0.5">
                  {{ uiText('注意：保存时由服务端重新规范化序列化；支持直接编辑全部标准与自定义字段。', 'Note: Re-normalized by server on save; supports all standard & custom fields.') }}
                </p>
              </div>
            </div>

            <textarea
              :value="raw"
              data-testid="dns-raw-editor"
              :aria-label="uiText('完整 YAML 配置草稿', 'Full YAML Configuration Draft')"
              class="textarea min-h-96 font-mono text-xs w-full bg-base-100 border border-base-300 rounded-xl leading-relaxed"
              spellcheck="false"
              :disabled="busy"
              @input="e => onRawInput((e.target as HTMLTextAreaElement).value)"
              @change="e => onRawInput((e.target as HTMLTextAreaElement).value)"
            />
          </div>

          <!-- Read-only YAML Preview Panel (Generated by validate endpoint) -->
          <div
            v-if="previewYaml"
            class="p-3 sm:p-4 rounded-xl bg-base-100/90 border border-primary/30 shadow-xs space-y-2"
            data-testid="dns-preview-section"
          >
            <div class="flex items-center justify-between">
              <div class="flex flex-wrap items-center gap-2">
                <EyeIcon class="w-4 h-4 text-primary shrink-0" />
                <span class="text-xs font-bold text-base-content">
                  {{ uiText('当前草稿规范化 DNS 配置块预览', 'Current Draft Normalized DNS Block Preview') }}
                </span>
                <!-- Current vs Stale preview status badge -->
                <span
                  v-if="!isPreviewStale"
                  class="badge badge-sm badge-success text-[10px]"
                  data-testid="dns-preview-status-current"
                >
                  {{ uiText('当前草稿预览 (最新)', 'Current draft preview (latest)') }}
                </span>
                <span
                  v-else
                  class="badge badge-sm badge-warning text-[10px]"
                  data-testid="dns-preview-status-stale"
                >
                  {{ uiText('草稿已变更 (预览已过时)', 'Draft changed (preview stale)') }}
                </span>
                <span class="badge badge-sm badge-neutral text-[10px]">
                  {{ uiText('只读 (未持久化)', 'Read-only (not saved)') }}
                </span>
              </div>
              <button
                type="button"
                class="btn btn-ghost btn-xs btn-circle"
                data-testid="dns-close-preview"
                :aria-label="uiText('关闭预览', 'Close preview')"
                :title="uiText('关闭预览', 'Close preview')"
                @click="clearPreview"
              >
                <XMarkIcon class="w-4 h-4" aria-hidden="true" />
              </button>
            </div>

            <p class="text-[11px] text-base-content/60">
              {{ uiText('此预览为当前草稿的服务端规范化 DNS 配置块；保存后导出订阅中的 dns: 配置结构与此一致；未保存到服务端，需点击保存生效。', 'Normalized DNS configuration block for current draft; reflects dns: block in exported subscription once saved; click Validate & Save to persist.') }}
            </p>

            <textarea
              readonly
              :value="previewYaml"
              data-testid="dns-preview-yaml"
              :aria-label="uiText('当前草稿规范化 DNS 配置块', 'Current draft normalized DNS block')"
              class="textarea min-h-48 font-mono text-xs w-full bg-base-200/50 border border-base-300 rounded-lg select-all"
              spellcheck="false"
            />
          </div>

          <!-- Bottom Actions & Meta Information -->
          <div class="flex flex-wrap items-center justify-between gap-3 pt-4 border-t border-base-300/60">
            <div class="flex flex-wrap items-center gap-2 text-xs font-mono opacity-70">
              <span class="badge badge-sm badge-ghost">
                {{ settings.source === 'saved' ? uiText('已保存配置', 'Saved config') : uiText('默认配置', 'Default config') }}
              </span>
              <span>revision {{ settings.revision }}</span>
              <span>·</span>
              <span>{{ settings.updated_at || '—' }}</span>
            </div>

            <div class="flex flex-wrap items-center justify-end gap-2 ml-auto w-full sm:w-auto">
              <!-- Preview YAML Button -->
              <button
                type="button"
                class="btn btn-sm btn-ghost border border-base-300 flex-1 sm:flex-none"
                data-testid="dns-preview-btn"
                :disabled="busy || hasValidationErrors"
                @click="preview"
              >
                <span v-if="previewing" class="loading loading-spinner loading-xs mr-1" />
                <EyeIcon v-else class="w-4 h-4 mr-1 text-base-content/70" />
                {{ previewing ? uiText('正在生成…', 'Generating…') : uiText('预览 YAML', 'Preview YAML') }}
              </button>

              <!-- Validate and Save Button -->
              <button
                type="button"
                class="btn btn-sm btn-primary flex-1 sm:flex-none"
                data-testid="dns-save"
                :disabled="busy || saveConflict || (!saveNeedsConfirmation && (!dirty || hasValidationErrors))"
                @click="save"
              >
                <span v-if="saving" class="loading loading-spinner loading-xs mr-1" />
                {{ saving ? (saveNeedsConfirmation ? uiText('正在确认…', 'Confirming…') : uiText('校验并保存…', 'Validating & saving…')) : saveNeedsConfirmation ? uiText('确认保存结果', 'Confirm save result') : uiText('校验并保存', 'Validate & Save') }}
              </button>
            </div>
          </div>
        </div>
      </section>
    </template>
  </section>
</template>
