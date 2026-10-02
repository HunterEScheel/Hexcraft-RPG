import { describe, expect, it } from 'vitest'
import { applyLongRest, emptyCharacter, rollLongRest } from './character'

describe('long rest', () => {
  it('rolls 1d6 HP and 1d4-1 EP per hour slept', () => {
    const max = rollLongRest(8, (sides) => sides)
    expect(max).toEqual({ hp: 48, ep: 24 })
    const min = rollLongRest(8, () => 1)
    expect(min).toEqual({ hp: 8, ep: 0 })
    expect(rollLongRest(0)).toEqual({ hp: 0, ep: 0 })
  })

  it('heals up to the maximums and clears temp HP and death saves', () => {
    const c = { ...emptyCharacter('Peasants', 150), hp: 30, ep: 10, currentHp: 0, currentEp: 2 }
    c.deathSaves = { successes: 1, failures: 2 }
    c.tempHp = 5
    const rested = applyLongRest(c, 20, 30)
    expect(rested.currentHp).toBe(20)
    expect(rested.currentEp).toBe(10)
    expect(rested.deathSaves).toEqual({ successes: 0, failures: 0 })
    expect(rested.tempHp).toBe(0)
    expect(applyLongRest(rested, 50, 0).currentHp).toBe(30)
  })
})
