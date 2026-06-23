// Clipboard helper: tracks which button was last copied so its icon can flip
// to a checkmark for 1.5s. `tag` is a per-button id unique within one panel.
export function useCopy() {
  const copied = ref('')
  async function copyText(text: string, tag: string) {
    try {
      await navigator.clipboard.writeText(text)
      copied.value = tag
      setTimeout(() => { if (copied.value === tag) copied.value = '' }, 1500)
    } catch {
      /* clipboard not available — silently ignore */
    }
  }
  return { copied, copyText }
}
