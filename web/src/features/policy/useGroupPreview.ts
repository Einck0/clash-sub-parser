import { onUnmounted, ref } from 'vue'
import { api } from '../../api/client'
import type {
  GroupDraftPreviewPayload,
  GroupDraftPreviewResponse,
  ResolverDiagnostic,
} from './policyTypes'

export function useGroupPreview() {
  const previewLoading = ref(false)
  const snapshotPreview = ref<GroupDraftPreviewResponse | null>(null)
  const previewDiagnostics = ref<Array<ResolverDiagnostic | string>>([])
  const previewError = ref('')
  const previewGeneration = ref(0)

  let previewDebounceTimer: ReturnType<typeof setTimeout> | null = null

  function clearTimer() {
    if (previewDebounceTimer) {
      clearTimeout(previewDebounceTimer)
      previewDebounceTimer = null
    }
  }

  async function executePreviewRequest(
    payload: GroupDraftPreviewPayload,
    gen: number
  ): Promise<void> {
    previewLoading.value = true
    previewError.value = ''

    try {
      const body: Record<string, unknown> = {
        group_id: payload.group_id,
        expected_revision: payload.expected_revision,
        name: payload.name,
        group_type: payload.group_type,
        sources: (payload.sources || []).map((s, idx) => ({
          ...s,
          position: s.position !== undefined ? s.position : idx,
        })),
        target: payload.target || 'mihomo',
      }
      if (payload.empty_fallback_pass !== undefined) {
        body.empty_fallback_pass = payload.empty_fallback_pass
      }
      if (payload.inherit_global_filter !== undefined) {
        body.inherit_global_filter = payload.inherit_global_filter
      }

      const res = await api.post<GroupDraftPreviewResponse>('/api/v1/policies/groups/preview', body)

      // Out of order response guard: if generation has advanced, discard response
      if (gen !== previewGeneration.value) {
        return
      }

      snapshotPreview.value = res
      previewDiagnostics.value = res.diagnostics || []
      previewError.value = ''
    } catch (err: any) {
      if (gen !== previewGeneration.value) {
        return
      }
      // Request failure is not business invalid: preserve draft and report network/service error
      previewError.value = err?.message || '预览请求失败'
    } finally {
      if (gen === previewGeneration.value) {
        previewLoading.value = false
      }
    }
  }

  function schedulePreview(payload: GroupDraftPreviewPayload, delayMs = 250) {
    clearTimer()
    const gen = ++previewGeneration.value

    previewDebounceTimer = setTimeout(() => {
      previewDebounceTimer = null
      void executePreviewRequest(payload, gen)
    }, delayMs)
  }

  function triggerImmediate(payload: GroupDraftPreviewPayload) {
    clearTimer()
    const gen = ++previewGeneration.value
    void executePreviewRequest(payload, gen)
  }

  function invalidatePreview() {
    clearTimer()
    previewGeneration.value++
    snapshotPreview.value = null
    previewDiagnostics.value = []
    previewLoading.value = false
    previewError.value = ''
  }

  function cleanup() {
    clearTimer()
    previewGeneration.value++
    snapshotPreview.value = null
    previewDiagnostics.value = []
    previewLoading.value = false
    previewError.value = ''
  }

  onUnmounted(() => {
    cleanup()
  })

  return {
    previewLoading,
    snapshotPreview,
    previewDiagnostics,
    previewError,
    previewGeneration,
    schedulePreview,
    triggerImmediate,
    invalidatePreview,
    cleanup,
  }
}

export type UseGroupPreviewReturn = ReturnType<typeof useGroupPreview>
