<script setup lang="ts">
const sess = useSession()
const { t } = useI18n()
onMounted(() => navigateTo(sess.value.role === 'admin' ? '/admin' : '/files'))
</script>

<template>
  <div class="p-8 text-muted">{{ t('index.loading') }}</div>
</template>
