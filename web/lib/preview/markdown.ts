// Lazy markdown renderer for note previews and the saved-articles reader.
// Safety model: html:false — raw HTML in the source is escaped and shown as
// text, so the rendered output can go through v-html without a sanitizer.
// Obsidian-style [[wikilinks]] render as inert styled text. Images: notes
// render placeholders (the modal can't resolve attachments in v1), while the
// articles reader opts into real <img> tags via {allowImages: true} — article
// images are external links by design. ![[embeds]] stay placeholders always.
import type MarkdownIt from 'markdown-it'

export interface MarkdownOptions {
  allowImages?: boolean
}

export function configureMarkdown(md: MarkdownIt, opts: MarkdownOptions = {}): MarkdownIt {
  // [[wikilink]] and ![[embed]] — must run before the standard link rule.
  md.inline.ruler.before('link', 'wikilink', (state, silent) => {
    const src = state.src
    let pos = state.pos
    const isEmbed = src.charCodeAt(pos) === 0x21 /* ! */
    if (isEmbed) pos++
    if (src.charCodeAt(pos) !== 0x5b /* [ */ || src.charCodeAt(pos + 1) !== 0x5b) return false
    const end = src.indexOf(']]', pos + 2)
    if (end < 0) return false
    const inner = src.slice(pos + 2, end)
    if (!inner || inner.includes('\n') || inner.includes('[')) return false
    if (!silent) {
      const token = state.push(isEmbed ? 'wikiembed' : 'wikilink', '', 0)
      token.content = inner
    }
    state.pos = end + 2
    return true
  })
  md.renderer.rules.wikilink = (tokens, idx) =>
    `<span class="md-wikilink">${md.utils.escapeHtml(tokens[idx].content)}</span>`
  md.renderer.rules.wikiembed = (tokens, idx) => imgPlaceholder(md, tokens[idx].content)
  md.renderer.rules.image = (tokens, idx) => {
    const t = tokens[idx]
    if (opts.allowImages) {
      const src = t.attrGet('src') || ''
      const alt = t.content || ''
      return `<img src="${md.utils.escapeHtml(src)}" alt="${md.utils.escapeHtml(alt)}" loading="lazy" referrerpolicy="no-referrer">`
    }
    return imgPlaceholder(md, t.content || t.attrGet('src') || '')
  }

  const defaultLink =
    md.renderer.rules.link_open ??
    ((tokens, idx, options, _env, self) => self.renderToken(tokens, idx, options))
  md.renderer.rules.link_open = (tokens, idx, options, env, self) => {
    tokens[idx].attrSet('target', '_blank')
    tokens[idx].attrSet('rel', 'noopener')
    return defaultLink(tokens, idx, options, env, self)
  }
  return md
}

function imgPlaceholder(md: MarkdownIt, name: string): string {
  return `<span class="md-img-placeholder">${md.utils.escapeHtml(name)}</span>`
}

// One cached instance per image mode (the renderer rules differ).
const mdPromises: Record<string, Promise<MarkdownIt> | undefined> = {}

export async function renderMarkdown(src: string, opts: MarkdownOptions = {}): Promise<string> {
  const key = opts.allowImages ? 'img' : 'plain'
  // default preset: tables and strikethrough are already on; html stays OFF.
  mdPromises[key] ??= import('markdown-it').then(
    (m) => configureMarkdown(new m.default({ html: false, linkify: true }), opts),
  )
  return (await mdPromises[key]!).render(src)
}
