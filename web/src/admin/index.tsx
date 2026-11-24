import { render } from 'preact'
import { useEffect, useState } from 'preact/hooks'
import { signal } from '@preact/signals'
import { COLOR_HEX, colorName, formatPrices, formatUsage, onColor, relativeTime, skillName } from '../format'
import type { AgentStatus, ModelOption, RobotState, Totals } from '../types'
import { lazyComponent } from '../lazy'
import { ALL_TEAMS, STATUS_LABEL, assignBody, assignedText, groupTasks, teamsWithout, type AdminTask, type AdminTeamTask, type TaskDetail } from './tasks'
import './admin.css'

const TaskEditor = lazyComponent(() => import('./TaskEditor'))

interface AdminRobot {
  id: string
  color: string
  online: boolean
  accepted: boolean
  script: string
  mem_free: number
  team: string
  alias: string
  state: RobotState
}

interface AdminTeam {
  id: string
  name: string
  color: string
  link: string
  connected: number
  created_at: string
  provider: string // model provider of the team's next run
  agent: AgentStatus // current or last run
  total: Totals // all runs and conversations
  tasks: AdminTeamTask[] // assigned tasks and where the team stands
}

interface AdminState {
  lab_code: string
  models: ModelOption[]
  conversation: boolean // teams may talk with their agent while no run is active
  teams: AdminTeam[]
  robots: AdminRobot[]
  tasks: AdminTask[]
}

const AGENT_LABEL = { waiting: 'wartet', thinking: 'denkt nach', acting: 'handelt' } as const

const state = signal<AdminState | null>(null)
const auth = signal<'checking' | 'login' | 'ok'>('checking')
const live = signal(false)
const toast = signal<{ text: string; error: boolean } | null>(null)
let toastTimer: number | undefined

function say(text: string, error = false) {
  toast.value = { text, error }
  window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => (toast.value = null), error ? 6000 : 2600)
}

async function call(method: string, path: string, body?: unknown): Promise<any> {
  let resp: Response
  try {
    resp = await fetch(`/admin/api${path}`, {
      method,
      headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch {
    throw new Error('Keine Verbindung zum Server.')
  }
  if (resp.status === 401) {
    auth.value = 'login'
    throw new Error('Bitte meldet euch neu an.')
  }
  if (resp.status === 204) return null
  const data = await resp.json().catch(() => null)
  if (!resp.ok) throw new Error(data?.error ?? `Fehler ${resp.status}`)
  return data
}

async function act(p: Promise<unknown>, ok?: string) {
  try {
    await p
    if (ok) say(ok)
    // Show the result right away instead of waiting for the next live update.
    if (auth.value === 'ok') state.value = await call('GET', '/state')
  } catch (e) {
    say((e as Error).message, true)
  }
}

let es: EventSource | null = null
function connect() {
  es?.close()
  es = new EventSource('/admin/api/events')
  es.addEventListener('open', () => (live.value = true))
  es.addEventListener('error', () => {
    live.value = false
    // A closed stream usually means the session expired: check and show the login if so.
    if (es?.readyState === EventSource.CLOSED) void start()
  })
  es.addEventListener('state', (ev) => (state.value = JSON.parse((ev as MessageEvent).data)))
}

async function start() {
  try {
    state.value = await call('GET', '/state')
    auth.value = 'ok'
    connect()
  } catch {
    if (auth.value !== 'login') window.setTimeout(start, 3000)
  }
}

const assign = (robot: string, team: string) => {
  const r = state.value?.robots.find((x) => x.id === robot)
  if (!r || r.team === team) return
  const tname = state.value?.teams.find((t) => t.id === team)?.name
  void act(call('POST', '/assign', { robot, team }), team ? `${robot} gehört jetzt ${tname}` : `${robot} ist frei`)
}

// --- tasks ------------------------------------------------------------------

/** The task open in the editor dialog. */
const editing = signal<TaskDetail | null>(null)

async function openTask(id: string) {
  try {
    editing.value = await call('GET', `/tasks/${id}`)
  } catch (e) {
    say((e as Error).message, true)
  }
}

async function newTask() {
  try {
    editing.value = await call('POST', '/tasks', { title: 'Neue Aufgabe', body: '', starter: '' })
    void act(Promise.resolve())
  } catch (e) {
    say((e as Error).message, true)
  }
}

/** Runs a task action from the dialog; true when it worked. */
async function taskAction(p: Promise<unknown>, ok: string): Promise<boolean> {
  try {
    await p
    say(ok)
    if (auth.value === 'ok') state.value = await call('GET', '/state')
    return true
  } catch (e) {
    say((e as Error).message, true)
    return false
  }
}

function assignTask(task: string, target: string) {
  const s = state.value
  const t = s?.tasks.find((x) => x.id === task)
  if (!s || !t) return
  if (target === ALL_TEAMS && !confirm(`„${t.title}“ allen ${s.teams.length} Teams zuweisen? Teams, die später beitreten, bekommen sie nicht automatisch.`)) return
  const names = Object.fromEntries(s.teams.map((x) => [x.id, x.name]))
  void call('POST', `/tasks/${task}/assign`, assignBody(target))
    .then((r: { assigned: string[] }) => act(Promise.resolve(), assignedText(t.title, r.assigned, names)))
    .catch((e: Error) => say(e.message, true))
}

function closeTask(t: { id: string; title: string }) {
  if (!confirm(`„${t.title}“ schließen? Laufende Läufe in dieser Aufgabe enden sofort. Danach kann kein Team darin mehr deployen oder starten; alles bleibt lesbar.`)) return false
  void taskAction(call('POST', `/tasks/${t.id}/close`), `„${t.title}“ geschlossen`).then((ok) => ok && (editing.value = null))
  return true
}

const modelName = (provider: string) => state.value?.models.find((m) => m.provider === provider)?.model ?? provider

/** Releases a model for teams or takes it back. Teams using a revoked model go back to the default. */
function release(provider: string, on: boolean) {
  const s = state.value!
  const allowed = s.models.filter((m) => !m.default && (m.provider === provider ? on : m.allowed)).map((m) => m.provider)
  const users = s.teams.filter((t) => t.provider === provider).length
  if (!on && users > 0) {
    const def = s.models.find((m) => m.default)?.model ?? 'das Standardmodell'
    if (!confirm(`${modelName(provider)} sperren? ${users === 1 ? '1 Team nutzt' : `${users} Teams nutzen`} es und ${users === 1 ? 'wechselt' : 'wechseln'} ab dem nächsten Lauf zu ${def}.`)) return
  }
  void act(call('PUT', '/models', { allowed }), on ? `${modelName(provider)} freigegeben` : `${modelName(provider)} gesperrt`)
}

// ---------------------------------------------------------------------------

function Login() {
  const [pw, setPw] = useState('')
  const [err, setErr] = useState<string | null>(null)
  return (
    <main class="admin-login">
      <form
        onSubmit={async (ev) => {
          ev.preventDefault()
          setErr(null)
          try {
            await call('POST', '/login', { password: pw })
            setPw('')
            await start()
          } catch (e) {
            setErr((e as Error).message)
          }
        }}
      >
        <h1>Leitstand</h1>
        <label>
          <span>Passwort</span>
          <input type="password" value={pw} autoComplete="current-password" onInput={(e) => setPw(e.currentTarget.value)} autoFocus required />
        </label>
        {err && (
          <p class="form-error" role="alert">
            {err}
          </p>
        )}
        <button class="btn btn-accent" type="submit">
          Anmelden
        </button>
      </form>
    </main>
  )
}

// robotInfo is the chip's tooltip: script version and free memory from hello.
function robotInfo(r: AdminRobot): string | undefined {
  if (!r.accepted) return undefined
  const mem = r.mem_free >= 0 ? `, freier Speicher ${Math.round(r.mem_free / 1024)} KB` : ''
  return `Skript ${r.script}${mem}`
}

function RobotChip({ r, teams, inTeam }: { r: AdminRobot; teams: AdminTeam[]; inTeam: boolean }) {
  const status = !r.online ? 'offline' : !r.accepted ? 'falsche Version' : r.state.tilted ? 'umgekippt' : `${r.state.battery} %`
  return (
    <li
      class="chip-robot"
      data-online={r.online && r.accepted}
      draggable
      onDragStart={(ev) => {
        ev.dataTransfer!.setData('text/robot', r.id)
        ev.dataTransfer!.effectAllowed = 'move'
      }}
    >
      <span class="robot-color" style={{ background: COLOR_HEX[r.color] ?? '#999' }} title={`Aufkleberfarbe: ${colorName(r.color)}`} />
      <span class="chip-main" title={robotInfo(r)}>
        <strong>{inTeam && r.alias ? r.alias : r.id}</strong>
        <span>
          {inTeam && r.alias ? `${r.id}, ` : ''}
          {status}
          {r.online && r.accepted ? `, ${skillName(r.state.skill)}` : ''}
        </span>
      </span>
      <select
        class="chip-move"
        aria-label={`${r.id} zuteilen`}
        value={r.team}
        onChange={(e) => assign(r.id, e.currentTarget.value)}
      >
        <option value="">frei</option>
        {teams.map((t) => (
          <option key={t.id} value={t.id}>
            {t.name}
          </option>
        ))}
      </select>
    </li>
  )
}

function DropZone(props: { team: string; class: string; children: preact.ComponentChildren; label: string }) {
  const [over, setOver] = useState(false)
  return (
    <section
      class={`${props.class}${over ? ' drop-over' : ''}`}
      aria-label={props.label}
      onDragOver={(ev) => {
        const types = ev.dataTransfer?.types ?? []
        if (types.includes('text/robot') || (props.team && types.includes('text/task'))) {
          ev.preventDefault()
          setOver(true)
        }
      }}
      onDragLeave={() => setOver(false)}
      onDrop={(ev) => {
        ev.preventDefault()
        setOver(false)
        const id = ev.dataTransfer?.getData('text/robot')
        if (id) assign(id, props.team)
        const task = ev.dataTransfer?.getData('text/task')
        if (task && props.team) assignTask(task, props.team)
      }}
    >
      {props.children}
    </section>
  )
}

// Same palette and order as lab.TeamColors on the server.
const TEAM_COLORS = ['blue', 'green', 'orange', 'purple', 'red', 'cyan', 'yellow']

function TeamCard({ t, robots, teams }: { t: AdminTeam; robots: AdminRobot[]; teams: AdminTeam[] }) {
  const [editing, setEditing] = useState(false)
  const [name, setName] = useState(t.name)
  const accent = COLOR_HEX[t.color] ?? COLOR_HEX.blue
  const mine = robots.filter((r) => r.team === t.id)
  return (
    <DropZone team={t.id} class="team-card" label={`Team ${t.name}`}>
      <header class="team-head" style={{ background: accent, color: onColor(accent) }}>
        {editing ? (
          <form
            class="rename"
            onSubmit={async (ev) => {
              ev.preventDefault()
              await act(call('PUT', `/teams/${t.id}`, { name }), 'Team umbenannt')
              setEditing(false)
            }}
          >
            <input value={name} maxLength={30} onInput={(e) => setName(e.currentTarget.value)} aria-label="Teamname" autoFocus />
            <button class="btn btn-small" type="submit">
              Speichern
            </button>
          </form>
        ) : (
          <h2>{t.name}</h2>
        )}
        {!editing && (
          <span class="presence" title="Offene Teamseiten">
            {t.connected === 0 ? 'nicht verbunden' : t.connected === 1 ? '1 Browser' : `${t.connected} Browser`}
          </span>
        )}
      </header>
      {editing && (
        <div class="team-colors" role="radiogroup" aria-label={`Teamfarbe von ${t.name}`}>
          <span>Teamfarbe</span>
          {TEAM_COLORS.map((c) => (
            <button
              key={c}
              type="button"
              role="radio"
              aria-checked={c === t.color}
              aria-label={colorName(c)}
              title={colorName(c)}
              class="team-swatch"
              style={{ background: COLOR_HEX[c] }}
              onClick={() => c !== t.color && act(call('PUT', `/teams/${t.id}/color`, { color: c }), `Teamfarbe ${colorName(c)}`)}
            />
          ))}
        </div>
      )}
      <TeamTasks t={t} />
      <ul class="robot-list">
        {mine.length === 0 ? (
          <li class="drop-hint">Roboter hierher ziehen oder unten auswählen</li>
        ) : (
          mine.map((r) => <RobotChip key={r.id} r={r} teams={teams} inTeam />)
        )}
      </ul>
      <footer class="team-foot">
        <select
          aria-label={`Roboter zu ${t.name} hinzufügen`}
          value=""
          onChange={(e) => {
            if (e.currentTarget.value) assign(e.currentTarget.value, t.id)
            e.currentTarget.value = ''
          }}
        >
          <option value="">Roboter hinzufügen …</option>
          {robots
            .filter((r) => r.team !== t.id)
            .map((r) => (
              <option key={r.id} value={r.id}>
                {r.id}
                {r.team ? ` (bei ${teams.find((x) => x.id === r.team)?.name ?? '?'})` : ' (frei)'}
              </option>
            ))}
        </select>
        <span class="spacer" />
        <button
          class="btn-link"
          onClick={() =>
            act(
              navigator.clipboard.writeText(location.origin + t.link),
              'Team-Link kopiert',
            )
          }
        >
          Link kopieren
        </button>
        <button class="btn-link" onClick={() => (setName(t.name), setEditing(!editing))}>
          {editing ? 'Fertig' : 'Bearbeiten'}
        </button>
        <button
          class="btn-link danger"
          onClick={() => {
            if (confirm(`Team „${t.name}“ löschen? Seine Roboter werden frei, und der Team-Link funktioniert nicht mehr.`)) {
              void act(call('DELETE', `/teams/${t.id}`), `Team ${t.name} gelöscht`)
            }
          }}
        >
          Löschen
        </button>
      </footer>
      <p class="team-agent" data-running={t.agent.running}>
        {t.agent.running ? `Agent ${AGENT_LABEL[t.agent.state]}` : t.agent.state !== 'waiting' ? 'Kein Lauf, Agent im Gespräch' : 'Kein Lauf'}
        {t.agent.calls > 0 &&
          `, ${t.agent.running ? 'dieser' : 'letzter'} Lauf: ${formatUsage(t.agent.tokens_in + t.agent.tokens_out, t.agent.cost_micro_eur)}`}
      </p>
      {(t.total.runs > 0 || t.total.conversation.count > 0) && (
        <p class="team-model">
          Gesamt: {formatUsage(t.total.tokens_in + t.total.tokens_out, t.total.cost_micro_eur)} in{' '}
          {t.total.runs === 1 ? '1 Lauf' : `${t.total.runs} Läufen`}
          {t.total.conversation.count > 0 &&
            `, davon ${formatUsage(t.total.conversation.tokens_in + t.total.conversation.tokens_out, t.total.conversation.cost_micro_eur)} in ${
              t.total.conversation.count === 1 ? '1 Gespräch' : `${t.total.conversation.count} Gesprächen`
            }`}
        </p>
      )}
      <p class="team-model">
        Modell {modelName(t.provider)}
        {t.agent.running && t.agent.provider && t.agent.provider !== t.provider && ` (Lauf noch mit ${t.agent.model})`}
      </p>
      <p class="team-age">Beigetreten {relativeTime(t.created_at)}</p>
    </DropZone>
  )
}

function TeamTasks({ t }: { t: AdminTeam }) {
  if (t.tasks.length === 0) return <p class="team-tasks-empty">Noch keine Aufgabe. Zieht eine offene Aufgabe hierher.</p>
  return (
    <ul class="team-tasks" aria-label={`Aufgaben von ${t.name}`}>
      {t.tasks.map((x) => (
        <li key={x.id} data-status={x.status} data-closed={x.state === 'closed'}>
          <span class="team-task-title">{x.title}</span>
          <span class="team-task-status">
            {STATUS_LABEL[x.status]}
            {x.state === 'closed' ? ', geschlossen' : ''}
          </span>
          {x.status === 'assigned' && x.state !== 'closed' && (
            <button
              class="btn-link"
              aria-label={`${x.title} von ${t.name} zurückziehen`}
              onClick={() => act(call('DELETE', `/tasks/${x.id}/assign/${t.id}`), `„${x.title}“ zurückgezogen`)}
            >
              Zurückziehen
            </button>
          )}
        </li>
      ))}
    </ul>
  )
}

function TaskItem({ task, teams }: { task: AdminTask; teams: AdminTeam[] }) {
  const open = task.state === 'open'
  const choices = open ? teamsWithout(teams, task.id) : []
  return (
    <li
      class="task-item"
      data-state={task.state}
      draggable={open}
      onDragStart={(ev) => {
        ev.dataTransfer!.setData('text/task', task.id)
        ev.dataTransfer!.effectAllowed = 'copy'
      }}
    >
      <button class="task-open" onClick={() => openTask(task.id)} title={task.state === 'draft' ? 'Bearbeiten' : 'Ansehen'}>
        {task.title}
      </button>
      {open && choices.length > 0 && (
        <select
          class="chip-move"
          aria-label={`${task.title} zuweisen`}
          value=""
          onChange={(e) => {
            const v = e.currentTarget.value
            e.currentTarget.value = ''
            if (v) assignTask(task.id, v)
          }}
        >
          <option value="">Zuweisen …</option>
          {choices.length > 0 && <option value={ALL_TEAMS}>An alle Teams</option>}
          {choices.map((t) => (
            <option key={t.id} value={t.id}>
              {t.name}
            </option>
          ))}
        </select>
      )}
    </li>
  )
}

function TasksPanel({ tasks, teams }: { tasks: AdminTask[]; teams: AdminTeam[] }) {
  const groups = groupTasks(tasks)
  return (
    <section class="models-panel tasks-panel" aria-label="Aufgaben">
      <div class="panel-head">
        <h2>Aufgaben</h2>
        <button class="btn btn-small btn-accent" onClick={newTask}>
          Neue Aufgabe
        </button>
      </div>
      <p class="pool-sub">Offene Aufgaben auf ein Team ziehen oder über „Zuweisen“ verteilen. Den Aufgabentext sehen nur die Teams, nie der Agent.</p>
      {groups.length === 0 && <p class="drop-hint">Noch keine Aufgaben.</p>}
      {groups.map((g) => (
        <div key={g.state} class="task-group">
          <h3>
            {g.label} <span class="count">{g.tasks.length}</span>
          </h3>
          <ul class="task-list">
            {g.tasks.map((t) => (
              <TaskItem key={t.id} task={t} teams={teams} />
            ))}
          </ul>
        </div>
      ))}
    </section>
  )
}

function ModelsPanel({ models, teams }: { models: ModelOption[]; teams: AdminTeam[] }) {
  return (
    <section class="models-panel" aria-label="Modelle">
      <h2>Modelle</h2>
      <p class="pool-sub">Teams wählen auf ihrer Seite unter den freigegebenen Modellen. Ein Wechsel gilt ab dem nächsten Lauf.</p>
      <ul class="model-list">
        {models.map((m) => {
          const users = teams.filter((t) => t.provider === m.provider).length
          return (
            <li key={m.provider}>
              <label>
                <input type="checkbox" checked={m.allowed} disabled={m.default} onChange={(e) => release(m.provider, e.currentTarget.checked)} />
                <span class="model-main">
                  <strong>{m.model}</strong>
                  <span>
                    {m.provider}
                    {m.default ? ', Standard' : ''}, {formatPrices(m.price_in, m.price_out)}
                    {users > 0 && `, ${users === 1 ? '1 Team' : `${users} Teams`}`}
                  </span>
                </span>
              </label>
            </li>
          )
        })}
      </ul>
    </section>
  )
}

function ConversationPanel({ on }: { on: boolean }) {
  return (
    <section class="models-panel" aria-label="Gespräch">
      <h2>Gespräch</h2>
      <p class="pool-sub">Ohne Lauf können Teams ihrem Agenten Fragen stellen. Er bewegt dabei keinen Roboter. Die Kosten zählen zum Team.</p>
      <ul class="model-list">
        <li>
          <label>
            <input
              type="checkbox"
              checked={on}
              onChange={(e) => {
                const next = e.currentTarget.checked
                void act(call('PUT', '/conversation', { on: next }), next ? 'Gespräche eingeschaltet' : 'Gespräche abgeschaltet')
              }}
            />
            <span class="model-main">
              <strong>Gespräch ohne Lauf erlauben</strong>
              <span>Für Wettkämpfe abschalten.</span>
            </span>
          </label>
        </li>
      </ul>
    </section>
  )
}

function Board() {
  const s = state.value!
  const free = s.robots.filter((r) => !r.team)
  const joinUrl = `${location.host}/join`
  const online = s.robots.filter((r) => r.online).length
  return (
    <div class="admin">
      <header class="admin-bar">
        <div class="admin-title">
          <h1>Leitstand</h1>
          <span class={live.value ? 'live on' : 'live'}>{live.value ? 'live' : 'verbinde …'}</span>
        </div>
        <div class="labcode" aria-label="Lab-Code">
          <span class="labcode-label">
            Teams gründen auf <strong>{joinUrl}</strong> mit dem Code
          </span>
          <span class="labcode-value">{s.lab_code}</span>
          <button
            class="btn btn-quiet btn-small"
            onClick={() => {
              if (confirm('Neuen Lab-Code erzeugen? Der alte gilt dann nicht mehr. Teams, die schon beigetreten sind, behalten ihren Link.')) {
                void act(call('POST', '/lab-code'), 'Neuer Lab-Code')
              }
            }}
          >
            Neuer Code
          </button>
        </div>
        <div class="admin-actions">
          <button class="btn btn-stop-all" onClick={() => act(call('POST', '/stop-all'), 'Alle Roboter angehalten')}>
            Alle anhalten
          </button>
          <button class="btn btn-quiet" onClick={() => act(call('POST', '/logout').then(() => (auth.value = 'login')))}>
            Abmelden
          </button>
        </div>
      </header>

      <main class="admin-main">
        <div class="side">
          <DropZone team="" class="pool" label="Freie Roboter">
            <h2>Freie Roboter</h2>
            <p class="pool-sub">
              {online} von {s.robots.length} online. Zieht einen Roboter auf ein Team.
            </p>
            <ul class="robot-list">
              {free.length === 0 ? <li class="drop-hint">Alle Roboter sind zugeteilt. Hierher ziehen gibt einen frei.</li> : free.map((r) => <RobotChip key={r.id} r={r} teams={s.teams} inTeam={false} />)}
            </ul>
          </DropZone>
          <TasksPanel tasks={s.tasks} teams={s.teams} />
          <ModelsPanel models={s.models} teams={s.teams} />
          <ConversationPanel on={s.conversation} />
        </div>

        <section class="teams" aria-label="Teams">
          {s.teams.length === 0 ? (
            <div class="teams-empty">
              <h2>Noch keine Teams</h2>
              <p>
                Teams öffnen <strong>{joinUrl}</strong>, geben den Lab-Code <strong>{s.lab_code}</strong> und ihren Namen ein. Sie erscheinen
                dann hier.
              </p>
            </div>
          ) : (
            s.teams.map((t) => <TeamCard key={t.id} t={t} robots={s.robots} teams={s.teams} />)
          )}
        </section>
      </main>
    </div>
  )
}

function AdminApp() {
  useEffect(() => {
    void start()
    return () => es?.close()
  }, [])
  return (
    <>
      {auth.value === 'login' ? <Login /> : auth.value === 'ok' && state.value ? <Board /> : <main class="fullpage-message" aria-busy="true" />}
      {editing.value && <TaskDialog task={editing.value} />}
      {toast.value && (
        <div class={`toast${toast.value.error ? ' toast-error' : ''}`} role={toast.value.error ? 'alert' : 'status'}>
          {toast.value.text}
        </div>
      )}
    </>
  )
}

function TaskDialog({ task }: { task: TaskDetail }) {
  const done = () => (editing.value = null)
  return (
    <TaskEditor
      key={task.id}
      task={task}
      onClose={done}
      onSave={(t) => taskAction(call('PUT', `/tasks/${task.id}`, t), 'Entwurf gespeichert')}
      onPublish={(t) =>
        taskAction(
          call('PUT', `/tasks/${task.id}`, t).then(() => call('POST', `/tasks/${task.id}/publish`)),
          `„${t.title}“ veröffentlicht`,
        )
      }
      onDuplicate={async () => {
        try {
          const copy: TaskDetail = await call('POST', `/tasks/${task.id}/duplicate`)
          editing.value = copy
          void act(Promise.resolve(), 'Kopie als Entwurf angelegt')
        } catch (e) {
          say((e as Error).message, true)
        }
      }}
      onCloseTask={() => closeTask(task)}
      onDelete={() => {
        if (confirm(`Entwurf „${task.title}“ löschen?`)) void taskAction(call('DELETE', `/tasks/${task.id}`), 'Entwurf gelöscht').then((ok) => ok && done())
      }}
    />
  )
}

export function mount(el: HTMLElement) {
  document.title = 'Leitstand: mBot2 Agent Lab'
  render(<AdminApp />, el)
}
