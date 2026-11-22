import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks'
import { useSignal } from '@preact/signals'
import { clock, describe, logGroup, relativeTime } from '../format'
import { addNote, deleteNote, editorBridge, log, notes, prefs, robots, saveNote, sendMessage, session, updatePrefs } from '../state'
import type { SideTab } from '../storage'
import type { LogEntry, Note } from '../types'
import { SendIcon } from './icons'

type Filter = 'all' | 'robot' | 'team' | 'agent' | 'system'
const FILTERS: [Filter, string][] = [
  ['all', 'Alles'],
  ['robot', 'Roboter'],
  ['agent', 'Agenten'],
  ['team', 'Ihr'],
  ['system', 'System'],
]
const SOURCE_LABEL = { robot: 'Roboter', team: 'Ihr', agent: 'Agent', system: 'System' } as const

function LogLine({ e }: { e: LogEntry }) {
  const g = logGroup(e)
  const who = g === 'robot' && e.robot ? e.robot : g === 'agent' ? e.source.slice(6) || 'Agent' : SOURCE_LABEL[g]
  return (
    <li class="log-line" data-group={g} data-kind={e.kind}>
      <time dateTime={e.at}>{clock(e.at)}</time>
      <span class="log-who">{who}</span>
      <span class="log-text">{describe(e)}</span>
    </li>
  )
}

function LogPanel() {
  const filter = useSignal<Filter>('all')
  const list = useRef<HTMLOListElement>(null)
  const stick = useRef(true)
  const entries = filter.value === 'all' ? log.value : log.value.filter((e) => logGroup(e) === filter.value)

  useLayoutEffect(() => {
    const el = list.current
    if (el && stick.current) el.scrollTop = el.scrollHeight
  }, [entries.length, filter.value])

  return (
    <div class="log-panel">
      <div class="chips" role="group" aria-label="Protokoll filtern">
        {FILTERS.map(([id, label]) => (
          <button key={id} class="chip" aria-pressed={filter.value === id} onClick={() => (filter.value = id)}>
            {label}
          </button>
        ))}
      </div>
      <ol
        class="log"
        ref={list}
        aria-live="polite"
        onScroll={(ev) => {
          const el = ev.currentTarget
          stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40
        }}
      >
        {entries.length === 0 ? (
          <li class="empty-state">
            {filter.value === 'all'
              ? 'Noch ist nichts passiert. Deployt euren Prompt oder schreibt eurem Agenten eine Nachricht.'
              : 'Dazu gibt es noch keine Einträge.'}
          </li>
        ) : (
          entries.map((e) => <LogLine key={e.id} e={e} />)
        )}
      </ol>
      <Composer />
    </div>
  )
}

function Composer() {
  const [text, setText] = useState('')
  const [sending, setSending] = useState(false)
  const limit = session.value!.limits.message_chars
  const submit = async () => {
    const t = text.trim()
    if (!t || sending || t.length > limit) return
    setSending(true)
    if (await sendMessage(t)) setText('')
    setSending(false)
  }
  return (
    <form
      class="composer"
      onSubmit={(ev) => {
        ev.preventDefault()
        void submit()
      }}
    >
      <textarea
        rows={2}
        value={text}
        placeholder="Nachricht an euren Agenten"
        aria-label="Nachricht an euren Agenten"
        onInput={(ev) => setText(ev.currentTarget.value)}
        onKeyDown={(ev) => {
          if (ev.key === 'Enter' && !ev.shiftKey && !ev.isComposing) {
            ev.preventDefault()
            void submit()
          }
        }}
      />
      {text.length > limit * 0.8 && (
        <span class={text.length > limit ? 'counter over' : 'counter'}>
          {text.length} / {limit}
        </span>
      )}
      <button class="btn btn-accent icon-only" type="submit" disabled={!text.trim() || sending || text.length > limit} aria-label="Senden">
        <SendIcon />
      </button>
    </form>
  )
}

function NoteItem({ n }: { n: Note }) {
  const [editing, setEditing] = useState(false)
  const [text, setText] = useState(n.text)
  useEffect(() => setText(n.text), [n.text])
  return (
    <li class="note" data-author={n.author}>
      <div class="note-meta">
        <span>{n.author === 'agent' ? 'Vom Agenten' : 'Von euch'}</span>
        <span class="faint">{relativeTime(n.created_at)}</span>
      </div>
      {editing ? (
        <form
          onSubmit={async (ev) => {
            ev.preventDefault()
            if (await saveNote(n.id, text.trim())) setEditing(false)
          }}
        >
          <textarea rows={3} value={text} onInput={(ev) => setText(ev.currentTarget.value)} aria-label="Notiz bearbeiten" />
          <div class="note-actions">
            <button class="btn btn-small" type="submit" disabled={!text.trim()}>
              Speichern
            </button>
            <button class="btn btn-small btn-quiet" type="button" onClick={() => (setText(n.text), setEditing(false))}>
              Abbrechen
            </button>
          </div>
        </form>
      ) : (
        <>
          <p>{n.text}</p>
          <div class="note-actions">
            <button class="btn-link" onClick={() => setEditing(true)}>
              Bearbeiten
            </button>
            <button class="btn-link danger" onClick={() => deleteNote(n.id)}>
              Löschen
            </button>
          </div>
        </>
      )}
    </li>
  )
}

function NotesPanel() {
  const [text, setText] = useState('')
  return (
    <div class="notes-panel scroll">
      <p class="panel-intro">Notizen sieht euer Agent in jeder Runde, auch nach einem Neustart. Er kann selbst welche anlegen.</p>
      {notes.value.length === 0 ? (
        <p class="empty-state">Noch keine Notizen.</p>
      ) : (
        <ul class="notes">
          {notes.value.map((n) => (
            <NoteItem key={n.id} n={n} />
          ))}
        </ul>
      )}
      <form
        class="note-new"
        onSubmit={async (ev) => {
          ev.preventDefault()
          if (text.trim() && (await addNote(text.trim()))) setText('')
        }}
      >
        <textarea rows={2} value={text} placeholder="Neue Notiz, zum Beispiel: Die Wand links ist näher, als sie aussieht." onInput={(ev) => setText(ev.currentTarget.value)} aria-label="Neue Notiz" />
        <button class="btn btn-small" type="submit" disabled={!text.trim()}>
          Notiz anlegen
        </button>
      </form>
    </div>
  )
}

function ToolsPanel() {
  const tools = session.value!.tools
  const groups = [...new Set(tools.map((t) => t.group))]
  return (
    <div class="tools-panel scroll">
      <p class="panel-intro">Diese Werkzeuge kann euer Agent benutzen. Klickt eins an, um seinen Namen in den Prompt einzufügen.</p>
      {robots.value.length > 1 && (
        <p class="panel-intro multi-robot">
          Euer Agent steuert {robots.value.length} Roboter: {robots.value.map((r, i) => (
            <span key={r.id}>
              {i > 0 && (i === robots.value.length - 1 ? ' und ' : ', ')}
              <button class="tool-inline" onClick={() => editorBridge.insert?.(r.alias)}>{r.alias}</button>
            </span>
          ))}
          . Bewegungs-Werkzeuge bekommen dann den Parameter <code>robot</code> mit einem dieser Namen, <code>stop</code> geht auch mit <code>alle</code>.
        </p>
      )}
      {groups.map((g) => (
        <section key={g}>
          <h3>{g}</h3>
          <ul class="tools">
            {tools
              .filter((t) => t.group === g)
              .map((t) => (
                <li key={t.name}>
                  <button class="tool" onClick={() => editorBridge.insert?.('`' + t.name + '`')} title="In den Prompt einfügen">
                    <code>{t.name}</code>
                    {t.params && <span class="tool-params">{t.params}</span>}
                    <span class="tool-summary">{t.summary}</span>
                  </button>
                </li>
              ))}
          </ul>
        </section>
      ))}
    </div>
  )
}

export function SidePane() {
  const tab = prefs.value.sideTab
  const tabs: [SideTab, string][] = [
    ['log', 'Protokoll'],
    ['notes', `Notizen${notes.value.length ? ` (${notes.value.length})` : ''}`],
    ['tools', 'Werkzeuge'],
  ]
  return (
    <section class="pane side-pane" aria-label="Protokoll, Notizen und Werkzeuge">
      <div class="pane-head">
        <div class="tabs" role="tablist">
          {tabs.map(([id, label]) => (
            <button key={id} role="tab" class="tab" aria-selected={tab === id} onClick={() => updatePrefs({ sideTab: id })}>
              {label}
            </button>
          ))}
        </div>
      </div>
      <div class="pane-body">
        {tab === 'log' && <LogPanel />}
        {tab === 'notes' && <NotesPanel />}
        {tab === 'tools' && <ToolsPanel />}
      </div>
    </section>
  )
}
