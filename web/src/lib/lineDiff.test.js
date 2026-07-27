import { describe, expect, it } from 'vitest'
import { computeLineDiff, computeWriteRows } from './lineDiff.js'

const types = rows => rows.map(r => r.type).join('')

describe('computeLineDiff', () => {
  it('marks a single replaced line as one del + one add', () => {
    const d = computeLineDiff('a\nb\nc', 'a\nB\nc')
    expect(d.removed).toBe(1)
    expect(d.added).toBe(1)
    const del = d.rows.find(r => r.type === 'del')
    const add = d.rows.find(r => r.type === 'add')
    expect(del.text).toBe('b')
    expect(add.text).toBe('B')
    expect(del.oldNo).toBe(2)
    expect(add.newNo).toBe(2)
  })

  it('detects pure insertion without phantom deletions', () => {
    const d = computeLineDiff('a\nc', 'a\nb\nc')
    expect(d.added).toBe(1)
    expect(d.removed).toBe(0)
    expect(types(d.rows)).toBe('ctxaddctx')
  })

  it('detects pure deletion', () => {
    const d = computeLineDiff('a\nb\nc', 'a\nc')
    expect(d.added).toBe(0)
    expect(d.removed).toBe(1)
    expect(d.rows.find(r => r.type === 'del').text).toBe('b')
  })

  it('keeps every row when nothing changed', () => {
    const d = computeLineDiff('a\nb', 'a\nb')
    expect(d.added + d.removed).toBe(0)
    expect(types(d.rows)).toBe('ctxctx')
  })

  it('collapses far-away context into a gap row', () => {
    const old = Array.from({ length: 40 }, (_, i) => `L${i}`).join('\n')
    const next = old.replace('L20', 'L20-changed')
    const d = computeLineDiff(old, next, { context: 2 })
    const gaps = d.rows.filter(r => r.type === 'gap')
    expect(gaps.length).toBe(2)
    expect(gaps[0].count).toBeGreaterThan(10)
    expect(d.rows.filter(r => r.type === 'ctx').length).toBe(4)
  })

  it('normalises CRLF so line endings alone are not a diff', () => {
    const d = computeLineDiff('a\r\nb', 'a\nb')
    expect(d.added + d.removed).toBe(0)
  })

  it('falls back to coarse mode on huge inputs but stays consistent', () => {
    const old = Array.from({ length: 1500 }, (_, i) => `x${i}`).join('\n')
    const next = old.replace('x700', 'x700!')
    const d = computeLineDiff(old, next, { context: 1 })
    expect(d.truncated).toBe(true)
    expect(d.added).toBe(1)
    expect(d.removed).toBe(1)
  })

  it('treats empty old content as all additions', () => {
    const d = computeLineDiff('', 'a\nb')
    expect(d.added).toBe(2)
    expect(d.removed).toBe(0)
  })
})

describe('computeWriteRows', () => {
  it('numbers every line as an addition', () => {
    const d = computeWriteRows('one\ntwo')
    expect(d.added).toBe(2)
    expect(d.rows[1]).toMatchObject({ type: 'add', text: 'two', newNo: 2 })
  })

  it('returns nothing for empty content', () => {
    expect(computeWriteRows('').rows).toEqual([])
  })
})
