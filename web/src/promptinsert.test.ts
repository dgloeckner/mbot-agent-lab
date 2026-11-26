import { describe, expect, it } from 'vitest'
import { planInsert } from './promptinsert'

const apply = (doc: string, sel: { from: number; to: number } | null) => {
  const p = planInsert(doc, '`drive`', sel)
  const out = doc.slice(0, p.from) + p.insert + doc.slice(p.to)
  expect(out.slice(p.start, p.end)).toBe('`drive`')
  return out
}

describe('planInsert', () => {
  it('appends on a new line when the team has not placed the cursor', () => {
    expect(apply('Du steuerst einen Roboter.', null)).toBe('Du steuerst einen Roboter.\n`drive`')
    expect(apply('Erste Zeile\n', null)).toBe('Erste Zeile\n`drive`')
    expect(apply('', null)).toBe('`drive`')
  })

  it('moves a cursor inside a word to its end and adds the spaces', () => {
    expect(apply('Fahre mit dann weiter', { from: 3, to: 3 })).toBe('Fahre `drive` mit dann weiter')
    expect(apply('Nutze dann', { from: 6, to: 6 })).toBe('Nutze `drive` dann')
  })

  it('at a line end adds only the space before', () => {
    const doc = 'Nutze\nzweite Zeile'
    expect(apply(doc, { from: 5, to: 5 })).toBe('Nutze `drive`\nzweite Zeile')
  })

  it('needs no space before punctuation or after an opening bracket', () => {
    expect(apply('Nutze .', { from: 6, to: 6 })).toBe('Nutze `drive`.')
    expect(apply('Nutze ()', { from: 7, to: 7 })).toBe('Nutze (`drive`)')
  })

  it('replaces a selection', () => {
    expect(apply('Nutze turn jetzt', { from: 6, to: 10 })).toBe('Nutze `drive` jetzt')
  })
})
