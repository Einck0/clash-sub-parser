import { computed, ref, watch } from 'vue'
import { api, ApiError } from '../../api/client'
import {
  DEFAULT_COMPILER_TARGET,
  isValidCompilerTarget,
  type CompilerTarget,
  type CompatMode,
  type Diagnostic,
  type PreviewResult,
  type PublicationDetail,
} from './publicationTypes'

export const TARGET_STORAGE_KEY = 'csp_publication_target'
export const PUBLICATION_STORAGE_PREFIX = 'csp_publication_active_'
export const PUBLICATION_LATEST_KEY = 'csp_publication_latest'

const publicationSession = ref(0)

export function clearPublicationCapabilities(): void {
  clearStoredPublication()
  publicationSession.value++
}

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

export function getStoredPublication(
  target?: CompilerTarget,
  storage: Storage | undefined = typeof window !== 'undefined' ? window.sessionStorage : undefined
): PublicationDetail | null {
  if (!storage) return null
  try {
    const key = target ? `${PUBLICATION_STORAGE_PREFIX}${target}` : PUBLICATION_LATEST_KEY
    let raw = storage.getItem(key)
    if (!raw && target) {
      const latestRaw = storage.getItem(PUBLICATION_LATEST_KEY)
      if (latestRaw) {
        const parsed = JSON.parse(latestRaw)
        if (parsed?.target === target) {
          raw = latestRaw
        }
      }
    } else if (!raw && !target) {
      raw = storage.getItem(PUBLICATION_LATEST_KEY)
    }
    if (!raw) return null
    const parsed = JSON.parse(raw)
    if (parsed && typeof parsed === 'object' && parsed.id && parsed.export_url && (!target || parsed.target === target)) {
      return parsed as PublicationDetail
    }
    return null
  } catch {
    return null
  }
}

export function setStoredPublication(
  pub: PublicationDetail,
  storage: Storage | undefined = typeof window !== 'undefined' ? window.sessionStorage : undefined
): boolean {
  if (!storage || !pub || !pub.id) return false
  try {
    const serialized = JSON.stringify(pub)
    storage.setItem(PUBLICATION_LATEST_KEY, serialized)
    if (pub.target) {
      storage.setItem(`${PUBLICATION_STORAGE_PREFIX}${pub.target}`, serialized)
    }
    return true
  } catch {
    return false
  }
}

export function clearStoredPublication(
  target?: CompilerTarget,
  storage: Storage | undefined = typeof window !== 'undefined' ? window.sessionStorage : undefined
): void {
  if (!storage) return
  try {
    if (target) {
      storage.removeItem(`${PUBLICATION_STORAGE_PREFIX}${target}`)
      const latest = storage.getItem(PUBLICATION_LATEST_KEY)
      if (latest && JSON.parse(latest)?.target === target) {
        storage.removeItem(PUBLICATION_LATEST_KEY)
      }
    } else {
      storage.removeItem(PUBLICATION_LATEST_KEY)
      for (let i = storage.length - 1; i >= 0; i--) {
        const key = storage.key(i)
        if (key?.startsWith(PUBLICATION_STORAGE_PREFIX)) storage.removeItem(key)
      }
    }
  } catch {}
}

export function usePublications(initialTarget?: CompilerTarget) {
  const resolvedInitial: CompilerTarget =
    initialTarget && isValidCompilerTarget(initialTarget)
      ? initialTarget
      : getStoredTarget()

  const selectedTarget = ref<CompilerTarget>(resolvedInitial)
  const compatMode = ref<CompatMode>('strict')
  const preview = ref<PreviewResult | null>(null)
  const activePublication = ref<PublicationDetail | null>(getStoredPublication(resolvedInitial))
  watch(publicationSession, () => {
    activePublication.value = null
  }, { flush: 'sync' })
  const loadingPreview = ref(false)
  const publishing = ref(false)
  const revoking = ref(false)
  const error = ref('')
  const errorDetail = ref<ApiError | Error | null>(null)
  const preflightDiagnostics = ref<Diagnostic[]>([])
  const copied = ref(false)
  let previewRequest = 0
  const publishRequests = new Map<CompilerTarget, number>()

  // Selection belongs to the user, never to an asynchronous server response.
  watch(selectedTarget, target => {
    ++previewRequest
    loadingPreview.value = false
    setStoredTarget(target)
    activePublication.value = getStoredPublication(target)
    preview.value = null
    error.value = ''
    errorDetail.value = null
    preflightDiagnostics.value = []
  }, { flush: 'sync' })

  watch(compatMode, () => {
    preview.value = null
    error.value = ''
    errorDetail.value = null
    preflightDiagnostics.value = []
  })

  const isNoActiveRevision = computed(() => {
    const err = errorDetail.value
    if (err && err instanceof ApiError) {
      return err.status === 409 && err.code === 'no_active_revision'
    }
    return false
  })

  async function fetchPreview(
    target: CompilerTarget,
    revisionId?: string,
    mode?: CompatMode
  ): Promise<PreviewResult | null> {
    const request = ++previewRequest
    const session = publicationSession.value
    const effectiveMode = mode ?? compatMode.value ?? 'strict'
    if (mode && compatMode.value !== mode) {
      compatMode.value = mode
    }
    loadingPreview.value = true
    error.value = ''
    errorDetail.value = null
    preflightDiagnostics.value = []
    try {
      const payload: Record<string, string> = {
        target,
        compat_mode: effectiveMode,
      }
      if (revisionId) payload.revision_id = revisionId
      const res = await api.post<PreviewResult>('/api/v1/publications/preview', payload)
      if (request === previewRequest && selectedTarget.value === target && session === publicationSession.value) {
        preview.value = res
        if (res?.diagnostics && res.diagnostics.length > 0) {
          preflightDiagnostics.value = res.diagnostics
        }
      }
      return res
    } catch (err) {
      if (request === previewRequest && selectedTarget.value === target && session === publicationSession.value) {
        preview.value = null
        errorDetail.value = err instanceof Error ? err : new Error(String(err))
        error.value = err instanceof Error ? err.message : '获取配置预览失败'
        if (err instanceof ApiError) {
          const details = err.details as any
          const diags = details?.diagnostics || details?.error?.details?.diagnostics || (Array.isArray(details) ? details : undefined)
          if (Array.isArray(diags)) {
            preflightDiagnostics.value = diags
          }
        }
      }
      return null
    } finally {
      if (request === previewRequest) loadingPreview.value = false
    }
  }

  async function publish(target: CompilerTarget, snapshotIdOrRevisionId?: string): Promise<PublicationDetail> {
    const session = publicationSession.value
    const request = (publishRequests.get(target) ?? 0) + 1
    publishRequests.set(target, request)
    publishing.value = true
    error.value = ''
    errorDetail.value = null
    preflightDiagnostics.value = []

    let effectiveSnapshotId: string | undefined
    if (snapshotIdOrRevisionId) {
      effectiveSnapshotId = snapshotIdOrRevisionId
    } else if (preview.value?.snapshot_id) {
      effectiveSnapshotId = preview.value.snapshot_id
    }

    if (preview.value?.snapshot_id && preview.value.target && preview.value.target !== target) {
      const mismatchErr = new ApiError(
        422,
        'snapshot_target_mismatch',
        `当前快照目标 (${preview.value.target}) 与请求发布目标 (${target}) 不匹配，请重新生成预览`
      )
      errorDetail.value = mismatchErr
      error.value = mismatchErr.message
      publishing.value = false
      throw mismatchErr
    }

    try {
      const payload: Record<string, string> = { target }
      if (effectiveSnapshotId) {
        payload.snapshot_id = effectiveSnapshotId
      }
      const rawRes = await api.post<any>('/api/v1/publications', payload)
      if (session !== publicationSession.value) throw new Error('publication session changed')
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
      if (!res.id || !res.export_url || new URL(res.export_url, 'http://localhost').pathname !== `/publish/v1/${encodeURIComponent(res.id)}`) {
        throw new Error('Publication response does not contain a valid ID and export URL')
      }
      if (request === publishRequests.get(target)) {
        setStoredPublication(res)
        if (selectedTarget.value === target) activePublication.value = res
      }
      return res
    } catch (err) {
      if (session === publicationSession.value && request === publishRequests.get(target) && selectedTarget.value === target) {
        errorDetail.value = err instanceof Error ? err : new Error(String(err))
        const msg = err instanceof Error ? err.message : '创建订阅发布失败'
        error.value = msg
        if (err instanceof ApiError) {
          const details = err.details as any
          const diags = details?.diagnostics || details?.error?.details?.diagnostics
          if (Array.isArray(diags)) {
            preflightDiagnostics.value = diags
          }
        }
      }
      throw err
    } finally {
      publishing.value = false
    }
  }

  async function fetchPublication(id: string): Promise<PublicationDetail | null> {
    const session = publicationSession.value
    try {
      const rawRes = await api.get<any>(`/api/v1/publications/${id}`)
      if (session !== publicationSession.value) return null
      const rawTarget = rawRes?.publication?.target || rawRes?.target
      const res: PublicationDetail = {
        id: rawRes?.publication?.id || rawRes?.id || '',
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
      if (res.id !== id || !res.export_url || new URL(res.export_url, 'http://localhost').pathname !== `/publish/v1/${encodeURIComponent(id)}`) {
        throw new Error('Publication detail does not match the requested ID')
      }
      if (selectedTarget.value === res.target && activePublication.value?.id === id) {
        // The GET response cannot recover the one-time publication token.
        res.export_url = activePublication.value.export_url
        activePublication.value = res
      }
      return res
    } catch (err) {
      if (session === publicationSession.value) {
        errorDetail.value = err instanceof Error ? err : new Error(String(err))
        error.value = err instanceof Error ? err.message : '获取发布详情失败'
      }
      return null
    }
  }

  async function revoke(id: string) {
    const session = publicationSession.value
    revoking.value = true
    error.value = ''
    try {
      await api.post(`/api/v1/publications/${id}/revoke`)
      if (session !== publicationSession.value) return
      if (activePublication.value?.id === id) {
        activePublication.value.revoked_at = new Date().toISOString()
        activePublication.value.state = 'revoked'
        clearStoredPublication(activePublication.value.target)
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : '撤销订阅发布失败'
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

  function restoreActivePublication(target?: CompilerTarget): PublicationDetail | null {
    const found = getStoredPublication(target ?? selectedTarget.value)
    activePublication.value = found
    return found
  }

  function getFullExportUrl(pub?: PublicationDetail | null, protectedExport = true, suppliedToken = ''): string {
    const targetPub = pub ?? activePublication.value
    if (!targetPub?.id || !targetPub.export_url || targetPub.target !== selectedTarget.value || targetPub.revoked_at || targetPub.state === 'revoked') return ''
    const origin = typeof window !== 'undefined' ? window.location.origin : 'http://localhost'
    const url = new URL(targetPub.export_url, origin)
    if (url.origin !== origin || url.pathname !== `/publish/v1/${encodeURIComponent(targetPub.id)}`) return ''
    const token = suppliedToken.trim() || url.searchParams.get('token') || ''
    url.search = ''
    url.hash = ''
    if (protectedExport) {
      if (!token) return ''
      url.searchParams.set('token', token)
    }
    return url.toString()
  }

  return {
    selectedTarget,
    compatMode,
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
    restoreActivePublication,
    getFullExportUrl,
  }
}
