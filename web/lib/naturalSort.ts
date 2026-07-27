// Natural ("2" before "10") case-insensitive name comparison. The file browser
// table and the player queue MUST sort with the same comparator: the queue is a
// snapshot of the folder the user is looking at, and "plays in the order I see"
// only holds while the two lists agree.
const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

export function naturalCompare(a: string, b: string): number {
  return collator.compare(a, b)
}
