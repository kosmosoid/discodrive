// DiscoDrive web UI — SPA, built to static files and embedded in the Go binary.
export default defineNuxtConfig({
  ssr: false,
  devtools: { enabled: false },
  modules: ['@nuxt/icon', '@nuxtjs/tailwindcss', '@nuxtjs/i18n'],
  i18n: {
    strategy: 'no_prefix',
    defaultLocale: 'en',
    locales: [
      { code: 'en', file: 'en.json' },
      { code: 'de', file: 'de.json' },
      { code: 'uk', file: 'uk.json' },
      { code: 'fr', file: 'fr.json' },
      { code: 'es', file: 'es.json' },
      { code: 'ru', file: 'ru.json' },
      { code: 'sr', file: 'sr.json' },
    ],
    langDir: 'locales/',
    detectBrowserLanguage: false,
  },
  css: ['~/assets/css/main.css'],
  app: {
    // UI is served under /app/; the root redirects here (Go side). The API lives at the root,
    // so page paths (/app/files) do not conflict with API paths (GET /files).
    baseURL: '/app/',
    head: {
      title: 'DiscoDrive',
      meta: [{ name: 'viewport', content: 'width=device-width, initial-scale=1' }],
      link: [
        { rel: 'icon', type: 'image/png', sizes: '32x32', href: '/app/favicon-32x32.png' },
        { rel: 'icon', type: 'image/png', sizes: '16x16', href: '/app/favicon-16x16.png' },
        { rel: 'apple-touch-icon', sizes: '180x180', href: '/app/apple-touch-icon.png' },
      ],
      // anti-FOUC: apply the stored theme before rendering
      script: [
        {
          innerHTML:
            "try{if(localStorage.getItem('theme')==='light')document.documentElement.classList.add('light')}catch(e){}",
          tagPosition: 'head',
        },
      ],
    },
  },
  icon: {
    // icons are bundled locally (offline), no requests to the Iconify API
    serverBundle: false,
    clientBundle: { scan: true },
  },
  // in dev, proxy API calls to the running backend (docker compose, nginx :8080)
  nitro: {
    devProxy: { '/health': 'http://localhost:8080' },
  },
})
