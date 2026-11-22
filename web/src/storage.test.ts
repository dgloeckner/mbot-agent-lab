import { describe, expect, it } from 'vitest'
import { LocalStore, MAX_SNAPSHOTS, changeSize, memoryKV, shouldAutoSnapshot, type KV } from './storage'

describe('changeSize', () => {
  it('counts only the changed middle', () => {
    expect(changeSize('abc', 'abc')).toBe(0)
    expect(changeSize('Fahr los', 'Fahr schnell los')).toBe(8)
    expect(changeSize('', 'neu')).toBe(3)
  })
})

describe('shouldAutoSnapshot', () => {
  const snap = (text: string, at: number) => ({ id: 'x', at, text, reason: 'auto' as const })
  it('snapshots the first non-empty text', () => {
    expect(shouldAutoSnapshot(undefined, 'a', 0)).toBe(true)
    expect(shouldAutoSnapshot(undefined, '   ', 0)).toBe(false)
  })
  it('waits for a pause or a larger change', () => {
    const s = snap('Fahr los', 1000)
    expect(shouldAutoSnapshot(s, 'Fahr los!', 2000)).toBe(false)
    expect(shouldAutoSnapshot(s, 'Fahr los!', 31_000)).toBe(true)
    expect(shouldAutoSnapshot(s, 'Fahr los' + 'x'.repeat(40), 2000)).toBe(true)
    expect(shouldAutoSnapshot(s, 'Fahr los', 99_000)).toBe(false)
  })
})

describe('LocalStore', () => {
  it('keeps drafts per team', () => {
    const kv = memoryKV()
    new LocalStore(kv, 'rot').saveDraft({ text: 'rot', base: 1, savedAt: 1 })
    expect(new LocalStore(kv, 'blau').loadDraft()).toBeNull()
    expect(new LocalStore(kv, 'rot').loadDraft()?.text).toBe('rot')
  })

  it('adds snapshots newest first, skips duplicates and caps the list', () => {
    const s = new LocalStore(memoryKV(), 'rot')
    s.addSnapshot('a', 'auto', undefined, 1)
    s.addSnapshot('a', 'auto', undefined, 2)
    expect(s.history()).toHaveLength(1)
    const h = s.addSnapshot('a', 'deploy', 3, 3)
    expect(h[0].reason).toBe('deploy')
    expect(h[0].version).toBe(3)
    for (let i = 0; i < MAX_SNAPSHOTS + 5; i++) s.addSnapshot(`t${i}`, 'auto', undefined, 10 + i)
    const all = s.history()
    expect(all).toHaveLength(MAX_SNAPSHOTS)
    expect(all[0].text).toBe(`t${MAX_SNAPSHOTS + 4}`)
  })

  it('shrinks the history when the quota is full', () => {
    const inner = memoryKV()
    const limited: KV = {
      ...inner,
      setItem(k, v) {
        if (v.length > 400) throw new Error('QuotaExceededError')
        inner.setItem(k, v)
      },
    }
    const s = new LocalStore(limited, 'rot')
    for (let i = 0; i < 20; i++) s.addSnapshot('x'.repeat(30) + i, 'auto', undefined, i)
    const h = s.history()
    expect(h.length).toBeGreaterThan(0)
    expect(h.length).toBeLessThan(20)
    expect(h[0].text.endsWith('19')).toBe(true)
  })

  it('falls back to defaults for broken prefs', () => {
    const kv = memoryKV()
    kv.setItem('mbot-lab:v1:prefs', '{not json')
    expect(LocalStore.loadPrefs(kv).fontSize).toBe(15)
    LocalStore.savePrefs(kv, { theme: 'dark', split: 5, fontSize: 99, sideTab: 'notes' })
    const p = LocalStore.loadPrefs(kv)
    expect(p).toMatchObject({ theme: 'dark', split: 0.8, fontSize: 22, sideTab: 'notes' })
  })
})
