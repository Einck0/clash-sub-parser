import { ref, watch, type Ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'

type UrlValue = string | string[] | null

type UrlTransform<T> = {
  from?: (value: unknown) => T
  to?: (value: T) => UrlValue
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

  // 浏览器前进后退时从地址栏同步状态
  watch(() => route.query[key], (val) => {
    const newVal = val === undefined ? defaultValue : (transform?.from ? transform.from(val) : val as T)
    if (newVal !== state.value) state.value = newVal
  })

  return state
}
