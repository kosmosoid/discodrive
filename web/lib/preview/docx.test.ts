// @vitest-environment jsdom
import { describe, it, expect } from 'vitest'
import { sanitizeDocxHtml } from './docx'

describe('sanitizeDocxHtml', () => {
  it('strips script and event handlers', async () => {
    const out = await sanitizeDocxHtml('<p onclick="alert(1)">hi</p><script>alert(1)</script>')
    expect(out).not.toContain('script')
    expect(out).not.toContain('onclick')
    expect(out).toContain('<p>hi</p>')
  })

  it('kills javascript: hyperlinks from the document', async () => {
    const out = await sanitizeDocxHtml('<a href="javascript:alert(1)">click</a>')
    expect(out).not.toContain('javascript:')
  })

  it('forces target=_blank rel=noopener on kept links', async () => {
    const out = await sanitizeDocxHtml('<a href="https://example.com">x</a>')
    expect(out).toContain('target="_blank"')
    expect(out).toContain('rel="noopener"')
  })

  it('keeps data: images (mammoth inlines pictures that way)', async () => {
    const out = await sanitizeDocxHtml('<img src="data:image/png;base64,AAAA" alt="pic">')
    expect(out).toContain('<img')
    expect(out).toContain('data:image/png')
  })

  it('drops tags outside the allowlist but keeps their text', async () => {
    const out = await sanitizeDocxHtml('<style>p{color:red}</style><iframe src="x"></iframe><p>ok</p>')
    expect(out).not.toContain('<style>')
    expect(out).not.toContain('<iframe')
    expect(out).toContain('<p>ok</p>')
  })

  it('keeps tables with col/rowspan', async () => {
    const out = await sanitizeDocxHtml('<table><tr><td colspan="2">a</td></tr></table>')
    expect(out).toContain('colspan="2"')
  })
})
