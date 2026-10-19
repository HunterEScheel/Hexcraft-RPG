import { describe, expect, it } from 'vitest'
import {
  SPELL_CRITERIA,
  applyConcentrationRule,
  emptySpellDraft,
  needsConcentration,
  selectedOption,
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
  it('needs concentration when it alters creatures: a buff, debuff or dice', () => {
    expect(needsConcentration(emptySpellDraft())).toBe(false)
    // A saving throw alone doesn't count.
    expect(needsConcentration({ ...emptySpellDraft(), targeting: 'grit' })).toBe(false)
    expect(needsConcentration({ ...emptySpellDraft(), damageDice: 2 })).toBe(true)
    const buffed = {
      ...emptySpellDraft(),
      selections: { ...emptySpellDraft().selections, buffDebuff: { modeIndex: 1, optionIndex: 0 } },
    }
    expect(needsConcentration(buffed)).toBe(true)
  })

  it('does not count magic detection, knowledge or illusions', () => {
    const pick = (label: string) => {
      const buff = SPELL_CRITERIA.find((c) => c.key === 'buffDebuff')!
      const modeIndex = buff.modes!.findIndex((m) => m.options.some((o) => o.label === label))
      const optionIndex = buff.modes![modeIndex].options.findIndex((o) => o.label === label)
      return {
        ...emptySpellDraft(),
        selections: { ...emptySpellDraft().selections, buffDebuff: { modeIndex, optionIndex } },
      }
    }
    for (const label of ['magic detection', 'cursory knowledge', 'total knowledge', '1-sense illusions', 'multi-sense illusions'])
      expect(needsConcentration(pick(label))).toBe(false)
    expect(needsConcentration(pick('darkvision'))).toBe(true)
    expect(needsConcentration(pick('detect lies'))).toBe(true)
  })

  it('moves a lasting damage spell onto concentration, keeping its length', () => {
    const d = withDuration({ ...emptySpellDraft(), damageDice: 2 }, 0, 2) // 1 hour
    expect(label(applyConcentrationRule(d))).toBe('concentration:1 hour')
    const long = withDuration(
      {
        ...emptySpellDraft(),
        selections: { ...emptySpellDraft().selections, buffDebuff: { modeIndex: 2, optionIndex: 0 } },
      },
      0,
      5,
    ) // a buff, permanent
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
