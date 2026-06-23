// Syncs the reactive theme state from localStorage on SPA startup.
// (The .light class itself is already applied by the anti-FOUC script in nuxt.config.ts.)
export default defineNuxtPlugin(() => {
  useTheme().init()
})
