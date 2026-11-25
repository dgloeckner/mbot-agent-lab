// Blockly setup shared by the tool editor and the tool comparison (both
// lazy-loaded): German messages and the block definitions served by the
// server (GET blocks), defined once per page.

import * as Blockly from 'blockly/core'
import * as De from 'blockly/msg/de'
import { api } from './api'

Blockly.setLocale(De as unknown as Record<string, string>)

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
