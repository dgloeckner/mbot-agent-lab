import { useEffect, useRef, useState } from 'preact/hooks'
import { MergeView } from '@codemirror/merge'
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { markdownSupport } from '../editor'
import { changeSize, type Snapshot } from '../storage'
import { formatNumber, relativeTime } from '../format'
import { deployed, draft, history, restore, restoreVersion, unsavedWork, versions, versionText } from '../state'

type Source = { kind: 'version'; version: number } | { kind: 'snapshot'; id: string }

const REASON: Record<Snapshot['reason'], string> = {
  auto: 'automatisch gesichert',
  manual: 'von euch gesichert',
  deploy: 'beim Deployen gesichert',
  restore: 'vor dem Wiederherstellen gesichert',
}

const readOnly = [
  EditorView.editable.of(false),
  EditorState.readOnly.of(true),
  EditorView.lineWrapping,
  markdownSupport(),
  EditorView.theme({
    '&': { color: 'var(--ink)', backgroundColor: 'transparent' },
    '.cm-scroller': { fontFamily: 'var(--font-mono)', lineHeight: '1.6' },
    '.cm-content': { padding: '14px 0' },
    '.cm-line': { padding: '0 14px' },
  }),
]

/** Side-by-side comparison of an older text (left) with the current draft (right). */
function Diff({ older, label }: { older: string; label: string }) {
  const host = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const mv = new MergeView({
      parent: host.current!,
      a: { doc: older, extensions: readOnly },
      b: { doc: draft.peek(), extensions: readOnly },
      collapseUnchanged: { margin: 3, minSize: 6 },
      highlightChanges: true,
      gutter: true,
    })
    return () => mv.destroy()
  }, [older])
  return (
    <div class="diff">
      <div class="diff-legend" aria-hidden="true">
        <span>{label}</span>
        <span>Euer Entwurf jetzt</span>
      </div>
      <div class="diff-body" ref={host} />
    </div>
  )
}

export default function HistoryView({ onRestored }: { onRestored: () => void }) {
  const snaps = history.value
  const vers = versions.value
  const initial: Source | null = vers[0] ? { kind: 'version', version: vers[0].version } : snaps[0] ? { kind: 'snapshot', id: snaps[0].id } : null
  const [sel, setSel] = useState<Source | null>(initial)
  const [older, setOlder] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setError(null)
    if (!sel) return setOlder(null)
    if (sel.kind === 'snapshot') {
      setOlder(snaps.find((s) => s.id === sel.id)?.text ?? null)
      return
    }
    setOlder(null)
    versionText(sel.version).then(setOlder, () => setError('Diese Version konnte nicht geladen werden.'))
  }, [sel?.kind === 'version' ? sel.version : sel?.id])

  if (!vers.length && !snaps.length) {
    return (
      <div class="empty-state">
        Noch kein Verlauf. Beim Schreiben sichert die Seite automatisch Zwischenstände in diesem Browser, und jede deployte Version
        erscheint hier.
      </div>
    )
  }

  const label =
    sel?.kind === 'version' ? `Version ${sel.version}` : sel ? `Stand ${relativeTime(snaps.find((s) => s.id === sel.id)?.at ?? 0)}` : ''
  const same = older !== null && older === draft.value
  const [asking, setAsking] = useState<string[] | null>(null)
  const restoreSelected = async () => {
    if (sel?.kind !== 'version') return
    if (await restoreVersion(sel.version)) {
      setAsking(null)
      onRestored()
    }
  }

  return (
    <div class="history">
      <nav class="history-list" aria-label="Verlauf">
        {vers.length > 0 && <h3>Deployt</h3>}
        <ul>
          {vers.map((v) => (
            <li key={`v${v.version}`}>
              <button
                class="history-item"
                aria-current={sel?.kind === 'version' && sel.version === v.version}
                onClick={() => setSel({ kind: 'version', version: v.version })}
              >
                <strong>Version {v.version}</strong>
                <span>
                  {relativeTime(v.created_at)}
                  {deployed.value?.version === v.version ? ', aktuell' : ''}
                </span>
                {v.tools && v.tools.length > 0 && (
                  <span class="tool-changes">
                    {v.tools.map((c) => (
                      <span key={c.id} class="tool-change" data-change={c.change}>
                        {c.name}: {c.change}
                      </span>
                    ))}
                  </span>
                )}
              </button>
            </li>
          ))}
        </ul>
        {snaps.length > 0 && <h3>In diesem Browser</h3>}
        <ul>
          {snaps.map((s, i) => {
            const prev = snaps[i + 1]
            const delta = prev ? changeSize(prev.text, s.text) : s.text.length
            return (
              <li key={s.id}>
                <button class="history-item" aria-current={sel?.kind === 'snapshot' && sel.id === s.id} onClick={() => setSel({ kind: 'snapshot', id: s.id })}>
                  <strong>{relativeTime(s.at)}</strong>
                  <span>
                    {REASON[s.reason]}
                    {s.version ? `, Version ${s.version}` : ''}, {formatNumber(delta)} Zeichen geändert
                  </span>
                </button>
              </li>
            )
          })}
        </ul>
      </nav>

      <div class="history-detail">
        <div class="history-toolbar">
          <span>{same ? `${label} ist identisch mit eurem Entwurf.` : `${label} im Vergleich zu eurem Entwurf`}</span>
          {sel?.kind === 'version' ? (
            <button
              class="btn btn-small"
              disabled={older === null}
              title="Prompt und eigene Werkzeuge werden wie in dieser Version."
              onClick={() => {
                const lost = unsavedWork()
                if (lost.length) setAsking(lost)
                else void restoreSelected()
              }}
            >
              Diese Version wiederherstellen
            </button>
          ) : (
            <button
              class="btn btn-small"
              disabled={older === null || same}
              onClick={() => {
                restore(older ?? '', label)
                onRestored()
              }}
            >
              {label} wiederherstellen
            </button>
          )}
        </div>
        {asking && (
          <div class="banner restore-ask" role="alertdialog" aria-label="Wiederherstellen bestätigen">
            <div>
              <strong>{label} wiederherstellen?</strong> Prompt und eigene Werkzeuge werden wie in dieser Version. Dabei geht verloren, was
              noch nicht deployt ist:
              <ul>
                {asking.map((a) => (
                  <li key={a}>{a}</li>
                ))}
              </ul>
              Im Verlauf bleibt jede deployte Version erhalten.
            </div>
            <div class="restore-ask-actions">
              <button class="btn btn-small btn-accent" onClick={() => void restoreSelected()}>
                Trotzdem wiederherstellen
              </button>
              <button class="btn btn-small btn-quiet" onClick={() => setAsking(null)}>
                Abbrechen
              </button>
            </div>
          </div>
        )}
        {error && <p class="error-text">{error}</p>}
        {older !== null && !same && <Diff older={older} label={label} />}
      </div>
    </div>
  )
}
