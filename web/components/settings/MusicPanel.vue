<script setup lang="ts">
const { t } = useI18n()
const { request } = useApi()
const { confirm } = useDialog()
const { copied, copyText } = useCopy()
const sess = useSession()

const origin = computed(() => (import.meta.client ? location.origin : ''))
// Subsonic clients (Amperfy, Feishin, …) append "/rest" themselves, so show the base URL only.
const musicUrl = computed(() => origin.value)

// Music / OpenSubsonic
interface MusicStatus { enabled: boolean; folder: { id: string; name: string } | null; hasPassword: boolean; tagEditVersioning: boolean }
const music = ref<MusicStatus | null>(null)
const musicBusy = ref(false)
const musicError = ref('')
const musicPassword = ref('')
const musicApiKey = ref('')
const musicPickerOpen = ref(false)

async function loadMusic() {
  try { music.value = await request<MusicStatus>('/me/music') } catch { /* silent */ }
}
onMounted(loadMusic)

async function saveMusicSettings(patch: Partial<{ enabled: boolean; folderNodeId: string | null; tagEditVersioning: boolean }>) {
  if (!music.value) return
  musicError.value = ''
  musicBusy.value = true
  try {
    music.value = await request<MusicStatus>('/me/music', {
      method: 'PUT',
      body: {
        enabled: patch.enabled ?? music.value.enabled,
        folderNodeId: 'folderNodeId' in patch ? patch.folderNodeId : (music.value.folder?.id ?? null),
        tagEditVersioning: 'tagEditVersioning' in patch ? patch.tagEditVersioning : music.value.tagEditVersioning,
      },
    })
  } catch (e: any) {
    musicError.value = e?.data?.error || t('common.error_save')
  } finally {
    musicBusy.value = false
  }
}

async function generateMusicPassword() {
  musicError.value = ''
  musicBusy.value = true
  try {
    const res = await request<{ password: string; apiKey: string }>('/me/music/password', { method: 'POST' })
    musicPassword.value = res.password
    musicApiKey.value = res.apiKey
    if (music.value) music.value.hasPassword = true
  } catch (e: any) {
    musicError.value = e?.data?.error || t('common.error_save')
  } finally {
    musicBusy.value = false
  }
}

async function deleteMusicPassword() {
  if (!(await confirm(t('settings.music_confirm_revoke'), { confirmText: t('settings.music_confirm_revoke_btn'), danger: true }))) return
  musicError.value = ''
  musicBusy.value = true
  try {
    await request('/me/music/password', { method: 'DELETE' })
    musicPassword.value = ''
    musicApiKey.value = ''
    if (music.value) music.value.hasPassword = false
  } catch (e: any) {
    musicError.value = e?.data?.error || t('common.error_delete')
  } finally {
    musicBusy.value = false
  }
}

function onMusicFolderPicked(folder: { id: string; name: string } | null) {
  if (!music.value) return
  music.value.folder = folder
  saveMusicSettings({ folderNodeId: folder?.id ?? null })
}

// Internet radio & podcasts (shown when music is enabled)
interface RadioStation { id: string; name: string; streamUrl: string; homepageUrl: string }
interface PodcastChannel { id: string; title: string; feedUrl: string; hasCover: boolean }
const radioStations = ref<RadioStation[]>([])
const podcasts = ref<PodcastChannel[]>([])
// Object URLs (blob:) for podcast covers, keyed by channel id. Covers are fetched
// as blobs through the authed same-origin endpoint because the CSP blocks external
// img hosts and the Bearer token can't ride along a plain <img src>.
const podcastCovers = ref<Record<string, string>>({})

function revokePodcastCovers() {
  for (const url of Object.values(podcastCovers.value)) URL.revokeObjectURL(url)
  podcastCovers.value = {}
}

async function loadLibrary() {
  try { radioStations.value = await request<RadioStation[]>('/me/music/radio') } catch { /* silent */ }
  try { podcasts.value = await request<PodcastChannel[]>('/me/music/podcasts') } catch { /* silent */ }
  revokePodcastCovers()
  const next: Record<string, string> = {}
  for (const ch of podcasts.value) {
    if (!ch.hasCover) continue
    try {
      const blob = await request<Blob>(`/me/music/podcasts/${ch.id}/cover`, { responseType: 'blob' })
      next[ch.id] = URL.createObjectURL(blob)
    } catch { /* no cover */ }
  }
  podcastCovers.value = next
}

onUnmounted(revokePodcastCovers)
// Load (re)when the music section becomes enabled.
watch(() => music.value?.enabled, (enabled) => { if (enabled) loadLibrary() }, { immediate: true })

// Radio add modal
const radioModalOpen = ref(false)
const radioBusy = ref(false)
const radioModalError = ref('')
const radioName = ref('')
const radioStreamUrl = ref('')
const radioHomepageUrl = ref('')
const editingRadioId = ref<string | null>(null) // null = create, set = edit
useModalEscape(radioModalOpen, () => (radioModalOpen.value = false))

function openRadioModal() {
  editingRadioId.value = null
  radioName.value = ''
  radioStreamUrl.value = ''
  radioHomepageUrl.value = ''
  radioModalError.value = ''
  radioModalOpen.value = true
}

function openRadioEdit(s: RadioStation) {
  editingRadioId.value = s.id
  radioName.value = s.name
  radioStreamUrl.value = s.streamUrl
  radioHomepageUrl.value = s.homepageUrl
  radioModalError.value = ''
  radioModalOpen.value = true
}

async function saveRadio() {
  radioModalError.value = ''
  radioBusy.value = true
  try {
    const body = { name: radioName.value.trim(), streamUrl: radioStreamUrl.value.trim(), homepageUrl: radioHomepageUrl.value.trim() }
    if (editingRadioId.value) {
      await request(`/me/music/radio/${editingRadioId.value}`, { method: 'PUT', body })
    } else {
      await request('/me/music/radio', { method: 'POST', body })
    }
    radioModalOpen.value = false
    await loadLibrary()
  } catch (e: any) {
    radioModalError.value = e?.data?.error || t('settings.library_save_error')
  } finally {
    radioBusy.value = false
  }
}

async function deleteRadio(s: RadioStation) {
  if (!(await confirm(t('settings.radio_confirm_delete'), { message: `«${s.name}»`, danger: true }))) return
  try {
    await request(`/me/music/radio/${s.id}`, { method: 'DELETE' })
    await loadLibrary()
  } catch (e: any) {
    musicError.value = e?.data?.error || t('common.error_delete')
  }
}

// Podcast add modal
const podcastModalOpen = ref(false)
const podcastBusy = ref(false)
const podcastModalError = ref('')
const podcastUrl = ref('')
useModalEscape(podcastModalOpen, () => (podcastModalOpen.value = false))

function openPodcastModal() {
  podcastUrl.value = ''
  podcastModalError.value = ''
  podcastModalOpen.value = true
}

async function addPodcast() {
  podcastModalError.value = ''
  podcastBusy.value = true
  try {
    await request('/me/music/podcasts', { method: 'POST', body: { url: podcastUrl.value.trim() } })
    podcastModalOpen.value = false
    await loadLibrary()
  } catch (e: any) {
    podcastModalError.value = e?.data?.error || t('settings.podcasts_fetch_error')
  } finally {
    podcastBusy.value = false
  }
}

async function deletePodcast(p: PodcastChannel) {
  if (!(await confirm(t('settings.podcasts_confirm_delete'), { message: `«${p.title}»`, danger: true }))) return
  try {
    await request(`/me/music/podcasts/${p.id}`, { method: 'DELETE' })
    await loadLibrary()
  } catch (e: any) {
    musicError.value = e?.data?.error || t('common.error_delete')
  }
}
</script>

<template>
  <div>
    <!-- Music / OpenSubsonic section -->
    <div class="mb-6 card p-5">
      <h2 class="mb-3 text-sm font-medium text-muted">{{ t('settings.music_section') }}</h2>
      <p class="mb-4 text-xs text-muted">{{ t('settings.music_note') }}</p>

      <p v-if="musicError" class="mb-3 flex items-center gap-2 text-sm text-danger">
        <Icon name="lucide:triangle-alert" size="16" /> {{ musicError }}
      </p>

      <!-- Enable toggle -->
      <label class="mb-4 flex items-center gap-3 text-sm">
        <input
          type="checkbox"
          class="h-5 w-5"
          :checked="music?.enabled ?? false"
          :disabled="musicBusy || !music"
          @change="saveMusicSettings({ enabled: ($event.target as HTMLInputElement).checked })"
        />
        <span>{{ t('settings.music_enabled') }}</span>
        <Icon v-if="musicBusy" name="lucide:loader-circle" class="animate-spin text-muted" size="16" />
      </label>

      <!-- Tag-edit versioning toggle -->
      <label class="mb-4 flex items-start gap-3 text-sm">
        <input
          type="checkbox"
          class="mt-0.5 h-5 w-5 shrink-0"
          :checked="music?.tagEditVersioning ?? true"
          :disabled="musicBusy || !music"
          @change="saveMusicSettings({ tagEditVersioning: ($event.target as HTMLInputElement).checked })"
        />
        <span>
          {{ t('settings.music_tag_versioning') }}
          <span class="block text-xs text-muted">{{ t('settings.music_tag_versioning_hint') }}</span>
        </span>
      </label>

      <!-- Folder picker -->
      <div class="mb-4 flex items-center gap-2">
        <span class="w-40 shrink-0 text-xs text-muted">{{ t('settings.music_folder') }}</span>
        <code class="min-w-0 flex-1 truncate rounded-md border border-line bg-panel2 px-2 py-1.5 font-mono text-xs">
          {{ music?.folder?.name || t('settings.music_no_folder') }}
        </code>
        <button class="btn-ghost shrink-0 px-2 py-1.5" :title="t('settings.music_pick_folder')" @click="musicPickerOpen = true">
          <Icon name="lucide:folder-open" size="16" />
        </button>
        <button
          v-if="music?.folder"
          class="btn-ghost shrink-0 px-2 py-1.5"
          :title="t('settings.music_clear_folder')"
          @click="onMusicFolderPicked(null)"
        >
          <Icon name="lucide:x" size="16" />
        </button>
      </div>

      <!-- Connection info -->
      <div class="mb-4">
        <div class="mb-1 text-xs font-medium text-muted">{{ t('settings.music_connection') }}</div>
        <div class="mb-2 flex items-center gap-2">
          <span class="w-40 shrink-0 text-xs text-muted">{{ t('settings.music_server_url') }}</span>
          <code class="min-w-0 flex-1 truncate rounded-md border border-line bg-panel2 px-2 py-1.5 font-mono text-xs">{{ musicUrl }}</code>
          <button class="btn-ghost shrink-0 px-2 py-1.5" :title="t('settings.webdav_copy')" @click="copyText(musicUrl, 'music-url')">
            <Icon :name="copied === 'music-url' ? 'lucide:check' : 'lucide:copy'" size="16" />
          </button>
        </div>
        <div class="flex items-center gap-2">
          <span class="w-40 shrink-0 text-xs text-muted">{{ t('settings.dav_login') }}</span>
          <code class="min-w-0 flex-1 truncate rounded-md border border-line bg-panel2 px-2 py-1.5 font-mono text-xs">{{ sess.email }}</code>
          <button class="btn-ghost shrink-0 px-2 py-1.5" :title="t('settings.webdav_copy')" @click="copyText(sess.email || '', 'music-login')">
            <Icon :name="copied === 'music-login' ? 'lucide:check' : 'lucide:copy'" size="16" />
          </button>
        </div>
      </div>

      <!-- Password management -->
      <div class="mb-1 text-xs font-medium text-muted">{{ t('settings.music_password_title') }}</div>
      <p class="mb-2 text-xs text-muted">{{ t('settings.music_password_hint') }}</p>
      <div class="flex gap-2">
        <button class="btn-accent" :disabled="musicBusy" @click="generateMusicPassword">
          <Icon v-if="musicBusy" name="lucide:loader-circle" class="animate-spin" size="16" />
          <Icon v-else name="lucide:key-round" size="16" />
          {{ music?.hasPassword ? t('settings.music_regenerate') : t('settings.music_generate') }}
        </button>
        <button v-if="music?.hasPassword" class="btn-danger" :disabled="musicBusy" @click="deleteMusicPassword">
          <Icon name="lucide:trash-2" size="16" /> {{ t('settings.music_revoke') }}
        </button>
      </div>

      <!-- Revealed password (shown once) -->
      <div v-if="musicPassword" class="mt-3 rounded-md border border-line bg-panel2 p-2 text-xs">
        <div class="mb-1 text-muted">{{ t('settings.music_password_label') }}</div>
        <div class="mb-2 flex items-center gap-2">
          <span class="flex-1 break-all font-mono text-accent">{{ musicPassword }}</span>
          <button class="btn-ghost shrink-0 px-2 py-1" :title="t('settings.webdav_copy')" @click="copyText(musicPassword, 'music-pw')">
            <Icon :name="copied === 'music-pw' ? 'lucide:check' : 'lucide:copy'" size="16" />
          </button>
        </div>
        <div class="mb-1 text-muted">{{ t('settings.music_apikey_label') }}</div>
        <div class="flex items-center gap-2">
          <span class="flex-1 break-all font-mono text-accent">{{ musicApiKey }}</span>
          <button class="btn-ghost shrink-0 px-2 py-1" :title="t('settings.webdav_copy')" @click="copyText(musicApiKey, 'music-key')">
            <Icon :name="copied === 'music-key' ? 'lucide:check' : 'lucide:copy'" size="16" />
          </button>
        </div>
      </div>
    </div>

    <!-- Folder picker modal -->
    <MusicFolderPicker
      v-if="musicPickerOpen"
      :model-value="music?.folder ?? null"
      @update:model-value="onMusicFolderPicked"
      @close="musicPickerOpen = false"
    />

    <!-- Radio & podcasts section (shown when music is enabled) -->
    <div v-if="music?.enabled" class="card p-5">
      <!-- Internet radio -->
      <h2 class="mb-3 text-sm font-medium text-muted">{{ t('settings.radio_title') }}</h2>
      <ul v-if="radioStations.length" class="mb-3 space-y-2">
        <li v-for="s in radioStations" :key="s.id" class="flex items-start justify-between gap-2 rounded-md border border-line bg-panel2 p-3 text-sm">
          <div class="min-w-0">
            <div class="font-medium">{{ s.name }}</div>
            <div class="mt-0.5 truncate text-xs text-muted">{{ s.streamUrl }}</div>
          </div>
          <div class="flex shrink-0 gap-2">
            <button class="btn-ghost px-2 py-1" :title="t('settings.radio_edit')" @click="openRadioEdit(s)">
              <Icon name="lucide:pencil" size="14" />
            </button>
            <button class="btn-danger px-2 py-1" @click="deleteRadio(s)">
              <Icon name="lucide:trash-2" size="14" />
            </button>
          </div>
        </li>
      </ul>
      <p v-else class="mb-3 text-sm text-muted">{{ t('settings.radio_empty') }}</p>
      <button class="btn-accent" @click="openRadioModal">
        <Icon name="lucide:radio" size="16" /> {{ t('settings.radio_add') }}
      </button>

      <!-- Podcasts -->
      <h2 class="mb-3 mt-6 text-sm font-medium text-muted">{{ t('settings.podcasts_title') }}</h2>
      <ul v-if="podcasts.length" class="mb-3 space-y-2">
        <li v-for="p in podcasts" :key="p.id" class="flex items-start justify-between gap-2 rounded-md border border-line bg-panel2 p-3 text-sm">
          <div class="flex min-w-0 items-center gap-3">
            <img v-if="podcastCovers[p.id]" :src="podcastCovers[p.id]" alt="" class="h-10 w-10 shrink-0 rounded border border-line object-cover" />
            <div v-else class="flex h-10 w-10 shrink-0 items-center justify-center rounded border border-line bg-panel">
              <Icon name="lucide:podcast" size="18" class="text-muted" />
            </div>
            <div class="min-w-0">
              <div class="font-medium">{{ p.title }}</div>
              <div class="mt-0.5 truncate text-xs text-muted">{{ p.feedUrl }}</div>
            </div>
          </div>
          <button class="btn-danger shrink-0 px-2 py-1" @click="deletePodcast(p)">
            <Icon name="lucide:trash-2" size="14" />
          </button>
        </li>
      </ul>
      <p v-else class="mb-3 text-sm text-muted">{{ t('settings.podcasts_empty') }}</p>
      <button class="btn-accent" @click="openPodcastModal">
        <Icon name="lucide:podcast" size="16" /> {{ t('settings.podcasts_add') }}
      </button>
    </div>

    <!-- Add radio station modal -->
    <div v-if="radioModalOpen" class="fixed inset-0 z-20 flex items-center justify-center bg-black/50 p-4" @click.self="radioModalOpen = false">
      <div class="card flex w-full max-w-md flex-col p-5">
        <div class="mb-3 flex items-center justify-between">
          <h2 class="text-sm font-medium text-muted">{{ editingRadioId ? t('settings.radio_edit') : t('settings.radio_add') }}</h2>
          <button class="btn-ghost px-2 py-1" :title="t('common.close')" @click="radioModalOpen = false">
            <Icon name="lucide:x" size="16" />
          </button>
        </div>
        <p v-if="radioModalError" class="mb-3 flex items-center gap-2 text-sm text-danger">
          <Icon name="lucide:triangle-alert" size="16" /> {{ radioModalError }}
        </p>
        <div class="grid gap-2">
          <input v-model="radioName" class="input" :placeholder="t('settings.radio_name')" />
          <input v-model="radioStreamUrl" class="input" :placeholder="t('settings.radio_stream_url')" />
          <input v-model="radioHomepageUrl" class="input" :placeholder="t('settings.radio_homepage_url')" />
          <div class="flex gap-2">
            <button class="btn-accent" :disabled="radioBusy || !radioName.trim() || !radioStreamUrl.trim()" @click="saveRadio">
              <Icon v-if="radioBusy" name="lucide:loader-circle" class="animate-spin" size="16" />
              <Icon v-else :name="editingRadioId ? 'lucide:check' : 'lucide:plus'" size="16" /> {{ editingRadioId ? t('common.save') : t('settings.radio_add') }}
            </button>
            <button class="btn-ghost" :disabled="radioBusy" @click="radioModalOpen = false">{{ t('common.cancel') }}</button>
          </div>
        </div>
      </div>
    </div>

    <!-- Add podcast modal -->
    <div v-if="podcastModalOpen" class="fixed inset-0 z-20 flex items-center justify-center bg-black/50 p-4" @click.self="podcastModalOpen = false">
      <div class="card flex w-full max-w-md flex-col p-5">
        <div class="mb-3 flex items-center justify-between">
          <h2 class="text-sm font-medium text-muted">{{ t('settings.podcasts_add') }}</h2>
          <button class="btn-ghost px-2 py-1" :title="t('common.close')" @click="podcastModalOpen = false">
            <Icon name="lucide:x" size="16" />
          </button>
        </div>
        <p v-if="podcastModalError" class="mb-3 flex items-center gap-2 text-sm text-danger">
          <Icon name="lucide:triangle-alert" size="16" /> {{ podcastModalError }}
        </p>
        <div class="grid gap-2">
          <input v-model="podcastUrl" class="input" :placeholder="t('settings.podcasts_feed_url')" @keyup.enter="addPodcast" />
          <div class="flex gap-2">
            <button class="btn-accent" :disabled="podcastBusy || !podcastUrl.trim()" @click="addPodcast">
              <Icon v-if="podcastBusy" name="lucide:loader-circle" class="animate-spin" size="16" />
              <Icon v-else name="lucide:plus" size="16" /> {{ t('settings.podcasts_add') }}
            </button>
            <button class="btn-ghost" :disabled="podcastBusy" @click="podcastModalOpen = false">{{ t('common.cancel') }}</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
