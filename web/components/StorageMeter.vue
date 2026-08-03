<script setup lang="ts">
// Occupied space and what is left of it, in the sidebar. Both `quota` and `available`
// are null for a user with no personal limit — there is nothing to fill a bar against,
// so it shows the number they occupy and an empty track. The server-wide cap is
// deliberately not part of this: it is the operator's business, and borrowing it as a
// total would present the shared disk's free space as the user's own.
interface Storage {
  used: number
  quota: number | null
  available: number | null
}

const { t } = useI18n()
const { request } = useApi()
const data = ref<Storage | null>(null)
const tick = useStorageTick()

async function load() {
  try {
    data.value = await request<Storage>('/me/storage')
  } catch {
    data.value = null // a storage read must never break the navigation around it
  }
}
onMounted(load)

// Every mutating request bumps the tick, and deleting twenty files is twenty of them —
// coalesce so the meter reads the numbers once, after the burst settles.
let pending: ReturnType<typeof setTimeout> | undefined
watch(tick, () => {
  clearTimeout(pending)
  pending = setTimeout(load, 400)
})
onBeforeUnmount(() => clearTimeout(pending))

const percent = computed(() => {
  const d = data.value
  if (!d?.quota) return 0 // no quota: nothing to be a share of, so the track stays empty
  return Math.min(100, Math.round((d.used / d.quota) * 100))
})
// Nearly full is worth noticing before an upload fails. 90% is also where the
// "running low on space" notification fires, so the two agree.
const tone = computed(() => (percent.value >= 90 ? 'bg-danger' : 'bg-accent'))
</script>

<template>
  <div v-if="data" class="mb-2 px-2">
    <div class="mb-1 flex items-baseline justify-between gap-2 text-xs">
      <span class="text-muted">{{ t('storage.title') }}</span>
      <span class="tabular-nums text-muted">
        <template v-if="data.quota != null">{{ formatBytes(data.used) }} / {{ formatBytes(data.quota) }}</template>
        <template v-else>{{ formatBytes(data.used) }}</template>
      </span>
    </div>
    <div class="h-1.5 w-full overflow-hidden rounded-full bg-ink/10">
      <div class="h-full rounded-full transition-all" :class="tone" :style="{ width: percent + '%' }" />
    </div>
    <div v-if="data.available != null" class="mt-1 text-[11px] text-muted">
      {{ t('storage.free', { size: formatBytes(data.available) }) }}
    </div>
    <div v-else class="mt-1 text-[11px] text-muted">{{ t('storage.no_limit') }}</div>
  </div>
</template>
