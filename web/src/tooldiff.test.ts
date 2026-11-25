import { describe, expect, it } from 'vitest'
import { chipsFor, layoutFor, marksFor, stateLabel, textChanges } from './tooldiff'

describe('tool comparison', () => {
  it('puts each mark on its side', () => {
    const { left, right } = marksFor([
      { kind: 'name', old: 'a', new: 'b' },
      { kind: 'removed', block: 'r' },
      { kind: 'new', block: 'n' },
      { kind: 'moved', block: 'm' },
      { kind: 'value', block: 'v', field: 'cm', old: 30, new: 40 },
      { kind: 'value', block: 'v', field: 'dir', old: 'right', new: 'left' },
    ])
    expect(left.blocks).toEqual({ r: ['removed'], m: ['moved'] })
    expect(right.blocks).toEqual({ n: ['new'], m: ['moved'] })
    expect(left.fields).toEqual({ v: ['cm', 'dir'] })
    expect(right.fields).toEqual({ v: ['cm', 'dir'] })
  })

  it('lists name and description as text', () => {
    expect(
      textChanges([
        { kind: 'description', old: 'alt', new: 'neu' },
        { kind: 'new', block: 'x' },
      ]),
    ).toEqual([{ label: 'Beschreibung', old: 'alt', new: 'neu' }])
  })

  it('makes chips: prompt first, then tools by name', () => {
    const chips = chipsFor([
      { id: 't2', name: 'wand', change: 'entfernt' },
      { id: 't1', name: 'quadrat', change: 'geändert' },
    ])
    expect(chips.map((c) => `${c.label}${c.change ? ':' + c.change : ''}`)).toEqual(['Prompt', 'quadrat:geändert', 'wand:entfernt'])
    expect(chipsFor([])).toEqual([{ key: 'prompt', label: 'Prompt' }])
  })

  it('chooses the layout from the width', () => {
    expect(layoutFor(1200)).toBe('side')
    expect(layoutFor(620)).toBe('side')
    expect(layoutFor(500)).toBe('stacked')
    expect(stateLabel(0)).toBe('Euer Stand jetzt')
    expect(stateLabel(3)).toBe('Version 3')
  })
})
