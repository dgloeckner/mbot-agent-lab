// Application state (Preact signals) and the actions that change it.

import { computed, effect, signal } from '@preact/signals'
import { api, ApiError, connectEvents, type ConnState } from './api'
import { LocalStore, browserKV, shouldAutoSnapshot, type Prefs, type Snapshot } from './storage'
import type { LogEntry, Note, PromptVersion, Robot, Session } from './types'

const kv = browserKV()
let local: LocalStore

export const session = signal<Session | null>(null)
export const loadError = signal<string | null>(null)
export const robots = signal<Robot[]>([])
export const log = signal<LogEntry[]>([])
export const conn = signal<ConnState>('connecting')
export const notes = signal<Note[]>([])

/** Deployed versions, newest first. `deployed` is the newest one with text. */
export const versions = signal<PromptVersion[]>([])
export const deployed = signal<PromptVersion | null>(null)

/** The editor content and the server version it was last synced with. */
export const draft = signal('')
export const draftBase = signal(0)
export const draftSavedAt = signal<number | null>(null)
export const history = signal<Snapshot[]>([])

/** A newer version deployed from another browser while this draft had changes. */
export const remoteVersion = signal<PromptVersion | null>(null)

export const prefs = signal<Prefs>(LocalStore.loadPrefs(kv))
export const toast = signal<{ text: string; kind: 'ok' | 'error' } | null>(null)
export const busy = signal<'deploy' | 'stop' | null>(null)

/** Set by the editor so other parts of the page can replace its content or insert text. */
export const editorBridge: {
  replace?: (text: string) => void
  insert?: (text: string) => void
  focus?: () => void
} = {}

export const dirty = computed(() => draft.value !== (deployed.value?.text ?? ''))

const MAX_LOG = 500
let toastTimer: number | undefined

export function showToast(text: string, kind: 'ok' | 'error' = 'ok') {
  toast.value = { text, kind }
  window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => (toast.value = null), kind === 'error' ? 6000 : 2800)
}

function fail(err: unknown) {
  showToast(err instanceof ApiError ? err.message : 'Etwas ist schiefgegangen. Versucht es noch einmal.', 'error')
}

export function updatePrefs(patch: Partial<Prefs>) {
  prefs.value = { ...prefs.value, ...patch }
  LocalStore.savePrefs(kv, prefs.value)
}

// Apply the theme preference to the document.
effect(() => {
  const t = prefs.value.theme
  if (t === 'system') delete document.documentElement.dataset.theme
  else document.documentElement.dataset.theme = t
})

export async function init() {
  let s: Session
  try {
    s = await api.session()
  } catch (err) {
    loadError.value = err instanceof Error ? err.message : String(err)
    return
  }
  local = new LocalStore(kv, s.team.id)
  session.value = s
  robots.value = s.robots
  notes.value = s.notes
  versions.value = s.versions
  deployed.value = s.prompt
  history.value = local.history()

  const saved = local.loadDraft()
  const serverText = s.prompt?.text ?? ''
  if (saved && saved.text !== serverText) {
    draft.value = saved.text
    draftBase.value = saved.base
    draftSavedAt.value = saved.savedAt
    if (s.prompt && s.prompt.version > saved.base) remoteVersion.value = s.prompt
  } else {
    draft.value = serverText
    draftBase.value = s.prompt?.version ?? 0
  }

  connectEvents({
    status: (st) => (conn.value = st),
    robots: (r) => (robots.value = r),
    log: (e) => {
      const cur = log.value
      if (cur.length && e.id <= cur[cur.length - 1].id) return // replayed after reconnect
      log.value = cur.length >= MAX_LOG ? [...cur.slice(-MAX_LOG + 1), e] : [...cur, e]
      if (e.kind === 'deploy') void onRemoteDeploy(Number(e.data?.version))
    },
  })
}

let saveTimer: number | undefined
let snapTimer: number | undefined

/** Called by the editor on every change. */
export function setDraft(text: string) {
  draft.value = text
  window.clearTimeout(saveTimer)
  saveTimer = window.setTimeout(() => {
    const at = Date.now()
    if (local.saveDraft({ text, base: draftBase.value, savedAt: at })) draftSavedAt.value = at
  }, 300)
  window.clearTimeout(snapTimer)
  snapTimer = window.setTimeout(() => {
    if (shouldAutoSnapshot(history.value[0], draft.value, Date.now())) {
      history.value = local.addSnapshot(draft.value, 'auto')
    }
  }, 2500)
}

export function snapshotNow() {
  if (!draft.value.trim()) return
  history.value = local.addSnapshot(draft.value, 'manual')
  showToast('Zwischenstand in diesem Browser gesichert')
}

/** Replaces the editor content, keeping the previous text in the history. */
export function restore(text: string, label: string) {
  if (draft.value.trim() && draft.value !== text) {
    history.value = local.addSnapshot(draft.value, 'restore')
  }
  editorBridge.replace?.(text)
  showToast(`${label} wiederhergestellt`)
}

export async function deploy(restart: boolean) {
  if (busy.value) return
  busy.value = 'deploy'
  const text = draft.value // the user may keep typing while the request runs
  try {
    const p = await api.deploy(text, restart)
    applyDeployed({ ...p, text })
    history.value = local.addSnapshot(text, 'deploy', p.version)
    showToast(`Version ${p.version} deployt`)
  } catch (err) {
    fail(err)
  } finally {
    busy.value = null
  }
}

function applyDeployed(p: PromptVersion) {
  deployed.value = p
  draftBase.value = p.version
  remoteVersion.value = null
  if (!versions.value.some((v) => v.version === p.version)) {
    versions.value = [{ version: p.version, created_at: p.created_at, chars: p.chars }, ...versions.value]
  }
  local.saveDraft({ text: draft.value, base: p.version, savedAt: Date.now() })
}

/** A deploy seen in the live log: ours, or one from another browser of the team. */
async function onRemoteDeploy(version: number) {
  if (!version || version <= draftBase.value) return
  let p: PromptVersion
  try {
    p = await api.prompt(version)
  } catch {
    return
  }
  if (p.version <= draftBase.value) return
  const clean = draft.value === (deployed.value?.text ?? '') || draft.value === p.text
  if (clean) {
    editorBridge.replace?.(p.text ?? '')
    applyDeployed(p)
  } else {
    deployed.value = p
    if (!versions.value.some((v) => v.version === p.version)) versions.value = [p, ...versions.value]
    remoteVersion.value = p
  }
}

/** Keep the local draft, but treat it as based on the newer version so the hint does not return. */
export function keepDraft() {
  const p = remoteVersion.value
  if (!p) return
  remoteVersion.value = null
  draftBase.value = p.version
  local.saveDraft({ text: draft.value, base: p.version, savedAt: Date.now() })
}

export function acceptRemote() {
  const p = remoteVersion.value
  if (!p) return
  restore(p.text ?? '', `Version ${p.version}`)
  applyDeployed(p)
}

const versionCache = new Map<number, string>()
export async function versionText(v: number): Promise<string> {
  const cached = versionCache.get(v)
  if (cached !== undefined) return cached
  const p = await api.prompt(v)
  versionCache.set(v, p.text ?? '')
  return p.text ?? ''
}

export async function sendMessage(text: string): Promise<boolean> {
  try {
    await api.message(text)
    return true
  } catch (err) {
    fail(err)
    return false
  }
}

export async function stopAll() {
  busy.value = 'stop'
  try {
    await api.stop()
  } catch (err) {
    fail(err)
  } finally {
    busy.value = null
  }
}

export async function startRun() {
  try {
    await api.start()
  } catch (err) {
    fail(err)
  }
}

export async function addNote(text: string): Promise<boolean> {
  try {
    const n = await api.addNote(text)
    notes.value = [...notes.value, n]
    return true
  } catch (err) {
    fail(err)
    return false
  }
}

export async function saveNote(id: number, text: string): Promise<boolean> {
  try {
    await api.updateNote(id, text)
    notes.value = notes.value.map((n) => (n.id === id ? { ...n, text } : n))
    return true
  } catch (err) {
    fail(err)
    return false
  }
}

export async function deleteNote(id: number) {
  try {
    await api.deleteNote(id)
    notes.value = notes.value.filter((n) => n.id !== id)
  } catch (err) {
    fail(err)
  }
}
