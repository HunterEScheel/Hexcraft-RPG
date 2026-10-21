import { describe, expect, it } from 'vitest'
import {
  CONCENTRATION_EFFECTS,
  SPELL_CRITERIA,
  applyConcentrationRule,
  emptySpellDraft,
  needsConcentration,
  selectedOption,
  spellCost,
  type SpellDraft,
} from './spells'
import { MANEUVER_CRITERIA } from './maneuvers'

const duration = SPELL_CRITERIA.find((c) => c.key === 'duration')!
const label = (d: SpellDraft) => {
  const sel = d.selections.duration
  return `${duration.modes![sel.modeIndex].key}:${selectedOption(duration, sel)?.label}`
}
const withDuration = (d: SpellDraft, modeIndex: number, optionIndex: number): SpellDraft => ({
  ...d,
  selections: { ...d.selections, duration: { modeIndex, optionIndex } },
})

describe('concentration rule', () => {
  const pick = (label: string) => {
    const buff = SPELL_CRITERIA.find((c) => c.key === 'buffDebuff')!
    const modeIndex = buff.modes!.findIndex((m) => m.options.some((o) => o.label === label))
    const optionIndex = buff.modes![modeIndex].options.findIndex((o) => o.label === label)
    return {
      ...emptySpellDraft(),
      selections: { ...emptySpellDraft().selections, buffDebuff: { modeIndex, optionIndex } },
    }
  }

  it('splits buffs and debuffs the way 5e does', () => {
    expect(needsConcentration(emptySpellDraft())).toBe(false)
    expect(needsConcentration({ ...emptySpellDraft(), targeting: 'grit' })).toBe(false)
    expect(needsConcentration({ ...emptySpellDraft(), damageDice: 2 })).toBe(true)
    // Enhance Ability, Slow, Hold Person, Silent/Major Image, Detect Magic.
    for (const label of ['darkvision', 'tremorsense', 'see invisibility', 'truesight', 'detect lies', '+5 skill', 'slowed', 'debilitated', 'multi-sense illusions', 'magic detection'])
      expect(needsConcentration(pick(label))).toBe(true)
    // Tongues, Charm Person, Blindness/Deafness, Contagion, Command, Minor Illusion.
    for (const label of ['languages', 'charmed', 'blinded', 'illness', 'prone', '1-sense illusions'])
      expect(needsConcentration(pick(label))).toBe(false)
  })

  it('makes area spells instantaneous or concentration', () => {
    const area = (modeIndex: number) => ({
      ...emptySpellDraft(),
      selections: { ...emptySpellDraft().selections, aoe: { modeIndex, optionIndex: 0 } },
    })
    expect(needsConcentration(area(1))).toBe(true) // splash
    expect(needsConcentration(area(2))).toBe(true) // line
    // Picking targets isn't an area.
    expect(needsConcentration({ ...area(0), selections: { ...area(0).selections, aoe: { modeIndex: 0, optionIndex: 3 } } })).toBe(false)
    const lasting = withDuration(area(1), 0, 3) // 1 day
    expect(label(applyConcentrationRule(lasting))).toBe('concentration:8 hours')
  })

  it('lists only real Buff/Debuff options as needing concentration', () => {
    const buff = SPELL_CRITERIA.find((c) => c.key === 'buffDebuff')!
    const labels = new Set(buff.modes!.flatMap((m) => m.options.map((o) => o.label)))
    for (const label of CONCENTRATION_EFFECTS) expect(labels.has(label)).toBe(true)
  })

  it('moves a lasting damage spell onto concentration, keeping its length', () => {
    const d = withDuration({ ...emptySpellDraft(), damageDice: 2 }, 0, 2) // 1 hour
    expect(label(applyConcentrationRule(d))).toBe('concentration:1 hour')
    const long = withDuration(
      {
        ...emptySpellDraft(),
        selections: pick('frightened').selections,
      },
      0,
      5,
    ) // a debuff, permanent
    expect(label(applyConcentrationRule(long))).toBe('concentration:8 hours')
  })

  it('leaves an instantaneous damage spell alone', () => {
    const d = { ...emptySpellDraft(), damageDice: 4 }
    expect(applyConcentrationRule(d)).toBe(d)
  })

  it('drops concentration when there is no save or damage', () => {
    const d = withDuration(emptySpellDraft(), 1, 0) // concentration 1 round
    expect(label(applyConcentrationRule(d))).toBe('standard:1 minute')
    const hour = withDuration(emptySpellDraft(), 1, 2)
    expect(label(applyConcentrationRule(hour))).toBe('standard:1 hour')
  })

  it('does not touch maneuvers', () => {
    const d = { ...emptySpellDraft(), damageDice: 3 }
    expect(applyConcentrationRule(d, MANEUVER_CRITERIA)).toBe(d)
  })
})

describe('several buffs/debuffs', () => {
  const withEffects = (first: SpellDraft['selections']['buffDebuff'], extras: SpellDraft['selections']['buffDebuff'][]) => ({
    ...emptySpellDraft(),
    selections: { ...emptySpellDraft().selections, buffDebuff: first },
    extraEffects: extras,
  })

  it('prices each one', () => {
    // Tier I (5) + Tier II (10).
    expect(spellCost(withEffects({ modeIndex: 1, optionIndex: 0 }, [{ modeIndex: 2, optionIndex: 0 }])).baseEp).toBe(15)
  })

  it('needs concentration if any of them does', () => {
    // darkvision (C) first is enough; languages alone is not.
    expect(needsConcentration(withEffects({ modeIndex: 1, optionIndex: 2 }, []))).toBe(false)
    expect(needsConcentration(withEffects({ modeIndex: 1, optionIndex: 2 }, [{ modeIndex: 1, optionIndex: 0 }]))).toBe(true)
  })
})
