import { ref, onUnmounted, type Ref } from 'vue'

function startOfToday(): Date { const d = new Date(); d.setHours(0, 0, 0, 0); return d }

// Reactive "today" (local midnight). A plain new Date() in a template is not
// reactive, so a tab left open past midnight keeps highlighting yesterday.
// The date rolls over via a minute timer and when the tab regains visibility
// or focus (laptop waking from sleep).
export function useToday(): { today: Ref<Date>; refresh: () => void } {
  const today = ref(startOfToday())
  const refresh = () => {
    const now = startOfToday()
    if (now.getTime() !== today.value.getTime()) today.value = now
  }
  if (import.meta.client) {
    const timer = setInterval(refresh, 60_000)
    document.addEventListener('visibilitychange', refresh)
    window.addEventListener('focus', refresh)
    onUnmounted(() => {
      clearInterval(timer)
      document.removeEventListener('visibilitychange', refresh)
      window.removeEventListener('focus', refresh)
    })
  }
  return { today, refresh }
}
