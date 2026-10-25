<script setup lang="ts">
import { computed, onMounted } from 'vue'
import {
  ArrowPathIcon,
  EyeIcon,
  InformationCircleIcon,
  XMarkIcon,
} from '@heroicons/vue/24/outline'
import PageToolbar from '../../ui/PageToolbar.vue'
import { uiText } from '../../locales/redesign'
import { useDnsDraft } from './useDnsDraft'
import {
  DNS_BASIC_FIELDS,
  DNS_GROUPS,
  getUnknownKeys,
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
  previewing,
  busy,
  previewYaml,
  error,
  notice,
  fieldErrors,
  invalidJsonInputs,
  hasValidationErrors,
  dirty,
  setListText,
  setJsonText,
  setString,
  setBool,
  removeKey,
  onRawInput,
  load,
  preview,
  switchMode,
  save,
} = useDnsDraft()

const unknownKeys = computed(() => {
  return getUnknownKeys(draft.value)
})

const dnsDisabledNotice = computed(() => {
  if (!settings.value) return null
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

onMounted(() => {
  void load(true)
})
</script>

<template>
  <section
    class="space-y-4 pb-8"
    data-testid="dns-view"
    :aria-label="uiText('客户端 DNS 配置', 'Client DNS Settings')"
  >
    <!-- Non-redundant Toolbar: Accessible SR-only title, quick mode tabs, actions -->
    <PageToolbar :title="uiText('客户端 DNS 配置', 'Client DNS Settings')" :sr-only-title="true">
      <div class="flex flex-wrap items-center gap-2">
        <label
          class="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-base-200/80 border border-base-300 cursor-pointer select-none transition-opacity"
          :class="{ 'opacity-50 pointer-events-none': busy }"
        >
          <input
            v-model="enabled"
            data-testid="dns-enabled"
            type="checkbox"
            class="toggle toggle-primary toggle-sm"
            :disabled="busy"
          />
          <span class="text-xs font-semibold">{{ uiText('启用客户端 DNS', 'Enable Client DNS') }}</span>
        </label>

        <div class="join" role="tablist" :aria-label="uiText('DNS 编辑模式', 'DNS Edit Mode')">
          <button
            role="tab"
            :aria-selected="mode === 'visual'"
            class="btn btn-sm join-item"
            :class="mode === 'visual' ? 'btn-primary' : 'btn-ghost'"
            data-testid="dns-visual-tab"
            :disabled="busy"
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
            :disabled="busy"
            @click="switchMode('raw')"
          >
            Raw YAML
          </button>
        </div>
      </div>

      <template #actions>
        <div v-if="settings" class="flex items-center gap-2">
          <!-- Dirty indicator badge -->
          <span
            v-if="dirty"
            class="badge badge-sm badge-warning font-semibold text-[11px]"
            data-testid="dns-dirty-badge"
          >
            {{ uiText('未保存', 'Unsaved') }}
          </span>
          <span
            v-else
            class="badge badge-sm badge-ghost opacity-60 text-[11px]"
            data-testid="dns-clean-badge"
          >
            {{ uiText('已保存', 'Saved') }}
          </span>

          <span class="badge badge-sm badge-neutral font-mono text-[11px] opacity-75">
            rev {{ settings.revision }}
          </span>
          <button
            class="btn btn-ghost btn-sm text-xs"
            :disabled="busy"
            data-testid="dns-reload"
            @click="load(false)"
          >
            <ArrowPathIcon class="w-3.5 h-3.5 mr-1" />
            {{ uiText('重新载入', 'Reload') }}
          </button>
        </div>
      </template>
    </PageToolbar>

    <!-- DNS Status Alert (shows when either saved is off, or draft differs from saved) -->
    <div
      v-if="dnsDisabledNotice"
      role="status"
      data-testid="dns-disabled-notice"
      class="alert bg-base-200/90 border border-base-300 text-base-content/80 shadow-xs py-2 px-3 text-xs flex items-center gap-2 rounded-xl"
    >
      <InformationCircleIcon class="w-4 h-4 text-warning shrink-0" />
      <span class="flex-1">{{ dnsDisabledNotice.text }}</span>
    </div>

    <!-- Loading / Status Alerts -->
    <div v-if="loading" role="status" class="alert shadow-xs py-2 px-3">
      <span class="loading loading-spinner loading-xs text-primary" />
      <span class="text-xs font-medium">{{ uiText('正在读取 DNS 设置…', 'Loading DNS settings…') }}</span>
    </div>

    <div v-if="error" role="alert" data-testid="dns-error" class="alert alert-error shadow-xs py-2 px-3">
      <span class="text-xs font-medium flex-1">{{ error }}</span>
      <button
        class="btn btn-sm btn-ghost border-current/30 text-xs"
        :disabled="busy"
        @click="load(false)"
      >
        {{ uiText('重试 / 重载', 'Retry / Reload') }}
      </button>
    </div>

    <div v-if="notice" role="status" class="alert alert-success shadow-xs py-2 px-3">
      <span class="text-xs font-medium">{{ notice }}</span>
    </div>

    <!-- Main Configuration Card -->
    <template v-if="settings">
      <section class="card bg-base-200/80 border border-base-300 rounded-2xl shadow-xs">
        <div class="card-body p-4 sm:p-6 space-y-6">
          <!-- Mode 1: Visual Editor -->
          <template v-if="mode === 'visual'">
            <!-- Section 1: Basic Settings -->
            <div class="space-y-3">
              <div class="flex items-center justify-between pb-1.5 border-b border-base-300/60">
                <div>
                  <h3 class="text-xs sm:text-sm font-bold uppercase tracking-wider text-base-content/90">
                    {{ uiText('基础设置', 'Basic Settings') }}
                  </h3>
                  <p class="text-[11px] text-base-content/60 mt-0.5">
                    {{ uiText('主要 DNS 上游服务器、解析增强模式及 IPv6 解析；未设置项均遵循客户端内核缺省。', 'Primary upstream DNS servers, enhanced mode and IPv6 resolution; unset fields use client core defaults.') }}
                  </p>
                </div>
              </div>

              <!-- Primary nameserver takes full width for clear multiline visibility -->
              <DnsFieldControl
                :field="DNS_BASIC_FIELDS[0]"
                :draft="draft"
                :field-error="fieldErrors[DNS_BASIC_FIELDS[0].key]"
                :invalid-json-input="invalidJsonInputs[DNS_BASIC_FIELDS[0].key]"
                :disabled="busy"
                @update-list="setListText"
                @update-scalar="setString"
                @update-bool="setBool"
                @update-json="setJsonText"
                @remove-key="removeKey"
              />

              <!-- Enhanced mode and IPv6 side by side -->
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 sm:gap-4">
                <DnsFieldControl
                  v-for="field in DNS_BASIC_FIELDS.slice(1)"
                  :key="field.key"
                  :field="field"
                  :draft="draft"
                  :field-error="fieldErrors[field.key]"
                  :invalid-json-input="invalidJsonInputs[field.key]"
                  :disabled="busy"
                  @update-list="setListText"
                  @update-scalar="setString"
                  @update-bool="setBool"
                  @update-json="setJsonText"
                  @remove-key="removeKey"
                />
              </div>
            </div>

            <!-- Section 2: Advanced Settings -->
            <div class="space-y-3 pt-2">
              <div class="flex items-center justify-between pb-1.5 border-b border-base-300/60">
                <div>
                  <h3 class="text-xs sm:text-sm font-bold uppercase tracking-wider text-base-content/90">
                    {{ uiText('高级配置', 'Advanced Settings') }}
                  </h3>
                  <p class="text-[11px] text-base-content/60 mt-0.5">
                    {{ uiText('按需配置引导服务器、Fake-IP 范围、回退策略及 Hosts 选项；未设置项遵循客户端缺省。', 'Configure bootstrap servers, Fake-IP range, fallback policy, and hosts options as needed; unset fields use client defaults.') }}
                  </p>
                </div>
              </div>

              <div class="space-y-3">
                <DnsGroupCard
                  v-for="group in DNS_GROUPS"
                  :key="group.id"
                  :group="group"
                  :draft="draft"
                  :field-errors="fieldErrors"
                  :invalid-json-inputs="invalidJsonInputs"
                  :disabled="busy"
                  @update-list="setListText"
                  @update-scalar="setString"
                  @update-bool="setBool"
                  @update-json="setJsonText"
                  @remove-key="removeKey"
                />
              </div>
            </div>

            <!-- Section 3: Preserved Custom / Extension Keys -->
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
                    class="badge badge-sm badge-outline font-mono text-[10px]"
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
          </template>

          <!-- Mode 2: Raw YAML Mode -->
          <div v-else class="space-y-2">
            <div class="flex items-center justify-between">
              <div>
                <span class="text-xs sm:text-sm font-semibold text-base-content/90">
                  {{ uiText('完整 YAML 配置草稿', 'Full YAML Configuration Draft') }}
                </span>
                <p class="text-[11px] text-base-content/60 mt-0.5">
                  {{ uiText('注意：保存时由服务端重新规范化序列化，不保留原始注释；支持直接编辑全部标准与自定义字段。', 'Note: Re-normalized by server on save without retaining comments; supports all standard & custom fields.') }}
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
              <div class="flex items-center gap-2">
                <EyeIcon class="w-4 h-4 text-primary" />
                <span class="text-xs font-bold text-base-content">
                  {{ uiText('当前草稿规范化 DNS 配置块预览', 'Current Draft Normalized DNS Block Preview') }}
                </span>
                <span class="badge badge-sm badge-primary text-[10px]">
                  {{ uiText('只读 (未持久化)', 'Read-only (not saved)') }}
                </span>
              </div>
              <button
                type="button"
                class="btn btn-ghost btn-xs btn-circle"
                data-testid="dns-close-preview"
                :aria-label="uiText('关闭预览', 'Close preview')"
                :title="uiText('关闭预览', 'Close preview')"
                @click="previewYaml = ''"
              >
                <XMarkIcon class="w-4 h-4" aria-hidden="true" />
              </button>
            </div>

            <p class="text-[11px] text-base-content/60">
              {{ uiText('此预览为当前草稿的服务端规范化 DNS 配置块（无注释），反映保存后导出订阅中的 dns: 配置结构；并非 DNS 连通性测试或已发布的客户端配置。', 'Normalized DNS configuration block for current draft (without comments); reflects dns: block in exported subscription once saved; not a DNS connectivity test or published configuration.') }}
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

            <div class="flex items-center gap-2 ml-auto">
              <!-- Preview YAML Button -->
              <button
                type="button"
                class="btn btn-sm btn-ghost border border-base-300"
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
                class="btn btn-sm btn-primary"
                data-testid="dns-save"
                :disabled="busy || !dirty || hasValidationErrors"
                @click="save"
              >
                <span v-if="saving" class="loading loading-spinner loading-xs mr-1" />
                {{ saving ? uiText('校验并保存…', 'Validating & saving…') : uiText('校验并保存', 'Validate & Save') }}
              </button>
            </div>
          </div>
        </div>
      </section>
    </template>
  </section>
</template>
