import { computed, inject, onUnmounted, provide, shallowRef, type InjectionKey, type Ref } from 'vue'
import { useUnsavedChanges } from '../../composables/useUnsavedChanges'
import { uiText } from '../../locales/redesign'

type Draft = Readonly<Ref<boolean>>
const settingsDraftsKey: InjectionKey<(draft: Draft) => void> = Symbol('settings-drafts')

export const settingsDiscardMessage = () => uiText(
  '设置中有未保存的修改，离开或重新读取将丢失这些草稿。确定继续吗？',
  'Settings has unsaved changes. Leaving or reloading will discard these drafts. Continue?'
)

const settingsReloadMessage = () => uiText(
  '此项设置有未保存的修改，重新读取会丢弃当前草稿。确定重新读取吗？',
  'This setting has unsaved changes. Reloading it will discard this draft. Continue?'
)

export function provideSettingsDrafts() {
  const drafts = shallowRef<Draft[]>([])
  provide(settingsDraftsKey, draft => {
    // Keep refs intact rather than deeply unwrapping them in a reactive collection.
    drafts.value = [...drafts.value, draft]
    onUnmounted(() => { drafts.value = drafts.value.filter(item => item !== draft) })
  })
  const dirty = computed(() => drafts.value.some(draft => draft.value))
  useUnsavedChanges(dirty, settingsDiscardMessage)
  return { dirty, hasOtherDrafts: (excluded: Draft) => drafts.value.some(draft => draft !== excluded && draft.value) }
}

export function useSettingsDraft(dirty: Draft) {
  const register = inject(settingsDraftsKey, null)
  if (!register) return useUnsavedChanges(dirty, settingsDiscardMessage)
  register(dirty)
  return {
    confirmDiscard: () => !dirty.value || window.confirm(settingsReloadMessage()),
  }
}
