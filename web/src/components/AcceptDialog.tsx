// Dialog to accept a task: shows its text and asks how the prompt starts.
// Loaded on demand.
import { useEffect, useRef, useState } from 'preact/hooks'
import { api } from '../api'
import { acceptTask, accepting, showToast, tasks } from '../state'
import type { PromptChoice, TeamTask } from '../types'
import TaskText from './TaskText'

/** Loads the task with its text and starter prompt (lists leave them out), then shows the dialog. */
export default function AcceptDialog({ task }: { task: TeamTask }) {
  const [full, setFull] = useState<TeamTask | null>(null)
  useEffect(() => {
    api.task(task.id).then(setFull, (err: Error) => {
      showToast(err.message, 'error')
      accepting.value = null
    })
  }, [task.id])
  return full ? <Dialog task={full} /> : null
}

function Dialog({ task }: { task: TeamTask }) {
  const dlg = useRef<HTMLDialogElement>(null)
  const others = tasks.value.filter((t) => t.status !== 'assigned' && t.id !== task.id)
  const [choice, setChoice] = useState<PromptChoice>(task.starter ? 'starter' : 'empty')
  const [from, setFrom] = useState(others[0]?.id ?? '')
  const [busy, setBusy] = useState(false)
  useEffect(() => dlg.current?.showModal(), [])
  const close = () => (accepting.value = null)

  return (
    <dialog ref={dlg} class="accept-dialog" aria-labelledby="accept-title" onCancel={(ev) => (ev.preventDefault(), close())}>
      <header class="accept-head">
        <span class="accept-kicker">Neue Aufgabe</span>
        <h2 id="accept-title">{task.title}</h2>
      </header>
      <section class="accept-start" aria-label="Womit beginnt euer Prompt?">
        <fieldset class="accept-choice">
          <legend>Womit beginnt euer Prompt?</legend>
          <div class="accept-options">
            {task.starter && (
              <label>
                <input type="radio" name="start" checked={choice === 'starter'} onChange={() => setChoice('starter')} />
                Startprompt der Aufgabe
              </label>
            )}
            <label>
              <input type="radio" name="start" checked={choice === 'empty'} onChange={() => setChoice('empty')} />
              Leer
            </label>
            {others.length > 0 && (
              <label>
                <input type="radio" name="start" checked={choice === 'copy'} onChange={() => setChoice('copy')} />
                Prompt aus einer anderen Aufgabe
              </label>
            )}
          </div>
        </fieldset>
        {/* The chosen option, previewed with a fixed height. */}
        {choice === 'starter' && task.starter && (
          <pre class="accept-starter" aria-label="Startprompt">
            {task.starter}
          </pre>
        )}
        {choice === 'empty' && <p class="accept-preview-note">Ihr schreibt den Prompt selbst.</p>}
        {choice === 'copy' && (
          <p class="accept-preview-note">
            Prompt und eigene Werkzeuge aus{' '}
            <select value={from} onChange={(e) => setFrom(e.currentTarget.value)} aria-label="Aufgabe, aus der der Prompt kommt">
              {others.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.title}
                </option>
              ))}
            </select>{' '}
            übernehmen.
          </p>
        )}
      </section>
      <div class="accept-body">
        <p class="accept-note" role="note">
          Die Aufgabe für euch. Euer Agent sieht diesen Text nicht. Schreibt in euren Prompt, was er wissen muss.
        </p>
        <TaskText task={task} />
      </div>
      <footer class="accept-foot">
        <button class="btn btn-quiet" onClick={close}>
          Später
        </button>
        <button
          class="btn btn-accent"
          disabled={busy}
          onClick={async () => {
            setBusy(true)
            const ok = await acceptTask(task, choice, choice === 'copy' ? from : undefined)
            setBusy(false)
            if (ok) close()
          }}
        >
          Aufgabe annehmen
        </button>
      </footer>
    </dialog>
  )
}
