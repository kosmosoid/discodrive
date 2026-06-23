<script setup lang="ts">
const { t } = useI18n()
const { request } = useApi()

const error = ref('')

interface NotifItem { key: string; category: string; mandatory: boolean; enabled: boolean }
const notifs = ref<NotifItem[]>([])

const notifKeyMap: Record<string, string> = {
  'share.received': 'notif_share_received',
  'quota.near_limit': 'notif_quota_near',
  'account.profile_changed': 'notif_profile_changed',
  'sync.failed': 'notif_sync_failed',
  'device.password_added': 'notif_device_password_added',
  'login.new_device': 'notif_login_new_device',
  'account.password_changed': 'notif_password_changed',
  'account.passkey_added': 'notif_passkey_added',
  'account.totp_enabled': 'notif_totp_enabled',
  'account.totp_disabled': 'notif_totp_disabled',
  'device.paired': 'notif_device_paired',
}
function notifLabel(k: string) {
  const msgKey = notifKeyMap[k]
  return msgKey ? t(`settings.${msgKey}`) : k
}

async function loadNotifs() {
  try { notifs.value = await request<NotifItem[]>('/me/notifications') } catch { /* silent */ }
}
onMounted(loadNotifs)

async function toggleNotif(n: NotifItem) {
  if (n.mandatory) return
  try {
    await request('/me/notifications', { method: 'PUT', body: { event_key: n.key, enabled: !n.enabled } })
    n.enabled = !n.enabled
  } catch (e: any) { error.value = e?.data?.error || t('common.error_save') }
}
const optionalNotifs = computed(() => notifs.value.filter((n) => !n.mandatory))
const securityNotifs = computed(() => notifs.value.filter((n) => n.mandatory))
</script>

<template>
  <div class="card p-5">
    <h2 class="mb-3 text-sm font-medium text-muted">{{ t('settings.notifications_section') }}</h2>

    <p v-if="error" class="mb-3 flex items-center gap-2 text-sm text-danger">
      <Icon name="lucide:triangle-alert" size="16" /> {{ error }}
    </p>

    <div class="space-y-2">
      <label v-for="n in optionalNotifs" :key="n.key" class="flex items-center justify-between text-sm">
        <span>{{ notifLabel(n.key) }}</span>
        <input type="checkbox" class="h-5 w-5" :checked="n.enabled" @change="toggleNotif(n)" />
      </label>
    </div>
    <div v-if="securityNotifs.length" class="mt-4 border-t border-line/50 pt-3">
      <div class="mb-2 text-xs text-muted">{{ t('settings.security_notifications') }}</div>
      <ul class="space-y-1 text-sm text-muted">
        <li v-for="n in securityNotifs" :key="n.key" class="flex items-center gap-2">
          <Icon name="lucide:shield-check" size="14" class="text-accent" /> {{ notifLabel(n.key) }}
        </li>
      </ul>
    </div>
  </div>
</template>
