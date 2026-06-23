// Size formatting: bytes → human-readable (KB/MB/GB).
export function formatBytes(n: number | null | undefined): string {
  if (n == null) return '—'
  if (n === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(n) / Math.log(1024))
  const v = n / Math.pow(1024, i)
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

// GB ↔ bytes conversion for quota fields (empty = no limit).
export function gbToBytes(gb: string): number | null {
  const v = parseFloat(gb)
  return Number.isFinite(v) && v > 0 ? Math.round(v * 1024 ** 3) : null
}
export function bytesToGb(n: number | null | undefined): string {
  return n == null ? '' : (n / 1024 ** 3).toFixed(n % 1024 ** 3 === 0 ? 0 : 2)
}
