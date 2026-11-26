// The team tool editor (Blockly), lazy-loaded into the left pane. The block
// definitions come from the server (GET blocks); the browser keeps no tool
// list and runs nothing: it shows the blocks, saves their state and marks the
// block the server is running.

import type { ComponentChildren } from 'preact'
import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks'
import * as Blockly from 'blockly/core'
import { api } from '../api'
import { darkTheme, defineBlocks, lightTheme, MEDIA, pageIsDark } from '../blocklySetup'
import { isEmptyTool, problemsByBlock, toolName } from '../toolrun'
import {
  agent,
  busy,
  continueTeamTool,
  deploy,
  deployed,
  dirty,
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
  wideTool,
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
  children,
}: {
  detail: ProgramDetail
  problems: ProgramProblem[]
  onProblems: (p: ProgramProblem[]) => void
  children?: ComponentChildren
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
    let stopResize = () => {}
    void defineBlocks().then(({ toolbox }) => {
      if (disposed || !host.current) return
      const w = Blockly.inject(host.current, {
        toolbox, media: MEDIA, sounds: false, readOnly: readOnly.peek(), trashcan: true,
        zoom: { controls: true, wheel: true, startScale: 0.9 }, move: { scrollbars: true, drag: true, wheel: true },
        grid: { spacing: 24, length: 2, colour: '#dfe3e9', snap: true }, // the dots take the page color in styles.css
        theme: pageIsDark() ? darkTheme : lightTheme,
      })
      // The canvas follows the page theme (the band's switch or the system's).
      const media = window.matchMedia('(prefers-color-scheme: dark)')
      const retheme = () => w.setTheme(pageIsDark() ? darkTheme : lightTheme)
      media.addEventListener('change', retheme)
      const themeObserver = new MutationObserver(retheme)
      themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
      ws.current = w
      // Splitter, window and "Groß" change the host's size; Blockly does not notice by itself.
      const resize = new ResizeObserver(() => Blockly.svgResize(w))
      resize.observe(host.current)
      stopResize = () => {
        resize.disconnect()
        media.removeEventListener('change', retheme)
        themeObserver.disconnect()
      }
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
      stopResize()
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
      >
        {children}
      </MetaFields>
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
  children?: ComponentChildren
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
        <textarea
          value={description}
          disabled={ro}
          maxLength={200}
          rows={2}
          placeholder="Was das Werkzeug tut und wann es fertig ist. Danach entscheidet euer Agent."
          aria-invalid={problems.description ? true : undefined}
          aria-describedby={problems.description ? 'tool-desc-problem' : undefined}
          onInput={(ev) => {
            setDescription(ev.currentTarget.value)
            onChange({ name, description: ev.currentTarget.value })
          }}
        />
      </label>
      <div class="tool-meta-extra">{props.children}</div>
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
  // "Ablauf" takes room from the canvas: shown while this tool runs or ran, or when the team opens it.
  const [ablaufOpen, setAblaufOpen] = useState(false)
  useEffect(() => {
    if (run.run && run.program === id) setAblaufOpen(true)
  }, [run.run, id])
  const blocked = readOnly.value ? 'Die Aufgabe ist geschlossen.' : agent.value.running ? 'Erst anhalten: Euer Agent fährt gerade.' : null
  const team = robots.value
  const target = team.length > 1 ? robot || team[0].alias : undefined
  const general = problemsByBlock(problems).general
  // An empty tool is not broken, only not built yet.
  const empty = isEmptyTool(problems)
  const start = async (step: boolean) => {
    await toolSaver.flush()
    if (id) await runTeamTool(id, step, target)
  }

  return (
    <div class="tool-editor">
      <div class="tool-head">
        <label class="tool-picker tool-picker-main">
          <span class="sr-only">Werkzeug</span>
          <select value={id ?? ''} onChange={(ev) => ev.currentTarget.value && editTool(ev.currentTarget.value)}>
            {!id && <option value="">Werkzeug wählen …</option>}
            {teamTools.value.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name}
                {isEmptyTool(t.problems) ? ' (leer)' : t.valid ? '' : ' (Fehler)'}
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
        <button
          class="btn btn-quiet icon-only"
          aria-pressed={wideTool.value}
          aria-label={wideTool.value ? 'Normal' : 'Groß'}
          onClick={() => (wideTool.value = !wideTool.value)}
          title={
            wideTool.value
              ? 'Normal: die rechte Seite wieder zeigen'
              : 'Groß: Der Editor bekommt die ganze Breite, die rechte Seite ist so lange ausgeblendet.'
          }
        >
          <svg aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            {wideTool.value ? <path d="M9 4v5H4M15 4v5h5M9 20v-5H4M15 20v-5h5" /> : <path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" />}
          </svg>
        </button>
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
          <button class="btn-link" onClick={() => void newTool()} aria-label="Neues Werkzeug" title="Neues Werkzeug">
            + Neu
          </button>
        )}
        {!readOnly.value && (
          <button
            class="btn btn-quiet"
            disabled={busy.value !== null || !draft.value.trim()}
            onClick={() => void deploy(false)}
            title={
              dirty.value
                ? 'Speichert alle eigenen Werkzeuge und den geänderten Prompt als neue Version. Schaut euch den Prompt vorher an.'
                : 'Speichert Prompt und alle eigenen Werkzeuge als neue Version.'
            }
          >
            {busy.value === 'deploy' ? 'Deploye …' : 'Deployen'}
            {dirty.value && deployed.value && <span class="deploy-with-prompt"> + Prompt</span>}
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
        <div class="tool-body" data-ablauf={ablaufOpen || undefined}>
          <div class="tool-canvas">
            <Workspace key={detail.id} detail={detail} problems={problems} onProblems={setProblems}>
              <button
                class="btn-link"
                aria-expanded={ablaufOpen}
                aria-controls="tool-ablauf"
                onClick={() => setAblaufOpen(!ablaufOpen)}
                title="Jeder Schritt des letzten Ausführens mit seinem Ergebnis"
              >
                {ablaufOpen ? 'Ablauf ausblenden' : 'Ablauf zeigen'}
              </button>
              {!readOnly.value && (
                <button
                  class="btn-link danger"
                  onClick={() => {
                    if (confirm(`Werkzeug „${detail.name}“ löschen?`)) void deleteTool(detail.id)
                  }}
                >
                  Löschen
                </button>
              )}
            </Workspace>
            {empty ? (
              <p class="tool-hint" role="status">
                Noch leer: Zieht Blöcke aus den Kategorien links unter „Wenn das Werkzeug startet“.
              </p>
            ) : (
              general.length > 0 && (
                <ul class="tool-problems" role="status">
                  {general.map((t) => (
                    <li key={t}>{t}</li>
                  ))}
                </ul>
              )
            )}
          </div>
          {ablaufOpen && (
            <section class="tool-ablauf" id="tool-ablauf" aria-label="Ablauf">
              <h3>Ablauf</h3>
              <Ablauf />
            </section>
          )}
        </div>
      )}
    </div>
  )
}
