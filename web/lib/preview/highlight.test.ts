import { describe, it, expect } from 'vitest'
import { highlightCode } from './highlight'
import { EXT_LANG } from './kind'

describe('highlightCode', () => {
  it('highlights known languages', async () => {
    const out = await highlightCode('package main\nfunc main() {}', 'go')
    expect(out).toContain('hljs-keyword')
  })

  it('escapes the source (safe for v-html)', async () => {
    const out = await highlightCode('const a = "<script>alert(1)</script>"', 'javascript')
    expect(out).not.toContain('<script>')
    expect(out).toContain('&lt;script&gt;')
  })

  it('returns null for an unknown language without throwing', async () => {
    expect(await highlightCode('whatever', 'no-such-lang')).toBeNull()
  })

  // INTEGRITY: every language kind.ts can hand out must have a grammar registered,
  // otherwise a source file silently loses highlighting.
  it('registers a grammar for every language in EXT_LANG', async () => {
    const langs = [...new Set(Object.values(EXT_LANG).filter((l): l is string => l !== null))]
    for (const lang of langs) {
      expect(await highlightCode('test', lang), `missing grammar: ${lang}`).not.toBeNull()
    }
  })
})
