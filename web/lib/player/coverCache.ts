// LRU cache of cover-art object URLs. Object URLs pin their blob in memory until
// revoked; a player tab that lives for days with prev/next would otherwise grow
// without bound. Capacity is small — covers are only shown one at a time.
export class CoverCache {
  private urls = new Map<string, string | null>() // nodeId → objectURL (null = known to have no cover)
  private pending = new Map<string, Promise<string | null>>()

  constructor(
    private fetchBlob: (nodeId: string) => Promise<Blob | null>,
    private capacity = 20,
    private makeURL: (b: Blob) => string = (b) => URL.createObjectURL(b),
    private revokeURL: (u: string) => void = (u) => URL.revokeObjectURL(u),
  ) {}

  // get returns an object URL for the node's cover, or null when it has none.
  // Concurrent calls for the same node share one fetch.
  async get(nodeId: string): Promise<string | null> {
    if (this.urls.has(nodeId)) {
      const url = this.urls.get(nodeId)!
      // refresh LRU position
      this.urls.delete(nodeId)
      this.urls.set(nodeId, url)
      return url
    }
    const inflight = this.pending.get(nodeId)
    if (inflight) return inflight
    const p = this.load(nodeId)
    this.pending.set(nodeId, p)
    try {
      return await p
    } finally {
      this.pending.delete(nodeId)
    }
  }

  private async load(nodeId: string): Promise<string | null> {
    let url: string | null = null
    try {
      const blob = await this.fetchBlob(nodeId)
      url = blob ? this.makeURL(blob) : null
    } catch {
      url = null
    }
    this.urls.set(nodeId, url)
    this.evict()
    return url
  }

  private evict() {
    while (this.urls.size > this.capacity) {
      const [oldest, url] = this.urls.entries().next().value as [string, string | null]
      this.urls.delete(oldest)
      if (url) this.revokeURL(url)
    }
  }

  clear() {
    for (const url of this.urls.values()) {
      if (url) this.revokeURL(url)
    }
    this.urls.clear()
  }
}
