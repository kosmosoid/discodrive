// Lazy SheetJS reader for spreadsheet previews (.xlsx/.xls/.ods). Security
// model: we never touch HTML from the file — SheetJS gives us cell values
// (formatted text, incl. cached formula results) and the component renders its
// own <table> through Vue's normal escaping. Output is sliced to a fixed
// rows×cols window so a million-row sheet can't hang the tab.

export const SHEET_MAX_ROWS = 500
export const SHEET_MAX_COLS = 60

export interface SheetSlice {
  rows: string[][]
  totalRows: number
  totalCols: number
  truncated: boolean
}

export interface WorkbookView {
  names: string[]
  slice: (name: string) => SheetSlice
}

export async function openWorkbook(data: ArrayBuffer): Promise<WorkbookView> {
  const XLSX = await import('xlsx')
  const wb = XLSX.read(data, { type: 'array' })
  return {
    names: wb.SheetNames,
    slice: (name: string) => {
      const ws = wb.Sheets[name]
      // raw:false → formatted cell text (dates, number formats, cached formula values)
      const aoa: unknown[][] = ws
        ? (XLSX.utils.sheet_to_json(ws, { header: 1, raw: false, defval: '' }) as unknown[][])
        : []
      return sliceAoa(aoa)
    },
  }
}

export function sliceAoa(aoa: unknown[][]): SheetSlice {
  const totalRows = aoa.length
  const totalCols = aoa.reduce<number>((m, r) => Math.max(m, r.length), 0)
  const rows = aoa
    .slice(0, SHEET_MAX_ROWS)
    .map((r) => r.slice(0, SHEET_MAX_COLS).map((c) => (c == null ? '' : String(c))))
  return {
    rows,
    totalRows,
    totalCols,
    truncated: totalRows > SHEET_MAX_ROWS || totalCols > SHEET_MAX_COLS,
  }
}
