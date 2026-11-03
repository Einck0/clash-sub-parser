import { ref } from 'vue'
import { api, ApiError } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { uiText } from '../../locales/redesign'
import type {
  ProxyChainBinding,
  ProxyChainConfig,
  OptionsResult,
  EffectsResult,
  ChainEffect,
  EffectNode,
} from './types'

export function useChains() {
  const bindings = ref<ProxyChainBinding[]>([])
  const configVersion = ref<string>('')
  const loading = ref(false)
  const error = ref<string | null>(null)

  const options = ref<OptionsResult>({
    nodes: [],
    groups: [],
    subscriptions: [],
  })
  const optionsLoading = ref(false)

  const effects = ref<ChainEffect[]>([])
  const nodes = ref<EffectNode[]>([])
  const effectsLoading = ref(false)
  const effectsError = ref<string | null>(null)

  // Per-row busy & error tracking
  const rowBusy = ref<Record<string, boolean>>({})
  const rowError = ref<Record<string, string | null>>({})

  // Delete modal state
  const isDeleteModalOpen = ref(false)
  const bindingToDelete = ref<ProxyChainBinding | null>(null)
  const deleteLoading = ref(false)
  const deleteError = ref<string | null>(null)

  async function loadBindings() {
    loading.value = true
    error.value = null
    try {
      const res = await api.get<ProxyChainConfig>('/api/v1/proxy-chains')
      bindings.value = res.bindings || []
      configVersion.value = res.version || ''
    } catch (err: any) {
      if (err instanceof ApiError) {
        error.value = err.message
      } else {
        error.value = err?.message || 'Failed to load proxy chain bindings'
      }
    } finally {
      loading.value = false
    }
  }

  async function loadOptions() {
    optionsLoading.value = true
    try {
      const res = await api.get<OptionsResult>('/api/v1/proxy-chains/options')
      options.value = {
        nodes: res.nodes || [],
        groups: res.groups || [],
        subscriptions: res.subscriptions || [],
      }
    } catch {
      // Non-fatal, options can be retried or empty
    } finally {
      optionsLoading.value = false
    }
  }

  async function loadEffects() {
    effectsLoading.value = true
    effectsError.value = null
    try {
      const res = await api.get<EffectsResult>('/api/v1/proxy-chains/effects')
      effects.value = res.effects || []
      nodes.value = res.nodes || []
    } catch (err: any) {
      if (err instanceof ApiError) {
        effectsError.value = err.message
      } else {
        effectsError.value = err?.message || 'Failed to load resolved chain effects'
      }
    } finally {
      effectsLoading.value = false
    }
  }

  async function loadAll() {
    await Promise.all([loadBindings(), loadOptions(), loadEffects()])
  }

  /**
   * Toggles enabled state for a specific binding row via PUT /api/v1/proxy-chains/{id}.
   * Guarded against rapid double-clicks.
   * If update fails, local state is NEVER optimistically flipped.
   */
  async function toggleEnabled(binding: ProxyChainBinding) {
    if (rowBusy.value[binding.id]) return
    rowBusy.value[binding.id] = true
    rowError.value[binding.id] = null

    const newEnabled = !binding.enabled

    try {
      const res = await api.put<{ binding: ProxyChainBinding; version: string }>(
        `/api/v1/proxy-chains/${encodeURIComponent(binding.id)}`,
        {
          binding: {
            id: binding.id,
            target_type: binding.target_type,
            target_id: binding.target_id,
            dialer_type: binding.dialer_type,
            dialer_id: binding.dialer_id,
            enabled: newEnabled,
            sort_order: binding.sort_order,
            note: binding.note,
            revision: binding.revision,
          },
          config_version: configVersion.value,
          expected_revision: binding.revision,
        }
      )

      // Replace binding in array
      const idx = bindings.value.findIndex((b) => b.id === binding.id)
      if (idx !== -1) {
        bindings.value[idx] = res.binding
      }
      configVersion.value = res.version

      toastStore.push({
        tone: 'success',
        duration: 3000,
        message: newEnabled
          ? uiText('链式代理已启用', 'Proxy chain enabled')
          : uiText('链式代理已停用', 'Proxy chain disabled'),
      })

      // Refresh effects
      void loadEffects()
    } catch (err: any) {
      // Revert / do not fake success!
      const msg = err?.message || 'Failed to toggle chain status'
      rowError.value[binding.id] = msg
      toastStore.push({
        tone: 'error',
        duration: 5000,
        title: uiText('切换启用状态失败', 'Toggle Status Failed'),
        message: msg,
      })
    } finally {
      rowBusy.value[binding.id] = false
    }
  }

  function openDeleteConfirm(binding: ProxyChainBinding) {
    bindingToDelete.value = binding
    deleteError.value = null
    isDeleteModalOpen.value = true
  }

  function closeDeleteConfirm() {
    if (deleteLoading.value) return
    isDeleteModalOpen.value = false
    bindingToDelete.value = null
    deleteError.value = null
  }

  async function confirmDelete(): Promise<boolean> {
    if (!bindingToDelete.value) return false
    deleteLoading.value = true
    deleteError.value = null

    const id = bindingToDelete.value.id
    const rev = bindingToDelete.value.revision

    try {
      const res = await api.delete<{ version: string }>(
        `/api/v1/proxy-chains/${encodeURIComponent(id)}`,
        {
          body: {
            config_version: configVersion.value,
            expected_revision: rev,
          } as any,
        }
      )

      bindings.value = bindings.value.filter((b) => b.id !== id)
      configVersion.value = res.version
      isDeleteModalOpen.value = false
      bindingToDelete.value = null

      toastStore.push({
        tone: 'success',
        duration: 3000,
        message: uiText('链式代理绑定已删除', 'Proxy chain binding deleted'),
      })

      void loadEffects()
      return true
    } catch (err: any) {
      // Preserve error for user inspection (e.g. 422 chain_cycle)
      deleteError.value = err?.message || 'Failed to delete chain binding'
      return false
    } finally {
      deleteLoading.value = false
    }
  }

  function onBindingSaved(savedBinding: ProxyChainBinding, newVersion: string) {
    const idx = bindings.value.findIndex((b) => b.id === savedBinding.id)
    if (idx !== -1) {
      bindings.value[idx] = savedBinding
    } else {
      bindings.value.push(savedBinding)
    }
    configVersion.value = newVersion
    void loadEffects()
    toastStore.push({
      tone: 'success',
      duration: 3000,
      message: uiText('链式代理配置已保存', 'Proxy chain binding saved'),
    })
  }

  return {
    bindings,
    configVersion,
    loading,
    error,
    options,
    optionsLoading,
    effects,
    nodes,
    effectsLoading,
    effectsError,
    rowBusy,
    rowError,
    isDeleteModalOpen,
    bindingToDelete,
    deleteLoading,
    deleteError,
    loadBindings,
    loadOptions,
    loadEffects,
    loadAll,
    toggleEnabled,
    openDeleteConfirm,
    closeDeleteConfirm,
    confirmDelete,
    onBindingSaved,
  }
}

export type UseChainsReturn = ReturnType<typeof useChains>
