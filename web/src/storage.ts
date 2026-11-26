// Browser-local persistence: the current draft, a history of draft snapshots
// and UI preferences. Deployed versions live on the server; this only covers
// what has not been deployed. Everything degrades to memory when storage is
// unavailable (private mode, blocked site data, full quota).

export interface KV {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
  removeItem(key: string): void
  keys(): string[]
}

export function memoryKV(): KV {
  const m = new Map<string, string>()
  return {
    getItem: (k) => m.get(k) ?? null,
    setItem: (k, v) => void m.set(k, v),
    removeItem: (k) => void m.delete(k),
    keys: () => [...m.keys()],
  }
}

export function browserKV(): KV {
  try {
    const ls = window.localStorage
    const probe = '__mbot_probe__'
    ls.setItem(probe, '1')
    ls.removeItem(probe)
    return {
      getItem: (k) => ls.getItem(k),
      setItem: (k, v) => ls.setItem(k, v),
      removeItem: (k) => ls.removeItem(k),
      keys: () => Object.keys(ls),
    }
  } catch {
    return memoryKV()
  }
}

// Drafts and snapshots are kept per (team, task) under NS. Keys of older
// namespaces are dropped (full reset with tasks). Preferences are per browser.
const NS = 'mbot-lab:v2'
const PREFS_KEY = 'mbot-lab:v1:prefs'
const OLD_PREFIX = 'mbot-lab:v1:'

export interface Draft {
  text: string
  base: number // server version the draft was last synced with, 0 if none
  savedAt: number
}

export type SnapshotReason = 'auto' | 'manual' | 'deploy' | 'restore'

export interface Snapshot {
  id: string
  at: number
  text: string
  reason: SnapshotReason
  version?: number
}

export type Theme = 'system' | 'light' | 'dark'
export type SideTab = 'task' | 'log' | 'conversation' | 'notes' | 'tools'

export interface Prefs {
  theme: Theme
  split: number // share of the width for the editor, 0.3 .. 0.8
  fontSize: number // editor font size in px
  sideTab: SideTab
}

export const defaultPrefs: Prefs = { theme: 'system', split: 0.58, fontSize: 15, sideTab: 'log' }

export const MAX_SNAPSHOTS = 80
const AUTO_MIN_GAP_MS = 30_000
const AUTO_MIN_CHANGE = 40

function read<T>(kv: KV, key: string): T | null {
  try {
    const raw = kv.getItem(key)
    return raw == null ? null : (JSON.parse(raw) as T)
  } catch {
    return null
  }
}

function write(kv: KV, key: string, value: unknown): boolean {
  try {
    kv.setItem(key, JSON.stringify(value))
    return true
  } catch {
    return false
  }
}

/** Number of characters that differ between a and b, ignoring the common prefix and suffix. */
export function changeSize(a: string, b: string): number {
  let start = 0
  const min = Math.min(a.length, b.length)
  while (start < min && a[start] === b[start]) start++
  let end = 0
  while (end < min - start && a[a.length - 1 - end] === b[b.length - 1 - end]) end++
  return Math.max(a.length, b.length) - start - end
}

/** Whether an automatic snapshot is due for text, given the newest snapshot. */
export function shouldAutoSnapshot(newest: Snapshot | undefined, text: string, now: number): boolean {
  if (!text.trim()) return false
  if (!newest) return true
  if (newest.text === text) return false
  return now - newest.at >= AUTO_MIN_GAP_MS || changeSize(newest.text, text) >= AUTO_MIN_CHANGE
}

/** Removes drafts and snapshots stored before tasks existed. Preferences stay. */
export function dropOldKeys(kv: KV): void {
  try {
    for (const k of kv.keys()) {
      if (k.startsWith(OLD_PREFIX) && k !== PREFS_KEY && k !== JOINED_KEY) kv.removeItem(k)
    }
  } catch {
    // storage unavailable: nothing to drop
  }
}

/** Draft and snapshots of one (team, task) pair in this browser. */
export class LocalStore {
  constructor(
    private kv: KV,
    private teamId: string,
    private taskId: string,
  ) {}

  private key(name: string) {
    return `${NS}:${this.teamId}:${this.taskId}:${name}`
  }

  loadDraft(): Draft | null {
    const d = read<Draft>(this.kv, this.key('draft'))
    return d && typeof d.text === 'string' ? d : null
  }

  saveDraft(d: Draft): boolean {
    return write(this.kv, this.key('draft'), d)
  }

  history(): Snapshot[] {
    const h = read<Snapshot[]>(this.kv, this.key('history'))
    return Array.isArray(h) ? h : []
  }

  /** Adds a snapshot unless it equals the newest one. Returns the new history. */
  addSnapshot(text: string, reason: SnapshotReason, version?: number, now = Date.now()): Snapshot[] {
    const h = this.history()
    if (h[0]?.text === text) {
      if (reason !== 'auto' && h[0].reason === 'auto') {
        h[0] = { ...h[0], reason, version }
        this.saveHistory(h)
      }
      return h
    }
    const snap: Snapshot = { id: `${now.toString(36)}-${Math.random().toString(36).slice(2, 6)}`, at: now, text, reason, version }
    return this.saveHistory([snap, ...h].slice(0, MAX_SNAPSHOTS))
  }

  removeSnapshot(id: string): Snapshot[] {
    return this.saveHistory(this.history().filter((s) => s.id !== id))
  }

  /** Writes the history, dropping the oldest half until it fits the quota. */
  private saveHistory(h: Snapshot[]): Snapshot[] {
    let list = h
    while (!write(this.kv, this.key('history'), list) && list.length > 1) {
      list = list.slice(0, Math.ceil(list.length / 2))
    }
    return list
  }

  static loadPrefs(kv: KV): Prefs {
    const p = read<Partial<Prefs>>(kv, PREFS_KEY) ?? {}
    const merged = { ...defaultPrefs, ...p }
    merged.split = Math.min(0.8, Math.max(0.3, Number(merged.split) || defaultPrefs.split))
    merged.fontSize = Math.min(22, Math.max(12, Number(merged.fontSize) || defaultPrefs.fontSize))
    return merged
  }

  static savePrefs(kv: KV, p: Prefs): void {
    write(kv, PREFS_KEY, p)
  }
}

// ---------- teams this browser has joined ----------

/** A team this browser has joined, so it finds its way back without the link. */
export interface Joined {
  name: string
  token: string
  color: string
}
const JOINED_KEY = 'mbot-lab:v1:joined'

export function loadJoined(kv: KV): Joined[] {
  try {
    const v = JSON.parse(kv.getItem(JOINED_KEY) ?? '[]')
    return Array.isArray(v) ? v : []
  } catch {
    return []
  }
}

export function rememberJoined(kv: KV, j: Joined) {
  const list = [j, ...loadJoined(kv).filter((x) => x.token !== j.token)].slice(0, 5)
  try {
    kv.setItem(JOINED_KEY, JSON.stringify(list))
  } catch {
    /* storage full or blocked: the link is still shown */
  }
}
