import { render } from 'preact'
import { useState } from 'preact/hooks'
import { COLOR_HEX, onColor } from '../format'
import { browserKV } from '../storage'
import './join.css'

// Teams this browser has joined, so a team finds its way back without the link.
interface Joined {
  name: string
  token: string
  color: string
}
const JOINED_KEY = 'mbot-lab:v1:joined'
const kv = browserKV()

function loadJoined(): Joined[] {
  try {
    const v = JSON.parse(kv.getItem(JOINED_KEY) ?? '[]')
    return Array.isArray(v) ? v : []
  } catch {
    return []
  }
}

function remember(j: Joined) {
  const list = [j, ...loadJoined().filter((x) => x.token !== j.token)].slice(0, 5)
  try {
    kv.setItem(JOINED_KEY, JSON.stringify(list))
  } catch {
    /* storage full or blocked: the link is still shown */
  }
}

function JoinPage() {
  const [code, setCode] = useState('')
  const [name, setName] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [joined, setJoined] = useState<Joined | null>(null)
  const [copied, setCopied] = useState(false)
  const [codeFocus, setCodeFocus] = useState(false)
  const earlier = loadJoined()

  const submit = async (ev: Event) => {
    ev.preventDefault()
    setError(null)
    setBusy(true)
    try {
      const resp = await fetch('/api/join', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ code, name }),
      })
      const data = await resp.json().catch(() => null)
      if (!resp.ok) {
        setError(data?.error ?? `Der Server hat mit Fehler ${resp.status} geantwortet.`)
        return
      }
      const j = { name: data.name, token: data.token, color: data.color }
      remember(j)
      setJoined(j)
    } catch {
      setError('Keine Verbindung zum Server. Prüft das Netzwerk und versucht es noch einmal.')
    } finally {
      setBusy(false)
    }
  }

  if (joined) {
    const link = `${location.origin}/t/${joined.token}`
    const accent = COLOR_HEX[joined.color] ?? COLOR_HEX.blue
    return (
      <main class="join" style={{ '--accent': accent, '--on-accent': onColor(accent) }}>
        <section class="join-card welcome">
          <h1>Willkommen, {joined.name}!</h1>
          <p>
            Das ist euer Team-Link. Öffnet ihn auf den anderen Rechnern eures Teams. Behaltet ihn für euch: Wer den Link hat, kann euren
            Agenten steuern.
          </p>
          <div class="team-link">
            <code>{link}</code>
            <button
              class="btn btn-quiet btn-small"
              onClick={async () => {
                await navigator.clipboard?.writeText(link).catch(() => undefined)
                setCopied(true)
              }}
            >
              {copied ? 'Kopiert' : 'Link kopieren'}
            </button>
          </div>
          <a class="btn btn-accent btn-big" href={`/t/${joined.token}`}>
            Zur Teamseite
          </a>
          <p class="hint">Gleich teilt euch der Leitstand einen oder mehrere Roboter zu.</p>
        </section>
      </main>
    )
  }

  return (
    <main class="join">
      <section class="join-card">
        <p class="join-kicker">mBot2 Agent Lab</p>
        <h1>Gründet euer Team</h1>
        <form onSubmit={submit} class="join-form">
          <label>
            <span>Lab-Code</span>
            <div class="code-box">
              <input
                class="code-input"
                value={code}
                maxLength={4}
                autoComplete="off"
                autoCapitalize="characters"
                spellcheck={false}
                aria-describedby="code-hint"
                onFocus={() => setCodeFocus(true)}
                onBlur={() => setCodeFocus(false)}
                onInput={(e) => setCode(e.currentTarget.value.toUpperCase().replace(/[^A-Z0-9]/g, '').slice(0, 4))}
                required
              />
              {[0, 1, 2, 3].map((i) => (
                <span key={i} class="tile" aria-hidden="true" data-active={codeFocus && i === Math.min(code.length, 3)}>
                  {code[i] ?? ''}
                </span>
              ))}
            </div>
            <small id="code-hint">Steht beim Leitstand oder auf dem Beamer.</small>
          </label>
          <label>
            <span>Teamname</span>
            <input value={name} maxLength={30} placeholder="z. B. Die Kurvenkratzer" onInput={(e) => setName(e.currentTarget.value)} required />
          </label>
          {error && (
            <p class="form-error" role="alert">
              {error}
            </p>
          )}
          <button class="btn btn-accent btn-big" type="submit" disabled={busy || code.length < 4 || name.trim().length < 2}>
            {busy ? 'Gründe …' : 'Team gründen'}
          </button>
        </form>
        {earlier.length > 0 && (
          <div class="earlier">
            <h2>Auf diesem Rechner schon dabei</h2>
            <ul>
              {earlier.map((j) => (
                <li key={j.token}>
                  <span class="swatch-dot" style={{ background: COLOR_HEX[j.color] ?? COLOR_HEX.blue }} aria-hidden="true" />
                  <a href={`/t/${j.token}`}>{j.name}</a>
                </li>
              ))}
            </ul>
          </div>
        )}
      </section>
    </main>
  )
}

export function mount(el: HTMLElement) {
  document.title = 'Team gründen: mBot2 Agent Lab'
  render(<JoinPage />, el)
}
