import { describe, expect, it } from 'vitest'
import { belongsTo, pickerLists, taskView } from './taskview'
import type { Session, TeamTask } from './types'

const task = (id: string, status: TeamTask['status'], state: TeamTask['state'] = 'open'): TeamTask => ({
  id, title: id, body: '', starter: '', state, status, assigned_at: '2026-11-24T08:00:00Z',
})

function session(t: TeamTask | null, promptText?: string, version = 1): Session {
  return {
    team: { id: 'rot', name: 'Rot', color: 'red' },
    task: t,
    tasks: t ? [t] : [],
    robots: [],
    agent: { state: 'waiting', running: false, tokens_in: 0, tokens_out: 0, cost_micro_eur: 0, calls: 0, conversation: true },
    total: { runs: 0, tokens_in: 0, tokens_out: 0, cost_micro_eur: 0, conversation: { count: 0, tokens_in: 0, tokens_out: 0, cost_micro_eur: 0 } },
    agent_available: true,
    models: { selected: 'm', options: [] },
    prompt: promptText === undefined ? null : { version, text: promptText, created_at: '', chars: promptText.length },
    versions: promptText === undefined ? [] : [{ version, created_at: '', chars: promptText.length }],
    notes: t ? [{ id: 1, text: 'Notiz ' + t.id, author: 'team', created_at: '' }] : [],
    tools: [],
    limits: { prompt_chars: 1, message_chars: 1, note_chars: 1 },
  }
}

describe('taskView after a task switch', () => {
  it('shows only the new task: its prompt, notes, draft; log and conversation start empty', () => {
    const v = taskView(session(task('b', 'active'), 'Prompt B'), null, [])
    expect(v.task?.id).toBe('b')
    expect(v.deployed?.text).toBe('Prompt B')
    expect(v.draft).toBe('Prompt B')
    expect(v.notes.map((n) => n.text)).toEqual(['Notiz b'])
    expect(v.log).toEqual([])
    expect(v.conversation).toEqual([])
  })

  it("restores the task's own unsaved draft and flags a newer deployed version", () => {
    const v = taskView(session(task('a', 'active'), 'Version 3', 3), { text: 'mein Entwurf', base: 2, savedAt: 9 }, [])
    expect(v.draft).toBe('mein Entwurf')
    expect(v.draftBase).toBe(2)
    expect(v.remoteVersion?.version).toBe(3)
  })

  it('shows nothing to edit without a task', () => {
    const v = taskView(session(null), { text: 'alt', base: 0, savedAt: 1 }, [])
    expect(v.task).toBeNull()
    expect(v.draft).toBe('')
    expect(v.notes).toEqual([])
  })
})

describe('belongsTo', () => {
  it('keeps entries of the shown task and entries without a task', () => {
    const e = { id: 1, at: '', source: 'team', kind: 'chat' }
    expect(belongsTo({ ...e, task_id: 'a' }, 'a')).toBe(true)
    expect(belongsTo({ ...e, task_id: 'a' }, 'b')).toBe(false)
    expect(belongsTo(e, 'b')).toBe(true)
  })
})

describe('pickerLists', () => {
  it('splits accepted tasks from new ones and hides closed new ones', () => {
    const l = pickerLists([task('a', 'active'), task('b', 'accepted', 'closed'), task('c', 'assigned'), task('d', 'assigned', 'closed')])
    expect(l.accepted.map((t) => t.id)).toEqual(['a', 'b'])
    expect(l.fresh.map((t) => t.id)).toEqual(['c'])
  })
})
