import { useState } from 'preact/hooks'
import { FLOOR_FILL, argsOf, fieldsOf, floorSegments, keyFields, resultSentence, traceEnd, type Field } from '../console'
import { clock } from '../format'
import { insertIntoPrompt, runTool, type ToolRun } from '../state'
import type { ToolInfo } from '../types'

const BOOL_LABEL: Record<string, string> = { '': 'Standard', true: 'ja', false: 'nein' }

function FieldInput({ f, value, set, disabled }: { f: Field; value: string; set: (v: string) => void; disabled: boolean }) {
  const label = f.required ? f.name : `${f.name}?`
  const common = { value, disabled, onInput: (ev: Event) => set((ev.currentTarget as HTMLInputElement).value) }
  let input
  if (f.kind === 'enum' || f.kind === 'boolean') {
    const options = f.kind === 'enum' ? f.options! : ['true', 'false']
    input = (
      <select {...common} onChange={(ev) => set(ev.currentTarget.value)}>
        {(!f.required || f.kind === 'boolean') && <option value="">{f.kind === 'boolean' ? BOOL_LABEL[''] : 'keine Angabe'}</option>}
        {f.required && f.kind === 'enum' && !value && <option value="">bitte wählen</option>}
        {options.map((o) => (
          <option key={o} value={o}>
            {f.kind === 'boolean' ? BOOL_LABEL[o] : o}
          </option>
        ))}
      </select>
    )
  } else if (f.kind === 'number') {
    input = <input {...common} inputMode="decimal" type="text" placeholder={f.min !== undefined && f.max !== undefined ? `${f.min} bis ${f.max}` : ''} />
  } else {
    input = <input {...common} type="text" maxLength={f.maxLength} />
  }
  return (
    <label class="console-field" title={f.hint}>
      <span>{label}</span>
      {input}
    </label>
  )
}

/** The floor trace as four thin bands, one per probe from left to right. */
function FloorBand({ result }: { result: Record<string, any> }) {
  const end = traceEnd(result)
  const segs = floorSegments(result.floor, end)
  if (!segs.length) return null
  const total = Math.max(1, segs[segs.length - 1].to - segs[0].from)
  const unit = typeof result.driven_cm === 'number' ? 'cm' : '°'
  const words = segs.map((s) => `${s.from} ${unit}: ${s.pattern}`).join(', ')
  return (
    <div class="floor-band">
      <span>
        Bodenspur, {segs[0].from} bis {Math.round(segs[segs.length - 1].to)} {unit}
        {result.floor_truncated && ' (abgeschnitten)'}
      </span>
      <svg role="img" aria-label={`Bodenspur: ${words}`} viewBox={`0 0 ${total} 4`} preserveAspectRatio="none">
        {segs.flatMap((s, i) =>
          [...s.pattern.padEnd(4, '?').slice(0, 4)].map((ch, row) => (
            <rect key={`${i}-${row}`} x={s.from - segs[0].from} y={row} width={Math.max(0, s.to - s.from)} height={1} fill={FLOOR_FILL[ch] ?? FLOOR_FILL['?']} />
          )),
        )}
      </svg>
    </div>
  )
}

function Result({ run }: { run: ToolRun }) {
  const r = run.result
  const fields = keyFields(r)
  return (
    <div class="console-result" data-ok={r.ok !== false}>
      <div class="console-result-head">
        <span>Rückgabe</span>
        <span>
          {clock(run.at)} · {(run.ms / 1000).toLocaleString('de-DE', { maximumFractionDigits: 1 })} s
        </span>
      </div>
      <p>{resultSentence(r)}</p>
      {fields.length > 0 && (
        <div class="console-keys">
          {fields.map(([k, v]) => (
            <span key={k}>
              {k}: {v}
            </span>
          ))}
        </div>
      )}
      <FloorBand result={r} />
      <details>
        <summary>JSON anzeigen</summary>
        <pre>{JSON.stringify(r, null, 2)}</pre>
      </details>
    </div>
  )
}

/** An opened by-hand tool: its form, "Ausführen" and the last result. */
export default function ToolConsole({ tool, robot, blocked }: { tool: ToolInfo; robot?: string; blocked: string | null }) {
  const fields = fieldsOf(tool.schema)
  const [values, setValues] = useState<Record<string, string>>({})
  const [running, setRunning] = useState(false)
  const [run, setRun] = useState<ToolRun | null>(null)
  const disabled = running || blocked !== null
  const submit = async (ev: Event) => {
    ev.preventDefault()
    if (disabled) return
    setRunning(true)
    const got = await runTool(tool.name, argsOf(fields, values, robot))
    setRunning(false)
    if (got) setRun(got)
  }
  return (
    <form class="console" onSubmit={submit} aria-label={`${tool.name} ausführen`}>
      {fields.length > 0 && (
        <div class="console-fields">
          {fields.map((f) => (
            <FieldInput key={f.name} f={f} value={values[f.name] ?? ''} disabled={disabled} set={(v) => setValues({ ...values, [f.name]: v })} />
          ))}
        </div>
      )}
      <div class="console-actions">
        <button class="btn btn-accent" type="submit" disabled={disabled}>
          <svg aria-hidden="true" width="13" height="13" viewBox="0 0 24 24">
            <path d="M7 4v16l13-8z" fill="currentColor" />
          </svg>
          {running ? 'Läuft …' : 'Ausführen'}
        </button>
        <button class="btn-link" type="button" onClick={() => insertIntoPrompt(tool.name)}>
          In den Prompt einfügen
        </button>
        {blocked && <span class="console-blocked">{blocked}</span>}
      </div>
      {run && <Result run={run} />}
    </form>
  )
}
