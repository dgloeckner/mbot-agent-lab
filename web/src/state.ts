// Application state (Preact signals) and the actions that change it.

import { computed, effect, signal } from '@preact/signals'
import { api, ApiError, connectEvents, NoConnection, type ConnState } from './api'
import { LocalStore, browserKV, dropOldKeys, shouldAutoSnapshot, type Prefs, type Snapshot } from './storage'
import { belongsTo, taskView } from './taskview'
import { createSaver, freeName, idleRun, isEmptyTool, reduceRun, type RunView } from './toolrun'
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
/** The team link is wrong or the team is gone: the page explains it instead of the team page. */
export const unknownTeam = signal(false)
/** When the last live event arrived: robot values are as old as this while the connection is lost. */
export const lastLive = signal(Date.now())
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
/** The warning of the last deploy (names the agent has no tool for); stays until closed or the next deploy. */
export const deployWarning = signal<string | null>(null)

/** Set by the editor so other parts of the page can replace its content or insert text. */
export const editorBridge: {
  replace?: (text: string) => void
  insert?: (text: string) => 'cursor' | 'end'
  focus?: () => void
  /** Shows the editor: the prompt view, tab "Schreiben". */
  show?: () => void
} = {}

/** "In den Prompt einfügen": the prompt comes into view, the name is inserted and flashes, a toast says where. */
export function insertIntoPrompt(name: string) {
  if (!editorBridge.insert) return
  const fromTool = leftView.peek() === 'tool'
  leftView.value = 'prompt'
  editorBridge.show?.()
  // After the prompt is visible, so the editor can scroll to the name.
  requestAnimationFrame(() => {
    const where = editorBridge.insert?.('`' + name + '`')
    const at = where === 'end' ? ' am Ende' : ''
    showToast(`\`${name}\` in den Prompt eingefügt${at}${fromTool ? '. Der Werkzeug-Editor ist weiter unter „Werkzeug bauen“.' : ''}`)
  })
}

export const dirty = computed(() => draft.value !== (deployed.value?.text ?? ''))
/** A team tool differs from the newest version (changed, new or deleted since). */
export const toolsDirty = signal(false)
/** Something to deploy: "Deployen" is the primary button only then. */
export const needsDeploy = computed(() => !deployed.value || dirty.value || toolsDirty.value)

const MAX_LOG = 500
let toastTimer: number | undefined

export function showToast(text: string, kind: 'ok' | 'error' = 'ok', ms = kind === 'error' ? 6000 : 2800) {
  toast.value = { text, kind }
  window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => (toast.value = null), ms)
}

function fail(err: unknown) {
  showToast(err instanceof ApiError ? err.message : 'Etwas ist schiefgegangen. Versucht es noch einmal.', 'error')
}

export function updatePrefs(patch: Partial<Prefs>) {
  prefs.value = { ...prefs.value, ...patch }
  LocalStore.savePrefs(kv, prefs.value)
}

// Seeing the conversation tab marks its answers as read, also after a reload.
effect(() => {
  if (prefs.value.sideTab !== 'conversation') return
  conversationUnread.value = 0
  const id = task.value?.id
  const last = conversation.value[conversation.value.length - 1]
  if (id && last && (prefs.peek().talkSeen[id] ?? '') < last.at) {
    // Keep only a few tasks: the map is per browser and never needs old ones.
    const seen = Object.fromEntries(Object.entries({ ...prefs.peek().talkSeen, [id]: last.at }).slice(-10))
    updatePrefs({ talkSeen: seen })
  }
})

const pageOpened = Date.now()

/** Lab events a team must notice on any tab: they also show as a toast. */
const LAB_NOTICES = new Set(['task_assigned', 'task_withdrawn', 'task_closed', 'robot_added', 'robot_removed'])

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
    unknownTeam.value = err instanceof ApiError && err.status === 404
    loadError.value = err instanceof Error ? err.message : String(err)
    return
  }
  applySession(s)

  events = connectEvents({
    task: (id) => void onTaskChanged(id),
    tasks: () => void reloadTasks(),
    programs: (id) => void onProgramsChanged(id),
    program: (e) => onProgramEvent(e),
    status: (st, at) => {
      conn.value = st
      lastLive.value = at
    },
    robots: (r) => (robots.value = r),
    agent: (a) => (agent.value = a),
    totals: (t) => (totals.value = t),
    models: (m) => {
      if (session.value) session.value = { ...session.value, models: m }
    },
    log: (e) => {
      if (!belongsTo(e, task.peek()?.id)) return
      if (e.channel === 'conversation') {
        const seen = prefs.peek().talkSeen[task.peek()?.id ?? ''] ?? ''
        if (append(conversation, e) && e.source === 'agent' && e.kind === 'text' && prefs.value.sideTab !== 'conversation' && e.at > seen) {
          conversationUnread.value++
        }
        return
      }
      if (!append(log, e)) return
      // Only what happened while the page is open: a reload replays older lines.
      if (LAB_NOTICES.has(e.kind) && e.text && Date.parse(e.at) >= pageOpened) showToast(e.text, 'ok', 6000)
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
  const promptChanged = deployed.peek() !== null && text !== deployed.peek()!.text
  try {
    await toolSaver.flush() // a version holds the team tools as they are now
    const p = await api.deploy(text, restart)
    if (p.unchanged) {
      // Same text and team tools as the newest version: the server created nothing.
      showToast(`Nichts geändert seit Version ${p.version}${restart ? '. Ein neuer Lauf mit dieser Version beginnt.' : '.'}`)
      return
    }
    applyDeployed({ version: p.version, created_at: p.created_at, chars: p.chars, text })
    if (local) history.value = local.addSnapshot(text, 'deploy', p.version)
    const withTools = p.tools.filter((t) => t.agent).length
    // Say what it shipped: a deploy from the tool editor also ships the prompt draft, unseen.
    const parts = [promptChanged && 'geändertem Prompt', withTools === 1 ? '1 eigenen Werkzeug' : withTools ? `${withTools} eigenen Werkzeugen` : '']
    const what = parts.filter(Boolean).join(' und ')
    const done = `Version ${p.version} deployt${what ? ` mit ${what}` : ''}`
    deployWarning.value = p.warning || null
    showToast(done)
    void reloadVersions()
    void loadTeamTools()
  } catch (err) {
    fail(err)
  } finally {
    busy.value = null
  }
}

async function reloadVersions() {
  try {
    versions.value = await api.versions()
  } catch {
    // the next deploy or reload brings it
  }
}

/**
 * "Diese Version wiederherstellen": the working state becomes that version:
 * the team tools on the server, the prompt text in the draft. The history
 * keeps everything; the next deploy is a new version.
 */
export async function restoreVersion(version: number): Promise<boolean> {
  try {
    await toolSaver.flush()
    toolSaver.cancel()
    const p = await api.restoreVersion(version)
    restore(p.text ?? '', `Version ${version}`)
    await loadTeamTools()
    if (openTool.peek()) toolBridge.reload?.(openTool.peek()!)
    return true
  } catch (err) {
    fail(err)
    return false
  }
}

/** What restoring a version would overwrite: the changed prompt draft and team tools changed or new since the last deploy. */
export function unsavedWork(): string[] {
  const out: string[] = []
  if (dirty.value) out.push('die Änderungen am Prompt seit dem letzten Deploy')
  for (const t of teamTools.value) {
    if (t.deploy === 'changed') out.push(`die Änderungen an „${t.name}“`)
    if (t.deploy === 'new') out.push(`das neue Werkzeug „${t.name}“`)
  }
  return out
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

/** Said when a stop cannot reach the server: nobody may believe the robots stopped. */
export const NOT_STOPPED =
  'Keine Verbindung zum Server: Eure Roboter wurden NICHT angehalten. Hebt den Roboter hoch und sagt der Lab-Leitung Bescheid.'

export async function stopAll() {
  busy.value = 'stop'
  // Without a live connection, say it at once; the request may hang until its timeout.
  if (conn.peek() !== 'live') showToast(NOT_STOPPED, 'error', 12_000)
  try {
    const running = agent.peek().running
    await api.stop()
    showToast(running ? 'Alle Roboter angehalten, der Lauf ist beendet.' : 'Alle Roboter angehalten.')
  } catch (err) {
    if (err instanceof NoConnection) showToast(NOT_STOPPED, 'error', 12_000)
    else fail(err)
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
/** The tool editor takes the full width; the side pane hides for a while. */
export const wideTool = signal(false)
/** The team tool open in the editor. */
export const openTool = signal<string | null>(null)
/** The team's running (or last) team tool, from SSE "program" events. */
export const toolRun = signal<RunView>(idleRun)
/** Set by the editor: reload the open tool when another PC saved it. */
export const toolBridge: { reload?: (id: string) => void } = {}

export async function loadTeamTools() {
  try {
    teamTools.value = await api.programs()
    // After a reload the editor opens the tool the team had open in this task.
    const remembered = prefs.peek().openTools[task.peek()?.id ?? '']
    if (!openTool.peek() && remembered && teamTools.peek().some((t) => t.id === remembered)) openTool.value = remembered
    const v = deployed.peek()?.version
    // Also counts tools deleted since the newest version, which the list cannot show.
    toolsDirty.value = v ? (await api.compareTools(v, 0)).tools.length > 0 : teamTools.peek().length > 0
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

/** Opens the tool editor on a team tool; the page remembers it per task for the next load. */
export function editTool(id: string) {
  openTool.value = id
  leftView.value = 'tool'
  const t = task.peek()?.id
  if (t && prefs.peek().openTools[t] !== id) {
    updatePrefs({ openTools: Object.fromEntries(Object.entries({ ...prefs.peek().openTools, [t]: id }).slice(-10)) })
  }
}

/**
 * "Neues Werkzeug": an empty tool with only its start block, opened in the
 * editor. An untouched new tool (default name, no description, no blocks)
 * is opened again instead of making another one.
 */
export async function newTool(): Promise<ProgramDetail | null> {
  const untouched = teamTools.peek().find((t) => /^werkzeug_\d+$/.test(t.name) && !t.description && isEmptyTool(t.problems))
  if (untouched) {
    editTool(untouched.id)
    return null
  }
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
