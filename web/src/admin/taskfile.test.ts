import { describe, expect, it } from 'vitest'
import { parseTaskFile } from './taskfile'

describe('parseTaskFile', () => {
  it('reads title, task text and starter prompt', () => {
    const t = parseTaskFile('---\ntitle: Labyrinth\n---\n\n## Aufgabe\n\nFindet **Blau**.\n\n## Startprompt\n\n# Feldwissen\nFahr los.\n')
    expect(t).toEqual({ title: 'Labyrinth', body: 'Findet **Blau**.', starter: '# Feldwissen\nFahr los.' })
  })
  it('reads English headings and Windows line ends', () => {
    const t = parseTaskFile('---\r\ntitle: "Maze"\r\n---\r\n## Task\r\nFind blue.\r\n## Starter prompt\r\nGo.\r\n')
    expect(t).toEqual({ title: 'Maze', body: 'Find blue.', starter: 'Go.' })
  })
  it('takes a pasted text without structure as the task text', () => {
    expect(parseTaskFile('# Rennen\n\nFahrt eine Runde.')).toEqual({ title: 'Rennen', body: 'Fahrt eine Runde.', starter: '' })
    expect(parseTaskFile('Nur Text.')).toEqual({ title: '', body: 'Nur Text.', starter: '' })
  })
})

// The lab material must import: every file has a title, a task text and the same file in both languages.
const files = import.meta.glob('../../../docs/lab/aufgaben/*/*.md', { query: '?raw', import: 'default', eager: true }) as Record<string, string>

describe('docs/lab/aufgaben', () => {
  const names = (lang: string) =>
    Object.keys(files)
      .filter((p) => p.includes(`/aufgaben/${lang}/`))
      .map((p) => p.split('/').pop()!)
      .sort()
  it('has the same files in German and English', () => {
    expect(names('en')).toEqual(names('de'))
    expect(names('de').length).toBeGreaterThan(0)
  })
  for (const [path, text] of Object.entries(files)) {
    it(`${path.split('/aufgaben/')[1]} parses`, () => {
      const t = parseTaskFile(text)
      expect(t.title).not.toBe('')
      expect(t.body.length).toBeGreaterThan(50)
      expect(t.starter.length).toBeGreaterThan(50)
      expect(t.body).not.toContain('## ')
    })
  }
})
