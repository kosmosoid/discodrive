<script setup lang="ts">
const { t } = useI18n()
const route = useRoute()
const { request } = useApi()
const code = (route.query.code as string) || ''
const info = ref<{ proposed_name: string; kind: string } | null>(null)
const name = ref('')
const state = ref<'loading' | 'ready' | 'done' | 'error'>('loading')
const error = ref('')

onMounted(async () => {
  if (!code) { state.value = 'error'; error.value = t('pair.error_no_code'); return }
  try {
    info.value = await request(`/pair/${encodeURIComponent(code)}`)
    name.value = info.value!.proposed_name
    state.value = 'ready'
  } catch (e: any) {
    state.value = 'error'
    error.value = e?.response?.status === 404 ? t('pair.error_not_found') : t('pair.error_load')
  }
})

async function approve() {
  try {
    await request(`/pair/${encodeURIComponent(code)}/approve`, { method: 'POST', body: { name: name.value } })
    state.value = 'done'
  } catch (e: any) {
    state.value = 'error'
    error.value = e?.response?.status === 409 ? t('pair.error_already')
      : e?.response?.status === 410 ? t('pair.error_expired')
      : t('pair.error_pair')
  }
}
</script>

<template>
  <div>
    <h1 class="mb-4 text-xl font-semibold">{{ t('pair.title') }}</h1>

    <p v-if="state === 'loading'" class="text-sm text-muted">{{ t('pair.loading') }}</p>

    <div v-else-if="state === 'ready'" class="card max-w-sm p-5">
      <p class="mb-4 text-sm">{{ t('pair.question') }}</p>
      <div class="mb-3">
        <label class="mb-1 block text-xs text-muted">{{ t('pair.device_name') }}</label>
        <input v-model="name" type="text" class="input" :placeholder="t('pair.device_name_ph')" />
      </div>
      <p class="mb-4 text-xs text-muted">{{ t('pair.type', { kind: info?.kind }) }}</p>
      <div class="flex gap-2">
        <button class="btn-accent" @click="approve">
          <Icon name="lucide:link" size="16" /> {{ t('pair.btn_pair') }}
        </button>
        <NuxtLink to="/files">
          <button class="btn-ghost">{{ t('pair.btn_cancel') }}</button>
        </NuxtLink>
      </div>
    </div>

    <div v-else-if="state === 'done'" class="card max-w-sm p-5">
      <p class="flex items-center gap-2 text-sm text-accent">
        <Icon name="lucide:check-circle" size="18" /> {{ t('pair.done') }}
      </p>
    </div>

    <p v-else class="flex items-center gap-2 text-sm text-danger">
      <Icon name="lucide:triangle-alert" size="16" /> {{ error }}
    </p>
  </div>
</template>
