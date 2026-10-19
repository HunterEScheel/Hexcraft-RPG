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
  it('needs concentration with a saving throw or damage dice', () => {
    expect(needsConcentration(emptySpellDraft())).toBe(false)
    expect(needsConcentration({ ...emptySpellDraft(), targeting: 'grit' })).toBe(true)
    expect(needsConcentration({ ...emptySpellDraft(), damageDice: 2 })).toBe(true)
  })

  it('moves a lasting damage spell onto concentration, keeping its length', () => {
    const d = withDuration({ ...emptySpellDraft(), damageDice: 2 }, 0, 2) // 1 hour
    expect(label(applyConcentrationRule(d))).toBe('concentration:1 hour')
    const long = withDuration({ ...emptySpellDraft(), targeting: 'dodge' }, 0, 5) // permanent
    expect(label(applyConcentrationRule(long))).toBe('concentration:8 hours')
  })

  it('leaves an instantaneous save spell alone', () => {
    const d = { ...emptySpellDraft(), targeting: 'resolve' as const }
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
