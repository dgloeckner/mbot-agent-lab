import { useEffect, useRef, useState } from 'preact/hooks'
import { useSignal } from '@preact/signals'
import { createEditor } from '../editor'
import { estimateTokens, formatNumber, relativeTime } from '../format'
import {
  acceptRemote,
  busy,
  deploy,
  deployed,
  dirty,
  draft,
  draftSavedAt,
  editorBridge,
  keepDraft,
  prefs,
  remoteVersion,
  session,
  setDraft,
  snapshotNow,
  updatePrefs,
} from '../state'
import { lazyComponent } from '../lazy'

const Preview = lazyComponent(() => import('./Preview'))
const HistoryView = lazyComponent(() => import('./HistoryView'))

type Tab = 'write' | 'preview' | 'history'

const isMac = /Mac|iPhone|iPad/.test(navigator.userAgent)
const MOD = isMac ? '⌘' : 'Strg'

function EditorHost() {
  const host = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const ed = createEditor({
      parent: host.current!,
      doc: draft.peek(),
      tools: session.peek()!.tools,
      onChange: setDraft,
      onSave: snapshotNow,
      onDeploy: () => void deploy(false),
    })
    editorBridge.replace = ed.replace
    editorBridge.insert = ed.insert
    editorBridge.focus = ed.focus
    ed.focus()
    return () => {
      editorBridge.replace = editorBridge.insert = editorBridge.focus = undefined
      ed.destroy()
    }
  }, [])
  return <div class="editor" ref={host} style={{ '--editor-size': `${prefs.value.fontSize}px` }} />
}

function StatusLine() {
  const limit = session.value!.limits.prompt_chars
  const chars = draft.value.length
  const d = deployed.value
  const [, tick] = useState(0)
  useEffect(() => {
    const t = window.setInterval(() => tick((n) => n + 1), 10_000)
    return () => window.clearInterval(t)
  }, [])
  let state: string
  if (!d) state = draft.value.trim() ? 'Noch nicht deployt' : 'Noch kein Prompt'
  else if (!dirty.value) state = `Entspricht Version ${d.version}`
  else state = `Geändert seit Version ${d.version}`
  return (
    <footer class="statusline">
      <span class={`deploy-state${dirty.value ? ' is-dirty' : ''}`}>{state}</span>
      {draftSavedAt.value && dirty.value && <span class="faint">Entwurf gesichert {relativeTime(draftSavedAt.value)}</span>}
      <span class="spacer" />
      <span class={chars > limit ? 'over' : 'faint'} title="Zeichen im Prompt">
        {formatNumber(chars)} / {formatNumber(limit)} Zeichen
      </span>
      <span title="Geschätzt. Die genaue Zahl hängt vom Sprachmodell ab.">≈ {formatNumber(estimateTokens(draft.value))} Token</span>
      <span class="font-size" role="group" aria-label="Schriftgröße">
        <button class="btn-mini" onClick={() => updatePrefs({ fontSize: prefs.value.fontSize - 1 })} aria-label="Schrift kleiner">A−</button>
        <button class="btn-mini" onClick={() => updatePrefs({ fontSize: prefs.value.fontSize + 1 })} aria-label="Schrift größer">A+</button>
      </span>
    </footer>
  )
}

export function PromptPane() {
  const tab = useSignal<Tab>('write')
  const remote = remoteVersion.value
  const tooLong = draft.value.length > session.value!.limits.prompt_chars
  const canDeploy = !busy.value && draft.value.trim() !== '' && !tooLong
  const tabs: [Tab, string][] = [
    ['write', 'Schreiben'],
    ['preview', 'Vorschau'],
    ['history', 'Verlauf'],
  ]
  return (
    <section class="pane prompt-pane" aria-label="Prompt">
      <div class="pane-head">
        <div class="tabs" role="tablist">
          {tabs.map(([id, label]) => (
            <button
              key={id}
              role="tab"
              aria-selected={tab.value === id}
              class="tab"
              onClick={() => {
                tab.value = id
                if (id === 'write') requestAnimationFrame(() => editorBridge.focus?.())
              }}
            >
              {label}
            </button>
          ))}
        </div>
        <div class="deploy-actions">
          <button
            class="btn btn-quiet"
            disabled={!canDeploy}
            onClick={() => deploy(true)}
            title="Speichert eine neue Version und beginnt einen neuen Lauf ohne den bisherigen Verlauf."
          >
            Deployen und neu starten
          </button>
          <button class="btn btn-accent" disabled={!canDeploy} onClick={() => deploy(false)} title={`Speichert eine neue Version (${MOD} + Enter)`}>
            {busy.value === 'deploy' ? 'Deploye …' : 'Deployen'}
            <kbd>{MOD} ↵</kbd>
          </button>
        </div>
      </div>

      {remote && (
        <div class="banner" role="status">
          <span>
            Version {remote.version} wurde an einem anderen Rechner deployt. Euer Entwurf hier ist anders.
          </span>
          <button class="btn btn-small" onClick={acceptRemote}>
            Version {remote.version} übernehmen
          </button>
          <button class="btn btn-small btn-quiet" onClick={keepDraft}>
            Entwurf behalten
          </button>
        </div>
      )}

      <div class="pane-body">
        <div hidden={tab.value !== 'write'} class="tab-panel">
          <EditorHost />
        </div>
        {tab.value === 'preview' && (
          <div class="tab-panel scroll">
            <Preview />
          </div>
        )}
        {tab.value === 'history' && (
          <div class="tab-panel">
            <HistoryView onRestored={() => (tab.value = 'write')} />
          </div>
        )}
      </div>
      <StatusLine />
    </section>
  )
}
