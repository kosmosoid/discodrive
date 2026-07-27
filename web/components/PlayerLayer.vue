<script setup lang="ts">
// Owns the single shared <video> element (always mounted, even when idle — the
// engine needs it before the first track starts). Renders the bar, the video
// theater and the Document PiP mini-window. The media element itself NEVER
// moves into the PiP window: only UI teleports there, so the future Web Audio
// graph and playback survive detach/reattach.
const { t } = useI18n()
const player = usePlayer()
const { active, theater, currentIsVideo, status, current, coverUrl } = player

const videoEl = ref<HTMLVideoElement | null>(null)
onMounted(() => {
  if (videoEl.value) player.attachMedia(videoEl.value)
})

// Theater is only meaningful while the current track is a video.
watch(currentIsVideo, (isVideo) => { if (!isVideo) theater.value = false })
// Auto-advance INTO a video opens the theater (queue = "watching this folder").
watch(current, (tr) => {
  if (tr && currentIsVideo.value && status.value !== 'idle') theater.value = true
})
useModalEscape(computed(() => theater.value), () => { theater.value = false })

function toggleOnVideo() { void player.toggle() }

async function nativePiP() {
  try { await (videoEl.value as any)?.requestPictureInPicture?.() } catch { /* unsupported/denied */ }
}
async function fullscreen() {
  try { await videoEl.value?.requestFullscreen() } catch { /* denied */ }
}

// --- Document Picture-in-Picture (Chromium 130+, Firefox 151+) ---
const canDetach = import.meta.client && 'documentPictureInPicture' in window
const pipBody = ref<HTMLElement | null>(null)

async function detach() {
  if (pipBody.value) return
  try {
    const win: Window = await (window as any).documentPictureInPicture.requestWindow({ width: 380, height: 150 })
    // The PiP document starts empty: carry our styles and theme over.
    for (const sheet of Array.from(document.styleSheets)) {
      try {
        if (sheet.href) {
          const link = win.document.createElement('link')
          link.rel = 'stylesheet'
          link.href = sheet.href
          win.document.head.appendChild(link)
        } else if (sheet.ownerNode instanceof HTMLStyleElement) {
          win.document.head.appendChild(sheet.ownerNode.cloneNode(true))
        }
      } catch { /* cross-origin sheet */ }
    }
    win.document.documentElement.className = document.documentElement.className
    for (const [k, v] of Object.entries((document.documentElement as HTMLElement).dataset)) {
      ;(win.document.documentElement as HTMLElement).dataset[k] = v ?? ''
    }
    win.document.body.className = 'bg-panel text-ink'
    win.addEventListener('pagehide', () => { pipBody.value = null })
    pipBody.value = win.document.body
  } catch { /* user gesture required / denied */ }
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

    <PlayerBar v-if="active" :can-detach="!!canDetach && !pipBody" @detach="detach" />

    <!-- Compact detached controls: UI only, playback stays in this window. -->
    <Teleport v-if="pipBody" :to="pipBody">
      <div class="flex h-full flex-col justify-center gap-2 p-3">
        <div class="flex items-center gap-3">
          <div class="flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-md bg-ink/10">
            <img v-if="coverUrl" :src="coverUrl" class="h-full w-full object-cover" alt="" />
            <Icon v-else name="lucide:music" size="20" class="text-muted" />
          </div>
          <div class="min-w-0 flex-1">
            <div class="truncate text-sm">{{ current?.title || current?.name }}</div>
            <div class="truncate text-xs text-muted">{{ current?.artist || '' }}</div>
          </div>
        </div>
        <div class="flex items-center justify-center gap-2">
          <button class="btn-ghost px-2 py-1" @click="player.prev()"><Icon name="lucide:skip-back" size="18" /></button>
          <button class="btn-ghost px-2.5 py-1.5" @click="player.toggle()">
            <Icon :name="status === 'playing' || status === 'buffering' ? 'lucide:pause' : 'lucide:play'" size="20" />
          </button>
          <button class="btn-ghost px-2 py-1" @click="player.next()"><Icon name="lucide:skip-forward" size="18" /></button>
        </div>
      </div>
    </Teleport>
  </div>
</template>
