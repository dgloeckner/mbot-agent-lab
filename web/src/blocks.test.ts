// The blocks the server serves (pinned in docs/examples/programs/blocks.json
// by a Go test) work in Blockly, and every example state round-trips: the Go
// tests convert real Blockly output.
import { describe, expect, it } from 'vitest'
import * as Blockly from 'blockly/core'

const files = import.meta.glob('../../docs/examples/programs/*.json', { eager: true, import: 'default' }) as Record<string, any>
const served = Object.entries(files).find(([f]) => f.endsWith('/blocks.json'))![1] as { definitions: any[]; toolbox: any }
const examples: [string, any][] = Object.entries(files).filter(([f]) => !f.endsWith('/blocks.json'))

Blockly.common.defineBlocksWithJsonArray(served.definitions)

describe('served block definitions', () => {
  it('Blockly accepts every definition', () => {
    const ws = new Blockly.Workspace()
    for (const def of served.definitions) {
      const block = ws.newBlock(def.type)
      expect(block.type).toBe(def.type)
    }
    ws.dispose()
  })
  it('the toolbox names only defined blocks', () => {
    const types = new Set(served.definitions.map((d) => d.type))
    const named = served.toolbox.contents.flatMap((c: any) => c.contents.map((b: any) => b.type))
    expect(named.filter((t: string) => !types.has(t))).toEqual([])
    expect(served.toolbox.contents.map((c: any) => c.name)).toEqual(['Fahren', 'Sensoren', 'Licht und Anzeige', 'Wiederholen', 'Entscheiden'])
  })
})

describe('examples', () => {
  it.each(examples)('%s round-trips through Blockly', (_: string, ex: any) => {
    const ws = new Blockly.Workspace()
    Blockly.serialization.workspaces.load(ex.blockly, ws)
    expect(Blockly.serialization.workspaces.save(ws)).toEqual(ex.blockly)
    ws.dispose()
  })
})
