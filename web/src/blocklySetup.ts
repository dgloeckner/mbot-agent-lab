// Blockly setup shared by the tool editor and the tool comparison (both
// lazy-loaded): German messages and the block definitions served by the
// server (GET blocks), defined once per page.

import * as Blockly from 'blockly/core'
import * as De from 'blockly/msg/de'
import { api } from './api'
import { DE_EXTRA } from './blocklyDe'

Blockly.setLocale({ ...(De as unknown as Record<string, string>), ...DE_EXTRA })

let defined: Promise<{ toolbox: any }> | null = null

/** Defines the served blocks once per page. */
export function defineBlocks() {
  defined ??= api.blocks().then((d) => {
    Blockly.common.defineBlocksWithJsonArray(d.definitions)
    return { toolbox: d.toolbox }
  })
  return defined
}

export const MEDIA = '/assets/blockly-media/'

/** Whether the page shows dark: its own choice, else the system's. */
export function pageIsDark(): boolean {
  const t = document.documentElement.dataset.theme
  return t === 'dark' || (t !== 'light' && window.matchMedia('(prefers-color-scheme: dark)').matches)
}

const font = { family: "'Bricolage Grotesque Variable', 'Segoe UI', system-ui, sans-serif", weight: '560', size: 11 }

/** The block canvas in the page's colors, light or dark. */
export const lightTheme = Blockly.Theme.defineTheme('lab-light', {
  name: 'lab-light',
  base: Blockly.Themes.Classic,
  fontStyle: font,
  componentStyles: { workspaceBackgroundColour: '#ffffff', flyoutBackgroundColour: '#eef0f3', flyoutOpacity: 1, scrollbarColour: '#c3c9d2' },
})

export const darkTheme = Blockly.Theme.defineTheme('lab-dark', {
  name: 'lab-dark',
  base: Blockly.Themes.Classic,
  fontStyle: font,
  componentStyles: {
    workspaceBackgroundColour: '#182030',
    flyoutBackgroundColour: '#1e2738',
    flyoutForegroundColour: '#e4e9f1',
    flyoutOpacity: 1,
    scrollbarColour: '#3a4658',
    insertionMarkerColour: '#ffffff',
    insertionMarkerOpacity: 0.3,
  },
})

export const GRID_COLOUR = { light: '#dfe3e9', dark: '#2c374a' }
