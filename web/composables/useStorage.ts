// Refresh signal for the storage numbers in the sidebar.
//
// Bumped centrally by useApi() after every mutating request rather than by each page
// that deletes or restores something: a counter nobody remembers to update is a counter
// that lies, and "I emptied the trash and nothing changed" is exactly how that shows up.
export const useStorageTick = () => useState<number>('storageTick', () => 0)
