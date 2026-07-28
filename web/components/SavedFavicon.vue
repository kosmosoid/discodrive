<script setup lang="ts">
// A small favicon image fetched as a blob through an authed endpoint (the CSP
// blocks external <img src>), with an icon fallback. `src` is the endpoint
// path; pass '' to always show the fallback icon.
const props = defineProps<{ src: string; icon: string }>()

const { request } = useApi()
const url = ref('')

async function load() {
  if (!props.src) return
  try {
    const blob = await request<Blob>(props.src, { responseType: 'blob' })
    url.value = URL.createObjectURL(blob)
  } catch { /* keep the icon fallback */ }
}
onMounted(load)
onBeforeUnmount(() => { if (url.value) URL.revokeObjectURL(url.value) })
</script>

<template>
  <img v-if="url" :src="url" class="h-5 w-5 shrink-0 rounded-sm object-contain" alt="" />
  <Icon v-else :name="icon" size="18" class="shrink-0 text-muted" />
</template>
