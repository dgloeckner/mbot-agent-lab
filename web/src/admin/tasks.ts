// Pure helpers for the task panel of the control room.

export type TaskState = 'draft' | 'open' | 'closed'
export type TeamTaskStatus = 'assigned' | 'accepted' | 'active'

export interface AdminTask {
  id: string
  title: string
  state: TaskState
  created_at: string
  published_at?: string
  closed_at?: string
}

/** A task with its text, as GET /admin/api/tasks/{id} returns it. */
export interface TaskDetail extends AdminTask {
  body: string
  starter: string
}

export interface AdminTeamTask {
  id: string
  title: string
  state: TaskState
  status: TeamTaskStatus
}

export const STATE_LABEL: Record<TaskState, string> = { draft: 'Entwurf', open: 'Offen', closed: 'Geschlossen' }
export const STATE_GROUP: Record<TaskState, string> = { draft: 'Entwürfe', open: 'Offen', closed: 'Geschlossen' }
export const STATUS_LABEL: Record<TeamTaskStatus, string> = { assigned: 'zugewiesen', accepted: 'angenommen', active: 'aktiv' }

/** The target "all" assigns a task to every team that exists now. */
export const ALL_TEAMS = 'all'

/** Tasks grouped by state in the order the panel shows them; empty groups are left out. Newest first within a group. */
export function groupTasks<T extends AdminTask>(tasks: T[]): { state: TaskState; label: string; tasks: T[] }[] {
  const order: TaskState[] = ['draft', 'open', 'closed']
  return order
    .map((state) => ({
      state,
      label: STATE_GROUP[state],
      tasks: tasks.filter((t) => t.state === state).sort((a, b) => (a.created_at < b.created_at ? 1 : a.created_at > b.created_at ? -1 : 0)),
    }))
    .filter((g) => g.tasks.length > 0)
}

/** The body of POST /admin/api/tasks/{id}/assign for one team or ALL_TEAMS. */
export function assignBody(target: string): { teams: string[] | typeof ALL_TEAMS } {
  return { teams: target === ALL_TEAMS ? ALL_TEAMS : [target] }
}

/** Teams that do not have the task yet: the choices of the assign menu. */
export function teamsWithout<T extends { id: string; tasks: AdminTeamTask[] }>(teams: T[], task: string): T[] {
  return teams.filter((t) => !t.tasks.some((x) => x.id === task))
}

/** Toast text after assigning: who got the task now. */
export function assignedText(title: string, assigned: string[], names: Record<string, string>): string {
  if (assigned.length === 0) return `Alle gewählten Teams haben „${title}“ schon.`
  if (assigned.length === 1) return `„${title}“ an ${names[assigned[0]] ?? 'ein Team'} zugewiesen`
  return `„${title}“ an ${assigned.length} Teams zugewiesen`
}
