<script setup lang="ts">
const sess = useSession()
const { fetchAndApply } = useLocale()

// Fetch user's language preference from server whenever the session becomes active.
watch(
  () => sess.value.token,
  (token) => { if (token) fetchAndApply() },
  { immediate: true },
)
</script>

<template>
  <NuxtLayout>
    <NuxtPage />
  </NuxtLayout>
  <AppDialog />
  <UploadPanel />
</template>
