import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useGroupPreview } from './useGroupPreview'
import { api } from '../../api/client'
import type { GroupDraftPreviewPayload } from './policyTypes'

describe('useGroupPreview Composable', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  const testPayload: GroupDraftPreviewPayload = {
    group_id: 'grp-test',
    expected_revision: 2,
    name: 'Preview Group',
    group_type: 'select',
    sources: [
      { operation: 'add', kind: 'manual_node', position: 0, node_logical_id: 'node-1' },
    ],
  }

  it('schedules preview with 250ms debounce and populates snapshot results', async () => {
    const postSpy = vi.spyOn(api, 'post').mockResolvedValue({
      group_id: 'grp-test',
      revision: 2,
      selection_digest: 'digest-abc',
      node_count: 5,
      member_count: 5,
      candidate_count: 8,
      excluded_count: 3,
      members: [{ node_logical_id: 'node-1', kind: 'manual_node' }],
      diagnostics: [],
    })

    const preview = useGroupPreview()
    preview.schedulePreview(testPayload, 250)

    // Initially not called
    expect(postSpy).not.toHaveBeenCalled()
    expect(preview.snapshotPreview.value).toBeNull()

    // Advance timers by 250ms
    vi.advanceTimersByTime(250)
    await Promise.resolve()

    expect(postSpy).toHaveBeenCalledTimes(1)
    expect(postSpy).toHaveBeenCalledWith('/api/v1/policies/groups/preview', expect.objectContaining({
      group_id: 'grp-test',
      expected_revision: 2,
      target: 'mihomo',
    }))

    expect(preview.snapshotPreview.value?.selection_digest).toBe('digest-abc')
    expect(preview.snapshotPreview.value?.member_count).toBe(5)
  })

  it('discards stale out-of-order preview response and keeps latest generation', async () => {
    let resolveGen1!: (v: any) => void
    let resolveGen2!: (v: any) => void

    let callCount = 0
    vi.spyOn(api, 'post').mockImplementation(() => {
      callCount++
      if (callCount === 1) {
        return new Promise((r) => { resolveGen1 = r })
      } else {
        return new Promise((r) => { resolveGen2 = r })
      }
    })

    const preview = useGroupPreview()

    // Generation 1
    preview.triggerImmediate({ ...testPayload, name: 'Gen 1' })
    expect(callCount).toBe(1)

    // Generation 2
    preview.triggerImmediate({ ...testPayload, name: 'Gen 2' })
    expect(callCount).toBe(2)

    // Resolve Generation 2 first with 10 members
    resolveGen2({
      selection_digest: 'gen-2-digest',
      member_count: 10,
      node_count: 10,
      members: [],
    })
    await Promise.resolve()

    expect(preview.snapshotPreview.value?.selection_digest).toBe('gen-2-digest')
    expect(preview.snapshotPreview.value?.member_count).toBe(10)

    // Resolve Generation 1 late with 1 member
    resolveGen1({
      selection_digest: 'gen-1-stale-digest',
      member_count: 1,
      node_count: 1,
      members: [],
    })
    await Promise.resolve()

    // Stale generation 1 response must be ignored
    expect(preview.snapshotPreview.value?.selection_digest).toBe('gen-2-digest')
    expect(preview.snapshotPreview.value?.member_count).toBe(10)
  })

  it('invalidates preview and clears pending requests on cleanup', () => {
    const postSpy = vi.spyOn(api, 'post').mockResolvedValue({})
    const preview = useGroupPreview()

    preview.schedulePreview(testPayload, 250)
    preview.invalidatePreview()

    vi.advanceTimersByTime(300)
    expect(postSpy).not.toHaveBeenCalled()
    expect(preview.snapshotPreview.value).toBeNull()
  })

  it('records preview error on request failure without crashing', async () => {
    vi.spyOn(api, 'post').mockRejectedValue(new Error('Network timeout'))
    const preview = useGroupPreview()

    preview.triggerImmediate(testPayload)
    await Promise.resolve()

    expect(preview.previewError.value).toBe('快照解析失败: Network timeout')
    expect(preview.previewLoading.value).toBe(false)
  })

  it('sanitizes sensitive tokens in preview error without leaking credentials', async () => {
    vi.spyOn(api, 'post').mockRejectedValue(new Error('Authentication failed with token secret-token-1234567890'))
    const preview = useGroupPreview()

    preview.triggerImmediate(testPayload)
    await Promise.resolve()

    expect(preview.previewError.value).toContain('快照解析失败')
    expect(preview.previewError.value).not.toContain('secret-token-1234567890')
    expect(preview.previewError.value).toContain('[REDACTED]')
    expect(preview.previewLoading.value).toBe(false)
  })
})
