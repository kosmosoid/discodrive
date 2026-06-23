// Theme: 'dark' (default) | 'light'. The .light class is toggled on <html>;
// the preference is persisted in localStorage. The anti-FOUC script in nuxt.config.ts
// applies the class before rendering; init() here syncs the reactive state.
export type Theme = 'dark' | 'light'

export function useTheme() {
  const theme = useState<Theme>('theme', () => 'dark')

  function apply(t: Theme) {
    theme.value = t
    if (import.meta.client) {
      document.documentElement.classList.toggle('light', t === 'light')
      try {
        localStorage.setItem('theme', t)
      } catch {
        /* private mode / no access — ignore */
      }
    }
  }

  function init() {
    if (!import.meta.client) return
    let stored: string | null = null
    try {
      stored = localStorage.getItem('theme')
    } catch {
      /* no access */
    }
    apply(stored === 'light' ? 'light' : 'dark')
  }

  function toggle() {
    apply(theme.value === 'light' ? 'dark' : 'light')
  }

  return { theme, init, toggle, apply }
}
