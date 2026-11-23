// Application state (Preact signals) and the actions that change it.

import { computed, effect, signal } from '@preact/signals'
import { api, ApiError, connectEvents, type ConnState } from './api'
import { LocalStore, browserKV, shouldAutoSnapshot, type Prefs, type Snapshot } from './storage'
import type { AgentStatus, LogEntry, Note, PromptVersion, Robot, Session, Totals } from './types'

const kv = browserKV()
let local: LocalStore

export const session = signal<Session | null>(null)
export const loadError = signal<string | null>(null)
export const robots = signal<Robot[]>([])
export const agent = signal<AgentStatus>({ state: 'waiting', running: false, tokens_in: 0, tokens_out: 0, cost_micro_eur: 0, calls: 0, conversation: false })
/** All runs and conversations of the team, the current one included. */
export const totals = signal<Totals>({
  runs: 0, tokens_in: 0, tokens_out: 0, cost_micro_eur: 0,
  conversation: { count: 0, tokens_in: 0, tokens_out: 0, cost_micro_eur: 0 },
})
/** The run protocol. */
export const log = signal<LogEntry[]>([])
/** The conversation with the agent while no run is active, kept apart from the protocol. */
export const conversation = signal<LogEntry[]>([])
/** Agent answers in the conversation the team has not seen yet. */
export const conversationUnread = signal(0)
export const conn = signal<ConnState>('connecting')
export const notes = signal<Note[]>([])

/** Deployed versions, newest first. `deployed` is the newest one with text. */
export const versions = signal<PromptVersion[]>([])
export const deployed = signal<PromptVersion | null>(null)

/** The editor content and the server version it was last synced with. */
export const draft = signal('')
export const draftBase = signal(0)
export const draftSavedAt = signal<number | null>(null)
export const history = signal<Snapshot[]>([])

/** A newer version deployed from another browser while this draft had changes. */
export const remoteVersion = signal<PromptVersion | null>(null)

export const prefs = signal<Prefs>(LocalStore.loadPrefs(kv))
export const toast = signal<{ text: string; kind: 'ok' | 'error' } | null>(null)
export const busy = signal<'deploy' | 'stop' | null>(null)

/** Set by the editor so other parts of the page can replace its content or insert text. */
export const editorBridge: {
  replace?: (text: string) => void
  insert?: (text: string) => void
  focus?: () => void
} = {}

export const dirty = computed(() => draft.value !== (deployed.value?.text ?? ''))

const MAX_LOG = 500
let toastTimer: number | undefined

export function showToast(text: string, kind: 'ok' | 'error' = 'ok') {
  toast.value = { text, kind }
  window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => (toast.value = null), kind === 'error' ? 6000 : 2800)
}

function fail(err: unknown) {
  showToast(err instanceof ApiError ? err.message : 'Etwas ist schiefgegangen. Versucht es noch einmal.', 'error')
}

export function updatePrefs(patch: Partial<Prefs>) {
  prefs.value = { ...prefs.value, ...patch }
  LocalStore.savePrefs(kv, prefs.value)
}

// Seeing the conversation tab marks its answers as read.
effect(() => {
  if (prefs.value.sideTab === 'conversation') conversationUnread.value = 0
})

/** Appends a live entry; false if it was a replay after a reconnect. */
function append(list: typeof log, e: LogEntry): boolean {
  const cur = list.value
  if (cur.length && e.id <= cur[cur.length - 1].id) return false
  list.value = cur.length >= MAX_LOG ? [...cur.slice(-MAX_LOG + 1), e] : [...cur, e]
  return true
}

// Apply the theme preference to the document.
effect(() => {
  const t = prefs.value.theme
  if (t === 'system') delete document.documentElement.dataset.theme
  else document.documentElement.dataset.theme = t
})

export async function init() {
  let s: Session
  try {
    s = await api.session()
  } catch (err) {
    loadError.value = err instanceof Error ? err.message : String(err)
    return
  }
  local = new LocalStore(kv, s.team.id)
  session.value = s
  robots.value = s.robots
  agent.value = s.agent
  totals.value = s.total
  notes.value = s.notes
  versions.value = s.versions
  deployed.value = s.prompt
  history.value = local.history()

  const saved = local.loadDraft()
  const serverText = s.prompt?.text ?? ''
  if (saved && saved.text !== serverText) {
    draft.value = saved.text
    draftBase.value = saved.base
    draftSavedAt.value = saved.savedAt
    if (s.prompt && s.prompt.version > saved.base) remoteVersion.value = s.prompt
  } else {
    draft.value = serverText
    draftBase.value = s.prompt?.version ?? 0
  }

  connectEvents({
    status: (st) => (conn.value = st),
    robots: (r) => (robots.value = r),
    agent: (a) => (agent.value = a),
    totals: (t) => (totals.value = t),
    models: (m) => {
      if (session.value) session.value = { ...session.value, models: m }
    },
    log: (e) => {
      if (e.channel === 'conversation') {
        if (append(conversation, e) && e.source === 'agent' && e.kind === 'text' && prefs.value.sideTab !== 'conversation') {
          conversationUnread.value++
        }
        return
      }
      if (!append(log, e)) return
      if (e.kind === 'deploy') void onRemoteDeploy(Number(e.data?.version))
      if (e.kind === 'team_renamed' && session.value && typeof e.data?.name === 'string') {
        session.value = { ...session.value, team: { ...session.value.team, name: e.data.name } }
      }
      if (e.kind === 'team_color' && session.value && typeof e.data?.color === 'string') {
        session.value = { ...session.value, team: { ...session.value.team, color: e.data.color } }
      }
    },
  })
}

let saveTimer: number | undefined
let snapTimer: number | undefined

/** Called by the editor on every change. */
export function setDraft(text: string) {
  draft.value = text
  window.clearTimeout(saveTimer)
  saveTimer = window.setTimeout(() => {
    const at = Date.now()
    if (local.saveDraft({ text, base: draftBase.value, savedAt: at })) draftSavedAt.value = at
  }, 300)
  window.clearTimeout(snapTimer)
  snapTimer = window.setTimeout(() => {
    if (shouldAutoSnapshot(history.value[0], draft.value, Date.now())) {
      history.value = local.addSnapshot(draft.value, 'auto')
    }
  }, 2500)
}

export function snapshotNow() {
  if (!draft.value.trim()) return
  history.value = local.addSnapshot(draft.value, 'manual')
  showToast('Zwischenstand in diesem Browser gesichert')
}

/** Replaces the editor content, keeping the previous text in the history. */
export function restore(text: string, label: string) {
  if (draft.value.trim() && draft.value !== text) {
    history.value = local.addSnapshot(draft.value, 'restore')
  }
  editorBridge.replace?.(text)
  showToast(`${label} wiederhergestellt`)
}

export async function deploy(restart: boolean) {
  if (busy.value) return
  busy.value = 'deploy'
  const text = draft.value // the user may keep typing while the request runs
  try {
    const p = await api.deploy(text, restart)
    applyDeployed({ ...p, text })
    history.value = local.addSnapshot(text, 'deploy', p.version)
    showToast(`Version ${p.version} deployt`)
  } catch (err) {
    fail(err)
  } finally {
    busy.value = null
  }
}

function applyDeployed(p: PromptVersion) {
  deployed.value = p
  draftBase.value = p.version
  remoteVersion.value = null
  if (!versions.value.some((v) => v.version === p.version)) {
    versions.value = [{ version: p.version, created_at: p.created_at, chars: p.chars }, ...versions.value]
  }
  local.saveDraft({ text: draft.value, base: p.version, savedAt: Date.now() })
}

/** A deploy seen in the live log: ours, or one from another browser of the team. */
async function onRemoteDeploy(version: number) {
  if (!version || version <= draftBase.value) return
  let p: PromptVersion
  try {
    p = await api.prompt(version)
  } catch {
    return
  }
  if (p.version <= draftBase.value) return
  const clean = draft.value === (deployed.value?.text ?? '') || draft.value === p.text
  if (clean) {
    editorBridge.replace?.(p.text ?? '')
    applyDeployed(p)
  } else {
    deployed.value = p
    if (!versions.value.some((v) => v.version === p.version)) versions.value = [p, ...versions.value]
    remoteVersion.value = p
  }
}

/** Keep the local draft, but treat it as based on the newer version so the hint does not return. */
export function keepDraft() {
  const p = remoteVersion.value
  if (!p) return
  remoteVersion.value = null
  draftBase.value = p.version
  local.saveDraft({ text: draft.value, base: p.version, savedAt: Date.now() })
}

export function acceptRemote() {
  const p = remoteVersion.value
  if (!p) return
  restore(p.text ?? '', `Version ${p.version}`)
  applyDeployed(p)
}

const versionCache = new Map<number, string>()
export async function versionText(v: number): Promise<string> {
  const cached = versionCache.get(v)
  if (cached !== undefined) return cached
  const p = await api.prompt(v)
  versionCache.set(v, p.text ?? '')
  return p.text ?? ''
}

/**
 * Sends a message to the agent. The server decides where it goes: to the run
 * if one is active, otherwise into the conversation. The page follows it.
 */
export async function sendMessage(text: string): Promise<boolean> {
  try {
    const e = await api.message(text)
    const tab = e.channel === 'conversation' ? 'conversation' : 'log'
    if (prefs.value.sideTab !== tab) updatePrefs({ sideTab: tab })
    return true
  } catch (err) {
    fail(err)
    return false
  }
}

/** Ends the conversation; the agent then only knows its notes and the last run. */
export const newConversation = () => attempt(api.newConversation())

export async function stopAll() {
  busy.value = 'stop'
  try {
    await api.stop()
  } catch (err) {
    fail(err)
  } finally {
    busy.value = null
  }
}

export async function startRun() {
  try {
    agent.value = await api.start()
  } catch (err) {
    fail(err)
  }
}

export async function addNote(text: string): Promise<boolean> {
  try {
    const n = await api.addNote(text)
    notes.value = [...notes.value, n]
    return true
  } catch (err) {
    fail(err)
    return false
  }
}

export async function saveNote(id: number, text: string): Promise<boolean> {
  try {
    await api.updateNote(id, text)
    notes.value = notes.value.map((n) => (n.id === id ? { ...n, text } : n))
    return true
  } catch (err) {
    fail(err)
    return false
  }
}

export async function deleteNote(id: number) {
  try {
    await api.deleteNote(id)
    notes.value = notes.value.filter((n) => n.id !== id)
  } catch (err) {
    fail(err)
  }
}

/** Runs an action and reports failures as a toast. Returns whether it worked. */
async function attempt(p: Promise<unknown>, ok?: string): Promise<boolean> {
  try {
    await p
    if (ok) showToast(ok)
    return true
  } catch (err) {
    fail(err)
    return false
  }
}

export const renameTeam = (name: string) => attempt(api.renameTeam(name.trim()), 'Teamname geändert')

/** Chooses the model for the team's next runs. */
export async function chooseModel(provider: string) {
  const s = session.value
  if (!s || s.models.selected === provider) return
  const m = s.models.options.find((o) => o.provider === provider)
  if (await attempt(api.setModel(provider), `Modell ${m?.model ?? provider} gilt ab dem nächsten Lauf`)) {
    session.value = { ...session.value!, models: { ...session.value!.models, selected: provider } }
  }
}

export const renameRobot = (robot: string, alias: string) => attempt(api.renameRobot(robot, alias.trim()))

export const identifyRobot = (robot: string, alias: string) =>
  attempt(api.identify(robot), `${alias} blinkt und piept jetzt`)

export async function copyTeamLink() {
  try {
    await navigator.clipboard.writeText(location.origin + location.pathname)
    showToast('Team-Link kopiert. Öffnet ihn auf den anderen Rechnern eures Teams.')
  } catch {
    showToast(`Team-Link: ${location.origin}${location.pathname}`)
  }
}
