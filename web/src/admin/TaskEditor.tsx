// Dialog to write a task (draft) or read it (published). Loaded on demand:
// it pulls in CodeMirror and the Markdown renderer.
import DOMPurify from 'dompurify'
import { marked } from 'marked'
import { useEffect, useRef, useState } from 'preact/hooks'
import { createMarkdownField } from '../editor'
import { STATE_LABEL, type TaskDetail } from './tasks'

export interface TaskEditorProps {
  task: TaskDetail
  onSave: (t: { title: string; body: string; starter: string }) => Promise<boolean>
  onPublish: (t: { title: string; body: string; starter: string }) => Promise<boolean>
  onDuplicate: () => void
  onClose: () => void // close the dialog
  onCloseTask: () => void // close the task for every team
  onDelete: () => void
}

function MarkdownField(props: { value: string; label: string; placeholder: string; readOnly: boolean; onChange: (v: string) => void }) {
  const host = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const f = createMarkdownField({ parent: host.current!, doc: props.value, label: props.label, placeholder: props.placeholder, readOnly: props.readOnly, onChange: props.onChange })
    return () => f.destroy()
  }, [props.readOnly])
  return <div class="task-field" ref={host} />
}

function Rendered({ text }: { text: string }) {
  if (!text.trim()) return <p class="pool-sub">Noch kein Text.</p>
  const html = DOMPurify.sanitize(marked.parse(text, { async: false, gfm: true, breaks: false }) as string)
  return <article class="prose" dangerouslySetInnerHTML={{ __html: html }} />
}

export default function TaskEditor(p: TaskEditorProps) {
  const dlg = useRef<HTMLDialogElement>(null)
  const draft = p.task.state === 'draft'
  const [title, setTitle] = useState(p.task.title)
  const [body, setBody] = useState(p.task.body)
  const [starter, setStarter] = useState(p.task.starter)
  const [tab, setTab] = useState<'write' | 'preview'>(draft ? 'write' : 'preview')
  const [busy, setBusy] = useState(false)
  const dirty = title !== p.task.title || body !== p.task.body || starter !== p.task.starter

  useEffect(() => {
    dlg.current?.showModal()
  }, [])

  const close = () => {
    if (dirty && !confirm('Änderungen verwerfen?')) return
    p.onClose()
  }
  const run = async (f: () => Promise<boolean>) => {
    setBusy(true)
    const ok = await f()
    setBusy(false)
    if (ok) p.onClose()
  }

  return (
    <dialog
      ref={dlg}
      class="task-dialog"
      aria-labelledby="task-dialog-title"
      onCancel={(ev) => {
        ev.preventDefault()
        close()
      }}
    >
      <header class="task-dialog-head">
        <h2 id="task-dialog-title">{draft ? (p.task.title ? 'Aufgabe bearbeiten' : 'Neue Aufgabe') : p.task.title}</h2>
        <span class={`task-state task-state-${p.task.state}`}>{STATE_LABEL[p.task.state]}</span>
        <span class="spacer" />
        <button class="btn btn-quiet btn-small" onClick={close}>
          Schließen
        </button>
      </header>

      {draft && (
        <label class="task-title">
          <span>Titel</span>
          <input value={title} maxLength={80} onInput={(e) => setTitle(e.currentTarget.value)} placeholder="z. B. Durchs Labyrinth" autoFocus />
        </label>
      )}

      <div class="tabs" role="tablist" aria-label="Ansicht">
        {draft && (
          <button class="tab" role="tab" aria-selected={tab === 'write'} onClick={() => setTab('write')}>
            Schreiben
          </button>
        )}
        <button class="tab" role="tab" aria-selected={tab === 'preview'} onClick={() => setTab('preview')}>
          {draft ? 'Vorschau' : 'Text'}
        </button>
      </div>

      {tab === 'write' ? (
        <div class="task-fields">
          <section>
            <h3>Aufgabentext für das Team</h3>
            <p class="pool-sub">Markdown. Nur das Team sieht ihn, der Agent nie: Daraus einen Prompt zu machen, ist die Aufgabe des Teams.</p>
            <MarkdownField value={body} label="Aufgabentext" placeholder="Was soll das Team erreichen? Woran merkt es, dass es geschafft ist?" readOnly={false} onChange={setBody} />
          </section>
          <section>
            <h3>Startprompt (freiwillig)</h3>
            <p class="pool-sub">Teams können ihn beim Annehmen als ersten Prompt übernehmen.</p>
            <MarkdownField value={starter} label="Startprompt" placeholder="Leer lassen, wenn die Teams bei null anfangen sollen." readOnly={false} onChange={setStarter} />
          </section>
        </div>
      ) : (
        <div class="task-preview">
          <p class="pool-sub">So sieht das Team die Aufgabe:</p>
          <div class="task-preview-card">
            <h3>{title || 'Ohne Titel'}</h3>
            <Rendered text={body} />
          </div>
          {starter.trim() && (
            <>
              <p class="pool-sub">Startprompt:</p>
              <pre class="task-starter">{starter}</pre>
            </>
          )}
        </div>
      )}

      <footer class="task-dialog-foot">
        {draft ? (
          <>
            <button class="btn-link danger" onClick={p.onDelete}>
              Löschen
            </button>
            <span class="spacer" />
            <button class="btn btn-quiet" disabled={busy} onClick={() => run(() => p.onSave({ title, body, starter }))}>
              Speichern
            </button>
            <button
              class="btn btn-accent"
              disabled={busy || !title.trim() || !body.trim()}
              title={!body.trim() ? 'Schreibt zuerst einen Aufgabentext.' : undefined}
              onClick={() => {
                if (confirm('Veröffentlichen? Danach lässt sich der Text nicht mehr ändern, nur duplizieren.')) void run(() => p.onPublish({ title, body, starter }))
              }}
            >
              Veröffentlichen
            </button>
          </>
        ) : (
          <>
            <span class="spacer" />
            <button class="btn btn-quiet" onClick={p.onDuplicate}>
              Duplizieren
            </button>
            {p.task.state === 'open' && (
              <button class="btn btn-quiet danger-text" onClick={p.onCloseTask}>
                Aufgabe schließen
              </button>
            )}
          </>
        )}
      </footer>
    </dialog>
  )
}
