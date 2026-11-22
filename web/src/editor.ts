// CodeMirror 6 setup for the prompt editor: Markdown source with highlighting,
// search, undo history and completion of tool names.

import { autocompletion, closeBrackets, closeBracketsKeymap, completionKeymap, type CompletionContext } from '@codemirror/autocomplete'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { markdownKeymap, markdownLanguage } from '@codemirror/lang-markdown'
import { HighlightStyle, LanguageSupport, syntaxHighlighting, indentOnInput, bracketMatching } from '@codemirror/language'
import { highlightSelectionMatches, search, searchKeymap } from '@codemirror/search'
import { Compartment, EditorState, type Extension } from '@codemirror/state'
import {
  EditorView,
  drawSelection,
  highlightActiveLine,
  highlightActiveLineGutter,
  keymap,
  lineNumbers,
  placeholder,
} from '@codemirror/view'
import { tags as t } from '@lezer/highlight'
import type { ToolInfo } from './types'

/** Highlighting uses CSS variables so light and dark themes switch without reconfiguring. */
const markdownHighlight = HighlightStyle.define([
  { tag: t.heading1, fontWeight: '700', fontSize: '1.22em', color: 'var(--ink)' },
  { tag: t.heading2, fontWeight: '700', fontSize: '1.1em', color: 'var(--ink)' },
  { tag: [t.heading3, t.heading4, t.heading5, t.heading6], fontWeight: '700', color: 'var(--ink)' },
  { tag: t.strong, fontWeight: '700' },
  { tag: t.emphasis, fontStyle: 'italic' },
  { tag: t.strikethrough, textDecoration: 'line-through' },
  { tag: t.monospace, color: 'var(--code-ink)', backgroundColor: 'var(--code-bg)', borderRadius: '3px' },
  { tag: [t.processingInstruction, t.meta, t.contentSeparator], color: 'var(--muted)' },
  { tag: t.quote, color: 'var(--muted)', fontStyle: 'italic' },
  { tag: [t.link, t.url], color: 'var(--accent-ink)', textDecoration: 'underline' },
])

const baseTheme = EditorView.theme({
  '&': { height: '100%', color: 'var(--ink)', backgroundColor: 'transparent' },
  '.cm-scroller': { fontFamily: 'var(--font-mono)', lineHeight: '1.65', overflow: 'auto' },
  '.cm-content': { padding: '18px 0 48px', caretColor: 'var(--accent)' },
  '.cm-line': { padding: '0 22px 0 14px' },
  '.cm-gutters': { backgroundColor: 'transparent', color: 'var(--faint)', border: 'none', paddingLeft: '8px' },
  '.cm-lineNumbers .cm-gutterElement': { minWidth: '2.2em', fontSize: '0.82em', paddingTop: '0.12em' },
  '.cm-activeLine': { backgroundColor: 'var(--active-line)' },
  '.cm-activeLineGutter': { backgroundColor: 'transparent', color: 'var(--muted)' },
  '&.cm-focused .cm-cursor': { borderLeftColor: 'var(--accent)', borderLeftWidth: '2px' },
  '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection': { backgroundColor: 'var(--selection) !important' },
  '.cm-placeholder': { color: 'var(--faint)', fontStyle: 'normal' },
  '.cm-searchMatch': { backgroundColor: 'var(--match)', outline: '1px solid var(--match-line)' },
  '.cm-selectionMatch': { backgroundColor: 'var(--active-line)' },
  '.cm-panels': { backgroundColor: 'var(--surface-2)', color: 'var(--ink)', borderColor: 'var(--line)' },
  '.cm-tooltip': { backgroundColor: 'var(--surface)', border: '1px solid var(--line)', borderRadius: '8px', overflow: 'hidden' },
  '.cm-tooltip-autocomplete > ul > li': { padding: '4px 10px', fontFamily: 'var(--font-mono)' },
  '.cm-tooltip-autocomplete > ul > li[aria-selected]': { backgroundColor: 'var(--accent)', color: 'var(--on-accent)' },
  '.cm-completionDetail': { fontStyle: 'normal', opacity: '0.7', marginLeft: '0.8em' },
  '.cm-completionInfo': { fontFamily: 'var(--font-ui)', padding: '8px 10px', maxWidth: '280px' },
})

/**
 * Markdown without the embedded HTML/CSS/JS support that `markdown()` pulls in:
 * prompts do not need it and it would triple the bundle size.
 */
export function markdownSupport(): LanguageSupport {
  return new LanguageSupport(markdownLanguage)
}

function toolCompletion(tools: ToolInfo[]) {
  return (ctx: CompletionContext) => {
    const word = ctx.matchBefore(/[a-z_]{2,}/)
    if (!word && !ctx.explicit) return null
    return {
      from: word ? word.from : ctx.pos,
      options: tools.map((tool) => ({
        label: tool.name,
        type: 'function',
        detail: tool.params ? `(${tool.params})` : '()',
        info: tool.summary,
      })),
      validFor: /^[a-z_]*$/,
    }
  }
}

export interface EditorOptions {
  parent: HTMLElement
  doc: string
  tools: ToolInfo[]
  onChange: (text: string) => void
  onSave: () => void
  onDeploy: () => void
}

export interface PromptEditor {
  view: EditorView
  replace(text: string): void
  insert(text: string): void
  focus(): void
  destroy(): void
}

export function createEditor(o: EditorOptions): PromptEditor {
  const lineNumbersSlot = new Compartment()
  const extensions: Extension[] = [
    lineNumbersSlot.of(lineNumbers()),
    highlightActiveLineGutter(),
    history(),
    drawSelection(),
    indentOnInput(),
    bracketMatching(),
    closeBrackets(),
    highlightActiveLine(),
    highlightSelectionMatches(),
    search({ top: true }),
    autocompletion({ override: [toolCompletion(o.tools)], icons: false }),
    markdownSupport(),
    syntaxHighlighting(markdownHighlight),
    EditorView.lineWrapping,
    placeholder('Beschreibt hier, was euer Agent tun soll.\n\nZum Beispiel: Finde das blaue Feld und bleib darauf stehen.'),
    EditorView.contentAttributes.of({ 'aria-label': 'Prompt', spellcheck: 'true', lang: 'de' }),
    baseTheme,
    keymap.of([
      { key: 'Mod-s', preventDefault: true, run: () => (o.onSave(), true) },
      { key: 'Mod-Enter', preventDefault: true, run: () => (o.onDeploy(), true) },
      ...markdownKeymap,
      ...closeBracketsKeymap,
      ...defaultKeymap,
      ...searchKeymap,
      ...historyKeymap,
      ...completionKeymap,
      indentWithTab,
    ]),
    EditorView.updateListener.of((u) => {
      if (u.docChanged) o.onChange(u.state.doc.toString())
    }),
  ]
  const view = new EditorView({ parent: o.parent, state: EditorState.create({ doc: o.doc, extensions }) })
  return {
    view,
    replace(text) {
      if (view.state.doc.toString() === text) return
      // One transaction, so the replacement itself can be undone.
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text }, selection: { anchor: Math.min(view.state.selection.main.head, text.length) } })
    },
    insert(text) {
      const { from, to } = view.state.selection.main
      view.dispatch({ changes: { from, to, insert: text }, selection: { anchor: from + text.length }, scrollIntoView: true })
      view.focus()
    },
    focus: () => view.focus(),
    destroy: () => view.destroy(),
  }
}
