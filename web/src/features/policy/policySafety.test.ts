// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref, type Component } from 'vue'
import { api, ApiError } from '../../api/client'
import PolicyEditorSheet from './PolicyEditorSheet.vue'
import PolicyView from './PolicyView.vue'
import SourcePipelineEditor from './SourcePipelineEditor.vue'
import type { PolicyGroup } from './policyTypes'

const group: PolicyGroup = {
  id: 'grp-safety', name: 'Original group', group_type: 'select', revision: 2, sources: [],
}
const mounted: Array<{ app: ReturnType<typeof createApp>; host: HTMLDivElement }> = []

async function flush() {
  for (let i = 0; i < 12; i++) await Promise.resolve()
  await nextTick()
}

async function mount(component: Component, props: Record<string, unknown> = {}) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp({ render: () => h(component, props) })
  mounted.push({ app, host })
  app.mount(host)
  await flush()
  return host
}

function dialog() {
  return document.body.querySelector<HTMLElement>('[data-drawer-panel]')!
}

function button(text: string) {
  return Array.from(dialog().querySelectorAll<HTMLButtonElement>('button')).find(b => b.textContent?.trim() === text)!
}

function editName(value: string) {
  const input = dialog().querySelector<HTMLInputElement>('input[type="text"]')!
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
  return input
}

async function openGroup() {
  const host = await mount(PolicyView)
  host.querySelector<HTMLElement>('[data-testid="group-card"] .cursor-pointer')!.click()
  await flush()
}

beforeEach(() => {
  vi.spyOn(api, 'get').mockImplementation(async path => {
    if (path === '/api/v1/policies/groups') return { items: [group], total: 1 }
    if (path === '/api/v1/policies/global-node-filter') return { spec: { conditions: [] } }
    return { items: [], total: 0 }
  })
  vi.spyOn(api, 'post').mockResolvedValue({ valid: true })
  if (!HTMLDialogElement.prototype.close) HTMLDialogElement.prototype.close = function () { this.open = false }
  if (!HTMLDialogElement.prototype.showModal) HTMLDialogElement.prototype.showModal = function () { this.open = true }
})

afterEach(() => {
  mounted.splice(0).forEach(({ app, host }) => { app.unmount(); host.remove() })
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('Policy editor draft and recovery safety', () => {
  it('handles a Drawer close request once, retaining a declined dirty draft', async () => {
    const open = ref(true)
    const onClose = vi.fn(() => { open.value = false })
    const confirm = vi.spyOn(window, 'confirm').mockReturnValueOnce(false).mockReturnValueOnce(true)
    await mount({ render: () => h(PolicyEditorSheet, { open: open.value, group, allGroups: [group], onClose }) })
    editName('Local draft')
    await flush()

    dialog().querySelector<HTMLButtonElement>('[data-testid="drawer-close"]')!.click()
    await flush()
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(onClose).not.toHaveBeenCalled()
    expect(dialog().querySelector<HTMLInputElement>('input[type="text"]')!.value).toBe('Local draft')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }))
    await flush()
    expect(confirm).toHaveBeenCalledTimes(2)
    expect(onClose).toHaveBeenCalledTimes(1)
    expect(dialog()).toBeNull()
    const unload = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(unload)
    expect(unload.defaultPrevented).toBe(false)
  })

  it('keeps the global filter shortcut on the shared draft-safe settings path', async () => {
    const host = await mount(PolicyView)
    host.querySelector<HTMLButtonElement>('[data-testid="global-filter-btn"]')!.click()
    await flush()
    const modal = host.querySelector<HTMLDialogElement>('dialog[open]')!
    const emptyPass = modal.querySelector<HTMLInputElement>('[data-testid="global-empty-pass"]')!
    emptyPass.click()
    await flush()
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    modal.querySelector<HTMLButtonElement>('[data-testid="modal-dialog-close"]')!.click()
    await flush()
    expect(confirm).toHaveBeenCalledOnce()
    expect(modal.open).toBe(true)
    expect(emptyPass.checked).toBe(false)

    const put = vi.spyOn(api, 'put').mockRejectedValueOnce(new Error('Global filter rejected'))
    modal.querySelector<HTMLButtonElement>('[data-testid="filter-save"]')!.click()
    await flush()
    expect(put).toHaveBeenCalledWith('/api/v1/policies/global-node-filter', {
      spec: { conditions: [] }, empty_fallback_pass: false,
    })
    expect(modal.textContent).toContain('Global filter rejected')
    expect(emptyPass.checked).toBe(false)
    const unload = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(unload)
    expect(unload.defaultPrevented).toBe(true)
  })

  it('keeps dirty state while save is pending or rejected, and clears it only after success', async () => {
    let rejectSave!: (error: Error) => void
    const put = vi.spyOn(api, 'put').mockImplementationOnce(() => new Promise((_resolve, reject) => { rejectSave = reject }))
    await openGroup()
    editName('Local draft')
    await flush()
    dialog().querySelector<HTMLButtonElement>('[data-testid="policy-sheet-save-btn"]')!.click()
    await flush()
    expect(put).toHaveBeenCalledTimes(1)
    expect(dialog().textContent).toContain('草稿已修改')
    const pendingUnload = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(pendingUnload)
    expect(pendingUnload.defaultPrevented).toBe(true)

    rejectSave(new ApiError(500, 'write_failed', 'Save failed; draft must remain'))
    await flush()
    expect(dialog().textContent).toContain('Save failed; draft must remain')
    expect(dialog().querySelector<HTMLInputElement>('input[type="text"]')!.value).toBe('Local draft')
    expect(dialog().textContent).toContain('草稿已修改')

    put.mockResolvedValueOnce({ ...group, name: 'Local draft', revision: 3 })
    dialog().querySelector<HTMLButtonElement>('[data-testid="policy-sheet-save-btn"]')!.click()
    await flush()
    expect(dialog()).toBeNull()
    const savedUnload = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(savedUnload)
    expect(savedUnload.defaultPrevented).toBe(false)
  })

  it('fetches a real missing conflict revision, preserving the draft if that read fails', async () => {
    const put = vi.spyOn(api, 'put').mockRejectedValue(new ApiError(409, 'revision_conflict', 'Version changed'))
    await openGroup()
    editName('Local draft')
    await flush()
    dialog().querySelector<HTMLButtonElement>('[data-testid="policy-sheet-save-btn"]')!.click()
    await flush()
    expect(put).toHaveBeenCalledWith('/api/v1/policies/groups/grp-safety', expect.objectContaining({ expected_revision: 2 }))

    vi.mocked(api.get).mockRejectedValueOnce(new Error('Latest revision read failed'))
    button('保留本地草稿').click()
    await flush()
    expect(dialog().textContent).toContain('Latest revision read failed')
    expect(dialog().textContent).toContain('版本冲突')
    expect(dialog().textContent).toContain('版本 2')
    expect(dialog().querySelector<HTMLInputElement>('input[type="text"]')!.value).toBe('Local draft')

    vi.mocked(api.get).mockResolvedValueOnce({ ...group, revision: 9 })
    button('保留本地草稿').click()
    await flush()
    expect(api.get).toHaveBeenLastCalledWith('/api/v1/policies/groups/grp-safety')
    expect(dialog().textContent).toContain('版本 9')
    dialog().querySelector<HTMLButtonElement>('[data-testid="policy-sheet-save-btn"]')!.click()
    await flush()
    expect(put).toHaveBeenLastCalledWith('/api/v1/policies/groups/grp-safety', expect.objectContaining({ name: 'Local draft', expected_revision: 9 }))
  })

  it('confirms reload and preserves content and conflict after a failed read', async () => {
    vi.spyOn(api, 'put').mockRejectedValue(new ApiError(409, 'revision_conflict', 'Version changed', undefined, { current_revision: 5 }))
    const confirm = vi.spyOn(window, 'confirm').mockReturnValueOnce(false).mockReturnValue(true)
    await openGroup()
    editName('Local draft')
    await flush()
    dialog().querySelector<HTMLButtonElement>('[data-testid="policy-sheet-save-btn"]')!.click()
    await flush()

    const get = vi.mocked(api.get)
    const reads = get.mock.calls.length
    button('重新拉取服务端版本').click()
    await flush()
    expect(get.mock.calls).toHaveLength(reads)

    get.mockRejectedValueOnce(new Error('Reload failed'))
    button('重新拉取服务端版本').click()
    await flush()
    expect(confirm).toHaveBeenCalledTimes(2)
    expect(dialog().textContent).toContain('Reload failed')
    expect(dialog().textContent).toContain('版本冲突')
    expect(dialog().querySelector<HTMLInputElement>('input[type="text"]')!.value).toBe('Local draft')
    expect(dialog().textContent).toContain('草稿已修改')

    get.mockResolvedValueOnce({ ...group, name: 'Server version', revision: 5 })
    button('重新拉取服务端版本').click()
    await flush()
    expect(dialog().querySelector<HTMLInputElement>('input[type="text"]')!.value).toBe('Server version')
    expect(dialog().textContent).toContain('无未保存改动')
    expect(dialog().textContent).not.toContain('版本冲突')
  })

  it('provides keyboard-operated narrow tabs and wrapping body errors', async () => {
    await mount(PolicyEditorSheet, {
      open: true, group, allGroups: [group], topologyStatus: 'error',
      topologyError: 'Full topology failed: '.repeat(30), serverError: 'Save failure: '.repeat(30),
    })
    const tabs = dialog().querySelectorAll<HTMLButtonElement>('[role="tab"]')
    tabs[0].focus()
    tabs[0].dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true }))
    await flush()
    expect(document.activeElement).toBe(tabs[1])
    expect(tabs[1].getAttribute('aria-selected')).toBe('true')
    expect(document.getElementById(tabs[1].getAttribute('aria-controls')!)?.getAttribute('aria-labelledby')).toBe(tabs[1].id)
    expect(dialog().querySelector('[data-testid="sheet-topology-error"] span')?.className).not.toContain('truncate')
    expect(dialog().textContent).toContain('Full topology failed: ')
    expect(dialog().textContent).toContain('Save failure: ')
  })
})

describe('Remote policy source candidates', () => {
  async function mountNodes(onAdd = vi.fn()) {
    const host = await mount(SourcePipelineEditor, {
      name: 'Sources', groupType: 'select', position: 0, inheritGlobalFilter: true, sources: [],
      availableNodes: [{ logicalId: 'old-fallback', displayName: 'Old fallback', active: true }],
      'onAdd-source': onAdd,
    })
    host.querySelector<HTMLInputElement>('input[type="radio"][value="node"]')!.click()
    await flush()
    return host
  }

  it('distinguishes empty search from failed read, does not restore unrelated fallback nodes, and retries', async () => {
    vi.useFakeTimers()
    vi.mocked(api.get).mockImplementation(async (path, options) => {
      if (path !== '/api/v1/nodes') return { items: [], total: 0 }
      if (options?.params?.search === 'failure') throw new Error('Remote search unavailable')
      return { items: [], page: 1, total: 0 }
    })
    const host = await mountNodes()
    const select = host.querySelector<HTMLSelectElement>('[data-testid="edge-node-select"]')!
    expect(select.textContent).toContain('暂无匹配候选节点')
    expect(select.textContent).not.toContain('Old fallback')
    expect(host.querySelector('[data-testid="candidate-node-error"]')).toBeNull()

    const search = host.querySelector<HTMLInputElement>('[data-testid="candidate-node-search-input"]')!
    search.value = 'failure'
    search.dispatchEvent(new Event('input'))
    await vi.advanceTimersByTimeAsync(301)
    await flush()
    expect(host.querySelector('[data-testid="candidate-node-error"]')?.textContent).toContain('Remote search unavailable')
    expect(select.textContent).toContain('候选节点读取失败')
    expect(select.disabled).toBe(true)

    vi.mocked(api.get).mockResolvedValueOnce({ items: [{ logical_id: 'node-recovered', display_name: 'Recovered', active: true }], page: 1, total: 1 })
    host.querySelector<HTMLButtonElement>('[data-testid="candidate-node-retry"]')!.click()
    await flush()
    expect(host.querySelector('[data-testid="candidate-node-error"]')).toBeNull()
    expect(select.disabled).toBe(false)
    expect(select.querySelector('option[value="node-recovered"]')).not.toBeNull()
    expect(api.get).toHaveBeenLastCalledWith('/api/v1/nodes', { params: expect.objectContaining({ page: 1, search: 'failure' }) })
  })

  it('exposes an incomplete multi-page group read instead of offering a partial or fallback list', async () => {
    const firstPage = Array.from({ length: 100 }, (_, i) => ({ ...group, id: `child-${i}`, name: `Child ${i}` }))
    let fail = true
    vi.mocked(api.get).mockImplementation(async (path, options) => {
      if (path !== '/api/v1/policies/groups') return { items: [], total: 0 }
      if (options?.params?.page === 2) {
        if (fail) throw new Error('Group page 2 unavailable')
        return { items: [{ ...group, id: 'last-child', name: 'Last child' }], total: 101 }
      }
      return { items: firstPage, total: 101 }
    })
    const host = await mount(SourcePipelineEditor, {
      name: 'Sources', groupType: 'select', position: 0, inheritGlobalFilter: true, sources: [],
      allGroups: [{ ...group, id: 'cached-group', name: 'Cached fallback' }], topologyStatus: 'ready',
    })
    host.querySelector<HTMLInputElement>('input[type="radio"][value="group"]')!.click()
    await flush()
    const select = host.querySelector<HTMLSelectElement>('[data-testid="edge-group-select"]')!
    expect(host.querySelector('[data-testid="candidate-group-error"]')?.textContent).toContain('Group page 2 unavailable')
    expect(select.disabled).toBe(true)
    expect(select.querySelectorAll('option')).toHaveLength(1)
    expect(select.textContent).not.toContain('Cached fallback')

    fail = false
    host.querySelector<HTMLButtonElement>('[data-testid="candidate-group-retry"]')!.click()
    await flush()
    expect(host.querySelector('[data-testid="candidate-group-error"]')).toBeNull()
    expect(select.disabled).toBe(false)
    expect(select.querySelector('option[value="last-child"]')?.textContent).toContain('Last child')
  })

  it('reports a later-page bulk read failure without partially adding sources', async () => {
    const nodes = Array.from({ length: 100 }, (_, i) => ({ logical_id: `node-${i}`, display_name: `Node ${i}`, active: true }))
    vi.mocked(api.get).mockImplementation(async (path, options) => {
      if (path !== '/api/v1/nodes') return { items: [], total: 0 }
      if (options?.params?.page === 2) throw new Error('Second page unavailable')
      const size = Number(options?.params?.page_size || 50)
      return { items: nodes.slice(0, size), page: 1, total: 150 }
    })
    const onAdd = vi.fn()
    const host = await mountNodes(onAdd)
    host.querySelector<HTMLButtonElement>('[data-testid="candidate-select-all"]')!.click()
    await flush()
    expect(host.querySelector('[data-testid="candidate-bulk-error"]')?.textContent).toContain('Second page unavailable')
    expect(onAdd).not.toHaveBeenCalled()
  })
})
