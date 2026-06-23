// Hydrate the session from localStorage on SPA startup. Initial setup status
// (/setup/status) is resolved by the route guard (auth.global.ts) — so we don't
// depend on this plugin running before the initial navigation.
export default defineNuxtPlugin(() => {
  const sess = useSession()
  const raw = localStorage.getItem('kf_session')
  if (raw) {
    try {
      sess.value = JSON.parse(raw)
    } catch {
      localStorage.removeItem('kf_session')
    }
  }
})
