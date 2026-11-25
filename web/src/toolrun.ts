// Team tools on the page: the "Ablauf" built from SSE "program" events,
// saving a few seconds after the last change, and problems per block. Pure
// functions; the server runs everything, the browser shows and sends.

import { describeTool, programEnd } from './format'
import type { ProgramEvent, ProgramProblem, ProgramResult } from './types'

export interface StepLine {
  kind: 'step' | 'round' | 'end'
  text: string
  block?: string
  failed?: boolean
}

/** What the page shows of the team's running (or last) team tool. */
export interface RunView {
  run: string | null
  program: string | null
  name: string
  running: boolean
  /** Single step: the run waits before the marked block for "Weiter". */
  waiting: boolean
  singleStep: boolean
  /** The block that runs (or ran last, or where it stopped). */
  marked: string | null
  lines: StepLine[]
  result: ProgramResult | null
  lastRound: number
}

export const idleRun: RunView = {
  run: null, program: null, name: '', running: false, waiting: false, singleStep: false, marked: null, lines: [], result: null, lastRound: 0,
}

/** Applies one SSE "program" event. Events of an older run are ignored. */
export function reduceRun(v: RunView, e: ProgramEvent): RunView {
  if (e.phase === 'start') {
    return { ...idleRun, run: e.run, program: e.program, name: e.name ?? '', running: true, singleStep: Boolean(e.step) }
  }
  if (e.run !== v.run) return v
  switch (e.phase) {
    case 'wait':
      return { ...v, waiting: true, marked: e.block ?? v.marked }
    case 'before': {
      const lines = [...v.lines]
      let lastRound = v.lastRound
      if (e.round && e.round !== v.lastRound) {
        lines.push({ kind: 'round', text: `Runde ${e.round}` })
        lastRound = e.round
      }
      return { ...v, waiting: false, marked: e.block ?? null, lines, lastRound }
    }
    case 'after':
      return { ...v, lines: [...v.lines, stepLine(e)] }
    case 'end': {
      const r = e.result as ProgramResult
      return {
        ...v, running: false, waiting: false, result: r, marked: r.end === 'done' && !r.value ? null : (r.block ?? v.marked),
        lines: [...v.lines, { kind: 'end', text: programEnd(v.name, r), failed: r.end !== 'done', block: r.block }],
      }
    }
  }
  return v
}

/** One line of the "Ablauf": the step with its result in words. */
export function stepLine(e: ProgramEvent): StepLine {
  if (e.kind === 'sense') {
    const r = e.result ?? {}
    const parts = []
    if (typeof r.distance_cm === 'number') parts.push(`Abstand ${r.distance_cm} cm`)
    if (r.floor) parts.push(`Boden ${r.floor}`)
    if (r.color) parts.push(`Farbe ${r.color}`)
    return { kind: 'step', block: e.block, text: `Sensoren gelesen: ${parts.join(', ')}` }
  }
  const t = describeTool({ name: e.tool, args: e.args, result: e.result })
  return { kind: 'step', block: e.block, text: `${t.call} → ${t.outcome}`, failed: t.failed }
}

/** Problems per block, per input field (name, description), and the rest about the whole tool. */
export function problemsByBlock(problems: ProgramProblem[]): {
  byBlock: Record<string, string[]>
  byField: Record<string, string[]>
  general: string[]
} {
  const byBlock: Record<string, string[]> = {}
  const byField: Record<string, string[]> = {}
  const general: string[] = []
  for (const p of problems) {
    if (p.block) (byBlock[p.block] ??= []).push(p.text)
    else if (p.field) (byField[p.field] ??= []).push(p.text)
    else general.push(p.text)
  }
  return { byBlock, byField, general }
}

/**
 * Saves a value some time after the last change. Last write wins on the
 * server; the saver only spares it a request per keystroke.
 */
export function createSaver<T>(save: (value: T) => Promise<void>, delayMs = 1500) {
  let timer: ReturnType<typeof setTimeout> | undefined
  let latest: { value: T } | null = null
  const run = async () => {
    timer = undefined
    if (!latest) return
    const { value } = latest
    latest = null
    await save(value)
  }
  return {
    schedule(value: T) {
      latest = { value }
      if (timer !== undefined) clearTimeout(timer)
      timer = setTimeout(() => void run(), delayMs)
    },
    /** Saves a pending change now. */
    async flush() {
      if (timer !== undefined) clearTimeout(timer)
      await run()
    },
    pending: () => latest !== null,
    cancel() {
      if (timer !== undefined) clearTimeout(timer)
      timer = undefined
      latest = null
    },
  }
}

/**
 * What a team may type as a tool name: lower-case letters, digits and _, at
 * most 30 (the server's rule). Capitals become small, umlauts are spelled
 * out, spaces and dashes become _, everything else is dropped.
 */
export function toolName(raw: string): string {
  const spelled = raw.toLowerCase().replace(/[äöüß]/g, (c) => ({ ä: 'ae', ö: 'oe', ü: 'ue', ß: 'ss' })[c] ?? '')
  return spelled.replace(/[\s-]/g, '_').replace(/[^a-z0-9_]/g, '').slice(0, 30)
}

/** A free default name for a new team tool: werkzeug_1, werkzeug_2, … */
export function freeName(taken: string[]): string {
  for (let i = 1; ; i++) if (!taken.includes(`werkzeug_${i}`)) return `werkzeug_${i}`
}

/** Whether a "programs" event should reload the open tool: it is that tool and not our own save. */
export function shouldReload(openId: string | null, eventId: string, ownSaveAt: string | null, serverAt: string): boolean {
  return openId !== null && openId === eventId && serverAt !== ownSaveAt
}
