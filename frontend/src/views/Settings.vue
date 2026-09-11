<template>
  <section class="page settings-page">
    <div class="page-head">
      <div>
        <p class="eyebrow">Settings</p>
        <h2>安全设置</h2>
        <p class="page-desc">运行时调整 Web UI、API 与导出地址的 Token 鉴权，不需要重建容器。</p>
      </div>
      <div class="head-actions">
        <Button variant="secondary" size="md" :disabled="loading || saving || isBusy" @click="load">{{ loading ? '刷新中...' : '刷新' }}</Button>
        <Button variant="primary" size="md" :loading="saving" :disabled="loading || isBusy" @click="save">{{ saving ? '保存中...' : '保存设置' }}</Button>
      </div>
    </div>

    <UiState v-if="message" :type="messageType" :title="messageType === 'error' ? '保存失败' : '设置已更新'" :description="message" compact />

    <div class="settings-layout">
      <div class="dns-section settings-card">
        <div class="section-title-row">
          <div>
            <h3>鉴权开关</h3>
            <p class="section-hint">总开关关闭时，下面的保护范围不会生效。</p>
          </div>
          <span class="sync-pill" :class="{ error: settings.auth_enabled && !settings.has_token }">
            {{ settings.auth_enabled ? '已开启' : '未开启' }}
          </span>
        </div>

        <div class="settings-toggle-list">
          <label class="settings-toggle">
            <Checkbox v-model="settings.auth_enabled" />
            <span><strong>开启 Token 鉴权</strong><small>保护所选范围，访问时需要 token。</small></span>
          </label>
          <label class="settings-toggle">
            <Checkbox v-model="settings.protect_frontend" :disabled="!settings.auth_enabled" />
            <span><strong>保护前端页面</strong><small>进入 Web UI 时显示 token 输入框；静态资源仍会放行。</small></span>
          </label>
          <label class="settings-toggle">
            <Checkbox v-model="settings.protect_api" :disabled="!settings.auth_enabled" />
            <span><strong>保护管理 API</strong><small>订阅、节点组、规则、DNS、设置等接口需要 token。</small></span>
          </label>
          <label class="settings-toggle">
            <Checkbox v-model="settings.protect_exports" :disabled="!settings.auth_enabled" />
            <span><strong>保护导出/订阅地址</strong><small>/yaml 和下载地址需要 URL query token。</small></span>
          </label>
        </div>
      </div>

      <div class="dns-section settings-card">
        <h3>Token / 密码</h3>
        <p class="section-hint">留空表示不修改当前 token。新 token 至少 8 位；公开部署建议使用生成的长随机 token。</p>
        <div class="field settings-token-field">
          <span>新 token</span>
          <div class="relative flex items-center">
            <Input
              v-model="newToken"
              :type="showToken ? 'text' : 'password'"
              autocomplete="new-password"
              placeholder="输入新的访问 token"
              @input="onManualTokenInput"
              style="padding-right: 36px; width: 100%;"
            />
            <Button
              type="button"
              class="absolute right-2 text-text-muted hover:text-text-main cursor-pointer"
              style="background: transparent; border: none; padding: 4px;"
              :aria-label="showToken ? '隐藏 Token' : '显示 Token'"
              :title="showToken ? '隐藏 Token' : '显示 Token'"
              @click="showToken = !showToken"
            >
              <EyeOff v-if="showToken" :size="16" />
              <Eye v-else :size="16" />
            </Button>
          </div>
        </div>
        <div class="row" style="margin-top: 8px;">
          <Button type="button" @click="generateToken">生成随机 token</Button>
        </div>
        <div class="settings-token-status" style="margin-top: 8px;">
          <span class="badge">当前：{{ settings.has_token ? '已设置 token' : '未设置 token' }}</span>
          <span class="badge" v-if="newToken">新 token：{{ newToken.length }} 位</span>
          <span class="badge ok" v-if="isGeneratedToken && tokenAcknowledged">已确认保存</span>
          <span class="badge error" v-else-if="isGeneratedToken && !tokenAcknowledged">待确认保存</span>
        </div>
      </div>

      <div class="dns-section settings-card">
        <div class="section-title-row">
          <div>
            <h3>订阅拉取代理</h3>
            <p class="section-hint">用于订阅 URL 拉取节点。开启后，后端会显式使用这里的代理地址请求订阅，而不是依赖容器环境变量。</p>
          </div>
          <span class="sync-pill" :class="{ error: settings.fetch_proxy_enabled && !settings.fetch_proxy_url }">
            {{ settings.fetch_proxy_enabled ? '已开启' : '未开启' }}
          </span>
        </div>
        <div class="settings-toggle-list">
          <label class="settings-toggle">
            <Checkbox v-model="settings.fetch_proxy_enabled" />
            <span><strong>订阅拉取走代理</strong><small>只影响订阅拉取，不影响 Web UI 和生成接口。</small></span>
          </label>
        </div>
        <label class="field settings-token-field">
          <span>代理地址</span>
          <Input v-model="settings.fetch_proxy_url" placeholder="例如：http://127.0.0.1:7890" />
        </label>
        <p class="section-hint">常见格式：<code>http://127.0.0.1:7890</code>。保存后新的订阅拉取会立即使用这个地址。</p>
      </div>

      <div class="dns-section settings-card wide">
        <div class="section-title-row">
          <div>
            <h3><Activity :size="16" aria-hidden="true" /> 节点检测与测速设置</h3>
            <p class="section-hint">配置全协议代理握手、真实出口 IP/国家识别、流媒体与 AI 解锁测试、服务级超时及定时质检调度。</p>
          </div>
          <span class="sync-pill" :class="{ ok: probeConfig.probe_enabled }">
            {{ probeConfig.probe_enabled ? '质检总开关已开启' : '质检总开关已暂停' }}
          </span>
        </div>

        <div class="settings-toggle-list" style="margin-bottom: 16px;">
          <label class="settings-toggle">
            <Checkbox v-model="probeConfig.probe_enabled" />
            <span><strong>开启节点出站校验（主开关）</strong><small>通过 sing-box 建立独立通道验证真实代理协议握手与延迟。关闭时暂停全部手动与定时质检。</small></span>
          </label>
          <label class="settings-toggle">
            <Checkbox v-model="probeConfig.probe_cron_enabled" :disabled="!probeConfig.probe_enabled" />
            <span><strong>开启后台定时质检</strong><small>按设定周期自动在后台对所有节点执行完整质检，受主开关控制。</small></span>
          </label>
          <label class="settings-toggle">
            <Checkbox v-model="probeConfig.media_check_enabled" :disabled="!probeConfig.probe_enabled" />
            <span><strong>开启流媒体与 AI 解锁检测</strong><small>通过待测节点探测 YouTube、Netflix、Disney+、ChatGPT 等平台的解锁能力。</small></span>
          </label>
          <label class="settings-toggle">
            <Checkbox v-model="probeConfig.speedtest_enabled" :disabled="!probeConfig.probe_enabled" />
            <span><strong>开启下载带宽测速</strong><small>通过小样本流量分块（受控流量）测量节点的实际下载速度 (Mbps)。</small></span>
          </label>
        </div>

        <div v-if="probeConfig.media_check_enabled" class="settings-sub-panel" style="margin-bottom: 16px;">
          <h4 style="margin: 0 0 8px; font-size: 0.95rem;">检测的流媒体 / AI 平台</h4>
          <div class="platform-chips" style="display: flex; gap: 8px; flex-wrap: wrap;">
            <label
              v-for="p in availablePlatforms"
              :key="p.id"
              class="platform-chip"
              :class="{ active: probeConfig.media_platforms.includes(p.id) }"
            >
              <Checkbox
                :model-value="probeConfig.media_platforms.includes(p.id)"
                @update:model-value="toggleMediaPlatform(p.id, $event)"
              />
              <span>{{ p.name }}</span>
            </label>
          </div>
        </div>

        <div class="settings-grid-2" style="display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 12px; margin-bottom: 12px;">
          <label class="field">
            <span class="field-title-hint">后台定时质检周期 (分钟)</span>
            <Input
              type="number"
              min="1"
              max="1440"
              v-model.number="probeConfig.probe_cron_interval_minutes"
              :disabled="!probeConfig.probe_enabled || !probeConfig.probe_cron_enabled"
              placeholder="60"
            />
            <small style="color: var(--text-muted); font-size: 0.8rem;">
              定时自动质检的时间间隔 (1–1440 分钟，默认 60 分钟)；设置保存后于下一调度周期生效。
            </small>
          </label>
          <label class="field">
            <span>服务级独立超时 (毫秒)</span>
            <Input
              type="number"
              min="500"
              max="30000"
              step="500"
              v-model.number="probeConfig.probe_service_timeout_ms"
              placeholder="2000"
            />
            <small style="color: var(--text-muted); font-size: 0.8rem;">
              独立作用于每个握手、出口定位、平台解锁或测速服务，非整批或整节点超时 (500–30000ms，默认 2000ms)。
            </small>
          </label>
          <label class="field">
            <span>探测最大并发数</span>
            <Input
              type="number"
              min="1"
              max="20"
              v-model.number="probeConfig.probe_concurrency"
              placeholder="10"
            />
            <small style="color: var(--text-muted); font-size: 0.8rem;">
              单批次同时执行质检的节点工作流上限 (1–20，默认 10)。
            </small>
          </label>
          <label class="field">
            <span>单节点总预算超时 (毫秒，可选)</span>
            <Input
              type="number"
              min="0"
              max="60000"
              step="500"
              v-model.number="probeConfig.probe_timeout_ms"
              placeholder="0 为不设总预算"
            />
            <small style="color: var(--text-muted); font-size: 0.8rem;">
              单节点端到端整体执行预算，0 表示不设硬限；各服务独立受上述服务级超时约束。
            </small>
          </label>
          <label class="field">
            <span class="field-title-hint">测速目标 URL</span>
            <Input v-model="probeConfig.speedtest_url" placeholder="https://speed.cloudflare.com/__down?bytes=5000000" />
          </label>
          <label class="field">
            <span>最大测速样本流量 (MB)</span>
            <Input
              type="number"
              min="1"
              max="50"
              :value="Math.round(probeConfig.speedtest_max_bytes / 1048576)"
              @input="probeConfig.speedtest_max_bytes = Math.max(1, Number($event.target.value || 5)) * 1048576"
            />
          </label>
          <label class="field">
            <span>测速超时时间 (秒)</span>
            <Input type="number" min="2" max="30" v-model.number="probeConfig.speedtest_timeout_s" />
          </label>
          <label class="field">
            <span>测速达标过滤阈值 (Mbps)</span>
            <Input type="number" min="0" step="0.5" v-model.number="probeConfig.speedtest_min_speed_mbps" placeholder="0 表示不设门槛" />
          </label>
        </div>
      </div>

      <div class="dns-section settings-card wide">
        <div class="section-title-row">
          <div>
            <h3>客户端下载</h3>
            <p class="section-hint">从 GitHub 拉取 Clash Verge Rev Windows x64 和 Clash Meta for Android arm64 最新版；也可以输入自定义 URL 下载到服务端后再从这里取回。</p>
          </div>
          <span class="sync-pill">{{ downloadItems.length }} 个文件</span>
        </div>
        <div class="row" style="gap:8px;flex-wrap:wrap;margin-bottom:12px">
          <Button class="primary" @click="refreshPresetDownloads" :disabled="isBusy">{{ working === 'refresh-downloads' ? '刷新下载中...' : '刷新最新版客户端' }}</Button>
          <Button @click="loadDownloads" :disabled="isBusy">重新读取本地缓存</Button>
        </div>
        <label class="field settings-token-field">
          <span>自定义下载 URL</span>
          <Input v-model="customDownloadUrl" placeholder="https://example.com/file.apk 或 GitHub release asset URL" />
        </label>
        <div class="row" style="margin-top:8px">
          <Button class="primary" @click="downloadCustom" :disabled="isBusy || !customDownloadUrl.trim()">{{ working === 'download-custom' ? '下载中...' : '下载自定义 URL' }}</Button>
        </div>
        <div v-if="downloadItems.length" class="download-list">
          <div v-for="item in downloadItems" :key="item.filename" class="download-row">
            <div>
              <strong>{{ item.filename }}</strong>
              <small>{{ formatBytes(item.size) }} · {{ formatDate(item.mtime) }}</small>
            </div>
            <a class="quick-link" :href="withAuthToken(item.download_url, exportNeedsToken)" target="_blank" rel="noreferrer">下载</a>
          </div>
        </div>
        <p v-else class="section-hint">暂无已下载文件。</p>
      </div>

      <div class="dns-section settings-card wide">
        <h3>配置备份 / 重置</h3>
        <p class="section-hint">导出的 JSON 不包含访问 token/hash。重置会清空订阅、节点组、规则、DNS、生成和安全设置，恢复成新安装状态。导入会覆盖对应的数据表。</p>
        <div class="row" style="gap:8px;flex-wrap:wrap">
          <Button @click="downloadConfig(true)" :disabled="isBusy">{{ working === 'export-full' ? '导出中...' : '导出（含订阅）' }}</Button>
          <Button @click="downloadConfig(false)" :disabled="isBusy">{{ working === 'export-no-subscriptions' ? '导出中...' : '导出（不含订阅）' }}</Button>
          <label class="import-label" :class="{ disabled: isBusy }">
            <input type="file" accept=".json" @change="onImportFile" :disabled="isBusy" ref="importInput" />
            <span class="button-like">{{ working === 'import' ? '导入中...' : '导入配置' }}</span>
          </label>
          <Button class="danger" @click="resetAllConfig" :disabled="isBusy">{{ working === 'reset' ? '重置中...' : '重置所有配置' }}</Button>
        </div>
      </div>

      <div class="dns-section settings-card wide">
        <h3>使用方式</h3>
        <div class="settings-help-grid">
          <div>
            <strong>进入管理界面</strong>
            <code>{{ uiUrlExample }}</code>
          </div>
          <div>
            <strong>Clash 订阅地址</strong>
            <code>{{ yamlUrlExample }}</code>
          </div>
          <div>
            <strong>前端 API</strong>
            <code>X-Clash-Token: &lt;token&gt;</code>
          </div>
        </div>
        <p class="section-hint">管理界面登录使用 HttpOnly cookie；如果关闭 API 鉴权但开启导出鉴权，Clash 订阅地址仍需要 URL token。</p>
      </div>
    </div>

    <!-- Generated Token Recovery & Acknowledgement Modal -->
    <AppModal
      v-model="showGeneratedModal"
      title="已生成新访问 Token"
      size="md"
      @close="onCloseGeneratedModal"
    >
      <div class="space-y-4">
        <div class="p-3 rounded-md border border-status-warning/30 bg-status-warning/10 text-xs text-status-warning font-mono">
          警告：请务必立即复制并妥善保存下方 Token。关闭此弹窗后将无法再次查看该明文。在您勾选确认已保存前，无法保存设置。
        </div>

        <div class="space-y-1.5">
          <label class="block text-xs font-mono text-text-muted">新生成的随机 Token</label>
          <div class="flex items-center gap-2">
            <Input
              :value="generatedTokenValue"
              readonly
              class="flex-1 min-h-[44px] rounded-md border border-border bg-surface-base px-3.5 py-2 font-mono text-xs text-text-main select-all focus:outline-hidden"
            />
            <Button
              type="button"
              class="min-h-[44px] px-4 py-2 rounded-md bg-accent text-xs font-medium text-white hover:bg-accent-hover transition-colors cursor-pointer whitespace-nowrap shrink-0"
              @click="copyGeneratedToken"
            >
              {{ tokenCopied ? '已复制' : '复制 Token' }}
            </Button>
          </div>
        </div>

        <label class="flex items-center gap-2.5 text-xs text-text-main cursor-pointer select-none pt-2">
          <Checkbox
            type="checkbox"
            v-model="tokenAcknowledged"
            :disabled="!tokenCopied"
            class="rounded border-border text-accent focus-ring cursor-pointer"
          />
          <span>我已复制并妥善保存此 Token，确认应用到设置</span>
        </label>

        <div class="flex justify-end gap-2.5 pt-4 border-t border-border-subtle">
          <Button
            type="button"
            class="min-h-[36px] px-4 py-1.5 rounded-md border border-border bg-surface-hover text-xs text-text-muted hover:text-text-main cursor-pointer"
            @click="onCloseGeneratedModal"
          >
            取消
          </Button>
          <Button
            type="button"
            class="min-h-[36px] px-4 py-1.5 rounded-md bg-accent text-xs font-semibold text-white hover:bg-accent-hover transition-colors cursor-pointer disabled:opacity-50"
            :disabled="!tokenAcknowledged"
            @click="applyGeneratedToken"
          >
            采纳并填入表单
          </Button>
        </div>
      </div>
    </AppModal>
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { Activity, Eye, EyeOff } from 'lucide-vue-next'
import { Button, Input, Checkbox } from '../components/ui'
import { useAppStore } from '../stores/app'
import AppModal from '../components/ui/AppModal.vue'
import { formatBytes, formatDate } from '../utils/format'
import {
  exportAppConfig,
  getApiErrorMessage,
  getDownloads,
  getProbeSettings,
  getSecuritySettings,
  importAppConfig,
  loginAuthToken,
  resetAppConfig,
  updateProbeSettings,
  updateSecuritySettings,
  downloadCustomAsset,
  downloadPresetAsset,
} from '../api'
import { setAuthToken, withAuthToken } from '../auth'
import UiState from '../components/UiState.vue'

const store = useAppStore()

const settings = reactive({
  auth_enabled: false,
  protect_frontend: true,
  protect_api: true,
  protect_exports: true,
  has_token: false,
  fetch_proxy_enabled: false,
  fetch_proxy_url: '',
})

const probeConfig = reactive({
  probe_enabled: true,
  probe_interval_minutes: 0,
  speedtest_enabled: false,
  speedtest_url: 'https://speed.cloudflare.com/__down?bytes=5000000',
  speedtest_timeout_s: 5,
  speedtest_max_bytes: 5242880,
  speedtest_min_speed_mbps: 0.0,
  media_check_enabled: true,
  media_platforms: ['youtube', 'netflix', 'disney', 'chatgpt', 'bilibili', 'meta_ai', 'gemini'],
  media_timeout_s: 5,
  probe_concurrency: 10,
  probe_timeout_ms: 3000,
  probe_service_timeout_ms: 2000,
  probe_cron_enabled: true,
  probe_cron_interval_minutes: 60,
})

const availablePlatforms = [
  { id: 'youtube', name: 'YouTube Premium' },
  { id: 'netflix', name: 'Netflix' },
  { id: 'disney', name: 'Disney+' },
  { id: 'chatgpt', name: 'ChatGPT / OpenAI' },
  { id: 'gemini', name: 'Google Gemini' },
  { id: 'meta_ai', name: 'Meta AI' },
  { id: 'bilibili', name: 'Bilibili 港澳台' },
]

const newToken = ref('')
const showToken = ref(false)
const loading = ref(false)
const saving = ref(false)
const working = ref('')
const message = ref('')
const messageType = ref('info')
const importInput = ref(null)
const customDownloadUrl = ref('')
const downloadItems = ref([])

const persistedAuthEnabled = ref(false)
const isGeneratedToken = ref(false)
const tokenAcknowledged = ref(false)
const tokenCopied = ref(false)
const showGeneratedModal = ref(false)
const generatedTokenValue = ref('')

function toggleMediaPlatform(platformId, checked) {
  const platforms = new Set(probeConfig.media_platforms)
  if (checked) platforms.add(platformId)
  else platforms.delete(platformId)
  probeConfig.media_platforms = [...platforms]
}

function onManualTokenInput() {
  isGeneratedToken.value = false
  tokenAcknowledged.value = true
}

const isBusy = computed(() => Boolean(working.value))
const uiUrlExample = computed(() => `${window.location.origin}/（页面输入框填写 token）`)
const exportNeedsToken = computed(() => Boolean(settings.auth_enabled && settings.protect_exports))
const yamlUrlExample = computed(() => withAuthToken(`${window.location.origin}/yaml`, exportNeedsToken.value))

onMounted(async () => {
  await load()
  await loadDownloads()
})

async function load() {
  loading.value = true
  message.value = ''
  try {
    const [secRes, probeRes] = await Promise.all([
      getSecuritySettings(),
      getProbeSettings().catch(() => ({ data: null })),
    ])
    if (secRes?.data) {
      Object.assign(settings, secRes.data)
      persistedAuthEnabled.value = Boolean(secRes.data.auth_enabled)
    }
    if (probeRes?.data) Object.assign(probeConfig, probeRes.data)
  } catch (err) {
    setMessage(getApiErrorMessage(err, '加载设置失败'), 'error')
  } finally {
    loading.value = false
  }
}

async function save() {
  if (isGeneratedToken.value && !tokenAcknowledged.value) {
    store.warning('已生成的 Token 尚未确认妥善保存，无法保存设置')
    return
  }

  // Dangerous confirmation on auth_enabled: true -> false
  if (persistedAuthEnabled.value && !settings.auth_enabled) {
    const confirmed = await store.confirm({
      title: '危险操作：停用 Token 鉴权',
      message: '关闭 Token 鉴权将导致 Web 控制台、API 及导出地址完全开放，任何能够访问服务的人均可查看节点与配置。\n\n确定要停用鉴权吗？',
      confirmText: '确认停用鉴权',
      cancelText: '取消',
      danger: true,
    })
    if (!confirmed) {
      settings.auth_enabled = true
      store.info('已取消停用鉴权，设置未保存')
      return
    }
  }

  saving.value = true
  message.value = ''
  try {
    const secPayload = {
      auth_enabled: settings.auth_enabled,
      protect_frontend: settings.protect_frontend,
      protect_api: settings.protect_api,
      protect_exports: settings.protect_exports,
      fetch_proxy_enabled: settings.fetch_proxy_enabled,
      fetch_proxy_url: settings.fetch_proxy_url?.trim() || '',
    }
    const tokenToSave = newToken.value
    if (tokenToSave) secPayload.token = tokenToSave

    const [secRes, probeRes] = await Promise.all([
      updateSecuritySettings(secPayload),
      updateProbeSettings(probeConfig),
    ])

    if (secRes?.data) {
      Object.assign(settings, secRes.data)
      persistedAuthEnabled.value = Boolean(secRes.data.auth_enabled)
    }
    if (probeRes?.data) Object.assign(probeConfig, probeRes.data)

    if (tokenToSave) {
      await loginAuthToken(tokenToSave)
    }
    newToken.value = ''
    isGeneratedToken.value = false
    tokenAcknowledged.value = false
    showToken.value = false
    store.success('所有设置已保存')
  } catch (err) {
    store.error(getApiErrorMessage(err, '保存设置失败'))
  } finally {
    saving.value = false
  }
}

async function loadDownloads() {
  try {
    const { data } = await getDownloads()
    downloadItems.value = data.items || []
  } catch (err) {
    setMessage(getApiErrorMessage(err, '加载下载列表失败'), 'error')
  }
}

async function refreshPresetDownloads() {
  working.value = 'refresh-downloads'
  message.value = ''
  try {
    const targets = ['clash-verge-rev-windows-x64', 'clash-meta-android-arm64']
    const results = []
    for (const presetId of targets) {
      const { data } = await downloadPresetAsset(presetId)
      results.push(`${data.filename}（${formatBytes(data.size)}）`)
    }
    await loadDownloads()
    store.success(`客户端已刷新：${results.join('、')}`)
  } catch (err) {
    store.error(getApiErrorMessage(err, '刷新最新版客户端失败'))
  } finally {
    working.value = ''
  }
}

async function downloadCustom() {
  const url = customDownloadUrl.value.trim()
  if (!url) return
  working.value = 'download-custom'
  message.value = ''
  try {
    const { data } = await downloadCustomAsset(url)
    customDownloadUrl.value = ''
    await loadDownloads()
    store.success(`已下载：${data.filename}（${formatBytes(data.size)}）`)
  } catch (err) {
    store.error(getApiErrorMessage(err, '下载失败'))
  } finally {
    working.value = ''
  }
}

function setMessage(text, type) {
  message.value = text
  messageType.value = type
}

async function downloadConfig(includeSubscriptions) {
  working.value = includeSubscriptions ? 'export-full' : 'export-no-subscriptions'
  message.value = ''
  try {
    const response = await exportAppConfig(includeSubscriptions)
    if (!response.ok) {
      let errData = null
      try { errData = await response.json() } catch {}
      throw new Error(errData?.detail || `HTTP ${response.status}`)
    }
    const blob = await response.blob()
    const disposition = response.headers.get('content-disposition') || ''
    const match = disposition.match(/filename="?([^"\s]+)"?/)
    const filename = match ? match[1] : (includeSubscriptions ? 'clash-sub-parser-full.json' : 'clash-sub-parser-no-subscriptions.json')
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = filename
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    URL.revokeObjectURL(url)
    store.success('配置已导出')
  } catch (err) {
    store.error(getApiErrorMessage(err, '导出配置失败'))
  } finally {
    working.value = ''
  }
}

async function onImportFile(event) {
  const file = event.target.files?.[0]
  if (!file) return
  const ok = await store.confirm({
    title: '导入配置',
    message: `确定要导入 "${file.name}" 吗？\n\n导入会覆盖当前配置（订阅、节点组、规则、DNS 等），无法撤销。建议先导出备份。`,
    confirmText: '确认导入',
    danger: true,
  })
  if (!ok) {
    if (importInput.value) importInput.value.value = ''
    return
  }
  doImport(file)
}

async function doImport(file) {
  const MAX_IMPORT_SIZE = 50 * 1024 * 1024 // 50MB safety limit
  if (file.size > MAX_IMPORT_SIZE) {
    store.error('文件过大，请检查是否选择了正确的文件（最大 50MB）')
    return
  }
  working.value = 'import'
  message.value = ''
  try {
    const text = await file.text()
    let data
    try {
      data = JSON.parse(text)
    } catch {
      throw new Error('文件不是有效的 JSON')
    }
    if (!data.tables || typeof data.tables !== 'object') {
      throw new Error('配置文件格式错误：缺少 tables 字段')
    }
    const result = await importAppConfig(data)
    const imported = result.data?.imported || {}
    const errors = result.data?.errors
    const summary = Object.entries(imported)
      .map(([table, count]) => `${table}: ${count} 条`)
      .join('、')
    let msg = `导入完成。${summary || '无数据'}`
    if (errors) {
      const errSummary = Object.entries(errors)
        .map(([table, err]) => `${table}: ${err}`)
        .join('；')
      msg += `。部分失败：${errSummary}`
    }
    store.success(msg)
    await load()
  } catch (err) {
    store.error(getApiErrorMessage(err, '导入配置失败'))
  } finally {
    working.value = ''
    if (importInput.value) importInput.value.value = ''
  }
}

async function resetAllConfig() {
  const ok = await store.confirm({
    title: '重置所有配置',
    message: '确定要清空所有配置并恢复成新安装状态吗？\n\n影响范围：\n- 订阅 / 节点 / 策略组\n- 规则分类与规则\n- DNS / 生成开关\n- 安全设置（token 等）\n- 链式绑定\n\n无法撤销。建议先导出备份。',
    confirmText: '确认重置',
    danger: true,
  })
  if (!ok) return
  working.value = 'reset'
  message.value = ''
  try {
    await resetAppConfig()
    setAuthToken('')
    Object.assign(settings, {
      auth_enabled: false,
      protect_frontend: true,
      protect_api: true,
      protect_exports: true,
      has_token: false,
      fetch_proxy_enabled: false,
      fetch_proxy_url: '',
    })
    store.success('所有配置已重置为新安装状态')
  } catch (err) {
    store.error(getApiErrorMessage(err, '重置配置失败'))
  } finally {
    working.value = ''
  }
}

function generateToken() {
  const bytes = new Uint8Array(32)
  crypto.getRandomValues(bytes)
  const binary = Array.from(bytes, (byte) => String.fromCharCode(byte)).join('')
  generatedTokenValue.value = btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
  tokenCopied.value = false
  tokenAcknowledged.value = false
  showGeneratedModal.value = true
}

async function copyGeneratedToken() {
  if (!generatedTokenValue.value) return
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(generatedTokenValue.value)
    } else {
      const textarea = document.createElement('textarea')
      textarea.value = generatedTokenValue.value
      textarea.style.position = 'fixed'
      textarea.style.opacity = '0'
      document.body.appendChild(textarea)
      textarea.select()
      document.execCommand('copy')
      document.body.removeChild(textarea)
    }
    tokenCopied.value = true
    store.success('Token 已成功复制到剪贴板')
  } catch (err) {
    tokenCopied.value = true
    store.warning('无法自动写入剪贴板，请手动选中文本并复制')
  }
}

function applyGeneratedToken() {
  if (!tokenAcknowledged.value) return
  newToken.value = generatedTokenValue.value
  isGeneratedToken.value = true
  showGeneratedModal.value = false
  showToken.value = true
  store.info('已将生成的 Token 填入设置，请点击“保存所有设置”以使其生效')
}

function onCloseGeneratedModal() {
  showGeneratedModal.value = false
}
</script>

<style>
.import-label {
  display: inline-flex;
  cursor: pointer;
}
.platform-chip {
  cursor: pointer;
  padding: 6px 12px;
  border-radius: 6px;
  border: 1px solid var(--border);
  font-size: 0.85rem;
  display: flex;
  align-items: center;
  gap: 6px;
}
.import-label input[type="file"] {
  display: none;
}
.button-like {
  display: inline-block;
  padding: 6px 14px;
  border: 1px solid var(--color-border, #444);
  border-radius: 6px;
  background: var(--color-surface-base);
  color: var(--color-text-main);
  font-size: 0.875rem;
  line-height: 1.4;
  text-align: center;
  cursor: pointer;
  user-select: none;
}
.button-like:hover {
  background: var(--color-surface-hover);
}
.import-label.disabled {
  pointer-events: none;
}
.import-label input:disabled + .button-like,
.import-label.disabled .button-like {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
