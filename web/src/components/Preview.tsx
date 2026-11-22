import DOMPurify from 'dompurify'
import { marked } from 'marked'
import { draft } from '../state'

/** Rendered Markdown of the current draft. Loaded on demand. */
export default function Preview() {
  if (!draft.value.trim()) {
    return <div class="preview empty-state">Noch nichts zu sehen. Schreibt euren Prompt im Tab „Schreiben“.</div>
  }
  const html = DOMPurify.sanitize(marked.parse(draft.value, { async: false, gfm: true, breaks: false }) as string)
  return <article class="preview prose" dangerouslySetInnerHTML={{ __html: html }} />
}
