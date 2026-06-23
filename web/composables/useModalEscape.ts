// Closes a modal on Escape. The listener is attached to window for the lifetime of
// the component and only fires when the modal is open (isOpen). This way Escape works
// regardless of where focus currently is (the overlay div never receives focus).
export function useModalEscape(isOpen: Ref<boolean>, close: () => void) {
  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape' && isOpen.value) close()
  }
  onMounted(() => window.addEventListener('keydown', onKey))
  onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
}
