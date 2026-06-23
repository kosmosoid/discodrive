<script setup lang="ts">
interface MetaNode { id: string; name: string; isDir: boolean }

interface BookMeta {
  title: string
  authors: string[]
  tags: string[]
  series: string
  seriesIndex: number
  language: string
  description: string
  publisher: string
  date: string
  edited: boolean
}

const props = defineProps<{ node: MetaNode }>()

const { t } = useI18n()
const { request } = useApi()
const { confirm } = useDialog()

// --- State ---
const meta = ref<BookMeta | null>(null)
const busy = ref(false)
const saveBusy = ref(false)
const error = ref('')
const savedMsg = ref(false)

// Editable fields
const form = reactive({
  title: '',
  series: '',
  seriesIndex: '',
  language: '',
  description: '',
  publisher: '',
  date: '',
})

// Array fields — chip editors
const authors = ref<string[]>([])
const tags = ref<string[]>([])

// New chip input state
const newAuthor = ref('')
const newTag = ref('')

function addAuthor() {
  const v = newAuthor.value.trim()
  if (v && !authors.value.includes(v)) authors.value.push(v)
  newAuthor.value = ''
}

function removeAuthor(i: number) {
  authors.value.splice(i, 1)
}

function addTag() {
  const v = newTag.value.trim()
  if (v && !tags.value.includes(v)) tags.value.push(v)
  newTag.value = ''
}

function removeTag(i: number) {
  tags.value.splice(i, 1)
}

// --- Bulk folder state ---
const BULK_FIELDS = ['authors', 'tags', 'series', 'language', 'publisher', 'date'] as const
type BulkField = typeof BULK_FIELDS[number]

const bulkApply = reactive<Record<BulkField, boolean>>({
  authors: false,
  tags: false,
  series: false,
  language: false,
  publisher: false,
  date: false,
})

// Chip arrays for bulk authors/tags
const bulkAuthors = ref<string[]>([])
const bulkTags = ref<string[]>([])
const bulkNewAuthor = ref('')
const bulkNewTag = ref('')

function addBulkAuthor() {
  const v = bulkNewAuthor.value.trim()
  if (v && !bulkAuthors.value.includes(v)) bulkAuthors.value.push(v)
  bulkNewAuthor.value = ''
}
function removeBulkAuthor(i: number) { bulkAuthors.value.splice(i, 1) }

function addBulkTag() {
  const v = bulkNewTag.value.trim()
  if (v && !bulkTags.value.includes(v)) bulkTags.value.push(v)
  bulkNewTag.value = ''
}
function removeBulkTag(i: number) { bulkTags.value.splice(i, 1) }

// Scalar bulk fields
const bulkForm = reactive({ series: '', language: '', publisher: '', date: '' })

interface BulkResult {
  affected: number
  updated: number
  failed: { path: string; error: string }[]
}
const bulkResult = ref<BulkResult | null>(null)
const bulkError = ref('')

function resetBulkState() {
  for (const k of BULK_FIELDS) bulkApply[k] = false
  bulkAuthors.value = []
  bulkTags.value = []
  bulkNewAuthor.value = ''
  bulkNewTag.value = ''
  bulkForm.series = ''
  bulkForm.language = ''
  bulkForm.publisher = ''
  bulkForm.date = ''
  bulkResult.value = null
  bulkError.value = ''
}

function buildBulkPayload(): Record<string, unknown> {
  const body: Record<string, unknown> = {}
  if (bulkApply.authors && bulkAuthors.value.length > 0) body.authors = bulkAuthors.value
  if (bulkApply.tags && bulkTags.value.length > 0) body.tags = bulkTags.value
  if (bulkApply.series) body.series = bulkForm.series
  if (bulkApply.language) body.language = bulkForm.language
  if (bulkApply.publisher) body.publisher = bulkForm.publisher
  if (bulkApply.date) body.date = bulkForm.date
  return body
}

async function saveFolder() {
  bulkError.value = ''
  bulkResult.value = null
  saveBusy.value = true
  // Commit any author/tag text still in the chip input before building the payload.
  if (bulkNewAuthor.value.trim()) addBulkAuthor()
  if (bulkNewTag.value.trim()) addBulkTag()
  try {
    const { affected } = await request<{ affected: number }>(
      `/me/ebooks/bulk/${props.node.id}/count`, { method: 'POST' },
    )
    const ok = await confirm(
      t('bookmeta.confirm_bulk', { count: affected, name: props.node.name }),
      { danger: true },
    )
    if (!ok) return
    const res = await request<BulkResult>(
      `/me/ebooks/bulk/${props.node.id}`,
      { method: 'POST', body: buildBulkPayload() },
    )
    bulkResult.value = res
  } catch (e: any) {
    bulkError.value = e?.data?.error || t('bookmeta.error_save')
  } finally {
    saveBusy.value = false
  }
}

// --- Load metadata on node change ---
async function loadMeta(node: MetaNode) {
  if (node.isDir) {
    meta.value = null
    resetBulkState()
    return
  }
  error.value = ''
  busy.value = true
  meta.value = null
  try {
    const data = await request<BookMeta>(`/me/ebooks/meta/${node.id}`)
    meta.value = data
    form.title = data.title ?? ''
    form.series = data.series ?? ''
    // Coerce: <input type="number"> hands v-model a number; .trim() would throw.
    form.seriesIndex = String(data.seriesIndex ?? '')
    form.language = data.language ?? ''
    form.description = data.description ?? ''
    form.publisher = data.publisher ?? ''
    form.date = data.date ?? ''
    authors.value = [...(data.authors ?? [])]
    tags.value = [...(data.tags ?? [])]
  } catch (e: any) {
    error.value = e?.data?.error || t('bookmeta.error_load')
  } finally {
    busy.value = false
  }
}

watch(() => props.node, (n) => { loadMeta(n) }, { immediate: true })

// --- Reset to file metadata ---
async function resetMeta() {
  saveBusy.value = true
  error.value = ''
  try {
    await request(`/me/ebooks/meta/${props.node.id}/reset`, { method: 'POST' })
    await loadMeta(props.node)
  } catch (e: any) {
    error.value = e?.data?.error || t('bookmeta.error_save')
  } finally {
    saveBusy.value = false
  }
}

// --- Save ---
async function save() {
  const ok = await confirm(t('bookmeta.confirm_save', { title: form.title || props.node.name }))
  if (!ok) return
  saveBusy.value = true
  error.value = ''
  try {
    // Commit any author/tag text still sitting in the chip input (not yet added via
    // Enter/+) so it isn't silently dropped on save.
    if (newAuthor.value.trim()) addAuthor()
    if (newTag.value.trim()) addTag()
    // Coerce seriesIndex: String() avoids .trim() TypeError on numbers from <input type="number">
    const trimmed = String(form.seriesIndex ?? '').trim()
    const seriesIndex = trimmed === '' ? 0 : parseFloat(trimmed)

    await request(`/me/ebooks/meta/${props.node.id}`, {
      method: 'PUT',
      body: {
        title: form.title,
        authors: authors.value,
        tags: tags.value,
        series: form.series,
        seriesIndex,
        language: form.language,
        description: form.description,
        publisher: form.publisher,
        date: form.date,
      },
    })
    savedMsg.value = true
    setTimeout(() => { savedMsg.value = false }, 2500)
    // Reload to sync edited badge and server-side state
    await loadMeta(props.node)
  } catch (e: any) {
    error.value = e?.data?.error || t('bookmeta.error_save')
  } finally {
    saveBusy.value = false
  }
}
</script>

<template>
  <!-- Bulk folder editor -->
  <div v-if="node.isDir" class="max-w-xl">
    <h2 class="mb-1 text-base font-semibold text-ink">{{ node.name }}</h2>
    <p class="mb-4 text-xs text-muted">{{ t('bookmeta.bulk_hint') }}</p>

    <!-- Error -->
    <p v-if="bulkError" class="mb-4 flex items-center gap-2 text-sm text-danger">
      <Icon name="lucide:triangle-alert" size="16" /> {{ bulkError }}
    </p>

    <!-- Result summary -->
    <div v-if="bulkResult" class="mb-4 rounded-lg border border-line bg-panel2 p-3 text-sm">
      <p class="font-medium text-ink">
        {{ t('bookmeta.bulk_done', { updated: bulkResult.updated, affected: bulkResult.affected }) }}
      </p>
      <template v-if="bulkResult.failed.length">
        <p class="mt-2 text-xs font-medium text-muted">{{ t('bookmeta.bulk_failed') }}</p>
        <ul class="mt-1 space-y-0.5">
          <li v-for="f in bulkResult.failed" :key="f.path" class="text-xs text-danger">
            {{ f.path }} — {{ f.error }}
          </li>
        </ul>
      </template>
    </div>

    <!-- Bulk fields with "apply" checkboxes -->
    <div class="grid gap-3">
      <!-- Authors chip editor -->
      <div>
        <label class="mb-1 flex items-center gap-1.5 text-xs text-muted">
          <input v-model="bulkApply.authors" type="checkbox" class="accent-accent" />
          {{ t('bookmeta.authors') }}
          <span class="text-muted/60">{{ t('bookmeta.apply') }}</span>
        </label>
        <div v-if="bulkApply.authors">
          <div class="mb-1 flex flex-wrap gap-1">
            <span
              v-for="(a, i) in bulkAuthors"
              :key="a"
              class="flex items-center gap-1 rounded bg-accent/10 px-2 py-0.5 text-xs text-accent"
            >
              {{ a }}
              <button class="hover:text-danger" @click="removeBulkAuthor(i)">×</button>
            </span>
          </div>
          <div class="flex gap-2">
            <input
              v-model="bulkNewAuthor"
              class="input min-w-0 flex-1 text-sm"
              :placeholder="t('bookmeta.add')"
              @keydown.enter.prevent="addBulkAuthor"
            />
            <button class="btn-ghost px-2 py-1 text-xs" @click="addBulkAuthor">
              <Icon name="lucide:plus" size="14" />
            </button>
          </div>
        </div>
        <div v-else class="input w-full bg-panel2 opacity-50" />
      </div>

      <!-- Tags chip editor -->
      <div>
        <label class="mb-1 flex items-center gap-1.5 text-xs text-muted">
          <input v-model="bulkApply.tags" type="checkbox" class="accent-accent" />
          {{ t('bookmeta.tags') }}
          <span class="text-muted/60">{{ t('bookmeta.apply') }}</span>
        </label>
        <div v-if="bulkApply.tags">
          <div class="mb-1 flex flex-wrap gap-1">
            <span
              v-for="(tag, i) in bulkTags"
              :key="tag"
              class="flex items-center gap-1 rounded bg-ink/10 px-2 py-0.5 text-xs text-ink"
            >
              {{ tag }}
              <button class="hover:text-danger" @click="removeBulkTag(i)">×</button>
            </span>
          </div>
          <div class="flex gap-2">
            <input
              v-model="bulkNewTag"
              class="input min-w-0 flex-1 text-sm"
              :placeholder="t('bookmeta.add')"
              @keydown.enter.prevent="addBulkTag"
            />
            <button class="btn-ghost px-2 py-1 text-xs" @click="addBulkTag">
              <Icon name="lucide:plus" size="14" />
            </button>
          </div>
        </div>
        <div v-else class="input w-full bg-panel2 opacity-50" />
      </div>

      <!-- Series -->
      <div>
        <label class="mb-1 flex items-center gap-1.5 text-xs text-muted">
          <input v-model="bulkApply.series" type="checkbox" class="accent-accent" />
          {{ t('bookmeta.series') }}
          <span class="text-muted/60">{{ t('bookmeta.apply') }}</span>
        </label>
        <input v-model="bulkForm.series" class="input w-full" :disabled="!bulkApply.series" />
      </div>

      <!-- Language + date -->
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="mb-1 flex items-center gap-1.5 text-xs text-muted">
            <input v-model="bulkApply.language" type="checkbox" class="accent-accent" />
            {{ t('bookmeta.language') }}
            <span class="text-muted/60">{{ t('bookmeta.apply') }}</span>
          </label>
          <input v-model="bulkForm.language" class="input w-full" :disabled="!bulkApply.language" />
        </div>
        <div>
          <label class="mb-1 flex items-center gap-1.5 text-xs text-muted">
            <input v-model="bulkApply.date" type="checkbox" class="accent-accent" />
            {{ t('bookmeta.date') }}
            <span class="text-muted/60">{{ t('bookmeta.apply') }}</span>
          </label>
          <input v-model="bulkForm.date" class="input w-full" placeholder="YYYY-MM-DD" :disabled="!bulkApply.date" />
        </div>
      </div>

      <!-- Publisher -->
      <div>
        <label class="mb-1 flex items-center gap-1.5 text-xs text-muted">
          <input v-model="bulkApply.publisher" type="checkbox" class="accent-accent" />
          {{ t('bookmeta.publisher') }}
          <span class="text-muted/60">{{ t('bookmeta.apply') }}</span>
        </label>
        <input v-model="bulkForm.publisher" class="input w-full" :disabled="!bulkApply.publisher" />
      </div>
    </div>

    <!-- Save button -->
    <div class="mt-4">
      <button class="btn-accent" :disabled="saveBusy" @click="saveFolder">
        <Icon v-if="saveBusy" name="lucide:loader-circle" class="animate-spin" size="16" />
        <Icon v-else name="lucide:save" size="16" />
        {{ t('bookmeta.save') }}
      </button>
    </div>
  </div>

  <!-- File metadata editor -->
  <div v-else class="max-w-xl">
    <h2 class="mb-4 text-base font-semibold text-ink">{{ node.name }}</h2>

    <div v-if="busy" class="text-sm text-muted">{{ t('common.loading') }}</div>

    <template v-else-if="meta">
      <!-- Edited badge + reset -->
      <div
        v-if="meta.edited"
        class="mb-4 flex items-center gap-3 rounded-lg border border-line bg-panel2 px-3 py-2"
      >
        <span class="flex items-center gap-1.5 text-sm text-accent">
          <Icon name="lucide:pencil" size="14" class="shrink-0" />
          {{ t('bookmeta.edited_badge') }}
        </span>
        <button
          class="btn-ghost ml-auto px-2 py-1 text-xs"
          :disabled="saveBusy"
          @click="resetMeta"
        >
          <Icon name="lucide:rotate-ccw" size="13" class="mr-1" />
          {{ t('bookmeta.reset') }}
        </button>
      </div>

      <!-- Error -->
      <p v-if="error" class="mb-4 flex items-center gap-2 text-sm text-danger">
        <Icon name="lucide:triangle-alert" size="16" /> {{ error }}
      </p>

      <!-- Success -->
      <p v-if="savedMsg" class="mb-4 flex items-center gap-2 text-sm text-accent">
        <Icon name="lucide:check" size="16" /> {{ t('bookmeta.saved') }}
      </p>

      <!-- Form fields -->
      <div class="grid gap-3">
        <!-- Title -->
        <div>
          <label class="mb-1 block text-xs text-muted">{{ t('bookmeta.title') }}</label>
          <input v-model="form.title" class="input w-full" />
        </div>

        <!-- Authors chip editor -->
        <div>
          <label class="mb-1 block text-xs text-muted">{{ t('bookmeta.authors') }}</label>
          <div class="mb-1 flex flex-wrap gap-1">
            <span
              v-for="(a, i) in authors"
              :key="a"
              class="flex items-center gap-1 rounded bg-accent/10 px-2 py-0.5 text-xs text-accent"
            >
              {{ a }}
              <button class="hover:text-danger" @click="removeAuthor(i)">×</button>
            </span>
          </div>
          <div class="flex gap-2">
            <input
              v-model="newAuthor"
              class="input min-w-0 flex-1 text-sm"
              :placeholder="t('bookmeta.add')"
              @keydown.enter.prevent="addAuthor"
            />
            <button class="btn-ghost px-2 py-1 text-xs" @click="addAuthor">
              <Icon name="lucide:plus" size="14" />
            </button>
          </div>
        </div>

        <!-- Tags chip editor -->
        <div>
          <label class="mb-1 block text-xs text-muted">{{ t('bookmeta.tags') }}</label>
          <div class="mb-1 flex flex-wrap gap-1">
            <span
              v-for="(tag, i) in tags"
              :key="tag"
              class="flex items-center gap-1 rounded bg-ink/10 px-2 py-0.5 text-xs text-ink"
            >
              {{ tag }}
              <button class="hover:text-danger" @click="removeTag(i)">×</button>
            </span>
          </div>
          <div class="flex gap-2">
            <input
              v-model="newTag"
              class="input min-w-0 flex-1 text-sm"
              :placeholder="t('bookmeta.add')"
              @keydown.enter.prevent="addTag"
            />
            <button class="btn-ghost px-2 py-1 text-xs" @click="addTag">
              <Icon name="lucide:plus" size="14" />
            </button>
          </div>
        </div>

        <!-- Series + seriesIndex -->
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="mb-1 block text-xs text-muted">{{ t('bookmeta.series') }}</label>
            <input v-model="form.series" class="input w-full" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-muted">{{ t('bookmeta.series_index') }}</label>
            <input v-model="form.seriesIndex" type="number" min="0" step="0.1" class="input w-full" />
          </div>
        </div>

        <!-- Language + date -->
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="mb-1 block text-xs text-muted">{{ t('bookmeta.language') }}</label>
            <input v-model="form.language" class="input w-full" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-muted">{{ t('bookmeta.date') }}</label>
            <input v-model="form.date" class="input w-full" placeholder="YYYY-MM-DD" />
          </div>
        </div>

        <!-- Publisher -->
        <div>
          <label class="mb-1 block text-xs text-muted">{{ t('bookmeta.publisher') }}</label>
          <input v-model="form.publisher" class="input w-full" />
        </div>

        <!-- Description -->
        <div>
          <label class="mb-1 block text-xs text-muted">{{ t('bookmeta.description') }}</label>
          <textarea v-model="form.description" class="input w-full" rows="4" />
        </div>
      </div>

      <!-- Save button -->
      <div class="mt-4">
        <button class="btn-accent" :disabled="saveBusy" @click="save">
          <Icon v-if="saveBusy" name="lucide:loader-circle" class="animate-spin" size="16" />
          <Icon v-else name="lucide:save" size="16" />
          {{ t('bookmeta.save') }}
        </button>
      </div>
    </template>

    <p v-else-if="error" class="text-sm text-danger">{{ error }}</p>
  </div>
</template>
