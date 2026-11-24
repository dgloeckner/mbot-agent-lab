// The console in the "Werkzeuge" tab: forms built from a tool's JSON schema,
// the arguments they send, and how a result reads. Pure functions; the
// server checks every rule, the browser only shows and sends.

import { reasonName, resultText } from './format'
import type { JSONSchema } from './types'

export type FieldKind = 'number' | 'enum' | 'text' | 'boolean'

export interface Field {
  name: string
  kind: FieldKind
  required: boolean
  options?: string[]
  min?: number
  max?: number
  maxLength?: number
  /** The tool's own words about the field, e.g. "Standard 40." */
  hint?: string
}

/** The form of a tool: one field per argument, required ones first. The robot comes from the picker. */
export function fieldsOf(schema: JSONSchema | undefined): Field[] {
  const props = schema?.properties ?? {}
  const required = new Set(schema?.required ?? [])
  const fields: Field[] = []
  for (const [name, p] of Object.entries(props)) {
    if (name === 'robot') continue
    const f: Field = { name, kind: 'text', required: required.has(name) }
    if (p.enum) {
      f.kind = 'enum'
      f.options = [...p.enum]
    } else if (p.type === 'number' || p.type === 'integer') {
      f.kind = 'number'
      f.min = p.minimum
      f.max = p.maximum
    } else if (p.type === 'boolean') {
      f.kind = 'boolean'
    } else {
      f.maxLength = p.maxLength
    }
    if (p.description) f.hint = p.description
    fields.push(f)
  }
  return fields.sort((a, b) => Number(b.required) - Number(a.required))
}

/** Form values (strings, as inputs hold them) to the arguments of a call. Empty optional fields are left out. */
export function argsOf(fields: Field[], values: Record<string, string>, robot?: string): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const f of fields) {
    const raw = (values[f.name] ?? '').trim()
    if (raw === '') continue
    switch (f.kind) {
      case 'number': {
        const n = Number(raw.replace(',', '.'))
        out[f.name] = Number.isFinite(n) ? n : raw // the server explains a wrong value
        break
      }
      case 'boolean':
        out[f.name] = raw === 'true'
        break
      default:
        out[f.name] = raw
    }
  }
  if (robot) out.robot = robot
  return out
}

/** One sentence for a tool result, in the words of the protocol. */
export function resultSentence(result: Record<string, any> | null | undefined): string {
  if (!result) return ''
  const text = resultText(result)
  const s = text.charAt(0).toUpperCase() + text.slice(1)
  return /[.!?]$/.test(s) ? s : s + '.'
}

/** The key fields of a result, shown under the sentence. */
export function keyFields(result: Record<string, any> | null | undefined): [string, string][] {
  if (!result) return []
  const out: [string, string][] = []
  if (typeof result.reason === 'string' && result.reason) out.push(['reason', `${result.reason} (${reasonName(result.reason)})`])
  for (const k of ['driven_cm', 'turned_deg', 'distance_cm', 'min_distance_cm', 'best_deg', 'best_cm', 'color']) {
    const v = result[k]
    if (typeof v === 'number') out.push([k, String(Math.round(v * 10) / 10)])
    else if (typeof v === 'string' && v) out.push([k, v])
  }
  return out
}

export interface BandSegment {
  from: number
  to: number
  /** Four letters, one per floor probe from left to right. */
  pattern: string
}

/** The floor trace as segments for a color band. A trace entry holds until the next; the last until `end`. */
export function floorSegments(floor: unknown, end?: number): BandSegment[] {
  if (!Array.isArray(floor)) return []
  const entries = floor.filter((e): e is [number, string] => Array.isArray(e) && typeof e[0] === 'number' && typeof e[1] === 'string')
  if (!entries.length) return []
  const last = entries[entries.length - 1][0]
  const stop = Math.max(last, Math.abs(end ?? last))
  return entries.map(([from, pattern], i) => ({ from, to: i + 1 < entries.length ? entries[i + 1][0] : stop, pattern }))
}

/** Where a motion ended, for the band's length: cm driven or degrees turned. */
export function traceEnd(result: Record<string, any>): number | undefined {
  if (typeof result.driven_cm === 'number') return result.driven_cm
  if (typeof result.turned_deg === 'number') return Math.abs(result.turned_deg)
  return undefined
}

/** Fill colors of the floor letters (w weiß, k schwarz, r rot, y gelb, g grün, b blau, ? unklar). */
export const FLOOR_FILL: Record<string, string> = {
  w: '#ffffff', k: '#1b2230', r: '#e5383b', y: '#f2b705', g: '#1f9d55', b: '#2b6cee', '?': '#aab2be',
}
