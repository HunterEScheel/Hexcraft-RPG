import { describe, expect, it } from 'vitest'
import { counterspellChance, counterspellDC } from './counterspell'

describe('counterspells', () => {
  it('succeed automatically with at least as much EP as the spell', () => {
    expect(counterspellDC(12, 16)).toBeNull()
    expect(counterspellDC(20, 20)).toBe(10) // a tie still rolls
    expect(counterspellChance(0, null)).toBe(1)
  })

  it('need DC 10 + every 10 EP the spell has over them', () => {
    expect(counterspellDC(17, 16)).toBe(10)
    expect(counterspellDC(40, 16)).toBe(12)
    expect(counterspellDC(150, 24)).toBe(22)
    expect(counterspellDC(250, 24)).toBe(32)
  })

  it('miss on a natural 1 and can become impossible', () => {
    expect(counterspellChance(20, 10)).toBe(0.95)
    expect(counterspellChance(8, 16)).toBe(0.65)
    expect(counterspellChance(8, 32)).toBe(0)
  })
})
