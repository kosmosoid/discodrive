<script setup lang="ts">
// File preview modal with folder-gallery navigation (←/→). Renderer is picked by
// lib/preview/kind.ts; heavy renderers (markdown-it, highlight.js, pdf.js) are
// lazy chunks loaded on first use. One live object URL at a time — revoked on
// every navigation and on close.
import type { PreviewItem } from '~/lib/preview/types'
import { previewPlan, decodeUtf8Strict, TEXT_PREVIEW_MAX, type PreviewKind } from '~/lib/preview/kind'
import { renderMarkdown } from '~/lib/preview/markdown'
import { highlightCode } from '~/lib/preview/highlight'

const props = defineProps<{ items: PreviewItem[]; startIndex: number }>()
const emit = defineEmits<{ close: [] }>()

const { t } = useI18n()

const index = ref(Math.min(Math.max(props.startIndex, 0), props.items.length - 1))
const current = computed(() => props.items[index.value])
const hasPrev = computed(() => index.value > 0)
const hasNext = computed(() => index.value < props.items.length - 1)

const st = reactive({
  busy: false,
  error: false,
  kind: 'none' as PreviewKind,
  text: null as string | null, // plain text (no grammar / probe success)
  codeHtml: null as string | null, // highlighted source (v-html, escaped by hljs)
  mdHtml: null as string | null, // rendered markdown (v-html, html:false)
  imgUrl: null as string | null,
  pdfBlob: null as Blob | null,
  blob: null as Blob | null, // downloaded bytes, reused by the download button
})

let objectUrl: string | null = null
// Guards against out-of-order async results when flipping through the gallery
// faster than files load: only the latest navigation may touch the state.
let loadSeq = 0

function revoke() {
  if (objectUrl) {
    URL.revokeObjectURL(objectUrl)
    objectUrl = null
  }
}

async function show(i: number) {
  const item = props.items[i]
  const seq = ++loadSeq
  revoke()
  Object.assign(st, {
    busy: false, error: false, kind: 'none' as PreviewKind,
    text: null, codeHtml: null, mdHtml: null, imgUrl: null, pdfBlob: null, blob: null,
  })

  const plan = previewPlan(item.name, item.size)
  st.kind = plan.kind
  // both placeholders are decided from n.size — not a single byte is downloaded
  if (plan.kind === 'too_large' || plan.kind === 'none') return

  st.busy = true
  try {
    const blob = await item.load()
    if (seq !== loadSeq) return
    st.blob = blob

    switch (plan.kind) {
      case 'image':
        objectUrl = URL.createObjectURL(new Blob([blob], { type: plan.mime }))
        st.imgUrl = objectUrl
        break
      case 'pdf':
        st.pdfBlob = blob
        break
      case 'markdown': {
        const html = await renderMarkdown(await blob.text())
        if (seq === loadSeq) st.mdHtml = html
        break
      }
      case 'code': {
        const text = decodeUtf8Strict(await blob.arrayBuffer())
        if (seq !== loadSeq) return
        if (text === null) {
          st.kind = 'none' // binary bytes behind a text extension
          break
        }
        st.text = text
        if (plan.lang) {
          const html = await highlightCode(text, plan.lang)
          if (seq === loadSeq && html !== null) st.codeHtml = html
        }
        break
      }
      case 'probe': {
        const text = decodeUtf8Strict(await blob.arrayBuffer())
        if (seq !== loadSeq) return
        if (text === null) st.kind = 'none'
        else {
          st.kind = 'code'
          st.text = text
        }
        break
      }
    }
  } catch {
    if (seq === loadSeq) st.error = true
  } finally {
    if (seq === loadSeq) st.busy = false
  }
}

function go(delta: number) {
  const next = index.value + delta
  if (next < 0 || next >= props.items.length) return
  index.value = next
}
watch(index, (i) => show(i))
onMounted(() => show(index.value))
onBeforeUnmount(revoke)

function onKey(e: KeyboardEvent) {
  if (e.key === 'ArrowLeft') go(-1)
  else if (e.key === 'ArrowRight') go(1)
  else if (e.key === 'Escape') emit('close')
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))

const downloading = ref(false)
async function download() {
  if (downloading.value) return
  downloading.value = true
  try {
    const blob = st.blob ?? (await current.value.load())
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = current.value.name
    a.click()
    URL.revokeObjectURL(url)
  } catch {
    st.error = true
  } finally {
    downloading.value = false
  }
}
</script>

<template>
  <div
    class="fixed inset-0 z-40 flex items-center justify-center bg-black/60 p-2 sm:p-4"
    @click.self="emit('close')"
  >
    <!-- gallery navigation -->
    <button
      v-if="hasPrev"
      class="btn-ghost absolute left-1 top-1/2 z-10 -translate-y-1/2 rounded-full bg-panel/70 p-2 sm:left-3"
      :title="t('preview.prev')"
      @click="go(-1)"
    >
      <Icon name="lucide:chevron-left" size="22" />
    </button>
    <button
      v-if="hasNext"
      class="btn-ghost absolute right-1 top-1/2 z-10 -translate-y-1/2 rounded-full bg-panel/70 p-2 sm:right-3"
      :title="t('preview.next')"
      @click="go(1)"
    >
      <Icon name="lucide:chevron-right" size="22" />
    </button>

    <!-- PDF gets an explicit card height: its viewer fills the card via absolute
         positioning (percentage heights through auto-sized flex chains don't
         resolve in Firefox), so the card can't size from its content. -->
    <div
      class="card flex h-full w-full max-w-5xl flex-col"
      :class="st.kind === 'pdf' ? 'sm:h-[92vh]' : 'sm:h-auto sm:max-h-[92vh]'"
      style="min-height: 40vh"
    >
      <div class="flex items-center justify-between gap-3 border-b border-line px-4 py-3 sm:px-5">
        <div class="min-w-0">
          <div class="truncate text-sm font-medium" :title="current.name">{{ current.name }}</div>
          <div class="text-xs text-muted">
            {{ formatBytes(current.size) }}
            <span v-if="items.length > 1"> · {{ t('preview.counter', { i: index + 1, n: items.length }) }}</span>
          </div>
        </div>
        <div class="flex shrink-0 items-center gap-1">
          <button class="btn-accent px-2 py-1 text-xs" :disabled="downloading" @click="download">
            <Icon :name="downloading ? 'lucide:loader-circle' : 'lucide:download'" :class="downloading ? 'animate-spin' : ''" size="15" />
            {{ t('common.download') }}
          </button>
          <button class="btn-ghost px-1.5 py-1" :title="t('common.close')" @click="emit('close')">
            <Icon name="lucide:x" size="18" />
          </button>
        </div>
      </div>

      <div class="min-h-0 flex-1" :class="st.kind === 'pdf' && st.pdfBlob ? 'relative' : 'overflow-auto'">
        <div v-if="st.busy" class="py-14 text-center text-sm text-muted">
          <Icon name="lucide:loader-circle" class="mx-auto mb-2 block animate-spin" size="28" />
          {{ t('common.loading') }}
        </div>

        <p v-else-if="st.error" class="py-14 text-center text-sm text-danger">
          <Icon name="lucide:triangle-alert" size="15" class="mr-1 inline" />
          {{ t('preview.error') }}
        </p>

        <img
          v-else-if="st.imgUrl"
          :src="st.imgUrl"
          :alt="current.name"
          class="mx-auto max-h-full max-w-full object-contain p-3"
        />

        <PreviewPdf v-else-if="st.pdfBlob" :key="index" :blob="st.pdfBlob" class="absolute inset-0" />

        <!-- rendered markdown: html:false in the renderer, safe for v-html -->
        <div v-else-if="st.mdHtml" class="md-body p-4 sm:p-6" v-html="st.mdHtml" />

        <!-- highlighted source: hljs escapes, safe for v-html -->
        <pre
          v-else-if="st.codeHtml"
          class="preview-code p-4 font-mono text-xs leading-relaxed"
        ><code v-html="st.codeHtml" /></pre>

        <pre
          v-else-if="st.text !== null"
          class="whitespace-pre-wrap break-words p-4 font-mono text-xs leading-relaxed text-ink"
        >{{ st.text }}</pre>

        <div v-else-if="st.kind === 'too_large'" class="py-14 text-center">
          <Icon name="lucide:file-warning" size="32" class="mx-auto mb-3 block text-muted/60" />
          <p class="text-sm text-muted">{{ t('preview.too_large', { max: formatBytes(TEXT_PREVIEW_MAX) }) }}</p>
        </div>

        <div v-else class="py-14 text-center">
          <Icon name="lucide:file-question" size="32" class="mx-auto mb-3 block text-muted/60" />
          <p class="text-sm text-muted">{{ t('preview.no_preview') }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<style>
/* --- markdown typography (rendered notes) --- */
.md-body {
  font-size: 0.9rem;
  line-height: 1.65;
  color: rgb(var(--c-ink));
  overflow-wrap: break-word;
}
.md-body :is(h1, h2, h3, h4, h5, h6) {
  font-weight: 600;
  line-height: 1.3;
  margin: 1.4em 0 0.5em;
}
.md-body :is(h1, h2, h3, h4, h5, h6):first-child { margin-top: 0; }
.md-body h1 { font-size: 1.5rem; }
.md-body h2 { font-size: 1.25rem; }
.md-body h3 { font-size: 1.1rem; }
.md-body p { margin: 0.6em 0; }
.md-body a { color: rgb(var(--c-accent)); text-decoration: underline; text-underline-offset: 2px; }
.md-body :is(ul, ol) { margin: 0.6em 0; padding-left: 1.5em; }
.md-body ul { list-style: disc; }
.md-body ol { list-style: decimal; }
.md-body li { margin: 0.2em 0; }
.md-body blockquote {
  margin: 0.8em 0;
  padding: 0.1em 1em;
  border-left: 3px solid rgb(var(--c-accent) / 0.5);
  color: rgb(var(--c-muted));
}
.md-body code {
  font-family: ui-monospace, monospace;
  font-size: 0.85em;
  background: rgb(var(--c-panel2));
  border: 1px solid rgb(var(--c-line));
  border-radius: 4px;
  padding: 0.1em 0.35em;
}
.md-body pre {
  margin: 0.8em 0;
  padding: 0.8em 1em;
  background: rgb(var(--c-panel2));
  border: 1px solid rgb(var(--c-line));
  border-radius: 8px;
  overflow-x: auto;
}
.md-body pre code { background: none; border: none; padding: 0; font-size: 0.8rem; }
.md-body table { border-collapse: collapse; margin: 0.8em 0; width: auto; }
/* ink-based border, not --c-line: the line color has almost no contrast against
   the white card in the light theme, tables looked borderless there */
.md-body :is(th, td) { border: 1px solid rgb(var(--c-ink) / 0.2); padding: 0.35em 0.7em; }
.md-body th { background: rgb(var(--c-panel2)); font-weight: 600; }
.md-body hr { border: none; border-top: 1px solid rgb(var(--c-line)); margin: 1.2em 0; }
.md-body .md-wikilink { color: rgb(var(--c-accent)); opacity: 0.8; }
.md-body .md-wikilink::before { content: '[['; opacity: 0.5; }
.md-body .md-wikilink::after { content: ']]'; opacity: 0.5; }
.md-body .md-img-placeholder {
  display: inline-block;
  padding: 0.2em 0.6em;
  border: 1px dashed rgb(var(--c-line));
  border-radius: 6px;
  color: rgb(var(--c-muted));
  font-size: 0.85em;
}
.md-body .md-img-placeholder::before { content: '🖼 '; }

/* --- highlight.js theme on the app palette (light/dark via --c-* vars) --- */
.preview-code { color: rgb(var(--c-ink)); }
.preview-code .hljs-comment,
.preview-code .hljs-quote { color: rgb(var(--c-muted)); font-style: italic; }
.preview-code .hljs-keyword,
.preview-code .hljs-selector-tag,
.preview-code .hljs-tag,
.preview-code .hljs-name,
.preview-code .hljs-meta { color: rgb(var(--c-accent)); }
.preview-code .hljs-string,
.preview-code .hljs-regexp,
.preview-code .hljs-addition { color: rgb(var(--c-accent2)); }
.preview-code .hljs-number,
.preview-code .hljs-literal,
.preview-code .hljs-symbol,
.preview-code .hljs-bullet,
.preview-code .hljs-link { color: rgb(var(--c-accent) / 0.85); }
.preview-code .hljs-title,
.preview-code .hljs-section,
.preview-code .hljs-attr,
.preview-code .hljs-attribute,
.preview-code .hljs-selector-id,
.preview-code .hljs-selector-class { color: rgb(var(--c-ink)); font-weight: 600; }
.preview-code .hljs-type,
.preview-code .hljs-built_in,
.preview-code .hljs-builtin-name,
.preview-code .hljs-template-variable,
.preview-code .hljs-variable { color: rgb(var(--c-danger) / 0.8); }
.preview-code .hljs-deletion { color: rgb(var(--c-danger)); }
.preview-code .hljs-emphasis { font-style: italic; }
.preview-code .hljs-strong { font-weight: 700; }
</style>
