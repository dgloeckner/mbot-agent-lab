import { describe, expect, it } from 'vitest'
import { ALL_TEAMS, assignBody, assignedText, groupTasks, teamsWithout, type AdminTask } from './tasks'

const task = (id: string, state: AdminTask['state'], created: string): AdminTask => ({ id, title: id, state, created_at: created })

describe('groupTasks', () => {
  it('groups by state in panel order, newest first, without empty groups', () => {
    const groups = groupTasks([
      task('a', 'open', '2026-11-24T08:00:00Z'),
      task('b', 'draft', '2026-11-24T09:00:00Z'),
      task('c', 'open', '2026-11-24T10:00:00Z'),
    ])
    expect(groups.map((g) => g.label)).toEqual(['Entwürfe', 'Offen'])
    expect(groups[1].tasks.map((t) => t.id)).toEqual(['c', 'a'])
  })
  it('returns no groups without tasks', () => {
    expect(groupTasks([])).toEqual([])
  })
})

describe('assignBody', () => {
  it('sends "all" for every team and a list for one team', () => {
    expect(assignBody(ALL_TEAMS)).toEqual({ teams: 'all' })
    expect(assignBody('t1')).toEqual({ teams: ['t1'] })
  })
})

describe('teamsWithout', () => {
  it('offers only teams that do not have the task', () => {
    const teams = [
      { id: 't1', tasks: [{ id: 'a', title: 'A', state: 'open' as const, status: 'assigned' as const }] },
      { id: 't2', tasks: [] },
    ]
    expect(teamsWithout(teams, 'a').map((t) => t.id)).toEqual(['t2'])
  })
})

describe('assignedText', () => {
  it('names one team, counts several and says when nothing changed', () => {
    expect(assignedText('Labyrinth', ['t1'], { t1: 'Rot' })).toBe('„Labyrinth“ an Rot zugewiesen')
    expect(assignedText('Labyrinth', ['t1', 't2'], {})).toBe('„Labyrinth“ an 2 Teams zugewiesen')
    expect(assignedText('Labyrinth', [], {})).toContain('schon')
  })
})
