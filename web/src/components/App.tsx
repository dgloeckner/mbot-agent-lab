import { useEffect, useRef } from 'preact/hooks'
import { COLOR_HEX, onColor } from '../format'
import { lazyComponent } from '../lazy'
import { pickerLists } from '../taskview'
import { accepting, loadError, prefs, readOnly, session, task, tasks, toast, updatePrefs } from '../state'
import { PromptPane } from './PromptPane'
import { SidePane } from './SidePane'
import { TeamBand } from './TeamBand'

/** Draggable divider between the prompt and the side pane. */
function Splitter() {
  const ref = useRef<HTMLDivElement>(null)
  const move = (clientX: number) => {
    const main = ref.current!.parentElement!.getBoundingClientRect()
    updatePrefs({ split: Math.min(0.8, Math.max(0.3, (clientX - main.left) / main.width)) })
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
      onKeyDown={(ev) => {
        const step = ev.key === 'ArrowLeft' ? -0.02 : ev.key === 'ArrowRight' ? 0.02 : 0
        if (step) updatePrefs({ split: Math.min(0.8, Math.max(0.3, prefs.value.split + step)) })
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
          <p>Sobald ihr eine bekommt, erscheint sie hier. Eure Roboter seht ihr oben schon.</p>
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
          <main class="main" style={{ '--split': prefs.value.split }}>
            <PromptPane key={task.value.id} />
            <Splitter />
            <SidePane />
          </main>
        </>
      ) : (
        <Waiting />
      )}
      {accepting.value && <AcceptDialog key={accepting.value.id} task={accepting.value} />}
      {toast.value && (
        <div class={`toast toast-${toast.value.kind}`} role={toast.value.kind === 'error' ? 'alert' : 'status'}>
          {toast.value.text}
        </div>
      )}
    </div>
  )
}
