import { onUnmounted, ref } from 'vue'
import { api, ApiError } from '../../api/client'
import type {
  GroupDraftPreviewPayload,
  GroupDraftPreviewResponse,
  ResolverDiagnostic,
} from './policyTypes'

export function sanitizeErrorMessage(msg: string): string {
  if (!msg) return ''
  return msg
    .replace(/(?:token|secret|password|key|auth|bearer|credential)[=:\s]+[A-Za-z0-9_\-\.]{8,}/gi, (match) => {
      const prefix = match.split(/[=:\s]+/)[0]
      return `${prefix}: [REDACTED]`
    })
    .replace(/[A-Za-z0-9_\-]{32,}/g, '[REDACTED]')
}

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
      snapshotPreview.value = null
      if (err instanceof ApiError) {
        if (err.status === 422 && err.code === 'cycle_detected') {
          const cycle = (err.details as any)?.cycle_path || []
          previewError.value = `检测到循环依赖：${cycle.join(' → ')}`
        } else {
          const raw = err.message || `HTTP ${err.status}`
          previewError.value = `快照解析失败: ${sanitizeErrorMessage(raw)}`
        }
      } else {
        const raw = err instanceof Error ? err.message : '未知异常'
        previewError.value = `快照解析失败: ${sanitizeErrorMessage(raw)}`
      }
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
