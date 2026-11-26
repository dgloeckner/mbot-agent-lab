// Team API client. The secret team token is the second path segment.

import type { ProgramDetail, ProgramEvent, ProgramInfo, ToolChanges, ToolDiff, VersionTool } from './types'
import type { AgentStatus, LogEntry, Note, PromptChoice, PromptVersion, Robot, Session, TeamModels, TeamTask, Totals } from './types'

export const token = decodeURIComponent(location.pathname.split('/')[2] ?? '')
const base = `/t/${encodeURIComponent(token)}`

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status = 0,
  ) {
    super(message)
  }
}

/** Thrown when the server could not be reached (network down, no answer in time). */
export class NoConnection extends ApiError {}

async function req<T>(method: string, path: string, body?: unknown, timeoutMs?: number): Promise<T> {
  let resp: Response
  const ctl = timeoutMs ? new AbortController() : undefined
  const timer = ctl ? window.setTimeout(() => ctl.abort(), timeoutMs) : undefined
  try {
    resp = await fetch(base + path, {
      method,
      headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: ctl?.signal,
    })
  } catch {
    throw new NoConnection('Keine Verbindung zum Server. Prüft das Netzwerk und versucht es noch einmal.')
  } finally {
    window.clearTimeout(timer)
  }
  if (resp.status === 204) return undefined as T
  const data = await resp.json().catch(() => null)
  if (!resp.ok) {
    throw new ApiError(data?.error ?? `Der Server hat mit Fehler ${resp.status} geantwortet.`, resp.status)
  }
  return data as T
}

export const api = {
  session: () => req<Session>('GET', '/api/session'),
  versions: () => req<PromptVersion[]>('GET', '/prompts'),
  prompt: (version: number) => req<PromptVersion>('GET', `/prompts/${version}`),
  deploy: (text: string, restart: boolean) =>
    req<Omit<PromptVersion, 'tools'> & { tools: VersionTool[]; warning?: string; unchanged?: boolean }>('POST', '/prompt', { text, restart }),
  restoreVersion: (version: number) => req<PromptVersion>('POST', `/prompts/${version}/restore`),
  /** Team tools that differ between a version and another one (against; 0 is the working state). */
  compareTools: (version: number, against: number) => req<ToolChanges>('GET', `/prompts/${version}/tools?against=${against || 'now'}`),
  compareTool: (version: number, against: number, id: string) =>
    req<ToolDiff>('GET', `/prompts/${version}/tools/${encodeURIComponent(id)}?against=${against || 'now'}`),
  message: (text: string) => req<LogEntry>('POST', '/message', { text }),
  newConversation: () => req<void>('POST', '/conversation/new'),
  start: () => req<AgentStatus>('POST', '/start'),
  /** Short timeout: a stop that hangs must not look like a stop that worked. */
  stop: () => req<unknown>('POST', '/stop', undefined, 5000),
  notes: () => req<Note[]>('GET', '/notes'),
  addNote: (text: string) => req<Note>('POST', '/notes', { text }),
  updateNote: (id: number, text: string) => req<void>('PUT', `/notes/${id}`, { text }),
  deleteNote: (id: number) => req<void>('DELETE', `/notes/${id}`),
  renameTeam: (name: string) => req<void>('PUT', '/name', { name }),
  /** A short-lived code another PC of the team enters on /join. */
  joinCode: () => req<{ code: string; expires_at: string }>('POST', '/join-code'),
  setModel: (provider: string) => req<void>('PUT', '/model', { provider }),
  renameRobot: (robot: string, alias: string) => req<void>('PUT', `/robots/${encodeURIComponent(robot)}/alias`, { alias }),
  identify: (robot: string) => req<void>('POST', `/robots/${encodeURIComponent(robot)}/identify`),
  tasks: () => req<{ tasks: TeamTask[]; active: string }>('GET', '/tasks'),
  task: (id: string) => req<TeamTask>('GET', `/tasks/${encodeURIComponent(id)}`),
  accept: (id: string, prompt: PromptChoice, fromTask?: string) =>
    req<TeamTask>('POST', `/tasks/${encodeURIComponent(id)}/accept`, { prompt, from_task: fromTask ?? '' }),
  /** Runs one built-in tool by hand; the answer is the tool result as the agent would get it. */
  runTool: (name: string, args: Record<string, unknown>) => req<Record<string, any>>('POST', `/tools/${encodeURIComponent(name)}`, args),
  programs: () => req<ProgramInfo[]>('GET', '/programs'),
  program: (id: string) => req<ProgramDetail>('GET', `/programs/${encodeURIComponent(id)}`),
  createProgram: (name: string, description: string, blockly: unknown) => req<ProgramDetail>('POST', '/programs', { name, description, blockly }),
  saveProgram: (id: string, name: string, description: string, blockly: unknown) =>
    req<ProgramDetail>('PUT', `/programs/${encodeURIComponent(id)}`, { name, description, blockly }),
  deleteProgram: (id: string) => req<void>('DELETE', `/programs/${encodeURIComponent(id)}`),
  runProgram: (id: string, robot: string | undefined, step: boolean) =>
    req<{ run: string }>('POST', `/programs/${encodeURIComponent(id)}/run`, { robot: robot ?? '', step }),
  continueProgram: (run: string) => req<void>('POST', `/program-runs/${encodeURIComponent(run)}/continue`),
  blocks: () => req<{ definitions: any[]; toolbox: any }>('GET', '/blocks'),
  activate: (id: string) => req<TeamTask>('POST', `/tasks/${encodeURIComponent(id)}/activate`),
}

export type ConnState = 'connecting' | 'live' | 'reconnecting'

/**
 * Opens the live stream. EventSource reconnects by itself after an error and
 * resumes the log via Last-Event-ID. A connection that stays open but carries
 * nothing (a dead Wi-Fi, a frozen server) gives no error: the server sends a
 * named keep-alive ("ping"), and after three intervals without any event the
 * page reports the connection as lost and opens a new stream (?after= the last
 * log entry). `status` gets the time of the last event with it.
 */
export function connectEvents(on: {
  robots: (r: Robot[]) => void
  agent: (a: AgentStatus) => void
  models: (m: TeamModels) => void
  totals: (t: Totals) => void
  log: (e: LogEntry) => void
  status: (s: ConnState, lastEvent: number) => void
  /** The active task changed (on any PC of the team): reload everything. */
  task: (id: string) => void
  /** A task was assigned, withdrawn, accepted or closed: reload the task list. */
  tasks: () => void
  /** A team tool was saved or deleted (on any PC of the team). */
  programs: (id: string) => void
  /** A step of a running team tool. */
  program: (e: ProgramEvent) => void
}): { close: () => void; restart: () => void } {
  let es: EventSource | null = null
  let retry: number | undefined
  let lastEvent = Date.now()
  let lastId = ''
  let everyMs = 10_000
  let state: ConnState = 'connecting'
  let silentSince = Date.now() // last event, or last attempt to reconnect
  const set = (st: ConnState) => {
    if (st !== state) on.status((state = st), lastEvent)
  }
  const seen = () => {
    lastEvent = silentSince = Date.now()
    set('live')
  }
  const open = (after = '') => {
    es = new EventSource(`${base}/events${after ? `?after=${encodeURIComponent(after)}` : ''}`)
    es.addEventListener('open', seen)
    es.addEventListener('error', () => {
      set('reconnecting')
      if (es?.readyState === EventSource.CLOSED) {
        es.close()
        retry = window.setTimeout(() => open(lastId), 3000)
      }
    })
    const listen = (name: string, f: (data: any, ev: MessageEvent) => void) =>
      es!.addEventListener(name, (ev) => {
        seen()
        f(JSON.parse((ev as MessageEvent).data), ev as MessageEvent)
      })
    listen('ping', (d) => (everyMs = Math.max(1000, (Number(d.every_s) || 10) * 1000)))
    listen('robots', (d) => on.robots(d))
    listen('agent', (d) => on.agent(d))
    listen('totals', (d) => on.totals(d))
    listen('models', (d) => on.models(d))
    listen('log', (d, ev) => {
      if (ev.lastEventId) lastId = ev.lastEventId
      on.log(d)
    })
    listen('task', (d) => on.task(d.task_id ?? ''))
    listen('tasks', () => on.tasks())
    listen('programs', (d) => on.programs(d.id ?? ''))
    listen('program', (d) => on.program(d))
  }
  // Silence for three keep-alive intervals: the connection is dead even if the browser does not know.
  const watchdog = window.setInterval(() => {
    if (Date.now() - silentSince < 3 * everyMs) return
    set('reconnecting')
    reconnect() // the next attempt after another three intervals
  }, 1000)
  // The browser knows it is offline: say so at once and reconnect when it is back.
  const reconnect = () => {
    silentSince = Date.now()
    window.clearTimeout(retry)
    es?.close()
    open(lastId)
  }
  const offline = () => set('reconnecting')
  window.addEventListener('offline', offline)
  window.addEventListener('online', reconnect)
  open()
  const close = () => {
    window.clearTimeout(retry)
    es?.close()
  }
  // A fresh connection (no Last-Event-ID) replays the whole backlog of the active task.
  return {
    close: () => {
      window.clearInterval(watchdog)
      window.removeEventListener('offline', offline)
      window.removeEventListener('online', reconnect)
      close()
    },
    restart: () => (close(), (lastId = ''), open()),
  }
}
