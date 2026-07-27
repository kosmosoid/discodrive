import { describe, it, expect } from 'vitest'
import { naturalCompare } from './naturalSort'

describe('naturalCompare', () => {
  // REGRESSION: files.vue switched from plain localeCompare to this comparator —
  // numeric names must order by value, which changes the visible listing order.
  it('sorts numbered tracks by value, not code point', () => {
    const names = ['10. Track.mp3', '2. Track.mp3', '1. Track.mp3']
    names.sort(naturalCompare)
    expect(names).toEqual(['1. Track.mp3', '2. Track.mp3', '10. Track.mp3'])
  })

  it('is case-insensitive', () => {
    expect(naturalCompare('abc', 'ABC')).toBe(0)
    const names = ['b.mp3', 'A.mp3', 'a.mp3']
    names.sort(naturalCompare)
    expect(names[0].toLowerCase()).toBe('a.mp3')
  })

  it('handles cyrillic names', () => {
    const names = ['Яблоко.mp3', 'Арбуз.mp3', 'Вишня.mp3']
    names.sort(naturalCompare)
    expect(names).toEqual(['Арбуз.mp3', 'Вишня.mp3', 'Яблоко.mp3'])
  })

  it('orders multi-digit disc/track patterns', () => {
    const names = ['CD10-01.flac', 'CD2-01.flac', 'CD2-02.flac']
    names.sort(naturalCompare)
    expect(names).toEqual(['CD2-01.flac', 'CD2-02.flac', 'CD10-01.flac'])
  })
})
