import { useEffect, useRef } from 'preact/hooks'
import { COLOR_HEX, onColor } from '../format'
import { loadError, prefs, session, toast, updatePrefs } from '../state'
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
      <main class="main" style={{ '--split': prefs.value.split }}>
        <PromptPane />
        <Splitter />
        <SidePane />
      </main>
      {toast.value && (
        <div class={`toast toast-${toast.value.kind}`} role={toast.value.kind === 'error' ? 'alert' : 'status'}>
          {toast.value.text}
        </div>
      )}
    </div>
  )
}
