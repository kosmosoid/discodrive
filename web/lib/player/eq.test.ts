import { describe, it, expect } from 'vitest'
import {
  EQ_BANDS, EQ_PRESETS, EQ_GAIN_MIN, EQ_GAIN_MAX,
  clampGain, defaultEqSettings, serializeEq, restoreEq,
} from './eq'

describe('eq presets', () => {
  it('every preset covers every band within the gain range', () => {
    for (const [name, gains] of Object.entries(EQ_PRESETS)) {
      expect(gains, name).toHaveLength(EQ_BANDS.length)
      for (const g of gains) {
        expect(g, name).toBeGreaterThanOrEqual(EQ_GAIN_MIN)
        expect(g, name).toBeLessThanOrEqual(EQ_GAIN_MAX)
      }
    }
  })

  it('flat preset is all zeros', () => {
    expect(EQ_PRESETS.flat.every((g) => g === 0)).toBe(true)
  })
})

describe('clampGain', () => {
  it('clamps to the slider range and swallows non-numbers', () => {
    expect(clampGain(99)).toBe(EQ_GAIN_MAX)
    expect(clampGain(-99)).toBe(EQ_GAIN_MIN)
    expect(clampGain(3.5)).toBe(3.5)
    expect(clampGain(NaN)).toBe(0)
    expect(clampGain(Infinity)).toBe(0)
  })
})

describe('eq settings persistence', () => {
  it('round-trips', () => {
    const s = { enabled: true, preset: 'rock', gains: [...EQ_PRESETS.rock] }
    expect(restoreEq(serializeEq(s))).toEqual(s)
  })

  it('defaults are valid and round-trip too', () => {
    const d = defaultEqSettings()
    expect(restoreEq(serializeEq(d))).toEqual(d)
  })

  it('rejects garbage and wrong band counts', () => {
    expect(restoreEq(null)).toBeNull()
    expect(restoreEq('nope')).toBeNull()
    expect(restoreEq(JSON.stringify({ v: 1, enabled: true, preset: 'flat', gains: [0, 0] }))).toBeNull()
    expect(restoreEq(JSON.stringify({ v: 99, enabled: true, preset: 'flat', gains: EQ_PRESETS.flat }))).toBeNull()
  })

  it('clamps out-of-range gains and downgrades unknown presets to custom', () => {
    const raw = JSON.stringify({ v: 1, enabled: true, preset: 'megabass', gains: [99, -99, 1, 0, 0, 0, 0, 0, 0, 'x'] })
    const got = restoreEq(raw)
    expect(got?.preset).toBe('custom')
    expect(got?.gains[0]).toBe(EQ_GAIN_MAX)
    expect(got?.gains[1]).toBe(EQ_GAIN_MIN)
    expect(got?.gains[9]).toBe(0)
  })
})
