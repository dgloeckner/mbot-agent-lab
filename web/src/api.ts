// Team API client. The secret team token is the second path segment.

import type { AgentStatus, LogEntry, Note, PromptChoice, PromptVersion, Robot, Session, TeamModels, TeamTask, Totals } from './types'

export const token = decodeURIComponent(location.pathname.split('/')[2] ?? '')
const base = `/t/${encodeURIComponent(token)}`

export class ApiError extends Error {}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  let resp: Response
  try {
    resp = await fetch(base + path, {
      method,
      headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch {
    throw new ApiError('Keine Verbindung zum Server. Prüft das Netzwerk und versucht es noch einmal.')
  }
  if (resp.status === 204) return undefined as T
  const data = await resp.json().catch(() => null)
  if (!resp.ok) {
    throw new ApiError(data?.error ?? `Der Server hat mit Fehler ${resp.status} geantwortet.`)
  }
  return data as T
}

export const api = {
  session: () => req<Session>('GET', '/api/session'),
  versions: () => req<PromptVersion[]>('GET', '/prompts'),
  prompt: (version: number) => req<PromptVersion>('GET', `/prompts/${version}`),
  deploy: (text: string, restart: boolean) => req<PromptVersion>('POST', '/prompt', { text, restart }),
  message: (text: string) => req<LogEntry>('POST', '/message', { text }),
  newConversation: () => req<void>('POST', '/conversation/new'),
  start: () => req<AgentStatus>('POST', '/start'),
  stop: () => req<unknown>('POST', '/stop'),
  notes: () => req<Note[]>('GET', '/notes'),
  addNote: (text: string) => req<Note>('POST', '/notes', { text }),
  updateNote: (id: number, text: string) => req<void>('PUT', `/notes/${id}`, { text }),
  deleteNote: (id: number) => req<void>('DELETE', `/notes/${id}`),
  renameTeam: (name: string) => req<void>('PUT', '/name', { name }),
  setModel: (provider: string) => req<void>('PUT', '/model', { provider }),
  renameRobot: (robot: string, alias: string) => req<void>('PUT', `/robots/${encodeURIComponent(robot)}/alias`, { alias }),
  identify: (robot: string) => req<void>('POST', `/robots/${encodeURIComponent(robot)}/identify`),
  tasks: () => req<{ tasks: TeamTask[]; active: string }>('GET', '/tasks'),
  task: (id: string) => req<TeamTask>('GET', `/tasks/${encodeURIComponent(id)}`),
  accept: (id: string, prompt: PromptChoice, fromTask?: string) =>
    req<TeamTask>('POST', `/tasks/${encodeURIComponent(id)}/accept`, { prompt, from_task: fromTask ?? '' }),
  /** Runs one built-in tool by hand; the answer is the tool result as the agent would get it. */
  runTool: (name: string, args: Record<string, unknown>) => req<Record<string, any>>('POST', `/tools/${encodeURIComponent(name)}`, args),
  activate: (id: string) => req<TeamTask>('POST', `/tasks/${encodeURIComponent(id)}/activate`),
}

export type ConnState = 'connecting' | 'live' | 'reconnecting'

/** Opens the live stream. EventSource reconnects by itself and resumes the log via Last-Event-ID. */
export function connectEvents(on: {
  robots: (r: Robot[]) => void
  agent: (a: AgentStatus) => void
  models: (m: TeamModels) => void
  totals: (t: Totals) => void
  log: (e: LogEntry) => void
  status: (s: ConnState) => void
  /** The active task changed (on any PC of the team): reload everything. */
  task: (id: string) => void
  /** A task was assigned, withdrawn, accepted or closed: reload the task list. */
  tasks: () => void
}): { close: () => void; restart: () => void } {
  let es: EventSource | null = null
  let retry: number | undefined
  const open = () => {
    es = new EventSource(`${base}/events`)
    es.addEventListener('open', () => on.status('live'))
    es.addEventListener('error', () => {
      on.status('reconnecting')
      if (es?.readyState === EventSource.CLOSED) {
        es.close()
        retry = window.setTimeout(open, 3000)
      }
    })
    es.addEventListener('robots', (ev) => on.robots(JSON.parse((ev as MessageEvent).data)))
    es.addEventListener('agent', (ev) => on.agent(JSON.parse((ev as MessageEvent).data)))
    es.addEventListener('totals', (ev) => on.totals(JSON.parse((ev as MessageEvent).data)))
    es.addEventListener('models', (ev) => on.models(JSON.parse((ev as MessageEvent).data)))
    es.addEventListener('log', (ev) => on.log(JSON.parse((ev as MessageEvent).data)))
    es.addEventListener('task', (ev) => on.task(JSON.parse((ev as MessageEvent).data).task_id ?? ''))
    es.addEventListener('tasks', () => on.tasks())
  }
  open()
  const close = () => {
    window.clearTimeout(retry)
    es?.close()
  }
  // A fresh connection (no Last-Event-ID) replays the whole backlog of the active task.
  return { close, restart: () => (close(), open()) }
}
