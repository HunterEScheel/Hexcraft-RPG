import { describe, expect, it } from 'vitest'
import { emptyMonster, monsterBpBreakdown } from './monster'
import { emptyCharacter, ensureCombatSkills, maneuverBonus, maneuverReach } from './character'
import { MANEUVER_CRITERIA, MANEUVER_SKILLS, canThrow, maneuverCriteria } from './maneuvers'
import { SPELL_CASTING_TIMES, emptySpellDraft, savedSpellCost, selectedOption, spellCost } from './spells'
import { conditionDescription } from './conditions'

describe('maneuvers', () => {
  it('are drawn only from attack skills', () => {
    expect(MANEUVER_SKILLS.map((s) => s.id)).not.toContain('combat-dodge')
    expect(MANEUVER_SKILLS.map((s) => s.id)).toContain('combat-melee-1h')
  })

  it('add the combat skill to its attribute for the bonus', () => {
    const c = ensureCombatSkills(emptyCharacter('New Adventurers', 400))
    c.attributes.Agility = 2
    c.attributes.Power = 1
    c.skills = c.skills.map((s) =>
      s.id === 'combat-melee-1h' ? { ...s, level: 3 } : s.id === 'combat-tackle' ? { ...s, level: 1 } : s,
    )
    expect(maneuverBonus(c, 'combat-melee-1h')).toBe(5) // 3 + Agility 2
    expect(maneuverBonus(c, 'combat-tackle')).toBe(2) // 1 + Power 1
  })

  it('default to none on older characters', () => {
    const legacy = { ...emptyCharacter('Peasants', 150), savedManeuvers: undefined }
    // @ts-expect-error older saved characters have no savedManeuvers
    expect(ensureCombatSkills(legacy).savedManeuvers).toEqual([])
  })

  it('price only their own criteria', () => {
    // Hamstring: slowed for 1 minute, +1d6, two actions.
    const draft = emptySpellDraft()
    draft.selections.buffDebuff = { modeIndex: 1, optionIndex: 5 } // slowed, Tier I
    draft.selections.duration = { modeIndex: 0, optionIndex: 2 } // 1 minute: effects ×2
    draft.selections.challenge = { modeIndex: 0, optionIndex: 6 } // not a maneuver criterion
    draft.damageDice = 1
    expect(spellCost(draft, MANEUVER_CRITERIA).totalEp).toBe(5 * 2 + 2)
    expect(savedSpellCost(draft, MANEUVER_CRITERIA).totalEp).toBe(9)
  })

  it('have no concentration or Challenge', () => {
    expect(MANEUVER_CRITERIA.map((c) => c.key)).not.toContain('challenge')
    const duration = MANEUVER_CRITERIA.find((c) => c.key === 'duration')!
    expect(duration.modes).toBeUndefined()
    expect(duration.options?.map((o) => o.label)).toEqual(['instantaneous', '1 round', '1 minute', '1 hour'])
    expect(duration.options?.map((o) => o.multiplier)).toEqual([1, 1, 2, 4])
  })

  it('refund EP for a self-debuff, multiplied the longer it lasts', () => {
    // Reckless Strike: advantage on the attack and +4d6, leaving you rooted.
    const draft = emptySpellDraft()
    draft.selections.buffDebuff = { modeIndex: 1, optionIndex: 0 } // advantage, 5 EP
    draft.damageDice = 4 // 8 EP
    draft.selections.selfDebuff = { modeIndex: 1, optionIndex: 0 } // rooted, −5 EP
    expect(spellCost(draft, MANEUVER_CRITERIA).totalEp).toBe(8) // 2 rounds: no extra refund
    draft.selections.selfDuration = { modeIndex: 0, optionIndex: 1 } // 5 rounds: refund ×2
    expect(spellCost(draft, MANEUVER_CRITERIA).totalEp).toBe(3)
  })

  it('refund ×12 for a self-debuff that lasts until a long rest', () => {
    const draft = emptySpellDraft()
    draft.damageDice = 40 // 80 EP
    draft.selections.selfDebuff = { modeIndex: 1, optionIndex: 0 } // rooted, −5 EP
    draft.selections.selfDuration = { modeIndex: 0, optionIndex: 4 } // until a long rest
    expect(spellCost(draft, MANEUVER_CRITERIA).baseEp).toBe(80 - 60)
  })

  it('ignore a self-debuff duration without a self-debuff, and never go below 0', () => {
    const draft = emptySpellDraft()
    draft.damageDice = 1 // 2 EP
    draft.selections.selfDuration = { modeIndex: 0, optionIndex: 3 } // 1 hour
    expect(spellCost(draft, MANEUVER_CRITERIA).totalEp).toBe(2)
    draft.selections.selfDebuff = { modeIndex: 4, optionIndex: 0 } // debilitated, −20 EP
    expect(spellCost(draft, MANEUVER_CRITERIA).totalEp).toBe(0)
  })

  it('leave spells untouched by the self-debuff picks', () => {
    const draft = emptySpellDraft()
    draft.damageDice = 3
    draft.selections.selfDebuff = { modeIndex: 4, optionIndex: 0 }
    expect(spellCost(draft).totalEp).toBe(6)
  })

  it('describe every self-debuff', () => {
    const self = MANEUVER_CRITERIA.find((c) => c.key === 'selfDebuff')!
    const labels = self.modes!.flatMap((m) => m.options.map((o) => o.label)).filter((l) => l !== 'none')
    const undefinedYet = labels.filter((l) => !conditionDescription(l))
    expect(undefinedYet).toEqual([])
    expect(selectedOption(self, { modeIndex: 1, optionIndex: 1 })?.label).toBe('exposed')
  })

  it('rewrite removed effects on older characters, once', () => {
    const pick = (modeIndex: number, optionIndex: number) => {
      const d = emptySpellDraft()
      d.selections.buffDebuff = { modeIndex, optionIndex }
      return d
    }
    const spell = (id: string, draft: ReturnType<typeof pick>) => ({
      id, name: id, school: 'Control', medium: 'Cognition', draft,
    })
    // Picks as saved before any removal: Tier IV was stunned, paralyzed,
    // incapacitated, banished, ...; Tier V was petrification, soul-bound, death.
    const old = {
      ...emptyCharacter('Peasants', 150),
      rulesVersion: undefined,
      savedSpells: [
        spell('incapacitate', pick(4, 2)),
        spell('banish', pick(4, 3)),
        spell('paralyze', pick(4, 1)),
        spell('petrify', pick(5, 0)),
        spell('kill', pick(5, 2)),
      ],
      savedManeuvers: [{ id: 'k', name: 'Knockout', skillId: 'combat-unarmed', draft: pick(4, 1) }],
    }
    const sels = (c: ReturnType<typeof ensureCombatSkills>) =>
      c.savedSpells.map((s) => [s.draft.selections.buffDebuff.modeIndex, s.draft.selections.buffDebuff.optionIndex])
    const once = ensureCombatSkills(old)
    // debilitated, banished, debilitated, debilitated, death
    expect(sels(once)).toEqual([[4, 0], [4, 1], [4, 0], [4, 0], [5, 1]])
    expect(once.savedManeuvers[0].draft.selections.buffDebuff).toEqual({ modeIndex: 4, optionIndex: 0 })
    expect(sels(ensureCombatSkills(once))).toEqual(sels(once))
  })
})

describe('maneuver timing', () => {
  it('takes no action at eight times the price', () => {
    const draft = { ...emptySpellDraft(), damageDice: 2, castingTime: 'none' as const }
    expect(spellCost(draft, MANEUVER_CRITERIA).totalEp).toBe(32)
    expect(SPELL_CASTING_TIMES.some((t) => t.key === 'none')).toBe(false)
  })
})

describe('several self-debuffs', () => {
  it('refund each one, all multiplied by the shared duration', () => {
    const base = { ...emptySpellDraft(), damageDice: 10 } // 20 EP
    const one = {
      ...base,
      selections: { ...base.selections, selfDebuff: { modeIndex: 1, optionIndex: 0 } }, // −5
    }
    const two = { ...one, extraSelfDebuffs: [{ modeIndex: 2, optionIndex: 0 }] } // −10 more
    expect(spellCost(one, MANEUVER_CRITERIA).baseEp).toBe(15)
    expect(spellCost(two, MANEUVER_CRITERIA).baseEp).toBe(5)
    // Removing the first still leaves the duration counting for the extra.
    const extraOnly = { ...base, extraSelfDebuffs: [{ modeIndex: 1, optionIndex: 0 }] }
    const lasting = {
      ...extraOnly,
      selections: { ...extraOnly.selections, selfDuration: { modeIndex: 0, optionIndex: 1 } },
    }
    expect(spellCost(lasting, MANEUVER_CRITERIA).baseEp).toBe(10) // 20 − 5 × 2
    const both = { ...two, selections: { ...two.selections, selfDuration: { modeIndex: 0, optionIndex: 2 } } }
    expect(spellCost(both, MANEUVER_CRITERIA).baseEp).toBe(0) // 20 − 15 × 4, floored at 0
  })
})

describe('maneuver reach', () => {
  const labels = (criteria: ReturnType<typeof maneuverCriteria>, key: string, mode?: string) => {
    const c = criteria.find((x) => x.key === key)!
    const opts = mode ? c.modes!.find((m) => m.key === mode)!.options : c.options!
    return opts.map((o) => o.label)
  }

  it('have no range for close skills, just a line by speed and 5–15 ft splashes', () => {
    const c = maneuverCriteria('combat-melee-1h', { speed: 25 })
    expect(c.map((x) => x.key)).not.toContain('range')
    expect(labels(c, 'aoe', 'line')).toEqual(['≤ 25 ft line', '≤ 50 ft line', '≤ 100 ft line', '≤ 200 ft line'])
    expect(labels(c, 'aoe', 'splash')).toEqual(['5 ft splash', '10 ft splash', '15 ft splash'])
    const aoe = c.find((x) => x.key === 'aoe')!
    const line = aoe.modes!.find((m) => m.key === 'line')!
    expect(line.options.map((o) => o.tier * aoe.epPerTier)).toEqual([5, 10, 20, 40])
  })

  it('run a close line a move per action the timing takes', () => {
    const line = (time: Parameters<typeof maneuverCriteria>[2], speed?: number) =>
      labels(maneuverCriteria('combat-tackle', speed ? { speed } : {}, time), 'aoe', 'line')
    expect(line('2actions', 25)).toEqual(['≤ 50 ft line', '≤ 100 ft line', '≤ 200 ft line', '≤ 400 ft line'])
    expect(line('1round', 25)[0]).toBe('≤ 100 ft line')
    expect(line('2actions')[0]).toBe('≤ 2× speed line')
    // A fired maneuver's reach doesn't care how long it takes.
    const fired = maneuverCriteria('combat-fired-1h', { maxRange: 80 }, '1round')
    expect(labels(fired, 'aoe', 'line')[0]).toBe('≤ 80 ft line')
  })

  it('goes by weapon range for fired skills', () => {
    const c = maneuverCriteria('combat-fired-2h', { minRange: 30, maxRange: 800 })
    expect(labels(c, 'range')).toEqual([
      'within range (30–800 ft)', '>2× max range (1,600 ft)', '>3× max range (2,400 ft)', '>4× max range (3,200 ft)',
    ])
    expect(labels(maneuverCriteria('combat-fired-1h'), 'range')).toEqual([
      'within range', '>2× max range', '>3× max range', '>4× max range',
    ])
    expect(labels(c, 'aoe', 'splash')).toContain('300 ft splash')
  })

  it('throws 120 ft + 30 ft per Power, whatever the Throw level', () => {
    const c = ensureCombatSkills(emptyCharacter('New Adventurers', 400))
    c.attributes.Power = 2
    c.skills = c.skills.map((s) => (s.id === 'combat-throw' ? { ...s, level: 3 } : s))
    expect(maneuverReach(c, 'combat-throw')).toEqual({ minRange: 0, maxRange: 180 })
    expect(maneuverBonus(c, 'combat-throw')).toBe(3 + c.attributes.Agility)
    expect(maneuverReach(c, 'combat-tackle')).toEqual({ speed: c.speed })
  })

  it('cannot throw at Power −4 or lower', () => {
    expect(canThrow(-3)).toBe(true) // 30 ft
    expect(canThrow(-4)).toBe(false)
    expect(canThrow(-5)).toBe(false)
  })

  it('reads a fired skill off the equipped weapon', () => {
    const c = ensureCombatSkills(emptyCharacter('New Adventurers', 400))
    c.inventory = [{ id: 'r', name: 'Revolver', quantity: 1, weaponCategory: 'fired-1h', equipped: true, minRange: 0, maxRange: 80 }]
    expect(maneuverReach(c, 'combat-fired-1h')).toEqual({ minRange: 0, maxRange: 80 })
  })

  it('price range at 0, 4, 8 and 16 EP for fired skills, and not at all for close ones', () => {
    const fired = maneuverCriteria('combat-fired-2h', { maxRange: 800 })
    const range = fired.find((x) => x.key === 'range')!
    expect(range.options!.map((o) => o.tier * range.epPerTier)).toEqual([0, 4, 8, 16])
    const draft = emptySpellDraft()
    draft.selections.range = { modeIndex: 0, optionIndex: 3 } // 16 EP
    draft.selections.aoe = { modeIndex: 1, optionIndex: 2 } // 15 ft or 120 ft splash, 15 EP
    expect(spellCost(draft, fired).baseEp).toBe(31)
    expect(spellCost(draft, maneuverCriteria('combat-melee-2h', { speed: 30 })).baseEp).toBe(15)
  })
})

describe('monster skills', () => {
  it('cost BP on the skill curve alongside combat skills', () => {
    const m = emptyMonster('New Adventurers', 400)
    const before = monsterBpBreakdown(m).total
    m.skills = [{ id: 's', name: 'Stealth', level: 3 }]
    const after = monsterBpBreakdown(m)
    expect(after.otherSkills).toBe(10)
    expect(after.total).toBe(before + 10)
  })
})
