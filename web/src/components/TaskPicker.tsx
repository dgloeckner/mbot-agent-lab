import { useEffect, useRef, useState } from 'preact/hooks'
import { accepting, agent, switchTask, task, tasks } from '../state'
import { pickerLists } from '../taskview'

/** The active task in the team band, with a menu to switch tasks and accept new ones. */
export function TaskPicker() {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const { accepted, fresh } = pickerLists(tasks.value)
  const running = agent.value.running
  const t = task.value

  useEffect(() => {
    if (!open) return
    const close = (ev: Event) => {
      if (ev instanceof KeyboardEvent ? ev.key === 'Escape' : !ref.current?.contains(ev.target as Node)) setOpen(false)
    }
    document.addEventListener('pointerdown', close)
    document.addEventListener('keydown', close)
    return () => {
      document.removeEventListener('pointerdown', close)
      document.removeEventListener('keydown', close)
    }
  }, [open])

  return (
    <div class="task-picker" ref={ref}>
      <button class="task-current" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen(!open)} title="Aufgabe wechseln oder neue annehmen">
        <span class="task-label">Aufgabe</span>
        <span class="task-name">{t ? t.title : 'keine'}</span>
        {t?.state === 'closed' && <span class="task-flag">geschlossen</span>}
        {fresh.length > 0 && (
          <span class="tab-badge" aria-label={`${fresh.length} neue Aufgaben`}>
            {fresh.length}
          </span>
        )}
      </button>
      {open && (
        <div class="task-menu" role="menu">
          {accepted.length > 0 && <p class="task-menu-head">Eure Aufgaben{running && <span> · Erst anhalten, dann wechseln</span>}</p>}
          {accepted.map((x) => (
            <button
              key={x.id}
              role="menuitemradio"
              aria-checked={x.id === t?.id}
              class="task-menu-item"
              disabled={running && x.id !== t?.id}
              title={running && x.id !== t?.id ? 'Erst anhalten' : undefined}
              onClick={() => (setOpen(false), void switchTask(x))}
            >
              <span>{x.title}</span>
              {x.state === 'closed' && <span class="task-flag">geschlossen</span>}
            </button>
          ))}
          {fresh.length > 0 && <p class="task-menu-head">Neu</p>}
          {fresh.map((x) => (
            <button
              key={x.id}
              role="menuitem"
              class="task-menu-item"
              disabled={running}
              title={running ? 'Erst anhalten' : undefined}
              onClick={() => (setOpen(false), (accepting.value = x))}
            >
              <span>{x.title}</span>
              <span class="task-new">neu</span>
            </button>
          ))}
          {accepted.length === 0 && fresh.length === 0 && <p class="task-menu-head">Noch keine Aufgabe.</p>}
        </div>
      )}
    </div>
  )
}
