import { computed, ref } from 'vue'
import { api } from '../../api/client'
import { normalizeNode, type NodeRecord, type NormalizedNode } from './nodeView'

interface NodePage {
  items: NodeRecord[]
  page: number
  page_size: number
  total: number
}

export function useNodes() {
  const items = ref<NormalizedNode[]>([])
  const loading = ref(false)
  const loadingMore = ref(false)
  const error = ref('')
  const page = ref(0)
  const pageSize = 100
  const total = ref(0)
  const requestGeneration = ref(0)
  const hasMore = computed(() => items.value.length < total.value)

  async function loadPage(nextPage: number, append = false, generation = requestGeneration.value) {
    if (append && (loadingMore.value || loading.value)) return
    if (append) loadingMore.value = true
    else loading.value = true
    error.value = ''
    try {
      const result = await api.get<NodePage>('/api/v1/nodes', {
        params: { page: nextPage, page_size: pageSize, sort_by: 'display_name', sort_order: 'asc' },
      })
      if (generation !== requestGeneration.value) return
      const normalized = result.items.map(normalizeNode)
      items.value = append ? [...items.value, ...normalized] : normalized
      page.value = result.page
      total.value = result.total
    } catch (cause) {
      if (generation === requestGeneration.value) {
        error.value = cause instanceof Error ? cause.message : 'Unable to load nodes'
      }
    } finally {
      if (generation === requestGeneration.value) {
        loading.value = false
        loadingMore.value = false
      }
    }
  }

  async function load() {
    requestGeneration.value += 1
    if (loadingMore.value) loadingMore.value = false
    await loadPage(1, false, requestGeneration.value)
  }

  async function loadMore() {
    if (hasMore.value) await loadPage(page.value + 1, true, requestGeneration.value)
  }

  return { items, loading, loadingMore, error, total, hasMore, load, loadMore }
}
