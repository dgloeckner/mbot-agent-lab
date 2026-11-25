// Pure helpers of the tool comparison in "Verlauf": which block and field
// gets which mark on which side, the chips of a version, the layout.
// The changes themselves come from the server (programs.Diff).

import type { BlockChange, ToolChange } from './types'

export type MarkKind = 'new' | 'removed' | 'moved'

/** Marks of one side: per block id its kinds, and the fields with a changed value. */
export interface SideMarks {
  blocks: Record<string, MarkKind[]>
  fields: Record<string, string[]>
}

const empty = (): SideMarks => ({ blocks: {}, fields: {} })

function add<T>(m: Record<string, T[]>, key: string, v: T) {
  const list = (m[key] ??= [])
  if (!list.includes(v)) list.push(v)
}

/**
 * Marks per side: a removed block on the left, a new block on the right, a
 * moved block on both sides, a changed value at its field on both sides.
 */
export function marksFor(changes: BlockChange[]): {
  left: SideMarks
  right: SideMarks
} {
  const left = empty()
  const right = empty()
  for (const c of changes) {
    if (!c.block) continue
    switch (c.kind) {
      case 'new':
        add(right.blocks, c.block, 'new')
        break
      case 'removed':
        add(left.blocks, c.block, 'removed')
        break
      case 'moved':
        add(left.blocks, c.block, 'moved')
        add(right.blocks, c.block, 'moved')
        break
      case 'value':
        if (c.field) {
          add(left.fields, c.block, c.field)
          add(right.fields, c.block, c.field)
        }
        break
    }
  }
  return { left, right }
}

/** Name and description changes, shown as text above the picture. */
export function textChanges(changes: BlockChange[]) {
  return changes
    .filter((c) => c.kind === 'name' || c.kind === 'description')
    .map((c) => ({
      label: c.kind === 'name' ? 'Name' : 'Beschreibung',
      old: String(c.old ?? ''),
      new: String(c.new ?? ''),
    }))
}

export interface Chip {
  key: string // 'prompt' or a tool id
  label: string
  change?: ToolChange['change']
}

/** The chips of a comparison: the prompt first, then every team tool that differs, by name. */
export function chipsFor(tools: ToolChange[]): Chip[] {
  const sorted = [...tools].sort((a, b) => a.name.localeCompare(b.name, 'de'))
  return [{ key: 'prompt', label: 'Prompt' }, ...sorted.map((t) => ({ key: t.id, label: t.name, change: t.change }))]
}

/** Below this width the two sides stand one above the other. */
export const SIDE_BY_SIDE_MIN = 620

export const layoutFor = (width: number): 'side' | 'stacked' => (width >= SIDE_BY_SIDE_MIN ? 'side' : 'stacked')

/** "Euer Stand jetzt" for the working state (0), else "Version n". */
export const stateLabel = (version: number) => (version === 0 ? 'Euer Stand jetzt' : `Version ${version}`)

/** Legend of the marks: sign, kind and what it looks like. Never by color alone. */
export const LEGEND = [
  {
    sign: '→',
    kind: 'value',
    text: 'Wert geändert (Feld umrandet, links alt, rechts neu)',
  },
  { sign: '+', kind: 'new', text: 'Block neu (durchgezogener Rand, rechts)' },
  {
    sign: '−',
    kind: 'removed',
    text: 'Block entfernt (gestrichelter Rand, links)',
  },
  {
    sign: '↕',
    kind: 'moved',
    text: 'Block verschoben (gepunkteter Rand, auf beiden Seiten)',
  },
] as const
