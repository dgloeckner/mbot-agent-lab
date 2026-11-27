import { render } from 'preact'
import { useEffect, useState } from 'preact/hooks'
import { COLOR_HEX, onColor } from '../format'
import { browserKV, loadJoined, rememberJoined, type Joined } from '../storage'
import { cleanLabCode, codeFromSearch } from './link'
import './join.css'

const kv = browserKV()

/** The short join code of a team, for its other PCs (POST join-code on the team link). */
async function fetchJoinCode(token: string): Promise<{ code: string; expires_at: string } | null> {
  try {
    const resp = await fetch(`/t/${encodeURIComponent(token)}/join-code`, { method: 'POST' })
    return resp.ok ? await resp.json() : null
  } catch {
    return null
  }
}

/** Another PC of a team comes in with the code a PC of the team shows. */
function JoinByCode() {
  const [code, setCode] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const submit = async (ev: Event) => {
    ev.preventDefault()
    setError(null)
    setBusy(true)
    try {
      const resp = await fetch('/api/join-team', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ code }),
      })
      const data = await resp.json().catch(() => null)
      if (!resp.ok) {
        setError(data?.error ?? `Der Server hat mit Fehler ${resp.status} geantwortet.`)
        return
      }
      rememberJoined(kv, { name: data.name, token: data.token, color: data.color })
      location.href = `/t/${data.token}`
    } catch {
      setError('Keine Verbindung zum Server. Prüft das Netzwerk und versucht es noch einmal.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <form onSubmit={submit} class="join-form join-by-code" aria-labelledby="join-by-code-title">
      <h2 id="join-by-code-title">Zu einem Team dazukommen</h2>
      <label>
        <span>Team-Code</span>
        <input
          class="team-code-input"
          value={code}
          maxLength={7}
          autoComplete="off"
          autoCapitalize="characters"
          spellcheck={false}
          placeholder="z. B. K7P3QX"
          aria-describedby="team-code-hint"
          onInput={(e) => setCode(e.currentTarget.value.toUpperCase().replace(/[^A-Z0-9]/g, '').slice(0, 6))}
          required
        />
        <small id="team-code-hint">Euer Team ist schon gegründet? Auf einem Rechner eures Teams steht der Code hinter dem Link-Knopf oben.</small>
      </label>
      {error && (
        <p class="form-error" role="alert">
          {error}
        </p>
      )}
      <button class="btn btn-quiet btn-big" type="submit" disabled={busy || code.length < 6}>
        {busy ? 'Komme dazu …' : 'Dazukommen'}
      </button>
    </form>
  )
}

function JoinPage() {
  // A join link (the control room's QR code) brings the lab code along.
  const linked = codeFromSearch(location.search)
  const [code, setCode] = useState(linked)
  const [name, setName] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [joined, setJoined] = useState<Joined | null>(null)
  const [copied, setCopied] = useState(false)
  const [codeFocus, setCodeFocus] = useState(false)
  const earlier = loadJoined(kv)
  const [joinCode, setJoinCode] = useState<string | null>(null)
  useEffect(() => {
    if (joined) void fetchJoinCode(joined.token).then((c) => setJoinCode(c?.code ?? null))
  }, [joined?.token])

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
      rememberJoined(kv, j)
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
          {joinCode && (
            <div class="join-code-box">
              <p>
                Auf den anderen Rechnern eures Teams: <strong>{location.host}/join</strong> öffnen, unter „Zu einem Team dazukommen“ diesen Code
                eingeben. Er gilt 10 Minuten; einen neuen zeigt euer Link-Knopf auf der Teamseite.
              </p>
              <code class="join-code">{joinCode}</code>
            </div>
          )}
          <p>
            Oder öffnet dort euren Team-Link. Behaltet Link und Code für euch: Wer sie hat, kann euren Agenten steuern.
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
                onInput={(e) => setCode(cleanLabCode(e.currentTarget.value))}
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
            <input value={name} maxLength={30} placeholder="z. B. Die Kurvenkratzer" autoFocus={linked.length === 4} onInput={(e) => setName(e.currentTarget.value)} required />
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
        <JoinByCode />
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
