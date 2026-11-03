import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import yaml from 'js-yaml'
import { ApiError, api } from '../../api/client'
import { uiText } from '../../locales/redesign'
import { useUnsavedChanges } from '../../composables/useUnsavedChanges'
import type { DnsSettings, ValidatedDns } from './dnsTypes'

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

function cloneValue<T>(val: T): T {
  if (val === null || typeof val !== 'object') {
    return val
  }
  if (Array.isArray(val)) {
    return val.map(cloneValue) as unknown as T
  }
  const result: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(val as Record<string, unknown>)) {
    result[k] = cloneValue(v)
  }
  return result as T
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
  const draft = ref<Record<string, unknown>>({})
  const mode = ref<'visual' | 'raw'>('visual')

  const loading = ref(false)
  const saving = ref(false)
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
    if (enabled.value !== settings.value.enabled) return true
    if (mode.value === 'visual') {
      return visualDirty.value
    }
    return raw.value !== settings.value.raw_yaml
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
      draft.value = cloneValue(settingsValue.config || {})
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
      const parsed = yaml.load(rawVal)
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

      draft.value = { ...draft.value, [key]: parsed }
      delete invalidMappingInputs.value[key]
      delete fieldErrors.value[key]
      cleanupFieldError(key)
      return true
    } catch (err: any) {
      invalidMappingInputs.value[key] = rawVal
      const reason = err?.message || ''
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
    if (dirty.value && !force && !confirmDiscard()) return
    loading.value = true
    error.value = ''
    notice.value = ''
    const currentGen = ++generation.value
    try {
      const fetched = await api.get<DnsSettings>('/api/v1/settings/dns')
      if (currentGen !== generation.value) return
      apply(fetched)
    } catch (e: any) {
      error.value = e instanceof Error ? e.message : uiText('读取 DNS 设置失败', 'Failed to read DNS settings')
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
    if (busy.value) return
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
      error.value = e instanceof Error ? e.message : uiText('生成 DNS 预览失败', 'Failed to generate DNS preview')
    } finally {
      previewing.value = false
    }
  }

  async function switchMode(next: 'visual' | 'raw') {
    if (next === mode.value) return
    if (busy.value) return
    if (mode.value === 'visual' && hasValidationErrors.value) {
      error.value = uiText('请先修正字段格式错误后再切换模式', 'Please correct field format errors before switching modes')
      return
    }

    error.value = ''
    const currentGen = ++generation.value
    const currentDraftRev = draftRevision.value
    const capturedEnabled = enabled.value

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
      mode.value = next
    } catch (e: any) {
      error.value = e instanceof Error ? e.message : uiText('DNS 配置校验失败', 'DNS configuration validation failed')
    }
  }

  async function save() {
    if (!settings.value || busy.value) return
    if (mode.value === 'visual' && hasValidationErrors.value) {
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
      const rawInput = mode.value === 'raw' ? raw.value : serializeDraftToYaml(draft.value, capturedEnabled)
      const validated = await validate(rawInput, capturedEnabled)
      if (currentGen !== generation.value || currentDraftRev !== draftRevision.value) {
        return
      }

      await api.put<DnsSettings>('/api/v1/settings/dns', {
        enabled: capturedEnabled,
        raw_yaml: validated.raw_yaml,
        expected_revision: capturedRevision,
      })
      if (currentGen !== generation.value || currentDraftRev !== draftRevision.value) {
        return
      }

      const confirmed = await api.get<DnsSettings>('/api/v1/settings/dns')
      if (currentGen !== generation.value || currentDraftRev !== draftRevision.value) {
        return
      }

      apply(confirmed)
      notice.value = uiText('DNS 设置已保存并从服务器重新读取。', 'DNS settings saved and reloaded from server.')
    } catch (e: any) {
      if (
        e?.status === 409 ||
        (e instanceof ApiError && e.status === 409) ||
        (e?.name === 'ApiError' && e?.status === 409)
      ) {
        error.value = uiText(
          'DNS 设置已被其他操作更新（revision 冲突）；草稿已保留，请重新加载或复制草稿后再处理。',
          'DNS settings updated by another operation (revision conflict); draft preserved, please reload or copy your draft.'
        )
      } else if (
        e?.status === 422 ||
        (e instanceof ApiError && e.status === 422) ||
        (e?.name === 'ApiError' && e?.status === 422)
      ) {
        error.value =
          e?.message ||
          uiText('DNS 配置校验失败（422 参数错误），草稿已保留。', 'DNS configuration validation failed (422 error); draft preserved.')
      } else {
        error.value = e instanceof Error ? e.message : uiText('保存 DNS 设置失败', 'Failed to save DNS settings')
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
