// Pure player-queue logic, kept free of Nuxt/DOM so vitest can cover every branch.
// The usePlayer composable wires this to the <video> element and the API.
import { naturalCompare } from '../naturalSort'

// MediaItem mirrors the server's mediaItemDTO (GET /files/{id}/media).
export interface MediaItem {
  node_id: string
  name: string
  mime: string
  size?: number
  version: number
  stream_url: string
  indexed: boolean
  title?: string
  artist?: string
  album?: string
  track?: number
  disc?: number
  duration?: number
  bitrate?: number
}

export interface QueueTrack extends MediaItem {
  // Whether this browser reports it can decode the mime (canPlayType). Unplayable
  // tracks stay visible in the queue (greyed) but are skipped by navigation.
  playable: boolean
}

export type RepeatMode = 'off' | 'all' | 'one'

export function isVideoMime(mime: string): boolean {
  return mime.startsWith('video/')
}

// buildQueue: the folder snapshot, natural-sorted by name — the same order the
// file browser shows (shared comparator, see naturalSort.ts).
export function buildQueue(items: MediaItem[], canPlay: (mime: string) => boolean): QueueTrack[] {
  return [...items]
    .sort((a, b) => naturalCompare(a.name, b.name))
    .map((it) => ({ ...it, playable: canPlay(it.mime) }))
}

export function indexOfTrack(queue: QueueTrack[], nodeId: string): number {
  return queue.findIndex((t) => t.node_id === nodeId)
}

// nextIndex returns the queue position to play after `current` ends or the user
// presses "next" — or null when playback should stop.
//   repeat=one + auto-advance → same track; an explicit "next" moves on.
//   Unplayable tracks are skipped; repeat=all wraps around once.
export function nextIndex(
  queue: QueueTrack[],
  current: number,
  repeat: RepeatMode,
  shuffle: boolean,
  opts: { manual?: boolean; random?: () => number } = {},
): number | null {
  if (!queue.length) return null
  if (repeat === 'one' && !opts.manual) return queue[current]?.playable ? current : null

  if (shuffle) {
    const candidates = queue.map((t, i) => (t.playable && i !== current ? i : -1)).filter((i) => i >= 0)
    if (!candidates.length) return queue[current]?.playable && repeat !== 'off' ? current : null
    const rnd = opts.random ?? Math.random
    return candidates[Math.floor(rnd() * candidates.length)]
  }

  for (let i = current + 1; i < queue.length; i++) {
    if (queue[i].playable) return i
  }
  if (repeat === 'all') {
    for (let i = 0; i <= current && i < queue.length; i++) {
      if (queue[i].playable) return i
    }
  }
  return null
}

// prevIndex: the closest playable track before `current`, or null (stay put).
export function prevIndex(queue: QueueTrack[], current: number): number | null {
  for (let i = current - 1; i >= 0; i--) {
    if (queue[i].playable) return i
  }
  return null
}

// --- Mid-playback error policy (spec §5.4) ---
// A media element error carries no HTTP status, so the composable probes the
// single-node media endpoint with the session and feeds the outcome here.
export type ProbeOutcome =
  | { ok: true; item: MediaItem }
  | { ok: false; status: number }

export type ErrorAction =
  | { kind: 'resume'; item: MediaItem } // fresh stream_url, restore position, retry once
  | { kind: 'skip' } // gone (or retry already spent) → toast + next track

export function errorAction(probe: ProbeOutcome, alreadyRetried: boolean): ErrorAction {
  if (!probe.ok || alreadyRetried) return { kind: 'skip' }
  return { kind: 'resume', item: probe.item }
}

// --- Persistence (localStorage) ---
export interface PersistedPlayer {
  parentId: string // folder the queue was snapshotted from ('' = root)
  items: MediaItem[]
  index: number
  position: number
  volume: number
  muted: boolean
  repeat: RepeatMode
  shuffle: boolean
}

const persistVersion = 1

export function serializePlayer(p: PersistedPlayer): string {
  return JSON.stringify({ v: persistVersion, ...p })
}

// restorePlayer parses a saved state, dropping anything malformed. Stale stream
// URLs inside items are fine: playback re-mints via the media endpoint anyway.
export function restorePlayer(raw: string | null): PersistedPlayer | null {
  if (!raw) return null
  try {
    const p = JSON.parse(raw)
    if (p?.v !== persistVersion || !Array.isArray(p.items) || !p.items.length) return null
    if (typeof p.index !== 'number' || p.index < 0 || p.index >= p.items.length) return null
    const items = p.items.filter((it: any) => it && typeof it.node_id === 'string' && typeof it.name === 'string')
    if (!items.length) return null
    return {
      parentId: typeof p.parentId === 'string' ? p.parentId : '',
      items,
      index: Math.min(p.index, items.length - 1),
      position: typeof p.position === 'number' && p.position >= 0 ? p.position : 0,
      volume: typeof p.volume === 'number' ? Math.min(1, Math.max(0, p.volume)) : 1,
      muted: !!p.muted,
      repeat: p.repeat === 'all' || p.repeat === 'one' ? p.repeat : 'off',
      shuffle: !!p.shuffle,
    }
  } catch {
    return null
  }
}

// resumePosition: a saved position only makes sense in the same file content.
// If the node's version moved on (file overwritten/restored), start from zero —
// second 172 of a different file is garbage.
export function resumePosition(savedPosition: number, savedVersion: number, currentVersion: number): number {
  return savedVersion === currentVersion ? savedPosition : 0
}
