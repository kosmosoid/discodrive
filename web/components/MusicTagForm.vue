<script setup lang="ts">
interface TagNode { id: string; name: string; isDir: boolean }

interface TagsDTO {
  title: string | null
  artist: string | null
  album: string | null
  albumArtist: string | null
  genre: string | null
  year: number | null
  track: number | null
  disc: number | null
}

interface TagInfo {
  tags: TagsDTO
  hasCover: boolean
  writable: boolean
  suffix: string
}

const props = defineProps<{ node: TagNode }>()

const { t } = useI18n()
const { request } = useApi()
const { confirm } = useDialog()

// --- State ---
const info = ref<TagInfo | null>(null)
const busy = ref(false)
const saveBusy = ref(false)
const error = ref('')
const savedMsg = ref(false)

// Editable fields mirror the loaded tags
const form = reactive({
  title: '',
  artist: '',
  album: '',
  albumArtist: '',
  genre: '',
  year: '',
  track: '',
  disc: '',
})

// Cover state (shared between single-file and bulk modes)
const coverUrl = ref<string | null>(null) // object URL for preview
let coverRevoke: (() => void) | null = null

// Pending cover change: 'keep' | 'remove' | { data: string; mime: string }
type CoverChange = 'keep' | 'remove' | { data: string; mime: string }
const pendingCover = ref<CoverChange>('keep')

function revokeCoverUrl() {
  if (coverRevoke) { coverRevoke(); coverRevoke = null }
  coverUrl.value = null
}

const coverInput = ref<HTMLInputElement>()

// --- Bulk folder state ---
// Per-field "apply" checkboxes for bulk mode (title/track excluded)
const BULK_FIELDS = ['artist', 'album', 'albumArtist', 'genre', 'year', 'disc'] as const
type BulkField = typeof BULK_FIELDS[number]

const bulkApply = reactive<Record<BulkField, boolean>>({
  artist: false,
  album: false,
  albumArtist: false,
  genre: false,
  year: false,
  disc: false,
})

const bulkForm = reactive<Record<BulkField, string>>({
  artist: '',
  album: '',
  albumArtist: '',
  genre: '',
  year: '',
  disc: '',
})

// Bulk result after save
interface BulkResult {
  affected: number
  updated: number
  failed: { path: string; error: string }[]
}
const bulkResult = ref<BulkResult | null>(null)
const bulkError = ref('')

function resetBulkState() {
  for (const k of BULK_FIELDS) { bulkApply[k] = false; bulkForm[k] = '' }
  pendingCover.value = 'keep'
  revokeCoverUrl()
  bulkResult.value = null
  bulkError.value = ''
}

// --- Load tags on node change ---
async function loadTags(node: TagNode) {
  if (node.isDir) {
    info.value = null
    revokeCoverUrl()
    resetBulkState()
    return
  }
  error.value = ''
  busy.value = true
  info.value = null
  revokeCoverUrl()
  pendingCover.value = 'keep'
  try {
    const data = await request<TagInfo>(`/me/music/tags/${node.id}`)
    info.value = data
    // Populate form
    form.title = data.tags.title ?? ''
    form.artist = data.tags.artist ?? ''
    form.album = data.tags.album ?? ''
    form.albumArtist = data.tags.albumArtist ?? ''
    form.genre = data.tags.genre ?? ''
    form.year = data.tags.year != null ? String(data.tags.year) : ''
    form.track = data.tags.track != null ? String(data.tags.track) : ''
    form.disc = data.tags.disc != null ? String(data.tags.disc) : ''
    // Load cover blob if present
    if (data.hasCover) {
      try {
        const blob = await request<Blob>(`/me/music/tags/${node.id}/cover`, { responseType: 'blob' })
        const url = URL.createObjectURL(blob)
        coverUrl.value = url
        coverRevoke = () => URL.revokeObjectURL(url)
      } catch { /* no cover preview */ }
    }
  } catch (e: any) {
    error.value = e?.data?.error || t('music.error_load')
  } finally {
    busy.value = false
  }
}

watch(() => props.node, (n) => { loadTags(n) }, { immediate: true })

onUnmounted(() => { revokeCoverUrl() })

// --- Cover replace via file input (shared for both modes) ---
function onCoverFile(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    const dataUrl = reader.result as string
    // dataUrl = "data:<mime>;base64,<data>"
    const commaIdx = dataUrl.indexOf(',')
    const meta = dataUrl.slice(5, commaIdx) // "image/jpeg;base64"
    const mime = meta.split(';')[0]
    const data = dataUrl.slice(commaIdx + 1)
    pendingCover.value = { data, mime }
    // Preview
    revokeCoverUrl()
    const url = URL.createObjectURL(file)
    coverUrl.value = url
    coverRevoke = () => URL.revokeObjectURL(url)
  }
  reader.readAsDataURL(file)
  // Reset input so same file can be re-picked
  if (coverInput.value) coverInput.value.value = ''
}

function removeCover() {
  pendingCover.value = 'remove'
  revokeCoverUrl()
}

// --- Build PUT payload (single-file) ---
function buildPayload() {
  if (!info.value) return {}
  const orig = info.value.tags
  const fields: string[] = []
  const values: Record<string, any> = {}

  const strField = (key: keyof TagsDTO, val: string) => {
    // Coerce: <input type="number"> can hand v-model a number, on which .trim() throws.
    const trimmed = String(val ?? '').trim()
    const origVal = orig[key] ?? ''
    if (trimmed !== String(origVal)) { fields.push(key); values[key] = trimmed }
  }
  const numField = (key: keyof TagsDTO, val: string) => {
    const trimmed = String(val ?? '').trim()
    const n = trimmed === '' ? null : parseInt(trimmed, 10)
    const origVal = orig[key] ?? null
    if (n !== origVal) { fields.push(key); values[key] = n }
  }

  strField('title', form.title)
  strField('artist', form.artist)
  strField('album', form.album)
  strField('albumArtist', form.albumArtist)
  strField('genre', form.genre)
  numField('year', form.year)
  numField('track', form.track)
  numField('disc', form.disc)

  const body: Record<string, any> = { fields, values }
  if (pendingCover.value === 'remove') body.cover = null
  else if (pendingCover.value !== 'keep') body.cover = pendingCover.value
  // else omit cover key → keep

  return body
}

// --- Build bulk POST payload ---
const NUM_BULK_FIELDS: BulkField[] = ['year', 'disc']

function buildBulkPayload() {
  const fields: string[] = []
  const values: Record<string, any> = {}

  for (const key of BULK_FIELDS) {
    if (!bulkApply[key]) continue
    fields.push(key)
    // Coerce: <input type="number"> can hand v-model a number, on which .trim() throws.
    if (NUM_BULK_FIELDS.includes(key)) {
      const trimmed = String(bulkForm[key] ?? '').trim()
      values[key] = trimmed === '' ? null : parseInt(trimmed, 10)
    } else {
      values[key] = String(bulkForm[key] ?? '').trim()
    }
  }

  const body: Record<string, any> = { fields, values }
  if (pendingCover.value === 'remove') body.cover = null
  else if (pendingCover.value !== 'keep') body.cover = pendingCover.value
  // else omit cover → keep

  return body
}

// Labels for the checked-fields summary in the confirm message
const FIELD_LABEL_KEYS: Record<BulkField, string> = {
  artist: 'music.artist',
  album: 'music.album',
  albumArtist: 'music.album_artist',
  genre: 'music.genre',
  year: 'music.year',
  disc: 'music.disc',
}

function checkedFieldLabels(): string {
  const parts: string[] = BULK_FIELDS.filter(k => bulkApply[k]).map(k => t(FIELD_LABEL_KEYS[k]))
  if (pendingCover.value !== 'keep') parts.push(t('music.cover'))
  return parts.join(', ')
}

// --- Save folder (bulk) ---
async function saveFolder() {
  bulkError.value = ''
  bulkResult.value = null
  saveBusy.value = true
  try {
    const { affected } = await request<{ affected: number }>(
      `/me/music/tags/folder/${props.node.id}/count`, { method: 'POST' },
    )
    const ok = await confirm(
      t('music.confirm_save_folder', { count: affected, name: props.node.name, fields: checkedFieldLabels() }),
      { danger: true },
    )
    if (!ok) return
    const res = await request<BulkResult>(
      `/me/music/tags/folder/${props.node.id}`,
      { method: 'POST', body: buildBulkPayload() },
    )
    bulkResult.value = res
  } catch (e: any) {
    bulkError.value = e?.data?.error || t('music.error_save')
  } finally {
    saveBusy.value = false
  }
}

// --- Save single file ---
async function save() {
  const ok = await confirm(t('music.confirm_save_title', { name: props.node.name }))
  if (!ok) return
  saveBusy.value = true
  error.value = ''
  try {
    await request(`/me/music/tags/${props.node.id}`, { method: 'PUT', body: buildPayload() })
    savedMsg.value = true
    setTimeout(() => { savedMsg.value = false }, 2500)
    // Reload to sync state
    await loadTags(props.node)
  } catch (e: any) {
    error.value = e?.data?.error || t('music.error_save')
  } finally {
    saveBusy.value = false
  }
}
</script>

<template>
  <!-- Bulk folder tag editor -->
  <div v-if="node.isDir" class="max-w-xl">
    <h2 class="mb-1 text-base font-semibold text-ink">{{ node.name }}</h2>
    <p class="mb-4 text-xs text-muted">{{ t('music.folder_hint') }}</p>

    <!-- Error -->
    <p v-if="bulkError" class="mb-4 flex items-center gap-2 text-sm text-danger">
      <Icon name="lucide:triangle-alert" size="16" /> {{ bulkError }}
    </p>

    <!-- Result summary -->
    <div v-if="bulkResult" class="mb-4 rounded-lg border border-line bg-panel2 p-3 text-sm">
      <p class="font-medium text-ink">
        {{ t('music.bulk_done', { updated: bulkResult.updated, affected: bulkResult.affected }) }}
      </p>
      <template v-if="bulkResult.failed.length">
        <p class="mt-2 text-xs font-medium text-muted">{{ t('music.bulk_failed') }}</p>
        <ul class="mt-1 space-y-0.5">
          <li v-for="f in bulkResult.failed" :key="f.path" class="text-xs text-danger">
            {{ f.path }} — {{ f.error }}
          </li>
        </ul>
      </template>
    </div>

    <!-- Cover -->
    <div class="mb-4 flex items-start gap-4">
      <div class="h-24 w-24 shrink-0 overflow-hidden rounded-lg border border-line bg-panel2">
        <img v-if="coverUrl" :src="coverUrl" alt="" class="h-full w-full object-cover" />
        <div v-else class="flex h-full w-full items-center justify-center">
          <Icon name="lucide:image" size="28" class="text-muted opacity-40" />
        </div>
      </div>
      <div class="flex flex-col gap-2 pt-1">
        <span class="text-xs font-medium text-muted">{{ t('music.cover') }}</span>
        <div class="flex flex-wrap gap-2">
          <button class="btn-ghost px-2 py-1 text-xs" @click="coverInput?.click()">
            <Icon name="lucide:image-plus" size="14" class="mr-1" />
            {{ t('music.cover_replace') }}
          </button>
          <button
            v-if="coverUrl || pendingCover !== 'keep'"
            class="btn-ghost px-2 py-1 text-xs text-danger"
            @click="removeCover"
          >
            <Icon name="lucide:trash-2" size="14" class="mr-1" />
            {{ t('music.cover_remove') }}
          </button>
        </div>
        <input ref="coverInput" type="file" accept="image/*" class="hidden" @change="onCoverFile" />
      </div>
    </div>

    <!-- Bulk fields with "apply" checkboxes -->
    <div class="grid gap-3">
      <!-- artist + album -->
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="mb-1 flex items-center gap-1.5 text-xs text-muted">
            <input v-model="bulkApply.artist" type="checkbox" class="accent-accent" />
            {{ t('music.artist') }}
            <span class="text-muted/60">{{ t('music.apply') }}</span>
          </label>
          <input v-model="bulkForm.artist" class="input w-full" :disabled="!bulkApply.artist" />
        </div>
        <div>
          <label class="mb-1 flex items-center gap-1.5 text-xs text-muted">
            <input v-model="bulkApply.album" type="checkbox" class="accent-accent" />
            {{ t('music.album') }}
            <span class="text-muted/60">{{ t('music.apply') }}</span>
          </label>
          <input v-model="bulkForm.album" class="input w-full" :disabled="!bulkApply.album" />
        </div>
      </div>
      <!-- albumArtist + genre -->
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="mb-1 flex items-center gap-1.5 text-xs text-muted">
            <input v-model="bulkApply.albumArtist" type="checkbox" class="accent-accent" />
            {{ t('music.album_artist') }}
            <span class="text-muted/60">{{ t('music.apply') }}</span>
          </label>
          <input v-model="bulkForm.albumArtist" class="input w-full" :disabled="!bulkApply.albumArtist" />
        </div>
        <div>
          <label class="mb-1 flex items-center gap-1.5 text-xs text-muted">
            <input v-model="bulkApply.genre" type="checkbox" class="accent-accent" />
            {{ t('music.genre') }}
            <span class="text-muted/60">{{ t('music.apply') }}</span>
          </label>
          <input v-model="bulkForm.genre" class="input w-full" :disabled="!bulkApply.genre" />
        </div>
      </div>
      <!-- year + disc -->
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="mb-1 flex items-center gap-1.5 text-xs text-muted">
            <input v-model="bulkApply.year" type="checkbox" class="accent-accent" />
            {{ t('music.year') }}
            <span class="text-muted/60">{{ t('music.apply') }}</span>
          </label>
          <input v-model="bulkForm.year" type="number" min="1" max="9999" class="input w-full" :disabled="!bulkApply.year" />
        </div>
        <div>
          <label class="mb-1 flex items-center gap-1.5 text-xs text-muted">
            <input v-model="bulkApply.disc" type="checkbox" class="accent-accent" />
            {{ t('music.disc') }}
            <span class="text-muted/60">{{ t('music.apply') }}</span>
          </label>
          <input v-model="bulkForm.disc" type="number" min="1" class="input w-full" :disabled="!bulkApply.disc" />
        </div>
      </div>
    </div>

    <!-- Save button -->
    <div class="mt-4">
      <button
        class="btn-accent"
        :disabled="saveBusy"
        @click="saveFolder"
      >
        <Icon v-if="saveBusy" name="lucide:loader-circle" class="animate-spin" size="16" />
        <Icon v-else name="lucide:save" size="16" />
        {{ t('music.save') }}
      </button>
    </div>
  </div>

  <!-- File tag editor -->
  <div v-else class="max-w-xl">
    <h2 class="mb-4 text-base font-semibold text-ink">{{ node.name }}</h2>

    <div v-if="busy" class="text-sm text-muted">{{ t('common.loading') }}</div>

    <template v-else-if="info">
      <!-- Read-only notice -->
      <div
        v-if="!info.writable"
        class="mb-4 flex items-center gap-2 rounded-lg border border-line bg-panel2 px-3 py-2 text-sm text-muted"
      >
        <Icon name="lucide:lock" size="16" class="shrink-0" />
        {{ t('music.read_only_notice') }}
      </div>

      <!-- Error -->
      <p v-if="error" class="mb-4 flex items-center gap-2 text-sm text-danger">
        <Icon name="lucide:triangle-alert" size="16" /> {{ error }}
      </p>

      <!-- Success -->
      <p v-if="savedMsg" class="mb-4 flex items-center gap-2 text-sm text-accent">
        <Icon name="lucide:check" size="16" /> {{ t('music.saved') }}
      </p>

      <!-- Cover -->
      <div class="mb-4 flex items-start gap-4">
        <div class="h-24 w-24 shrink-0 overflow-hidden rounded-lg border border-line bg-panel2">
          <img v-if="coverUrl" :src="coverUrl" alt="" class="h-full w-full object-cover" />
          <div v-else class="flex h-full w-full items-center justify-center">
            <Icon name="lucide:image" size="28" class="text-muted opacity-40" />
          </div>
        </div>
        <div class="flex flex-col gap-2 pt-1">
          <span class="text-xs font-medium text-muted">{{ t('music.cover') }}</span>
          <div class="flex flex-wrap gap-2">
            <button
              class="btn-ghost px-2 py-1 text-xs"
              :disabled="!info.writable"
              @click="coverInput?.click()"
            >
              <Icon name="lucide:image-plus" size="14" class="mr-1" />
              {{ t('music.cover_replace') }}
            </button>
            <button
              v-if="coverUrl || info.hasCover"
              class="btn-ghost px-2 py-1 text-xs text-danger"
              :disabled="!info.writable"
              @click="removeCover"
            >
              <Icon name="lucide:trash-2" size="14" class="mr-1" />
              {{ t('music.cover_remove') }}
            </button>
          </div>
          <input ref="coverInput" type="file" accept="image/*" class="hidden" @change="onCoverFile" />
        </div>
      </div>

      <!-- Tag fields -->
      <fieldset :disabled="!info.writable" class="grid gap-3">
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="mb-1 block text-xs text-muted">{{ t('music.title') }}</label>
            <input v-model="form.title" class="input w-full" :disabled="!info.writable" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-muted">{{ t('music.artist') }}</label>
            <input v-model="form.artist" class="input w-full" :disabled="!info.writable" />
          </div>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="mb-1 block text-xs text-muted">{{ t('music.album') }}</label>
            <input v-model="form.album" class="input w-full" :disabled="!info.writable" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-muted">{{ t('music.album_artist') }}</label>
            <input v-model="form.albumArtist" class="input w-full" :disabled="!info.writable" />
          </div>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="mb-1 block text-xs text-muted">{{ t('music.genre') }}</label>
            <input v-model="form.genre" class="input w-full" :disabled="!info.writable" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-muted">{{ t('music.year') }}</label>
            <input v-model="form.year" type="number" min="1" max="9999" class="input w-full" :disabled="!info.writable" />
          </div>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="mb-1 block text-xs text-muted">{{ t('music.track') }}</label>
            <input v-model="form.track" type="number" min="1" class="input w-full" :disabled="!info.writable" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-muted">{{ t('music.disc') }}</label>
            <input v-model="form.disc" type="number" min="1" class="input w-full" :disabled="!info.writable" />
          </div>
        </div>
      </fieldset>

      <!-- Save button -->
      <div class="mt-4">
        <button
          class="btn-accent"
          :disabled="!info.writable || saveBusy"
          @click="save"
        >
          <Icon v-if="saveBusy" name="lucide:loader-circle" class="animate-spin" size="16" />
          <Icon v-else name="lucide:save" size="16" />
          {{ t('music.save') }}
        </button>
      </div>
    </template>

    <p v-else-if="error" class="text-sm text-danger">{{ error }}</p>
  </div>
</template>
