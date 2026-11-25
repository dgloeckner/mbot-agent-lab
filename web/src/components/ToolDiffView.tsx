// A team tool in two states side by side, for "Verlauf": each side is a
// read-only Blockly workspace loaded from its stored block state, with marks
// per block id from the server's diff. Lazy-loaded with Blockly.

import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks'
import * as Blockly from 'blockly/core'
import { api } from '../api'
import { defineBlocks, MEDIA } from '../blocklySetup'
import { LEGEND, layoutFor, marksFor, stateLabel, textChanges, type SideMarks } from '../tooldiff'
import type { ToolDiff, ToolState } from '../types'

const MIN_H = 140
const MAX_H = 560
const PAD = 24

function mark(ws: Blockly.WorkspaceSvg, marks: SideMarks) {
  for (const [id, kinds] of Object.entries(marks.blocks)) {
    const root = ws.getBlockById(id)?.getSvgRoot()
    for (const k of kinds) root?.classList.add(`diff-${k}`)
  }
  for (const [id, fields] of Object.entries(marks.fields)) {
    const block = ws.getBlockById(id)
    for (const f of fields) block?.getField(f)?.getSvgRoot()?.classList.add('diff-field')
  }
}

/** One read-only workspace that fits the height of its blocks and scrolls sideways. */
function Picture({ tool, marks, label }: { tool: ToolState; marks: SideMarks; label: string }) {
  const host = useRef<HTMLDivElement>(null)
  useEffect(() => {
    let ws: Blockly.WorkspaceSvg | null = null
    let disposed = false
    void defineBlocks().then(() => {
      if (disposed || !host.current) return
      ws = Blockly.inject(host.current, {
        readOnly: true,
        media: MEDIA,
        sounds: false,
        move: { scrollbars: true, drag: true, wheel: false },
        zoom: { controls: false, wheel: false, startScale: 0.8 },
      })
      Blockly.Events.disable()
      try {
        Blockly.serialization.workspaces.load(tool.blockly as any, ws)
      } finally {
        Blockly.Events.enable()
      }
      mark(ws, marks)
      const box = ws.getBlocksBoundingBox()
      const h = Math.min(MAX_H, Math.max(MIN_H, (box.bottom - box.top) * ws.scale + 2 * PAD))
      host.current.style.height = `${Math.round(h)}px`
      Blockly.svgResize(ws)
      const m = ws.getMetricsManager().getContentMetrics()
      ws.scroll(-m.left + PAD, -m.top + PAD)
    })
    return () => {
      disposed = true
      ws?.dispose()
    }
  }, [tool, marks])
  return <div class="tooldiff-canvas" ref={host} role="img" aria-label={`Blöcke von „${tool.name}“ in ${label}`} />
}

function Side({ which, version, tool, marks }: { which: 'left' | 'right'; version: number; tool: ToolState | null; marks: SideMarks }) {
  const label = stateLabel(version)
  return (
    <section class={`tooldiff-side tooldiff-${which}`} aria-label={label}>
      <header>
        <strong>{label}</strong>
        {!tool && (
          <span class="tool-change" data-change={which === 'left' ? 'neu' : 'entfernt'}>
            {which === 'left' ? 'neu' : 'entfernt'}
          </span>
        )}
      </header>
      {tool ? (
        <Picture tool={tool} marks={marks} label={label} />
      ) : (
        <p class="tooldiff-missing">
          {which === 'left' ? `Gab es in ${label} noch nicht.` : `Entfernt: gibt es in ${version === 0 ? 'eurem Stand jetzt' : label} nicht mehr.`}
        </p>
      )}
    </section>
  )
}

export default function ToolDiffView({ version, against, id }: { version: number; against: number; id: string }) {
  const [d, setD] = useState<ToolDiff | null>(null)
  const [error, setError] = useState<string | null>(null)
  const root = useRef<HTMLDivElement>(null)
  const [layout, setLayout] = useState<'side' | 'stacked'>('side')

  useEffect(() => {
    setD(null)
    setError(null)
    api.compareTool(version, against, id).then(setD, (e) => setError((e as Error).message))
  }, [version, against, id])

  useLayoutEffect(() => {
    const el = root.current
    if (!el) return
    const ro = new ResizeObserver(() => setLayout(layoutFor(el.clientWidth)))
    ro.observe(el)
    setLayout(layoutFor(el.clientWidth))
    return () => ro.disconnect()
  }, [])

  const marks = d ? marksFor(d.changes) : null
  const texts = d ? textChanges(d.changes) : []
  const name = d?.right.tool?.name ?? d?.left.tool?.name ?? ''
  const blockChanges = d?.changes.filter((c) => c.block).length ?? 0

  return (
    <div class="tooldiff" ref={root} data-layout={layout}>
      {error && <p class="error-text">{error}</p>}
      {!d && !error && <div class="loading" aria-busy="true" />}
      {d && marks && (
        <>
          <p class="tooldiff-title">
            Werkzeug „{name}“: {stateLabel(d.left.version)} links, {stateLabel(d.right.version)} rechts
            {d.left.tool && d.right.tool && blockChanges === 0 && texts.length === 0 ? '. Keine Änderung.' : ''}
          </p>
          {texts.length > 0 && (
            <dl class="tooldiff-text">
              {texts.map((t) => (
                <div key={t.label}>
                  <dt>{t.label}</dt>
                  <dd>
                    <del>{t.old || '(leer)'}</del> <span aria-hidden="true">→</span> <ins>{t.new || '(leer)'}</ins>
                  </dd>
                </div>
              ))}
            </dl>
          )}
          <div class="tooldiff-sides">
            <Side which="left" version={d.left.version} tool={d.left.tool} marks={marks.left} />
            <Side which="right" version={d.right.version} tool={d.right.tool} marks={marks.right} />
          </div>
          <ul class="tooldiff-legend" aria-label="Legende">
            {LEGEND.map((l) => (
              <li key={l.kind} data-kind={l.kind}>
                <span class="tooldiff-sign" aria-hidden="true">
                  {l.sign}
                </span>
                <span class="tooldiff-swatch" aria-hidden="true" />
                {l.text}
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  )
}
