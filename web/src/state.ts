// Application state (Preact signals) and the actions that change it.

import { computed, effect, signal } from '@preact/signals'
import { api, ApiError, connectEvents, type ConnState } from './api'
import { LocalStore, browserKV, dropOldKeys, shouldAutoSnapshot, type Prefs, type Snapshot } from './storage'
import { belongsTo, taskView } from './taskview'
import { createSaver, freeName, idleRun, reduceRun, type RunView } from './toolrun'
import type { ProgramDetail, ProgramEvent, ProgramInfo } from './types'
import type { AgentStatus, LogEntry, Note, PromptChoice, PromptVersion, Robot, Session, TeamTask, Totals } from './types'

const kv = browserKV()
/** Browser storage of the active (team, task); null while the team has no task. */
let local: LocalStore | null = null

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

/** The active task (with its text) and every task assigned to the team. */
export const task = signal<TeamTask | null>(null)
export const tasks = signal<TeamTask[]>([])
/** A closed task can be read but not changed. */
export const readOnly = computed(() => task.value?.state === 'closed')
/** The task whose accept dialog is open. */
export const accepting = signal<TeamTask | null>(null)

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

/** Shows the session's active task: prompt, notes, draft and history of that (team, task) only. */
function applySession(s: Session) {
  window.clearTimeout(saveTimer)
  window.clearTimeout(snapTimer)
  versionCache.clear()
  local = s.task ? new LocalStore(kv, s.team.id, s.task.id) : null
  session.value = s
  robots.value = s.robots
  agent.value = s.agent
  totals.value = s.total
  tasks.value = s.tasks
  const v = taskView(s, local?.loadDraft() ?? null, local?.history() ?? [])
  task.value = v.task
  notes.value = v.notes
  versions.value = v.versions
  deployed.value = v.deployed
  draft.value = v.draft
  draftBase.value = v.draftBase
  draftSavedAt.value = v.draftSavedAt
  remoteVersion.value = v.remoteVersion
  history.value = v.history
  log.value = v.log
  conversation.value = v.conversation
  conversationUnread.value = 0
  teamTools.value = []
  toolRun.value = idleRun
  if (s.task) void loadTeamTools()
  else leftView.value = 'prompt'
}

let events: { close: () => void; restart: () => void } | null = null

/** The active task changed, here or on another PC of the team: reload everything for the new task. */
async function onTaskChanged(id: string) {
  if (id === (task.peek()?.id ?? '') && session.peek()) return
  try {
    applySession(await api.session())
  } catch (err) {
    fail(err)
    return
  }
  // The stream keeps running; a fresh one replays the new task's backlog.
  events?.restart()
}

async function reloadTasks() {
  try {
    const r = await api.tasks()
    tasks.value = r.tasks
    const active = r.tasks.find((t) => t.id === task.peek()?.id)
    // The active task's state (closed) changes without a switch.
    if (active && task.peek() && active.state !== task.peek()!.state) task.value = { ...task.peek()!, state: active.state }
  } catch {
    // the next event or reload brings it
  }
}

export async function init() {
  dropOldKeys(kv)
  let s: Session
  try {
    s = await api.session()
  } catch (err) {
    loadError.value = err instanceof Error ? err.message : String(err)
    return
  }
  applySession(s)

  events = connectEvents({
    task: (id) => void onTaskChanged(id),
    tasks: () => void reloadTasks(),
    programs: (id) => void onProgramsChanged(id),
    program: (e) => onProgramEvent(e),
    status: (st) => (conn.value = st),
    robots: (r) => (robots.value = r),
    agent: (a) => (agent.value = a),
    totals: (t) => (totals.value = t),
    models: (m) => {
      if (session.value) session.value = { ...session.value, models: m }
    },
    log: (e) => {
      if (!belongsTo(e, task.peek()?.id)) return
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
  const store = local
  if (!store) return
  saveTimer = window.setTimeout(() => {
    const at = Date.now()
    if (store.saveDraft({ text, base: draftBase.value, savedAt: at })) draftSavedAt.value = at
  }, 300)
  window.clearTimeout(snapTimer)
  snapTimer = window.setTimeout(() => {
    if (shouldAutoSnapshot(history.value[0], draft.value, Date.now())) {
      history.value = store.addSnapshot(draft.value, 'auto')
    }
  }, 2500)
}

export function snapshotNow() {
  if (!draft.value.trim() || !local) return
  history.value = local.addSnapshot(draft.value, 'manual')
  showToast('Zwischenstand in diesem Browser gesichert')
}

/** Replaces the editor content, keeping the previous text in the history. */
export function restore(text: string, label: string) {
  if (draft.value.trim() && draft.value !== text && local) {
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
    if (local) history.value = local.addSnapshot(text, 'deploy', p.version)
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
  local?.saveDraft({ text: draft.value, base: p.version, savedAt: Date.now() })
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
  local?.saveDraft({ text: draft.value, base: p.version, savedAt: Date.now() })
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

/** The result of a by-hand call, with when it ended and how long it took. */
export interface ToolRun {
  result: Record<string, any>
  at: number
  ms: number
}

/** Runs one tool by hand. A refusal (run active, closed task) is shown as a toast. */
export async function runTool(name: string, args: Record<string, unknown>): Promise<ToolRun | null> {
  const start = Date.now()
  try {
    const result = await api.runTool(name, args)
    return { result, at: Date.now(), ms: Date.now() - start }
  } catch (err) {
    fail(err)
    return null
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

/** Accepts a task with the chosen start prompt; the server makes it active and every PC of the team follows. */
export async function acceptTask(t: TeamTask, prompt: PromptChoice, fromTask?: string): Promise<boolean> {
  try {
    await api.accept(t.id, prompt, fromTask)
    showToast(`Aufgabe „${t.title}“ angenommen`)
    await onTaskChanged(t.id)
    return true
  } catch (err) {
    fail(err)
    return false
  }
}

/** Switches to another accepted task. Refused by the server while a run is active. */
export async function switchTask(t: TeamTask) {
  if (t.id === task.value?.id) return
  try {
    await api.activate(t.id)
    await onTaskChanged(t.id)
  } catch (err) {
    fail(err)
  }
}

export async function copyTeamLink() {
  try {
    await navigator.clipboard.writeText(location.origin + location.pathname)
    showToast('Team-Link kopiert. Öffnet ihn auf den anderen Rechnern eures Teams.')
  } catch {
    showToast(`Team-Link: ${location.origin}${location.pathname}`)
  }
}

// ---------- team tools ("eigene Werkzeuge") ----------

/** The team tools of the active task. */
export const teamTools = signal<ProgramInfo[]>([])
/** The left pane shows the prompt or the tool editor. */
export const leftView = signal<'prompt' | 'tool'>('prompt')
/** The team tool open in the editor. */
export const openTool = signal<string | null>(null)
/** The team's running (or last) team tool, from SSE "program" events. */
export const toolRun = signal<RunView>(idleRun)
/** Set by the editor: reload the open tool when another PC saved it. */
export const toolBridge: { reload?: (id: string) => void } = {}

export async function loadTeamTools() {
  try {
    teamTools.value = await api.programs()
  } catch {
    // the next "programs" event or a reload brings it
  }
}

async function onProgramsChanged(id: string) {
  await loadTeamTools()
  if (openTool.peek() === id) {
    if (!teamTools.peek().some((t) => t.id === id)) openTool.value = null
    else toolBridge.reload?.(id)
  }
}

function onProgramEvent(e: ProgramEvent) {
  toolRun.value = reduceRun(toolRun.peek(), e)
}

/** Opens the tool editor on a team tool. */
export function editTool(id: string) {
  openTool.value = id
  leftView.value = 'tool'
}

/** "Neues Werkzeug": an empty tool with only its start block, opened in the editor. */
export async function newTool(): Promise<ProgramDetail | null> {
  const name = freeName(teamTools.peek().map((t) => t.name))
  const blockly = { blocks: { languageVersion: 0, blocks: [{ type: 'program_start', id: 'start', x: 30, y: 30 }] } }
  try {
    const p = await api.createProgram(name, '', blockly)
    await loadTeamTools()
    editTool(p.id)
    return p
  } catch (err) {
    fail(err)
    return null
  }
}

export async function deleteTool(id: string) {
  if (await attempt(api.deleteProgram(id), 'Werkzeug gelöscht')) {
    if (openTool.peek() === id) openTool.value = null
    await loadTeamTools()
  }
}

/** Runs a team tool on a robot ("Ausführen", or "Einzelschritt" with step). */
export async function runTeamTool(id: string, step: boolean, robot?: string): Promise<boolean> {
  try {
    await api.runProgram(id, robot, step)
    return true
  } catch (err) {
    fail(err)
    return false
  }
}

/** "Weiter": the single-step run takes its next call. */
export const continueTeamTool = () => {
  const run = toolRun.peek().run
  return run ? attempt(api.continueProgram(run)) : Promise.resolve(false)
}

/** Saves the open tool a moment after the last change (last write wins). */
export const toolSaver = createSaver<{ id: string; name: string; description: string; blockly: unknown; done?: (p: ProgramDetail) => void }>(
  async (v) => {
    try {
      const p = await api.saveProgram(v.id, v.name, v.description, v.blockly)
      teamTools.value = teamTools.peek().map((t) => (t.id === p.id ? { ...p, blockly: undefined } as ProgramInfo : t))
      v.done?.(p)
    } catch (err) {
      fail(err)
    }
  },
  1500,
)
