import { describe, it, expect } from 'vitest'
import * as XLSX from 'xlsx'
import { openWorkbook, sliceAoa, SHEET_MAX_ROWS, SHEET_MAX_COLS } from './sheet'

function wbBytes(sheets: Record<string, unknown[][]>): ArrayBuffer {
  const wb = XLSX.utils.book_new()
  for (const [name, aoa] of Object.entries(sheets)) {
    XLSX.utils.book_append_sheet(wb, XLSX.utils.aoa_to_sheet(aoa), name)
  }
  return XLSX.write(wb, { type: 'array', bookType: 'xlsx' }) as ArrayBuffer
}

describe('openWorkbook', () => {
  it('reads sheet names and cell values as text', async () => {
    const view = await openWorkbook(
      wbBytes({ 'Лист1': [['Имя', 'Цена'], ['товар', 42.5]], Other: [['x']] }),
    )
    expect(view.names).toEqual(['Лист1', 'Other'])
    const s = view.slice('Лист1')
    expect(s.rows[0]).toEqual(['Имя', 'Цена'])
    expect(s.rows[1][0]).toBe('товар')
    expect(s.rows[1][1]).toBe('42.5')
    expect(s.truncated).toBe(false)
  })

  it('fills gaps with empty strings (no undefined cells)', async () => {
    const view = await openWorkbook(wbBytes({ S: [['a', , 'c'], ['d']] } as any))
    const s = view.slice('S')
    expect(s.rows[0]).toEqual(['a', '', 'c'])
  })

  it('returns an empty slice for an unknown sheet name', async () => {
    const view = await openWorkbook(wbBytes({ S: [['a']] }))
    expect(view.slice('nope').rows).toEqual([])
  })
})

describe('sliceAoa', () => {
  it('truncates rows beyond the cap and reports it', () => {
    const aoa = Array.from({ length: SHEET_MAX_ROWS + 100 }, (_, i) => [String(i)])
    const s = sliceAoa(aoa)
    expect(s.rows.length).toBe(SHEET_MAX_ROWS)
    expect(s.totalRows).toBe(SHEET_MAX_ROWS + 100)
    expect(s.truncated).toBe(true)
  })

  it('truncates columns beyond the cap', () => {
    const s = sliceAoa([Array.from({ length: SHEET_MAX_COLS + 10 }, (_, i) => i)])
    expect(s.rows[0].length).toBe(SHEET_MAX_COLS)
    expect(s.totalCols).toBe(SHEET_MAX_COLS + 10)
    expect(s.truncated).toBe(true)
  })

  it('does not flag small sheets as truncated', () => {
    expect(sliceAoa([['a', 'b']]).truncated).toBe(false)
  })
})
