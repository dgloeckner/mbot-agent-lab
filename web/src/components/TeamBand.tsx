import { agent, busy, chooseModel, conn, copyTeamLink, lastLive, deployed, identifyRobot, prefs, readOnly, renameRobot, renameTeam, robots, session, startRun, stopAll, task, totals, updatePrefs } from '../state'
import { TaskPicker } from './TaskPicker'
import { COLOR_HEX, colorName, formatPrices, formatUsage, skillName } from '../format'
import type { Robot } from '../types'
import { AutoIcon, LinkIcon, MoonIcon, PlayIcon, StopIcon, SunIcon } from './icons'
import { InlineEdit } from './InlineEdit'
import { useEffect, useState } from 'preact/hooks'
import type { Theme } from '../storage'

const AGENT_LABEL = { waiting: 'wartet', thinking: 'denkt nach', acting: 'handelt' } as const

const NEXT_THEME: Record<Theme, Theme> = { system: 'light', light: 'dark', dark: 'system' }
const THEME_LABEL: Record<Theme, string> = { system: 'wie System', light: 'hell', dark: 'dunkel' }

/** Seconds since the last live event, ticking while the connection is lost. */
function useStaleSeconds(): number | null {
  const lost = conn.value === 'reconnecting'
  const [now, setNow] = useState(Date.now())
  useEffect(() => {
    if (!lost) return
    setNow(Date.now())
    const t = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(t)
  }, [lost])
  return lost ? Math.max(0, Math.round((now - lastLive.value) / 1000)) : null
}

function RobotCard({ r, stale }: { r: Robot; stale: number | null }) {
  const s = r.state
  const status =
    stale !== null ? `Stand vor ${stale} s` : !r.online ? 'offline' : !r.accepted ? 'falsche Version' : s.tilted ? 'umgekippt' : 'online'
  const near = r.online && s.distance_cm < 15
  const gauge = Math.max(0, Math.min(1, s.distance_cm / 100))
  return (
    <div
      class="robot"
      data-status={stale !== null ? 'stale' : r.online && r.accepted ? (s.tilted ? 'warn' : 'ok') : 'off'}
      title={stale !== null ? 'Keine Verbindung zum Server: Diese Werte sind alt.' : undefined}
    >
      <div class="robot-name">
        <InlineEdit class="robot-alias" value={r.alias} label="Name des Roboters" maxLength={16} onSave={(v) => renameRobot(r.id, v)} />
        <span class="robot-id">
          {r.id}
          <button class="robot-find" onClick={() => identifyRobot(r.id, r.alias)} disabled={!r.online} title="Lässt den Roboter blinken und piepen">
            Finden
          </button>
        </span>
      </div>
      <dl class="robot-facts">
        <div title="Status">
          <dt>Status</dt>
          <dd><span class="dot" />{status}</dd>
        </div>
        <div class={near ? 'near' : undefined} title="Abstand">
          <dt>Abstand</dt>
          <dd>
            <span class="gauge" style={{ '--v': gauge }} aria-hidden="true" />
            {r.online ? `${s.distance_cm} cm` : '–'}
          </dd>
        </div>
        <div title="Boden">
          <dt>Boden</dt>
          <dd>
            <span class="swatch" style={{ background: COLOR_HEX[s.color] ?? 'transparent' }} aria-hidden="true" />
            {r.online ? colorName(s.color || 'none') : '–'}
          </dd>
        </div>
        <div title="Akku">
          <dt>Akku</dt>
          <dd>{r.online ? `${s.battery} %` : '–'}</dd>
        </div>
        <div title="Macht">
          <dt>Macht</dt>
          <dd class="mono">{r.online ? skillName(s.skill) : '–'}</dd>
        </div>
      </dl>
    </div>
  )
}

/** The language model of the team's next run; a choice if the control room released several. */
function ModelPick() {
  const { selected, options } = session.value!.models
  const a = agent.value
  const current = options.find((o) => o.provider === selected)
  const pending = a.running && a.provider && a.provider !== selected
  return (
    <p class="model-pick">
      {options.length > 1 ? (
        <label title="Gilt ab dem nächsten Lauf. Die Preise zeigen, was eine Million Token kostet.">
          <span>Modell</span>
          <select value={selected} onChange={(e) => chooseModel(e.currentTarget.value)}>
            {options.map((o) => (
              <option key={o.provider} value={o.provider}>
                {o.model} ({formatPrices(o.price_in, o.price_out)})
              </option>
            ))}
          </select>
        </label>
      ) : (
        <span title={current ? formatPrices(current.price_in, current.price_out) : undefined}>Modell {current?.model ?? selected}</span>
      )}
      {pending && <span class="model-pending">ab dem nächsten Lauf, jetzt noch {a.model}</span>}
    </p>
  )
}

/** One line of token use: the current run while it runs, else the team's total; all numbers in the tooltip. */
function Usage() {
  const a = agent.value
  const t = totals.value
  const run = a.calls > 0 ? `${a.running ? 'dieser Lauf' : 'letzter Lauf'}: ${formatUsage(a.tokens_in + a.tokens_out, a.cost_micro_eur)}` : ''
  const total = t.runs > 1 || t.conversation.count > 0 ? `gesamt: ${formatUsage(t.tokens_in + t.tokens_out, t.cost_micro_eur)}` : ''
  const talk =
    t.conversation.count > 0
      ? `davon Gespräche: ${formatUsage(t.conversation.tokens_in + t.conversation.tokens_out, t.conversation.cost_micro_eur)}`
      : ''
  const shown = a.running ? run : total || run
  if (!shown) return null
  const details = [run && a.model ? `${run} (Modell ${a.model}, ${a.calls} Aufrufe)` : run, total, talk].filter(Boolean).join('\n')
  return (
    <span class="run-usage" title={details}>
      {shown}
    </span>
  )
}

export function TeamBand() {
  const s = session.value!
  const theme = prefs.value.theme
  const ThemeIcon = theme === 'light' ? SunIcon : theme === 'dark' ? MoonIcon : AutoIcon
  const a = agent.value
  const stale = useStaleSeconds()
  const startHint = !s.agent_available
    ? 'Es ist kein Sprachmodell eingerichtet.'
    : !task.value
      ? 'Nehmt zuerst eine Aufgabe an.'
      : readOnly.value
        ? 'Die Aufgabe ist geschlossen.'
        : !deployed.value
      ? 'Deployt zuerst einen Prompt.'
      : 'Startet einen neuen Lauf mit eurem aktuellen Prompt.'
  return (
    <header class="band" data-agent={a.running || a.state !== 'waiting' ? a.state : 'off'} data-robots={robots.value.length > 1 ? 'many' : 'one'}>
      <div class="band-team">
        <h1>
          <InlineEdit value={s.team.name} label="Teamname" maxLength={30} onSave={renameTeam} />
        </h1>
        <p class="agent-state">
          <span class="agent-dot" aria-hidden="true" />
          <span class="agent-label" aria-live="polite">
            {a.running ? `Agent ${AGENT_LABEL[a.state]}` : a.state !== 'waiting' ? 'Agent antwortet im Gespräch' : 'Kein Lauf'}
          </span>
          <Usage />
        </p>
        <ModelPick />
        <TaskPicker />
      </div>

      <div class="band-robots">
        {robots.value.length === 0 ? (
          <p class="robots-empty">Noch kein Roboter. Der Leitstand teilt euch gleich einen oder mehrere zu.</p>
        ) : (
          robots.value.map((r) => <RobotCard key={r.id} r={r} stale={stale} />)
        )}
      </div>

      <div class="band-actions">
        {conn.value !== 'live' && (
          <span class="conn" role="status" data-lost={conn.value === 'reconnecting' || undefined}>
            {conn.value === 'connecting' ? 'Verbinde …' : 'Verbindung unterbrochen, verbinde neu …'}
          </span>
        )}
        <button class="btn btn-ghost-on-band icon-only" onClick={copyTeamLink} title="Team-Link kopieren" aria-label="Team-Link kopieren">
          <LinkIcon />
        </button>
        <button
          class="btn btn-ghost-on-band icon-only"
          title={`Darstellung: ${THEME_LABEL[theme]}`}
          aria-label={`Darstellung: ${THEME_LABEL[theme]}. Klicken zum Wechseln.`}
          onClick={() => updatePrefs({ theme: NEXT_THEME[theme] })}
        >
          <ThemeIcon />
        </button>
        <button
          class="btn btn-on-band"
          onClick={startRun}
          disabled={a.running || !s.agent_available || !task.value || readOnly.value || !deployed.value}
          title={a.running ? 'Ein Lauf läuft schon. Mit Stopp beendet ihr ihn.' : startHint}
        >
          <PlayIcon /> Lauf starten
        </button>
        <button
          class="btn btn-stop"
          onClick={stopAll}
          disabled={busy.value === 'stop'}
          title={a.running ? 'Hält eure Roboter an und beendet den Lauf.' : 'Hält eure Roboter an.'}
        >
          <StopIcon /> Stopp
        </button>
      </div>
    </header>
  )
}
