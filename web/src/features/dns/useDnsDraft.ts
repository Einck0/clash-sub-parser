import { computed, ref, type ComputedRef, type Ref } from 'vue'
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

export interface UseDnsDraftReturn {
  settings: Ref<DnsSettings | null>
  enabled: Ref<boolean>
  raw: Ref<string>
  draft: Ref<Record<string, unknown>>
  mode: Ref<'visual' | 'raw'>
  loading: Ref<boolean>
  saving: Ref<boolean>
  validating: Ref<boolean>
  busy: ComputedRef<boolean>
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
  onRawInput: (val: string) => void

  load: (force?: boolean) => Promise<void>
  validate: (input?: string) => Promise<ValidatedDns>
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
  const busy = computed(() => loading.value || saving.value || validating.value)

  const error = ref('')
  const notice = ref('')

  const fieldErrors = ref<Record<string, string>>({})
  const invalidJsonInputs = ref<Record<string, string>>({})
  const hasValidationErrors = computed(() => Object.keys(fieldErrors.value).length > 0)

  // Monotonic counters to invalidate stale async responses and race conditions
  const draftRevision = ref(0)
  const generation = ref(0)

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

  function apply(settingsValue: DnsSettings) {
    settings.value = settingsValue
    enabled.value = settingsValue.enabled
    raw.value = settingsValue.raw_yaml
    draft.value = structuredClone(settingsValue.config || {})
    fieldErrors.value = {}
    invalidJsonInputs.value = {}
    error.value = ''
    draftRevision.value++
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
    draft.value = { ...draft.value, [key]: lines }
  }

  function getJsonText(key: string): string {
    if (invalidJsonInputs.value[key] !== undefined) {
      return invalidJsonInputs.value[key]
    }
    const val = draft.value[key]
    return JSON.stringify(val ?? {}, null, 2)
  }

  function setJsonText(key: string, rawVal: string): boolean {
    draftRevision.value++
    try {
      const parsed = JSON.parse(rawVal)
      draft.value = { ...draft.value, [key]: parsed }
      delete invalidJsonInputs.value[key]
      delete fieldErrors.value[key]
      if (Object.keys(fieldErrors.value).length === 0 && error.value.includes('必须是有效 JSON')) {
        error.value = ''
      }
      return true
    } catch {
      invalidJsonInputs.value[key] = rawVal
      fieldErrors.value[key] = `${key} 必须是有效 JSON`
      error.value = `${key} 必须是有效 JSON`
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
    draft.value = { ...draft.value, [key]: val }
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

  async function validate(input = raw.value): Promise<ValidatedDns> {
    validating.value = true
    try {
      return await api.post<ValidatedDns>('/api/v1/settings/dns/validate', {
        enabled: enabled.value,
        raw_yaml: input,
      })
    } finally {
      validating.value = false
    }
  }

  async function switchMode(next: 'visual' | 'raw') {
    if (next === mode.value) return
    if (saving.value || validating.value || loading.value) return
    if (mode.value === 'visual' && hasValidationErrors.value) {
      error.value = uiText('请先修正字段格式错误后再切换模式', 'Please correct field format errors before switching modes')
      return
    }

    error.value = ''
    const currentGen = ++generation.value
    const currentDraftRev = draftRevision.value

    try {
      if (next === 'visual') {
        const parsed = await validate(raw.value)
        // Guard against race conditions: user edited or another operation was triggered while validating
        if (currentGen !== generation.value || currentDraftRev !== draftRevision.value) {
          return
        }
        draft.value = structuredClone(parsed.config || {})
        raw.value = parsed.raw_yaml
        fieldErrors.value = {}
        invalidJsonInputs.value = {}
      } else {
        const parsed = await validate(JSON.stringify(draft.value, null, 2))
        // Guard against race conditions: user edited or another operation was triggered while validating
        if (currentGen !== generation.value || currentDraftRev !== draftRevision.value) {
          return
        }
        raw.value = parsed.raw_yaml
        draft.value = structuredClone(parsed.config || {})
        fieldErrors.value = {}
        invalidJsonInputs.value = {}
      }
      mode.value = next
    } catch (e: any) {
      error.value = e instanceof Error ? e.message : uiText('DNS 配置校验失败', 'DNS configuration validation failed')
    }
  }

  async function save() {
    if (!settings.value || saving.value || validating.value || loading.value) return
    if (mode.value === 'visual' && hasValidationErrors.value) {
      error.value = uiText('请先修正字段格式错误后再保存', 'Please correct field format errors before saving')
      return
    }

    saving.value = true
    error.value = ''
    notice.value = ''
    try {
      const rawInput = mode.value === 'raw' ? raw.value : JSON.stringify(draft.value, null, 2)
      const validated = await validate(rawInput)
      await api.put<DnsSettings>('/api/v1/settings/dns', {
        enabled: enabled.value,
        raw_yaml: validated.raw_yaml,
        expected_revision: settings.value.revision,
      })
      const confirmed = await api.get<DnsSettings>('/api/v1/settings/dns')
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
    busy,
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
    onRawInput,

    load,
    validate,
    switchMode,
    save,
    confirmDiscard,
  }
}
