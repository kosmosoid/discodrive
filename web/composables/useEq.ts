// Web Audio equalizer wired to the window's media element. Key constraints:
//
// - createMediaElementSource can be called ONCE per element, ever — after that
//   ALL audio routes through the graph. So the graph is created lazily on the
//   first enable/play, and "disabled" means zeroed filter gains, not teardown.
// - An AudioContext created without a user gesture starts suspended and would
//   MUTE the element (its output is captured by the silent graph). We therefore
//   only build the graph inside user-initiated paths (enable click, play event)
//   and re-try ctx.resume() on every play.
// - Each window (main tab, pop-out player) runs its own element and thus its
//   own graph; settings are shared through localStorage and follow the other
//   window live via the 'storage' event.
import {
  EQ_BANDS, EQ_PRESETS, clampGain, defaultEqSettings, serializeEq, restoreEq,
} from '~/lib/player/eq'

const EQ_KEY = 'kf_eq'

let el: HTMLMediaElement | null = null
let ctx: AudioContext | null = null
let filters: BiquadFilterNode[] = []
let listenersBound = false

export function useEq() {
  const enabled = useState<boolean>('eq.enabled', () => false)
  const preset = useState<string>('eq.preset', () => 'flat')
  const gains = useState<number[]>('eq.gains', () => [...EQ_PRESETS.flat])

  function applyGains() {
    for (let i = 0; i < filters.length; i++) {
      filters[i].gain.value = enabled.value ? clampGain(gains.value[i] ?? 0) : 0
    }
  }

  async function ensureGraph() {
    if (!el || !import.meta.client) return
    if (!ctx) {
      const AC: typeof AudioContext = window.AudioContext || (window as any).webkitAudioContext
      if (!AC) return
      ctx = new AC()
      const source = ctx.createMediaElementSource(el)
      filters = EQ_BANDS.map((freq, i) => {
        const f = ctx!.createBiquadFilter()
        f.type = i === 0 ? 'lowshelf' : i === EQ_BANDS.length - 1 ? 'highshelf' : 'peaking'
        f.frequency.value = freq
        if (f.type === 'peaking') f.Q.value = 1.0
        return f
      })
      let prev: AudioNode = source
      for (const f of filters) {
        prev.connect(f)
        prev = f
      }
      prev.connect(ctx.destination)
    }
    applyGains()
    if (ctx.state === 'suspended') {
      try { await ctx.resume() } catch { /* no user activation yet */ }
    }
  }

  function persist() {
    if (!import.meta.client) return
    localStorage.setItem(EQ_KEY, serializeEq({
      enabled: enabled.value, preset: preset.value, gains: [...gains.value],
    }))
  }

  // attach is called once per window, right after player.attachMedia.
  function attach(mediaEl: HTMLMediaElement) {
    el = mediaEl
    if (!import.meta.client || listenersBound) return
    listenersBound = true
    const saved = restoreEq(localStorage.getItem(EQ_KEY)) ?? defaultEqSettings()
    enabled.value = saved.enabled
    preset.value = saved.preset
    gains.value = [...saved.gains]

    // Every play is a chance to (re)build/resume the graph under user activation.
    mediaEl.addEventListener('play', () => {
      if (enabled.value) void ensureGraph()
    })
    // Follow settings changed in the other window. Only touch an EXISTING graph:
    // building one from a storage event (no gesture) could suspend-mute playback.
    window.addEventListener('storage', (e) => {
      if (e.key !== EQ_KEY) return
      const s = restoreEq(e.newValue)
      if (!s) return
      enabled.value = s.enabled
      preset.value = s.preset
      gains.value = [...s.gains]
      if (ctx) applyGains()
    })
  }

  async function setEnabled(v: boolean) {
    enabled.value = v
    if (v) await ensureGraph() // user gesture path — safe to create the context
    else applyGains()
    persist()
  }

  function setGain(i: number, v: number) {
    const next = [...gains.value]
    next[i] = clampGain(v)
    gains.value = next
    preset.value = 'custom'
    applyGains()
    persist()
  }

  function applyPreset(name: string) {
    const p = EQ_PRESETS[name]
    if (!p) return
    preset.value = name
    gains.value = [...p]
    applyGains()
    persist()
  }

  return { enabled, preset, gains, attach, setEnabled, setGain, applyPreset }
}
