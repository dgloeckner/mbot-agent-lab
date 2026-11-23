import { describe, expect, it } from 'vitest'
import { matchTools } from './completion'
import type { ToolInfo } from './types'

const tool = (name: string): ToolInfo => ({ name, group: 'Bewegen', params: '', summary: '', ready: true })
const tools = ['drive', 'drive_until', 'turn', 'stop', 'scan', 'set_leds', 'show_text', 'say', 'finish'].map(tool)
const names = (before: string, explicit = false) => matchTools(before, tools, explicit)?.tools.map((t) => t.name) ?? null

describe('matchTools', () => {
  it('matches tool names by prefix only', () => {
    expect(names('Schicke see')).toBeNull() // was a fuzzy match for set_leds
    expect(names('Nutze dri')).toEqual(['drive', 'drive_until'])
    expect(names('dann set')).toEqual(['set_leds'])
    expect(names('drive_u')).toEqual(['drive_until'])
  })

  it('needs three letters of a whole lowercase word', () => {
    expect(names('dr')).toBeNull()
    expect(names('Stopp')).toBeNull() // "topp" inside a capitalised word does not count
    expect(names('Fahrt')).toBeNull()
    expect(names('größe')).toBeNull()
  })

  it('stays quiet once a single tool name is complete', () => {
    expect(names('dann stop')).toBeNull()
    expect(names('drive')).toEqual(['drive', 'drive_until'])
  })

  it('lists everything on explicit request', () => {
    expect(names('', true)).toHaveLength(tools.length)
    expect(names('Ruf ', true)).toHaveLength(tools.length)
    expect(names('dr', true)).toEqual(['drive', 'drive_until'])
  })
})
