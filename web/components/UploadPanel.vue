<script setup lang="ts">
const { t } = useI18n()
const { tasks, pause, resume, cancel, clearFinished } = useUploads()
const active = computed(() => tasks.value.length > 0)
function pct(task: { sent: number; total: number }) {
  return task.total > 0 ? Math.min(100, Math.round((task.sent / task.total) * 100)) : 100
}
</script>

<template>
  <div v-if="active" class="fixed bottom-4 right-4 z-30 w-80 card p-3 shadow-lg">
    <div class="mb-2 flex items-center justify-between">
      <span class="text-sm font-medium">{{ t('uploads.title') }}</span>
      <button class="btn-ghost px-1.5 py-0.5 text-xs" @click="clearFinished">{{ t('uploads.clear') }}</button>
    </div>
    <div class="max-h-72 space-y-2 overflow-auto">
      <div v-for="task in tasks" :key="task.id" class="text-xs">
        <div class="mb-1 flex items-center justify-between gap-2">
          <span class="truncate" :title="task.name">{{ task.name }}</span>
          <div class="flex shrink-0 items-center gap-1">
            <button v-if="task.status === 'uploading'" class="btn-ghost px-1 py-0.5" :title="t('uploads.pause')" @click="pause(task.id)"><Icon name="lucide:pause" size="14" /></button>
            <button v-if="task.status === 'paused' || task.status === 'error'" class="btn-ghost px-1 py-0.5" :title="t('uploads.resume')" @click="resume(task.id)"><Icon name="lucide:play" size="14" /></button>
            <button v-if="task.status !== 'done' && task.status !== 'canceled'" class="btn-ghost px-1 py-0.5" :title="t('uploads.cancel')" @click="cancel(task.id)"><Icon name="lucide:x" size="14" /></button>
            <Icon v-if="task.status === 'done'" name="lucide:check" size="14" class="text-accent" />
          </div>
        </div>
        <div class="h-1.5 w-full overflow-hidden rounded-full bg-ink/5">
          <div class="h-full rounded-full transition-all"
               :class="task.status === 'error' ? 'bg-danger' : 'bg-accent'"
               :style="{ width: pct(task) + '%' }" />
        </div>
        <div v-if="task.status === 'error'" class="mt-0.5 text-danger">{{ task.error }}</div>
        <div v-else-if="task.status === 'paused'" class="mt-0.5 text-muted">{{ t('uploads.paused') }}</div>
      </div>
    </div>
  </div>
</template>
