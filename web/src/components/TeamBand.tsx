import { busy, conn, copyTeamLink, identifyRobot, prefs, renameRobot, renameTeam, robots, session, startRun, stopAll, updatePrefs } from '../state'
import { COLOR_HEX, colorName, skillName } from '../format'
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

export function TeamBand() {
  const s = session.value!
  const theme = prefs.value.theme
  const ThemeIcon = theme === 'light' ? SunIcon : theme === 'dark' ? MoonIcon : AutoIcon
  return (
    <header class="band" data-agent={s.agent} data-robots={robots.value.length > 1 ? 'many' : 'one'}>
      <div class="band-team">
        <h1>
          <InlineEdit value={s.team.name} label="Teamname" maxLength={30} onSave={renameTeam} />
        </h1>
        <p class="agent-state" aria-live="polite">
          <span class="agent-dot" aria-hidden="true" />
          Agent {AGENT_LABEL[s.agent]}
        </p>
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
        <button class="btn btn-on-band" onClick={startRun} disabled={!s.agent_available} title={s.agent_available ? undefined : 'Der Agent ist noch nicht verfügbar.'}>
          <PlayIcon /> Lauf starten
        </button>
        <button class="btn btn-stop" onClick={stopAll} disabled={busy.value === 'stop'}>
          <StopIcon /> Stopp
        </button>
      </div>
    </header>
  )
}
