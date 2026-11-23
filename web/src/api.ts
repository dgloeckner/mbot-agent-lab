// Team API client. The secret team token is the second path segment.

import type { AgentStatus, LogEntry, Note, PromptVersion, Robot, Session, TeamModels, Totals } from './types'

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
}): () => void {
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
  }
  open()
  return () => {
    window.clearTimeout(retry)
    es?.close()
  }
}
