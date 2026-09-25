import { computed, ref } from 'vue'
import { api, ApiError } from '../../api/client'
import type { CompilerTarget, Diagnostic, PreviewResult, PublicationDetail } from './publicationTypes'

export function usePublications() {
  const selectedTarget = ref<CompilerTarget>('clash')
  const preview = ref<PreviewResult | null>(null)
  const activePublication = ref<PublicationDetail | null>(null)
  const loadingPreview = ref(false)
  const publishing = ref(false)
  const revoking = ref(false)
  const error = ref('')
  const errorDetail = ref<ApiError | Error | null>(null)
  const preflightDiagnostics = ref<Diagnostic[]>([])
  const copied = ref(false)

  const isNoActiveRevision = computed(() => {
    const err = errorDetail.value
    if (err && err instanceof ApiError) {
      return err.status === 409 && err.code === 'no_active_revision'
    }
    return false
  })

  async function fetchPreview(target: CompilerTarget, revisionId?: string): Promise<PreviewResult | null> {
    loadingPreview.value = true
    error.value = ''
    errorDetail.value = null
    preflightDiagnostics.value = []
    try {
      const payload: Record<string, string> = { target }
      if (revisionId) payload.revision_id = revisionId
      const res = await api.post<PreviewResult>('/api/v1/publications/preview', payload)
      preview.value = res
      selectedTarget.value = target
      return res
    } catch (err) {
      preview.value = null
      errorDetail.value = err instanceof Error ? err : new Error(String(err))
      error.value = err instanceof Error ? err.message : 'Failed to fetch preview'
      if (err instanceof ApiError && (err.details as any)?.diagnostics) {
        preflightDiagnostics.value = (err.details as any).diagnostics
      }
      return null
    } finally {
      loadingPreview.value = false
    }
  }

  async function publish(target: CompilerTarget, revisionId?: string): Promise<PublicationDetail> {
    publishing.value = true
    error.value = ''
    errorDetail.value = null
    preflightDiagnostics.value = []
    try {
      const payload: Record<string, string> = { target }
      if (revisionId) payload.revision_id = revisionId
      const res = await api.post<PublicationDetail>('/api/v1/publications', payload)
      activePublication.value = res
      return res
    } catch (err) {
      errorDetail.value = err instanceof Error ? err : new Error(String(err))
      const msg = err instanceof Error ? err.message : 'Failed to publish configuration'
      error.value = msg
      if (err instanceof ApiError && (err.details as any)?.diagnostics) {
        preflightDiagnostics.value = (err.details as any).diagnostics
      }
      throw err
    } finally {
      publishing.value = false
    }
  }

  async function revoke(id: string) {
    revoking.value = true
    error.value = ''
    try {
      await api.post(`/api/v1/publications/${id}/revoke`)
      if (activePublication.value?.id === id) {
        activePublication.value.revoked_at = new Date().toISOString()
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to revoke publication'
      throw err
    } finally {
      revoking.value = false
    }
  }

  async function copyToClipboard(text: string): Promise<boolean> {
    try {
      if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(text)
      } else {
        const textarea = document.createElement('textarea')
        textarea.value = text
        textarea.style.position = 'fixed'
        textarea.style.opacity = '0'
        document.body.appendChild(textarea)
        textarea.select()
        document.execCommand('copy')
        document.body.removeChild(textarea)
      }
      copied.value = true
      setTimeout(() => {
        copied.value = false
      }, 2000)
      return true
    } catch {
      return false
    }
  }

  function downloadFile(filename: string, content: string, contentType: string) {
    try {
      const blob = new Blob([content], { type: contentType || 'text/plain' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = filename
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
    } catch {
      // fallback
    }
  }

  return {
    selectedTarget,
    preview,
    activePublication,
    loadingPreview,
    publishing,
    revoking,
    error,
    errorDetail,
    preflightDiagnostics,
    isNoActiveRevision,
    copied,
    fetchPreview,
    publish,
    revoke,
    copyToClipboard,
    downloadFile,
  }
}
