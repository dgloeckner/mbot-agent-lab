// German wording for everything the team page shows.

import type { LogEntry } from './types'

const COLORS: Record<string, string> = {
  white: 'weiß', black: 'schwarz', red: 'rot', green: 'grün', blue: 'blau', yellow: 'gelb',
  purple: 'lila', cyan: 'türkis', orange: 'orange', none: 'keine',
}

/** Hex values for robot and floor colors, matching the robot's LED palette. */
export const COLOR_HEX: Record<string, string> = {
  red: '#E5383B', blue: '#2B6CEE', green: '#1F9D55', yellow: '#F2B705', purple: '#8A4FFF',
  cyan: '#12B5CB', orange: '#F26B1D', white: '#FFFFFF', black: '#1B2230',
}

const REASONS: Record<string, string> = {
  done: 'fertig', wall: 'Wand erreicht', color: 'Farbe erreicht', max_cm: 'Strecke erreicht',
  timeout: 'Zeit abgelaufen', stopped: 'gestoppt', obstacle: 'Hindernis im Weg', busy: 'noch beschäftigt',
  tilted: 'umgekippt', reassigned: 'neu zugeteilt', error: 'Fehler',
  clear: 'Weg frei', max_deg: 'kein freier Weg gefunden', line_lost: 'Linie zu Ende', crossing: 'Kreuzung erreicht',
}

export const colorName = (c: string) => COLORS[c] ?? c
export const reasonName = (r: string) => REASONS[r] ?? r

export function skillName(skill: string): string {
  return skill === 'idle' || !skill ? 'steht' : skill
}

/** Rough token estimate. Real counts depend on the model's tokenizer. */
export function estimateTokens(text: string): number {
  return Math.ceil(text.length / 3.6)
}

const num = new Intl.NumberFormat('de-DE')
export const formatNumber = (n: number) => num.format(n)

export function clock(at: string | number | Date): string {
  return new Date(at).toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

export function relativeTime(at: number | string, now = Date.now()): string {
  const s = Math.round((now - new Date(at).getTime()) / 1000)
  if (s < 5) return 'gerade eben'
  if (s < 60) return `vor ${s} s`
  const m = Math.round(s / 60)
  if (m < 60) return `vor ${m} min`
  const d = new Date(at)
  const today = new Date(now).toDateString() === d.toDateString()
  return today
    ? `um ${d.toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' })}`
    : d.toLocaleString('de-DE', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}

const RUN_END: Record<string, string> = {
  finish: 'Der Agent hat den Lauf beendet',
  stop: 'Lauf gestoppt',
  restart: 'Lauf für den Neustart beendet',
  time_limit: 'Lauf beendet: Zeitlimit erreicht',
  token_limit: 'Lauf beendet: Token-Limit erreicht',
  team_deleted: 'Lauf beendet: Team gelöscht',
  server_stop: 'Lauf beendet: Server wurde angehalten',
  server_restart: 'Lauf beendet: Server wurde neu gestartet',
}

/** "max_cm: 120, robot: Scout" from tool arguments. */
function compactArgs(args: unknown): string {
  if (!args || typeof args !== 'object') return ''
  return Object.entries(args as Record<string, unknown>)
    .map(([k, v]) => `${k}: ${typeof v === 'string' ? v : JSON.stringify(v)}`)
    .join(', ')
}

/** The essential outcome of a tool result. */
function compactResult(result: unknown): string {
  if (!result || typeof result !== 'object') return ''
  const r = result as Record<string, any>
  if (r.ok === false) return r.error ?? (r.reason ? reasonName(String(r.reason)) : 'fehlgeschlagen')
  const parts: string[] = []
  if (r.reason) parts.push(reasonName(String(r.reason)))
  if (typeof r.driven_cm === 'number') parts.push(`${Math.round(r.driven_cm)} cm gefahren`)
  if (typeof r.turned_deg === 'number') parts.push(`${r.turned_deg}° gedreht`)
  if (typeof r.distance_cm === 'number') parts.push(`Abstand ${r.distance_cm} cm`)
  if (typeof r.color === 'string' && r.color) parts.push(`Boden ${colorName(r.color)}`)
  if (typeof r.best_deg === 'number') parts.push(`am freiesten bei ${r.best_deg}° (${r.best_cm} cm)`)
  if (Array.isArray(r.floor) && r.floor.length > 1) {
    const patterns = r.floor.map((e: unknown) => (Array.isArray(e) ? String(e[1]) : '?'))
    parts.push(`Bodenspur ${patterns.join(' ')}${r.floor_truncated ? ' (abgeschnitten)' : ''}`)
  }
  if (Array.isArray(r.delivered)) parts.push(r.delivered.length ? `an ${r.delivered.join(', ')}` : 'niemand erreicht')
  if (r.event) parts.push(`${r.event} von ${r.robot}`)
  if (r.note_id) parts.push(`Notiz ${r.note_id}`)
  return parts.join(', ') || 'ok'
}

/** Tool call for the log: name, arguments and outcome. */
export function describeTool(d: Record<string, any>): { call: string; outcome: string; failed: boolean } {
  const args = compactArgs(d.args)
  const result = d.result as Record<string, any> | undefined
  return { call: `${d.name}(${args})`, outcome: compactResult(result), failed: result?.ok === false }
}

/** Cost in cents from micro-euros, readable for small amounts. */
export function formatCost(microEUR: number): string {
  const cents = microEUR / 10_000
  if (microEUR === 0) return '0 ct'
  if (cents < 0.01) return 'unter 0,01 ct'
  return `${cents.toLocaleString('de-DE', { maximumFractionDigits: 2 })} ct`
}

/** Model prices per million tokens, e.g. "0,1 / 0,5 € pro Mio. Token". */
export function formatPrices(priceIn: number, priceOut: number): string {
  if (!priceIn && !priceOut) return 'ohne Kosten'
  const f = (n: number) => n.toLocaleString('de-DE', { maximumFractionDigits: 2 })
  return `${f(priceIn)} / ${f(priceOut)} € pro Mio. Token`
}

/** One line of text for a log entry. */
export function describe(e: LogEntry): string {
  const d = (e.data ?? {}) as Record<string, any>
  switch (e.kind) {
    case 'obstacle_near':
      return `Hindernis in ${d.distance_cm} cm`
    case 'obstacle_clear':
      return 'Weg wieder frei'
    case 'color_changed':
      return `Boden ist jetzt ${colorName(String(d.color))}`
    case 'tilted':
      return 'Umgekippt oder hochgehoben'
    case 'button_a':
      return 'Knopf A gedrückt'
    case 'button_b':
      return 'Knopf B gedrückt'
    case 'clap':
      return 'Klatschen gehört'
    case 'battery_low':
      return `Akku schwach (${d.battery} %)`
    case 'skill_done':
      return `${d.skill}: ${reasonName(String(d.reason))}`
    case 'skill_aborted':
      return `${d.skill} abgebrochen: ${reasonName(String(d.reason))}`
    case 'deploy':
      return d.restart ? `Version ${d.version} deployt, Neustart angefordert` : `Version ${d.version} deployt`
    case 'stop':
      return 'Stopp gedrückt'
    case 'run_start':
      return `Lauf gestartet mit Prompt-Version ${d.version}, Modell ${d.model}`
    case 'run_end':
      return `${RUN_END[String(d.reason)] ?? `Lauf beendet (${d.reason})`}${d.by && d.reason === 'stop' ? ` von ${d.by === 'Team' ? 'euch' : d.by}` : ''}`
    case 'tool': {
      const t = describeTool(d)
      return `${t.call} → ${t.outcome}`
    }
    default:
      return e.text ?? e.kind
  }
}

/** Which filter a log entry belongs to. */
export function logGroup(e: LogEntry): 'robot' | 'team' | 'agent' | 'system' {
  if (e.source === 'robot') return 'robot'
  if (e.source === 'team') return 'team'
  if (e.source.startsWith('agent')) return 'agent'
  return 'system'
}

/** Text color (ink or white) that reads well on the given background hex. */
export function onColor(hex: string): string {
  const v = parseInt(hex.slice(1), 16)
  const [r, g, b] = [(v >> 16) & 255, (v >> 8) & 255, v & 255].map((c) => {
    const s = c / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  })
  const lum = 0.2126 * r + 0.7152 * g + 0.0722 * b
  return lum > 0.4 ? '#1B2230' : '#FFFFFF'
}
