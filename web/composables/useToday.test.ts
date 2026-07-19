import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { useToday } from './useToday'

describe('useToday', () => {
  beforeEach(() => { vi.useFakeTimers() })
  afterEach(() => { vi.useRealTimers() })

  it('rolls over to the new day after midnight', () => {
    vi.setSystemTime(new Date(2026, 6, 19, 23, 59)) // Sunday
    const { today, refresh } = useToday()
    expect(today.value.getDay()).toBe(0)
    vi.setSystemTime(new Date(2026, 6, 20, 0, 1)) // Monday
    refresh()
    expect(today.value.getDay()).toBe(1)
    expect(today.value.getDate()).toBe(20)
    expect(today.value.getHours()).toBe(0)
  })

  it('keeps the same value while the day is unchanged', () => {
    vi.setSystemTime(new Date(2026, 6, 20, 10, 0))
    const { today, refresh } = useToday()
    const before = today.value
    vi.setSystemTime(new Date(2026, 6, 20, 18, 30))
    refresh()
    expect(today.value).toBe(before)
  })
})
