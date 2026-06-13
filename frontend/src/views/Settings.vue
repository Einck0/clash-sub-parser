<template>
  <section class="page settings-page">
    <div class="page-head">
      <div>
        <p class="eyebrow">Settings</p>
        <h2>安全设置</h2>
        <p class="page-desc">运行时调整 Web UI、API 与导出地址的 Token 鉴权，不需要重建容器。</p>
      </div>
      <div class="head-actions">
        <button @click="load" :disabled="loading || saving || isBusy">{{ loading ? '刷新中...' : '刷新' }}</button>
        <button class="primary" @click="save" :disabled="loading || saving || isBusy">{{ saving ? '保存中...' : '保存设置' }}</button>
      </div>
    </div>

    <UiState v-if="message" :type="messageType" :title="messageTitle" :description="message" compact />

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
            <input type="checkbox" v-model="settings.auth_enabled" />
            <span><strong>开启 Token 鉴权</strong><small>保护所选范围，访问时需要 token。</small></span>
          </label>
          <label class="settings-toggle">
            <input type="checkbox" v-model="settings.protect_frontend" :disabled="!settings.auth_enabled" />
            <span><strong>保护前端页面</strong><small>进入 Web UI 时显示 token 输入框；静态资源仍会放行。</small></span>
          </label>
          <label class="settings-toggle">
            <input type="checkbox" v-model="settings.protect_api" :disabled="!settings.auth_enabled" />
            <span><strong>保护管理 API</strong><small>订阅、节点组、规则、DNS、设置等接口需要 token。</small></span>
          </label>
          <label class="settings-toggle">
            <input type="checkbox" v-model="settings.protect_exports" :disabled="!settings.auth_enabled" />
            <span><strong>保护导出/订阅地址</strong><small>/yaml、/script 和下载地址需要 URL query token。</small></span>
          </label>
        </div>
      </div>

      <div class="dns-section settings-card">
        <h3>Token / 密码</h3>
        <p class="section-hint">留空表示不修改当前 token。新 token 至少 8 位；公开部署建议使用生成的长随机 token。</p>
        <label class="field settings-token-field">
          <span>新 token</span>
          <input v-model="newToken" :type="showToken ? 'text' : 'password'" autocomplete="new-password" placeholder="输入新的访问 token" />
        </label>
        <div class="row">
          <button type="button" @click="generateToken">生成随机 token</button>
        </div>
        <label class="switch-line">
          <input type="checkbox" v-model="showToken" /> 显示 token
        </label>
        <div class="settings-token-status">
          <span class="badge">当前：{{ settings.has_token ? '已设置 token' : '未设置 token' }}</span>
          <span class="badge" v-if="newToken">新 token：{{ newToken.length }} 位</span>
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
            <input type="checkbox" v-model="settings.fetch_proxy_enabled" />
            <span><strong>订阅拉取走代理</strong><small>只影响订阅拉取，不影响 Web UI 和生成接口。</small></span>
          </label>
        </div>
        <label class="field settings-token-field">
          <span>代理地址</span>
          <input v-model="settings.fetch_proxy_url" placeholder="例如：http://127.0.0.1:7890" />
        </label>
        <p class="section-hint">常见格式：<code>http://127.0.0.1:7890</code>。保存后新的订阅拉取会立即使用这个地址。</p>
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
          <button class="primary" @click="refreshPresetDownloads" :disabled="isBusy">{{ working === 'refresh-downloads' ? '刷新下载中...' : '刷新最新版客户端' }}</button>
          <button @click="loadDownloads" :disabled="isBusy">重新读取本地缓存</button>
        </div>
        <label class="field settings-token-field">
          <span>自定义下载 URL</span>
          <input v-model="customDownloadUrl" placeholder="https://example.com/file.apk 或 GitHub release asset URL" />
        </label>
        <div class="row" style="margin-top:8px">
          <button class="primary" @click="downloadCustom" :disabled="isBusy || !customDownloadUrl.trim()">{{ working === 'download-custom' ? '下载中...' : '下载自定义 URL' }}</button>
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
          <button @click="downloadConfig(true)" :disabled="isBusy">{{ working === 'export-full' ? '导出中...' : '导出（含订阅）' }}</button>
          <button @click="downloadConfig(false)" :disabled="isBusy">{{ working === 'export-no-subscriptions' ? '导出中...' : '导出（不含订阅）' }}</button>
          <label class="import-label" :class="{ disabled: isBusy }">
            <input type="file" accept=".json" @change="onImportFile" :disabled="isBusy" ref="importInput" />
            <span class="button-like">{{ working === 'import' ? '导入中...' : '导入配置' }}</span>
          </label>
          <button class="danger" @click="resetAllConfig" :disabled="isBusy">{{ working === 'reset' ? '重置中...' : '重置所有配置' }}</button>
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
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { exportAppConfig, getApiErrorMessage, getDownloads, getSecuritySettings, importAppConfig, loginAuthToken, resetAppConfig, updateSecuritySettings, downloadCustomAsset, downloadPresetAsset } from '../api'
import { setAuthToken, withAuthToken } from '../auth'
import UiState from '../components/UiState.vue'

const settings = reactive({
  auth_enabled: false,
  protect_frontend: true,
  protect_api: true,
  protect_exports: true,
  has_token: false,
  fetch_proxy_enabled: false,
  fetch_proxy_url: '',
})
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

const isBusy = computed(() => Boolean(working.value))
const messageTitle = computed(() => messageType.value === 'error' ? '保存失败' : '设置已更新')
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
    const { data } = await getSecuritySettings()
    Object.assign(settings, data)
  } catch (err) {
    setMessage(getApiErrorMessage(err, '加载安全设置失败'), 'error')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  message.value = ''
  try {
    const payload = {
      auth_enabled: settings.auth_enabled,
      protect_frontend: settings.protect_frontend,
      protect_api: settings.protect_api,
      protect_exports: settings.protect_exports,
      fetch_proxy_enabled: settings.fetch_proxy_enabled,
      fetch_proxy_url: settings.fetch_proxy_url?.trim() || '',
    }
    if (newToken.value) payload.token = newToken.value
    const { data } = await updateSecuritySettings(payload)
    Object.assign(settings, data)
    if (newToken.value) {
      await loginAuthToken(newToken.value)
      setAuthToken(newToken.value)
    }
    newToken.value = ''
    showToken.value = false
    setMessage('安全设置已保存。管理界面已使用 HttpOnly cookie 登录；导出链接仅在本次页面会话中使用当前 token。', 'success')
  } catch (err) {
    setMessage(getApiErrorMessage(err, '保存安全设置失败'), 'error')
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
    setMessage(`客户端已刷新：${results.join('、')}。`, 'success')
  } catch (err) {
    setMessage(getApiErrorMessage(err, '刷新最新版客户端失败'), 'error')
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
    setMessage(`已下载：${data.filename}（${formatBytes(data.size)}）。`, 'success')
  } catch (err) {
    setMessage(getApiErrorMessage(err, '下载失败'), 'error')
  } finally {
    working.value = ''
  }
}

function formatBytes(value) {
  const size = Number(value || 0)
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`
  return `${(size / 1024 / 1024 / 1024).toFixed(1)} GB`
}

function formatDate(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
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
    // Get filename from Content-Disposition header, or use default
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
    setMessage('配置已导出。', 'success')
  } catch (err) {
    setMessage(getApiErrorMessage(err, '导出配置失败'), 'error')
  } finally {
    working.value = ''
  }
}

function onImportFile(event) {
  const file = event.target.files?.[0]
  if (!file) return
  const ok = confirm(`确定要导入配置文件 “${file.name}” 吗？\n\n导入会覆盖当前配置中对应的数据表（订阅、节点组、规则、DNS、生成设置等）。建议先导出备份。`)
  if (!ok) {
    if (importInput.value) importInput.value.value = ''
    return
  }
  const ok2 = confirm('最后确认：导入后会覆盖当前配置，无法从界面撤销。确定继续吗？')
  if (!ok2) {
    if (importInput.value) importInput.value.value = ''
    return
  }
  doImport(file)
}

async function doImport(file) {
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
    setMessage(msg, errors ? 'info' : 'success')
    // Reload settings in case security_settings was imported
    await load()
  } catch (err) {
    setMessage(getApiErrorMessage(err, '导入配置失败'), 'error')
  } finally {
    working.value = ''
    if (importInput.value) importInput.value.value = ''
  }
}

async function resetAllConfig() {
  const ok = confirm('确定要重置所有配置吗？\n\n这会清空订阅、节点组、规则、DNS、生成和安全设置，覆盖当前配置，无法从界面撤销。建议先导出备份。')
  if (!ok) return
  const ok2 = confirm('最后确认：真的要清空并恢复成新安装状态吗？这会覆盖当前配置。')
  if (!ok2) return
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
    setMessage('所有配置已重置为新安装状态。', 'success')
  } catch (err) {
    setMessage(getApiErrorMessage(err, '重置配置失败'), 'error')
  } finally {
    working.value = ''
  }
}

function generateToken() {
  const bytes = new Uint8Array(32)
  crypto.getRandomValues(bytes)
  const binary = Array.from(bytes, (byte) => String.fromCharCode(byte)).join('')
  newToken.value = btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
  showToken.value = true
}
</script>

<style scoped>
.import-label {
  display: inline-flex;
  cursor: pointer;
}
.import-label input[type="file"] {
  display: none;
}
.button-like {
  display: inline-block;
  padding: 6px 14px;
  border: 1px solid var(--color-border, #444);
  border-radius: 6px;
  background: var(--color-bg-secondary, #2a2a2a);
  color: var(--color-text, #eee);
  font-size: 0.875rem;
  line-height: 1.4;
  text-align: center;
  cursor: pointer;
  user-select: none;
}
.button-like:hover {
  background: var(--color-bg-tertiary, #333);
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
