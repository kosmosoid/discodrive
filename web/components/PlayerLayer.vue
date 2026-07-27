<script setup lang="ts">
// Owns the single shared <video> element (always mounted, even when idle — the
// engine needs it before the first track starts) plus the video theater. The
// pop-out player is a real popup window (/player page), not Document PiP: a PiP
// window dies with its opener, while the user's point of detaching is playback
// that survives reloading or closing the main tab.
const player = usePlayer()
const { t } = useI18n()
const { active, theater, currentIsVideo } = player

const videoEl = ref<HTMLVideoElement | null>(null)
onMounted(() => {
  if (videoEl.value) player.attachMedia(videoEl.value, 'main')
})

// Theater is only meaningful while the current track is a video; the engine
// opens it automatically when a video track starts (usePlayer.loadAndPlay).
watch(currentIsVideo, (isVideo) => { if (!isVideo) theater.value = false })
useModalEscape(computed(() => theater.value), () => { theater.value = false })

function toggleOnVideo() { void player.toggle() }

async function nativePiP() {
  try { await (videoEl.value as any)?.requestPictureInPicture?.() } catch { /* unsupported/denied */ }
}
async function fullscreen() {
  try { await videoEl.value?.requestFullscreen() } catch { /* denied */ }
}
</script>

<template>
  <div>
    <!-- The one media element. Hidden for audio; fills the theater for video. -->
    <video
      ref="videoEl"
      playsinline
      :class="theater && currentIsVideo
        ? 'fixed inset-x-0 top-0 bottom-16 z-40 h-auto w-full bg-black object-contain'
        : 'hidden'"
      @click="toggleOnVideo"
    />

    <!-- Theater chrome: floats over the video, bar stays visible below. -->
    <div v-if="theater && currentIsVideo" class="fixed right-3 top-3 z-50 flex gap-1">
      <button class="btn-ghost bg-black/40 px-2 py-1 text-white" :title="t('player.pip')" @click="nativePiP">
        <Icon name="lucide:picture-in-picture" size="18" />
      </button>
      <button class="btn-ghost bg-black/40 px-2 py-1 text-white" :title="t('player.fullscreen')" @click="fullscreen">
        <Icon name="lucide:maximize" size="18" />
      </button>
      <button class="btn-ghost bg-black/40 px-2 py-1 text-white" :title="t('player.collapse_video')" @click="theater = false">
        <Icon name="lucide:x" size="18" />
      </button>
    </div>

    <PlayerBar v-if="active" />
  </div>
</template>
