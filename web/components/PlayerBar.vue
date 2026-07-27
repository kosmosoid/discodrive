<script setup lang="ts">
// Bottom player bar: transport controls, seek, volume, queue popup. All state
// lives in usePlayer(); the <video> element itself is owned by PlayerLayer.
const { t } = useI18n()
const player = usePlayer()
const {
  queue, index, status, position, duration, volume, muted, repeat, shuffle,
  theater, coverUrl, notice, current, currentIsVideo,
} = player

const emit = defineEmits<{ (e: 'detach'): void }>()
const props = defineProps<{ canDetach: boolean }>()

const queueOpen = ref(false)

const playing = computed(() => status.value === 'playing' || status.value === 'buffering')
const repeatIcon = computed(() => (repeat.value === 'one' ? 'lucide:repeat-1' : 'lucide:repeat'))

function onSeek(e: Event) {
  player.seek(parseFloat((e.target as HTMLInputElement).value))
}
function onVolume(e: Event) {
  player.setVolume(parseFloat((e.target as HTMLInputElement).value))
}
function pickTrack(i: number) {
  queueOpen.value = false
  void player.playAt(i)
}
useModalEscape(computed(() => queueOpen.value), () => { queueOpen.value = false })
</script>

<template>
  <div class="relative border-t border-line bg-panel/95 backdrop-blur">
    <!-- transient notice (skipped track etc.) -->
    <div
      v-if="notice"
      class="pointer-events-none absolute -top-8 left-1/2 -translate-x-1/2 rounded-md bg-ink/80 px-3 py-1 text-xs text-panel"
    >{{ notice }}</div>

    <!-- queue popup -->
    <div
      v-if="queueOpen"
      class="absolute bottom-full right-2 z-30 mb-2 max-h-72 w-80 overflow-auto rounded-xl border border-line bg-panel p-1 shadow-lg"
    >
      <button
        v-for="(tr, i) in queue"
        :key="tr.node_id"
        class="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-xs"
        :class="[
          i === index ? 'bg-accent/15 text-accent' : tr.playable ? 'hover:bg-ink/5' : '',
          !tr.playable ? 'cursor-default opacity-40' : '',
        ]"
        :title="tr.playable ? tr.name : t('player.unplayable')"
        @click="tr.playable && pickTrack(i)"
      >
        <Icon :name="i === index && playing ? 'lucide:volume-2' : tr.mime.startsWith('video/') ? 'lucide:film' : 'lucide:music'" size="14" class="shrink-0" />
        <span class="min-w-0 flex-1 truncate">{{ tr.title || tr.name }}</span>
        <span v-if="tr.duration" class="shrink-0 text-muted">{{ formatTime(tr.duration) }}</span>
      </button>
    </div>

    <div class="flex items-center gap-3 px-3 py-2">
      <!-- cover + titles -->
      <div class="flex min-w-0 flex-1 items-center gap-2.5 md:w-56 md:flex-none">
        <div class="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-md bg-ink/10">
          <img v-if="coverUrl" :src="coverUrl" class="h-full w-full object-cover" alt="" />
          <Icon v-else :name="currentIsVideo ? 'lucide:film' : 'lucide:music'" size="18" class="text-muted" />
        </div>
        <div class="min-w-0">
          <div class="truncate text-sm text-ink">{{ current?.title || current?.name }}</div>
          <div class="truncate text-xs text-muted">
            {{ current?.artist || '' }}<template v-if="current?.artist && current?.album"> · </template>{{ current?.album || '' }}
          </div>
        </div>
      </div>

      <!-- transport + seek -->
      <div class="flex min-w-0 flex-[2] flex-col items-center gap-1">
        <div class="flex items-center gap-1">
          <button class="btn-ghost hidden px-2 py-1 md:inline-flex" :class="shuffle ? 'text-accent' : ''" :title="t('player.shuffle')" @click="player.toggleShuffle()">
            <Icon name="lucide:shuffle" size="16" />
          </button>
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
          <button class="btn-ghost hidden px-2 py-1 md:inline-flex" :class="repeat !== 'off' ? 'text-accent' : ''" :title="t('player.repeat')" @click="player.cycleRepeat()">
            <Icon :name="repeatIcon" size="16" />
          </button>
        </div>
        <div class="hidden w-full max-w-xl items-center gap-2 text-[11px] text-muted md:flex">
          <span class="w-10 text-right tabular-nums">{{ formatTime(position) }}</span>
          <input
            type="range" min="0" :max="duration || 0" step="0.1" :value="position"
            class="h-1 w-full accent-accent"
            :disabled="!duration"
            @input="onSeek"
          />
          <span class="w-10 tabular-nums">{{ formatTime(duration) }}</span>
        </div>
      </div>

      <!-- right controls -->
      <div class="flex shrink-0 items-center gap-1">
        <button
          v-if="currentIsVideo"
          class="btn-ghost px-2 py-1"
          :class="theater ? 'text-accent' : ''"
          :title="theater ? t('player.collapse_video') : t('player.expand_video')"
          @click="theater = !theater"
        >
          <Icon name="lucide:film" size="16" />
        </button>
        <div class="hidden items-center gap-1 lg:flex">
          <button class="btn-ghost px-2 py-1" :title="muted ? t('player.unmute') : t('player.mute')" @click="player.toggleMute()">
            <Icon :name="muted || volume === 0 ? 'lucide:volume-x' : volume < 0.5 ? 'lucide:volume-1' : 'lucide:volume-2'" size="16" />
          </button>
          <input
            type="range" min="0" max="1" step="0.01" :value="muted ? 0 : volume"
            class="h-1 w-20 accent-accent"
            @input="onVolume"
          />
        </div>
        <button class="btn-ghost px-2 py-1" :class="queueOpen ? 'text-accent' : ''" :title="t('player.queue')" @click="queueOpen = !queueOpen">
          <Icon name="lucide:list-music" size="16" />
        </button>
        <button v-if="props.canDetach" class="btn-ghost hidden px-2 py-1 md:inline-flex" :title="t('player.detach')" @click="emit('detach')">
          <Icon name="lucide:picture-in-picture-2" size="16" />
        </button>
        <button class="btn-ghost px-2 py-1" :title="t('player.close')" @click="player.close()">
          <Icon name="lucide:x" size="16" />
        </button>
      </div>
    </div>
  </div>
</template>
