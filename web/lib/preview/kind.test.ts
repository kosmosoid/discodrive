import { describe, it, expect } from 'vitest'
import { previewPlan, decodeUtf8Strict, extOf, TEXT_PREVIEW_MAX } from './kind'

describe('previewPlan', () => {
  it('maps images with the right blob mime (SVG must be image/svg+xml for <img>)', () => {
    expect(previewPlan('photo.jpg', 100)).toEqual({ kind: 'image', mime: 'image/jpeg' })
    expect(previewPlan('logo.svg', 100)).toEqual({ kind: 'image', mime: 'image/svg+xml' })
    expect(previewPlan('anim.webp', 100)).toEqual({ kind: 'image', mime: 'image/webp' })
  })

  it('is case-insensitive on extensions', () => {
    expect(previewPlan('NOTES.MD', 100).kind).toBe('markdown')
    expect(previewPlan('Photo.Jpg', 100).kind).toBe('image')
  })

  it('routes markdown and pdf', () => {
    expect(previewPlan('note.md', 100).kind).toBe('markdown')
    expect(previewPlan('doc.pdf', 100).kind).toBe('pdf')
    // pdf has no text cap
    expect(previewPlan('big.pdf', TEXT_PREVIEW_MAX * 100).kind).toBe('pdf')
  })

  it('treats HTML as source code, never a rendered page', () => {
    const plan = previewPlan('page.html', 100)
    expect(plan.kind).toBe('code')
    expect(plan.lang).toBe('xml')
  })

  it('maps source extensions to hljs languages', () => {
    expect(previewPlan('main.go', 100)).toEqual({ kind: 'code', lang: 'go' })
    expect(previewPlan('app.tsx', 100)).toEqual({ kind: 'code', lang: 'typescript' })
    expect(previewPlan('run.sh', 100)).toEqual({ kind: 'code', lang: 'bash' })
  })

  it('treats plain-text extensions as code without a language', () => {
    expect(previewPlan('server.log', 100)).toEqual({ kind: 'code', lang: null })
    expect(previewPlan('.gitignore', 100)).toEqual({ kind: 'code', lang: null })
    expect(previewPlan('go.mod', 100)).toEqual({ kind: 'code', lang: null })
  })

  it('recognizes well-known extensionless files', () => {
    expect(previewPlan('Makefile', 100)).toEqual({ kind: 'code', lang: 'makefile' })
    expect(previewPlan('Dockerfile', 100)).toEqual({ kind: 'code', lang: 'dockerfile' })
    expect(previewPlan('LICENSE', 100)).toEqual({ kind: 'code', lang: null })
  })

  it('probes unknown files under the cap, refuses over it', () => {
    expect(previewPlan('mystery.bin', 100).kind).toBe('probe')
    expect(previewPlan('noext', 100).kind).toBe('probe')
    expect(previewPlan('mystery.bin', TEXT_PREVIEW_MAX + 1).kind).toBe('none')
  })

  it('enforces the cap boundary: exactly 2 MiB previews, one byte more does not', () => {
    expect(previewPlan('a.md', TEXT_PREVIEW_MAX).kind).toBe('markdown')
    expect(previewPlan('a.md', TEXT_PREVIEW_MAX + 1).kind).toBe('too_large')
    expect(previewPlan('a.go', TEXT_PREVIEW_MAX).kind).toBe('code')
    expect(previewPlan('a.go', TEXT_PREVIEW_MAX + 1).kind).toBe('too_large')
  })

  it('handles null size (unknown) as previewable', () => {
    expect(previewPlan('a.md', null).kind).toBe('markdown')
    expect(previewPlan('x.unknownext', null).kind).toBe('probe')
  })

  it('images have no size cap', () => {
    expect(previewPlan('huge.png', TEXT_PREVIEW_MAX * 50).kind).toBe('image')
  })
})

describe('extOf', () => {
  it('extracts the lowercased extension', () => {
    expect(extOf('a.TXT')).toBe('txt')
    expect(extOf('archive.tar.gz')).toBe('gz')
    expect(extOf('noext')).toBe('')
    expect(extOf('.gitignore')).toBe('gitignore')
  })
})

describe('decodeUtf8Strict', () => {
  it('decodes valid UTF-8 including cyrillic', () => {
    const bytes = new TextEncoder().encode('привет, world')
    expect(decodeUtf8Strict(bytes)).toBe('привет, world')
  })

  it('returns null for binary data', () => {
    expect(decodeUtf8Strict(new Uint8Array([0xff, 0xfe, 0x00, 0x81, 0xc0]))).toBeNull()
  })
})
