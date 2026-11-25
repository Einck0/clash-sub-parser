import { computed, ref } from 'vue'
import { displayText } from '../../utils/displayNames'
import { uiText } from '../../locales/redesign'
import { useSettingsDraft } from './useSettingsDraft'

// Use the server's validated canonical PUT response, never an optimistic copy of the request.
export function useServerSettings<T>(options: {
  read: () => Promise<T>
  apply: (value: T) => void
  snapshot: () => unknown
}) {
  const loaded = ref(false)
  const loading = ref(false)
  const saving = ref(false)
  const error = ref('')
  const readFailed = ref(false)
  const baseline = ref('')
  const dirty = computed(() => loaded.value && JSON.stringify(options.snapshot()) !== baseline.value)
  const { confirmDiscard } = useSettingsDraft(dirty)
  const busy = computed(() => loading.value || saving.value)

  function accept(value: T) {
    // apply must validate the entire response before touching the draft.
    options.apply(value)
    baseline.value = JSON.stringify(options.snapshot())
    loaded.value = true
  }

  async function load() {
    if (busy.value || !confirmDiscard()) return false
    loading.value = true
    readFailed.value = false
    error.value = ''
    try {
      accept(await options.read())
      readFailed.value = false
      return true
    } catch (err) {
      readFailed.value = true
      error.value = displayText(err instanceof Error ? err.message : uiText('读取配置失败，请重试', 'Failed to read settings. Retry.'))
      return false
    } finally {
      loading.value = false
    }
  }

  async function save(write: () => Promise<T>) {
    if (!loaded.value || readFailed.value || busy.value) return false
    saving.value = true
    error.value = ''
    try {
      const value = await write()
      try {
        accept(value)
      } catch {
        readFailed.value = true
        throw new Error(uiText(
          '请求已完成，但服务端返回的配置无法确认。草稿已保留，请重新读取核对后再保存。',
          'The request completed, but the returned settings could not be verified. Draft retained; reload to verify before saving again.'
        ))
      }
      return true
    } catch (err) {
      error.value = displayText(err instanceof Error ? err.message : uiText('保存失败，草稿已保留', 'Save failed. Draft retained.'))
      return false
    } finally {
      saving.value = false
    }
  }

  return { loaded, loading, saving, busy, dirty, error, readFailed, load, save }
}
