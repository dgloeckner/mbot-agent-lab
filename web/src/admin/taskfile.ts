// Task files from docs/lab/aufgaben/{de,en}/*.md: front matter with the
// title, then the task text for the teams and the starter prompt. The control
// room imports one into a draft (upload or paste); the server never reads files.

export interface TaskFile {
  title: string
  body: string
  starter: string
}

const BODY_HEADINGS = ['aufgabe', 'task']
const STARTER_HEADINGS = ['startprompt', 'starter prompt']

/**
 * Parses a task file. Without front matter the first "# " heading is the
 * title; without section headings the whole text is the task text.
 */
export function parseTaskFile(text: string): TaskFile {
  let rest = text.replace(/\r\n?/g, '\n')
  let title = ''
  const fm = /^---\n([\s\S]*?)\n---\n?/.exec(rest)
  if (fm) {
    for (const line of fm[1].split('\n')) {
      const m = /^title:\s*(.*)$/.exec(line.trim())
      if (m) title = m[1].trim().replace(/^["']|["']$/g, '')
    }
    rest = rest.slice(fm[0].length)
  }
  const lines = rest.split('\n')
  const level2 = (l: string, names: string[]) => {
    const m = /^##\s+(.+?)\s*$/.exec(l)
    return !!m && names.includes(m[1].toLowerCase())
  }
  const b = lines.findIndex((l) => level2(l, BODY_HEADINGS))
  const s = lines.findIndex((l) => level2(l, STARTER_HEADINGS))
  let body: string
  let starter = ''
  if (s >= 0) {
    starter = lines.slice(s + 1).join('\n')
    body = lines.slice(b >= 0 && b < s ? b + 1 : 0, s).join('\n')
  } else {
    body = lines.slice(b >= 0 ? b + 1 : 0).join('\n')
  }
  if (!title) {
    const h = /^#\s+(.+)$/m.exec(body)
    if (h) {
      title = h[1].trim()
      body = body.replace(h[0], '')
    }
  }
  return { title: title.slice(0, 80), body: body.trim(), starter: starter.trim() }
}
