// Manages the user's UI language: fetches from server on login, persists via PUT.
// Falls back to 'en' on any error or when not authenticated.

const SUPPORTED = ['en', 'de', 'uk', 'fr', 'es', 'ru', 'sr'] as const
type LangCode = (typeof SUPPORTED)[number]

function isSupported(code: string): code is LangCode {
  return (SUPPORTED as readonly string[]).includes(code)
}

export function useLocale() {
  const { setLocale } = useI18n()
  const sess = useSession()

  async function fetchAndApply() {
    if (!sess.value.token) return
    try {
      const res = await apiFetch<{ language: string }>('/me/language', {
        headers: { Authorization: `Bearer ${sess.value.token}` },
      })
      const code = res.language
      if (isSupported(code)) await setLocale(code)
    } catch {
      // not authed yet or network error — stay on default (en)
    }
  }

  async function saveLanguage(code: LangCode) {
    await apiFetch('/me/language', {
      method: 'PUT',
      body: { language: code },
      headers: { Authorization: `Bearer ${sess.value.token}` },
    })
    await setLocale(code)
  }

  return { fetchAndApply, saveLanguage, SUPPORTED }
}
