// Lazy highlight.js: core + a fixed set of grammars, loaded as a separate chunk
// on the first source-file preview. Language is picked strictly by extension
// (kind.ts EXT_LANG) — no auto-detection, it's slow and wrong too often.
import type { HLJSApi } from 'highlight.js'

// Every distinct language id used in kind.ts EXT_LANG must be registered here
// (guarded by highlight.test.ts).
const GRAMMARS: Record<string, () => Promise<{ default: any }>> = {
  json: () => import('highlight.js/lib/languages/json'),
  yaml: () => import('highlight.js/lib/languages/yaml'),
  ini: () => import('highlight.js/lib/languages/ini'),
  xml: () => import('highlight.js/lib/languages/xml'),
  css: () => import('highlight.js/lib/languages/css'),
  scss: () => import('highlight.js/lib/languages/scss'),
  less: () => import('highlight.js/lib/languages/less'),
  javascript: () => import('highlight.js/lib/languages/javascript'),
  typescript: () => import('highlight.js/lib/languages/typescript'),
  go: () => import('highlight.js/lib/languages/go'),
  rust: () => import('highlight.js/lib/languages/rust'),
  python: () => import('highlight.js/lib/languages/python'),
  ruby: () => import('highlight.js/lib/languages/ruby'),
  php: () => import('highlight.js/lib/languages/php'),
  java: () => import('highlight.js/lib/languages/java'),
  kotlin: () => import('highlight.js/lib/languages/kotlin'),
  swift: () => import('highlight.js/lib/languages/swift'),
  c: () => import('highlight.js/lib/languages/c'),
  cpp: () => import('highlight.js/lib/languages/cpp'),
  csharp: () => import('highlight.js/lib/languages/csharp'),
  bash: () => import('highlight.js/lib/languages/bash'),
  sql: () => import('highlight.js/lib/languages/sql'),
  lua: () => import('highlight.js/lib/languages/lua'),
  dart: () => import('highlight.js/lib/languages/dart'),
  scala: () => import('highlight.js/lib/languages/scala'),
  perl: () => import('highlight.js/lib/languages/perl'),
  r: () => import('highlight.js/lib/languages/r'),
  powershell: () => import('highlight.js/lib/languages/powershell'),
  dos: () => import('highlight.js/lib/languages/dos'),
  dockerfile: () => import('highlight.js/lib/languages/dockerfile'),
  makefile: () => import('highlight.js/lib/languages/makefile'),
  diff: () => import('highlight.js/lib/languages/diff'),
  graphql: () => import('highlight.js/lib/languages/graphql'),
  protobuf: () => import('highlight.js/lib/languages/protobuf'),
}

let hljsPromise: Promise<HLJSApi> | null = null

async function loadHljs(): Promise<HLJSApi> {
  const { default: hljs } = await import('highlight.js/lib/core')
  await Promise.all(
    Object.entries(GRAMMARS).map(async ([name, load]) => {
      hljs.registerLanguage(name, (await load()).default)
    }),
  )
  return hljs
}

// Returns highlighted HTML (hljs escapes the source itself — safe for v-html),
// or null when the language is unknown/fails — caller falls back to plain <pre>.
export async function highlightCode(code: string, lang: string): Promise<string | null> {
  hljsPromise ??= loadHljs()
  const hljs = await hljsPromise
  if (!hljs.getLanguage(lang)) return null
  try {
    return hljs.highlight(code, { language: lang }).value
  } catch {
    return null
  }
}
