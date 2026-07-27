<script setup lang="ts">
// Standalone pop-out player window. Playback lives HERE (own media element, own
// JS context) — it keeps playing when the main tab reloads or closes. State is
// handed over via localStorage; coordination with the main window runs over a
// BroadcastChannel inside usePlayer (role 'popup').
definePageMeta({ layout: false })

const { t } = useI18n()
const player = usePlayer()
const {
  status, position, duration, volume, muted, coverUrl, current, currentIsVideo, notice,
} = player

const videoEl = ref<HTMLVideoElement | null>(null)
onMounted(async () => {
  if (videoEl.value) player.attachMedia(videoEl.value, 'popup')
  // Continue where the main window left off. Autoplay may be refused (no user
  // gesture in THIS window yet) — then we sit paused at the right position.
  await player.toggle()
})

const playing = computed(() => status.value === 'playing' || status.value === 'buffering')

watch(current, (tr) => {
  if (import.meta.client) document.title = tr ? `${tr.title || tr.name} — DiscoDrive` : 'DiscoDrive'
}, { immediate: true })

function onSeek(e: Event) { player.seek(parseFloat((e.target as HTMLInputElement).value)) }
function onVolume(e: Event) { player.setVolume(parseFloat((e.target as HTMLInputElement).value)) }
</script>

<template>
  <div class="flex h-screen flex-col bg-panel text-ink">
    <video
      ref="videoEl"
      playsinline
      :class="currentIsVideo ? 'min-h-0 w-full flex-1 bg-black object-contain' : 'hidden'"
      @click="player.toggle()"
    />

    <div class="flex flex-1 flex-col justify-center gap-2 p-3" :class="currentIsVideo ? 'flex-none' : ''">
      <div
        v-if="notice"
        class="rounded-md bg-ink/80 px-3 py-1 text-center text-xs text-panel"
      >{{ notice }}</div>

      <div v-if="!currentIsVideo" class="flex items-center gap-3">
        <div class="flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-md bg-ink/10">
          <img v-if="coverUrl" :src="coverUrl" class="h-full w-full object-cover" alt="" />
          <Icon v-else name="lucide:music" size="20" class="text-muted" />
        </div>
        <div class="min-w-0 flex-1">
          <div class="truncate text-sm">{{ current?.title || current?.name }}</div>
          <div class="truncate text-xs text-muted">
            {{ current?.artist || '' }}<template v-if="current?.artist && current?.album"> · </template>{{ current?.album || '' }}
          </div>
        </div>
      </div>

      <div class="flex items-center gap-2 text-[11px] text-muted">
        <span class="w-9 text-right tabular-nums">{{ formatTime(position) }}</span>
        <input
          type="range" min="0" :max="duration || 0" step="0.1" :value="position"
          class="h-1 w-full accent-accent" :disabled="!duration" @input="onSeek"
        />
        <span class="w-9 tabular-nums">{{ formatTime(duration) }}</span>
      </div>

      <div class="flex items-center justify-between">
        <div class="w-24" />
        <div class="flex items-center justify-center gap-1">
          <button class="btn-ghost px-2 py-1" :title="t('player.prev')" @click="player.prev()">
            <Icon name="lucide:skip-back" size="18" />
          </button>
          <button class="btn-ghost px-2.5 py-1.5" :title="playing ? t('player.pause') : t('player.play')" @click="player.toggle()">
            <Icon v-if="status === 'buffering'" name="lucide:loader-circle" size="20" class="animate-spin" />
            <Icon v-else :name="playing ? 'lucide:pause' : 'lucide:play'" size="20" />
          </button>
          <button class="btn-ghost px-2 py-1" :title="t('player.next')" @click="player.next()">
            <Icon name="lucide:skip-forward" size="18" />
          </button>
        </div>
        <div class="flex w-24 items-center gap-1">
          <button class="btn-ghost px-1.5 py-1" :title="muted ? t('player.unmute') : t('player.mute')" @click="player.toggleMute()">
            <Icon :name="muted || volume === 0 ? 'lucide:volume-x' : 'lucide:volume-2'" size="14" />
          </button>
          <input
            type="range" min="0" max="1" step="0.01" :value="muted ? 0 : volume"
            class="h-1 w-full accent-accent" @input="onVolume"
          />
        </div>
      </div>
    </div>
  </div>
</template>
