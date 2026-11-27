import { describe, expect, it } from 'vitest'
import { cleanLabCode, codeFromSearch, joinLink } from './link'

describe('join link', () => {
  it('carries the lab code to the join page', () => {
    const link = joinLink('http://lab.example:8080', 'K7P3')
    expect(link).toBe('http://lab.example:8080/join?code=K7P3')
    expect(codeFromSearch(new URL(link).search)).toBe('K7P3')
  })

  it('reads a code like the form does', () => {
    expect(codeFromSearch('?code=k7p3')).toBe('K7P3')
    expect(codeFromSearch('?code=K7-P3XYZ')).toBe('K7P3')
    expect(codeFromSearch('')).toBe('')
    expect(codeFromSearch('?other=1')).toBe('')
    expect(cleanLabCode(' ab 12 ')).toBe('AB12')
  })
})
