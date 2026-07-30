import { describe, it, expect } from 'vitest'
import {
  buildQueue, indexOfTrack, nextIndex, prevIndex, errorAction,
  serializePlayer, restorePlayer, resumePosition, isVideoMime,
  type MediaItem, type QueueTrack,
} from './core'

function item(name: string, mime = 'audio/mpeg', extra: Partial<MediaItem> = {}): MediaItem {
  return { node_id: 'id-' + name, name, mime, version: 1, stream_url: '/s', indexed: false, ...extra }
}

const playAll = () => true
const playAudioOnly = (m: string) => m.startsWith('audio/')

describe('buildQueue', () => {
  it('natural-sorts by name and marks playability', () => {
    const q = buildQueue(
      [item('10.mp3'), item('2.mp3'), item('clip.mkv', 'video/x-matroska')],
      playAudioOnly,
    )
    expect(q.map((t) => t.name)).toEqual(['2.mp3', '10.mp3', 'clip.mkv'])
    expect(q.map((t) => t.playable)).toEqual([true, true, false])
  })
})

describe('nextIndex', () => {
  const q: QueueTrack[] = buildQueue(
    [item('1.mp3'), item('2.mkv', 'video/x-matroska'), item('3.mp3'), item('4.mp3')],
    playAudioOnly,
  ) // playable: [0], skip [1], [2], [3]

  it('advances and skips unplayable tracks', () => {
    expect(nextIndex(q, 0, 'off', false)).toBe(2)
    expect(nextIndex(q, 2, 'off', false)).toBe(3)
  })

  it('stops at the end with repeat off', () => {
    expect(nextIndex(q, 3, 'off', false)).toBeNull()
  })

  it('wraps with repeat all', () => {
    expect(nextIndex(q, 3, 'all', false)).toBe(0)
  })

  it('repeat one repeats on auto-advance but moves on manual next', () => {
    expect(nextIndex(q, 2, 'one', false)).toBe(2)
    expect(nextIndex(q, 2, 'one', false, { manual: true })).toBe(3)
  })

  it('shuffle picks a playable track that is not the current one', () => {
    const got = nextIndex(q, 0, 'off', true, { random: () => 0.99 })
    expect([2, 3]).toContain(got)
    // deterministic: random() → 0 picks the first candidate
    expect(nextIndex(q, 0, 'off', true, { random: () => 0 })).toBe(2)
  })

  it('empty queue → null', () => {
    expect(nextIndex([], 0, 'all', false)).toBeNull()
  })

  it('all-unplayable queue → null instead of an infinite skip loop', () => {
    const dead = buildQueue([item('a.mkv', 'video/x-matroska'), item('b.mkv', 'video/x-matroska')], playAudioOnly)
    expect(nextIndex(dead, 0, 'all', false)).toBeNull()
    expect(nextIndex(dead, 0, 'all', true)).toBeNull()
  })
})

describe('prevIndex', () => {
  const q = buildQueue([item('1.mp3'), item('2.mkv', 'video/x-matroska'), item('3.mp3')], playAudioOnly)
  it('skips unplayable going backwards', () => {
    expect(prevIndex(q, 2, )).toBe(0)
  })
  it('null at the beginning', () => {
    expect(prevIndex(q, 0)).toBeNull()
  })
})

describe('errorAction (mid-playback policy)', () => {
  it('probe ok → refresh URL and resume, once', () => {
    const fresh = item('x.mp3')
    expect(errorAction({ ok: true, item: fresh }, false)).toEqual({ kind: 'resume', item: fresh })
  })
  it('second failure on the same track → skip (no retry storm)', () => {
    expect(errorAction({ ok: true, item: item('x.mp3') }, true)).toEqual({ kind: 'skip' })
  })
  it('probe 404 (file gone) → skip', () => {
    expect(errorAction({ ok: false, status: 404 }, false)).toEqual({ kind: 'skip' })
  })
})

describe('persistence', () => {
  const owner = 'alice@example.com'
  const state = {
    owner,
    parentId: 'p1',
    items: [item('a.mp3'), item('b.mp3')],
    index: 1,
    position: 42.5,
    volume: 0.7,
    muted: false,
    repeat: 'all' as const,
    shuffle: true,
  }
  // Restored items always come back with stream_url: '' — URLs are re-minted.
  const restoredState = { ...state, items: state.items.map((it) => ({ ...it, stream_url: '' })) }

  it('round-trips (minus stream URLs)', () => {
    expect(restorePlayer(serializePlayer(state), owner)).toEqual(restoredState)
  })

  it('never writes stream URLs: they embed bearer tokens that outlive the session', () => {
    expect(serializePlayer(state)).not.toContain('stream_url')
  })

  it('rejects a snapshot stamped for another account', () => {
    expect(restorePlayer(serializePlayer(state), 'bob@example.com')).toBeNull()
    expect(restorePlayer(serializePlayer(state), '')).toBeNull()
    // An unstamped snapshot never matches, even an empty expected owner.
    expect(restorePlayer(JSON.stringify({ ...state, v: 2, owner: '' }), '')).toBeNull()
  })

  it('rejects pre-v2 snapshots (unstamped, may carry stream URLs)', () => {
    const { owner: _o, ...v1 } = state
    expect(restorePlayer(JSON.stringify({ v: 1, ...v1 }), owner)).toBeNull()
  })

  it('rejects garbage, empty and out-of-range states', () => {
    expect(restorePlayer(null, owner)).toBeNull()
    expect(restorePlayer('not json', owner)).toBeNull()
    expect(restorePlayer('{"v":2,"items":[]}', owner)).toBeNull()
    expect(restorePlayer(JSON.stringify({ v: 999, ...state }), owner)).toBeNull()
    expect(restorePlayer(JSON.stringify({ ...state, v: 2, index: 5 }), owner)).toBeNull()
  })

  it('drops malformed items and clamps the index', () => {
    const raw = JSON.stringify({ v: 2, ...state, items: [state.items[0], { junk: true }], index: 1 })
    const got = restorePlayer(raw, owner)
    expect(got?.items).toHaveLength(1)
    expect(got?.index).toBe(0)
  })

  it('resume position survives only for the same file version', () => {
    expect(resumePosition(42, 3, 3)).toBe(42)
    // file was overwritten → second 42 of a different file is garbage
    expect(resumePosition(42, 3, 4)).toBe(0)
  })
})

describe('isVideoMime', () => {
  it('classifies', () => {
    expect(isVideoMime('video/mp4')).toBe(true)
    expect(isVideoMime('audio/flac')).toBe(false)
  })
})
