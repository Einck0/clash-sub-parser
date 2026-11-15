import { shallowRef } from 'vue'

// Share open order across drawer instances; reopening a mounted drawer makes it topmost.
export const drawerStack = shallowRef<symbol[]>([])

export function openDrawer(id: symbol) {
  drawerStack.value = [...drawerStack.value.filter((entry) => entry !== id), id]
}

export function removeDrawer(id: symbol): boolean {
  const wasTopmost = drawerStack.value.at(-1) === id
  drawerStack.value = drawerStack.value.filter((entry) => entry !== id)
  return wasTopmost
}
