<script setup lang="ts">
interface Device { id: string; name: string; kind: string; last_seen_at: string | null }

const { t, locale } = useI18n()
const { request } = useApi()
const { confirm } = useDialog()
const { SUPPORTED, saveLanguage } = useLocale()
const sess = useSession()

const devices = ref<Device[]>([])
const error = ref('')

async function load() {
  error.value = ''
  try {
    devices.value = await request<Device[]>('/devices')
  } catch (e: any) {
    error.value = e?.data?.error || t('common.error_load')
  }
}
onMounted(load)

function lastSeen(d: Device) {
  return d.last_seen_at
    ? t('settings.device_online', { date: new Date(d.last_seen_at).toLocaleString() })
    : t('settings.device_never')
}

async function disconnect(d: Device) {
  if (!(await confirm(t('settings.confirm_disconnect'), { message: `«${d.name}»`, confirmText: t('settings.confirm_disconnect_btn'), danger: true }))) return
  error.value = ''
  try {
    await request(`/devices/${d.id}`, { method: 'DELETE' })
    await load()
  } catch (e: any) {
    error.value = e?.data?.error || t('settings.error_disconnect')
  }
}

// Language selector
const langBusy = ref(false)
const langError = ref('')

async function onLanguageChange(e: Event) {
  const code = (e.target as HTMLSelectElement).value
  if (!code) return
  langBusy.value = true
  langError.value = ''
  try {
    await saveLanguage(code as any)
  } catch {
    langError.value = t('settings.error_language')
  } finally {
    langBusy.value = false
  }
}
</script>

<template>
  <div>
    <p v-if="error" class="mb-4 flex items-center gap-2 text-sm text-danger">
      <Icon name="lucide:triangle-alert" size="16" /> {{ error }}
    </p>

    <div class="mb-6 card p-5">
      <h2 class="mb-3 text-sm font-medium text-muted">{{ t('settings.profile') }}</h2>
      <dl class="grid grid-cols-[8rem_1fr] gap-y-2 text-sm">
        <dt class="text-muted">Email</dt><dd>{{ sess.email }}</dd>
        <dt class="text-muted">{{ t('settings.role') }}</dt><dd class="font-mono uppercase">{{ sess.role }}</dd>
      </dl>
    </div>

    <div class="card overflow-hidden mb-6">
      <h2 class="border-b border-line px-5 py-3 text-sm font-medium text-muted">{{ t('settings.devices_section') }}</h2>
      <div v-if="!devices.length" class="p-6 text-sm text-muted">{{ t('settings.no_devices') }}</div>
      <table v-else class="w-full text-sm">
        <tbody>
          <tr v-for="d in devices" :key="d.id" class="group border-b border-line/50 last:border-0">
            <td class="py-3 pl-5">
              <Icon name="lucide:monitor-smartphone" size="18" class="text-muted" />
            </td>
            <td class="py-3">
              <div>{{ d.name }}</div>
              <div class="text-xs text-muted">{{ d.kind }} · {{ lastSeen(d) }}</div>
            </td>
            <td class="py-3 pr-5 text-right">
              <button class="btn-danger px-2 py-1 opacity-0 transition group-hover:opacity-100" @click="disconnect(d)">
                <Icon name="lucide:unplug" size="16" /> {{ t('settings.btn_disconnect') }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="card p-5">
      <h2 class="mb-3 text-sm font-medium text-muted">{{ t('settings.language_section') }}</h2>
      <label class="flex items-center gap-3 text-sm">
        <span class="text-muted">{{ t('settings.language_label') }}</span>
        <select
          :value="locale"
          class="input w-44"
          :disabled="langBusy"
          @change="onLanguageChange"
        >
          <option v-for="code in SUPPORTED" :key="code" :value="code">
            {{ t(`languages.${code}`) }}
          </option>
        </select>
        <Icon v-if="langBusy" name="lucide:loader-circle" class="animate-spin text-muted" size="16" />
      </label>
      <p v-if="langError" class="mt-2 flex items-center gap-2 text-sm text-danger">
        <Icon name="lucide:triangle-alert" size="14" /> {{ langError }}
      </p>
    </div>
  </div>
</template>
