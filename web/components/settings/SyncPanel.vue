<script setup lang="ts">
const { t } = useI18n()
const { request } = useApi()
const { confirm } = useDialog()

// Folder sync (daemon)
interface SyncStatus { enabled: boolean; folder: { id: string; name: string } | null; epoch: number }
const sync = ref<SyncStatus | null>(null)
const syncBusy = ref(false)
const syncError = ref('')
const syncPickerOpen = ref(false)

async function loadSync() {
  try { sync.value = await request<SyncStatus>('/me/sync') } catch { /* silent */ }
}
onMounted(loadSync)

// Any scope change makes the daemon WIPE & REBUILD its local folder, so confirm (danger) first.
async function applySyncChange(patch: { enabled: boolean; folderNodeId: string | null }) {
  if (!(await confirm(t('settings.sync_confirm'), {
    message: t('settings.sync_confirm_msg'),
    confirmText: t('settings.sync_confirm_btn'),
    danger: true,
  }))) return
  syncError.value = ''
  syncBusy.value = true
  try {
    sync.value = await request<SyncStatus>('/me/sync', { method: 'PUT', body: patch })
  } catch (e: any) {
    syncError.value = e?.data?.error || t('common.error_save')
  } finally {
    syncBusy.value = false
  }
}

// Enabling with no folder yet → open the picker; the actual enable happens on pick
// (server rejects enabled:true without a folderNodeId). Disabling is a direct PUT.
function onSyncToggle(checked: boolean) {
  if (checked) {
    if (sync.value?.folder) applySyncChange({ enabled: true, folderNodeId: sync.value.folder.id })
    else syncPickerOpen.value = true
  } else {
    applySyncChange({ enabled: false, folderNodeId: null })
  }
}

function onSyncFolderPicked(folder: { id: string; name: string } | null) {
  if (folder) applySyncChange({ enabled: true, folderNodeId: folder.id })
  else applySyncChange({ enabled: false, folderNodeId: null })
}
</script>

<template>
  <div>
    <!-- Folder sync (daemon) section -->
    <div class="card p-5">
      <h2 class="mb-3 text-sm font-medium text-muted">{{ t('settings.sync_section') }}</h2>
      <p class="mb-4 text-xs text-muted">{{ t('settings.sync_note') }}</p>

      <p v-if="syncError" class="mb-3 flex items-center gap-2 text-sm text-danger">
        <Icon name="lucide:triangle-alert" size="16" /> {{ syncError }}
      </p>

      <!-- Enable toggle -->
      <label class="mb-4 flex items-center gap-3 text-sm">
        <input
          type="checkbox"
          class="h-5 w-5"
          :checked="sync?.enabled ?? false"
          :disabled="syncBusy || !sync"
          @change="onSyncToggle(($event.target as HTMLInputElement).checked)"
        />
        <span>{{ t('settings.sync_enabled') }}</span>
        <Icon v-if="syncBusy" name="lucide:loader-circle" class="animate-spin text-muted" size="16" />
      </label>

      <!-- Folder picker -->
      <div class="flex items-center gap-2">
        <span class="w-40 shrink-0 text-xs text-muted">{{ t('settings.sync_folder') }}</span>
        <code class="min-w-0 flex-1 truncate rounded-md border border-line bg-panel2 px-2 py-1.5 font-mono text-xs">
          {{ sync?.folder?.name || t('settings.sync_no_folder') }}
        </code>
        <button class="btn-ghost shrink-0 px-2 py-1.5" :title="t('settings.sync_pick_folder')" @click="syncPickerOpen = true">
          <Icon name="lucide:folder-open" size="16" />
        </button>
        <button
          v-if="sync?.folder"
          class="btn-ghost shrink-0 px-2 py-1.5"
          :title="t('settings.sync_clear_folder')"
          @click="onSyncFolderPicked(null)"
        >
          <Icon name="lucide:x" size="16" />
        </button>
      </div>
    </div>

    <!-- Folder sync picker modal -->
    <MusicFolderPicker
      v-if="syncPickerOpen"
      :model-value="sync?.folder ?? null"
      @update:model-value="onSyncFolderPicked"
      @close="syncPickerOpen = false"
    />
  </div>
</template>
