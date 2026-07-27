// Player engine: one shared <video> element (audio plays fine on it, and a single
// element is the anchor for the future EQ and for Document PiP — it never leaves
// the main window). Queue/persistence/error-policy logic lives in ~/lib/player/core
// (pure, unit-tested); this composable wires it to the DOM and the API.
import { computed, ref, watch } from 'vue'
import {
  buildQueue, indexOfTrack, nextIndex, prevIndex, errorAction,
  serializePlayer, restorePlayer, resumePosition, isVideoMime,
  type MediaItem, type QueueTrack, type RepeatMode,
} from '~/lib/player/core'
import { CoverCache } from '~/lib/player/coverCache'

export type PlayerStatus = 'idle' | 'playing' | 'paused' | 'buffering'

const PERSIST_KEY = 'kf_player'
const PERSIST_INTERVAL_MS = 3000

// Module-level singletons: the SPA has exactly one player. The media element is
// deliberately NOT reactive state — it is DOM owned by PlayerLayer.vue.
let media: HTMLVideoElement | null = null
let coverCache: CoverCache | null = null
let persistTimer: ReturnType<typeof setTimeout> | null = null
let retriedCurrent = false // one URL-refresh retry per track (spec §5.4)
let restored = false

function useQueueState() {
  return {
    queue: useState<QueueTrack[]>('player.queue', () => []),
    index: useState<number>('player.index', () => -1),
    parentId: useState<string>('player.parent', () => ''),
    status: useState<PlayerStatus>('player.status', () => 'idle'),
    position: useState<number>('player.position', () => 0),
    duration: useState<number>('player.duration', () => 0),
    volume: useState<number>('player.volume', () => 1),
    muted: useState<boolean>('player.muted', () => false),
    repeat: useState<RepeatMode>('player.repeat', () => 'off'),
    shuffle: useState<boolean>('player.shuffle', () => false),
    theater: useState<boolean>('player.theater', () => false),
    coverUrl: useState<string | null>('player.cover', () => null),
    // Transient in-bar notice ("skipped: <name>"), cleared automatically.
    notice: useState<string>('player.notice', () => ''),
  }
}

// Server-side mime detection (Go mime.TypeByExtension) emits a few legacy names
// that canPlayType does not recognise — normalise before probing.
const mimeAliases: Record<string, string> = {
  'audio/mp4a-latm': 'audio/mp4', // .m4a
  'audio/x-m4a': 'audio/mp4',
  'audio/x-flac': 'audio/flac',
  'audio/x-wav': 'audio/wav',
}

// canPlayType probe on a detached element; '' means "definitely not".
let probeEl: HTMLVideoElement | null = null
export function browserCanPlay(mime: string): boolean {
  if (!mime || !(mime.startsWith('audio/') || mime.startsWith('video/'))) return false
  if (!import.meta.client) return false
  if (!probeEl) probeEl = document.createElement('video')
  return probeEl.canPlayType(mimeAliases[mime] ?? mime) !== ''
}

export function usePlayer() {
  const st = useQueueState()
  const sess = useSession()
  const { t } = useI18n()

  const current = computed<QueueTrack | null>(() => st.queue.value[st.index.value] ?? null)
  const currentIsVideo = computed(() => !!current.value && isVideoMime(current.value.mime))
  const active = computed(() => st.status.value !== 'idle' && !!current.value)

  function authHeaders(): Record<string, string> {
    return sess.value.token ? { Authorization: `Bearer ${sess.value.token}` } : {}
  }

  function mediaEndpoint(parentId: string): string {
    return parentId ? `/files/${parentId}/media` : '/files/media'
  }

  if (!coverCache) {
    coverCache = new CoverCache(async (nodeId) => {
      try {
        return await apiFetch<Blob>(`/files/${nodeId}/media-cover`, {
          responseType: 'blob', headers: authHeaders(),
        })
      } catch {
        return null
      }
    })
  }

  // ---- persistence ----
  function persistSoon() {
    if (!import.meta.client || persistTimer) return
    persistTimer = setTimeout(() => {
      persistTimer = null
      if (!st.queue.value.length) return
      localStorage.setItem(PERSIST_KEY, serializePlayer({
        parentId: st.parentId.value,
        items: st.queue.value.map(({ playable, ...it }) => it),
        index: st.index.value,
        position: st.position.value,
        volume: st.volume.value,
        muted: st.muted.value,
        repeat: st.repeat.value,
        shuffle: st.shuffle.value,
      }))
    }, PERSIST_INTERVAL_MS)
  }

  function restoreOnce() {
    if (restored || !import.meta.client) return
    restored = true
    const saved = restorePlayer(localStorage.getItem(PERSIST_KEY))
    if (!saved) return
    st.queue.value = buildQueue(saved.items, browserCanPlay)
    // buildQueue re-sorts; find the saved track again by id.
    const savedId = saved.items[saved.index]?.node_id
    st.index.value = savedId ? Math.max(0, indexOfTrack(st.queue.value, savedId)) : 0
    st.parentId.value = saved.parentId
    st.position.value = saved.position
    st.volume.value = saved.volume
    st.muted.value = saved.muted
    st.repeat.value = saved.repeat
    st.shuffle.value = saved.shuffle
    st.status.value = 'paused' // browsers block autoplay after reload anyway
    void updateSessionMetadata()
  }

  // ---- media element wiring (called once by PlayerLayer) ----
  function attachMedia(el: HTMLVideoElement) {
    media = el
    el.volume = st.volume.value
    el.muted = st.muted.value

    el.addEventListener('timeupdate', () => {
      st.position.value = el.currentTime
      persistSoon()
    })
    el.addEventListener('durationchange', () => { st.duration.value = el.duration || 0 })
    el.addEventListener('playing', () => {
      st.status.value = 'playing'
      retriedCurrent = false
    })
    el.addEventListener('pause', () => {
      if (st.status.value === 'playing') st.status.value = 'paused'
    })
    el.addEventListener('waiting', () => {
      if (st.status.value === 'playing') st.status.value = 'buffering'
    })
    el.addEventListener('stalled', () => {
      if (st.status.value === 'playing') st.status.value = 'buffering'
    })
    el.addEventListener('ended', () => { advance(false) })
    el.addEventListener('error', () => { void handleMediaError() })

    setupMediaSession()
    restoreOnce()
  }

  function showNotice(msg: string) {
    st.notice.value = msg
    setTimeout(() => { if (st.notice.value === msg) st.notice.value = '' }, 4000)
  }

  // ---- core playback ----
  async function loadAndPlay(fromPosition = 0) {
    const track = current.value
    if (!media || !track) return
    if (!track.playable) {
      showNotice(t('player.skipped_unplayable', { name: track.name }))
      advance(false)
      return
    }
    st.duration.value = track.duration ?? 0
    st.position.value = fromPosition
    media.src = track.stream_url
    if (fromPosition > 0) media.currentTime = fromPosition
    try {
      await media.play()
    } catch {
      // Autoplay refused (e.g. right after restore) — stay paused at position.
      st.status.value = 'paused'
    }
    void updateSessionMetadata()
    persistSoon()
  }

  function advance(manual: boolean) {
    const ni = nextIndex(st.queue.value, st.index.value, st.repeat.value, st.shuffle.value, { manual })
    if (ni === null) {
      st.status.value = 'paused'
      if (media) media.pause()
      return
    }
    st.index.value = ni
    void loadAndPlay(0)
  }

  // Media element errors carry no HTTP status → probe the single-node endpoint
  // with the session. Alive → fresh stream_url (+version check for resume), retry
  // once. Gone/forbidden → skip. The probe deliberately bypasses useApi: its 401
  // handler force-logouts, which is wrong for a background stream hiccup.
  async function handleMediaError() {
    const track = current.value
    if (!media || !track || !media.error) return
    const savedPos = st.position.value
    let probe: { ok: true; item: MediaItem } | { ok: false; status: number }
    try {
      const item = await apiFetch<MediaItem>(mediaEndpoint(st.parentId.value), {
        query: { node_id: track.node_id }, headers: authHeaders(),
      })
      probe = { ok: true, item }
    } catch (e: any) {
      probe = { ok: false, status: e?.response?.status ?? 0 }
    }
    const action = errorAction(probe, retriedCurrent)
    if (action.kind === 'resume') {
      retriedCurrent = true
      const fresh = action.item
      st.queue.value.splice(st.index.value, 1, { ...track, ...fresh, playable: track.playable })
      await loadAndPlay(resumePosition(savedPos, track.version, fresh.version))
      return
    }
    showNotice(t('player.skipped_missing', { name: track.name }))
    advance(false)
  }

  // ---- public controls ----
  async function playFolder(parentId: string, clickedNodeId: string) {
    const { request } = useApi()
    const resp = await request<{ items: MediaItem[] }>(mediaEndpoint(parentId))
    const queue = buildQueue(resp.items, browserCanPlay)
    if (!queue.length) return
    st.parentId.value = parentId
    st.queue.value = queue
    st.index.value = Math.max(0, indexOfTrack(queue, clickedNodeId))
    retriedCurrent = false
    await loadAndPlay(0)
  }

  async function playAt(i: number) {
    if (i < 0 || i >= st.queue.value.length) return
    st.index.value = i
    retriedCurrent = false
    await loadAndPlay(0)
  }

  async function toggle() {
    if (!media || !current.value) return
    if (st.status.value === 'playing' || st.status.value === 'buffering') {
      media.pause()
      return
    }
    // After a reload the element has no src yet — (re)load at the saved position.
    if (!media.src) {
      await loadAndPlay(st.position.value)
      return
    }
    try { await media.play() } catch { /* autoplay policy */ }
  }

  function next() { advance(true) }
  function prev() {
    // Convention: >3s into the track "prev" restarts it, otherwise goes back.
    if (media && media.currentTime > 3) {
      media.currentTime = 0
      return
    }
    const pi = prevIndex(st.queue.value, st.index.value)
    if (pi !== null) void playAt(pi)
    else if (media) media.currentTime = 0
  }

  function seek(sec: number) {
    if (!media) return
    media.currentTime = Math.max(0, Math.min(sec, st.duration.value || sec))
    st.position.value = media.currentTime
  }

  function setVolume(v: number) {
    st.volume.value = Math.min(1, Math.max(0, v))
    if (media) media.volume = st.volume.value
    if (st.volume.value > 0 && st.muted.value) toggleMute()
    persistSoon()
  }

  function toggleMute() {
    st.muted.value = !st.muted.value
    if (media) media.muted = st.muted.value
    persistSoon()
  }

  function cycleRepeat() {
    st.repeat.value = st.repeat.value === 'off' ? 'all' : st.repeat.value === 'all' ? 'one' : 'off'
    persistSoon()
  }

  function toggleShuffle() {
    st.shuffle.value = !st.shuffle.value
    persistSoon()
  }

  function close() {
    if (media) {
      media.pause()
      media.removeAttribute('src')
      media.load()
    }
    st.status.value = 'idle'
    st.theater.value = false
    st.queue.value = []
    st.index.value = -1
    st.coverUrl.value = null
    if (import.meta.client) localStorage.removeItem(PERSIST_KEY)
  }

  // ---- Media Session (OS media keys, lock screen) ----
  function setupMediaSession() {
    if (!import.meta.client || !('mediaSession' in navigator)) return
    const ms = navigator.mediaSession
    ms.setActionHandler('play', () => { void toggle() })
    ms.setActionHandler('pause', () => { void toggle() })
    ms.setActionHandler('previoustrack', () => prev())
    ms.setActionHandler('nexttrack', () => next())
    try {
      ms.setActionHandler('seekto', (d) => { if (d.seekTime != null) seek(d.seekTime) })
    } catch { /* seekto unsupported */ }
  }

  async function updateSessionMetadata() {
    const track = current.value
    st.coverUrl.value = track ? await coverCache!.get(track.node_id) : null
    if (!import.meta.client || !('mediaSession' in navigator) || !track) return
    navigator.mediaSession.metadata = new MediaMetadata({
      title: track.title || track.name,
      artist: track.artist || '',
      album: track.album || '',
      artwork: st.coverUrl.value ? [{ src: st.coverUrl.value }] : [],
    })
  }

  return {
    // state
    queue: st.queue, index: st.index, status: st.status, position: st.position,
    duration: st.duration, volume: st.volume, muted: st.muted, repeat: st.repeat,
    shuffle: st.shuffle, theater: st.theater, coverUrl: st.coverUrl, notice: st.notice,
    current, currentIsVideo, active,
    // wiring + controls
    attachMedia, playFolder, playAt, toggle, next, prev, seek,
    setVolume, toggleMute, cycleRepeat, toggleShuffle, close,
  }
}
