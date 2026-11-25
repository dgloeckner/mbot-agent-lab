import { useEffect, useRef, useState } from 'preact/hooks'
import { api } from '../api'
import { lazyComponent } from '../lazy'
import { chipsFor, stateLabel } from '../tooldiff'
import type { ToolChange } from '../types'
import { MergeView } from '@codemirror/merge'
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { markdownSupport } from '../editor'
import { changeSize, type Snapshot } from '../storage'
import { formatNumber, relativeTime } from '../format'
import { deployed, draft, history, restore, restoreVersion, unsavedWork, versions, versionText } from '../state'

const ToolDiffView = lazyComponent(() => import('./ToolDiffView'))

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

/** Side-by-side comparison of an older text (left) with a newer one (right, by default the current draft). */
function Diff({ older, label, newer, newerLabel = 'Euer Entwurf jetzt' }: { older: string; label: string; newer?: string; newerLabel?: string }) {
  const host = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const mv = new MergeView({
      parent: host.current!,
      a: { doc: older, extensions: readOnly },
      b: { doc: newer ?? draft.peek(), extensions: readOnly },
      collapseUnchanged: { margin: 3, minSize: 6 },
      highlightChanges: true,
      gutter: true,
    })
    return () => mv.destroy()
  }, [older, newer])
  return (
    <div class="diff">
      <div class="diff-legend" aria-hidden="true">
        <span>{label}</span>
        <span>{newerLabel}</span>
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
  // Comparison of a version: against the working state (0) or another version.
  const [against, setAgainst] = useState(0)
  const [otherText, setOtherText] = useState<string | null>(null)
  const [tools, setTools] = useState<ToolChange[]>([])
  const [item, setItem] = useState('prompt')
  const selVersion = sel?.kind === 'version' ? sel.version : 0

  useEffect(() => {
    setTools([])
    if (!selVersion) return
    let gone = false
    api.compareTools(selVersion, against).then(
      (c) => {
        if (gone) return
        setTools(c.tools)
        setItem((it) => (it === 'prompt' || c.tools.some((t) => t.id === it) ? it : 'prompt'))
      },
      () => !gone && setTools([]),
    )
    return () => void (gone = true)
  }, [selVersion, against, vers.length])

  useEffect(() => {
    if (against === selVersion) setAgainst(0)
  }, [selVersion])

  useEffect(() => {
    setOtherText(null)
    if (selVersion && against) versionText(against).then(setOtherText, () => setError('Diese Version konnte nicht geladen werden.'))
  }, [selVersion, against])

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
        Noch kein Verlauf. Beim Schreiben sichert die Seite automatisch Zwischenstände in diesem Browser, und jede deployte Version erscheint hier.
      </div>
    )
  }

  const label = sel?.kind === 'version' ? `Version ${sel.version}` : sel ? `Stand ${relativeTime(snaps.find((s) => s.id === sel.id)?.at ?? 0)}` : ''
  const versus = selVersion && against ? against : 0
  const same = older !== null && (versus ? older === otherText : older === draft.value)
  // The older version is on the left.
  const [leftText, leftLabel, rightText, rightLabel] =
    versus && versus < selVersion
      ? [otherText, stateLabel(versus), older, label]
      : [older, label, versus ? otherText : undefined, versus ? stateLabel(versus) : undefined]
  const chips = selVersion ? chipsFor(tools) : []
  const showTool = selVersion !== 0 && item !== 'prompt'
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
                <button
                  class="history-item"
                  aria-current={sel?.kind === 'snapshot' && sel.id === s.id}
                  onClick={() => setSel({ kind: 'snapshot', id: s.id })}
                >
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
          <span>
            {versus
              ? `${label} im Vergleich zu Version ${versus}`
              : same
                ? `${label}: Prompt identisch mit eurem Entwurf.`
                : `${label} im Vergleich zu eurem Entwurf`}
          </span>
          {selVersion !== 0 && vers.length > 1 && (
            <label class="history-against">
              <span>Vergleichen mit</span>
              <select value={String(against)} onChange={(ev) => setAgainst(Number(ev.currentTarget.value))}>
                <option value="0">Euer Stand jetzt</option>
                {vers
                  .filter((v) => v.version !== selVersion)
                  .map((v) => (
                    <option key={v.version} value={String(v.version)}>
                      Version {v.version}
                    </option>
                  ))}
              </select>
            </label>
          )}
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
              <strong>{label} wiederherstellen?</strong> Prompt und eigene Werkzeuge werden wie in dieser Version. Dabei geht verloren, was noch nicht
              deployt ist:
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
        {chips.length > 1 && (
          <div class="history-chips" role="tablist" aria-label="Was vergleichen">
            {chips.map((c) => (
              <button
                key={c.key}
                role="tab"
                class="history-chip"
                aria-selected={item === c.key}
                data-change={c.change}
                onClick={() => setItem(c.key)}
              >
                {c.label}
                {c.change && (
                  <span class="tool-change" data-change={c.change}>
                    {c.change}
                  </span>
                )}
              </button>
            ))}
          </div>
        )}
        {error && <p class="error-text">{error}</p>}
        {showTool ? (
          <ToolDiffView version={selVersion} against={against} id={item} />
        ) : (
          leftText !== null &&
          (!versus || rightText !== null) &&
          (same ? (
            <p class="history-same">Der Prompt ist in beiden gleich.</p>
          ) : (
            <Diff older={leftText} label={leftLabel} newer={rightText ?? undefined} newerLabel={rightLabel} />
          ))
        )}
      </div>
    </div>
  )
}
