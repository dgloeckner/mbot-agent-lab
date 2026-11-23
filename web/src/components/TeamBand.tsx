import { agent, busy, chooseModel, conn, copyTeamLink, deployed, identifyRobot, prefs, renameRobot, renameTeam, robots, session, startRun, stopAll, updatePrefs } from '../state'
import { COLOR_HEX, colorName, formatCost, formatNumber, formatPrices, skillName } from '../format'
import type { Robot } from '../types'
import { AutoIcon, LinkIcon, MoonIcon, PlayIcon, StopIcon, SunIcon } from './icons'
import { InlineEdit } from './InlineEdit'
import type { Theme } from '../storage'

const AGENT_LABEL = { waiting: 'wartet', thinking: 'denkt nach', acting: 'handelt' } as const

const NEXT_THEME: Record<Theme, Theme> = { system: 'light', light: 'dark', dark: 'system' }
const THEME_LABEL: Record<Theme, string> = { system: 'wie System', light: 'hell', dark: 'dunkel' }

function RobotCard({ r }: { r: Robot }) {
  const s = r.state
  const status = !r.online ? 'offline' : !r.accepted ? 'falsche Version' : s.tilted ? 'umgekippt' : 'online'
  const near = r.online && s.distance_cm < 15
  const gauge = Math.max(0, Math.min(1, s.distance_cm / 100))
  return (
    <div class="robot" data-status={r.online && r.accepted ? (s.tilted ? 'warn' : 'ok') : 'off'}>
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
        <div>
          <dt>Status</dt>
          <dd><span class="dot" />{status}</dd>
        </div>
        <div class={near ? 'near' : undefined}>
          <dt>Abstand</dt>
          <dd>
            <span class="gauge" style={{ '--v': gauge }} aria-hidden="true" />
            {r.online ? `${s.distance_cm} cm` : '–'}
          </dd>
        </div>
        <div>
          <dt>Boden</dt>
          <dd>
            <span class="swatch" style={{ background: COLOR_HEX[s.color] ?? 'transparent' }} aria-hidden="true" />
            {r.online ? colorName(s.color || 'none') : '–'}
          </dd>
        </div>
        <div>
          <dt>Akku</dt>
          <dd>{r.online ? `${s.battery} %` : '–'}</dd>
        </div>
        <div>
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

export function TeamBand() {
  const s = session.value!
  const theme = prefs.value.theme
  const ThemeIcon = theme === 'light' ? SunIcon : theme === 'dark' ? MoonIcon : AutoIcon
  const a = agent.value
  const startHint = !s.agent_available
    ? 'Es ist kein Sprachmodell eingerichtet.'
    : !deployed.value
      ? 'Deployt zuerst einen Prompt.'
      : 'Startet einen neuen Lauf mit eurem aktuellen Prompt.'
  return (
    <header class="band" data-agent={a.running ? a.state : 'off'} data-robots={robots.value.length > 1 ? 'many' : 'one'}>
      <div class="band-team">
        <h1>
          <InlineEdit value={s.team.name} label="Teamname" maxLength={30} onSave={renameTeam} />
        </h1>
        <p class="agent-state" aria-live="polite">
          <span class="agent-dot" aria-hidden="true" />
          {a.running ? `Agent ${AGENT_LABEL[a.state]}` : 'Kein Lauf'}
          {a.calls > 0 && (
            <span class="run-usage" title={`Modell ${a.model ?? ''}, ${a.calls} Aufrufe`}>
              {formatNumber(a.tokens_in + a.tokens_out)} Token, {formatCost(a.cost_micro_eur)}
            </span>
          )}
        </p>
        <ModelPick />
      </div>

      <div class="band-robots">
        {robots.value.length === 0 ? (
          <p class="robots-empty">Noch kein Roboter. Der Leitstand teilt euch gleich einen oder mehrere zu.</p>
        ) : (
          robots.value.map((r) => <RobotCard key={r.id} r={r} />)
        )}
      </div>

      <div class="band-actions">
        {conn.value !== 'live' && (
          <span class="conn" role="status">
            {conn.value === 'connecting' ? 'Verbinde …' : 'Verbindung unterbrochen, versuche es erneut …'}
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
        {!a.running && (
          <button class="btn btn-on-band" onClick={startRun} disabled={!s.agent_available || !deployed.value} title={startHint}>
            <PlayIcon /> Lauf starten
          </button>
        )}
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
