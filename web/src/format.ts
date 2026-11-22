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
