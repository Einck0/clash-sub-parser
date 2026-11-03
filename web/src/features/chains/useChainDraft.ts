import { ref, computed } from 'vue'
import { api, ApiError } from '../../api/client'
import { uiText } from '../../locales/redesign'
import type {
  ProxyChainBinding,
  ProxyChainTargetType,
  ProxyChainDialerType,
  ChainEffect,
  EffectNode,
  EffectsResult,
  OptionsResult,
} from './types'

export interface UseChainDraftOptions {
  getConfigVersion: () => string
  getOptions: () => OptionsResult
  onSaved?: (binding: ProxyChainBinding, newConfigVersion: string) => void
}

export function useChainDraft(options: UseChainDraftOptions) {
  const isOpen = ref(false)
  const isEditing = ref(false)
  const saving = ref(false)
  const saveError = ref<string | null>(null)

  // 409 Conflict State
  const conflictState = ref<{
    message: string
    newVersion?: string
  } | null>(null)

  // Form draft
  const draft = ref<ProxyChainBinding>({
    id: '',
    target_type: 'subscription',
    target_id: '',
    dialer_type: 'node_group',
    dialer_id: '',
    enabled: true,
    sort_order: 10,
    note: '',
    revision: 0,
  })

  // Baseline copy to detect dirty changes
  const baseline = ref<ProxyChainBinding>({ ...draft.value })

  const isDirty = computed(() => {
    if (!isOpen.value) return false
    return (
      draft.value.target_type !== baseline.value.target_type ||
      draft.value.target_id !== baseline.value.target_id ||
      draft.value.dialer_type !== baseline.value.dialer_type ||
      draft.value.dialer_id !== baseline.value.dialer_id ||
      draft.value.enabled !== baseline.value.enabled ||
      draft.value.sort_order !== baseline.value.sort_order ||
      draft.value.note !== baseline.value.note
    )
  })

  // Live preview state
  const previewLoading = ref(false)
  const previewError = ref<string | null>(null)
  const previewEffects = ref<ChainEffect[]>([])
  const previewNodes = ref<EffectNode[]>([])
  const previewVersion = ref<string>('')
  const isPreviewStale = ref(false)

  let previewGeneration = 0
  let debounceTimer: ReturnType<typeof setTimeout> | null = null

  function clientValidate(): string | null {
    if (!draft.value.target_id.trim()) {
      return uiText('请选择出口目标', 'Please select a target')
    }
    if (!draft.value.dialer_id.trim()) {
      return uiText('请选择前置跳板', 'Please select an upstream dialer')
    }
    if (
      draft.value.target_type === 'node' &&
      draft.value.dialer_type === 'node' &&
      draft.value.target_id === draft.value.dialer_id
    ) {
      return uiText('单节点不能将自身作为跳板代理', 'A node cannot chain to itself')
    }
    if (
      draft.value.target_type === 'node_group' &&
      draft.value.dialer_type === 'node_group' &&
      draft.value.target_id === draft.value.dialer_id
    ) {
      return uiText('策略组不能将自身作为跳板代理', 'A group cannot chain to itself')
    }
    return null
  }

  async function executePreview(currentDraft: ProxyChainBinding, configVer: string) {
    const generation = ++previewGeneration
    previewLoading.value = true
    previewError.value = null

    // Client-side quick check
    const validationError = clientValidate()
    if (validationError) {
      previewLoading.value = false
      previewError.value = validationError
      previewEffects.value = []
      previewNodes.value = []
      return
    }

    try {
      const res = await api.post<EffectsResult>('/api/v1/proxy-chains/preview', {
        binding: currentDraft,
        config_version: configVer,
      })

      if (generation === previewGeneration) {
        previewEffects.value = res.effects || []
        previewNodes.value = res.nodes || []
        previewVersion.value = res.version || configVer
        isPreviewStale.value = false
      }
    } catch (err: any) {
      if (generation === previewGeneration) {
        previewEffects.value = []
        previewNodes.value = []
        if (err?.status === 409 || err?.code === 'conflict') {
          isPreviewStale.value = true
          previewError.value = uiText(
            '配置版本已变动 (409 Conflict)，预览已过期，请刷新或重试',
            'Config version changed (409 Conflict), preview is stale, please refresh or retry'
          )
        } else if (err instanceof ApiError) {
          previewError.value = err.message
        } else {
          previewError.value = err?.message || 'Preview failed'
        }
      }
    } finally {
      if (generation === previewGeneration) {
        previewLoading.value = false
      }
    }
  }

  function triggerPreview(immediate = false) {
    if (debounceTimer) {
      clearTimeout(debounceTimer)
      debounceTimer = null
    }

    const currentDraft = { ...draft.value }
    const configVer = options.getConfigVersion()

    if (immediate) {
      void executePreview(currentDraft, configVer)
      return
    }

    debounceTimer = setTimeout(() => {
      void executePreview(currentDraft, configVer)
    }, 250)
  }

  function openCreate() {
    const opts = options.getOptions()
    const defaultTargetId =
      opts.subscriptions?.[0]?.id || opts.groups?.[0]?.id || opts.nodes?.[0]?.id || ''
    const defaultTargetType: ProxyChainTargetType = opts.subscriptions?.length
      ? 'subscription'
      : opts.groups?.length
      ? 'node_group'
      : 'node'
    const defaultDialerId = opts.groups?.[0]?.id || opts.nodes?.[0]?.id || ''
    const defaultDialerType: ProxyChainDialerType = opts.groups?.length ? 'node_group' : 'node'

    draft.value = {
      id: '',
      target_type: defaultTargetType,
      target_id: defaultTargetId,
      dialer_type: defaultDialerType,
      dialer_id: defaultDialerId,
      enabled: true,
      sort_order: 10,
      note: '',
      revision: 0,
    }
    baseline.value = { ...draft.value }
    isEditing.value = false
    isOpen.value = true
    saveError.value = null
    conflictState.value = null
    previewError.value = null
    isPreviewStale.value = false

    triggerPreview(true)
  }

  function openEdit(binding: ProxyChainBinding) {
    draft.value = { ...binding }
    baseline.value = { ...binding }
    isEditing.value = true
    isOpen.value = true
    saveError.value = null
    conflictState.value = null
    previewError.value = null
    isPreviewStale.value = false

    triggerPreview(true)
  }

  function close() {
    if (debounceTimer) {
      clearTimeout(debounceTimer)
      debounceTimer = null
    }
    isOpen.value = false
    draft.value = { ...baseline.value }
    saveError.value = null
    conflictState.value = null
  }

  function setTargetType(type: ProxyChainTargetType) {
    if (draft.value.target_type === type) return
    draft.value.target_type = type
    const opts = options.getOptions()
    if (type === 'subscription') {
      draft.value.target_id = opts.subscriptions?.[0]?.id || ''
    } else if (type === 'node_group') {
      draft.value.target_id = opts.groups?.[0]?.id || ''
    } else if (type === 'node') {
      draft.value.target_id = opts.nodes?.[0]?.id || ''
    }
    triggerPreview()
  }

  function setDialerType(type: ProxyChainDialerType) {
    if (draft.value.dialer_type === type) return
    draft.value.dialer_type = type
    const opts = options.getOptions()
    if (type === 'node_group') {
      draft.value.dialer_id = opts.groups?.[0]?.id || ''
    } else if (type === 'node') {
      draft.value.dialer_id = opts.nodes?.[0]?.id || ''
    }
    triggerPreview()
  }

  function setTargetId(id: string) {
    draft.value.target_id = id
    triggerPreview()
  }

  function setDialerId(id: string) {
    draft.value.dialer_id = id
    triggerPreview()
  }

  function setEnabled(enabled: boolean) {
    draft.value.enabled = enabled
    triggerPreview()
  }

  function setSortOrder(order: number) {
    draft.value.sort_order = order
    triggerPreview()
  }

  function setNote(note: string) {
    draft.value.note = note
  }

  /**
   * Rebase on 409 conflict: preserves user draft edits while adopting latest config version and row revision.
   */
  function rebaseAndKeepDraft(latestConfigVersion: string, latestRowRevision?: number) {
    if (latestRowRevision !== undefined && isEditing.value) {
      draft.value.revision = latestRowRevision
      baseline.value.revision = latestRowRevision
    }
    conflictState.value = null
    saveError.value = null
    isPreviewStale.value = false
    // Immediately re-trigger preview with the new version
    void executePreview(draft.value, latestConfigVersion)
  }

  async function save(): Promise<boolean> {
    saveError.value = null
    conflictState.value = null

    const validationMsg = clientValidate()
    if (validationMsg) {
      saveError.value = validationMsg
      return false
    }

    saving.value = true
    const currentConfigVersion = options.getConfigVersion()

    try {
      if (isEditing.value) {
        // PUT /api/v1/proxy-chains/{id}
        const res = await api.put<{ binding: ProxyChainBinding; version: string }>(
          `/api/v1/proxy-chains/${encodeURIComponent(draft.value.id)}`,
          {
            binding: {
              id: draft.value.id,
              target_type: draft.value.target_type,
              target_id: draft.value.target_id,
              dialer_type: draft.value.dialer_type,
              dialer_id: draft.value.dialer_id,
              enabled: draft.value.enabled,
              sort_order: Number(draft.value.sort_order),
              note: draft.value.note,
              revision: draft.value.revision,
            },
            config_version: currentConfigVersion,
            expected_revision: draft.value.revision,
          }
        )

        baseline.value = { ...res.binding }
        draft.value = { ...res.binding }
        options.onSaved?.(res.binding, res.version)
        close()
        return true
      } else {
        // POST /api/v1/proxy-chains
        const res = await api.post<{ binding: ProxyChainBinding; version: string }>(
          '/api/v1/proxy-chains',
          {
            binding: {
              id: '',
              target_type: draft.value.target_type,
              target_id: draft.value.target_id,
              dialer_type: draft.value.dialer_type,
              dialer_id: draft.value.dialer_id,
              enabled: draft.value.enabled,
              sort_order: Number(draft.value.sort_order),
              note: draft.value.note,
              revision: 0,
            },
            config_version: currentConfigVersion,
          }
        )

        baseline.value = { ...res.binding }
        draft.value = { ...res.binding }
        options.onSaved?.(res.binding, res.version)
        close()
        return true
      }
    } catch (err: any) {
      if (err?.status === 409 || err?.code === 'conflict') {
        // Keep draft exactly as is; show conflict banner
        conflictState.value = {
          message:
            err?.message ||
            uiText(
              '检测到并发修改冲突 (409 Conflict)，配置已被其他人修改。',
              'Concurrent modification conflict (409 Conflict). Configuration was updated by another process.'
            ),
        }
      } else if (err instanceof ApiError) {
        saveError.value = err.message
      } else {
        saveError.value = err?.message || 'Save failed'
      }
      return false
    } finally {
      saving.value = false
    }
  }

  return {
    isOpen,
    isEditing,
    saving,
    saveError,
    conflictState,
    draft,
    baseline,
    isDirty,
    previewLoading,
    previewError,
    previewEffects,
    previewNodes,
    previewVersion,
    isPreviewStale,
    clientValidate,
    triggerPreview,
    openCreate,
    openEdit,
    close,
    setTargetType,
    setDialerType,
    setTargetId,
    setDialerId,
    setEnabled,
    setSortOrder,
    setNote,
    rebaseAndKeepDraft,
    save,
  }
}

export type UseChainDraftReturn = ReturnType<typeof useChainDraft>
