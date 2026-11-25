// The team tool editor (Blockly), lazy-loaded into the left pane. The block
// definitions come from the server (GET blocks); the browser keeps no tool
// list and runs nothing: it shows the blocks, saves their state and marks the
// block the server is running.

import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks'
import * as Blockly from 'blockly/core'
import { api } from '../api'
import { defineBlocks, MEDIA } from '../blocklySetup'
import { problemsByBlock, toolName } from '../toolrun'
import {
  agent,
  busy,
  continueTeamTool,
  deploy,
  draft,
  deleteTool,
  editTool,
  newTool,
  openTool,
  readOnly,
  robots,
  runTeamTool,
  stopAll,
  teamTools,
  toolBridge,
  toolRun,
  toolSaver,
} from '../state'
import type { ProgramDetail, ProgramProblem } from '../types'

function markProblems(ws: Blockly.WorkspaceSvg, problems: ProgramProblem[]) {
  const { byBlock } = problemsByBlock(problems)
  for (const b of ws.getAllBlocks(false)) {
    const texts = byBlock[b.id]
    b.setWarningText(texts ? texts.join('\n') : null)
    const root = b.getSvgRoot()
    if (texts) root.classList.add('block-problem')
    else root.classList.remove('block-problem')
  }
}

function Ablauf() {
  const run = toolRun.value
  const list = useRef<HTMLOListElement>(null)
  useLayoutEffect(() => {
    if (list.current) list.current.scrollTop = list.current.scrollHeight
  }, [run.lines.length])
  if (!run.run) return <p class="ablauf-empty">Hier erscheint der Ablauf, sobald ihr das Werkzeug ausführt: jeder Schritt mit seinem Ergebnis.</p>
  return (
    <ol class="ablauf" ref={list} aria-live="polite" aria-label="Ablauf">
      {run.lines.map((l, i) => (
        <li key={i} class={`ablauf-${l.kind}`} data-failed={l.failed || undefined}>
          {l.text}
        </li>
      ))}
      {run.waiting && <li class="ablauf-wait">Wartet auf „Weiter“ …</li>}
    </ol>
  )
}

function Workspace({
  detail,
  problems,
  onProblems,
}: {
  detail: ProgramDetail
  problems: ProgramProblem[]
  onProblems: (p: ProgramProblem[]) => void
}) {
  const host = useRef<HTMLDivElement>(null)
  const ws = useRef<Blockly.WorkspaceSvg | null>(null)
  const meta = useRef({ name: detail.name, description: detail.description })
  // Shown in the name and description fields; replaced when another PC's save is loaded.
  const [shown, setShown] = useState({ name: detail.name, description: detail.description, at: detail.updated_at })
  const ownSave = useRef<string | null>(detail.updated_at)
  const loading = useRef(false)

  useEffect(() => {
    let disposed = false
    void defineBlocks().then(({ toolbox }) => {
      if (disposed || !host.current) return
      const w = Blockly.inject(host.current, {
        toolbox, media: MEDIA, sounds: false, readOnly: readOnly.peek(), trashcan: true,
        zoom: { controls: true, wheel: true, startScale: 0.9 }, move: { scrollbars: true, drag: true, wheel: true },
        grid: { spacing: 24, length: 2, colour: '#dfe3e9', snap: true },
      })
      ws.current = w
      const load = (state: unknown) => {
        loading.current = true
        Blockly.Events.disable()
        try {
          w.clear()
          Blockly.serialization.workspaces.load(state as any, w)
        } finally {
          Blockly.Events.enable()
          loading.current = false
        }
      }
      load(detail.blockly)
      markProblems(w, detail.problems)
      w.addChangeListener((e: Blockly.Events.Abstract) => {
        if (e.isUiEvent || loading.current || readOnly.peek()) return
        save()
      })
      toolBridge.reload = (id) => {
        if (id !== detail.id || toolSaver.pending()) return
        void api.program(id).then((p) => {
          if (p.updated_at === ownSave.current) return
          ownSave.current = p.updated_at
          meta.current = { name: p.name, description: p.description }
          setShown({ name: p.name, description: p.description, at: p.updated_at })
          load(p.blockly)
          markProblems(w, p.problems)
          onProblems(p.problems)
        })
      }
    })
    return () => {
      disposed = true
      void toolSaver.flush()
      toolBridge.reload = undefined
      ws.current?.dispose()
      ws.current = null
    }
  }, [detail.id])

  // Mark the block the server is running.
  const marked = toolRun.value.program === detail.id ? toolRun.value.marked : null
  useEffect(() => {
    const w = ws.current
    if (!w) return
    w.highlightBlock(marked)
  }, [marked])

  const save = () => {
    const w = ws.current
    if (!w) return
    toolSaver.schedule({
      id: detail.id, ...meta.current, blockly: Blockly.serialization.workspaces.save(w),
      done: (p) => {
        ownSave.current = p.updated_at
        if (ws.current) markProblems(ws.current, p.problems)
        onProblems(p.problems)
      },
    })
  }
  return (
    <>
      <MetaFields
        key={detail.id + shown.at}
        name={shown.name}
        description={shown.description}
        problems={problemsByBlock(problems).byField}
        onChange={(m) => {
          meta.current = m
          save()
        }}
      />
      <div class="blockly-host" ref={host} />
    </>
  )
}

function FieldProblems({ id, texts }: { id: string; texts?: string[] }) {
  if (!texts?.length) return null
  return (
    <span class="field-problem" id={id} role="status">
      {texts.join(' ')}
    </span>
  )
}

function MetaFields(props: {
  name: string
  description: string
  problems: Record<string, string[]>
  onChange: (m: { name: string; description: string }) => void
}) {
  const { onChange, problems } = props
  const [name, setName] = useState(props.name)
  const [description, setDescription] = useState(props.description)
  const ro = readOnly.value
  return (
    <div class="tool-meta">
      <label>
        <span>Name</span>
        <input
          value={name}
          disabled={ro}
          maxLength={30}
          spellcheck={false}
          autocapitalize="off"
          placeholder="z. B. vor_und_zurueck"
          title="Kleine Buchstaben, Ziffern und _"
          aria-invalid={problems.name ? true : undefined}
          aria-describedby={problems.name ? 'tool-name-problem' : undefined}
          onInput={(ev) => {
            // Only allowed characters reach the field; keep the caret where the team typed.
            const el = ev.currentTarget
            const caret = toolName(el.value.slice(0, el.selectionStart ?? el.value.length)).length
            const clean = toolName(el.value)
            if (clean !== el.value) {
              el.value = clean
              el.setSelectionRange(caret, caret)
            }
            setName(clean)
            onChange({ name: clean, description })
          }}
        />
      </label>
      <label class="tool-meta-desc">
        <span>Beschreibung für den Agenten</span>
        <input
          value={description}
          disabled={ro}
          maxLength={200}
          placeholder="Was das Werkzeug tut und wann es fertig ist."
          aria-invalid={problems.description ? true : undefined}
          aria-describedby={problems.description ? 'tool-desc-problem' : undefined}
          onInput={(ev) => {
            setDescription(ev.currentTarget.value)
            onChange({ name, description: ev.currentTarget.value })
          }}
        />
      </label>
      <FieldProblems id="tool-name-problem" texts={problems.name} />
      <FieldProblems id="tool-desc-problem" texts={problems.description} />
    </div>
  )
}

/** The left pane in "Werkzeug bauen". */
export default function ToolEditor() {
  const id = openTool.value
  const [detail, setDetail] = useState<ProgramDetail | null>(null)
  const [problems, setProblems] = useState<ProgramProblem[]>([])
  const [robot, setRobot] = useState('')
  useEffect(() => {
    setDetail(null)
    if (!id) return
    void toolSaver.flush().then(() =>
      api.program(id).then((p) => {
        setDetail(p)
        setProblems(p.problems)
      }),
    )
  }, [id])

  const run = toolRun.value
  const thisRuns = run.running && run.program === id
  const blocked = readOnly.value ? 'Die Aufgabe ist geschlossen.' : agent.value.running ? 'Erst anhalten: Euer Agent fährt gerade.' : null
  const team = robots.value
  const target = team.length > 1 ? robot || team[0].alias : undefined
  const general = problemsByBlock(problems).general
  const start = async (step: boolean) => {
    await toolSaver.flush()
    if (id) await runTeamTool(id, step, target)
  }

  return (
    <div class="tool-editor">
      <div class="tool-head">
        <label class="tool-picker">
          <span class="sr-only">Werkzeug</span>
          <select value={id ?? ''} onChange={(ev) => ev.currentTarget.value && editTool(ev.currentTarget.value)}>
            {!id && <option value="">Werkzeug wählen …</option>}
            {teamTools.value.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name}
                {t.valid ? '' : ' (Fehler)'}
              </option>
            ))}
          </select>
        </label>
        {team.length > 1 && (
          <label class="tool-picker">
            <span class="sr-only">Roboter</span>
            <select value={target} onChange={(ev) => setRobot(ev.currentTarget.value)}>
              {team.map((r) => (
                <option key={r.id} value={r.alias}>
                  {r.alias}
                </option>
              ))}
            </select>
          </label>
        )}
        <span class="spacer" />
        {thisRuns && run.waiting ? (
          <button class="btn btn-accent" onClick={() => void continueTeamTool()}>
            Weiter
          </button>
        ) : (
          <>
            <button class="btn btn-accent" disabled={!id || blocked !== null || run.running} onClick={() => void start(false)} title={blocked ?? undefined}>
              Ausführen
            </button>
            <button class="btn btn-quiet" disabled={!id || blocked !== null || run.running} onClick={() => void start(true)} title={blocked ?? 'Hält vor jedem Werkzeug an.'}>
              Einzelschritt
            </button>
          </>
        )}
        <button class="btn btn-stop-quiet" disabled={!run.running} onClick={() => void stopAll()}>
          Stopp
        </button>
        {!readOnly.value && (
          <button class="btn-link" onClick={() => void newTool()}>
            Neues Werkzeug
          </button>
        )}
        {!readOnly.value && (
          <button class="btn btn-quiet" disabled={busy.value !== null || !draft.value.trim()} onClick={() => void deploy(false)} title="Speichert Prompt und alle eigenen Werkzeuge als neue Version.">
            {busy.value === 'deploy' ? 'Deploye …' : 'Deployen'}
          </button>
        )}
      </div>
      {blocked && <p class="console-hint">{blocked}</p>}
      {!id ? (
        <div class="tool-empty">
          <p>Hier baut ihr eigene Werkzeuge aus Blöcken. Euer Agent kann sie später benutzen wie die eingebauten.</p>
          {!readOnly.value && (
            <button class="btn btn-accent" onClick={() => void newTool()}>
              Neues Werkzeug
            </button>
          )}
        </div>
      ) : !detail ? (
        <div class="loading" aria-busy="true" />
      ) : (
        <div class="tool-body">
          <div class="tool-canvas">
            <Workspace key={detail.id} detail={detail} problems={problems} onProblems={setProblems} />
            {general.length > 0 && (
              <ul class="tool-problems" role="status">
                {general.map((t) => (
                  <li key={t}>{t}</li>
                ))}
              </ul>
            )}
          </div>
          <section class="tool-ablauf" aria-label="Ablauf">
            <h3>Ablauf</h3>
            <Ablauf />
            {!readOnly.value && (
              <button
                class="btn-link danger"
                onClick={() => {
                  if (confirm(`Werkzeug „${detail.name}“ löschen?`)) void deleteTool(detail.id)
                }}
              >
                Werkzeug löschen
              </button>
            )}
          </section>
        </div>
      )}
    </div>
  )
}
