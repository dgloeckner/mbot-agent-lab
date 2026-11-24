// What the page shows for one task: everything the team produced in it.
// Used at start and whenever the active task changes, so nothing of the
// previous task stays on screen.

import type { Draft, Snapshot } from './storage'
import type { LogEntry, Note, PromptVersion, Session, TeamTask } from './types'

export interface TaskView {
  task: TeamTask | null
  notes: Note[]
  versions: PromptVersion[]
  deployed: PromptVersion | null
  draft: string
  draftBase: number
  draftSavedAt: number | null
  remoteVersion: PromptVersion | null
  history: Snapshot[]
  log: LogEntry[]
  conversation: LogEntry[]
}

/** The view of the session's active task. saved and history come from the browser storage of that (team, task). */
export function taskView(s: Session, saved: Draft | null, history: Snapshot[]): TaskView {
  const serverText = s.prompt?.text ?? ''
  const v: TaskView = {
    task: s.task,
    notes: s.notes,
    versions: s.versions,
    deployed: s.prompt,
    draft: serverText,
    draftBase: s.prompt?.version ?? 0,
    draftSavedAt: null,
    remoteVersion: null,
    history,
    log: [],
    conversation: [],
  }
  if (s.task && saved && saved.text !== serverText) {
    v.draft = saved.text
    v.draftBase = saved.base
    v.draftSavedAt = saved.savedAt
    if (s.prompt && s.prompt.version > saved.base) v.remoteVersion = s.prompt
  }
  return v
}

/** Whether a live log entry belongs on the page that shows task. */
export function belongsTo(e: LogEntry, task: string | undefined): boolean {
  return !e.task_id || e.task_id === (task ?? '')
}

/** Tasks the team can switch to, and new ones to accept. */
export function pickerLists(tasks: TeamTask[]): { accepted: TeamTask[]; fresh: TeamTask[] } {
  return {
    accepted: tasks.filter((t) => t.status !== 'assigned'),
    fresh: tasks.filter((t) => t.status === 'assigned' && t.state !== 'closed'),
  }
}
