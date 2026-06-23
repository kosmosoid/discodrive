<script setup lang="ts">
interface TrashNode { id: string; name: string; is_dir: boolean; size: number | null; deleted_at: string | null }

const { t } = useI18n()
const { request } = useApi()
const { confirm } = useDialog()
const items = ref<TrashNode[]>([])
const error = ref('')

async function load() {
  error.value = ''
  try {
    items.value = await request<TrashNode[]>('/files/trash')
  } catch (e: any) {
    error.value = e?.data?.error || t('trash.error_load')
  }
}
onMounted(load)

function delAt(d: string | null) {
  return d ? t('trash.deleted_at', { date: new Date(d).toLocaleString() }) : ''
}

async function restore(n: TrashNode) {
  error.value = ''
  try {
    await request(`/files/${n.id}/undelete`, { method: 'POST' })
    await load()
  } catch (e: any) {
    error.value = e?.data?.error || t('trash.error_restore')
  }
}

async function purge(n: TrashNode) {
  if (!(await confirm(t('trash.confirm_purge'), { message: t('trash.confirm_purge_msg', { name: n.name }), confirmText: t('trash.confirm_purge_btn'), danger: true }))) return
  error.value = ''
  try {
    await request(`/files/${n.id}/purge`, { method: 'DELETE' })
    await load()
  } catch (e: any) {
    error.value = e?.data?.error || t('trash.error_purge')
  }
}

async function purgeAll() {
  if (!(await confirm(t('trash.confirm_purge_all'), { message: t('trash.confirm_purge_all_msg'), confirmText: t('trash.confirm_purge_all_btn'), danger: true }))) return
  error.value = ''
  try {
    await request('/files/trash', { method: 'DELETE' })
    await load()
  } catch (e: any) {
    error.value = e?.data?.error || t('trash.error_purge_all')
  }
}
</script>

<template>
  <div>
    <div class="mb-4 flex items-center justify-between">
      <h1 class="text-xl font-semibold">{{ t('trash.title') }}</h1>
      <button v-if="items.length" class="btn-danger" @click="purgeAll">
        <Icon name="lucide:trash-2" size="16" /> {{ t('trash.purge_all') }}
      </button>
    </div>
    <p v-if="error" class="mb-4 flex items-center gap-2 text-sm text-danger">
      <Icon name="lucide:triangle-alert" size="16" /> {{ error }}
    </p>
    <div class="card overflow-hidden">
      <div v-if="!items.length" class="p-10 text-center text-sm text-muted">
        <Icon name="lucide:trash-2" size="28" class="mx-auto mb-2 block opacity-50" /> {{ t('trash.empty') }}
      </div>
      <table v-else class="w-full text-sm">
        <tbody>
          <tr v-for="n in items" :key="n.id" class="group border-b border-line/50 last:border-0 hover:bg-ink/5">
            <td class="w-8 py-2.5 pl-4">
              <Icon :name="n.is_dir ? 'lucide:folder' : 'lucide:file'" :class="n.is_dir ? 'text-accent' : 'text-muted'" size="18" />
            </td>
            <td class="py-2.5">{{ n.name }}</td>
            <td class="py-2.5 text-right text-xs text-muted">{{ n.is_dir ? '' : formatBytes(n.size) }}</td>
            <td class="py-2.5 text-xs text-muted">{{ delAt(n.deleted_at) }}</td>
            <td class="py-2.5 pr-4">
              <div class="flex justify-end gap-1 opacity-0 transition group-hover:opacity-100">
                <button class="btn-ghost px-2 py-1" :title="t('trash.btn_restore')" @click="restore(n)">
                  <Icon name="lucide:undo-2" size="16" />
                </button>
                <button class="btn-danger px-2 py-1" :title="t('trash.btn_purge')" @click="purge(n)">
                  <Icon name="lucide:trash-2" size="16" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
