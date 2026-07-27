// Equalizer core: band layout, presets and settings (de)serialization — pure,
// unit-tested. The Web Audio graph itself lives in composables/useEq.ts.

// Classic 10-band layout (ISO octave centers). The first band is a low shelf,
// the last a high shelf, the middle ones peaking filters (see useEq).
export const EQ_BANDS = [31, 62, 125, 250, 500, 1000, 2000, 4000, 8000, 16000] as const

export const EQ_GAIN_MIN = -12
export const EQ_GAIN_MAX = 12

export interface EqSettings {
  enabled: boolean
  // A key of EQ_PRESETS, or 'custom' once the user moves any slider.
  preset: string
  gains: number[] // dB per band, EQ_BANDS.length entries
}

export const EQ_PRESETS: Record<string, readonly number[]> = {
  flat:       [0, 0, 0, 0, 0, 0, 0, 0, 0, 0],
  rock:       [5, 4, 3, 1, -1, -1, 1, 3, 4, 5],
  pop:        [-1, 2, 3, 4, 3, 1, 0, -1, -1, -1],
  jazz:       [3, 2, 1, 2, -2, -2, 0, 1, 2, 3],
  classical:  [4, 3, 2, 1, -1, -1, 0, 2, 3, 4],
  bass:       [6, 5, 4, 2, 1, 0, 0, 0, 0, 0],
  treble:     [0, 0, 0, 0, 0, 1, 2, 4, 5, 6],
  vocal:      [-2, -1, 0, 2, 4, 4, 3, 1, 0, -1],
  electronic: [4, 3, 1, 0, -1, 1, 0, 1, 3, 4],
}

export function clampGain(v: number): number {
  if (!Number.isFinite(v)) return 0
  return Math.min(EQ_GAIN_MAX, Math.max(EQ_GAIN_MIN, v))
}

export function defaultEqSettings(): EqSettings {
  return { enabled: false, preset: 'flat', gains: [...EQ_PRESETS.flat] }
}

const eqVersion = 1

export function serializeEq(s: EqSettings): string {
  return JSON.stringify({ v: eqVersion, enabled: s.enabled, preset: s.preset, gains: s.gains })
}

// restoreEq parses saved settings, falling back to defaults on anything malformed.
export function restoreEq(raw: string | null): EqSettings | null {
  if (!raw) return null
  try {
    const p = JSON.parse(raw)
    if (p?.v !== eqVersion || !Array.isArray(p.gains) || p.gains.length !== EQ_BANDS.length) return null
    const preset = typeof p.preset === 'string' && (p.preset === 'custom' || p.preset in EQ_PRESETS)
      ? p.preset
      : 'custom'
    return {
      enabled: !!p.enabled,
      preset,
      gains: p.gains.map((g: unknown) => clampGain(typeof g === 'number' ? g : 0)),
    }
  } catch {
    return null
  }
}
