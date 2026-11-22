import { describe as group, expect, it } from 'vitest'
import { describe, estimateTokens, logGroup, onColor, relativeTime } from './format'

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
