import { computed, ref } from 'vue'
import { api, ApiError } from '../../api/client'
import {
  DEFAULT_COMPILER_TARGET,
  isValidCompilerTarget,
  type CompilerTarget,
  type Diagnostic,
  type PreviewResult,
  type PublicationDetail,
} from './publicationTypes'

export const TARGET_STORAGE_KEY = 'csp_publication_target'

export function getStoredTarget(
  storage: Storage | undefined = typeof window !== 'undefined' ? window.localStorage : undefined
): CompilerTarget {
  if (!storage) return DEFAULT_COMPILER_TARGET
  try {
    const raw = storage.getItem(TARGET_STORAGE_KEY)
    return isValidCompilerTarget(raw) ? raw : DEFAULT_COMPILER_TARGET
  } catch {
    return DEFAULT_COMPILER_TARGET
  }
}

export function setStoredTarget(
  target: CompilerTarget,
  storage: Storage | undefined = typeof window !== 'undefined' ? window.localStorage : undefined
): boolean {
  if (!storage || !isValidCompilerTarget(target)) {
    return false
  }
  try {
    storage.setItem(TARGET_STORAGE_KEY, target)
    return true
  } catch {
    return false
  }
}

export function usePublications(initialTarget?: CompilerTarget) {
  const resolvedInitial: CompilerTarget =
    initialTarget && isValidCompilerTarget(initialTarget)
      ? initialTarget
      : getStoredTarget()

  const selectedTarget = ref<CompilerTarget>(resolvedInitial)
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
      if (isValidCompilerTarget(target)) {
        selectedTarget.value = target
        setStoredTarget(target)
      }
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
      const rawRes = await api.post<any>('/api/v1/publications', payload)
      const resolvedTarget: CompilerTarget = isValidCompilerTarget(rawRes?.publication?.target)
        ? rawRes.publication.target
        : isValidCompilerTarget(rawRes?.target)
        ? rawRes.target
        : target
      const res: PublicationDetail = {
        id: rawRes?.publication?.id || rawRes?.id || '',
        target: resolvedTarget,
        state: rawRes?.publication?.state || rawRes?.state || 'active',
        snapshot_digest: rawRes?.publication?.snapshot_digest || rawRes?.snapshot_digest || '',
        content_digest: rawRes?.content_digest,
        content_type: rawRes?.content_type,
        filename: rawRes?.filename,
        export_url: rawRes?.export_url,
        revoked_at: rawRes?.publication?.revoked_at || rawRes?.revoked_at,
        created_at: rawRes?.publication?.created_at || rawRes?.created_at || new Date().toISOString(),
      }
      activePublication.value = res
      if (isValidCompilerTarget(target)) {
        selectedTarget.value = target
        setStoredTarget(target)
      }
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

  async function fetchPublication(id: string): Promise<PublicationDetail | null> {
    try {
      const rawRes = await api.get<any>(`/api/v1/publications/${id}`)
      const rawTarget = rawRes?.publication?.target || rawRes?.target
      const res: PublicationDetail = {
        id: rawRes?.publication?.id || rawRes?.id || id,
        target: isValidCompilerTarget(rawTarget) ? rawTarget : DEFAULT_COMPILER_TARGET,
        state: rawRes?.publication?.state || rawRes?.state || 'active',
        snapshot_digest: rawRes?.publication?.snapshot_digest || rawRes?.snapshot_digest || '',
        content_digest: rawRes?.content_digest,
        content_type: rawRes?.content_type,
        filename: rawRes?.filename,
        export_url: rawRes?.export_url,
        revoked_at: rawRes?.publication?.revoked_at || rawRes?.revoked_at,
        created_at: rawRes?.publication?.created_at || rawRes?.created_at || new Date().toISOString(),
      }
      activePublication.value = res
      return res
    } catch (err) {
      errorDetail.value = err instanceof Error ? err : new Error(String(err))
      error.value = err instanceof Error ? err.message : 'Failed to fetch publication'
      return null
    }
  }

  async function revoke(id: string) {
    revoking.value = true
    error.value = ''
    try {
      await api.post(`/api/v1/publications/${id}/revoke`)
      if (activePublication.value?.id === id) {
        activePublication.value.revoked_at = new Date().toISOString()
        activePublication.value.state = 'revoked'
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
    fetchPublication,
    revoke,
    copyToClipboard,
    downloadFile,
  }
}
