import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks'
import { useSignal } from '@preact/signals'
import { clock, describe, describeTool, logGroup, relativeTime } from '../format'
import {
  addNote,
  agent,
  conversation,
  conversationUnread,
  deleteNote,
  editorBridge,
  log,
  newConversation,
  notes,
  prefs,
  robots,
  saveNote,
  sendMessage,
  session,
  updatePrefs,
} from '../state'
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

function ToolText({ d }: { d: Record<string, any> }) {
  const t = describeTool(d)
  return (
    <>
      <code class="tool-call">{t.call}</code> <span class={t.failed ? 'tool-failed' : 'tool-ok'}>{t.outcome}</span>
    </>
  )
}

function LogLine({ e }: { e: LogEntry }) {
  const g = logGroup(e)
  const who = g === 'robot' && e.robot ? e.robot : g === 'agent' ? e.source.slice(6) || 'Agent' : SOURCE_LABEL[g]
  return (
    <li class="log-line" data-group={g} data-kind={e.kind}>
      <time dateTime={e.at}>{clock(e.at)}</time>
      <span class="log-who">{who}</span>
      <span class="log-text">
        {e.kind === 'tool' ? <ToolText d={e.data ?? {}} /> : describe(e)}
        {e.kind === 'run_end' && typeof e.data?.report === 'string' && e.data.report && <q class="run-report">{e.data.report}</q>}
      </span>
    </li>
  )
}

/** Keeps a list scrolled to the bottom while the reader is there. */
function useStickToBottom(...deps: unknown[]) {
  const list = useRef<HTMLOListElement>(null)
  const stick = useRef(true)
  useLayoutEffect(() => {
    const el = list.current
    if (el && stick.current) el.scrollTop = el.scrollHeight
  }, deps)
  const onScroll = (ev: Event) => {
    const el = ev.currentTarget as HTMLElement
    stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40
  }
  return { list, onScroll }
}

function LogPanel() {
  const filter = useSignal<Filter>('all')
  const entries = filter.value === 'all' ? log.value : log.value.filter((e) => logGroup(e) === filter.value)
  const { list, onScroll } = useStickToBottom(entries.length, filter.value)

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
        onScroll={onScroll}
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
      {agent.value.running ? <Composer placeholder="Nachricht an euren Agenten" /> : <NoRunHint />}
    </div>
  )
}

/** Shown in the protocol instead of the composer while no run is active. */
function NoRunHint() {
  const talk = agent.value.conversation
  return (
    <div class="composer-hint">
      <p>
        Gerade läuft kein Lauf.{' '}
        {talk ? 'Fragen an euren Agenten stellt ihr im Gespräch.' : 'Startet einen Lauf, dann liest euer Agent eure Nachrichten.'}
      </p>
      {talk && (
        <button class="btn btn-small" onClick={() => updatePrefs({ sideTab: 'conversation' })}>
          Zum Gespräch
        </button>
      )}
    </div>
  )
}

function TalkLine({ e }: { e: LogEntry }) {
  if (e.kind === 'tool') {
    return (
      <li class="talk-note">
        <ToolText d={e.data ?? {}} />
      </li>
    )
  }
  if (e.source !== 'team' && e.source !== 'agent') {
    return <li class="talk-note">{e.text}</li>
  }
  const team = e.source === 'team'
  return (
    <li class="talk-msg" data-from={team ? 'team' : 'agent'}>
      <span class="talk-who">
        {team ? 'Ihr' : 'Agent'} <time dateTime={e.at}>{clock(e.at)}</time>
      </span>
      <p>{e.text}</p>
    </li>
  )
}

function ConversationPanel() {
  const a = agent.value
  const entries = conversation.value
  const thinking = !a.running && a.state !== 'waiting'
  const { list, onScroll } = useStickToBottom(entries.length, thinking)
  return (
    <div class="log-panel conversation-panel">
      <div class="conversation-head">
        <p class="panel-intro">
          Ohne Lauf redet ihr hier mit eurem Agenten: über seine Werkzeuge, euren Plan oder den letzten Lauf. Er bewegt dabei keinen Roboter. Im
          nächsten Lauf kennt er von diesem Gespräch nur die Notizen, die er sich macht.
        </p>
        {a.conversation_id && !a.running && (
          <button class="btn btn-small btn-quiet" onClick={() => void newConversation()} title="Der Agent vergisst dieses Gespräch, Notizen bleiben.">
            Neues Gespräch
          </button>
        )}
      </div>
      <ol class="talk" ref={list} aria-live="polite" onScroll={onScroll}>
        {entries.length === 0 && !thinking ? (
          <li class="empty-state">Noch kein Gespräch. Fragt euren Agenten zum Beispiel: „Welche Werkzeuge hast du zum Fahren?“</li>
        ) : (
          entries.map((e) => <TalkLine key={e.id} e={e} />)
        )}
        {thinking && <li class="talk-note talk-thinking">Euer Agent denkt nach …</li>}
      </ol>
      {a.running ? (
        <p class="composer-hint">Gerade läuft ein Lauf. Schreibt eurem Agenten im Protokoll.</p>
      ) : !a.conversation ? (
        <p class="composer-hint">
          {session.value!.agent_available ? 'Der Leitstand hat das Gespräch gerade abgeschaltet.' : 'Es ist kein Sprachmodell eingerichtet.'}
        </p>
      ) : (
        <Composer placeholder="Frage an euren Agenten" />
      )}
    </div>
  )
}

function Composer({ placeholder }: { placeholder: string }) {
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
        placeholder={placeholder}
        aria-label={placeholder}
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
      <p class="panel-intro">Diese Werkzeuge kann euer Agent benutzen. Klickt eins an, um seinen Namen in den Prompt einzufügen. Beim Schreiben schlägt der Editor ab drei Buchstaben passende Werkzeuge vor; <kbd>Tab</kbd> übernimmt den Vorschlag.</p>
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
                  <button class="tool" data-ready={t.ready} onClick={() => editorBridge.insert?.('`' + t.name + '`')} title="In den Prompt einfügen">
                    <code>{t.name}</code>
                    {t.params && <span class="tool-params">{t.params}</span>}
                    <span class="tool-summary">
                      {t.summary}
                      {!t.ready && ' Kommt bald, euer Agent hat es noch nicht.'}
                    </span>
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
  const unread = conversationUnread.value
  const tabs: [SideTab, string][] = [
    ['log', 'Protokoll'],
    ['conversation', 'Gespräch'],
    ['notes', `Notizen${notes.value.length ? ` (${notes.value.length})` : ''}`],
    ['tools', 'Werkzeuge'],
  ]
  return (
    <section class="pane side-pane" aria-label="Protokoll, Gespräch, Notizen und Werkzeuge">
      <div class="pane-head">
        <div class="tabs" role="tablist">
          {tabs.map(([id, label]) => (
            <button key={id} role="tab" class="tab" aria-selected={tab === id} onClick={() => updatePrefs({ sideTab: id })}>
              {label}
              {id === 'conversation' && unread > 0 && tab !== id && (
                <span class="tab-badge" aria-label={`${unread} neue Antworten`}>
                  {unread}
                </span>
              )}
            </button>
          ))}
        </div>
      </div>
      <div class="pane-body">
        {tab === 'log' && <LogPanel />}
        {tab === 'conversation' && <ConversationPanel />}
        {tab === 'notes' && <NotesPanel />}
        {tab === 'tools' && <ToolsPanel />}
      </div>
    </section>
  )
}
