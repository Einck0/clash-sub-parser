import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import { ApiError, api } from '../../api/client'
import { uiText } from '../../locales/redesign'
import { useUnsavedChanges, type UseUnsavedChangesReturn } from '../../composables/useUnsavedChanges'
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
  error: Ref<string>
  notice: Ref<string>
  fieldErrors: Ref<Record<string, string>>
  invalidJsonInputs: Ref<Record<string, string>>
  hasValidationErrors: ComputedRef<boolean>
  visualDirty: ComputedRef<boolean>
  dirty: ComputedRef<boolean>
  draftRevision: Ref<number>
  generation: Ref<number>

  apply: (settingsValue: DnsSettings) => void
  getListText: (key: string) => string
  setListText: (key: string, text: string) => void
  getJsonText: (key: string) => string
  setJsonText: (key: string, rawVal: string) => boolean
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
  const error = ref('')
  const notice = ref('')

  const fieldErrors = ref<Record<string, string>>({})
  const invalidJsonInputs = ref<Record<string, string>>({})
  const hasValidationErrors = computed(() => Object.keys(fieldErrors.value).length > 0)

  // Monotonic counters to invalidate stale async responses and race conditions
  const draftRevision = ref(0)
  const generation = ref(0)

  let applying = false

  function invalidatePreview() {
    previewYaml.value = ''
  }

  // enabled direct v-model mutations must bump draftRevision and invalidate preview
  watch(
    enabled,
    () => {
      if (applying) return
      draftRevision.value++
      invalidatePreview()
    },
    { flush: 'sync' }
  )

  const visualDirty = computed(() => {
    if (!settings.value) return false
    if (hasValidationErrors.value) return true
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
    const remainingKeys = Object.keys(fieldErrors.value)
    if (remainingKeys.length === 0) {
      if (
        error.value.includes('必须是有效 JSON') ||
        error.value.includes('必须是 JSON') ||
        error.value.includes('valid JSON') ||
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
      raw.value = settingsValue.raw_yaml || JSON.stringify(settingsValue.config || {}, null, 2)
      draft.value = structuredClone(settingsValue.config || {})
      fieldErrors.value = {}
      invalidJsonInputs.value = {}
      error.value = ''
      invalidatePreview()
      draftRevision.value++
    } finally {
      applying = false
    }
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
    invalidatePreview()
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
    draft.value = { ...draft.value, [key]: lines }
  }

  function getJsonText(key: string): string {
    if (invalidJsonInputs.value[key] !== undefined) {
      return invalidJsonInputs.value[key]
    }
    const val = draft.value[key]
    if (val === undefined || val === null) {
      return ''
    }
    return JSON.stringify(val, null, 2)
  }

  function setJsonText(key: string, rawVal: string): boolean {
    const trimmed = rawVal.trim()
    if (trimmed === '') {
      if (Object.prototype.hasOwnProperty.call(draft.value, key)) {
        const next = { ...draft.value }
        delete next[key]
        draft.value = next
      }
      delete invalidJsonInputs.value[key]
      delete fieldErrors.value[key]
      cleanupFieldError(key)
      draftRevision.value++
      invalidatePreview()
      return true
    }

    try {
      const parsed = JSON.parse(rawVal)
      if (!isPlainObject(parsed)) {
        invalidJsonInputs.value[key] = rawVal
        const errMsg = uiText(
          `${key} 必须是有效 JSON 对象（键值映射）`,
          `${key} must be a valid JSON object mapping`
        )
        fieldErrors.value[key] = errMsg
        error.value = errMsg
        draftRevision.value++
        invalidatePreview()
        return false
      }

      draft.value = { ...draft.value, [key]: parsed }
      delete invalidJsonInputs.value[key]
      delete fieldErrors.value[key]
      cleanupFieldError(key)
      draftRevision.value++
      invalidatePreview()
      return true
    } catch {
      invalidJsonInputs.value[key] = rawVal
      const errMsg = uiText(`${key} 必须是有效 JSON`, `${key} must be valid JSON`)
      fieldErrors.value[key] = errMsg
      error.value = errMsg
      draftRevision.value++
      invalidatePreview()
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
    invalidatePreview()
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
    invalidatePreview()
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
    if (invalidJsonInputs.value[key] !== undefined || fieldErrors.value[key] !== undefined) {
      delete invalidJsonInputs.value[key]
      delete fieldErrors.value[key]
      cleanupFieldError(key)
      changed = true
    }
    if (changed) {
      draftRevision.value++
      invalidatePreview()
    }
  }

  function onRawInput(val: string) {
    draftRevision.value++
    invalidatePreview()
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
    const rawInput = mode.value === 'raw' ? raw.value : JSON.stringify(draft.value, null, 2)

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
    } catch (e: any) {
      if (currentGen !== generation.value || currentDraftRev !== draftRevision.value) {
        return
      }
      previewYaml.value = ''
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
        // Guard against race conditions: user edited or another operation was triggered while validating
        if (currentGen !== generation.value || currentDraftRev !== draftRevision.value) {
          return
        }
        draft.value = structuredClone(parsed.config || {})
        raw.value = parsed.raw_yaml
        fieldErrors.value = {}
        invalidJsonInputs.value = {}
        draftRevision.value++
        invalidatePreview()
      } else {
        const parsed = await validate(JSON.stringify(draft.value, null, 2), capturedEnabled)
        // Guard against race conditions: user edited or another operation was triggered while validating
        if (currentGen !== generation.value || currentDraftRev !== draftRevision.value) {
          return
        }
        raw.value = parsed.raw_yaml
        draft.value = structuredClone(parsed.config || {})
        fieldErrors.value = {}
        invalidJsonInputs.value = {}
        draftRevision.value++
        invalidatePreview()
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
      const rawInput = mode.value === 'raw' ? raw.value : JSON.stringify(draft.value, null, 2)
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
    error,
    notice,
    fieldErrors,
    invalidJsonInputs,
    hasValidationErrors,
    visualDirty,
    dirty,
    draftRevision,
    generation,

    apply,
    getListText,
    setListText,
    getJsonText,
    setJsonText,
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
