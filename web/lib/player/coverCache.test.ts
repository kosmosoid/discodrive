import { describe, it, expect, vi } from 'vitest'
import { CoverCache } from './coverCache'

function makeCache(capacity: number) {
  const fetched: string[] = []
  const revoked: string[] = []
  let n = 0
  const cache = new CoverCache(
    async (id) => {
      fetched.push(id)
      if (id.startsWith('none')) return null
      return {} as Blob
    },
    capacity,
    () => `url-${++n}`,
    (u) => revoked.push(u),
  )
  return { cache, fetched, revoked }
}

describe('CoverCache', () => {
  it('caches: same node fetched once', async () => {
    const { cache, fetched } = makeCache(5)
    const a1 = await cache.get('a')
    const a2 = await cache.get('a')
    expect(a1).toBe(a2)
    expect(fetched).toEqual(['a'])
  })

  it('caches negative results too (no re-fetch of coverless tracks)', async () => {
    const { cache, fetched } = makeCache(5)
    expect(await cache.get('none-1')).toBeNull()
    expect(await cache.get('none-1')).toBeNull()
    expect(fetched).toEqual(['none-1'])
  })

  it('evicts the least recently used and revokes its object URL', async () => {
    const { cache, fetched, revoked } = makeCache(2)
    await cache.get('a') // url-1
    await cache.get('b') // url-2
    await cache.get('a') // refresh a → b is now oldest
    await cache.get('c') // url-3 → evicts b (url-2)
    expect(revoked).toEqual(['url-2'])
    await cache.get('a') // still cached
    expect(fetched).toEqual(['a', 'b', 'c'])
  })

  it('concurrent gets share one fetch', async () => {
    const { cache, fetched } = makeCache(5)
    const [x, y] = await Promise.all([cache.get('a'), cache.get('a')])
    expect(x).toBe(y)
    expect(fetched).toEqual(['a'])
  })

  it('fetch errors degrade to null without caching a broken URL', async () => {
    const boom = new CoverCache(async () => { throw new Error('net') }, 5, () => 'u', () => {})
    expect(await boom.get('a')).toBeNull()
  })

  it('clear revokes everything', async () => {
    const { cache, revoked } = makeCache(5)
    await cache.get('a')
    await cache.get('b')
    cache.clear()
    expect(revoked.sort()).toEqual(['url-1', 'url-2'])
  })
})
