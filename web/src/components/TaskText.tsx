// The text of a task as the team reads it. Loaded on demand (Markdown renderer).
import DOMPurify from 'dompurify'
import { marked } from 'marked'
import type { TeamTask } from '../types'

export default function TaskText({ task }: { task: TeamTask }) {
  const html = DOMPurify.sanitize(marked.parse(task.body || '', { async: false, gfm: true, breaks: false }) as string)
  return <article class="prose task-text" dangerouslySetInnerHTML={{ __html: html }} />
}
