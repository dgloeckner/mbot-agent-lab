import { useEffect, useRef } from 'preact/hooks'
import { COLOR_HEX, onColor } from '../format'
import { lazyComponent } from '../lazy'
import { pickerLists } from '../taskview'
import { accepting, deployWarning, leftView, loadError, robots, unknownTeam, prefs, readOnly, session, task, tasks, toast, updatePrefs, wideTool } from '../state'
import { browserKV, defaultPrefs, loadJoined } from '../storage'
import { PromptPane } from './PromptPane'
import { SidePane } from './SidePane'
import { TeamBand } from './TeamBand'

/** Minimum widths in px of the prompt and the side pane (styles.css, .main). */
const PANE_MIN = { left: 420, right: 400 }

/** The split clamped so both panes keep their minimum width in a main area this wide. */
function clampSplit(split: number, width: number): number {
  const lo = Math.max(0.3, PANE_MIN.left / width)
  const hi = Math.min(0.8, 1 - (PANE_MIN.right + 8) / width)
  return Math.min(Math.max(split, lo), Math.max(lo, hi))
}

/** Draggable divider between the prompt and the side pane; a double click resets it. */
function Splitter() {
  const ref = useRef<HTMLDivElement>(null)
  const width = () => ref.current!.parentElement!.getBoundingClientRect()
  const move = (clientX: number) => {
    const main = width()
    updatePrefs({ split: clampSplit((clientX - main.left) / main.width, main.width) })
  }
  return (
    <div
      ref={ref}
      class="splitter"
      role="separator"
      aria-orientation="vertical"
      aria-label="Breite der Bereiche ändern"
      aria-valuenow={Math.round(prefs.value.split * 100)}
      aria-valuemin={30}
      aria-valuemax={80}
      tabIndex={0}
      onPointerDown={(ev) => {
        ev.currentTarget.setPointerCapture(ev.pointerId)
        document.body.classList.add('resizing')
      }}
      onPointerMove={(ev) => ev.currentTarget.hasPointerCapture(ev.pointerId) && move(ev.clientX)}
      onPointerUp={(ev) => {
        ev.currentTarget.releasePointerCapture(ev.pointerId)
        document.body.classList.remove('resizing')
      }}
      onDblClick={() => updatePrefs({ split: defaultPrefs.split })}
      title="Ziehen ändert die Breite, Doppelklick stellt sie zurück."
      onKeyDown={(ev) => {
        const step = ev.key === 'ArrowLeft' ? -0.02 : ev.key === 'ArrowRight' ? 0.02 : 0
        if (step) updatePrefs({ split: clampSplit(prefs.value.split + step, width().width) })
        if (ev.key === 'Home') updatePrefs({ split: defaultPrefs.split })
      }}
    />
  )
}

const AcceptDialog = lazyComponent(() => import('./AcceptDialog'))

/** Without an active task: the team waits, or accepts its first task. */
function Waiting() {
  const { fresh } = pickerLists(tasks.value)
  return (
    <main class="waiting">
      {fresh.length === 0 ? (
        <div class="waiting-card">
          <h2>Wartet auf eine Aufgabe von der Lab-Leitung</h2>
          <p>
            Sobald ihr eine bekommt, erscheint sie hier.{' '}
            {robots.value.length === 0
              ? 'Roboter teilt euch der Leitstand gleich zu; ihr seht sie dann oben.'
              : robots.value.length === 1
                ? 'Euren Roboter seht ihr oben schon.'
                : 'Eure Roboter seht ihr oben schon.'}
          </p>
        </div>
      ) : (
        <div class="waiting-card">
          <h2>{fresh.length === 1 ? 'Ihr habt eine Aufgabe bekommen' : `Ihr habt ${fresh.length} Aufgaben bekommen`}</h2>
          <ul class="waiting-tasks">
            {fresh.map((t) => (
              <li key={t.id}>
                <strong>{t.title}</strong>
                <button class="btn btn-accent" onClick={() => (accepting.value = t)}>
                  Ansehen und annehmen
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </main>
  )
}

export function App() {
  const s = session.value
  const accent = s ? (COLOR_HEX[s.team.color] ?? COLOR_HEX.blue) : COLOR_HEX.blue

  useEffect(() => {
    const root = document.documentElement.style
    root.setProperty('--accent', accent)
    root.setProperty('--on-accent', onColor(accent))
    if (s) document.title = `${s.team.name}: mBot2 Agent Lab`
  }, [accent, s?.team.name])

  if (unknownTeam.value) {
    const earlier = loadJoined(browserKV()).filter((j) => !location.pathname.endsWith(j.token))
    return (
      <main class="fullpage-message unknown-team">
        <h1>Diesen Team-Link gibt es nicht (mehr)</h1>
        <p>Vielleicht ist er vertippt, oder die Lab-Leitung hat das Team gelöscht.</p>
        <p>
          <a class="btn btn-accent" href="/join">
            Zur Anmeldung
          </a>{' '}
          Dort gründet ihr ein Team oder kommt mit dem Code eines Team-Rechners dazu.
        </p>
        {earlier.length > 0 && (
          <>
            <h2>Auf diesem Rechner schon dabei</h2>
            <ul>
              {earlier.map((j) => (
                <li key={j.token}>
                  <a href={`/t/${j.token}`}>{j.name}</a>
                </li>
              ))}
            </ul>
          </>
        )}
      </main>
    )
  }
  if (loadError.value) {
    return (
      <main class="fullpage-message">
        <h1>Die Teamseite konnte nicht geladen werden</h1>
        <p>{loadError.value}</p>
      </main>
    )
  }
  if (!s) {
    return <main class="fullpage-message" aria-busy="true" />
  }
  return (
    <div class="app">
      <TeamBand />
      {task.value ? (
        <>
          {readOnly.value && (
            <div class="banner banner-closed" role="status">
              Die Lab-Leitung hat die Aufgabe „{task.value.title}“ geschlossen. Ihr könnt alles ansehen, aber nichts mehr deployen, starten oder ändern.
            </div>
          )}
          <main class="main" data-wide={(leftView.value === 'tool' && wideTool.value) || undefined} style={{ '--split': prefs.value.split }}>
            <PromptPane key={task.value.id} />
            <Splitter />
            <SidePane />
          </main>
        </>
      ) : (
        <Waiting />
      )}
      {accepting.value && <AcceptDialog key={accepting.value.id} task={accepting.value} />}
      {deployWarning.value && (
        <div class="deploy-warning" role="status">
          <span>
            <strong>Achtung:</strong> {deployWarning.value}
          </span>
          <button class="btn-link" onClick={() => (deployWarning.value = null)} aria-label="Hinweis schließen" title="Hinweis schließen">
            ✕
          </button>
        </div>
      )}
      {toast.value && (
        <div class={`toast toast-${toast.value.kind}`} role={toast.value.kind === 'error' ? 'alert' : 'status'}>
          {toast.value.text}
        </div>
      )}
    </div>
  )
}
