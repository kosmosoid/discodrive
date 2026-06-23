<script setup lang="ts">
interface Overview {
  disk: { total: number; used: number; free: number }
  users: { id: string; email: string; role: string; quota: number | null; used: number }[]
}

const { t } = useI18n()
const { request } = useApi()
const data = ref<Overview | null>(null)
const error = ref('')

async function load() {
  error.value = ''
  try {
    data.value = await request<Overview>('/admin/overview')
  } catch (e: any) {
    error.value = e?.data?.error || t('admin.error_load')
  }
}
onMounted(load)

const diskPct = computed(() => {
  const d = data.value?.disk
  return d && d.total ? Math.round((d.used / d.total) * 100) : 0
})
</script>

<template>
  <div>
    <h1 class="mb-4 text-xl font-semibold">{{ t('admin.dashboard') }}</h1>
    <p v-if="error" class="mb-4 flex items-center gap-2 text-sm text-danger">
      <Icon name="lucide:triangle-alert" size="16" /> {{ error }}
    </p>

    <div class="mb-6 grid gap-4 sm:grid-cols-3">
      <div class="card p-5">
        <div class="mb-2 text-xs text-muted">{{ t('admin.disk_used') }}</div>
        <div class="text-2xl font-semibold">{{ formatBytes(data?.disk.used) }}</div>
        <div class="mt-1 text-xs text-muted">{{ t('admin.of') }} {{ formatBytes(data?.disk.total) }}</div>
        <div class="mt-3 h-1.5 w-full overflow-hidden rounded-full bg-ink/5">
          <div class="h-full rounded-full bg-accent" :style="{ width: diskPct + '%' }" />
        </div>
      </div>
      <div class="card p-5">
        <div class="mb-2 text-xs text-muted">{{ t('admin.disk_free') }}</div>
        <div class="text-2xl font-semibold">{{ formatBytes(data?.disk.free) }}</div>
      </div>
      <div class="card p-5">
        <div class="mb-2 text-xs text-muted">{{ t('admin.users_count') }}</div>
        <div class="text-2xl font-semibold">{{ data?.users.length ?? '—' }}</div>
      </div>
    </div>

    <div class="card overflow-hidden">
      <table class="w-full text-sm">
        <thead class="border-b border-line text-left text-xs text-muted">
          <tr>
            <th class="px-4 py-3 font-medium">{{ t('admin.col_email') }}</th>
            <th class="px-4 py-3 font-medium">{{ t('admin.col_role') }}</th>
            <th class="px-4 py-3 font-medium">{{ t('admin.col_used') }}</th>
            <th class="px-4 py-3 font-medium">{{ t('admin.col_quota') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="u in data?.users" :key="u.id" class="border-b border-line/50 last:border-0">
            <td class="px-4 py-3">{{ u.email }}</td>
            <td class="px-4 py-3">
              <span class="rounded bg-ink/5 px-1.5 py-0.5 font-mono text-[10px] uppercase">{{ u.role }}</span>
            </td>
            <td class="px-4 py-3 text-muted">{{ formatBytes(u.used) }}</td>
            <td class="px-4 py-3 text-muted">{{ u.quota == null ? t('admin.no_limit') : formatBytes(u.quota) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
