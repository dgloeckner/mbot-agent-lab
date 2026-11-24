import { describe as group, expect, it } from 'vitest'
import { describe, estimateTokens, programEnd, formatCost, formatPrices, formatUsage, logGroup, onColor, relativeTime } from './format'

group('describe', () => {
  it('words robot events in German', () => {
    expect(describe({ id: 1, at: '', source: 'robot', kind: 'obstacle_near', data: { distance_cm: 8 } })).toBe('Hindernis in 8 cm')
    expect(describe({ id: 1, at: '', source: 'robot', kind: 'skill_aborted', data: { skill: 'drive', reason: 'obstacle' } })).toBe(
      'drive abgebrochen: Hindernis im Weg',
    )
    expect(describe({ id: 1, at: '', source: 'robot', kind: 'color_changed', data: { color: 'blue' } })).toBe('Boden ist jetzt blau')
  })
  it('falls back to free text', () => {
    expect(describe({ id: 1, at: '', source: 'team', kind: 'chat', text: 'Hallo' })).toBe('Hallo')
  })
})

it('groups log sources', () => {
  expect(logGroup({ id: 1, at: '', source: 'agent:blau', kind: 'chat' })).toBe('agent')
  expect(logGroup({ id: 1, at: '', source: 'system', kind: 'info' })).toBe('system')
})

it('picks readable text on team colors', () => {
  expect(onColor('#F2B705')).toBe('#1B2230')
  expect(onColor('#2B6CEE')).toBe('#FFFFFF')
})

it('estimates tokens and relative times', () => {
  expect(estimateTokens('')).toBe(0)
  expect(estimateTokens('x'.repeat(36))).toBe(10)
  expect(relativeTime(10_000, 12_000)).toBe('gerade eben')
  expect(relativeTime(0, 125_000)).toBe('vor 2 min')
})

it('words tool calls and runs', () => {
  const tool = { id: 1, at: '', source: 'agent', kind: 'tool', data: { name: 'drive_until', args: { max_cm: 120, robot: 'Scout' }, result: { ok: true, reason: 'wall', driven_cm: 41.6, distance_cm: 14 } } }
  expect(describe(tool)).toBe('drive_until(max_cm: 120, robot: Scout) → Wand erreicht, 42 cm gefahren, Abstand 14 cm')
  const failed = { ...tool, data: { name: 'turn', args: {}, result: { ok: false, error: 'Es fehlt das Argument "degrees".' } } }
  expect(describe(failed)).toBe('turn() → Es fehlt das Argument "degrees".')
  expect(describe({ id: 2, at: '', source: 'system', kind: 'run_end', data: { reason: 'stop', by: 'Team' } })).toBe('Lauf gestoppt von euch')
  expect(formatCost(0)).toBe('0 ct')
  expect(formatCost(50)).toBe('unter 0,01 ct')
  expect(formatCost(12_500)).toBe('1,25 ct')
})

it('formats model prices', () => {
  expect(formatPrices(0.1, 0.5)).toBe('0,1 / 0,5 € pro Mio. Token')
  expect(formatPrices(4, 20)).toBe('4 / 20 € pro Mio. Token')
  expect(formatPrices(0, 0)).toBe('ohne Kosten')
  expect(formatUsage(67_476, 109_800)).toBe('67.476 Token, 10,98 ct')
})

it('words goal tools', () => {
  const scan = { id: 3, at: '', source: 'agent', kind: 'tool', data: { name: 'scan', args: {}, result: { ok: true, reason: 'done', turned_deg: 90, best_deg: 90, best_cm: 190, distance_cm: 188 } } }
  expect(describe(scan)).toBe('scan() → fertig, 90° gedreht, Abstand 188 cm, am freiesten bei 90° (190 cm)')
  const clear = { ...scan, data: { name: 'turn_until_clear', args: { direction: 'right' }, result: { ok: true, reason: 'clear', turned_deg: 75, distance_cm: 46 } } }
  expect(describe(clear)).toBe('turn_until_clear(direction: right) → Weg frei, 75° gedreht, Abstand 46 cm')
  const trip = { ...scan, data: { name: 'drive', args: { cm: 30 }, result: { ok: true, reason: 'done', driven_cm: 30, floor: [[0, 'wkkw'], [2.1, 'rkkr'], [4, 'wkkw']] } } }
  expect(describe(trip)).toBe('drive(cm: 30) → fertig, 30 cm gefahren, Bodenspur wkkw rkkr wkkw')
  const cut = { ...trip, data: { ...trip.data, result: { ...trip.data.result, floor_truncated: true } } }
  expect(describe(cut)).toContain('(abgeschnitten)')
})

group('team tool lines', () => {
  it('words the start and the end of a team tool', () => {
    expect(describe({ id: 1, at: '', source: 'team', kind: 'program_start', data: { name: 'quadrat', robot: 'rot', step: true } } as any)).toBe(
      'Werkzeug quadrat gestartet im Einzelschritt mit rot',
    )
    expect(programEnd('quadrat', { end: 'done', steps: 8, runtime_s: 15.9 })).toBe('Werkzeug quadrat: fertig nach 8 Schritten, 15,9 s')
    expect(programEnd('labyrinth', { end: 'aborted', steps: 1, error: 'drive endete unerwartet mit obstacle.', at: 'fahre 50 cm' })).toBe(
      'Werkzeug labyrinth: abgebrochen nach 1 Schritt. drive endete unerwartet mit obstacle. (bei „fahre 50 cm“)',
    )
  })
})

