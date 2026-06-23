<script setup lang="ts">
definePageMeta({ layout: 'auth' })

const { t } = useI18n()
const email = ref('')
const password = ref('')
const password2 = ref('')
const error = ref('')
const busy = ref(false)

async function submit() {
  error.value = ''
  if (password.value !== password2.value) {
    error.value = t('setup.error_mismatch')
    return
  }
  busy.value = true
  try {
    await apiFetch('/setup/admin', { method: 'POST', body: { email: email.value, password: password.value } })
    await login(email.value, password.value)
    useSetupNeeded().value = false
    await navigateTo('/admin')
  } catch (e: any) {
    error.value = e?.data?.error || t('setup.error_create')
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="card w-full max-w-sm p-7">
    <div class="mb-1 flex items-center gap-2">
      <Icon name="lucide:hard-drive" class="text-accent" size="26" />
      <span class="font-mono text-2xl font-semibold tracking-tight">Disco<span class="text-accent">Drive</span></span>
    </div>
    <p class="mb-6 text-sm text-muted">{{ t('setup.subtitle') }}</p>
    <form class="space-y-4" @submit.prevent="submit">
      <div>
        <label class="mb-1 block text-xs text-muted">Email</label>
        <input v-model="email" type="email" autocomplete="username" class="input" placeholder="you@host" />
      </div>
      <div>
        <label class="mb-1 block text-xs text-muted">{{ t('setup.password_label') }}</label>
        <input v-model="password" type="password" autocomplete="new-password" class="input" placeholder="••••••••" />
      </div>
      <div>
        <label class="mb-1 block text-xs text-muted">{{ t('setup.password_again') }}</label>
        <input v-model="password2" type="password" autocomplete="new-password" class="input" placeholder="••••••••" />
      </div>
      <p v-if="error" class="flex items-center gap-2 text-sm text-danger">
        <Icon name="lucide:triangle-alert" size="16" /> {{ error }}
      </p>
      <button type="submit" class="btn-accent w-full justify-center" :disabled="busy">
        <Icon v-if="busy" name="lucide:loader-circle" class="animate-spin" size="18" />
        <Icon v-else name="lucide:shield-check" size="18" />
        {{ t('setup.submit') }}
      </button>
    </form>
  </div>
</template>
