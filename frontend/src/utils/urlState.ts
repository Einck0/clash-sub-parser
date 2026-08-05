import { ref, watch, onMounted, type Ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'

type UrlValue = string | string[] | null

type UrlTransform<T> = {
  from?: (value: unknown) => T
  to?: (value: T) => UrlValue
}

type UrlStateEntry = {
  state: Ref<unknown>
  defaultValue: unknown
  transform?: UrlTransform<unknown>
}

/**
 * Sync a reactive ref with URL query params.
 * Call this in a view's setup to keep URL in sync with filter/search/page state.
 */
export function useUrlState<T>(key: string, defaultValue: T, options: { transform?: UrlTransform<T> } = {}) {
  const router = useRouter()
  const route = useRoute()
  const { transform } = options

  const fromUrl = (): T => {
    const val = route.query[key]
    if (val === undefined || val === null) return defaultValue
    if (transform?.from) return transform.from(val)
    return val as T
  }

  const state = ref<T>(fromUrl())

  watch(state, (val) => {
    const query = { ...route.query }
    if (val === defaultValue || val === '' || val === null || val === undefined) {
      delete query[key]
    } else {
      query[key] = transform?.to ? transform.to(val) : String(val)
    }
    router.replace({ query })
  })

  // Sync from URL on popstate (browser back/forward)
  watch(() => route.query[key], (val) => {
    const newVal = val === undefined ? defaultValue : (transform?.from ? transform.from(val) : val as T)
    if (newVal !== state.value) state.value = newVal
  })

  return state
}

/**
 * Batch sync multiple URL state refs
 */
export function useUrlStates(states: Record<string, UrlStateEntry>) {
  const router = useRouter()
  const route = useRoute()

  onMounted(() => {
    // Restore from URL on mount
    for (const [key, { state, transform }] of Object.entries(states)) {
      const val = route.query[key]
      if (val !== undefined) {
        state.value = transform?.from ? transform.from(val) : val
      }
    }
  })

  // Watch each state and update URL
  for (const { state } of Object.values(states)) {
    watch(state, () => {
      const query = { ...route.query }
      for (const [key, { state: currentState, defaultValue, transform }] of Object.entries(states)) {
        const value = currentState.value
        if (value === defaultValue || value === '' || value === null || value === undefined) {
          delete query[key]
        } else {
          query[key] = transform?.to ? transform.to(value) : String(value)
        }
      }
      router.replace({ query })
    })
  }
}
