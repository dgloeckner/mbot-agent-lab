// Which tool names to suggest while a team writes its prompt. Kept free of
// CodeMirror imports so it can be tested without a DOM.

import type { ToolInfo } from './types'

/** Suggestions start after this many letters of a word. */
export const MIN_PREFIX = 3

// The word that ends at the cursor: letters (any language), digits, underscore.
const WORD_AT_END = /[\p{L}\p{N}_]+$/u
// Tool names are lowercase ASCII with underscores.
const TOOL_PREFIX = /^[a-z_]+$/

/**
 * Tools whose name starts with the word that ends `before` (the text of the
 * line up to the cursor). Only whole words count, so "Schicke see" does not
 * match set_leds and "Fahrt" does not match via "ahrt". explicit (Ctrl-Space)
 * lists all tools for an empty or short word.
 */
export function matchTools(before: string, tools: ToolInfo[], explicit = false): { word: string; tools: ToolInfo[] } | null {
  const word = WORD_AT_END.exec(before)?.[0] ?? ''
  if (word && !TOOL_PREFIX.test(word)) return null
  if (!explicit && word.length < MIN_PREFIX) return null
  const hits = tools.filter((t) => t.name.startsWith(word))
  if (hits.length === 0) return null
  // A word that already is a complete tool name needs no popup.
  if (!explicit && hits.length === 1 && hits[0].name === word) return null
  return { word, tools: hits }
}
