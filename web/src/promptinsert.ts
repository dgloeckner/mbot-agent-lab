// Where "In den Prompt einfügen" puts a tool name. At the cursor only if the
// team placed it in this page view; otherwise on a new line at the end. The
// spaces that are missing around the name are added, and a cursor inside a
// word moves to the word's end first.

export interface InsertPlan {
  from: number
  to: number
  insert: string
  /** Where the inserted name itself starts and ends (for the flash). */
  start: number
  end: number
}

const isWord = (c: string | undefined) => c !== undefined && /[\p{L}\p{N}_`]/u.test(c)
const isSpace = (c: string | undefined) => c === undefined || /\s/.test(c)

/** sel: the selection the team made in this page view, or null. */
export function planInsert(doc: string, text: string, sel: { from: number; to: number } | null): InsertPlan {
  if (!sel) {
    if (doc.trim() === '') return { from: 0, to: doc.length, insert: text, start: 0, end: text.length }
    const prefix = doc.endsWith('\n') ? '' : '\n'
    return { from: doc.length, to: doc.length, insert: prefix + text, start: doc.length + prefix.length, end: doc.length + prefix.length + text.length }
  }
  let { from, to } = sel
  if (from === to) {
    while (isWord(doc[from - 1]) && isWord(doc[from])) from++
    to = from
  }
  const before = isSpace(doc[from - 1]) || /[(["„]/.test(doc[from - 1]) ? '' : ' '
  const after = isSpace(doc[to]) || /[.,;:!?)\]“"]/.test(doc[to]) ? '' : ' '
  const start = from + before.length
  return { from, to, insert: before + text + after, start, end: start + text.length }
}
