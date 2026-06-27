import { ref, watch, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'

/**
 * Sync a reactive ref with URL query params.
 * Call this in a view's setup to keep URL in sync with filter/search/page state.
 */
export function useUrlState(key, defaultValue, { transform } = {}) {
  const router = useRouter()
  const route = useRoute()

  const fromUrl = () => {
    const val = route.query[key]
    if (val === undefined || val === null) return defaultValue
    if (transform?.from) return transform.from(val)
    return val
  }

  const state = ref(fromUrl())

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
    const newVal = val === undefined ? defaultValue : (transform?.from ? transform.from(val) : val)
    if (newVal !== state.value) state.value = newVal
  })

  return state
}

/**
 * Batch sync multiple URL state refs
 */
export function useUrlStates(states) {
  const router = useRouter()
  const route = useRoute()

  onMounted(() => {
    // Restore from URL on mount
    for (const [key, { state, defaultValue, transform }] of Object.entries(states)) {
      const val = route.query[key]
      if (val !== undefined) {
        state.value = transform?.from ? transform.from(val) : val
      }
    }
  })

  // Watch each state and update URL
  for (const [key, { state, defaultValue, transform }] of Object.entries(states)) {
    watch(state, () => {
      const query = { ...route.query }
      for (const [k, { state: s, defaultValue: d, transform: t }] of Object.entries(states)) {
        const v = s.value
        if (v === d || v === '' || v === null || v === undefined) {
          delete query[k]
        } else {
          query[k] = t?.to ? t.to(v) : String(v)
        }
      }
      router.replace({ query })
    })
  }
}
