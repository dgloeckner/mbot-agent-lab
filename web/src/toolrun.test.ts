import { afterEach, describe, expect, it, vi } from 'vitest'
import { createSaver, freeName, idleRun, problemsByBlock, reduceRun, shouldReload } from './toolrun'
import type { ProgramEvent } from './types'

const ev = (e: Partial<ProgramEvent>): ProgramEvent => ({ run: 'r1', program: 'p1', phase: 'before', ...e }) as ProgramEvent

describe('reduceRun', () => {
  it('builds the Ablauf with rounds and marks the running block', () => {
    let v = reduceRun(idleRun, ev({ phase: 'start', name: 'quadrat' }))
    expect(v.running).toBe(true)
    v = reduceRun(v, ev({ phase: 'before', block: 'q_drive', round: 1 }))
    expect(v.marked).toBe('q_drive')
    v = reduceRun(v, ev({ phase: 'after', block: 'q_drive', kind: 'call', tool: 'drive', args: { cm: 30 }, result: { ok: true, reason: 'done', driven_cm: 30 }, round: 1 }))
    v = reduceRun(v, ev({ phase: 'before', block: 'q_turn', round: 1 }))
    v = reduceRun(v, ev({ phase: 'before', block: 'q_drive', round: 2 }))
    expect(v.lines.map((l) => l.kind + ':' + l.text)).toEqual(['round:Runde 1', 'step:drive(cm: 30) → fertig, 30 cm gefahren', 'round:Runde 2'])
    v = reduceRun(v, ev({ phase: 'end', result: { ok: true, end: 'done', steps: 8, runtime_s: 15.9 } }))
    expect(v.running).toBe(false)
    expect(v.marked).toBeNull()
    expect(v.lines.at(-1)).toMatchObject({ kind: 'end', text: 'Werkzeug quadrat: fertig nach 8 Schritten, 15,9 s', failed: false })
  })
  it('marks where an aborted run stopped and ignores older runs', () => {
    let v = reduceRun(idleRun, ev({ phase: 'start', name: 'labyrinth', step: true }))
    v = reduceRun(v, ev({ phase: 'wait', block: 'l_drive' }))
    expect(v).toMatchObject({ waiting: true, marked: 'l_drive', singleStep: true })
    expect(reduceRun(v, ev({ run: 'old', phase: 'before', block: 'x' }))).toBe(v)
    v = reduceRun(v, ev({ phase: 'end', result: { ok: false, end: 'aborted', steps: 1, runtime_s: 2, block: 'l_drive', error: 'drive_until endete unerwartet mit obstacle.' } }))
    expect(v.marked).toBe('l_drive')
    expect(v.lines.at(-1)?.failed).toBe(true)
  })
  it('words a sensor reading', () => {
    const v = reduceRun(reduceRun(idleRun, ev({ phase: 'start' })), ev({ phase: 'after', kind: 'sense', block: 'c', result: { distance_cm: 12, floor: 'wwww' } }))
    expect(v.lines[0].text).toBe('Sensoren gelesen: Abstand 12 cm, Boden wwww')
  })
})

describe('createSaver', () => {
  afterEach(() => vi.useRealTimers())
  it('saves once, a while after the last change', async () => {
    vi.useFakeTimers()
    const saved: number[] = []
    const s = createSaver<number>(async (v) => void saved.push(v), 1000)
    s.schedule(1)
    vi.advanceTimersByTime(500)
    s.schedule(2)
    vi.advanceTimersByTime(900)
    expect(saved).toEqual([])
    expect(s.pending()).toBe(true)
    await vi.advanceTimersByTimeAsync(200)
    expect(saved).toEqual([2])
    expect(s.pending()).toBe(false)
  })
  it('flushes a pending change at once', async () => {
    vi.useFakeTimers()
    const saved: number[] = []
    const s = createSaver<number>(async (v) => void saved.push(v), 1000)
    s.schedule(3)
    await s.flush()
    expect(saved).toEqual([3])
  })
})

describe('team tool helpers', () => {
  it('reloads the open tool only for another PC’s save', () => {
    expect(shouldReload('p1', 'p1', '2026-01-01T00:00:00Z', '2026-01-01T00:00:05Z')).toBe(true)
    expect(shouldReload('p1', 'p1', '2026-01-01T00:00:05Z', '2026-01-01T00:00:05Z')).toBe(false)
    expect(shouldReload('p1', 'p2', null, 'x')).toBe(false)
    expect(shouldReload(null, 'p1', null, 'x')).toBe(false)
  })
  it('turns problems into warnings per block', () => {
    expect(problemsByBlock([{ block: 'd', text: 'zu weit' }, { block: 'd', text: 'noch was' }, { text: 'Name fehlt' }])).toEqual({
      byBlock: { d: ['zu weit', 'noch was'] },
      general: ['Name fehlt'],
    })
  })
  it('finds a free default name', () => {
    expect(freeName(['werkzeug_1', 'quadrat'])).toBe('werkzeug_2')
  })
})
