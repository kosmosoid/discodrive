// Lazy docx renderer: mammoth converts the document to HTML, DOMPurify strips
// anything outside a fixed allowlist before the result may touch v-html.
// mammoth builds its HTML itself (it doesn't pass document markup through), but
// hyperlink hrefs come from the file — a javascript: URL in a docx must die
// here, not in the DOM. Images arrive as data: URIs (allowed by our CSP).

const ALLOWED_TAGS = [
  'p', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'br', 'hr',
  'ul', 'ol', 'li', 'table', 'thead', 'tbody', 'tfoot', 'tr', 'td', 'th', 'caption',
  'strong', 'em', 'b', 'i', 'u', 's', 'sup', 'sub', 'span',
  'blockquote', 'pre', 'code', 'a', 'img',
]
const ALLOWED_ATTR = ['href', 'src', 'alt', 'colspan', 'rowspan']

let purifyPromise: Promise<typeof import('dompurify').default> | null = null

async function getPurify() {
  const { default: DOMPurify } = await import('dompurify')
  DOMPurify.addHook('afterSanitizeAttributes', (node) => {
    if (node.tagName === 'A' && node.getAttribute('href')) {
      node.setAttribute('target', '_blank')
      node.setAttribute('rel', 'noopener')
    }
  })
  return DOMPurify
}

export async function sanitizeDocxHtml(html: string): Promise<string> {
  const DOMPurify = await (purifyPromise ??= getPurify())
  return DOMPurify.sanitize(html, { ALLOWED_TAGS, ALLOWED_ATTR })
}

export async function renderDocx(data: ArrayBuffer): Promise<string> {
  const m: any = await import('mammoth')
  const mammoth = m.default ?? m
  const { value } = await mammoth.convertToHtml({ arrayBuffer: data })
  return sanitizeDocxHtml(value)
}
