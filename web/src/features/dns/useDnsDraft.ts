import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import yaml from 'js-yaml'
import { api } from '../../api/client'
import { uiText } from '../../locales/redesign'
import { useUnsavedChanges } from '../../composables/useUnsavedChanges'
import type { DnsSettings, ValidatedDns } from './dnsTypes'
import { displayText } from '../../utils/displayNames'

function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true
  if (typeof a !== 'object' || a === null || typeof b !== 'object' || b === null) return false
  if (Array.isArray(a) !== Array.isArray(b)) return false
  if (Array.isArray(a) && Array.isArray(b)) {
    if (a.length !== b.length) return false
    for (let i = 0; i < a.length; i++) {
      if (!deepEqual(a[i], b[i])) return false
    }
    return true
  }
  const keysA = Object.keys(a as Record<string, unknown>)
  const keysB = Object.keys(b as Record<string, unknown>)
  if (keysA.length !== keysB.length) return false
  for (const key of keysA) {
    if (!Object.prototype.hasOwnProperty.call(b, key)) return false
    if (!deepEqual((a as Record<string, unknown>)[key], (b as Record<string, unknown>)[key])) return false
  }
  return true
}

function isPlainObject(val: unknown): val is Record<string, unknown> {
  return typeof val === 'object' && val !== null && !Array.isArray(val)
}

function cloneValue<T>(val: T, ancestors = new WeakSet<object>()): T {
  if (val === null || typeof val !== 'object') return val
  if (ancestors.has(val)) {
    throw new Error(uiText('DNS YAML 不支持递归引用', 'DNS YAML does not support recursive references'))
  }
  ancestors.add(val)
  try {
    if (Array.isArray(val)) return val.map((item) => cloneValue(item, ancestors)) as unknown as T
    // Define own properties so extension keys such as __proto__ remain ordinary data.
    return Object.fromEntries(
      Object.entries(val).map(([key, value]) => [key, cloneValue(value, ancestors)] as const)
    ) as unknown as T
  } finally {
    ancestors.delete(val)
  }
}

export function serializeDraftToYaml(draftObj: Record<string, unknown>, enabledVal: boolean): string {
  const clone = cloneValue(draftObj)
  clone.enable = enabledVal
  return yaml.dump(clone, { indent: 2, lineWidth: -1, noRefs: true, sortKeys: false })
}

export interface UseDnsDraftReturn {
  settings: Ref<DnsSettings | null>
  enabled: Ref<boolean>
  raw: Ref<string>
  draft: Ref<Record<string, unknown>>
  mode: Ref<'visual' | 'raw'>
  loading: Ref<boolean>
  saving: Ref<boolean>
  saveNeedsConfirmation: ComputedRef<boolean>
  saveConflict: Ref<boolean>
  validating: Ref<boolean>
  previewing: Ref<boolean>
  busy: ComputedRef<boolean>
  previewYaml: Ref<string>
  previewDraftRevision: Ref<number>
  isPreviewStale: ComputedRef<boolean>
  error: Ref<string>
  notice: Ref<string>
  fieldErrors: Ref<Record<string, string>>
  invalidMappingInputs: Ref<Record<string, string>>
  hasValidationErrors: ComputedRef<boolean>
  visualDirty: ComputedRef<boolean>
  dirty: ComputedRef<boolean>
  draftRevision: Ref<number>
  generation: Ref<number>

  apply: (settingsValue: DnsSettings) => void
  getList: (key: string) => string[]
  setList: (key: string, items: string[]) => void
  addListItem: (key: string, item: string) => void
  removeListItem: (key: string, index: number) => void
  pasteListItems: (key: string, multilineText: string) => void
  getListText: (key: string) => string
  setListText: (key: string, text: string) => void
  getYamlMappingText: (key: string) => string
  setYamlMappingText: (key: string, rawVal: string) => boolean
  getString: (key: string, fallback?: string) => string
  setString: (key: string, val: string) => void
  getBool: (key: string, fallback?: boolean) => boolean
  setBool: (key: string, val: boolean) => void
  hasKey: (key: string) => boolean
  removeKey: (key: string) => void
  onRawInput: (val: string) => void

  load: (force?: boolean) => Promise<void>
  validate: (input?: string, enabledOverride?: boolean) => Promise<ValidatedDns>
  preview: () => Promise<void>
  switchMode: (next: 'visual' | 'raw') => Promise<void>
  save: () => Promise<void>
  confirmDiscard: () => boolean
}

export function useDnsDraft(): UseDnsDraftReturn {
  const settings = ref<DnsSettings | null>(null)
  const enabled = ref(false)
  const raw = ref('')
  const rawBaseline = ref('')
  const draft = ref<Record<string, unknown>>({})
  const mode = ref<'visual' | 'raw'>('visual')

  const loading = ref(false)
  const saving = ref(false)
  const pendingSave = ref<{ settings: DnsSettings; draftRevision: number } | null>(null)
  const saveNeedsConfirmation = computed(() => pendingSave.value !== null)
  const saveConflict = ref(false)
  const validating = ref(false)
  const previewing = ref(false)
  const busy = computed(() => loading.value || saving.value || validating.value || previewing.value)

  const previewYaml = ref('')
  const previewDraftRevision = ref(-1)
  const error = ref('')
  const notice = ref('')

  const fieldErrors = ref<Record<string, string>>({})
  const invalidMappingInputs = ref<Record<string, string>>({})
  const hasValidationErrors = computed(() => Object.keys(fieldErrors.value).length > 0)

  // Monotonic counters to invalidate stale async responses and track edits
  const draftRevision = ref(0)
  const generation = ref(0)

  let applying = false

  const isPreviewStale = computed(() => {
    return Boolean(previewYaml.value) && previewDraftRevision.value !== draftRevision.value
  })

  // Direct v-model changes to enabled bump draftRevision
  watch(
    enabled,
    () => {
      if (applying) return
      draftRevision.value++
    },
    { flush: 'sync' }
  )

  const visualDirty = computed(() => {
    if (!settings.value) return false
    if (hasValidationErrors.value || Object.keys(invalidMappingInputs.value).length > 0) return true
    if (enabled.value !== settings.value.enabled) return true
    return !deepEqual(draft.value, settings.value.config || {})
  })

  const dirty = computed(() => {
    if (!settings.value) return false
    if (saveNeedsConfirmation.value || saveConflict.value) return true
    if (enabled.value !== settings.value.enabled) return true
    if (mode.value === 'visual') {
      return visualDirty.value
    }
    return raw.value !== rawBaseline.value
  })

  // Hook unsaved changes to browser beforeunload and router leave guards
  const { confirmDiscard } = useUnsavedChanges(
    dirty,
    () => uiText('放弃未保存的 DNS 草稿并重新加载？', 'Discard unsaved DNS draft and reload?')
  )

  function cleanupFieldError(clearedKey?: string) {
    if (clearedKey && fieldErrors.value[clearedKey]) {
      delete fieldErrors.value[clearedKey]
    }
    const remainingKeys = Object.keys(fieldErrors.value)
    if (remainingKeys.length === 0) {
      if (
        error.value.includes('YAML') ||
        error.value.includes('请先修正字段格式错误') ||
        error.value.includes('correct field format errors')
      ) {
        error.value = ''
      }
    } else {
      const nextKey = remainingKeys[0]
      error.value = fieldErrors.value[nextKey]
    }
  }

  function apply(settingsValue: DnsSettings) {
    applying = true
    try {
      settings.value = settingsValue
      enabled.value = settingsValue.enabled
      raw.value = settingsValue.raw_yaml || serializeDraftToYaml(settingsValue.config || {}, settingsValue.enabled)
      rawBaseline.value = raw.value
      draft.value = cloneValue(settingsValue.config || {})
      pendingSave.value = null
      saveConflict.value = false
      fieldErrors.value = {}
      invalidMappingInputs.value = {}
      error.value = ''
      previewYaml.value = ''
      previewDraftRevision.value = -1
      draftRevision.value++
    } finally {
      applying = false
    }
  }

  function getList(key: string): string[] {
    const val = draft.value[key]
    if (Array.isArray(val)) {
      return val.map(String)
    }
    return []
  }

  function setList(key: string, items: string[]) {
    draftRevision.value++
    draft.value = { ...draft.value, [key]: items.map((s) => s.trim()).filter((s) => s.length > 0) }
  }

  function addListItem(key: string, item: string) {
    const trimmed = item.trim()
    if (!trimmed) return
    draftRevision.value++
    const existing = getList(key)
    const newLines = trimmed
      .split('\n')
      .map((s) => s.trim())
      .filter((s) => s.length > 0)
    draft.value = { ...draft.value, [key]: [...existing, ...newLines] }
  }

  function removeListItem(key: string, index: number) {
    const existing = getList(key)
    if (index < 0 || index >= existing.length) return
    draftRevision.value++
    const next = existing.filter((_, i) => i !== index)
    if (next.length === 0) {
      const copy = { ...draft.value }
      delete copy[key]
      draft.value = copy
    } else {
      draft.value = { ...draft.value, [key]: next }
    }
  }

  function pasteListItems(key: string, multilineText: string) {
    const lines = multilineText
      .split('\n')
      .map((s) => s.trim())
      .filter((s) => s.length > 0)
    if (lines.length === 0) return
    draftRevision.value++
    const existing = getList(key)
    draft.value = { ...draft.value, [key]: [...existing, ...lines] }
  }

  function getListText(key: string): string {
    const val = draft.value[key]
    if (Array.isArray(val)) {
      return val.join('\n')
    }
    return ''
  }

  function setListText(key: string, text: string) {
    draftRevision.value++
    const trimmed = text.trim()
    if (trimmed.startsWith('[') && trimmed.endsWith(']')) {
      try {
        const parsed = JSON.parse(trimmed)
        if (Array.isArray(parsed)) {
          draft.value = { ...draft.value, [key]: parsed.map(String) }
          return
        }
      } catch {
        // Fall through to newline split
      }
    }
    const lines = text
      .split('\n')
      .map((s) => s.trim())
      .filter((s) => s.length > 0)
    if (lines.length === 0) {
      if (Object.prototype.hasOwnProperty.call(draft.value, key)) {
        const copy = { ...draft.value }
        delete copy[key]
        draft.value = copy
      }
    } else {
      draft.value = { ...draft.value, [key]: lines }
    }
  }

  function getYamlMappingText(key: string): string {
    if (invalidMappingInputs.value[key] !== undefined) {
      return invalidMappingInputs.value[key]
    }
    const val = draft.value[key]
    if (val === undefined || val === null) {
      return ''
    }
    try {
      return yaml.dump(val, { indent: 2, lineWidth: -1, noRefs: true, sortKeys: false }).trimEnd()
    } catch {
      return ''
    }
  }

  function setYamlMappingText(key: string, rawVal: string): boolean {
    draftRevision.value++
    const trimmed = rawVal.trim()
    if (trimmed === '') {
      if (Object.prototype.hasOwnProperty.call(draft.value, key)) {
        const next = { ...draft.value }
        delete next[key]
        draft.value = next
      }
      delete invalidMappingInputs.value[key]
      delete fieldErrors.value[key]
      cleanupFieldError(key)
      return true
    }

    try {
      // Keep date-like scalars as text, as they arrive through the JSON API.
      const parsed = yaml.load(rawVal, { schema: yaml.JSON_SCHEMA })
      if (!isPlainObject(parsed)) {
        invalidMappingInputs.value[key] = rawVal
        const errMsg = uiText(
          `${key} 必须是有效 YAML 键值映射（非标量或列表）`,
          `${key} must be a valid YAML mapping (not scalar or list)`
        )
        fieldErrors.value[key] = errMsg
        error.value = errMsg
        return false
      }

      draft.value = { ...draft.value, [key]: cloneValue(parsed) }
      delete invalidMappingInputs.value[key]
      delete fieldErrors.value[key]
      cleanupFieldError(key)
      return true
    } catch (err: any) {
      invalidMappingInputs.value[key] = rawVal
      const reason = displayText(err?.message || '')
      const errMsg = uiText(
        `${key} YAML 格式无效${reason ? `: ${reason}` : ''}`,
        `${key} invalid YAML${reason ? `: ${reason}` : ''}`
      )
      fieldErrors.value[key] = errMsg
      error.value = errMsg
      return false
    }
  }

  function getString(key: string, fallback = ''): string {
    const val = draft.value[key]
    if (val === undefined || val === null) return fallback
    return String(val)
  }

  function setString(key: string, val: string) {
    draftRevision.value++
    const trimmed = typeof val === 'string' ? val.trim() : ''
    if (!trimmed) {
      if (Object.prototype.hasOwnProperty.call(draft.value, key)) {
        const next = { ...draft.value }
        delete next[key]
        draft.value = next
      }
    } else {
      draft.value = { ...draft.value, [key]: val }
    }
  }

  function getBool(key: string, fallback = false): boolean {
    const val = draft.value[key]
    if (typeof val === 'boolean') return val
    if (val === undefined || val === null) return fallback
    return Boolean(val)
  }

  function setBool(key: string, val: boolean) {
    draftRevision.value++
    draft.value = { ...draft.value, [key]: val }
  }

  function hasKey(key: string): boolean {
    return Object.prototype.hasOwnProperty.call(draft.value, key) && draft.value[key] !== undefined
  }

  function removeKey(key: string): void {
    let changed = false
    if (Object.prototype.hasOwnProperty.call(draft.value, key)) {
      const next = { ...draft.value }
      delete next[key]
      draft.value = next
      changed = true
    }
    if (invalidMappingInputs.value[key] !== undefined || fieldErrors.value[key] !== undefined) {
      delete invalidMappingInputs.value[key]
      delete fieldErrors.value[key]
      cleanupFieldError(key)
      changed = true
    }
    if (changed) {
      draftRevision.value++
    }
  }

  function onRawInput(val: string) {
    draftRevision.value++
    raw.value = val
  }

  async function load(force = false) {
    if (busy.value) return
    if (dirty.value && !force && !confirmDiscard()) return
    loading.value = true
    error.value = ''
    notice.value = ''
    const currentGen = ++generation.value
    const currentDraftRev = draftRevision.value
    try {
      const fetched = await api.get<DnsSettings>('/api/v1/settings/dns')
      if (currentGen !== generation.value || currentDraftRev !== draftRevision.value) return
      apply(fetched)
    } catch (e: any) {
      if (currentGen !== generation.value || currentDraftRev !== draftRevision.value) return
      error.value = displayText(e instanceof Error ? e.message : uiText('读取 DNS 设置失败', 'Failed to read DNS settings'))
    } finally {
      loading.value = false
    }
  }

  async function validate(input = raw.value, enabledOverride?: boolean): Promise<ValidatedDns> {
    validating.value = true
    try {
      return await api.post<ValidatedDns>('/api/v1/settings/dns/validate', {
        enabled: enabledOverride !== undefined ? enabledOverride : enabled.value,
        raw_yaml: input,
      })
    } finally {
      validating.value = false
    }
  }

  async function preview(): Promise<void> {
    if (!settings.value || busy.value) return
    if (mode.value === 'visual' && hasValidationErrors.value) {
      error.value = uiText('请先修正字段格式错误后再生成预览', 'Please correct field format errors before previewing')
      return
    }

    previewing.value = true
    error.value = ''
    const currentGen = ++generation.value
    const currentDraftRev = draftRevision.value
    const capturedEnabled = enabled.value
    const rawInput = mode.value === 'raw' ? raw.value : serializeDraftToYaml(draft.value, capturedEnabled)

    try {
      const validated = await api.post<ValidatedDns>('/api/v1/settings/dns/validate', {
        enabled: capturedEnabled,
        raw_yaml: rawInput,
      })
      // Guard against race conditions: draft or enabled changed while request in flight
      if (currentGen !== generation.value || currentDraftRev !== draftRevision.value) {
        return
      }
      previewYaml.value = validated.raw_yaml
      previewDraftRevision.value = currentDraftRev
    } catch (e: any) {
      if (currentGen !== generation.value || currentDraftRev !== draftRevision.value) {
        return
      }
      previewYaml.value = ''
      previewDraftRevision.value = -1
      error.value = displayText(e instanceof Error ? e.message : uiText('生成 DNS 预览失败', 'Failed to generate DNS preview'))
    } finally {
      previewing.value = false
    }
  }

  async function switchMode(next: 'visual' | 'raw') {
    if (next === mode.value) return
    if (!settings.value || busy.value) return
    if (mode.value === 'visual' && hasValidationErrors.value) {
      error.value = uiText('请先修正字段格式错误后再切换模式', 'Please correct field format errors before switching modes')
      return
    }

    error.value = ''
    const currentGen = ++generation.value
    const currentDraftRev = draftRevision.value
    const capturedEnabled = enabled.value
    const wasDirty = dirty.value

    try {
      if (next === 'visual') {
        const parsed = await validate(raw.value, capturedEnabled)
        if (currentGen !== generation.value || currentDraftRev !== draftRevision.value) {
          return
        }
        draft.value = cloneValue(parsed.config || {})
        raw.value = parsed.raw_yaml
        fieldErrors.value = {}
        invalidMappingInputs.value = {}
        draftRevision.value++
      } else {
        const yamlInput = serializeDraftToYaml(draft.value, capturedEnabled)
        const parsed = await validate(yamlInput, capturedEnabled)
        if (currentGen !== generation.value || currentDraftRev !== draftRevision.value) {
          return
        }
        raw.value = parsed.raw_yaml
        draft.value = cloneValue(parsed.config || {})
        fieldErrors.value = {}
        invalidMappingInputs.value = {}
        draftRevision.value++
      }
      if (!wasDirty) rawBaseline.value = raw.value
      mode.value = next
    } catch (e: any) {
      if (currentGen !== generation.value || currentDraftRev !== draftRevision.value) return
      error.value = displayText(e instanceof Error ? e.message : uiText('DNS 配置校验失败', 'DNS configuration validation failed'))
    }
  }

  async function save() {
    if (!settings.value || busy.value || saveConflict.value) return
    if (!pendingSave.value && mode.value === 'visual' && hasValidationErrors.value) {
      error.value = uiText('请先修正字段格式错误后再保存', 'Please correct field format errors before saving')
      return
    }

    saving.value = true
    error.value = ''
    notice.value = ''
    const currentGen = ++generation.value
    const currentDraftRev = draftRevision.value
    const capturedEnabled = enabled.value
    const capturedRevision = settings.value.revision

    try {
      if (!pendingSave.value) {
        const rawInput = mode.value === 'raw' ? raw.value : serializeDraftToYaml(draft.value, capturedEnabled)
        const validated = await validate(rawInput, capturedEnabled)
        if (currentGen !== generation.value || currentDraftRev !== draftRevision.value) return

        const written = await api.put<DnsSettings>('/api/v1/settings/dns', {
          enabled: capturedEnabled,
          raw_yaml: validated.raw_yaml,
          expected_revision: capturedRevision,
        })
        if (currentGen !== generation.value) return

        // A successful PUT owns a real new revision, even if the confirmation GET fails.
        settings.value = written
        pendingSave.value = { settings: written, draftRevision: currentDraftRev }
      }

      const pending = pendingSave.value!
      const confirmed = await api.get<DnsSettings>('/api/v1/settings/dns')
      if (currentGen !== generation.value) return

      if (
        confirmed.revision !== pending.settings.revision ||
        confirmed.enabled !== pending.settings.enabled ||
        confirmed.raw_yaml !== pending.settings.raw_yaml ||
        !deepEqual(confirmed.config, pending.settings.config)
      ) {
        pendingSave.value = null
        saveConflict.value = true
        error.value = uiText(
          `DNS 已写入 revision ${pending.settings.revision}，但服务器回读为 revision ${confirmed.revision}，内容或版本已变化。草稿已保留，请复制草稿后重载核对。`,
          `DNS was written at revision ${pending.settings.revision}, but the server returned revision ${confirmed.revision} with changed content or version. Draft preserved; copy it before reloading to compare.`
        )
        return
      }

      if (draftRevision.value === pending.draftRevision) {
        apply(confirmed)
        notice.value = uiText('DNS 设置已保存并从服务器重新读取。', 'DNS settings saved and reloaded from server.')
      } else {
        // Confirmation must not replace edits made after the accepted write.
        settings.value = confirmed
        rawBaseline.value = confirmed.raw_yaml
        pendingSave.value = null
        notice.value = dirty.value
          ? uiText('上次保存已从服务器确认；之后的草稿修改仍未保存。', 'The previous save is confirmed; newer draft edits are still unsaved.')
          : uiText('DNS 设置已保存并从服务器重新读取。', 'DNS settings saved and reloaded from server.')
      }
    } catch (e: any) {
      if (currentGen !== generation.value) return
      if (pendingSave.value) {
        const reason = displayText(e instanceof Error ? e.message : uiText('读取失败', 'Read failed'))
        error.value = uiText(
          `服务器已接受 DNS 保存（revision ${pendingSave.value.settings.revision}），但回读失败。草稿已保留；点击“确认保存结果”只会重新读取，不会重复提交。${reason}`,
          `The server accepted the DNS save (revision ${pendingSave.value.settings.revision}), but read-back failed. Draft preserved; Confirm save only retries the read, not the write. ${reason}`
        )
      } else if (e?.status === 409) {
        saveConflict.value = true
        error.value = uiText(
          'DNS 设置已被其他操作更新（revision 冲突）；草稿已保留，原 revision 未变，不会自动覆盖新版本。请复制草稿后重新加载核对。',
          'DNS settings updated by another operation (revision conflict); draft and original revision preserved. The newer version will not be overwritten automatically. Copy your draft before reloading to compare.'
        )
      } else {
        error.value = displayText(e instanceof Error ? e.message : uiText('保存 DNS 设置失败；草稿已保留', 'Failed to save DNS settings; draft preserved'))
      }
    } finally {
      saving.value = false
    }
  }

  return {
    settings,
    enabled,
    raw,
    draft,
    mode,
    loading,
    saving,
    saveNeedsConfirmation,
    saveConflict,
    validating,
    previewing,
    busy,
    previewYaml,
    previewDraftRevision,
    isPreviewStale,
    error,
    notice,
    fieldErrors,
    invalidMappingInputs,
    hasValidationErrors,
    visualDirty,
    dirty,
    draftRevision,
    generation,

    apply,
    getList,
    setList,
    addListItem,
    removeListItem,
    pasteListItems,
    getListText,
    setListText,
    getYamlMappingText,
    setYamlMappingText,
    getString,
    setString,
    getBool,
    setBool,
    hasKey,
    removeKey,
    onRawInput,

    load,
    validate,
    preview,
    switchMode,
    save,
    confirmDiscard,
  }
}
