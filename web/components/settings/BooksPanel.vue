<script setup lang="ts">
const { t } = useI18n()
const { request } = useApi()
const { confirm } = useDialog()
const { copied, copyText } = useCopy()
const sess = useSession()

const origin = computed(() => (import.meta.client ? location.origin : ''))
// OPDS root catalog — readers (KOReader, Thorium, Moon+ Reader, Calibre) connect to this URL.
const opdsUrl = computed(() => origin.value + '/opds')

// Books / OPDS
interface EbookStatus { enabled: boolean; folder: { id: string; name: string } | null; hasPassword: boolean; hasApiKey: boolean }
const ebook = ref<EbookStatus | null>(null)
const ebookBusy = ref(false)
const ebookError = ref('')
const ebookPassword = ref('')
const ebookApiKey = ref('')
const ebookPickerOpen = ref(false)

async function loadEbook() {
  try { ebook.value = await request<EbookStatus>('/me/ebooks') } catch { /* silent */ }
}
onMounted(loadEbook)

async function saveEbookSettings(patch: Partial<{ enabled: boolean; folderNodeId: string | null }>) {
  if (!ebook.value) return
  ebookError.value = ''
  ebookBusy.value = true
  try {
    ebook.value = await request<EbookStatus>('/me/ebooks', {
      method: 'PUT',
      body: {
        enabled: patch.enabled ?? ebook.value.enabled,
        folderNodeId: 'folderNodeId' in patch ? patch.folderNodeId : (ebook.value.folder?.id ?? null),
      },
    })
  } catch (e: any) {
    ebookError.value = e?.data?.error || t('common.error_save')
  } finally {
    ebookBusy.value = false
  }
}

async function generateEbookPassword() {
  ebookError.value = ''
  ebookBusy.value = true
  try {
    const res = await request<{ password: string; apiKey: string }>('/me/ebooks/password', { method: 'POST' })
    ebookPassword.value = res.password
    ebookApiKey.value = res.apiKey
    if (ebook.value) { ebook.value.hasPassword = true; ebook.value.hasApiKey = true }
  } catch (e: any) {
    ebookError.value = e?.data?.error || t('common.error_save')
  } finally {
    ebookBusy.value = false
  }
}

async function deleteEbookPassword() {
  if (!(await confirm(t('settings.books_confirm_revoke'), { confirmText: t('settings.books_confirm_revoke_btn'), danger: true }))) return
  ebookError.value = ''
  ebookBusy.value = true
  try {
    await request('/me/ebooks/password', { method: 'DELETE' })
    ebookPassword.value = ''
    ebookApiKey.value = ''
    if (ebook.value) { ebook.value.hasPassword = false; ebook.value.hasApiKey = false }
  } catch (e: any) {
    ebookError.value = e?.data?.error || t('common.error_delete')
  } finally {
    ebookBusy.value = false
  }
}

function onEbookFolderPicked(folder: { id: string; name: string } | null) {
  if (!ebook.value) return
  ebook.value.folder = folder
  saveEbookSettings({ folderNodeId: folder?.id ?? null })
}
</script>

<template>
  <div>
    <!-- Books / OPDS section -->
    <div class="card p-5">
      <h2 class="mb-3 text-sm font-medium text-muted">{{ t('settings.books_section') }}</h2>
      <p class="mb-4 text-xs text-muted">{{ t('settings.books_note') }}</p>

      <p v-if="ebookError" class="mb-3 flex items-center gap-2 text-sm text-danger">
        <Icon name="lucide:triangle-alert" size="16" /> {{ ebookError }}
      </p>

      <!-- Enable toggle -->
      <label class="mb-4 flex items-center gap-3 text-sm">
        <input
          type="checkbox"
          class="h-5 w-5"
          :checked="ebook?.enabled ?? false"
          :disabled="ebookBusy || !ebook"
          @change="saveEbookSettings({ enabled: ($event.target as HTMLInputElement).checked })"
        />
        <span>{{ t('settings.books_enabled') }}</span>
        <Icon v-if="ebookBusy" name="lucide:loader-circle" class="animate-spin text-muted" size="16" />
      </label>

      <!-- Folder picker -->
      <div class="mb-4 flex items-center gap-2">
        <span class="w-40 shrink-0 text-xs text-muted">{{ t('settings.books_folder') }}</span>
        <code class="min-w-0 flex-1 truncate rounded-md border border-line bg-panel2 px-2 py-1.5 font-mono text-xs">
          {{ ebook?.folder?.name || t('settings.books_no_folder') }}
        </code>
        <button class="btn-ghost shrink-0 px-2 py-1.5" :title="t('settings.books_pick_folder')" @click="ebookPickerOpen = true">
          <Icon name="lucide:folder-open" size="16" />
        </button>
        <button
          v-if="ebook?.folder"
          class="btn-ghost shrink-0 px-2 py-1.5"
          :title="t('settings.books_clear_folder')"
          @click="onEbookFolderPicked(null)"
        >
          <Icon name="lucide:x" size="16" />
        </button>
      </div>

      <!-- Connection info -->
      <div class="mb-4">
        <div class="mb-1 text-xs font-medium text-muted">{{ t('settings.books_connection') }}</div>
        <div class="mb-2 flex items-center gap-2">
          <span class="w-40 shrink-0 text-xs text-muted">{{ t('settings.books_opds_url') }}</span>
          <code class="min-w-0 flex-1 truncate rounded-md border border-line bg-panel2 px-2 py-1.5 font-mono text-xs">{{ opdsUrl }}</code>
          <button class="btn-ghost shrink-0 px-2 py-1.5" :title="t('settings.webdav_copy')" @click="copyText(opdsUrl, 'books-url')">
            <Icon :name="copied === 'books-url' ? 'lucide:check' : 'lucide:copy'" size="16" />
          </button>
        </div>
        <div class="flex items-center gap-2">
          <span class="w-40 shrink-0 text-xs text-muted">{{ t('settings.dav_login') }}</span>
          <code class="min-w-0 flex-1 truncate rounded-md border border-line bg-panel2 px-2 py-1.5 font-mono text-xs">{{ sess.email }}</code>
          <button class="btn-ghost shrink-0 px-2 py-1.5" :title="t('settings.webdav_copy')" @click="copyText(sess.email || '', 'books-login')">
            <Icon :name="copied === 'books-login' ? 'lucide:check' : 'lucide:copy'" size="16" />
          </button>
        </div>
      </div>

      <!-- Password management -->
      <div class="mb-1 text-xs font-medium text-muted">{{ t('settings.books_password_title') }}</div>
      <p class="mb-2 text-xs text-muted">{{ t('settings.books_password_hint') }}</p>
      <div class="flex gap-2">
        <button class="btn-accent" :disabled="ebookBusy" @click="generateEbookPassword">
          <Icon v-if="ebookBusy" name="lucide:loader-circle" class="animate-spin" size="16" />
          <Icon v-else name="lucide:key-round" size="16" />
          {{ ebook?.hasPassword ? t('settings.books_regenerate') : t('settings.books_generate') }}
        </button>
        <button v-if="ebook?.hasPassword" class="btn-danger" :disabled="ebookBusy" @click="deleteEbookPassword">
          <Icon name="lucide:trash-2" size="16" /> {{ t('settings.books_revoke') }}
        </button>
      </div>

      <!-- Revealed password (shown once) -->
      <div v-if="ebookPassword" class="mt-3 rounded-md border border-line bg-panel2 p-2 text-xs">
        <div class="mb-1 text-muted">{{ t('settings.books_password_label') }}</div>
        <div class="mb-2 flex items-center gap-2">
          <span class="flex-1 break-all font-mono text-accent">{{ ebookPassword }}</span>
          <button class="btn-ghost shrink-0 px-2 py-1" :title="t('settings.webdav_copy')" @click="copyText(ebookPassword, 'books-pw')">
            <Icon :name="copied === 'books-pw' ? 'lucide:check' : 'lucide:copy'" size="16" />
          </button>
        </div>
        <div class="mb-1 text-muted">{{ t('settings.books_apikey_label') }}</div>
        <div class="flex items-center gap-2">
          <span class="flex-1 break-all font-mono text-accent">{{ ebookApiKey }}</span>
          <button class="btn-ghost shrink-0 px-2 py-1" :title="t('settings.webdav_copy')" @click="copyText(ebookApiKey, 'books-key')">
            <Icon :name="copied === 'books-key' ? 'lucide:check' : 'lucide:copy'" size="16" />
          </button>
        </div>
      </div>
    </div>

    <!-- Books folder picker modal -->
    <MusicFolderPicker
      v-if="ebookPickerOpen"
      :model-value="ebook?.folder ?? null"
      @update:model-value="onEbookFolderPicked"
      @close="ebookPickerOpen = false"
    />
  </div>
</template>
