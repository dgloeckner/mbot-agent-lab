import { describe, expect, it } from 'vitest'
import { argsOf, fieldsOf, floorSegments, keyFields, resultSentence, traceEnd } from './console'
import type { JSONSchema } from './types'

const driveUntil: JSONSchema = {
  type: 'object',
  properties: {
    stop_on_color: { type: 'string', enum: ['white', 'blue'] },
    max_cm: { type: 'number', minimum: 1, maximum: 300 },
    stop_below_cm: { type: 'number', minimum: 3, maximum: 100 },
    robot: { type: 'string', enum: ['rot', 'blau'] },
  },
  required: ['max_cm', 'robot'],
}

describe('fieldsOf', () => {
  it('builds one field per argument, required first, without robot', () => {
    const f = fieldsOf(driveUntil)
    expect(f.map((x) => x.name)).toEqual(['max_cm', 'stop_on_color', 'stop_below_cm'])
    expect(f[0]).toMatchObject({ kind: 'number', required: true, min: 1, max: 300 })
    expect(f[1]).toMatchObject({ kind: 'enum', required: false, options: ['white', 'blue'] })
  })
  it('knows text and boolean fields', () => {
    const f = fieldsOf({ properties: { text: { type: 'string', maxLength: 60 }, stop_at_crossing: { type: 'boolean', description: 'Standard true.' } } })
    expect(f).toEqual([
      { name: 'text', kind: 'text', required: false, maxLength: 60 },
      { name: 'stop_at_crossing', kind: 'boolean', required: false, hint: 'Standard true.' },
    ])
  })
  it('has no fields without a schema', () => {
    expect(fieldsOf(undefined)).toEqual([])
  })
})

describe('argsOf', () => {
  const f = fieldsOf(driveUntil)
  it('types the values and leaves out empty optional fields', () => {
    expect(argsOf(f, { max_cm: '100', stop_below_cm: '', stop_on_color: 'blue' })).toEqual({ max_cm: 100, stop_on_color: 'blue' })
  })
  it('reads a decimal comma and adds the picked robot', () => {
    expect(argsOf(f, { max_cm: '12,5' }, 'blau')).toEqual({ max_cm: 12.5, robot: 'blau' })
  })
  it('sends a wrong number as text, so the server explains it', () => {
    expect(argsOf(f, { max_cm: 'weit' })).toEqual({ max_cm: 'weit' })
  })
  it('turns booleans into true and false', () => {
    const b = fieldsOf({ properties: { stop_at_crossing: { type: 'boolean' } } })
    expect(argsOf(b, { stop_at_crossing: 'false' })).toEqual({ stop_at_crossing: false })
  })
})

describe('resultSentence', () => {
  it('words a result like the protocol', () => {
    expect(resultSentence({ ok: true, reason: 'wall', driven_cm: 42, distance_cm: 11 })).toBe('Wand erreicht, 42 cm gefahren, Abstand 11 cm.')
    expect(resultSentence({ ok: false, error: 'rot ist nicht verbunden.' })).toBe('Rot ist nicht verbunden.')
  })
  it('lists the key fields', () => {
    expect(keyFields({ ok: true, reason: 'wall', driven_cm: 42.04, floor: [] })).toEqual([
      ['reason', 'wall (Wand erreicht)'],
      ['driven_cm', '42'],
    ])
  })
})

describe('floorSegments', () => {
  it('lets each entry hold until the next and the last until the end', () => {
    const r = { driven_cm: 42, floor: [[0, 'wwww'], [18, 'wkkw'], [20, 'wwww']] }
    expect(floorSegments(r.floor, traceEnd(r))).toEqual([
      { from: 0, to: 18, pattern: 'wwww' },
      { from: 18, to: 20, pattern: 'wkkw' },
      { from: 20, to: 42, pattern: 'wwww' },
    ])
  })
  it('uses degrees for a turn and ignores what is not a trace', () => {
    expect(floorSegments([[0, 'wwww']], traceEnd({ turned_deg: -90 }))).toEqual([{ from: 0, to: 90, pattern: 'wwww' }])
    expect(floorSegments(undefined)).toEqual([])
    expect(floorSegments([['x', 1]])).toEqual([])
  })
})
