// Preview renderer selection: extension → kind, with a size cap for anything that
// gets parsed/highlighted on the client. Pure logic, no DOM — unit-tested.
//
// Security note (see internal/api/handlers.go setDownloadHeaders): user-uploaded
// HTML/SVG must NEVER be rendered as a document on our origin. SVG is shown only
// via <img> (scripts don't run there), HTML files are shown as highlighted source.

export type PreviewKind =
  | 'image' // blob URL in <img>
  | 'markdown' // markdown-it render
  | 'pdf' // pdf.js canvas + text layer
  | 'code' // <pre> + optional highlight.js language
  | 'probe' // unknown: download and strict-UTF-8 probe, text on success
  | 'too_large' // known-text over the cap: don't download, offer download instead
  | 'none' // no preview: placeholder with a download button

export interface PreviewPlan {
  kind: PreviewKind
  // hljs language id for 'code' (null = plain text, no highlighting)
  lang?: string | null
  // blob mime override for 'image' (SVG needs image/svg+xml to render in <img>)
  mime?: string
}

// Parsing/highlighting multi-megabyte files hangs the tab; beyond the cap the
// modal shows a "too large" placeholder without downloading a single byte.
export const TEXT_PREVIEW_MAX = 2 * 1024 * 1024

export function extOf(name: string): string {
  const i = name.lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1).toLowerCase() : ''
}

const IMAGE_MIME: Record<string, string> = {
  png: 'image/png',
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  gif: 'image/gif',
  webp: 'image/webp',
  svg: 'image/svg+xml',
  bmp: 'image/bmp',
  ico: 'image/x-icon',
  avif: 'image/avif',
}

// Extension → highlight.js language id; null = readable text without a grammar.
// Keys double as the "this is text" allowlist, so keep plain formats here too.
// Exported so tests can assert every language here is registered in highlight.ts.
export const EXT_LANG: Record<string, string | null> = {
  // plain text / data
  txt: null, log: null, csv: null, tsv: null, env: null,
  gitignore: null, gitattributes: null, editorconfig: null, lock: null,
  mod: null, sum: null, srt: null, vtt: null, m3u: null, m3u8: null, cue: null, nfo: null,
  // config
  json: 'json', jsonc: 'json', yaml: 'yaml', yml: 'yaml',
  toml: 'ini', ini: 'ini', conf: 'ini', properties: 'ini',
  xml: 'xml', html: 'xml', htm: 'xml', xhtml: 'xml', svelte: 'xml', vue: 'xml',
  // web
  css: 'css', scss: 'scss', less: 'less',
  js: 'javascript', mjs: 'javascript', cjs: 'javascript', jsx: 'javascript',
  ts: 'typescript', mts: 'typescript', cts: 'typescript', tsx: 'typescript',
  // languages
  go: 'go', rs: 'rust', py: 'python', rb: 'ruby', php: 'php',
  java: 'java', kt: 'kotlin', kts: 'kotlin', swift: 'swift',
  c: 'c', h: 'c', cpp: 'cpp', cc: 'cpp', cxx: 'cpp', hpp: 'cpp', hh: 'cpp',
  cs: 'csharp', sh: 'bash', bash: 'bash', zsh: 'bash', fish: 'bash',
  sql: 'sql', lua: 'lua', dart: 'dart', scala: 'scala', pl: 'perl', r: 'r',
  ps1: 'powershell', bat: 'dos', cmd: 'dos',
  dockerfile: 'dockerfile', makefile: 'makefile', mk: 'makefile',
  diff: 'diff', patch: 'diff', graphql: 'graphql', proto: 'protobuf', tf: 'ini',
}

// Well-known extensionless files (lowercased basename).
const BASENAME_LANG: Record<string, string | null> = {
  makefile: 'makefile',
  dockerfile: 'dockerfile',
  license: null,
  readme: null,
  changelog: null,
  authors: null,
  notice: null,
  version: null,
}

export function previewPlan(name: string, size: number | null): PreviewPlan {
  const ext = extOf(name)
  const overCap = size != null && size > TEXT_PREVIEW_MAX

  const mime = IMAGE_MIME[ext]
  if (mime) return { kind: 'image', mime }
  if (ext === 'pdf') return { kind: 'pdf' }
  if (ext === 'md' || ext === 'markdown') {
    return overCap ? { kind: 'too_large' } : { kind: 'markdown' }
  }

  const lang = ext ? EXT_LANG[ext] : BASENAME_LANG[name.toLowerCase()]
  if (lang !== undefined) {
    return overCap ? { kind: 'too_large' } : { kind: 'code', lang }
  }

  // Unknown extension: worth a strict-UTF-8 probe (Makefile-style files with odd
  // names, logs without extension), but only under the cap — probing means downloading.
  return overCap ? { kind: 'none' } : { kind: 'probe' }
}

// Strict UTF-8 decode: returns the text, or null for binary data. This is what
// turns a 'probe' into readable text or a "no preview" placeholder.
export function decodeUtf8Strict(bytes: ArrayBuffer | Uint8Array): string | null {
  try {
    return new TextDecoder('utf-8', { fatal: true }).decode(bytes)
  } catch {
    return null
  }
}
