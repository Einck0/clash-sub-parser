/** Interactive elements should never start a reorder drag. */
const INTERACTIVE_SELECTOR = [
  'input',
  'textarea',
  'select',
  'option',
  'button',
  'a',
  'label',
  '[contenteditable="true"]',
  '[contenteditable=""]',
  '.no-drag',
].join(',')

export function isInteractiveDragTarget(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return false
  return Boolean(target.closest(INTERACTIVE_SELECTOR))
}

/** True only when the event started from an explicit drag handle. */
export function isDragHandleTarget(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return false
  return Boolean(target.closest('.drag-handle, .drag-mini, [data-drag-handle]'))
}

/**
 * Allow drag only from a handle (or non-interactive area when allowWholeRow is true).
 * Returns false when the drag should be cancelled.
 */
export function shouldAllowDragStart(
  event: DragEvent,
  options: { requireHandle?: boolean } = {},
): boolean {
  const target = event.target
  if (isInteractiveDragTarget(target) && !isDragHandleTarget(target)) {
    return false
  }
  if (options.requireHandle !== false && !isDragHandleTarget(target)) {
    return false
  }
  return true
}

export function setDragGhost(event: DragEvent, label = '排序') {
  if (!event.dataTransfer) return
  event.dataTransfer.effectAllowed = 'move'
  event.dataTransfer.setData('text/plain', label)
}
