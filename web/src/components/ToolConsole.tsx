import { useEffect, useRef, useState } from 'preact/hooks'
import { FLOOR_FILL, argsOf, fieldsOf, floorSegments, keyFields, missingRequired, resultSentence, traceEnd, type Field } from '../console'
import { clock } from '../format'
import { insertIntoPrompt, runTool, type ToolRun } from '../state'
import type { ToolInfo } from '../types'

const BOOL_LABEL: Record<string, string> = { '': 'Standard', true: 'ja', false: 'nein' }

function FieldInput({ f, value, set, disabled, invalid }: { f: Field; value: string; set: (v: string) => void; disabled: boolean; invalid: boolean }) {
  const common = {
    value,
    disabled,
    name: f.name,
    'aria-invalid': invalid || undefined,
    onInput: (ev: Event) => set((ev.currentTarget as HTMLInputElement).value),
  }
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
    <label class="console-field" title={f.hint} data-optional={!f.required || undefined}>
      <span>
        {f.name}
        {!f.required && <span class="console-optional"> optional</span>}
      </span>
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

function Result({ run, old }: { run: ToolRun; old: boolean }) {
  const r = run.result
  const fields = keyFields(r)
  return (
    <div class="console-result" data-ok={r.ok !== false} data-old={old || undefined} aria-busy={old || undefined}>
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
  const fields = fieldsOf(tool.schema, tool.params)
  const [values, setValues] = useState<Record<string, string>>({})
  const [running, setRunning] = useState(false)
  const [run, setRun] = useState<ToolRun | null>(null)
  const [missing, setMissing] = useState<string | null>(null)
  const actions = useRef<HTMLDivElement>(null)
  const result = useRef<HTMLDivElement>(null)
  const disabled = running || blocked !== null
  // The answer comes into view when it arrives: on a laptop it would start at the fold.
  useEffect(() => {
    if (run) result.current?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  }, [run])
  const submit = async (ev: Event) => {
    ev.preventDefault()
    if (disabled) return
    const lack = missingRequired(fields, values)
    setMissing(lack)
    if (lack) return // the browser says it; nothing goes to the server
    setRunning(true)
    actions.current?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
    const got = await runTool(tool.name, argsOf(fields, values, robot))
    setRunning(false)
    if (got) setRun(got)
  }
  const lacking = missing ? fields.find((f) => missing.includes(`"${f.name}"`))?.name : undefined
  return (
    <form class="console" onSubmit={submit} aria-label={`${tool.name} ausführen`} noValidate>
      {fields.length > 0 && (
        <div class="console-fields">
          {fields.map((f) => (
            <FieldInput
              key={f.name}
              f={f}
              value={values[f.name] ?? ''}
              disabled={disabled}
              invalid={f.name === lacking}
              set={(v) => {
                setValues({ ...values, [f.name]: v })
                if (f.name === lacking) setMissing(null)
              }}
            />
          ))}
        </div>
      )}
      <div class="console-actions" ref={actions}>
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
        {missing && (
          <span class="console-missing" role="alert">
            {missing}
          </span>
        )}
      </div>
      {run && (
        <div ref={result}>
          <Result run={run} old={running} />
        </div>
      )}
    </form>
  )
}
